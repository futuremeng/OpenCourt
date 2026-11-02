// Package appupdate 程序自身的更新检查与自动更新（macOS）。
// 发布形态：GitHub Release 携带 OpenCourt-<ver>-darwin-<arch>.dmg 与同名
// .sha256 资产；客户端拉 releases/latest API 比较 semver，下载后先按
// sha256 校验完整性，再由分离的外部脚本完成「等待退出 → 挂载 dmg →
// 替换 .app → 重启」（自更新不经浏览器下载，无 quarantine 属性，
// 不受 Gatekeeper 弹窗干扰；正式分发仍建议 codesign+notarize）。
package appupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/futuremeng/OpenCourt/core/lawinstall"
)

// DefaultAllowedHosts 更新源白名单：官方仓库 API 与 Release 资产 CDN 域名。
//
// 为什么必须有白名单：settings.yaml 是用户可写文件。没有白名单时，一次改写
// app.update_api 就能把更新流指向任意主机——而 sha256 由同一台主机的响应
// 提供，等于「自证清白」，完整性校验形同虚设。要换源必须同时提供签名方案。
var DefaultAllowedHosts = []string{
	"api.github.com",
	"github.com",
	"objects.githubusercontent.com",
	"codeload.github.com",
}

// ValidateOrigin 校验 URL 是否落在可信源之内。白名单条目两种写法：
//
//   - 裸主机名（生产默认，如 "api.github.com"）：必须 https；
//   - 完整 origin（如 "http://127.0.0.1:8080"）：协议与端口逐字匹配——
//     仅供测试与自建内网源显式声明，绝不出现在默认白名单里。
//
// 只写主机名就想走 http 是不行的：那等于把更新流降级成明文可篡改。
func ValidateOrigin(rawURL string, allowed []string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("更新源地址无法解析: %w", err)
	}
	for _, a := range allowed {
		if strings.Contains(a, "://") { // 显式 origin 条目
			o, err := url.Parse(strings.TrimSpace(a))
			if err == nil && strings.EqualFold(o.Scheme, u.Scheme) &&
				strings.EqualFold(o.Host, u.Host) {
				return nil
			}
			continue
		}
		if u.Scheme == "https" && strings.EqualFold(u.Hostname(), a) {
			return nil
		}
	}
	return fmt.Errorf("更新源 %q 不在可信发布白名单内，拒绝访问（自建源需显式声明源地址并同时提供签名校验方案）", u.String())
}

func allowedOrDefault(allowed []string) []string {
	if len(allowed) > 0 && allowed[0] != "" {
		return allowed
	}
	return DefaultAllowedHosts
}

// assetNameMatches 资产名必须与 tag + 平台严格对应（OpenCourt-<tag>-<goos>-
// <arch>.dmg / -universal.dmg）。只按「有同名 .sha256 兄弟」挑包是不够的：
// 攻击者往 Release 里塞一个自带 sha256 的 dmg 即可被装上。
func assetNameMatches(name, tag, goos, goarch string) bool {
	if strings.TrimSpace(tag) == "" {
		return false
	}
	for _, t := range []string{tag, strings.TrimPrefix(tag, "v")} {
		for _, a := range []string{goarch, "universal"} {
			if name == fmt.Sprintf("OpenCourt-%s-%s-%s.dmg", t, goos, a) {
				return true
			}
		}
	}
	return false
}

// Asset Release 上的一个附件。
type Asset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Release GitHub Releases API 的 latest 响应（截取所需字段）。
type Release struct {
	TagName string  `json:"tag_name"`
	Body    string  `json:"body"`
	Assets  []Asset `json:"assets"`
}

// newClient 带跳转复检的客户端：每一跳都必须仍是 https 且在白名单内
// （GitHub 资产会 302 到 objects.githubusercontent.com，白名单已含）。
func newClient(timeout time.Duration, allowed []string) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			return ValidateOrigin(req.URL.String(), allowed)
		},
	}
}

