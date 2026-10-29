// OpenCourt 桌面应用入口（Wails v2）。
// 桌面端为主分发形态：单应用、数据全本地、默认离线。
// Go 侧组合四层架构（core/neutral/profiles/adversary），通过 Wails bindings
// 直接暴露给 Vue 前端，无 HTTP 服务、无端口暴露。
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/futuremeng/OpenCourt/config"
	"github.com/futuremeng/OpenCourt/core/appupdate"
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
	// currentCaseID 当前活动案件（v0.2 M1，plan-v0.2 D1/D5）：全部事实/冲突/
	// 文档/要素读写经 store.WithCase 自动限定；state.json 持久化，重启恢复。
	currentCaseID string
	auditLog      *audit.Logger
	model         plugins.ModelAdapter
	// lawSource OpenLaw 法条源（M4 对接）：nil = 未配置，法条检索降级不可用。
	lawSource plugins.LawSourcePlugin
	// browser 浏览器辅助插件（BrowserOS neo，外部组件默认禁用）：
	// 未启用时联网查阅调用直接报错，绝不降级到其他抓取方式。
	browser plugins.BrowserPlugin
	ingest  *ingest.Registry
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
	a.currentCaseID = a.loadCurrentCase()

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

	// 浏览器辅助（BrowserOS neo，外部可选组件）：默认禁用；启用仅指向
	// 本机回环 MCP 端点，不新增任何监听端口（出站连接，桌面安全模型不变）。
	a.browser = plugins.NewBrowserOSPlugin(settings.BrowserOS.Endpoint, settings.BrowserOS.Enabled)
	if a.browser.Enabled() {
		_ = a.auditLog.Log(settings.Profile.Active, "BROWSER_SOURCE_OK", "browseros", a.browser.Name())
	}
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

// CheckLawDataUpdates 拉取发布索引并与本机语料比对，返回可下载包列表：
// action = install（本机缺）/ update（有更新版本）/ optional（可重装）/
// skip（不适用，如未装 core 时的 evidence）。前端据此渲染勾选项。
func (a *App) CheckLawDataUpdates() map[string]any {
	indexURL := a.settings.OpenLaw.IndexURL
	if indexURL == "" {
		return map[string]any{"ok": false, "error": "未配置数据包索引（openlaw.index_url）"}
	}
	ctx := context.Background()
	if a.ctx != nil {
		ctx = a.ctx
	}
	idx, err := lawinstall.FetchIndex(ctx, indexURL)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	cur := lawinstall.Status(a.settings.OpenLaw.DataDir)
	hasNewer := func(date string) bool {
		return cur.CorpusDate != "" && date > cur.CorpusDate
	}
	pkgs := make([]map[string]any, 0, len(idx.Packages))
	for _, p := range idx.Packages {
		action := "optional"
		switch p.Kind {
		case "core", "full":
			if !cur.Installed {
				action = "install"
			} else if hasNewer(p.Date) {
				action = "update"
			}
		case "evidence":
			switch {
			case !cur.Installed:
				action = "skip"
			case !cur.EvidenceOK:
				action = "install"
			case hasNewer(p.Date):
				action = "update"
			}
		case "update":
			if cur.Installed && hasNewer(p.Date) {
				action = "install"
			} else {
				action = "skip"
			}
		}
		pkgs = append(pkgs, map[string]any{
			"kind": p.Kind, "name": p.Name, "date": p.Date,
			"size": p.Size, "action": action,
		})
	}
	return map[string]any{
		"ok": true, "generated": idx.GeneratedAt, "packages": pkgs,
		"current": a.LawDataStatus(),
	}
}

