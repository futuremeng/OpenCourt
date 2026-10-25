package lawinstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchIndexAndDownload(t *testing.T) {
	zipBody := []byte("PK fake zip body")
	sum := sha256.Sum256(zipBody)
	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(PackageIndex{
			GeneratedAt: "2026-10-25",
			Packages: []PackageInfo{{
				Kind: "core", Name: "lawdata-core-20261025.zip",
				Date: "20261025", Size: int64(len(zipBody)),
				SHA256: hex.EncodeToString(sum[:]),
			}},
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(zipBody)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	idx, err := FetchIndex(context.Background(), srv.URL+"/index.json")
	if err != nil {
		t.Fatal(err)
	}
	idx.BaseURL = srv.URL // 测试用镜像前缀
	if len(idx.Packages) != 1 || idx.Packages[0].Kind != "core" {
		t.Fatalf("索引解析不符: %+v", idx)
	}
	url := PackageURL(idx, idx.Packages[0], srv.URL+"/index.json")
	if url == "" {
		t.Fatal("base_url URL 解析失败")
	}

	dest := filepath.Join(t.TempDir(), "dl.zip")
	if err := DownloadFile(context.Background(), url, dest, hex.EncodeToString(sum[:]), nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != string(zipBody) {
		t.Error("下载内容不符")
	}

	// sha256 不符 → 报错且不留半成品
	if err := DownloadFile(context.Background(), url, dest+"2", "deadbeef", nil); err == nil {
		t.Error("sha256 不符应报错")
	}
	if _, err := os.Stat(dest + "2"); !os.IsNotExist(err) {
		t.Error("校验失败应删除半成品")
	}
}

func TestCorpusDate(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "manifest"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest", "manifest.json"),
		[]byte(`{"generated_at":"2026-10-25T08:00:00Z","files":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := corpusDate(dir); got != "20261025" {
		t.Errorf("corpusDate: got %q", got)
	}
}
