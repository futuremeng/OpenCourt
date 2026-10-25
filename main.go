// OpenCourt 桌面应用入口（Wails v2）。
// 桌面端为主分发形态：单应用、数据全本地、默认离线。
// Go 侧组合四层架构（core/neutral/profiles/adversary），通过 Wails bindings
// 直接暴露给 Vue 前端，无 HTTP 服务、无端口暴露。
package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/futuremeng/OpenCourt/config"
	"github.com/futuremeng/OpenCourt/core/audit"
	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/gate"
	"github.com/futuremeng/OpenCourt/core/ingest"
	"github.com/futuremeng/OpenCourt/core/lawinstall"
	"github.com/futuremeng/OpenCourt/core/redact"
	"github.com/futuremeng/OpenCourt/core/statemachine"
	"github.com/futuremeng/OpenCourt/core/storage"
	"github.com/futuremeng/OpenCourt/neutral"
	"github.com/futuremeng/OpenCourt/plugins"
	"github.com/futuremeng/OpenCourt/profiles"

	"github.com/futuremeng/OpenLaw/pack"
	"github.com/futuremeng/OpenLaw/store"
)

//go:embed all:frontend/dist
var assets embed.FS

// version 应用版本（Semantic Versioning）。
const version = "0.1.0-preview"

// App 应用服务：Wails bindings 的绑定对象。
type App struct {
	ctx      context.Context
	dataDir  string
	settings *config.Settings
	profile  profiles.Profile
	store    *storage.Store
	auditLog *audit.Logger
	model    plugins.ModelAdapter
	// lawSource OpenLaw 法条源（M4 对接）：nil = 未配置，法条检索降级不可用。
	lawSource plugins.LawSourcePlugin
	ingest    *ingest.Registry
}

// appDataDir 返回本机应用数据目录（默认 ~/Library/Application Support/OpenCourt 等），
// 数据不出域。
func appDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "OpenCourt")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// NewApp 创建应用实例。
func NewApp() *App {
	return &App{}
}

