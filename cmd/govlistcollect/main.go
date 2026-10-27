// govlistcollect 国家规章库清单批量采集（人在环代理浏览的 CLI 入口，与应用
// BrowserOS 面板同一代码路径 neutral.RunGovListBrowse——同一套硬护栏）。
//
// 用途：全库约 1000 页、单会话硬上限 200 页，分批续接：
//
//	govlistcollect -endpoint http://127.0.0.1:9010/mcp -start 1   -pages 200
//	govlistcollect -endpoint http://127.0.0.1:9010/mcp -start 201 -pages 200
//	…
//
// 起始页经官方「跳转」控件到达并核验页码（等价人工输入页码），绝不构造请求。
// 产物：-out 目录（默认 appData/incoming/govgz）下 list_agent_<ts>.json，
// 交 OpenLaw openlaw-govcn -source gzk 走详情页合规抓取 + 人工确认闸口。
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/futuremeng/OpenCourt/neutral"
	"github.com/futuremeng/OpenCourt/plugins"
)

func defaultOutDir() string {
	hd, err := os.UserHomeDir()
	if err != nil {
		return "incoming/govgz"
	}
	return filepath.Join(hd, "Library", "Application Support", "OpenCourt", "incoming", "govgz")
}

func main() {
	endpoint := flag.String("endpoint", os.Getenv("OPENCOURT_BROWSEROS_ENDPOINT"), "BrowserOS neo MCP 端点（必填）")
	cat := flag.String("cat", "", "分类过滤：部门规章 / 地方政府规章 / 空 = 混合清单全收")
	start := flag.Int("start", 1, "起始页（官方跳转控件续接）")
	pages := flag.Int("pages", 200, "单会话翻页上限（硬上限 200）")
	outDir := flag.String("out", defaultOutDir(), "清单输出目录")
	flag.Parse()

	if *endpoint == "" {
		fmt.Fprintln(os.Stderr, "必须提供 -endpoint（或环境变量 OPENCOURT_BROWSEROS_ENDPOINT）")
		os.Exit(2)
	}
	b := plugins.NewBrowserOSPlugin(*endpoint, true)
	ctx := context.Background()
	t0 := time.Now()
	rep, err := neutral.RunGovListBrowse(ctx, b, neutral.GovListOptions{
		Cat: *cat, MaxPages: *pages, StartPage: *start,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "浏览失败（已浏览 %d 页）: %v\n", rep.Pages, err)
		os.Exit(1)
	}
	if len(rep.Entries) == 0 {
		if rep.StoppedAt == "end_of_list" {
			fmt.Printf("会话结束：起始页越界（越过末页），0 条\n")
			return // 正常空会话，不中断批次
		}
		fmt.Fprintf(os.Stderr, "未收集到条目（%s）\n", rep.StoppedAt)
		os.Exit(1)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out := filepath.Join(*outDir, "list_agent_"+time.Now().UTC().Format("20060102T150405")+".json")
	bb, err := json.MarshalIndent(rep.Entries, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, bb, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sum := sha256.Sum256(bb)
	fmt.Printf("浏览 %d 页（起始页 %d）得 %d 条（%s），用时 %s\n→ %s sha256=%s\n",
		rep.Pages, rep.StartPage, len(rep.Entries), rep.StoppedAt,
		time.Since(t0).Round(time.Second), out, hex.EncodeToString(sum[:])[:12])
}
