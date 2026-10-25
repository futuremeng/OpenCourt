// 应用内数据包获取（发布索引 → 下载 → sha256 校验）：用户不再需要去
// 发布页手动下载，界面勾选即可让软件自动完成「发现 → 下载 → 校验 → 安装」。
// 索引由发布方生成（scripts/package-lawdata.sh → index.json，随包公示
// sha256）；下载完整性以索引中的 sha256 为准，内容真实性由包内 Ed25519
// 签名 manifest 持续校验（两层独立）。
package lawinstall

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
	"strings"
	"time"
)

// PackageInfo 索引中的一个可下载包。
type PackageInfo struct {
	Kind   string `json:"kind"` // core | evidence | full | update
	Name   string `json:"name"` // lawdata-core-20261025.zip
	Date   string `json:"date"` // YYYYMMDD（版本序）
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	URL    string `json:"url,omitempty"` // 绝对 URL；空 = 用索引 base_url 或 GitHub latest 规则
}

// PackageIndex 发布索引（index.json）。
type PackageIndex struct {
	GeneratedAt string        `json:"generated"`
	BaseURL     string        `json:"base_url,omitempty"` // 相对 URL 的前缀（镜像/网盘）
	Packages    []PackageInfo `json:"packages"`
}

// FetchIndex 拉取发布索引（15s 超时；ctx 可取消）。
func FetchIndex(ctx context.Context, url string) (*PackageIndex, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "OpenCourt/lawdata")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取数据包索引失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取数据包索引失败: HTTP %d", resp.StatusCode)
	}
	var idx PackageIndex
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&idx); err != nil {
		return nil, fmt.Errorf("数据包索引格式错误: %w", err)
	}
	return &idx, nil
}

// PackageURL 解析包的下载地址：绝对 URL 直接用；否则 base_url + name；
// 再无则取索引 URL 所在目录（GitHub Release 的 assets 与 index.json 同目录：
// …/releases/latest/download/<name>）。
func PackageURL(idx *PackageIndex, p PackageInfo, indexURL string) string {
	if strings.HasPrefix(p.URL, "http://") || strings.HasPrefix(p.URL, "https://") {
		return p.URL
	}
	if idx.BaseURL != "" {
		return strings.TrimSuffix(idx.BaseURL, "/") + "/" + p.Name
	}
	if i := strings.LastIndex(indexURL, "/"); i >= 0 {
		return indexURL[:i+1] + p.Name
	}
	return ""
}

// DownloadFile 下载 url 到 destPath 并流式校验 sha256（不匹配即删除报错）。
// progress 报告（已下载字节, 总字节或 -1）。ctx 取消 → 清理半成品。
func DownloadFile(ctx context.Context, url, destPath, wantSHA string, progress func(done, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "OpenCourt/lawdata")
	client := &http.Client{Timeout: 0} // 大包：不设全局限时，靠 ctx 控制
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()
	h := sha256.New()
	buf := make([]byte, 256*1024)
	var done int64
	lastReport := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			os.Remove(destPath)
			return err
		}
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				os.Remove(destPath)
				return werr
			}
			h.Write(buf[:n])
			done += int64(n)
			if progress != nil && time.Since(lastReport) > 500*time.Millisecond {
				progress(done, resp.ContentLength)
				lastReport = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			os.Remove(destPath)
			return fmt.Errorf("下载中断: %w", rerr)
		}
	}
	if progress != nil {
		progress(done, resp.ContentLength)
	}
	if wantSHA != "" && hex.EncodeToString(h.Sum(nil)) != strings.ToLower(wantSHA) {
		os.Remove(destPath)
		return fmt.Errorf("下载文件 sha256 与发布索引不符，已丢弃（可能被篡改或传输损坏）: %s", filepath.Base(destPath))
	}
	return nil
}
