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
	"sync"
	"time"

	wails "github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/futuremeng/OpenCourt/config"
	"github.com/futuremeng/OpenCourt/core/audit"
	"github.com/futuremeng/OpenCourt/core/ingest"
	"github.com/futuremeng/OpenCourt/core/lawinstall"
	"github.com/futuremeng/OpenCourt/core/storage"
	"github.com/futuremeng/OpenCourt/plugins"
	"github.com/futuremeng/OpenCourt/profiles"
)

//go:embed all:frontend/dist
var assets embed.FS

// version 应用版本（Semantic Versioning）。
const version = "0.1.0-preview"

// App 应用服务：Wails bindings 的绑定对象。
//
// 并发模型（务必先读）：Wails v2 每次前端调用都在独立 goroutine 中执行绑定
// 方法，因此「切换案件」「加载语料包」与「检索/阅卷」可能同时发生。所有跨
// goroutine 可变的字段一律标 [mu] 并由 mu 保护，读取走带锁访问器，不得裸读。
type App struct {
	// mu 保护下面标注 [mu] 的字段：读用 RLock（withLaw / scope / lawDataDir），
	// 写用 Lock（SwitchCase / setLawSource / setLawDataDir）。
	// 关键约束：语料替换要关闭 SQLite 句柄，必须先拿到写锁——withLaw 的读者
	// 持 RLock 跨整个插件调用，绝不在已关闭句柄上查询。
	mu sync.RWMutex

	ctx      context.Context
	dataDir  string
	settings *config.Settings // 启动后只读（OpenLaw.DataDir 的运行期值改由 lawDataDir 承载）
	profile  profiles.Profile
	store    *storage.Store
	// currentCaseID 当前活动案件（v0.2 M1，plan-v0.2 D1/D5）：全部事实/冲突/
	// 文档/要素读写经 store.WithCase 自动限定；state.json 持久化，重启恢复。[mu]
	currentCaseID string
	// lawDataDirOverride 用户显式加载的语料目录（state.json 持久化），优先于
	// 配置与自动发现（修：原先 LoadLawData 只改内存值，重启即丢）。[mu]
	lawDataDirOverride string
	auditLog           *audit.Logger
	model              plugins.ModelAdapter
	// lawSource OpenLaw 法条源（M4 对接）：nil = 未配置，法条检索降级不可用。[mu]
	lawSource plugins.LawSourcePlugin
	// browser 浏览器辅助插件（BrowserOS neo，外部组件默认禁用）：
	// 未启用时联网查阅调用直接报错，绝不降级到其他抓取方式。
	browser plugins.BrowserPlugin
	// caseSource 类案源（v0.2 M3，LocalCases 外部可选组件默认禁用）：
	// nil/未启用时类案检索直接报错带指引，绝不降级到其他来源（D3）。
	// 启动时构造后只读，无需加锁。
	caseSource plugins.CaseSourcePlugin
	ingest     *ingest.Registry
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

// --- 并发安全访问器（务必经这里，不得裸读 [mu] 字段） ---

// scope 当前案件的仓储视图。RLock 下取一次 case_id 快照：并发切换案件时，
// 单次请求要么完整看到旧案件、要么完整看到新案件，绝不撕裂。
func (a *App) scope() *storage.CaseScope {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.store.WithCase(a.currentCaseID)
}

// caseID 当前活动案件 ID。
func (a *App) caseID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.currentCaseID
}

// withLaw 在 RLock 保护下取用法条源（src 可能为 nil = 未配置）。读者持锁跨
// 整个插件调用，语料替换（setLawSource 取写锁）只能等到查询结束才关句柄——
// 杜绝「一边检索一边换语料」使用已关闭的 SQLite 句柄。
//
// ⚠️ 闭包内不得再调用 scope() / caseID() / lawDataDir()：Go 的 RWMutex 读锁
// 不可重入，此时若有写者在排队会死锁。需要数据先在闭包外取好。
func (a *App) withLaw(fn func(src plugins.LawSourcePlugin) error) error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return fn(a.lawSource)
}

// setLawSource 替换法条源句柄（写锁）。调用前必须已关闭旧句柄。
func (a *App) setLawSource(src plugins.LawSourcePlugin) {
	a.mu.Lock()
	a.lawSource = src
	a.mu.Unlock()
}

// lawDataDir 生效的语料目录：state.json 记录的用户选择优先，其次配置文件。
func (a *App) lawDataDir() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.lawDataDirOverride != "" {
		return a.lawDataDirOverride
	}
	return a.settings.OpenLaw.DataDir
}

