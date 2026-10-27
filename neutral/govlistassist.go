// govlistassist 国家规章库清单浏览（人在环代理浏览；合规画像见 OpenLaw
// docs/sources-govcn.md，浏览模式扩展见 docs/browseros-integration.md 第 7 节）。
//
// 背景：规章库（www.gov.cn/zhengce/xxgk/gjgzk/）的检索 API（sousuoht Athena）带
// RSA appKey 应用层鉴权、sousuo 检索域 robots Disallow——清单不得由程序调用
// 检索接口发现。合规替代：智能体驱动用户自己的真实浏览器（BrowserOS neo），
// 只在主站 robots 允许的公开列表页上人工式翻页浏览，读 DOM 收集条目
// （与用户自己浏览并点「导出清单」等价），产物 list.json 交 OpenLaw
// openlaw-govcn -source gzk 走详情页合规抓取通道。
//
// 真机核实形态（neo 0.0.80，2026-10-26）：
//   - 默认列表即「部门规章 + 地方政府规章」混合清单（共约 1000 页），无独立分类页签；
//     分类按发布机关逐条推断（puborg 含「人民政府」→ 地方政府规章，否则 → 部门规章，
//     与 OpenLaw govgzResolveLevel 同规则）；
//   - 分页 = 对官方分页按钮 button.turnPage.next-page 的 DOM 点击（页面自身 JS
//     取下一页）——等价人工点击，绝不构造检索请求、绝不访问 sousuo/sousuoht；
//   - 地方政府规章详情页直链地方政府网站（同 .gov.cn 域名族）：清单如实记录，
//     由 OpenLaw 侧主机策略决定抓取范围（报告可见），清单层不擅自丢弃。
//
// 硬护栏（代码级，不是提示词）：
//   - 只允许打开固定起点 URL（规章库官方列表页）；
//   - evaluate 脚本只读 DOM 或对官方分页按钮 click()（无 fetch/XHR/导航构造）；
//   - 礼貌限速：翻页间隔下限 2s（默认 2.5s），单会话页数上限 200；
//   - 条目 URL 硬校验（https + .gov.cn 域名族 + .htm/.html/.shtml），噪声链接丢弃；
//   - SPA 就绪重试只重读 DOM（零额外网络请求）；
//   - 全程审计由调用方（main.go）落 BROWSER_GOVLIST 事件。
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

// govListEntryURLRe 条目详情页硬校验：https + .gov.cn 域名族（主站与地方政府
// 网站均在内）+ .htm/.html/.shtml 结尾；导航/资源链接一律过滤。
var govListEntryURLRe = regexp.MustCompile(`^https://(?:[A-Za-z0-9-]+\.)*gov\.cn/[^\s"']+\.s?html?$`)

// 限速与规模护栏。
const (
	govListDefaultPages = 50
	govListMaxPages     = 200
	govListMinDelay     = 2 * time.Second
	govListDefaultDelay = 2500 * time.Millisecond
	govListReadyTries   = 5 // SPA 就绪重试（只重读 DOM）
	govListReadyPause   = 1500 * time.Millisecond
	govListReloadTries  = 2 // 空页时官方跳转重刷重试（等价人工刷新）
	govListEmptyStop    = 3 // 连续真空页达到此数才停止（防偶发空壳误停）
	govListJumpTries    = 6 // 跳转页码核验重试
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
	Cat       string        // 分类过滤器（部门规章/地方政府规章；空 = 混合清单全收）
	MaxPages  int           // 单会话翻页上限（默认 50，硬上限 200）
	Delay     time.Duration // 翻页间隔（默认 2.5s，下限 2s）
	StartPage int           // 起始页（默认 1；>1 用官方「跳转」控件到达，跨会话续接用）
}

// GovListReport 浏览任务结果（OutPath 由调用方写盘后填充）。
type GovListReport struct {
	StartURL  string         `json:"start_url"`
	Cat       string         `json:"cat"`
	StartPage int            `json:"start_page"`
	Pages     int            `json:"pages"`
	Entries   []GovListEntry `json:"entries"`
	StoppedAt string         `json:"stopped_at"` // done / max_pages / empty_page / no_next
	OutPath   string         `json:"out_path,omitempty"`
}

// govListExtractJS 页面内数据抽取脚本：从官方结果 DOM 读取条目（只读，无网络）。
// 结构来自站点渲染模板（div.depart_m > a[docurl] > h5 + p.de_part span）。
// 注意：neo evaluate 的 code 是**函数体**（须用 return 取值），不是表达式——
// IIFE 形态会返回 undefined（真机实测 0.0.80）。
const govListExtractJS = `
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
return JSON.stringify(out);`

// govListNextJS 官方分页按钮点击（等价人工点击「下一页」；页面自身 JS 取数，
// 不构造任何请求）。按钮不存在或已禁用 → 返回 no_next。
const govListNextJS = `
const b = document.querySelector('button.turnPage.next-page');
if (!b || b.disabled) return 'no_next';
b.click();
return 'clicked';`