// DownloadAndInstallLawData 按名称下载所选包（自动排序：core/full →
// evidence → update），每个包下载后先按索引 sha256 校验完整性，再走
// LoadLawData 完整流水线（内容真实性由包内签名 manifest 独立校验）。
// 成功即清缓存；失败保留已装结果并报错（审计 LAW_DATA_LOAD_FAIL）。
func (a *App) DownloadAndInstallLawData(names []string) (map[string]any, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("未勾选任何数据包")
	}
	indexURL := a.settings.OpenLaw.IndexURL
	if indexURL == "" {
		return nil, fmt.Errorf("未配置数据包索引（openlaw.index_url）")
	}
	ctx := context.Background()
	if a.ctx != nil {
		ctx = a.ctx
	}
	idx, err := lawinstall.FetchIndex(ctx, indexURL)
	if err != nil {
		return nil, err
	}
	byName := map[string]lawinstall.PackageInfo{}
	for _, p := range idx.Packages {
		byName[p.Name] = p
	}
	order := map[string]int{"core": 0, "full": 0, "evidence": 1, "update": 2}
	sel := make([]lawinstall.PackageInfo, 0, len(names))
	for _, n := range names {
		p, ok := byName[n]
		if !ok {
			return nil, fmt.Errorf("所选包不在发布索引中: %s", n)
		}
		sel = append(sel, p)
	}
	sort.SliceStable(sel, func(i, j int) bool { return order[sel[i].Kind] < order[sel[j].Kind] })

	cache := filepath.Join(a.dataDir, "lawdata-cache")
	emit := func(s string) {
		log.Printf("法条数据: %s", s)
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "lawdata:progress", s)
		}
	}
	for _, p := range sel {
		url := lawinstall.PackageURL(idx, p, indexURL)
		if url == "" {
			return nil, fmt.Errorf("包 %s 无法解析下载地址", p.Name)
		}
		dest := filepath.Join(cache, p.Name)
		emit(fmt.Sprintf("下载 %s（%s）…", p.Name, humanBytes(p.Size)))
		err := lawinstall.DownloadFile(ctx, url, dest, p.SHA256, func(done, total int64) {
			if total > 0 {
				emit(fmt.Sprintf("下载 %s：%d%%（%s / %s）",
					p.Name, done*100/total, humanBytes(done), humanBytes(total)))
			} else {
				emit(fmt.Sprintf("下载 %s：%s…", p.Name, humanBytes(done)))
			}
		})
		if err != nil {
			return nil, err
		}
		_ = a.auditLog.Log(a.settings.Profile.Active, "LAW_DATA_DOWNLOADED", p.Name, "sha256 校验通过")
		if _, err := a.LoadLawData(dest); err != nil {
			return nil, err
		}
		_ = os.Remove(dest) // 安装成功即清缓存
	}
	return a.LawDataStatus(), nil
}

// humanBytes 人类可读字节数。
func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0fMB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// --- 程序自身更新（GitHub Releases，macOS 自动替换重启） ---

// CheckAppUpdate 检查程序新版本：比较 GitHub Release tag 与内置版本号。
// 返回 available / notes / 包信息，供前端横幅提示。
func (a *App) CheckAppUpdate() map[string]any {
	if a.settings.App.UpdateAPI == "" {
		return map[string]any{"ok": false, "error": "未配置更新源（app.update_api）"}
	}
	ctx := context.Background()
	if a.ctx != nil {
		ctx = a.ctx
	}
	rel, err := appupdate.CheckLatest(ctx, a.settings.App.UpdateAPI)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	avail := appupdate.CompareVersions(rel.TagName, version) > 0
	dmg, sha := appupdate.PickAssets(rel, goosForUpdate(), goarchForUpdate())
	notes := rel.Body
	if len(notes) > 4000 {
		notes = notes[:4000]
	}
	res := map[string]any{
		"ok": true, "current": version, "latest": rel.TagName,
		"available": avail, "notes": notes,
	}
	if dmg != nil {
		res["package"] = dmg.Name
		res["size"] = dmg.Size
		res["has_sha256"] = sha != nil
	}
	return res
}