// startup 初始化：加载配置与 Profile，打开本地 SQLite 与审计日志。
// 数据全部位于用户本机应用数据目录，不出域。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	dataDir, err := appDataDir()
	if err != nil {
		log.Fatalf("无法创建本机数据目录: %v", err)
	}
	a.dataDir = dataDir
	// .env 注入敏感信息（API Key 等），文件不入库；已存在的环境变量优先。
	if err := config.LoadDotEnv(".env"); err != nil {
		log.Printf(".env 加载失败: %v", err)
	}
	settings, err := loadSettings(dataDir)
	if err != nil {
		log.Printf("配置加载失败，使用默认配置: %v", err)
		settings = config.Default()
	}
	a.settings = settings

	profile, err := profiles.Load("profiles", settings.Profile.Active)
	if err != nil {
		log.Printf("Profile 加载失败，回退到 court: %v", err)
		profile, _ = profiles.Load("profiles", "court")
	}
	a.profile = profile

	store, err := storage.Open(dataDir + "/opencourt.db")
	if err != nil {
		log.Fatalf("本地数据库打开失败: %v", err)
	}
	a.store = store

	logger, err := audit.NewLogger(dataDir + "/audit.jsonl")
	if err != nil {
		log.Fatalf("审计日志初始化失败: %v", err)
	}
	a.auditLog = logger

	_ = a.auditLog.Log(settings.Profile.Active, "SESSION_START", "app", "版本 "+version)

	// 模型适配器：统一 OpenAI 兼容接口；默认禁用（默认离线安全）。
	// API Key 只从配置指定的环境变量读取，严禁落盘。
	apiKey := ""
	if settings.Model.APIKeyEnv != "" {
		apiKey = os.Getenv(settings.Model.APIKeyEnv)
	}
	a.model = plugins.NewOpenAIAdapter(
		settings.Model.BaseURL, apiKey,
		settings.Model.ReasoningModel, settings.Model.EmbeddingModel,
		settings.Model.Enabled,
	)
	a.ingest = ingest.NewRegistry()

	// OpenLaw 法条源（M4 对接）：进程内 sdk，禁止直连数据库；未配置 → nil 降级。
	// 打开失败不阻断启动（法条检索不可用 + 审计留痕），其余功能不受影响。
	// 桌面分发：未配置（或配置路径失效）且分发包内带 law-data（dmg 卷内）→
	// 首次启动自动安装到本机应用数据目录（一次性、幂等；SQLite 需本地读写）。
	if exe, err := os.Executable(); err == nil {
		if bundled := lawinstall.FindBundled(exe); bundled != "" {
			settings.OpenLaw.DataDir = a.ensureLawData(settings, bundled)
		} else if home, err := os.UserHomeDir(); err == nil {
			downloads := filepath.Join(home, "Downloads")
			if dl := lawinstall.FindDownloaded(downloads); dl != "" {
				settings.OpenLaw.DataDir = a.ensureLawData(settings, dl)
			}
			// 证据层包（docx/md/provenance）：核心包已就位时自动合并，
			// 合并后溯源链补全（UNVERIFIED → 完整 trace）
			if ev := lawinstall.FindEvidence(downloads); ev != "" &&
				settings.OpenLaw.DataDir != "" && dirExists(settings.OpenLaw.DataDir) {
				if err := lawinstall.MergeEvidence(ev, settings.OpenLaw.DataDir, func(s string) {
					log.Printf("法条证据层: %s", s)
				}); err != nil {
					log.Printf("法条证据层合并失败: %v", err)
					_ = a.auditLog.Log(settings.Profile.Active, "LAW_EVIDENCE_FAIL", "lawinstall", err.Error())
				} else {
					_ = a.auditLog.Log(settings.Profile.Active, "LAW_EVIDENCE_MERGED", "lawinstall", ev)
				}
			}
		}
	}
	a.openLawSource()
}

// openLawSource 按当前设置打开法条源（启动时与数据包加载后复用）。
// 打开失败不阻断（法条检索不可用 + 审计留痕），其余功能不受影响。
func (a *App) openLawSource() {
	a.closeLawSource()
	src, err := plugins.NewOpenLawSource(a.settings.OpenLaw.DataDir)
	if err != nil {
		log.Printf("OpenLaw 法条源初始化失败（法条检索不可用）: %v", err)
		_ = a.auditLog.Log(a.settings.Profile.Active, "LAW_SOURCE_FAIL", "openlaw", err.Error())
	}
	a.lawSource = src
	if src != nil {
		_ = a.auditLog.Log(a.settings.Profile.Active, "LAW_SOURCE_OK", "openlaw", src.Name())
	}
}

// closeLawSource 关闭法条源句柄（替换语料前必须释放 SQLite 文件锁）。
func (a *App) closeLawSource() {
	if a.lawSource == nil {
		return
	}
	if c, ok := a.lawSource.(interface{ Close() error }); ok {
		_ = c.Close()
	}
	a.lawSource = nil
}

// loadSettings 配置加载顺序（桌面分发）：
//  1. <应用数据目录>/settings.yaml —— 正式位置：双击 .app 启动（工作目录不定）也可读
//  2. config/settings.yaml（相对工作目录）—— 开发模式
//  3. 都没有 → 默认配置（OPENCOURT_* 环境变量覆盖仍生效）
//
// 首个候选解析失败不中断：记录后回退下一候选（与「配置坏不挡启动」一致）。
func loadSettings(appDir string) (*config.Settings, error) {
	for _, p := range []string{filepath.Join(appDir, "settings.yaml"), "config/settings.yaml"} {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		s, err := config.Load(p)
		if err != nil {
			log.Printf("配置文件解析失败（尝试下一来源）: %s: %v", p, err)
			continue
		}
		log.Printf("配置来源: %s", p)
		return s, nil
	}
	return config.Load("") // 不存在 → 默认 + 环境变量覆盖
}

