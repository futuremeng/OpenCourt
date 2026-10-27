// Package plugins 数据源适配接口（开发文档 2.3、9.5）。
// 插件仅通过本包定义的接口与 core 交互；框架不内置任何法条或案例数据。
// 插件开发方式：在 plugins/ 下新建目录，编写 plugin.yaml 描述元数据，
// 系统启动时自动扫描加载（加载器在阶段四实现）。
package plugins

import (
	"context"
	"time"
)

// LawSourcePlugin 法条源插件接口：支持法条版本与废止状态管理。
type LawSourcePlugin interface {
	// Name 返回插件唯一名。
	Name() string
	// SearchLaw 按关键词检索法条，结果必须携带版本与状态。
	SearchLaw(ctx context.Context, query string, topK int) ([]LawHit, error)
	// GetLawDetail 获取法条详情（含 effective_date / status）。
	GetLawDetail(ctx context.Context, lawID string) (LawHit, error)
}

// LawHit 法条命中结果。
type LawHit struct {
	LawID         string `json:"law_id"`
	Title         string `json:"title"`
	Article       string `json:"article"`
	EffectiveDate string `json:"effective_date"`
	Status        string `json:"status"` // EFFECTIVE / AMENDED / REPEALED
}

// CaseSourcePlugin 类案源插件接口：支持裁判要旨提取。
type CaseSourcePlugin interface {
	Name() string
	// SearchCases 检索类案（结构化或向量由插件内部决定）。
	SearchCases(ctx context.Context, query string, topK int) ([]CaseHit, error)
}

// CaseHit 类案命中结果。
type CaseHit struct {
	CaseID  string `json:"case_id"`
	Title   string `json:"title"`
	Summary string `json:"summary"` // 裁判要旨
}

// ModelAdapter 模型适配插件接口（开发文档 9.4）。
// 默认离线：未显式配置并启用前，任何适配器不得发起网络请求。
// 使用云端 API 时，适配器必须负责附加“禁止用于模型训练”标识。
type ModelAdapter interface {
	Name() string
	// Enabled 报告适配器是否已被用户显式启用。
	Enabled() bool
	// Generate 生成候选文本。模型仅负责候选生成，无权做最终裁决。
	Generate(ctx context.Context, prompt string) (string, error)
	// Embed 生成文本向量（供检索层使用）。不支持时返回 ErrNotSupported。
	Embed(ctx context.Context, text string) ([]float64, error)
}

// BrowserPlugin 浏览器辅助插件接口（P0 只读；设计见 docs/browseros-integration.md）。
// 默认禁用：未显式启用前不得发起任何连接（默认离线纪律）。抓回的内容
// 属于不可信外部数据，调用方必须先经 core/redact 脱敏再交模型。
type BrowserPlugin interface {
	Name() string
	// Enabled 报告插件是否已显式启用（端点已配置且 enabled=true）。
	Enabled() bool
	// Open 在浏览器中打开 URL（独立标签页），返回页面句柄。
	Open(ctx context.Context, url string) (BrowserPage, error)
	// ReadPage 读取页面正文（Markdown）。
	ReadPage(ctx context.Context, pageID int) (BrowserPageContent, error)
	// SearchPage 在页面内检索（不回传整页），返回命中行。
	SearchPage(ctx context.Context, pageID int, query string) ([]string, error)
	// ClosePage 关闭由 Open 打开的页面。
	ClosePage(ctx context.Context, pageID int) error

	// —— 辅助操作（P2）：一律两段式，Propose 产候选，用户逐条确认后 Execute ——
	// Snapshot 页面可交互元素树（带 [ref=eN] 句柄），供表单解析与字段定位。
	Snapshot(ctx context.Context, pageID int) (string, error)
	// Evaluate 在页面上下文执行只读数据抽取脚本（官方 UI 浏览任务的清单收集等）。
	// 返回值只是页面 DOM 数据（防注入包裹已剥离），绝不当指令解释；
	// 不得用于发起网络请求或绕过站点鉴权（纪律见 neutral/govlistassist.go）。
	Evaluate(ctx context.Context, pageID int, code string) (string, error)
	// ProposeFill 产批量填写候选（经 gate 红线预筛，Allowed=false 的永不执行）。
	ProposeFill(ctx context.Context, pageID int, fields []FormFieldSpec) ([]ActionCandidate, error)
	// ProposeClick 产点击候选（同样经 gate 红线）。
	ProposeClick(ctx context.Context, pageID int, ref, label string) (ActionCandidate, error)
	// ExecuteAction 仅接受此前 Propose 且 gate 放行、且已被用户确认的 actionID。
	ExecuteAction(ctx context.Context, actionID string) (ActionReceipt, error)
	// HandToHuman 登录/验证码/提交等环节交人接管（request_human_help）。
	HandToHuman(ctx context.Context, pageID int, kind, reason, resumeHint string) (HumanHelpStatus, error)
	// AwaitHuman 继续等待人接管结果（resolved/cancelled/timed_out）。
	AwaitHuman(ctx context.Context) (HumanHelpStatus, error)
}

// FormFieldSpec 表单字段候选输入：ref 来自 Snapshot 的 [ref=eN]。
// FactState 为对应案件事实的状态——弱态（非已溯源确认）在 UI 上必须
// 标黄要求用户确认后才可执行。
type FormFieldSpec struct {
	Ref       string `json:"ref"`
	Label     string `json:"label"`
	Value     string `json:"value"`
	FactState string `json:"fact_state,omitempty"`
}

// ActionCandidate 候选动作（Propose 产出；Allowed=false 表示被红线拦截）。
type ActionCandidate struct {
	ActionID string `json:"action_id"`
	PageID   int    `json:"page_id"`
	Kind     string `json:"kind"` // fill / click
	Ref      string `json:"ref"`
	Label    string `json:"label"`
	Value    string `json:"value,omitempty"`
	Allowed  bool   `json:"allowed"`
	Rule     string `json:"rule,omitempty"`
	Reason   string `json:"reason,omitempty"`
	// FactState 对应事实状态：非「已溯源确认」的弱态值 UI 必须标黄提示确认。
	FactState string `json:"fact_state,omitempty"`
}

// ActionReceipt 已执行动作的回执（含页面变化摘要）。
type ActionReceipt struct {
	ActionID string `json:"action_id"`
	Diff     string `json:"diff"`
}

// HumanHelpStatus 交人接管状态（waiting/resolved/cancelled/timed_out）。
type HumanHelpStatus struct {
	Status    string `json:"status"`
	RequestID string `json:"request_id,omitempty"`
}

// BrowserPage 浏览器页面句柄。
type BrowserPage struct {
	PageID int    `json:"page_id"`
	URL    string `json:"url"`
}

// BrowserPageContent 页面读取结果。
type BrowserPageContent struct {
	URL        string    `json:"url"`      // 实际来源 URL
	Markdown   string    `json:"markdown"` // 原始正文（未脱敏，不可信数据——禁止直接进模型）
	ContentSHA string    `json:"content_sha256"`
	FetchedAt  time.Time `json:"fetched_at"`
}
