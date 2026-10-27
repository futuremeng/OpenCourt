// govlistassist 国家规章库清单浏览（人在环代理浏览；合规画像见 OpenLaw
// docs/sources-govcn.md，浏览模式扩展见 docs/browseros-integration.md 第 7 节）。
//
// 背景：规章库（www.gov.cn/zhengce/xxgk/gjgzk/）的检索 API（sousuoht Athena）带
// RSA appKey 应用层鉴权、sousuo 检索域 robots Disallow——清单不得由程序调用
// 检索接口发现。合规替代：智能体驱动用户自己的真实浏览器（BrowserOS neo），
// 只在主站 robots 允许的公开列表页上，通过官方 UI 人工式翻页浏览，读 DOM 收集
// 条目（与用户自己浏览并点「导出清单」等价），产物 list.json 交 OpenLaw
// openlaw-govcn -source gzk 走详情页合规抓取通道。
//
// 硬护栏（代码级，不是提示词）：
//   - 只允许打开固定起点 URL（规章库官方列表页），翻页只点官方「下一页」控件，
//     绝不构造检索请求、绝不访问 sousuo/sousuoht 等检索域、绝不触碰鉴权；
//   - evaluate 脚本只读 DOM（无 fetch/XHR/导航），内容仅作数据解析；
//   - 礼貌限速：翻页间隔下限 2s（默认 2.5s），单会话页数上限 200；
//   - 条目 URL 硬校验（https + www.gov.cn + content_<id> 形态），不合规条目丢弃；
//   - 点击「下一页」仍走 Propose→gate 红线→Execute 两段式（审计留痕）；
//   - flk.npc.gov.cn 等硬禁止站不适用本任务（起点 URL 白名单从机制上排除）。
package neutral

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/futuremeng/OpenCourt/plugins"
)

// GovListStartURL 国家规章库官方列表页（唯一允许的浏览起点）。
const GovListStartURL = "https://www.gov.cn/zhengce/xxgk/gjgzk/index.htm"

// govListEntryURLRe 条目详情页硬校验（与 OpenLaw ingest 清单校验同形态）。
var govListEntryURLRe = regexp.MustCompile(`^https://www\.gov\.cn/.+/content_\d+\.html?$`)

// 限速与规模护栏。
const (
	govListDefaultPages = 50
	govListMaxPages     = 200
	govListMinDelay     = 2 * time.Second
	govListDefaultDelay = 2500 * time.Millisecond
)

// GovListEntry 规章库清单条目（字段与 OpenLaw govgz list.json 完全一致）。
type GovListEntry struct {
	Title  string `json:"title"`
	URL    string `json:"url"`
	Cat    string `json:"cat,omitempty"`
	PubOrg string `json:"puborg,omitempty"`
	PCode  string `json:"pcode,omitempty"`
}

// GovListOptions 浏览任务参数。
type GovListOptions struct {
	Cat      string        // 分类页签：部门规章 / 地方政府规章（空 = 不切页签）
	MaxPages int           // 单会话翻页上限（默认 50，硬上限 200）
	Delay    time.Duration // 翻页间隔（默认 2.5s，下限 2s）
}

// GovListReport 浏览任务结果（OutPath 由调用方写盘后填充）。
type GovListReport struct {
	StartURL  string         `json:"start_url"`
	Cat       string         `json:"cat"`
	Pages     int            `json:"pages"`
	Entries   []GovListEntry `json:"entries"`
	StoppedAt string         `json:"stopped_at"` // done / max_pages / empty_page / no_next
	OutPath   string         `json:"out_path,omitempty"`
}

// govListExtractJS 页面内数据抽取脚本：从官方结果 DOM 读取条目（只读，无网络）。
// 结构来自站点渲染模板（div.depart_m > a[docurl] > h5 + p.de_part span）。
const govListExtractJS = `(() => {
  const out = [];
  document.querySelectorAll('div.depart_m').forEach((item) => {
    const a = item.querySelector('a[docurl]');
    const h5 = item.querySelector('h5');
    const sp = item.querySelector('p.de_part span');
    if (!a || !h5) return;
    let u = '';
    try { u = new URL(a.getAttribute('docurl'), location.href).href; } catch (e) {}
    out.push({
      title: (h5.textContent || '').replace(/\s+/g, ' ').trim(),
      url: u,
      puborg: sp ? (sp.textContent || '').replace(/\s+/g, ' ').trim() : ''
    });
  });
  return JSON.stringify(out);
})()`

// reNextPageRef 快照中「下一页」控件句柄行。
var reNextPageRef = regexp.MustCompile(`(?m)^.*下\s*一\s*页.*?\[ref=(e\d+)\].*$`)

// reCatTabRef 快照中分类页签句柄行（部门规章/地方政府规章）。
var reCatTabRef = regexp.MustCompile(`(?m)^.*"?(部门规章|地方政府规章)"?.*?\[ref=(e\d+)\].*$`)

// govListRawEntry evaluate 回传的页面条目（未过滤）。
type govListRawEntry struct {
	Title  string `json:"title"`
	URL    string `json:"url"`
	PubOrg string `json:"puborg"`
}