// ensureLawData 把发现的语料源（分发包内 / 下载目录解压）安装到本机应用
// 数据目录并返回应使用的路径；用户已配置有效路径 → 原样返回不动。
func (a *App) ensureLawData(settings *config.Settings, source string) string {
	if settings.OpenLaw.DataDir != "" && dirExists(settings.OpenLaw.DataDir) {
		return settings.OpenLaw.DataDir
	}
	target := filepath.Join(a.dataDir, "law-data")
	if err := lawinstall.Install(source, target, func(s string) {
		log.Printf("法条数据安装: %s", s)
	}); err != nil {
		log.Printf("法条数据安装失败: %v", err)
		_ = a.auditLog.Log(settings.Profile.Active, "LAW_DATA_INSTALL_FAIL", "lawinstall", err.Error())
		return settings.OpenLaw.DataDir
	}
	_ = a.auditLog.Log(settings.Profile.Active, "LAW_DATA_INSTALLED", "lawinstall", target)
	return target
}

// dirExists 目录存在判据（配置路径有效性检查）。
func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// --- 法条数据管理（数据包加载与更新，前端「法条数据」面板） ---

// LawDataStatus 语料概况：是否已装、文档/条文计数、证据层与签名 manifest
// 状态。前端据此在首屏提示「加载数据包」或「加载更新包」。
func (a *App) LawDataStatus() map[string]any {
	st := lawinstall.Status(a.settings.OpenLaw.DataDir)
	return map[string]any{
		"installed":       st.Installed,
		"data_dir":        st.DataDir,
		"docs":            st.Docs,
		"articles":        st.Articles,
		"evidence_ok":     st.EvidenceOK,
		"manifest_signed": st.ManifestSigned,
	}
}

// PickLawDataPackage 弹出文件选择框：语料/更新包 zip。
func (a *App) PickLawDataPackage() (string, error) {
	return a.pickLawDataPath("选择法条数据包（core/evidence/更新包 zip）", "*.zip")
}

// PickLawDataDir 弹出目录选择框：已解压的数据目录。
func (a *App) PickLawDataDir() (string, error) {
	return a.pickLawDataPath("选择法条数据目录（已解压的 law-data / law-evidence / 更新包）", "")
}

func (a *App) pickLawDataPath(title, pattern string) (string, error) {
	if pattern == "" {
		return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{Title: title})
	}
	return wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title:   title,
		Filters: []wailsRuntime.FileFilter{{DisplayName: "数据包 (zip)", Pattern: pattern}},
	})
}