// InstallAppUpdate 下载并校验最新程序包（sha256 资产强制），随后由分离
// 脚本完成替换并重启新版本。仅支持 macOS 应用包（.app）形态。
func (a *App) InstallAppUpdate() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("自动更新目前仅支持 macOS，请从发布页手动更新")
	}
	if a.settings.App.UpdateAPI == "" {
		return fmt.Errorf("未配置更新源（app.update_api）")
	}
	// 定位自身 .app 包（开发模式 go run 不在 bundle 内 → 拒绝）
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	idx := strings.LastIndex(exe, ".app")
	if idx < 0 {
		return fmt.Errorf("当前不是应用包形态（开发模式），无法自动更新")
	}
	appPath := exe[:idx+4]

	emit := func(s string) {
		log.Printf("程序更新: %s", s)
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "appupdate:progress", s)
		}
	}
	ctx := context.Background()
	if a.ctx != nil {
		ctx = a.ctx
	}
	emit("下载新版本安装包并校验…")
	dmgPath, err := appupdate.DownloadRelease(ctx, a.settings.App.UpdateAPI,
		filepath.Join(a.dataDir, "update-cache"), func(done, total int64) {
			if total > 0 {
				emit(fmt.Sprintf("下载更新包：%d%%（%s / %s）",
					done*100/total, humanBytes(done), humanBytes(total)))
			}
		})
	if err != nil {
		_ = a.auditLog.Log(a.settings.Profile.Active, "APP_UPDATE_FAIL", "updater", err.Error())
		return err
	}
	script := appupdate.UpdaterScript(dmgPath, appPath, os.Getpid())
	scriptPath := filepath.Join(a.dataDir, "update-cache", "updater.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		return err
	}
	_ = a.auditLog.Log(a.settings.Profile.Active, "APP_UPDATE_APPLYING", "updater", dmgPath)
	emit("安装包校验通过，即将退出并完成替换重启…")
	cmd := exec.Command("/bin/sh", scriptPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // 脱离应用进程组，应用退出后继续
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动更新脚本失败: %w", err)
	}
	if a.ctx != nil {
		wailsRuntime.Quit(a.ctx)
	}
	return nil
}

// goosForUpdate / goarchForUpdate 平台标识（拆出便于测试资产挑选）。
func goosForUpdate() string   { return runtime.GOOS }
func goarchForUpdate() string { return runtime.GOARCH }

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

// --- 案件上下文（v0.2 M1，plan-v0.2 D1/D5/§5.1）---

// scope 当前案件的仓储视图：全部事实/冲突/文档/要素读写自动限定 case_id，
// 跨案件访问在 SQL 层不可能发生（编译期已无全局仓储方法）。
func (a *App) scope() *storage.CaseScope {
	return a.store.WithCase(a.currentCaseID)
}

// statePath 应用状态文件（当前案件 ID 持久化，重启恢复）。
func (a *App) statePath() string { return filepath.Join(a.dataDir, "state.json") }

// loadCurrentCase 从 state.json 恢复当前案件；案件不存在（删除/损坏）回落默认案件。
func (a *App) loadCurrentCase() string {
	if b, err := os.ReadFile(a.statePath()); err == nil {
		var st struct {
			CurrentCase string `json:"current_case"`
		}
		if json.Unmarshal(b, &st) == nil && st.CurrentCase != "" {
			if _, err := a.store.GetCase(st.CurrentCase); err == nil {
				return st.CurrentCase
			}
			log.Printf("state.json 中的案件不存在，回落默认案件: %s", st.CurrentCase)
		}
	}
	return domain.DefaultCaseID
}

// saveCurrentCase 持久化当前案件 ID。
func (a *App) saveCurrentCase() error {
	b, err := json.Marshal(map[string]string{"current_case": a.currentCaseID})
	if err != nil {
		return err
	}
	return os.WriteFile(a.statePath(), b, 0o600)
}

// CreateCase 新建案件并即时切换为当前案件。标题必填；案由为自由文本
// （不自动判定案由）；stage 缺省 CONSULT，非法阶段直接报错。
func (a *App) CreateCase(title, caseType, stage string) (domain.Case, error) {
	if strings.TrimSpace(title) == "" {
		return domain.Case{}, fmt.Errorf("案件标题不能为空")
	}
	st := domain.StageConsult
	if stage != "" {
		st = domain.CaseStage(stage)
		if !st.Valid() {
			return domain.Case{}, fmt.Errorf("非法案件阶段: %q（允许: CONSULT/FILING/FIRST_TRIAL/APPEAL/SECOND_TRIAL）", stage)
		}
	}
	c := domain.Case{
		CaseID: newID("CASE"), Title: strings.TrimSpace(title), CaseType: strings.TrimSpace(caseType),
		Stage: st, Visibility: domain.VisBoth, CreatedAt: time.Now().UTC(),
	}
	if err := a.store.CreateCase(c); err != nil {
		return domain.Case{}, err
	}
	_ = a.auditLog.Log(a.profile.Role, "CASE_CREATE", c.CaseID, c.Title)
	if err := a.SwitchCase(c.CaseID); err != nil {
		return c, err
	}
	return c, nil
}

// ListCases 列出全部案件（创建时间倒序）。
func (a *App) ListCases() ([]domain.Case, error) {
	return a.store.ListCases()
}

