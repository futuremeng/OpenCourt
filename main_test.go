package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futuremeng/OpenLaw/store"

	"github.com/futuremeng/OpenCourt/config"
	"github.com/futuremeng/OpenCourt/core/audit"
	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/storage"
	"github.com/futuremeng/OpenCourt/plugins"
	"github.com/futuremeng/OpenCourt/profiles"
)

func TestLoadSettingsAppDirPriority(t *testing.T) {
	appDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(appDir, "settings.yaml"),
		[]byte("profile:\n  active: plaintiff\nopenlaw:\n  data_dir: /x/data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wd, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wd, "config", "settings.yaml"),
		[]byte("profile:\n  active: court\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(wd)

	s, err := loadSettings(appDir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Profile.Active != "plaintiff" || s.OpenLaw.DataDir != "/x/data" {
		t.Errorf("应用数据目录应优先: %+v", s)
	}
}

func TestLoadSettingsDevFallback(t *testing.T) {
	appDir := t.TempDir() // 空：无正式配置
	wd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wd, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wd, "config", "settings.yaml"),
		[]byte("profile:\n  active: defendant\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(wd)

	s, err := loadSettings(appDir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Profile.Active != "defendant" {
		t.Errorf("开发模式回退应生效: %+v", s)
	}
}

func TestLoadSettingsBrokenFallsBack(t *testing.T) {
	appDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(appDir, "settings.yaml"),
		[]byte("profile: [broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := loadSettings(appDir) // 坏配置 → 默认而非崩溃
	if err != nil {
		t.Fatal(err)
	}
	if s.Profile.Active != "court" {
		t.Errorf("坏配置应回退默认: %+v", s)
	}
}

// --- 法条数据包加载（前端「法条数据」面板后端流程） ---

func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	lg, err := audit.NewLogger(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return &App{settings: config.Default(), dataDir: dir, auditLog: lg}
}

// mkCoreDir 造合法 core 包目录（真实 SQLite，sdk 可打开）。
func mkCoreDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "db", "openlaw.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	return dir
}

func TestLoadLawDataFlow(t *testing.T) {
	a := newTestApp(t)

	// 证据层先行 → 拒绝（须先装 core）
	ev := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ev, "docx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ev, "docx", "a.docx"), []byte("e"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.LoadLawData(ev); err == nil {
		t.Error("core 未装时 evidence 应被拒绝")
	}

	// core 包 → 安装 + 语料源打开
	st, err := a.LoadLawData(mkCoreDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if st["installed"] != true || a.lawSource == nil {
		t.Errorf("core 加载后应已安装且语料源可用: %+v", st)
	}
	target := a.settings.OpenLaw.DataDir

	// evidence 包 → 合并进已装语料
	if _, err := a.LoadLawData(ev); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "docx", "a.docx")); err != nil {
		t.Error("证据层应已合并:", err)
	}

	// 增量包（无签名）→ pack.Verify 拒绝，旧数据不动
	up := t.TempDir()
	if err := os.WriteFile(filepath.Join(up, "changes.csv"), []byte("action,doc_id\nadd,d1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.LoadLawData(up); err == nil {
		t.Error("未签名增量包应被拒绝导入")
	}

	// 无法识别的目录 → 明确报错
	if _, err := a.LoadLawData(t.TempDir()); err == nil {
		t.Error("无法识别的包应报错")
	}

	// 再次 core 包 → 全量替换（更新语义）：本机旧语料（含已合并证据层）整体换新
	if _, err := a.LoadLawData(mkCoreDir(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "docx", "a.docx")); err == nil {
		t.Error("全量替换后旧语料（含合并的证据层）应已被新包取代")
	}
}

// --- 应用内勾选下载（发布索引 → 下载 → sha256 校验 → 自动安装） ---

// mkZipBytes 造 zip（root/... 结构）并返回 (bytes, sha256hex)。
func mkZipBytes(t *testing.T, files map[string][]byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		fw, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:])
}

func TestCheckAndDownloadLawData(t *testing.T) {
	// core 包：真实 SQLite（sdk 可打开）
	coreDir := mkCoreDir(t)
	dbBytes, err := os.ReadFile(filepath.Join(coreDir, "db", "openlaw.db"))
	if err != nil {
		t.Fatal(err)
	}
	coreZip, coreSHA := mkZipBytes(t, map[string][]byte{"law-data/db/openlaw.db": dbBytes})
	evZip, evSHA := mkZipBytes(t, map[string][]byte{"law-evidence/docx/a.docx": []byte("e")})

	mux := http.NewServeMux()
	mux.HandleFunc("/lawdata-core-x.zip", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(coreZip) })
	mux.HandleFunc("/lawdata-evidence-x.zip", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(evZip) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	// 绝对 url 字段（GitHub 直链规则不适用于本地测试服务器）
	mux.HandleFunc("/abs-index.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"generated": "2026-10-25",
			"packages": []map[string]any{
				{"kind": "core", "name": "lawdata-core-x.zip", "date": "20261025", "size": len(coreZip), "sha256": coreSHA, "url": srv.URL + "/lawdata-core-x.zip"},
				{"kind": "evidence", "name": "lawdata-evidence-x.zip", "date": "20261025", "size": len(evZip), "sha256": evSHA, "url": srv.URL + "/lawdata-evidence-x.zip"},
			},
		})
	})

	a := newTestApp(t)
	a.settings.OpenLaw.IndexURL = srv.URL + "/abs-index.json"

	// 检查更新：未装 → core=install、evidence=skip（须先装 core）
	res := a.CheckLawDataUpdates()
	if res["ok"] != true {
		t.Fatalf("检查更新失败: %+v", res)
	}
	pkgs := res["packages"].([]map[string]any)
	actions := map[string]string{}
	for _, p := range pkgs {
		actions[p["name"].(string)] = p["action"].(string)
	}
	if actions["lawdata-core-x.zip"] != "install" || actions["lawdata-evidence-x.zip"] != "skip" {
		t.Errorf("action 判定不符: %+v", actions)
	}

	// 勾选下载（乱序传入 → 自动 core 先装）→ 下载校验 → 安装 + 合并
	st, err := a.DownloadAndInstallLawData([]string{"lawdata-evidence-x.zip", "lawdata-core-x.zip"})
	if err != nil {
		t.Fatal(err)
	}
	if st["installed"] != true || a.lawSource == nil {
		t.Errorf("下载后应已安装: %+v", st)
	}
	target := a.settings.OpenLaw.DataDir
	if _, err := os.Stat(filepath.Join(target, "docx", "a.docx")); err != nil {
		t.Error("evidence 应已自动合并:", err)
	}
	// 缓存清理
	cacheFiles, _ := os.ReadDir(filepath.Join(a.dataDir, "lawdata-cache"))
	if len(cacheFiles) != 0 {
		t.Errorf("安装成功后应清空下载缓存: %d 个残留", len(cacheFiles))
	}

	// sha256 不符 → 拒绝安装（篡改/传输损坏防护）
	mux.HandleFunc("/bad-index.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"packages": []map[string]any{
				{"kind": "core", "name": "lawdata-core-x.zip", "date": "20261025", "size": len(coreZip), "sha256": "deadbeef", "url": srv.URL + "/lawdata-core-x.zip"},
			},
		})
	})
	a2 := newTestApp(t)
	a2.settings.OpenLaw.IndexURL = srv.URL + "/bad-index.json"
	if _, err := a2.DownloadAndInstallLawData([]string{"lawdata-core-x.zip"}); err == nil {
		t.Error("sha256 不符应拒绝")
	}
}