// setLawDataDir 记录用户显式选择的语料目录（写锁）。
func (a *App) setLawDataDir(dir string) {
	a.mu.Lock()
	a.lawDataDirOverride = dir
	a.mu.Unlock()
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

	db, err := storage.Open(dataDir + "/opencourt.db")
	if err != nil {
		log.Fatalf("本地数据库打开失败: %v", err)
	}
	a.store = db
	// state.json：当前案件 + 用户显式选择的语料目录（重启后保持原样）。
	st := loadState(a.statePath())
	a.currentCaseID = a.validatedCaseID(st.CurrentCase)
	a.lawDataDirOverride = st.LawDataDir

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
	// OCR 引擎（O1/D1）：默认 none——扫描件明确拒绝；auto 探测
	// macOS Vision helper → PATH tesseract。引擎缺失绝不静默放行。
	ocrEngine, ocrNote := ingest.DetectOCR(ingest.OCRConfig{
		Engine: settings.Ingest.OCR.Engine,
		Path:   settings.Ingest.OCR.Path,
		Lang:   settings.Ingest.OCR.Lang,
	})
	if ocrNote != "" {
		log.Println("OCR:", ocrNote)
		_ = a.auditLog.Log(settings.Profile.Active, "OCR_UNAVAILABLE", "ingest", ocrNote)
	}
	a.ingest.SetOCR(ocrEngine, ingest.ExtractEmbeddedImages)

	// OpenLaw 法条源（M4 对接）：进程内 sdk，禁止直连数据库；未配置 → nil 降级。
	// 打开失败不阻断启动（法条检索不可用 + 审计留痕），其余功能不受影响。
	// 桌面分发：未配置（或配置路径失效）且分发包内带 law-data（dmg 卷内）→
	// 首次启动自动安装到本机应用数据目录（一次性、幂等；SQLite 需本地读写）。
	if exe, err := os.Executable(); err == nil {
		if bundled := lawinstall.FindBundled(exe); bundled != "" {
			a.setLawDataDir(a.ensureLawData(bundled))
		} else if home, err := os.UserHomeDir(); err == nil {
			downloads := filepath.Join(home, "Downloads")
			if dl := lawinstall.FindDownloaded(downloads); dl != "" {
				a.setLawDataDir(a.ensureLawData(dl))
			}
			// 证据层包（docx/md/provenance）：核心包已就位时自动合并，
			// 合并后溯源链补全（UNVERIFIED → 完整 trace）
			if ev := lawinstall.FindEvidence(downloads); ev != "" &&
				a.lawDataDir() != "" && dirExists(a.lawDataDir()) {
				if err := lawinstall.MergeEvidence(ev, a.lawDataDir(), func(s string) {
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

	// 浏览器辅助（BrowserOS neo，外部可选组件）：默认禁用；启用仅指向
	// 本机回环 MCP 端点，不新增任何监听端口（出站连接，桌面安全模型不变）。
	a.browser = plugins.NewBrowserOSPlugin(settings.BrowserOS.Endpoint, settings.BrowserOS.Enabled)
	if a.browser.Enabled() {
		_ = a.auditLog.Log(settings.Profile.Active, "BROWSER_SOURCE_OK", "browseros", a.browser.Name())
	}

	// 类案源（LocalCases，外部可选组件）：默认禁用；启用仅指向用户自装的
	// 回环 HTTP 服务（出站连接，不新增监听端口，绝不拉起外部进程）。
	// LC-6 发现文件未就绪前 base_url + token_env 为手动临时通道。
	a.caseSource = a.newLocalCasesSource()
}

// newLocalCasesSource 按配置构造类案源：token 只经 token_env 指向的环境变量
// 注入（严禁写入配置文件）；启用但环境变量缺失不阻断构造——请求时 401 如实
// 报错并提示检查 token_env（§9）。
func (a *App) newLocalCasesSource() plugins.CaseSourcePlugin {
	cfg := a.settings.LocalCases
	if !cfg.Enabled || cfg.BaseURL == "" {
		return nil
	}
	token := ""
	if cfg.TokenEnv != "" {
		token = os.Getenv(cfg.TokenEnv)
	}
	src := plugins.NewLocalCasesSource(cfg.BaseURL, token, true)
	if !src.Enabled() {
		_ = a.auditLog.Log(a.settings.Profile.Active, "CASE_SOURCE_FAIL", "localcases",
			"base_url 非法或非回环地址（类案语料含未脱敏文书，只允许回环）")
		return nil
	}
	_ = a.auditLog.Log(a.settings.Profile.Active, "CASE_SOURCE_OK", "localcases", cfg.BaseURL)
	return src
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

// shutdown 关闭资源。
func (a *App) shutdown(ctx context.Context) {
	a.closeLawSource()
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

// --- 案件上下文（v0.2 M1，plan-v0.2 D1/D5/§5.1）---

// AppInfo 返回应用与当前 Profile 概况（前端首屏展示）。
func (a *App) AppInfo() map[string]any {
	return map[string]any{
		"version":        version,
		"profile":        a.profile.Name,
		"role":           a.profile.Role,
		"dataDir":        a.dataDir,
		"redactLevel":    a.settings.Security.RedactLevel,
		"telemetry":      a.settings.Security.Telemetry,
		"modelEnabled":   a.settings.Model.Enabled,
		"browserEnabled": a.browser != nil && a.browser.Enabled(),
		"disabledTools":  a.profile.DisabledTools,
	}
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
