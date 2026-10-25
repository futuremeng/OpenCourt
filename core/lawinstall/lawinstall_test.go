package lawinstall

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/futuremeng/OpenLaw/store"
)

// mkLawData 造一个最小合法数据目录（db/openlaw.db + 一个文件 + logs/）。
func mkLawData(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "db", "openlaw.db"), []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "docx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docx", "a.docx"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "logs", "old.log"), []byte("skip"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindBundled(t *testing.T) {
	vol := t.TempDir()
	exe := filepath.Join(vol, "OpenCourt.app", "Contents", "MacOS", "OpenCourt")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 无 law-data → ""
	if got := FindBundled(exe); got != "" {
		t.Errorf("无随包数据应空, got %q", got)
	}
	// 卷根布局命中
	mkLawData(t, filepath.Join(vol, "law-data"))
	if got := FindBundled(exe); got != filepath.Join(vol, "law-data") {
		t.Errorf("卷根 law-data 应命中, got %q", got)
	}
}

func TestFindDownloaded(t *testing.T) {
	dl := t.TempDir()
	if got := FindDownloaded(dl); got != "" {
		t.Errorf("空 Downloads 应空, got %q", got)
	}
	// 普通文件夹与非法语料目录都不认
	if err := os.MkdirAll(filepath.Join(dl, "law-data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dl, "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := FindDownloaded(dl); got != "" {
		t.Errorf("无 db/openlaw.db 不应命中, got %q", got)
	}
	// 解压出 law-data（含合法 db）→ 命中；重名后缀 "law-data 2" 也认
	mkLawData(t, filepath.Join(dl, "law-data"))
	if got := FindDownloaded(dl); got != filepath.Join(dl, "law-data") {
		t.Errorf("应命中 law-data, got %q", got)
	}
}

func TestFindEvidenceAndMerge(t *testing.T) {
	dl := t.TempDir()
	ev := filepath.Join(dl, "law-evidence")
	for _, layer := range []string{"docx", "md", "provenance"} {
		if err := os.MkdirAll(filepath.Join(ev, layer), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(ev, layer, "f1"), []byte("evidence"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := FindEvidence(dl); got != ev {
		t.Errorf("law-evidence 应命中, got %q", got)
	}

	// 合并进已安装数据目录
	data := t.TempDir()
	mkLawData(t, data)
	if err := MergeEvidence(ev, data, nil); err != nil {
		t.Fatal(err)
	}
	for _, layer := range []string{"docx", "md", "provenance"} {
		if _, err := os.Stat(filepath.Join(data, layer, "f1")); err != nil {
			t.Errorf("%s/f1 应已合并: %v", layer, err)
		}
	}
	// 幂等 + 不覆盖本机同名文件
	if err := os.WriteFile(filepath.Join(data, "docx", "f1"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := MergeEvidence(ev, data, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(data, "docx", "f1"))
	if string(got) != "mine" {
		t.Error("合并不得覆盖本机已有文件")
	}
	// 目标非法 → 报错
	if err := MergeEvidence(ev, filepath.Join(t.TempDir(), "none"), nil); err == nil {
		t.Error("目标非数据目录应报错")
	}
}

func TestInstallCopyAndIdempotent(t *testing.T) {
	src := t.TempDir()
	mkLawData(t, src)
	target := filepath.Join(t.TempDir(), "law-data")

	if err := Install(src, target, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "db", "openlaw.db")); err != nil {
		t.Fatal("数据应已安装:", err)
	}
	if _, err := os.Stat(filepath.Join(target, "logs")); err == nil {
		t.Error("logs/ 不应随包安装")
	}
	// 幂等：已有数据不重拷（改动保留）
	if err := os.WriteFile(filepath.Join(target, "db", "marker"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(src, target, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "db", "marker")); err != nil {
		t.Error("重复安装不应覆盖本机数据")
	}
	// 半途失败不留 .installing 残骸：目标非法 + 源非法 → 报错
	if err := Install(filepath.Join(t.TempDir(), "none"), filepath.Join(t.TempDir(), "x"), nil); err == nil {
		t.Error("源缺 db 应报错")
	}
}

// --- 数据包显式加载：识别 / zip / 替换 / 状态 ---

func makeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for name, body := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClassify(t *testing.T) {
	core := t.TempDir()
	if err := os.MkdirAll(filepath.Join(core, "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(core, "db", "openlaw.db"), []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Classify(core); got != "core" {
		t.Errorf("core: got %q", got)
	}

	full := t.TempDir()
	mkLawData(t, full)
	if got := Classify(full); got != "full" {
		t.Errorf("full: got %q", got)
	}

	ev := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ev, "md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ev, "md", "a.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Classify(ev); got != "evidence" {
		t.Errorf("evidence: got %q", got)
	}

	up := t.TempDir()
	if err := os.WriteFile(filepath.Join(up, "changes.csv"), []byte("action,doc_id\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Classify(up); got != "update" {
		t.Errorf("update: got %q", got)
	}

	if got := Classify(t.TempDir()); got != "" {
		t.Errorf("空目录应不可识别: got %q", got)
	}
}

func TestNormalizeSourceZip(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "lawdata-core.zip")
	makeZip(t, zipPath, map[string]string{
		"law-data/db/openlaw.db": "db",
		"__MACOSX/junk":          "x",
	})
	src, tmp, err := NormalizeSource(zipPath)
	if tmp != "" {
		defer os.RemoveAll(tmp)
	}
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(src) != "law-data" {
		t.Errorf("应进入唯一根目录 law-data, got %s", src)
	}
	if got := Classify(src); got != "core" {
		t.Errorf("zip 内容应识别为 core: got %q", got)
	}
}

func TestExtractZipSlip(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "evil.zip")
	makeZip(t, zipPath, map[string]string{"../escape.txt": "evil"})
	if err := ExtractZip(zipPath, t.TempDir()); err == nil {
		t.Error("zip-slip 条目应被拒绝")
	}
}

func TestReplaceAtomic(t *testing.T) {
	target := t.TempDir()
	mkLawData(t, target)
	if err := os.WriteFile(filepath.Join(target, "db", "marker"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	mkLawData(t, src)
	if err := os.WriteFile(filepath.Join(src, "db", "marker"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Replace(src, target, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(target, "db", "marker"))
	if string(got) != "new" {
		t.Errorf("替换后应为新语料, got %q", got)
	}
	if _, err := os.Stat(target + ".old"); !os.IsNotExist(err) {
		t.Error("替换成功不应留下 .old")
	}
	// 源非法 → 旧语料原样保留
	if err := Replace(filepath.Join(t.TempDir(), "none"), target, nil); err == nil {
		t.Error("源非法应报错")
	}
	got, _ = os.ReadFile(filepath.Join(target, "db", "marker"))
	if string(got) != "new" {
		t.Error("失败替换不得破坏现有语料")
	}
}

func TestStatus(t *testing.T) {
	st := Status(t.TempDir())
	if st.Installed {
		t.Error("空目录应未安装")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "db", "openlaw.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO document (doc_id, file_name, file_path, file_size, sha256, hash_computed_at, confirm_status) VALUES ('d1','d1.docx','docx/d1.docx',1,'aa','2026-01-01','confirmed')"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := os.MkdirAll(filepath.Join(dir, "provenance"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "provenance", "p.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	st = Status(dir)
	if !st.Installed || st.Docs != 1 || !st.EvidenceOK || st.ManifestSigned {
		t.Errorf("状态汇总不符: %+v", st)
	}
	if st.License != "" {
		t.Errorf("无 manifest 时许可应为空: %q", st.License)
	}
	// manifest 带机器可读许可声明 → 状态透出
	if err := os.MkdirAll(filepath.Join(dir, "manifest"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest", "manifest.json"),
		[]byte(`{"generated_at":"2026-10-25T00:00:00Z","license":"公有领域 + CC BY 4.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Status(dir).License; got != "公有领域 + CC BY 4.0" {
		t.Errorf("许可声明未透出: %q", got)
	}
}
