package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/futuremeng/OpenLaw/store"

	"github.com/futuremeng/OpenCourt/config"
	"github.com/futuremeng/OpenCourt/core/audit"
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