// SwitchCase 切换当前案件（不存在的案件报错；审计 CASE_SWITCH；
// state.json 持久化——切换失败即回滚，绝不出现内存与磁盘状态分裂）。
func (a *App) SwitchCase(caseID string) error {
	c, err := a.store.GetCase(caseID)
	if err != nil {
		return err
	}
	if caseID == a.currentCaseID {
		return nil
	}
	prev := a.currentCaseID
	a.currentCaseID = caseID
	if err := a.saveCurrentCase(); err != nil {
		a.currentCaseID = prev
		return err
	}
	_ = a.auditLog.Log(a.profile.Role, "CASE_SWITCH", caseID, c.Title)
	return nil
}

// CurrentCase 当前活动案件。
func (a *App) CurrentCase() (domain.Case, error) {
	return a.store.GetCase(a.currentCaseID)
}

// --- 法条检索与引用核验（v0.2 M2，plan-v0.2 4.1）---
// 纪律：检索/核验全部经 plugins 适配层走 OpenLaw sdk（禁直连 DB）；核验
// 判定透传 OpenLaw 错误码（OK / OL-1001..OL-1006），不另建同义枚举；
// pending 确认状态原样上浮——OpenCourt 不自动确认、不自行判定效力。

// SearchLaw 法条关键词检索。法条源未配置显式报错（不静默冒充空结果）。
func (a *App) SearchLaw(query string, topK int) ([]plugins.LawHit, error) {
	hits, err := neutral.SearchLaw(context.Background(), a.lawSource, query, topK)
	if err != nil {
		return nil, err
	}
	_ = a.auditLog.Log(a.profile.Role, "LAW_SEARCH", "", query)
	return hits, nil
}

// VerifyDocLawCitations 文书法条引用核验：确定性抽取「《法名》第X条」→ 逐条
// 交 OpenLaw CheckCitation → 引用留痕入当前案件 law_refs（跨案件天然隔离）。
// 断链（OL-1006）留痕照常入库，界面必须标注「无法溯源」。
func (a *App) VerifyDocLawCitations(docID, atDate string) ([]domain.LawRef, error) {
	verifier, ok := a.lawSource.(neutral.LawVerifier)
	if !ok {
		return nil, fmt.Errorf("法条源未配置（profile: openlaw.data_dir）：引用核验不可用")
	}
	doc, err := a.scope().GetDocument(docID)
	if err != nil {
		return nil, err
	}
	pages := make([]int, 0, len(doc.Pages))
	for p := range doc.Pages {
		pages = append(pages, p)
	}
	sort.Ints(pages)
	var sb strings.Builder
	for _, p := range pages {
		sb.WriteString(doc.Pages[p])
		sb.WriteString("\n")
	}
	drafts := neutral.ExtractLawCitations(sb.String())
	if len(drafts) == 0 {
		return []domain.LawRef{}, nil
	}
	refs, err := neutral.VerifyCitations(context.Background(), verifier, atDate, drafts)
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "LAW_VERIFY_FAIL", docID, err.Error())
		return nil, err
	}
	sc := a.scope()
	for _, r := range refs {
		if err := sc.AddLawRef(r); err != nil {
			return nil, err
		}
		if r.VerifyResult != "OK" {
			_ = a.auditLog.Log(a.profile.Role, "LAW_VERIFY_FAIL", docID,
				fmt.Sprintf("%s %s → %s", r.LawName, r.ArticleNo, r.VerifyResult))
		}
	}
	_ = a.auditLog.Log(a.profile.Role, "LAW_VERIFY", docID, fmt.Sprintf("%d 条引用", len(refs)))
	return refs, nil
}

// ListLawRefs 本案法条引用留痕（创建时间倒序）。
func (a *App) ListLawRefs() ([]domain.LawRef, error) {
	return a.scope().ListLawRefs()
}

