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
	s := UpdaterScript("/tmp/a.dmg", "/Applications/OpenCourt.app", 4242, false)
	for _, want := range []string{"hdiutil attach", "hdiutil detach", `mv "$APP" "$APP.updating-old"`,
		`cp -R "$MNT/OpenCourt.app" "$APP"`, `open "$APP"`, "kill -0", "4242"} {
		if !strings.Contains(s, want) {
			t.Errorf("脚本缺关键步骤: %s", want)
		}
	}
	// 落盘前必须过 Gatekeeper 评估（sha256 只证明内容一致，不证明发布方是谁）
	if !strings.Contains(s, "spctl --assess") {
		t.Error("默认必须执行 Gatekeeper 评估")
	}
	if !strings.Contains(s, "exit 3") {
		t.Error("评估未通过必须中止（不得落盘）")
	}
	// 显式 allow_unsigned 才允许跳过
	s2 := UpdaterScript("/tmp/a.dmg", "/Applications/OpenCourt.app", 4242, true)
	if strings.Contains(s2, "spctl --assess") {
		t.Error("allow_unsigned=true 才应跳过评估")
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

// serveReleaseNamed 模拟 GitHub Releases API：资产名可指定（默认按当前真实平台命名）。
func serveReleaseNamed(t *testing.T, dmgBody []byte, shaBody string, withSHA bool, nameOverride string) *httptest.Server {
	t.Helper()
	name := nameOverride
	if name == "" {
		name = "OpenCourt-9.9.9-" + runtime.GOOS + "-" + runtime.GOARCH + ".dmg"
	}
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

func serveRelease(t *testing.T, dmgBody []byte, shaBody string, withSHA bool) *httptest.Server {
	t.Helper()
	return serveReleaseNamed(t, dmgBody, shaBody, withSHA, "")
}

// TestValidateOrigin 更新源白名单（P1-6）：settings.yaml 用户可写，没有白名单
// 就能把更新流指向任意主机——而 sha256 由同一主机提供，等于自证清白。
func TestValidateOrigin(t *testing.T) {
	ok := []string{
		"https://api.github.com/repos/futuremeng/OpenCourt/releases/latest",
		"https://objects.githubusercontent.com/x/y.dmg",
	}
	for _, u := range ok {
		if err := ValidateOrigin(u, DefaultAllowedHosts); err != nil {
			t.Errorf("可信源被拒: %s: %v", u, err)
		}
	}
	bad := []string{
		"http://api.github.com/releases/latest",          // 明文
		"https://evil.example.com/releases/latest",       // 白名单外
		"https://api.github.com.evil.example.com/latest", // 后缀伪装
		"file:///tmp/fake/releases/latest",               // 非 http
	}
	for _, u := range bad {
		if err := ValidateOrigin(u, DefaultAllowedHosts); err == nil {
			t.Errorf("不可信源被放行: %s", u)
		}
	}
	// 显式 origin 条目（测试/内网自建源）：协议与端口必须逐字匹配
	if err := ValidateOrigin("http://127.0.0.1:9999/latest", []string{"http://127.0.0.1:9999"}); err != nil {
		t.Errorf("显式声明的源应放行: %v", err)
	}
	if err := ValidateOrigin("http://127.0.0.1:9998/latest", []string{"http://127.0.0.1:9999"}); err == nil {
		t.Error("端口不同必须拒绝")
	}
}

// TestAssetNameMatches 资产名必须与 tag + 平台严格对应。
func TestAssetNameMatches(t *testing.T) {
	if !assetNameMatches("OpenCourt-v0.2.0-darwin-arm64.dmg", "v0.2.0", "darwin", "arm64") {
		t.Error("带 v 前缀命名应匹配")
	}
	if !assetNameMatches("OpenCourt-0.2.0-darwin-universal.dmg", "v0.2.0", "darwin", "arm64") {
		t.Error("universal 命名应匹配")
	}
	for _, n := range []string{
		"OpenCourt-0.2.0-windows-amd64.dmg", // 平台不符
		"OpenCourt-0.9.9-darwin-arm64.dmg",  // 版本不符
		"evil-darwin-arm64.dmg",             // 完全无关
	} {
		if assetNameMatches(n, "v0.2.0", "darwin", "arm64") {
			t.Errorf("不该匹配: %s", n)
		}
	}
}

func TestDownloadRelease(t *testing.T) {
	dmgBody := []byte("fake dmg bytes")
	sum := sha256.Sum256(dmgBody)
	goodSHA := hex.EncodeToString(sum[:]) + "  OpenCourt.dmg\n"

	srv := serveRelease(t, dmgBody, goodSHA, true)
	defer srv.Close()
	dest, err := DownloadRelease(context.Background(), srv.URL+"/latest", t.TempDir(), nil, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(dest, ".dmg") {
		t.Errorf("应返回 dmg 路径: %s", dest)
	}

	// 默认白名单（不含测试服务器）→ 必须拒绝：证明生产路径不会下载任意源
	if _, err := DownloadRelease(context.Background(), srv.URL+"/latest", t.TempDir(), nil); err == nil {
		t.Error("非白名单更新源必须拒绝")
	}

	// sha 不符 → 拒绝
	srvBad := serveRelease(t, dmgBody, strings.Repeat("be", 32)+"  x\n", true)
	defer srvBad.Close()
	if _, err := DownloadRelease(context.Background(), srvBad.URL+"/latest", t.TempDir(), nil, srvBad.URL); err == nil {
		t.Error("sha256 不符应拒绝")
	}

	// 缺 sha 资产 → 拒绝（不做无校验安装）
	srvNo := serveRelease(t, dmgBody, "", false)
	defer srvNo.Close()
	if _, err := DownloadRelease(context.Background(), srvNo.URL+"/latest", t.TempDir(), nil, srvNo.URL); err == nil {
		t.Error("缺 .sha256 资产应拒绝")
	}

	// 资产名与 tag 不符（Release 里塞入的任意 dmg）→ 拒绝
	srvEvil := serveReleaseNamed(t, dmgBody, goodSHA, true, "OpenCourt-0.0.1-"+runtime.GOOS+"-"+runtime.GOARCH+".dmg")
	defer srvEvil.Close()
	if _, err := DownloadRelease(context.Background(), srvEvil.URL+"/latest", t.TempDir(), nil, srvEvil.URL); err == nil {
		t.Error("资产名与版本不符必须拒绝")
	}
}
