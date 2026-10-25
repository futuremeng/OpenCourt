package appupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.1.0", "0.1.0", 0},
		{"v0.1.1", "0.1.0", 1},
		{"v0.1.0", "0.2.0", -1},
		{"v1.0.0", "0.9.9", 1},
		{"0.2.0", "0.2.0-preview", 1}, // 正式版 > 预发布
		{"0.2.0-preview", "0.2.0", -1},
		{"0.1.0-preview", "0.1.0-preview", 0},
		{"v2.17.0", "2.9.0", 1},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestPickAssets(t *testing.T) {
	rel := &Release{Assets: []Asset{
		{Name: "OpenCourt-0.2.0-darwin-arm64.dmg"},
		{Name: "OpenCourt-0.2.0-darwin-arm64.dmg.sha256"},
		{Name: "OpenCourt-0.2.0-darwin-amd64.dmg"},
		{Name: "index.json"},
	}}
	dmg, sha := PickAssets(rel, "darwin", "arm64")
	if dmg == nil || dmg.Name != "OpenCourt-0.2.0-darwin-arm64.dmg" {
		t.Fatalf("dmg 挑选不符: %+v", dmg)
	}
	if sha == nil || sha.Name != dmg.Name+".sha256" {
		t.Errorf("sha256 资产挑选不符: %+v", sha)
	}
	if d2, _ := PickAssets(rel, "darwin", "riscv"); d2 != nil {
		t.Error("无匹配平台应返回 nil")
	}

	// 旧发布形态：universal dmg 作为回退
	relU := &Release{Assets: []Asset{
		{Name: "OpenCourt-0.2.0-darwin-universal.dmg"},
		{Name: "OpenCourt-0.2.0-darwin-universal.dmg.sha256"},
	}}
	du, su := PickAssets(relU, "darwin", "arm64")
	if du == nil || su == nil || du.Name != "OpenCourt-0.2.0-darwin-universal.dmg" {
		t.Errorf("应回退到 universal 包: %+v / %+v", du, su)
	}
	// 精确平台包优先于 universal
	relB := &Release{Assets: []Asset{
		{Name: "OpenCourt-0.2.0-darwin-universal.dmg"},
		{Name: "OpenCourt-0.2.0-darwin-arm64.dmg"},
	}}
	db, _ := PickAssets(relB, "darwin", "arm64")
	if db == nil || db.Name != "OpenCourt-0.2.0-darwin-arm64.dmg" {
		t.Errorf("精确平台包应优先: %+v", db)
	}
}

func TestUpdaterScript(t *testing.T) {
	s := UpdaterScript("/tmp/a.dmg", "/Applications/OpenCourt.app", 4242)
	for _, want := range []string{"hdiutil attach", "hdiutil detach", `mv "$APP" "$APP.updating-old"`,
		`cp -R "$MNT/OpenCourt.app" "$APP"`, `open "$APP"`, "kill -0", "4242"} {
		if !strings.Contains(s, want) {
			t.Errorf("脚本缺关键步骤: %s", want)
		}
	}
}

func TestFirstHex64(t *testing.T) {
	if got := firstHex64("ab12  x.dmg"); got != "" {
		t.Error("非 64hex 应空")
	}
	h := strings.Repeat("a1", 32)
	if got := firstHex64(h + "  OpenCourt.dmg\n"); got != h {
		t.Errorf("应提取 64hex: %q", got)
	}
}

// serveRelease 模拟 GitHub Releases API：按当前真实平台命名资产。
func serveRelease(t *testing.T, dmgBody []byte, shaBody string, withSHA bool) *httptest.Server {
	t.Helper()
	name := "OpenCourt-9.9.9-" + runtime.GOOS + "-" + runtime.GOARCH + ".dmg"
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		assets := []Asset{{Name: name, Size: int64(len(dmgBody)), BrowserDownloadURL: base + "/app.dmg"}}
		if withSHA {
			assets = append(assets, Asset{Name: name + ".sha256", BrowserDownloadURL: base + "/app.dmg.sha256"})
		}
		_ = json.NewEncoder(w).Encode(Release{TagName: "v9.9.9", Assets: assets})
	})
	mux.HandleFunc("/app.dmg", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(dmgBody) })
	mux.HandleFunc("/app.dmg.sha256", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(shaBody)) })
	srv := httptest.NewServer(mux)
	base = srv.URL
	return srv
}

func TestDownloadRelease(t *testing.T) {
	dmgBody := []byte("fake dmg bytes")
	sum := sha256.Sum256(dmgBody)
	goodSHA := hex.EncodeToString(sum[:]) + "  OpenCourt.dmg\n"

	srv := serveRelease(t, dmgBody, goodSHA, true)
	defer srv.Close()
	dest, err := DownloadRelease(context.Background(), srv.URL+"/latest", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(dest, ".dmg") {
		t.Errorf("应返回 dmg 路径: %s", dest)
	}

	// sha 不符 → 拒绝
	srvBad := serveRelease(t, dmgBody, strings.Repeat("be", 32)+"  x\n", true)
	defer srvBad.Close()
	if _, err := DownloadRelease(context.Background(), srvBad.URL+"/latest", t.TempDir(), nil); err == nil {
		t.Error("sha256 不符应拒绝")
	}

	// 缺 sha 资产 → 拒绝（不做无校验安装）
	srvNo := serveRelease(t, dmgBody, "", false)
	defer srvNo.Close()
	if _, err := DownloadRelease(context.Background(), srvNo.URL+"/latest", t.TempDir(), nil); err == nil {
		t.Error("缺 .sha256 资产应拒绝")
	}
}
