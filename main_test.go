package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/futuremeng/OpenLaw/store"

	"github.com/futuremeng/OpenCourt/config"
	"github.com/futuremeng/OpenCourt/core/audit"
	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/storage"
	"github.com/futuremeng/OpenCourt/neutral"
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
	a := &App{settings: config.Default(), dataDir: dir, auditLog: lg}
	// 测试结束按与生产 shutdown 相同的顺序关资源：Windows 上不关的 SQLite
	// 句柄会阻止 TempDir 清理（Unix 可删打开文件才一直没暴露这个问题）。
	t.Cleanup(func() {
		a.closeLawSource()
		if a.store != nil {
			a.store.Close()
		}
	})
	return a
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
	target := a.lawDataDir()

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
	target := a.lawDataDir()
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
	a.currentCaseID = a.validatedCaseID(loadState(a.statePath()).CurrentCase)
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

// TestConcurrentCaseSwitchAndReads 并发回归（review P0-2）：Wails 每次前端调用
// 都在独立 goroutine 中执行绑定方法，「切换案件」与「读事实」并发是常态。
// 未加锁时 -race 会直接报 data race；同时断言切换是原子的——单次读取要么
// 完整看到旧案件、要么完整看到新案件，绝不撕裂。
func TestConcurrentCaseSwitchAndReads(t *testing.T) {
	dir := t.TempDir()
	a := newCaseApp(t, dir)
	c2, err := a.CreateCase("并发案件", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SwitchCase(domain.DefaultCaseID); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := a.ListFacts(); err != nil {
					t.Errorf("并发读事实失败: %v", err)
					return
				}
				if _, err := a.CurrentCase(); err != nil {
					t.Errorf("并发读当前案件失败: %v", err)
					return
				}
				runtime.Gosched() // 避免忙等饿死切换方（CI 上 race 检测器已很重）
			}
		}()
	}
	for i := 0; i < 40; i++ {
		id := c2.CaseID
		if i%2 == 0 {
			id = domain.DefaultCaseID
		}
		if err := a.SwitchCase(id); err != nil {
			t.Fatalf("切换案件失败: %v", err)
		}
	}
	close(stop)
	wg.Wait()

	// 落盘与内存必须一致（写锁内完成「改 + 落盘」，不留撕裂状态）
	if err := a.SwitchCase(c2.CaseID); err != nil {
		t.Fatal(err)
	}
	st := loadState(a.statePath())
	if st.CurrentCase != c2.CaseID {
		t.Errorf("state.json 与内存不一致: %+v", st)
	}
	if cur, err := a.CurrentCase(); err != nil || cur.CaseID != c2.CaseID {
		t.Errorf("CurrentCase 与 state.json 不一致: %+v %v", cur, err)
	}
}