// govListJumpJSFmt 官方「跳转」控件（输入页码 + 点「确定」，页面自身 JS 生效）
// ——等价人工输入页码跳页，用于跨会话续接；%d 只接受编排层生成的整数页码。
const govListJumpJSFmt = `
const inp = document.querySelector('input.jumpText');
if (!inp) return 'no_input';
inp.value = '%d';
inp.dispatchEvent(new Event('input', {bubbles: true}));
const btns = Array.from(document.querySelectorAll('button.jump-button'));
const ok = btns.find(b => (b.textContent || '').includes('确定'));
if (!ok) return 'no_btn';
ok.click();
return 'jumped';`

// govListSelPageJS 读取官方分页控件当前选中页码（跳转结果核验）。
const govListSelPageJS = `
const sel = document.querySelector('li.sel-page');
return sel ? sel.textContent.trim() : '';`

// govListRawEntry evaluate 回传的页面条目（未过滤）。
type govListRawEntry struct {
	Title  string `json:"title"`
	URL    string `json:"url"`
	PubOrg string `json:"puborg"`
}

// inferGovListCat 按发布机关推断分类（与 OpenLaw govgzResolveLevel 同规则）：
// puborg 含「人民政府」→ 地方政府规章；非空其他机关 → 部门规章；空 → 空（人工复核）。
func inferGovListCat(puborg string) string {
	if puborg == "" {
		return ""
	}
	if strings.Contains(puborg, "人民政府") {
		return "地方政府规章"
	}
	return "部门规章"
}

// ParseGovListExtract 解析 evaluate 回传的 JSON 数组（容忍包裹噪声），
// 按硬校验过滤（https + .gov.cn 域名族 + .htm/.html/.shtml），并逐条推断 cat。
// 页面内容仅作数据解析，绝不当指令解释。
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
		out = append(out, GovListEntry{
			Title: r.Title, URL: r.URL, PubOrg: r.PubOrg, Cat: inferGovListCat(r.PubOrg),
		})
	}
	return out
}