// LoadLawData 加载数据包（zip 或目录），自动识别类型并执行：
//   - core / full：首次安装，或全量替换现有语料（更新语义；失败回滚旧语料）
//   - evidence   ：合并证据层（只补缺失，溯源链补全）
//   - update     ：openlaw-pack 增量包——签名/哈希校验通过后逐条导入，
//     校验失败拒绝导入并保留旧数据（6.4 增量包契约）
//
// 全程通过 "lawdata:progress" 事件向前端报告进度；结果审计留痕。
func (a *App) LoadLawData(path string) (map[string]any, error) {
	if path == "" {
		return nil, fmt.Errorf("未选择数据包路径")
	}
	role := a.settings.Profile.Active
	report := func(s string) {
		log.Printf("法条数据: %s", s)
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "lawdata:progress", s)
		}
	}
	fail := func(err error) (map[string]any, error) {
		log.Printf("法条数据加载失败: %v", err)
		_ = a.auditLog.Log(role, "LAW_DATA_LOAD_FAIL", path, err.Error())
		return nil, err
	}

	src, tmp, err := lawinstall.NormalizeSource(path)
	if tmp != "" {
		defer os.RemoveAll(tmp)
	}
	if err != nil {
		return fail(err)
	}
	kind := lawinstall.Classify(src)
	if kind == "" {
		return fail(fmt.Errorf("无法识别的数据包（需为 core/evidence/更新包）: %s", filepath.Base(path)))
	}
	target := a.settings.OpenLaw.DataDir
	if target == "" {
		target = filepath.Join(a.dataDir, "law-data")
	}
	report(fmt.Sprintf("识别为 %s 包: %s", kind, filepath.Base(path)))

	switch kind {
	case "core", "full":
		a.closeLawSource() // 释放旧 SQLite 句柄再换目录
		if lawinstall.Installed(target) {
			if err := lawinstall.Replace(src, target, report); err != nil {
				return fail(err)
			}
			_ = a.auditLog.Log(role, "LAW_DATA_REPLACED", target, path)
		} else {
			if err := lawinstall.Install(src, target, report); err != nil {
				return fail(err)
			}
			_ = a.auditLog.Log(role, "LAW_DATA_INSTALLED", target, path)
		}
		a.settings.OpenLaw.DataDir = target
	case "evidence":
		if !lawinstall.Installed(target) {
			return fail(fmt.Errorf("尚未安装语料：请先加载 core 数据包，再补装证据层"))
		}
		if err := lawinstall.MergeEvidence(src, target, report); err != nil {
			return fail(err)
		}
		_ = a.auditLog.Log(role, "LAW_EVIDENCE_MERGED", target, path)
	case "update":
		if !lawinstall.Installed(target) {
			return fail(fmt.Errorf("尚未安装语料：增量更新包需要基础语料先就位"))
		}
		a.closeLawSource() // 导入前释放句柄，避免 SQLite 写冲突
		db, err := store.Open(filepath.Join(target, "db", "openlaw.db"))
		if err != nil {
			return fail(fmt.Errorf("打开语料库失败: %w", err))
		}
		res, err := pack.Import(db, target, src)
		db.Close()
		if err != nil {
			return fail(err)
		}
		_ = a.auditLog.Log(role, "LAW_PACK_IMPORTED", target, fmt.Sprintf("%d 条文档", len(res)))
		report(fmt.Sprintf("增量导入完成：新增 %d 条文档", len(res)))
	}

	a.openLawSource() // 语料已变，重开法条源
	return a.LawDataStatus(), nil
}

// shutdown 关闭资源。
func (a *App) shutdown(ctx context.Context) {
	if a.lawSource != nil {
		if c, ok := a.lawSource.(interface{ Close() error }); ok {
			_ = c.Close()
		}
	}
	if a.store != nil {
		a.store.Close()
	}
}

// newID 生成随机 ID。
func newID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b)
}

// AppInfo 返回应用与当前 Profile 概况（前端首屏展示）。
func (a *App) AppInfo() map[string]any {
	return map[string]any{
		"version":       version,
		"profile":       a.profile.Name,
		"role":          a.profile.Role,
		"dataDir":       a.dataDir,
		"redactLevel":   a.settings.Security.RedactLevel,
		"telemetry":     a.settings.Security.Telemetry,
		"modelEnabled":  a.settings.Model.Enabled,
		"disabledTools": a.profile.DisabledTools,
	}
}

// ListFacts 列出全部事实（先过可见性过滤——本部署仅本方数据）。
func (a *App) ListFacts() ([]domain.Fact, error) {
	facts, err := a.store.ListFacts()
	if err != nil {
		return nil, err
	}
	_ = a.auditLog.Log(a.profile.Role, "QUERY", "facts", "列出事实 "+fmt.Sprint(len(facts))+" 条")
	return facts, nil
}