// GetLawRelations 法条关系（修改/废止/解释/配套）：pending 关系原样返回，
// 界面必须标「待人工确认」——机器不自动判定废止。
func (a *App) GetLawRelations(refID string) ([]plugins.Relation, error) {
	verifier, ok := a.lawSource.(neutral.LawVerifier)
	if !ok {
		return nil, fmt.Errorf("法条源未配置：法条关系查询不可用")
	}
	refs, err := a.scope().ListLawRefs()
	if err != nil {
		return nil, err
	}
	for _, r := range refs {
		if r.RefID == refID {
			// relation 表以 law_id 为键；SDK 现不返回 law_id——为空时
			// fail-closed 报错并给出出路，绝不拿 article_id 去猜。
			if r.LawID == "" {
				return nil, fmt.Errorf("引用 %s 缺法规标识（law_id 待 OpenLaw SDK 透传，见桥接清单缺口）：关系查询不可用", refID)
			}
			return verifier.FindRelation(context.Background(), r.LawID)
		}
	}
	return nil, fmt.Errorf("法条引用不存在: %s", refID)
}

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

// ListFacts 列出全部事实（先过可见性过滤——本部署仅本方数据）。
func (a *App) ListFacts() ([]domain.Fact, error) {
	facts, err := a.scope().ListFacts()
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
	if err := a.scope().CreateFact(f); err != nil {
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
	f, err := a.scope().GetFact(factID)
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
	if err := a.scope().UpdateFactState(updated.FactID, updated.State); err != nil {
		return domain.Fact{}, err
	}
	_ = a.auditLog.Log(a.profile.Role, "STATE_UPGRADE", factID, f.State.DisplayName()+" -> "+updated.State.DisplayName())
	return updated, nil
}

// ListConflicts 列出全部冲突（并列展示，不自动消解）。
func (a *App) ListConflicts() ([]domain.Conflict, error) {
	return a.scope().ListConflicts()
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
	if err := a.scope().SaveDocument(docID, doc.File, doc.Pages); err != nil {
		return storage.DocumentInfo{}, err
	}
	_ = a.auditLog.Log(a.profile.Role, "INGEST", docID, fmt.Sprintf("%s 共 %d 页", filepath.Base(path), len(doc.Pages)))
	return a.scope().GetDocumentInfo(docID)
}

// ListDocuments 列出已接入文档。
func (a *App) ListDocuments() ([]storage.DocumentInfo, error) {
	return a.scope().ListDocuments()
}

// RunReader 对指定文档执行阅卷：模型产候选事实 → 核验器回原文定位仲裁。
// 定位失败的候选被拒绝入库并记审计；时间/金额要素由正则确定性抽取。
func (a *App) RunReader(docID string) (neutral.ReaderReport, error) {
	doc, err := a.scope().GetDocument(docID)
	if err != nil {
		return neutral.ReaderReport{}, err
	}
	report, err := neutral.RunReader(a.ctx, docID, doc, a.model)
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "READER_FAILED", docID, err.Error())
		return neutral.ReaderReport{}, err
	}
	for _, f := range report.Accepted {
		if err := a.scope().CreateFact(f); err != nil {
			return neutral.ReaderReport{}, err
		}
	}
	if err := a.scope().SaveElements(report.Elements); err != nil {
		return neutral.ReaderReport{}, err
	}
	_ = a.auditLog.Log(a.profile.Role, "READER", docID,
		fmt.Sprintf("模型 %s：候选入库 %d 条，回原文失败拒绝 %d 条，要素 %d 个",
			report.ModelName, len(report.Accepted), len(report.Rejected), len(report.Elements)))
	return report, nil
}

// --- 网络资料查阅（BrowserOS 辅助，默认禁用；docs/browseros-integration.md） ---

// ResearchWebPage 网络查阅：BrowserOS 抓取 → 强制脱敏 → 「单源未补强」事实
// 入库（Citation 含 URL/抓取时间/快照哈希）。返回文本已脱敏；原始页面内容
// 不回流模型。names 为已知当事人姓名（一致替换用）。
func (a *App) ResearchWebPage(url string, names []string) (neutral.ResearchResult, error) {
	if url == "" {
		return neutral.ResearchResult{}, fmt.Errorf("未提供 URL")
	}
	res, err := neutral.RunResearch(a.ctx, a.browser, names, url)
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "BROWSER_RESEARCH_FAIL", url, err.Error())
		return neutral.ResearchResult{}, err
	}
	if err := a.scope().CreateFact(res.Fact); err != nil {
		return neutral.ResearchResult{}, err
	}
	for _, act := range res.RedactActions {
		_ = a.auditLog.Log(a.profile.Role, "REDACT", act.RuleID, act.Strategy)
	}
	_ = a.auditLog.Log(a.profile.Role, "BROWSER_RESEARCH", url,
		fmt.Sprintf("快照哈希 %s；脱敏 %d 项；事实 %s 初始状态 %s（联网源至多单源未补强）",
			res.ContentSHA[:12], len(res.RedactActions), res.Fact.FactID, res.Fact.State.DisplayName()))
	return res, nil
}