// extractReady 抽取当前页条目。SPA 异步渲染/翻页生效可能滞后——重试只重读
// DOM（零网络请求，不构成额外访问），最多 govListReadyTries 次；仍为空则用
// 官方「跳转」控件重刷当前页（等价人工刷新页面，页面自身 JS 重新取数）再试，
// 最多 govListReloadTries 次（真机实测：偶发空壳渲染持续 >6s，纯 DOM 重读
// 无效，重刷可恢复）。返回的条目可能全部已见（翻页尚未生效时）。
func extractReady(ctx context.Context, b plugins.BrowserPlugin, pageID, curPage int, seen map[string]bool,
	clock *time.Time, pauseCh chan time.Duration, pauseDur time.Duration) ([]GovListEntry, error) {
	var last []GovListEntry
	for reload := 0; reload <= govListReloadTries; reload++ {
		if reload > 0 {
			if err := jumpToPage(ctx, b, pageID, curPage); err != nil {
				return nil, err
			}
		}
		for tries := 0; tries < govListReadyTries; tries++ {
			if tries > 0 {
				if pauseCh != nil {
					select {
					case pauseCh <- govListReadyPause:
					case <-ctx.Done():
						return nil, ctx.Err()
					}
					if clock != nil {
						*clock = clock.Add(pauseDur)
					}
				} else {
					select {
					case <-time.After(govListReadyPause):
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
			}
			raw, err := b.Evaluate(ctx, pageID, govListExtractJS)
			if err != nil {
				return nil, err
			}
			last = ParseGovListExtract(raw)
			if len(last) == 0 {
				continue // 空页或渲染未就绪，重读确认
			}
			fresh := false
			for _, e := range last {
				if !seen[e.URL] {
					fresh = true
					break
				}
			}
			if fresh {
				return last, nil // 出现新条目 = 新页已就绪
			}
		}
		if len(last) > 0 {
			return last, nil // 有条目但全已见：交主循环判重，不再重刷
		}
	}
	return last, nil
}

// clickNextPage 点击官方「下一页」分页按钮（DOM click，等价人工点击）。
func clickNextPage(ctx context.Context, b plugins.BrowserPlugin, pageID int) (bool, error) {
	raw, err := b.Evaluate(ctx, pageID, govListNextJS)
	if err != nil {
		return false, err
	}
	return strings.Contains(raw, "clicked"), nil
}

// jumpToPage 用官方「跳转」控件到达指定页（等价人工输入页码 + 点确定），并以
// 分页控件选中页码核验生效（SPA 生效滞后重试 ≤govListReadyTries 次）。
// 核验不过（页码越界/控件改版）直接报错——绝不在错误页上采集。
func jumpToPage(ctx context.Context, b plugins.BrowserPlugin, pageID, page int) error {
	raw, err := b.Evaluate(ctx, pageID, fmt.Sprintf(govListJumpJSFmt, page))
	if err != nil {
		return err
	}
	if !strings.Contains(raw, "jumped") {
		if len(raw) > 80 {
			raw = raw[:80]
		}
		return fmt.Errorf("官方跳转控件不可用: %s", raw)
	}
	want := fmt.Sprintf("%d", page)
	for tries := 0; tries < govListJumpTries; tries++ {
		if tries > 0 {
			select {
			case <-time.After(govListReadyPause):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		raw, err := b.Evaluate(ctx, pageID, govListSelPageJS)
		if err != nil {
			return err
		}
		if strings.TrimSpace(raw) == want {
			return nil
		}
	}
	return fmt.Errorf("跳转后页码核验失败（期望第 %d 页）", page)
}

// RunGovListBrowse 执行规章库清单浏览：打开官方列表页（混合清单）→ 逐页抽取
// 条目（cat 按发布机关逐条推断）→ 官方分页按钮翻页（限速）→ 直到无下一页/达上限。
// 返回的清单字段与 OpenLaw openlaw-govcn -source gzk 的 list.json 直接兼容。
//
// 时钟注入（clock/pauseCh/pauseDur）仅供单测用虚拟时钟加速；生产路径恒为
// nil（time.Sleep / time.After 真实限速）。
func RunGovListBrowse(ctx context.Context, b plugins.BrowserPlugin, opts GovListOptions) (GovListReport, error) {
	return runGovListBrowse(ctx, b, opts, nil, nil, 0)
}

func runGovListBrowse(ctx context.Context, b plugins.BrowserPlugin, opts GovListOptions,
	clock *time.Time, pauseCh chan time.Duration, pauseDur time.Duration) (GovListReport, error) {
	sleep := func(d time.Duration) {
		if pauseCh != nil {
			select {
			case pauseCh <- d:
			case <-ctx.Done():
			}
			if clock != nil {
				*clock = clock.Add(pauseDur)
			}
			return
		}
		time.Sleep(d)
	}
	if b == nil || !b.Enabled() {
		return GovListReport{}, fmt.Errorf("浏览器辅助插件未启用：%s", plugins.BrowserOSGuidance)
	}
	if opts.Cat != "" && opts.Cat != "部门规章" && opts.Cat != "地方政府规章" {
		return GovListReport{}, fmt.Errorf("分类过滤只能为 部门规章 或 地方政府规章（空 = 混合清单全收）")
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
	if opts.StartPage < 1 {
		opts.StartPage = 1
	}

	page, err := b.Open(ctx, GovListStartURL)
	if err != nil {
		return GovListReport{}, fmt.Errorf("打开规章库列表页失败: %w", err)
	}
	defer func() { _ = b.ClosePage(context.WithoutCancel(ctx), page.PageID) }()

	rep := GovListReport{StartURL: GovListStartURL, Cat: opts.Cat, StartPage: opts.StartPage, StoppedAt: "done"}
	if opts.StartPage > 1 {
		if err := jumpToPage(ctx, b, page.PageID, opts.StartPage); err != nil {
			return rep, fmt.Errorf("跳转到第 %d 页失败: %w", opts.StartPage, err)
		}
	}
	seen := map[string]bool{}
	emptyStreak := 0

	for pageno := 1; pageno <= opts.MaxPages; pageno++ {
		curPage := opts.StartPage + pageno - 1
		entries, err := extractReady(ctx, b, page.PageID, curPage, seen, clock, pauseCh, pauseDur)
		if err != nil {
			return rep, fmt.Errorf("第 %d 页抽取失败: %w", pageno, err)
		}
		if len(entries) == 0 {
			// DOM 重读 + 官方跳转重刷后仍空。连续 govListEmptyStop 页才认定真空页
			// 停止（真机实测偶发空壳渲染会连续误报）；未到阈值则官方翻页继续，
			// 疑似假空页跳过的条目由合并工具的缺段检测补采。
			emptyStreak++
			rep.Pages = pageno
			if emptyStreak >= govListEmptyStop {
				rep.StoppedAt = "empty_page"
				return rep, nil
			}
		} else {
			emptyStreak = 0
			for _, e := range entries {
				if seen[e.URL] {
					continue
				}
				seen[e.URL] = true // 如实记录（含被过滤分类的），过滤只影响输出
				if opts.Cat != "" && e.Cat != opts.Cat {
					continue
				}
				rep.Entries = append(rep.Entries, e)
			}
			rep.Pages = pageno
		}
		if pageno == opts.MaxPages {
			rep.StoppedAt = "max_pages"
			return rep, nil
		}
		if pauseCh == nil {
			select {
			case <-time.After(opts.Delay):
			case <-ctx.Done():
				return rep, ctx.Err()
			}
		} else {
			// 翻页间隔 = 对官方分页按钮的真实点击间隔（虚拟时钟仅跳过墙钟等待，
			// 绝不跳过点击本身）
			sleep(opts.Delay)
			if ctx.Err() != nil {
				return rep, ctx.Err()
			}
		}
		ok, err := clickNextPage(ctx, b, page.PageID)
		if err != nil {
			return rep, fmt.Errorf("翻页失败: %w", err)
		}
		if !ok {
			rep.StoppedAt = "no_next"
			return rep, nil
		}
	}
	return rep, nil
}