// --- 案件实体（v0.2 M1）：建案/切换/隔离/重启恢复 ---

// newCaseApp 打开真实 SQLite 的最小 App（案件绑定测试用）。
func newCaseApp(t *testing.T, dir string) *App {
	t.Helper()
	lg, err := audit.NewLogger(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := storage.Open(filepath.Join(dir, "opencourt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	a := &App{
		settings: config.Default(), dataDir: dir, auditLog: lg, store: st,
		profile: profiles.Profile{Role: "court", Name: "court"},
	}
	a.currentCaseID = a.loadCurrentCase()
	return a
}

func TestCaseLifecycle(t *testing.T) {
	dir := t.TempDir()
	a := newCaseApp(t, dir)

	// 初始即默认案件
	cur, err := a.CurrentCase()
	if err != nil || cur.CaseID != domain.DefaultCaseID {
		t.Fatalf("初始应为默认案件: %+v %v", cur, err)
	}
	if _, err := a.CreateCase("", "", ""); err == nil {
		t.Error("空标题应被拒绝")
	}
	if _, err := a.CreateCase("x 案", "", "BOGUS"); err == nil {
		t.Error("非法阶段应被拒绝")
	}

	c, err := a.CreateCase("民间借贷纠纷", "民间借贷", "FIRST_TRIAL")
	if err != nil {
		t.Fatal(err)
	}
	if cur, _ = a.CurrentCase(); cur.CaseID != c.CaseID {
		t.Errorf("新建案件应即时切换: %+v", cur)
	}

	// 事实挂当前案件；切回默认案件后不可见（SQL 层隔离）
	if _, err := a.CreateFact("A 案事实", "", 0, "", false); err != nil {
		t.Fatal(err)
	}
	if facts, _ := a.ListFacts(); len(facts) != 1 {
		t.Errorf("新案件应见 1 条事实: %+v", facts)
	}
	if err := a.SwitchCase(domain.DefaultCaseID); err != nil {
		t.Fatal(err)
	}
	if facts, _ := a.ListFacts(); len(facts) != 0 {
		t.Errorf("默认案件不得见新案件事实: %+v", facts)
	}
	if err := a.SwitchCase("CASE-NOPE"); err == nil {
		t.Error("切换到不存在的案件必须报错")
	}

	// 重启恢复：同目录重建 App，state.json 生效
	a2 := newCaseApp(t, dir)
	if cur, _ = a2.CurrentCase(); cur.CaseID != domain.DefaultCaseID {
		t.Errorf("重启应恢复 state.json 中的当前案件: %+v", cur)
	}
	if err := a2.SwitchCase(c.CaseID); err != nil {
		t.Fatal(err)
	}
	if facts, _ := a2.ListFacts(); len(facts) != 1 {
		t.Errorf("切回新案件应见其事实: %+v", facts)
	}
}

// M2 绑定守卫：法条源未配置时检索/核验/关系查询必须 fail-closed 显式报错，
// 绝不静默返回空冒充「语料里没有」。
func TestLawBindingsFailClosedWithoutSource(t *testing.T) {
	a := newCaseApp(t, t.TempDir())
	if _, err := a.SearchLaw("违约金", 10); err == nil {
		t.Error("SearchLaw 未配置法条源必须报错")
	}
	if _, err := a.VerifyDocLawCitations("DOC-X", ""); err == nil {
		t.Error("VerifyDocLawCitations 未配置法条源必须报错")
	}
	if _, err := a.GetLawRelations("LAW-x"); err == nil {
		t.Error("GetLawRelations 未配置法条源必须报错")
	}
	if _, err := a.RunElementMatchByCaseType("公司纠纷"); err == nil {
		t.Error("RunElementMatchByCaseType 未配置要件库必须报错（绝不回退模型生成要件）")
	}
	refs, err := a.ListLawRefs()
	if err != nil || len(refs) != 0 {
		t.Errorf("本案留痕应为空列表: %v %+v", err, refs)
	}
}

// M3 绑定守卫：类案源未配置时 SearchSimilarCases 必须 fail-closed 报错带指引
// （不降级到其他来源、不返回空列表冒充「语料里没有」）；ListCaseRefs 为本案
// 空留痕（类案引用非事实）；LocalCasesStatus 如实报告未启用。
func TestCaseBindingsFailClosedWithoutSource(t *testing.T) {
	a := newCaseApp(t, t.TempDir())
	if _, err := a.SearchSimilarCases("借款", "", "", 0, 0, 10, nil); err == nil ||
		!strings.Contains(err.Error(), plugins.LocalCasesGuidance) {
		t.Errorf("SearchSimilarCases 未配置类案源必须报错带指引: %v", err)
	}
	refs, err := a.ListCaseRefs()
	if err != nil || len(refs) != 0 {
		t.Errorf("本案类案留痕应为空列表: %v %+v", err, refs)
	}
	if st := a.LocalCasesStatus(); st["enabled"] != false {
		t.Errorf("LocalCasesStatus 必须如实报告未启用: %v", st)
	}
}