// CheckLatest 拉取最新 Release（15s 超时；ctx 可取消）。
// allowed 省略时使用 DefaultAllowedHosts；测试可注入自己的主机名。
func CheckLatest(ctx context.Context, apiURL string, allowed ...string) (*Release, error) {
	host := allowedOrDefault(allowed)
	if err := ValidateOrigin(apiURL, host); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "OpenCourt/updater")
	resp, err := newClient(15*time.Second, host).Do(req)
	if err != nil {
		return nil, fmt.Errorf("检查更新失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("检查更新失败: HTTP %d", resp.StatusCode)
	}
	var rel Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		return nil, fmt.Errorf("更新信息格式错误: %w", err)
	}
	return &rel, nil
}

// CompareVersions 比较 semver（容忍 v 前缀与 -prerelease 后缀）：
// a<b → -1，相等 → 0，a>b → 1。正式版大于同号预发布版（0.2.0 > 0.2.0-preview）。
func CompareVersions(a, b string) int {
	pa, pb := parseVer(a), parseVer(b)
	for i := 0; i < 3; i++ {
		if pa.core[i] != pb.core[i] {
			if pa.core[i] < pb.core[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case pa.pre == pb.pre:
		return 0
	case pa.pre == "":
		return 1 // 正式版 > 预发布
	case pb.pre == "":
		return -1
	case pa.pre < pb.pre:
		return -1
	default:
		return 1
	}
}

type ver struct {
	core [3]int
	pre  string
}

func parseVer(s string) ver {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	pre := ""
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		pre, s = s[i+1:], s[:i]
	}
	var v ver
	v.pre = pre
	parts := strings.Split(s, ".")
	for i := 0; i < 3 && i < len(parts); i++ {
		for _, r := range parts[i] {
			if r < '0' || r > '9' {
				break
			}
			v.core[i] = v.core[i]*10 + int(r-'0')
		}
	}
	return v
}

// PickAssets 挑选当前平台的 dmg 与其 .sha256 资产
// （命名约定 OpenCourt-<ver>-<goos>-<goarch>.dmg；
// 兼容旧发布形态 -<goos>-universal.dmg）。找不到返回 nil。
func PickAssets(rel *Release, goos, goarch string) (dmg, sha *Asset) {
	suffixes := []string{"-" + goos + "-" + goarch + ".dmg", "-" + goos + "-universal.dmg"}
	for _, suffix := range suffixes {
		for i, as := range rel.Assets {
			if strings.HasSuffix(as.Name, suffix) {
				dmg = &rel.Assets[i]
				break
			}
		}
		if dmg != nil {
			break
		}
	}
	if dmg == nil {
		return nil, nil
	}
	for i, as := range rel.Assets {
		if as.Name == dmg.Name+".sha256" {
			sha = &rel.Assets[i]
			break
		}
	}
	return dmg, sha
}

// SHA256File 计算文件 sha256（hex）。
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// fetchText 下载小文件文本（.sha256 资产）。
func fetchText(ctx context.Context, url string, allowed []string) (string, error) {
	if err := ValidateOrigin(url, allowed); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "OpenCourt/updater")
	resp, err := newClient(30*time.Second, allowed).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("获取校验文件失败: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// firstHex64 提取文本中首个 64 位 hex 串（sha256sum 输出格式兼容）。
func firstHex64(s string) string {
	isHex := func(r rune) bool {
		return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
	}
	for _, f := range strings.Fields(s) {
		if len(f) == 64 && strings.TrimFunc(f, isHex) == "" {
			return strings.ToLower(f)
		}
	}
	return ""
}

// DownloadRelease 检查最新 Release 并把当前平台 dmg 下载到 cacheDir，
// 完成 sha256 完整性校验（发布缺 .sha256 资产 → 拒绝，不做无校验安装）。
// 三重来源约束（缺一不可，任一不满足即拒绝）：
//  1. API 与资产 URL 必须 https 且在可信主机白名单内（含每一跳跳转复检）；
//  2. 资产名必须与 tag + 平台严格对应——不能只认「带 sha256 兄弟的任意 dmg」；
//  3. sha256 必须与 .sha256 资产一致。
//
// allowed 省略时使用 DefaultAllowedHosts。返回已校验的 dmg 路径。
func DownloadRelease(ctx context.Context, apiURL, cacheDir string, progress func(done, total int64), allowed ...string) (string, error) {
	host := allowedOrDefault(allowed)
	rel, err := CheckLatest(ctx, apiURL, host...)
	if err != nil {
		return "", err
	}
	dmg, sha := PickAssets(rel, runtime.GOOS, runtime.GOARCH)
	if dmg == nil {
		return "", fmt.Errorf("最新 Release 没有当前平台（%s-%s）的安装包", runtime.GOOS, runtime.GOARCH)
	}
	if !assetNameMatches(dmg.Name, rel.TagName, runtime.GOOS, runtime.GOARCH) {
		return "", fmt.Errorf("安装包名 %q 与版本 %s / 平台不匹配，拒绝安装（发布产物须命名 OpenCourt-<tag>-%s-<arch>.dmg）",
			dmg.Name, rel.TagName, runtime.GOOS)
	}
	if sha == nil {
		return "", fmt.Errorf("发布缺少 %s.sha256 校验资产，拒绝无完整性校验的安装", dmg.Name)
	}
	if err := ValidateOrigin(dmg.BrowserDownloadURL, host); err != nil {
		return "", err
	}
	shaText, err := fetchText(ctx, sha.BrowserDownloadURL, host)
	if err != nil {
		return "", err
	}
	want := firstHex64(shaText)
	if want == "" {
		return "", fmt.Errorf("校验文件格式无法解析 sha256: %s", sha.Name)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(cacheDir, dmg.Name)
	if err := lawinstall.DownloadFile(ctx, dmg.BrowserDownloadURL, dest, want, progress); err != nil {
		return "", err
	}
	return dest, nil
}

// UpdaterScript 生成分离执行的替换脚本（macOS）：等待主进程（pid）退出 →
// 挂载 dmg → 来源校验 → 换 .app（失败回滚旧版）→ 卸载 → 清理 → 重启新版本。
//
// allowUnsigned=false（默认）：挂载后先跑 Gatekeeper 评估（spctl）再落盘——
// 完整性校验只证明「和发布文件一致」，不证明「发布方是谁」；自构建想跳过
// 评估必须显式传 true（并在审计里留痕）。
func UpdaterScript(dmg, app string, pid int, allowUnsigned bool) string {
	assess := `if command -v spctl >/dev/null 2>&1; then
  if ! spctl --assess --type execute --verbose "$MNT/OpenCourt.app" 2>>"$LOG"; then
    echo "spctl: Gatekeeper 评估未通过" >>"$LOG"
    hdiutil detach "$MNT" >/dev/null
    exit 3
  fi
fi
`
	if allowUnsigned {
		assess = `# allow_unsigned=true：跳过 Gatekeeper 评估（仅限自构建/内部分发）
`
	}
	return fmt.Sprintf(`#!/bin/sh
# OpenCourt 自更新脚本（由应用生成，分离进程执行）
set -u
DMG=%[1]q
APP=%[2]q
PID=%[3]d
MNT="/tmp/opencourt-update-$$"
LOG="/tmp/opencourt-update-$$.log"
while kill -0 "$PID" 2>/dev/null; do sleep 0.3; done
if ! hdiutil attach "$DMG" -nobrowse -mountpoint "$MNT" >/dev/null; then exit 1; fi
%[4]sif [ -d "$APP" ]; then mv "$APP" "$APP.updating-old" || { hdiutil detach "$MNT" >/dev/null; exit 1; }; fi
if ! cp -R "$MNT/OpenCourt.app" "$APP"; then
  mv "$APP.updating-old" "$APP"   # 回滚旧版
  hdiutil detach "$MNT" >/dev/null
  exit 1
fi
rm -rf "$APP.updating-old"
hdiutil detach "$MNT" >/dev/null
rm -f "$DMG"
open "$APP"
`, dmg, app, pid, assess)
}