// CreateFact 录入一条事实。初始状态由 statemachine.InitialState 判定：
// 有可定位引用→单源未补强；对方来源→对方主张未验证；无来源→无依据。
// 「已溯源确认」只能通过 UpgradeFact 的证据驱动路径达成。
func (a *App) CreateFact(description, sourceFile string, pageNum int, snippet string, fromOpponent bool) (domain.Fact, error) {
	if description == "" {
		return domain.Fact{}, fmt.Errorf("事实描述不能为空")
	}
	f := domain.Fact{
		FactID:      newID("FACT"),
		Description: description,
		Visibility:  domain.VisBoth,
	}
	if sourceFile != "" {
		f.Citation = &domain.Citation{SourceFile: sourceFile, PageNum: pageNum, Snippet: snippet}
	}
	f.State = statemachine.InitialState(f, fromOpponent)
	if err := a.store.CreateFact(f); err != nil {
		return domain.Fact{}, err
	}
	_ = a.auditLog.Log(a.profile.Role, "CREATE", f.FactID, "初始状态 "+f.State.DisplayName())
	return f, nil
}

// UpgradeFact 提交状态升级候选，由 statemachine 确定性仲裁。
// 模型或前端无权直接改状态——这里就是“代码仲裁优先于模型判断”的入口。
func (a *App) UpgradeFact(factID string, targetState string, evidenceImported, locatedInSource, noInternalConflict bool) (domain.Fact, error) {
	target, err := domain.ParseFactState(targetState)
	if err != nil {
		return domain.Fact{}, err
	}
	f, err := a.store.GetFact(factID)
	if err != nil {
		return domain.Fact{}, err
	}
	updated, err := statemachine.ApplyUpgrade(statemachine.UpgradeRequest{
		Fact:               f,
		Target:             target,
		EvidenceImported:   evidenceImported,
		LocatedInSource:    locatedInSource,
		NoInternalConflict: noInternalConflict,
	})
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "STATE_UPGRADE_BLOCKED", factID, err.Error())
		return domain.Fact{}, err
	}
	if err := a.store.UpdateFactState(updated.FactID, updated.State); err != nil {
		return domain.Fact{}, err
	}
	_ = a.auditLog.Log(a.profile.Role, "STATE_UPGRADE", factID, f.State.DisplayName()+" -> "+updated.State.DisplayName())
	return updated, nil
}

// ListConflicts 列出全部冲突（并列展示，不自动消解）。
func (a *App) ListConflicts() ([]domain.Conflict, error) {
	return a.store.ListConflicts()
}

// CheckOutput 对任意文本执行输出门禁（引导性检测），供前端预检与合规演示。
func (a *App) CheckOutput(text string) []gate.Violation {
	violations := gate.CheckGuidance(text)
	if len(violations) > 0 {
		_ = a.auditLog.Log(a.profile.Role, "GATE_BLOCK", "output", fmt.Sprint(len(violations))+" 条违规")
	}
	return violations
}

// RedactText 对文本执行强制脱敏，返回脱敏文本与操作审计记录。
func (a *App) RedactText(text string, names []string) redact.Result {
	r := redact.New(names)
	res := r.RedactText(text)
	for _, act := range res.Actions {
		_ = a.auditLog.Log(a.profile.Role, "REDACT", act.RuleID, act.Strategy)
	}
	return res
}

// --- 阶段二：文档接入与阅卷（开发文档 4.1、4.8-4.12、11.2） ---

// PickDocumentFile 弹出系统文件选择框（PDF/txt/md；图片与扫描件需 OCR）。
func (a *App) PickDocumentFile() (string, error) {
	return wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择卷宗文档",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "卷宗文档（PDF/txt/md）", Pattern: "*.pdf;*.txt;*.md"},
		},
	})
}