// ParseGovListExtract 解析 evaluate 回传的 JSON 数组（容忍包裹噪声），
// 并按硬校验过滤（https + www.gov.cn + content_<id>）。页面内容仅作数据。
func ParseGovListExtract(raw string) []GovListEntry {
	i, j := strings.Index(raw, "["), strings.LastIndex(raw, "]")
	if i < 0 || j <= i {
		return nil
	}
	var raws []govListRawEntry
	if err := json.Unmarshal([]byte(raw[i:j+1]), &raws); err != nil {
		return nil
	}
	var out []GovListEntry
	for _, r := range raws {
		if !govListEntryURLRe.MatchString(r.URL) || r.Title == "" {
			continue
		}
		out = append(out, GovListEntry{Title: r.Title, URL: r.URL, PubOrg: r.PubOrg})
	}
	return out
}

// clickConfirmed 走两段式（Propose→gate→Execute）执行一次官方 UI 点击。
// 人在环授权在任务级（用户显式启动浏览任务），每次点击仍过红线并留 pending 审计。
func clickConfirmed(ctx context.Context, b plugins.BrowserPlugin, pageID int, ref, label string) error {
	cand, err := b.ProposeClick(ctx, pageID, ref, label)
	if err != nil {
		return err
	}
	if !cand.Allowed {
		return fmt.Errorf("红线拦截(%s): %s", cand.Rule, cand.Reason)
	}
	if _, err := b.ExecuteAction(ctx, cand.ActionID); err != nil {
		return err
	}
	return nil
}

// RunGovListBrowse 执行规章库清单浏览：打开官方列表页 →（可选切分类页签）→
// 逐页抽取条目 → 官方「下一页」翻页（限速）→ 直到无下一页/达上限。
// 返回的清单字段与 OpenLaw openlaw-govcn -source gzk 的 list.json 直接兼容。
func RunGovListBrowse(ctx context.Context, b plugins.BrowserPlugin, opts GovListOptions) (GovListReport, error) {
	if b == nil || !b.Enabled() {
		return GovListReport{}, fmt.Errorf("浏览器辅助插件未启用：%s", plugins.BrowserOSGuidance)
	}
	if opts.Cat != "" && opts.Cat != "部门规章" && opts.Cat != "地方政府规章" {
		return GovListReport{}, fmt.Errorf("分类只能为 部门规章 或 地方政府规章（空 = 当前页签）")
	}
	if opts.MaxPages <= 0 {
		opts.MaxPages = govListDefaultPages
	}
	if opts.MaxPages > govListMaxPages {
		opts.MaxPages = govListMaxPages
	}
	if opts.Delay < govListMinDelay {
		opts.Delay = govListDefaultDelay
	}

	page, err := b.Open(ctx, GovListStartURL)
	if err != nil {
		return GovListReport{}, fmt.Errorf("打开规章库列表页失败: %w", err)
	}
	defer func() { _ = b.ClosePage(context.WithoutCancel(ctx), page.PageID) }()

	rep := GovListReport{StartURL: GovListStartURL, Cat: opts.Cat, StoppedAt: "done"}

	// 可选：切换到指定分类页签（官方 UI 点击，非构造请求）。
	if opts.Cat != "" {
		snap, err := b.Snapshot(ctx, page.PageID)
		if err != nil {
			return rep, err
		}
		if m := reCatTabRef.FindStringSubmatch(snap); m != nil {
			if err := clickConfirmed(ctx, b, page.PageID, m[2], m[1]); err != nil {
				return rep, fmt.Errorf("切换分类页签失败: %w", err)
			}
		}
	}

	seen := map[string]bool{}
	for pageno := 1; pageno <= opts.MaxPages; pageno++ {
		raw, err := b.Evaluate(ctx, page.PageID, govListExtractJS)
		if err != nil {
			return rep, fmt.Errorf("第 %d 页抽取失败: %w", pageno, err)
		}
		fresh := 0
		for _, e := range ParseGovListExtract(raw) {
			if seen[e.URL] {
				continue
			}
			seen[e.URL] = true
			e.Cat = opts.Cat
			rep.Entries = append(rep.Entries, e)
			fresh++
		}
		rep.Pages = pageno
		if fresh == 0 {
			rep.StoppedAt = "empty_page"
			return rep, nil
		}
		if pageno == opts.MaxPages {
			rep.StoppedAt = "max_pages"
			return rep, nil
		}
		// 官方「下一页」翻页（两段式点击）。
		snap, err := b.Snapshot(ctx, page.PageID)
		if err != nil {
			return rep, fmt.Errorf("第 %d 页快照失败: %w", pageno, err)
		}
		m := reNextPageRef.FindStringSubmatch(snap)
		if m == nil {
			rep.StoppedAt = "no_next"
			return rep, nil
		}
		select {
		case <-time.After(opts.Delay):
		case <-ctx.Done():
			return rep, ctx.Err()
		}
		if err := clickConfirmed(ctx, b, page.PageID, m[1], "下一页"); err != nil {
			return rep, fmt.Errorf("翻页被拦截: %w", err)
		}
	}
	return rep, nil
}
