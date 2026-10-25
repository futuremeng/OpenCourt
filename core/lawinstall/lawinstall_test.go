package lawinstall

import (
	"os"
	"path/filepath"
	"testing"
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
