// Package plugins 数据源适配接口（开发文档 2.3、9.5）。
// 插件仅通过本包定义的接口与 core 交互；框架不内置任何法条或案例数据。
// 插件开发方式：在 plugins/ 下新建目录，编写 plugin.yaml 描述元数据，
// 系统启动时自动扫描加载（加载器在阶段四实现）。
package plugins

import "context"

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
