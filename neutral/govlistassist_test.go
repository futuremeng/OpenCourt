package neutral

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/futuremeng/OpenCourt/plugins"
)

// govListStub 多页模拟浏览器：抽取脚本按当前页返回条目；翻页脚本按序返回
// clicked/no_next（clicked 才推进页码）。
type govListStub struct {
	pages    []string // 每页抽取回传（可含噪声）
	nextSeq  []string // 每次翻页脚本回传（clicked / no_next）
	page     int      // 当前页（0 基）
	nextCall int
	jumps    int // 官方跳转控件点击次数
	opens    []string
	execs    int // act 执行次数（新编排不应再有 act 翻页）
	disabled bool
	sawCode  string
}

func (s *govListStub) Name() string  { return "govstub" }
func (s *govListStub) Enabled() bool { return !s.disabled }
func (s *govListStub) Open(ctx context.Context, url string) (plugins.BrowserPage, error) {
	s.opens = append(s.opens, url)
	return plugins.BrowserPage{PageID: 7, URL: url}, nil
}
func (s *govListStub) ReadPage(ctx context.Context, pageID int) (plugins.BrowserPageContent, error) {
	return plugins.BrowserPageContent{}, fmt.Errorf("unused")
}
func (s *govListStub) SearchPage(ctx context.Context, pageID int, q string) ([]string, error) {
	return nil, nil
}
func (s *govListStub) ClosePage(ctx context.Context, pageID int) error { return nil }
func (s *govListStub) Snapshot(ctx context.Context, pageID int) (string, error) {
	return "", fmt.Errorf("unused")
}
func (s *govListStub) Evaluate(ctx context.Context, pageID int, code string) (string, error) {
	s.sawCode = code
	if isNextScript(code) {
		if s.nextCall < len(s.nextSeq) {
			out := s.nextSeq[s.nextCall]
			s.nextCall++
			if out == "clicked" {
				s.page++ // 翻页生效，下一页可读
			}
			return out, nil
		}
		return "no_next", nil
	}
	if strings.Contains(code, "jumpText") {
		i := strings.Index(code, "inp.value = '")
		if i < 0 {
			return "no_input", nil
		}
		rest := code[i+13:]
		j := strings.Index(rest, "'")
		if j < 0 {
			return "no_input", nil
		}
		n := 0
		for _, c := range rest[:j] {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		s.jumps++
		s.page = n - 1 // 跳转生效（0 基）
		return "jumped", nil
	}
	if strings.Contains(code, "sel-page") {
		return fmt.Sprintf("%d", s.page+1), nil
	}
	if s.page < len(s.pages) {
		return s.pages[s.page], nil
	}
	return "[]", nil
}
func (s *govListStub) ProposeFill(ctx context.Context, pageID int, f []plugins.FormFieldSpec) ([]plugins.ActionCandidate, error) {
	return nil, fmt.Errorf("unused")
}
func (s *govListStub) ProposeClick(ctx context.Context, pageID int, ref, label string) (plugins.ActionCandidate, error) {
	return plugins.ActionCandidate{}, fmt.Errorf("unused")
}
func (s *govListStub) ExecuteAction(ctx context.Context, actionID string) (plugins.ActionReceipt, error) {
	s.execs++
	return plugins.ActionReceipt{ActionID: actionID}, nil
}
func (s *govListStub) HandToHuman(ctx context.Context, pageID int, k, r, h string) (plugins.HumanHelpStatus, error) {
	return plugins.HumanHelpStatus{}, fmt.Errorf("unused")
}
func (s *govListStub) AwaitHuman(ctx context.Context) (plugins.HumanHelpStatus, error) {
	return plugins.HumanHelpStatus{}, fmt.Errorf("unused")
}

func isNextScript(code string) bool {
	return strings.Contains(code, "next-page") || strings.Contains(code, "turnPage")
}

func TestParseGovListExtract(t *testing.T) {
	raw := `Tip: blah
[{"title":"甲办法","url":"https://www.gov.cn/zhengce/202610/content_1.htm","puborg":"国家统计局"},
 {"title":"乙办法","url":"https://www.changdu.gov.cn/cdrmzf/202610/abc.shtml","puborg":"昌都市人民政府"},
 {"title":"丙规定","url":"https://evil.example/content_2.htm","puborg":"X"},
 {"title":"","url":"https://www.gov.cn/a/content_3.htm","puborg":"Y"},
 {"title":"丁通告","url":"https://www.gov.cn/a/content_4","puborg":"Y"}]`
	got := ParseGovListExtract(raw)
	if len(got) != 2 {
		t.Fatalf("过滤错误: %+v", got)
	}
	if got[0].Cat != "部门规章" || got[0].PubOrg != "国家统计局" {
		t.Errorf("cat 推断错误: %+v", got[0])
	}
	if got[1].Cat != "地方政府规章" {
		t.Errorf("cat 推断错误: %+v", got[1])
	}
	if ParseGovListExtract("not json") != nil {
		t.Error("非 JSON 应返回空")
	}
	if got := ParseGovListExtract(`[{"title":"戊","url":"https://www.gov.cn/x/content_5.htm","puborg":""}]`); len(got) != 1 || got[0].Cat != "" {
		t.Errorf("puborg 缺失应留空 cat 交人工: %+v", got)
	}
}

func TestRunGovListBrowseFlow(t *testing.T) {
	page1 := `[{"title":"甲办法","url":"https://www.gov.cn/zhengce/a/content_1.htm","puborg":"国家统计局"}]`
	page2 := `[{"title":"甲办法","url":"https://www.gov.cn/zhengce/a/content_1.htm","puborg":"国家统计局"},
	           {"title":"乙办法","url":"https://www.changdu.gov.cn/x/abc.shtml","puborg":"昌都市人民政府"}]`
	s := &govListStub{
		pages:   []string{page1, page2},
		nextSeq: []string{"clicked", "clicked"}, // 第 2 次翻页后 page=2 越界 → 抽取空 → 停
	}
	rep, err := RunGovListBrowse(context.Background(), s, GovListOptions{Delay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if rep.StoppedAt != "empty_page" || rep.Pages != 3 {
		t.Errorf("应在空页停: %+v", rep)
	}
	if len(rep.Entries) != 2 {
		t.Fatalf("应按 URL 去重得 2 条: %d", len(rep.Entries))
	}
	if rep.Entries[0].Cat != "部门规章" || rep.Entries[1].Cat != "地方政府规章" {
		t.Errorf("cat 逐条推断错误: %+v", rep.Entries)
	}
	if s.opens[0] != GovListStartURL {
		t.Errorf("只允许打开官方起点: %v", s.opens)
	}
	if s.execs != 0 {
		t.Errorf("翻页不得走 act 写通道（应纯 DOM click 官方按钮）: %d", s.execs)
	}
}

func TestRunGovListBrowseStartPage(t *testing.T) {
	pages := make([]string, 6)
	for i := range pages {
		pages[i] = fmt.Sprintf(`[{"title":"t%d","url":"https://www.gov.cn/a/content_%d.htm","puborg":"教育部"}]`, i, i)
	}
	s := &govListStub{pages: pages, nextSeq: []string{"clicked"}}
	rep, err := RunGovListBrowse(context.Background(), s, GovListOptions{
		StartPage: 3, MaxPages: 2, Delay: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.jumps != 1 {
		t.Errorf("必须经官方跳转控件到达起始页: jumps=%d", s.jumps)
	}
	if len(rep.Entries) != 2 || rep.Entries[0].Title != "t2" || rep.Entries[1].Title != "t3" {
		t.Errorf("应从第 3 页开始采集: %+v", rep.Entries)
	}
	if rep.StartPage != 3 {
		t.Errorf("报告应记录起始页: %+v", rep)
	}
	// 页码核验不过（stub 恒核验通过，此处验证越界跳转后 extract 空即停）
	s2 := &govListStub{pages: pages, nextSeq: []string{"clicked"}}
	rep2, err := RunGovListBrowse(context.Background(), s2, GovListOptions{
		StartPage: 99, MaxPages: 2, Delay: time.Millisecond,
	})
	if err != nil || rep2.StoppedAt != "empty_page" || len(rep2.Entries) != 0 {
		t.Errorf("越界起始页应空页即停: %+v %v", rep2, err)
	}
}

func TestRunGovListBrowseCatFilter(t *testing.T) {
	page1 := `[{"title":"甲","url":"https://www.gov.cn/a/content_1.htm","puborg":"教育部"},
	           {"title":"乙","url":"https://www.beijing.gov.cn/x/a.html","puborg":"北京市人民政府"}]`
	s := &govListStub{pages: []string{page1}, nextSeq: []string{"no_next"}}
	rep, err := RunGovListBrowse(context.Background(), s, GovListOptions{
		Cat: "部门规章", Delay: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.StoppedAt != "no_next" || len(rep.Entries) != 1 || rep.Entries[0].Title != "甲" {
		t.Errorf("分类过滤错误: %+v", rep)
	}
}

func TestRunGovListBrowseGuardrails(t *testing.T) {
	if _, err := RunGovListBrowse(context.Background(), nil, GovListOptions{}); err == nil {
		t.Error("插件缺失必须报错")
	}
	s := &govListStub{disabled: true}
	if _, err := RunGovListBrowse(context.Background(), s, GovListOptions{}); err == nil {
		t.Error("插件禁用必须报错（绝不降级）")
	}
	s2 := &govListStub{}
	if _, err := RunGovListBrowse(context.Background(), s2, GovListOptions{Cat: "规范性文件"}); err == nil {
		t.Error("未知分类必须拒绝")
	}
	// 空清单页：SPA 就绪重试（只重读 DOM）后即停，绝不翻页点击。
	s3 := &govListStub{pages: []string{"[]"}}
	rep, err := RunGovListBrowse(context.Background(), s3, GovListOptions{MaxPages: 9999, Delay: time.Millisecond})
	if err != nil || rep.StoppedAt != "empty_page" || rep.Pages != 1 || s3.nextCall != 0 {
		t.Errorf("空页应即停不翻页: %+v %v", rep, err)
	}
	// MaxPages 钳制：200 页上限。用虚拟时钟加速墙钟等待，但断言每一次翻页
	// 间隔都 >= 2s（真实点击间隔语义不因加速而失真）。
	pages := make([]string, 0, 250)
	for i := 0; i < 250; i++ {
		pages = append(pages, fmt.Sprintf(`[{"title":"t%d","url":"https://www.gov.cn/a/content_%d.htm","puborg":"教育部"}]`, i, i))
	}
	next := make([]string, 0, 250)
	for i := 0; i < 250; i++ {
		next = append(next, "clicked")
	}
	s4 := &govListStub{pages: pages, nextSeq: next}
	pauseCh := make(chan time.Duration, 400)
	clock := time.Unix(0, 0)
	rep, err = runGovListBrowse(context.Background(), s4, GovListOptions{MaxPages: 9999},
		&clock, pauseCh, 3*time.Second)
	if err != nil || rep.StoppedAt != "max_pages" || rep.Pages != govListMaxPages || len(rep.Entries) != govListMaxPages {
		t.Errorf("MaxPages 应钳制到 %d: %+v %v", govListMaxPages, rep, err)
	}
	if s4.nextCall != govListMaxPages-1 {
		t.Errorf("翻页点击次数应 = 页数-1: %d", s4.nextCall)
	}
	if clock.Sub(time.Unix(0, 0)) < time.Duration(govListMaxPages-1)*govListMinDelay {
		t.Errorf("翻页间隔必须 >= %v（真实点击间隔）", govListMinDelay)
	}
}