// --- 网上立案辅助（两段式，设计见 docs/browseros-integration.md 第 6 节）---

// BrowserOpenPage 在用户 BrowserOS 中打开页面（辅助模式；不抓回整页内容）。
func (a *App) BrowserOpenPage(url string) (plugins.BrowserPage, error) {
	if url == "" {
		return plugins.BrowserPage{}, fmt.Errorf("未提供 URL")
	}
	if a.browser == nil || !a.browser.Enabled() {
		return plugins.BrowserPage{}, fmt.Errorf("浏览器辅助插件未启用：%s", plugins.BrowserOSGuidance)
	}
	page, err := a.browser.Open(a.ctx, url)
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "BROWSER_OPEN_FAIL", url, err.Error())
		return plugins.BrowserPage{}, err
	}
	_ = a.auditLog.Log(a.profile.Role, "BROWSER_OPEN", url, fmt.Sprintf("page=%d", page.PageID))
	return page, nil
}

// BrowserPlanFill 生成当前页填写计划（候选，不执行）：可交互快照 → 表单解析
// → 值映射 → gate 红线预筛。UI 必须逐条展示全部候选——含被红线拦截项
// （Allowed=false，附原因）与弱态事实字段（FactState 标黄），
// 经用户逐条确认后方可 BrowserExecuteAction。
func (a *App) BrowserPlanFill(pageID int, values map[string]string, factStates map[string]string) ([]plugins.ActionCandidate, error) {
	cands, elements, err := neutral.PlanPage(a.ctx, a.browser, pageID, values, factStates)
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "BROWSER_PLAN_FAIL", fmt.Sprintf("page=%d", pageID), err.Error())
		return nil, err
	}
	blocked := 0
	for _, c := range cands {
		if !c.Allowed {
			blocked++
			_ = a.auditLog.Log(a.profile.Role, "BROWSER_ACTION_BLOCKED", c.ActionID,
				fmt.Sprintf("%s: %s", c.Rule, c.Reason))
		}
	}
	_ = a.auditLog.Log(a.profile.Role, "BROWSER_PLAN", fmt.Sprintf("page=%d", pageID),
		fmt.Sprintf("元素 %d 个；候选 %d 项；红线拦截 %d 项", len(elements), len(cands), blocked))
	return cands, nil
}

// BrowserExecuteAction 执行单个「已经 Propose 且经用户确认」的候选动作。
// 插件侧三重硬约束（必须来自 Propose / gate 放行 / 一次性消费）；
// 回执 diff 为页面内容，返回 UI 前强制脱敏。
func (a *App) BrowserExecuteAction(actionID string, names []string) (plugins.ActionReceipt, error) {
	if a.browser == nil || !a.browser.Enabled() {
		return plugins.ActionReceipt{}, fmt.Errorf("浏览器辅助插件未启用：%s", plugins.BrowserOSGuidance)
	}
	rec, err := a.browser.ExecuteAction(a.ctx, actionID)
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "BROWSER_EXECUTE_FAIL", actionID, err.Error())
		return plugins.ActionReceipt{}, err
	}
	sum := sha256.Sum256([]byte(rec.Diff))
	red := redact.New(names)
	redRes := red.RedactText(rec.Diff)
	rec.Diff = redRes.Text
	for _, act := range redRes.Actions {
		_ = a.auditLog.Log(a.profile.Role, "REDACT", act.RuleID, act.Strategy)
	}
	_ = a.auditLog.Log(a.profile.Role, "BROWSER_EXECUTE", actionID,
		fmt.Sprintf("回执哈希 %s；脱敏 %d 项", hex.EncodeToString(sum[:])[:12], len(redRes.Actions)))
	return rec, nil
}

