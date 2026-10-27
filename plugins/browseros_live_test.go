//go:build live

// browseros_live_test.go 真机联调测试（默认不参与 go test ./...）：
// 需本机运行 BrowserOS neo 并设置环境变量指向其 MCP 端点：
//
//	OPENCOURT_BROWSEROS_ENDPOINT=http://127.0.0.1:<port>/mcp \
//	  go test -tags live ./plugins -run Live -v
//
// 端点从 BrowserOS neo：新标签页 → 侧栏 MCP → 复制顶部 URL（端口随构建变化）。
package plugins_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/futuremeng/OpenCourt/neutral"
	"github.com/futuremeng/OpenCourt/plugins"
)

func TestLiveBrowserOSResearch(t *testing.T) {
	endpoint := os.Getenv("OPENCOURT_BROWSEROS_ENDPOINT")
	if endpoint == "" {
		t.Skip("未设置 OPENCOURT_BROWSEROS_ENDPOINT，跳过真机联调")
	}
	p := plugins.NewBrowserOSPlugin(endpoint, true)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	page, err := p.Open(ctx, "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	content, err := p.ReadPage(ctx, page.PageID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content.Markdown, "domain") {
		t.Fatalf("正文异常: %.200q", content.Markdown)
	}
	if content.URL == "" || content.ContentSHA == "" {
		t.Fatalf("来源/哈希缺失: %+v", content)
	}
	lines, err := p.SearchPage(ctx, page.PageID, "example")
	if err != nil || len(lines) == 0 {
		t.Fatalf("检索失败: %v %v", err, lines)
	}
	// P2 只读部分：可交互快照（含 [ref=eN] 句柄）。写操作不在真机测试中执行。
	snap, err := p.Snapshot(ctx, page.PageID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snap, "[ref=e") {
		t.Fatalf("快照缺句柄: %.200q", snap)
	}
	if err := p.ClosePage(ctx, page.PageID); err != nil {
		t.Fatal(err)
	}
	t.Logf("真机联调通过：page=%d origin=%s sha=%s 命中 %d 行 快照 %d 字节",
		page.PageID, content.URL, content.ContentSHA[:12], len(lines), len(snap))
}

// TestLiveGovListBrowse 国家规章库清单浏览真机联调（人在环代理浏览）。
// 双重显式开关：端点环境变量 + OPENCOURT_GOVLIST_LIVE=1（会真实访问 gov.cn
// 官方列表页并翻页，默认绝不自动跑）。浏览只走官方 UI，限速 ≥2s/页。
//
//	OPENCOURT_BROWSEROS_ENDPOINT=http://127.0.0.1:<port>/mcp \
//	OPENCOURT_GOVLIST_LIVE=1 go test -tags live ./plugins -run LiveGovList -v
func TestLiveGovListBrowse(t *testing.T) {
	endpoint := os.Getenv("OPENCOURT_BROWSEROS_ENDPOINT")
	if endpoint == "" || os.Getenv("OPENCOURT_GOVLIST_LIVE") != "1" {
		t.Skip("需同时设置 OPENCOURT_BROWSEROS_ENDPOINT 与 OPENCOURT_GOVLIST_LIVE=1")
	}
	p := plugins.NewBrowserOSPlugin(endpoint, true)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// 通过 neutral 层跑完整浏览编排（混合清单：cat 逐条推断；含 URL 硬校验与限速护栏）。
	rep, err := neutral.RunGovListBrowse(ctx, p, neutral.GovListOptions{MaxPages: 2})
	if err != nil {
		t.Fatalf("规章库浏览失败: %v（部分结果 %+v）", err, rep)
	}
	if rep.StartURL != neutral.GovListStartURL {
		t.Fatalf("起点必须是官方列表页: %s", rep.StartURL)
	}
	if len(rep.Entries) == 0 {
		t.Fatalf("未收集到条目（站点改版或页面未就绪？）: %+v", rep)
	}
	for _, e := range rep.Entries {
		if !strings.Contains(e.URL, ".gov.cn/") {
			t.Fatalf("条目 URL 越界: %s", e.URL)
		}
	}
	t.Logf("规章库浏览通过：%d 页 %d 条（%s），首条 %q [%s]",
		rep.Pages, len(rep.Entries), rep.StoppedAt, rep.Entries[0].Title, rep.Entries[0].Cat)
}