// IngestDocument 解析并入库文档（保留页码坐标，供核验器回原文定位）。
// 扫描型文档会被明确拒绝（OCR 引擎未配置）。
func (a *App) IngestDocument(path string) (storage.DocumentInfo, error) {
	if path == "" {
		return storage.DocumentInfo{}, fmt.Errorf("未选择文件")
	}
	doc, err := a.ingest.Parse(path)
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "INGEST_REJECTED", path, err.Error())
		return storage.DocumentInfo{}, err
	}
	docID := newID("DOC")
	if err := a.store.SaveDocument(docID, doc.File, doc.Pages); err != nil {
		return storage.DocumentInfo{}, err
	}
	_ = a.auditLog.Log(a.profile.Role, "INGEST", docID, fmt.Sprintf("%s 共 %d 页", filepath.Base(path), len(doc.Pages)))
	return a.store.GetDocumentInfo(docID)
}

// ListDocuments 列出已接入文档。
func (a *App) ListDocuments() ([]storage.DocumentInfo, error) {
	return a.store.ListDocuments()
}

// RunReader 对指定文档执行阅卷：模型产候选事实 → 核验器回原文定位仲裁。
// 定位失败的候选被拒绝入库并记审计；时间/金额要素由正则确定性抽取。
func (a *App) RunReader(docID string) (neutral.ReaderReport, error) {
	doc, err := a.store.GetDocument(docID)
	if err != nil {
		return neutral.ReaderReport{}, err
	}
	report, err := neutral.RunReader(a.ctx, docID, doc, a.model)
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "READER_FAILED", docID, err.Error())
		return neutral.ReaderReport{}, err
	}
	for _, f := range report.Accepted {
		if err := a.store.CreateFact(f); err != nil {
			return neutral.ReaderReport{}, err
		}
	}
	if err := a.store.SaveElements(report.Elements); err != nil {
		return neutral.ReaderReport{}, err
	}
	_ = a.auditLog.Log(a.profile.Role, "READER", docID,
		fmt.Sprintf("模型 %s：候选入库 %d 条，回原文失败拒绝 %d 条，要素 %d 个",
			report.ModelName, len(report.Accepted), len(report.Rejected), len(report.Elements)))
	return report, nil
}

// BuildTimeline 构建案件时间线（按时间要素锚定，无锚点排末尾，不虚构日期）。
func (a *App) BuildTimeline() ([]neutral.TimelineItem, error) {
	facts, err := a.store.ListFacts()
	if err != nil {
		return nil, err
	}
	elements, err := a.store.ListElements()
	if err != nil {
		return nil, err
	}
	return neutral.BuildTimeline(facts, elements), nil
}

// RunAggregate 汇聚全部事实：检测冲突并列输出入库（不自动消解）。
func (a *App) RunAggregate() ([]domain.Conflict, error) {
	facts, err := a.store.ListFacts()
	if err != nil {
		return nil, err
	}
	_, conflicts := neutral.Aggregate(facts)
	for _, c := range conflicts {
		if err := a.store.CreateConflict(c); err != nil {
			return nil, err
		}
	}
	_ = a.auditLog.Log(a.profile.Role, "AGGREGATE", "facts", fmt.Sprintf("发现冲突 %d 组（并列展示，不自动消解）", len(conflicts)))
	return conflicts, nil
}

// RunElementMatch 要件匹配：只有「已溯源确认」的事实才能覆盖要件。
func (a *App) RunElementMatch(requirements []string) (neutral.MatchResult, error) {
	facts, err := a.store.ListFacts()
	if err != nil {
		return neutral.MatchResult{}, err
	}
	return neutral.MatchElements(facts, requirements), nil
}

// RunBlindspots 盲区生成：输出无法判断的问题与补证方向，绝不幻觉填补。
func (a *App) RunBlindspots(requirements []string) ([]neutral.Blindspot, error) {
	match, err := a.RunElementMatch(requirements)
	if err != nil {
		return nil, err
	}
	return neutral.GenerateBlindspots(match), nil
}

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "OpenCourt — 中立法律信息整理与逻辑推演辅助",
		Width:     1280,
		Height:    800,
		MinWidth:  960,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []any{
			app,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