// BrowserHandToHuman 登录/验证码/实名/最终提交等环节交还给人接管——
// 这些环节工具永不代做（红线，见 docs/browseros-integration.md 5.3）。
func (a *App) BrowserHandToHuman(pageID int, kind, reason string) (plugins.HumanHelpStatus, error) {
	st, err := a.browser.HandToHuman(a.ctx, pageID, kind, reason, "")
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "BROWSER_HUMAN_FAIL", fmt.Sprintf("page=%d", pageID), err.Error())
		return st, err
	}
	_ = a.auditLog.Log(a.profile.Role, "BROWSER_HUMAN", fmt.Sprintf("page=%d", pageID), st.Status)
	return st, nil
}

// BrowserAwaitHuman 等待人接管结果（resolved 后才允许继续辅助操作）。
func (a *App) BrowserAwaitHuman() (plugins.HumanHelpStatus, error) {
	st, err := a.browser.AwaitHuman(a.ctx)
	_ = a.auditLog.Log(a.profile.Role, "BROWSER_HUMAN_WAIT", "-", st.Status)
	return st, err
}

// --- 国家规章库清单浏览（人在环代理浏览，neutral/govlistassist.go）---

// BrowseGovList 用用户自己的 BrowserOS neo 浏览国家规章库官方列表页并导出清单
// （混合清单；cat = 可选分类过滤 部门规章/地方政府规章/空 = 全收；maxPages 单会话上限；
// startPage 起始页——跨会话续接，经官方「跳转」控件到达并核验页码）。清单是人工浏览的
// 代理等价物：只走官方 UI 翻页（限速 ≥2s、红线审计），绝不触碰检索 API/鉴权。
// 产物写 appData/incoming/govgz/list_agent_<ts>.json，交 OpenLaw
// openlaw-govcn -source gzk -list <此文件> 走详情页合规抓取 + 人工确认闸口。
func (a *App) BrowseGovList(cat string, maxPages int, startPage int) (neutral.GovListReport, error) {
	dir, err := appDataDir()
	if err != nil {
		return neutral.GovListReport{}, err
	}
	rep, err := neutral.RunGovListBrowse(a.ctx, a.browser, neutral.GovListOptions{
		Cat: cat, MaxPages: maxPages, StartPage: startPage,
	})
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "BROWSER_GOVLIST_FAIL", cat, err.Error())
		return rep, err
	}
	outDir := filepath.Join(dir, "incoming", "govgz")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return rep, err
	}
	out := filepath.Join(outDir, "list_agent_"+time.Now().UTC().Format("20060102T150405")+".json")
	b, err := json.MarshalIndent(rep.Entries, "", "  ")
	if err != nil {
		return rep, err
	}
	if err := os.WriteFile(out, b, 0o644); err != nil {
		return rep, err
	}
	rep.OutPath = out
	sum := sha256.Sum256(b)
	_ = a.auditLog.Log(a.profile.Role, "BROWSER_GOVLIST", cat,
		fmt.Sprintf("浏览 %d 页（起始页 %d）得 %d 条（%s）→ %s sha256=%s；待 openlaw-govcn -source gzk 抓取+人工确认",
			rep.Pages, rep.StartPage, len(rep.Entries), rep.StoppedAt, out, hex.EncodeToString(sum[:])[:12]))
	return rep, nil
}

// BuildTimeline 构建案件时间线（按时间要素锚定，无锚点排末尾，不虚构日期）。
func (a *App) BuildTimeline() ([]neutral.TimelineItem, error) {
	facts, err := a.scope().ListFacts()
	if err != nil {
		return nil, err
	}
	elements, err := a.scope().ListElements()
	if err != nil {
		return nil, err
	}
	return neutral.BuildTimeline(facts, elements), nil
}

// RunAggregate 汇聚全部事实：检测冲突并列输出入库（不自动消解）。
func (a *App) RunAggregate() ([]domain.Conflict, error) {
	facts, err := a.scope().ListFacts()
	if err != nil {
		return nil, err
	}
	_, conflicts := neutral.Aggregate(facts)
	for _, c := range conflicts {
		if err := a.scope().CreateConflict(c); err != nil {
			return nil, err
		}
	}
	_ = a.auditLog.Log(a.profile.Role, "AGGREGATE", "facts", fmt.Sprintf("发现冲突 %d 组（并列展示，不自动消解）", len(conflicts)))
	return conflicts, nil
}

// RunElementMatch 要件匹配：只有「已溯源确认」的事实才能覆盖要件。
func (a *App) RunElementMatch(requirements []string) (neutral.MatchResult, error) {
	facts, err := a.scope().ListFacts()
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
