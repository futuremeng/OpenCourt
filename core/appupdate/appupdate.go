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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/futuremeng/OpenCourt/core/lawinstall"
)

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

// CheckLatest 拉取最新 Release（15s 超时；ctx 可取消）。
func CheckLatest(ctx context.Context, apiURL string) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "OpenCourt/updater")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
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
func fetchText(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "OpenCourt/updater")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
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
// 返回已校验的 dmg 路径。
func DownloadRelease(ctx context.Context, apiURL, cacheDir string, progress func(done, total int64)) (string, error) {
	rel, err := CheckLatest(ctx, apiURL)
	if err != nil {
		return "", err
	}
	dmg, sha := PickAssets(rel, runtime.GOOS, runtime.GOARCH)
	if dmg == nil {
		return "", fmt.Errorf("最新 Release 没有当前平台（%s-%s）的安装包", runtime.GOOS, runtime.GOARCH)
	}
	if sha == nil {
		return "", fmt.Errorf("发布缺少 %s.sha256 校验资产，拒绝无完整性校验的安装", dmg.Name)
	}
	shaText, err := fetchText(ctx, sha.BrowserDownloadURL)
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
// 挂载 dmg → 换 .app（失败回滚旧版）→ 卸载 → 清理 → 重启新版本。
func UpdaterScript(dmg, app string, pid int) string {
	return fmt.Sprintf(`#!/bin/sh
# OpenCourt 自更新脚本（由应用生成，分离进程执行）
set -u
DMG=%[1]q
APP=%[2]q
PID=%[3]d
MNT="/tmp/opencourt-update-$$"
while kill -0 "$PID" 2>/dev/null; do sleep 0.3; done
if ! hdiutil attach "$DMG" -nobrowse -mountpoint "$MNT" >/dev/null; then exit 1; fi
if [ -d "$APP" ]; then mv "$APP" "$APP.updating-old" || { hdiutil detach "$MNT" >/dev/null; exit 1; }; fi
if ! cp -R "$MNT/OpenCourt.app" "$APP"; then
  mv "$APP.updating-old" "$APP"   # 回滚旧版
  hdiutil detach "$MNT" >/dev/null
  exit 1
fi
rm -rf "$APP.updating-old"
hdiutil detach "$MNT" >/dev/null
rm -f "$DMG"
open "$APP"
`, dmg, app, pid)
}