// TestConcurrentLawSourceSwap 并发换语料与检索：withLaw 的读者持 RLock 跨整个
// 插件调用，替换方必须等到查询结束才关句柄——绝不在已关闭句柄上查询。
// 换语料窗口内 lawSource 短暂为 nil，此时检索 fail-closed 报错是正确行为。
// 调度纪律：全量套件负载下 goroutine 可能长时间未被调度，必须先等每个读者
// 至少完成一次检索（启动 barrier）再开始换语料，且换完后保留读者活跃窗口
// 才停——否则读者可能全程未参与，okSearch 恒 0 造成与产品无关的假失败。
func TestConcurrentLawSourceSwap(t *testing.T) {
	a := newCaseApp(t, t.TempDir())
	var (
		track sync.Mutex
		stubs []*swapStub
	)
	newStub := func() *swapStub {
		s := &swapStub{}
		track.Lock()
		stubs = append(stubs, s)
		track.Unlock()
		return s
	}
	a.setLawSource(newStub())

	var wg sync.WaitGroup
	stop := make(chan struct{})
	var okSearch, calls int64
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := a.SearchLaw("违约金", 5); err == nil {
					atomic.AddInt64(&okSearch, 1)
				}
				atomic.AddInt64(&calls, 1)
				runtime.Gosched()
			}
		}()
	}
	// 启动 barrier：每个读者至少完成一次检索后再开始换语料（防负载下读者缺席）
	for n := 0; atomic.LoadInt64(&calls) < 4 && n < 1_000_000; n++ {
		runtime.Gosched()
	}
	for i := 0; i < 20; i++ {
		a.closeLawSource()
		a.setLawSource(newStub())
	}
	// 换完后保留读者活跃窗口：等到出现成功检索再停，断言窗口覆盖正常完成路径
	for n := 0; atomic.LoadInt64(&okSearch) == 0 && n < 1_000_000; n++ {
		runtime.Gosched()
	}
	close(stop)
	wg.Wait()

	track.Lock()
	defer track.Unlock()
	for i, s := range stubs {
		if s.closed.Load() && s.useAfterClose.Load() {
			t.Errorf("第 %d 个句柄被关闭后仍被查询（锁未覆盖插件调用全程）", i)
		}
	}
	if atomic.LoadInt64(&okSearch) == 0 {
		t.Error("并发换语料期间检索应仍可正常完成（不应全部失败）")
	}
}

// swapStub 可关闭的法条源：关闭后再被查询即计数（供并发测试断言）。
type swapStub struct {
	closed        atomic.Bool
	useAfterClose atomic.Bool
}

func (s *swapStub) Name() string { return "swapstub" }
func (s *swapStub) SearchLaw(ctx context.Context, query string, topK int) ([]plugins.LawHit, error) {
	if s.closed.Load() {
		s.useAfterClose.Store(true)
	}
	return []plugins.LawHit{{Title: "测试法", Article: "第1条", Status: "EFFECTIVE", ConfirmStatus: "confirmed"}}, nil
}
func (s *swapStub) GetLawDetail(ctx context.Context, lawID string) (plugins.LawHit, error) {
	return plugins.LawHit{Title: "测试法", Status: "EFFECTIVE", ConfirmStatus: "confirmed"}, nil
}
func (s *swapStub) Close() error {
	s.closed.Store(true)
	return nil
}

// ---------- v0.3 M5 卷宗混合检索绑定 ----------

// 模型未启用（默认离线）：绑定必须照常可用、Mode=keyword_only 并显式标注；
// 索引状态如实显示「未启用/已索引 0」；空检索词显式报错。
func TestSearchCaseDocsBindingOffline(t *testing.T) {
	a := newCaseApp(t, t.TempDir())
	if err := a.scope().SaveDocument("d1", "合同.pdf", map[int]string{
		1: "合同约定违约金为合同金额的百分之二十",
		2: "送达地址确认书一份",
	}); err != nil {
		t.Fatal(err)
	}
	rep, err := a.SearchCaseDocs("违约金", 5)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Mode != neutral.SearchModeKeywordOnly || !strings.Contains(rep.Note, "未启用语义召回") {
		t.Errorf("离线默认必须降级并显式标注: %+v", rep)
	}
	if len(rep.Hits) != 1 || rep.Hits[0].DocID != "d1" || rep.Hits[0].Page != 1 {
		t.Errorf("命中定位错误: %+v", rep.Hits)
	}
	if facts, _ := a.scope().ListFacts(); len(facts) != 0 {
		t.Errorf("检索不得写事实: %+v", facts)
	}
	st, err := a.CaseIndexStatus()
	if err != nil {
		t.Fatal(err)
	}
	if st["model_enabled"] != false || st["total_pages"] != 2 || st["indexed_pages"] != 0 || st["scope"] != "case" {
		t.Errorf("索引状态如实显示错误: %+v", st)
	}
	if _, err := a.SearchCaseDocs("   ", 5); err == nil {
		t.Error("空检索词必须报错")
	}
}
