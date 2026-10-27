package neutral

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/futuremeng/OpenCourt/plugins"
)

// govListStub 多页模拟浏览器：Evaluate 按当前页返回条目，点击「下一页」翻页。
type govListStub struct {
	pages    []string // 每页 evaluate 回传（含噪声也仿）
	snapNext string   // 快照：含下一页句柄（空 = 无下一页）
	snapTab  string   // 快照：含分类页签句柄
	page     int      // 当前页（0 基）
	clicks   []string // 已执行点击标签
	opens    []string
	execs    int
	disabled bool
	maxSeen  int
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
	if s.page < len(s.pages) { // 还有下一页可读时才给出「下一页」句柄
		return s.snapTab + "\n" + s.snapNext, nil
	}
	return s.snapTab, nil
}
func (s *govListStub) Evaluate(ctx context.Context, pageID int, code string) (string, error) {
	s.sawCode = code
	if s.page >= len(s.pages) {
		return "[]", nil
	}
	out := s.pages[s.page]
	s.page++
	return out, nil
}
func (s *govListStub) ProposeFill(ctx context.Context, pageID int, f []plugins.FormFieldSpec) ([]plugins.ActionCandidate, error) {
	return nil, fmt.Errorf("unused")
}
func (s *govListStub) ProposeClick(ctx context.Context, pageID int, ref, label string) (plugins.ActionCandidate, error) {
	return plugins.ActionCandidate{ActionID: "A" + fmt.Sprint(len(s.clicks)), Kind: "click", Allowed: true, Ref: ref, Label: label}, nil
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

func TestParseGovListExtract(t *testing.T) {
	raw := `Tip: blah
[{"title":"甲办法","url":"https://www.gov.cn/zhengce/zhengceku/202401/content_1.htm","puborg":"教育部"},
 {"title":"乙规定","url":"https://evil.example/content_2.htm","puborg":"X"},
 {"title":"","url":"https://www.gov.cn/a/content_3.htm","puborg":"Y"},
 {"title":"丙决定","url":"http://www.gov.cn/a/content_4.htm","puborg":"Y"}]`
	got := ParseGovListExtract(raw)
	if len(got) != 1 || got[0].Title != "甲办法" || got[0].PubOrg != "教育部" {
		t.Fatalf("过滤/解析错误: %+v", got)
	}
	if ParseGovListExtract("not json") != nil {
		t.Error("非 JSON 应返回空")
	}
}

func TestRunGovListBrowseFlow(t *testing.T) {
	page1 := `[{"title":"甲办法","url":"https://www.gov.cn/zhengce/a/content_1.htm","puborg":"教育部"}]`
	page2 := `[{"title":"甲办法","url":"https://www.gov.cn/zhengce/a/content_1.htm","puborg":"教育部"},
	           {"title":"乙规定","url":"https://www.gov.cn/gongbao/b/content_2.html","puborg":"北京市人民政府"}]`
	s := &govListStub{
		pages:    []string{page1, page2},
		snapNext: `- link "下一页" [ref=e99]`,
	}
	rep, err := RunGovListBrowse(context.Background(), s, GovListOptions{
		Cat: "部门规章", Delay: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.StoppedAt != "no_next" || rep.Pages != 2 {
		t.Errorf("应在无下一页处停: %+v", rep)
	}
	if len(rep.Entries) != 2 {
		t.Fatalf("应按 URL 去重得 2 条: %d", len(rep.Entries))
	}
	if rep.Entries[0].Cat != "部门规章" {
		t.Errorf("cat 标注错误: %+v", rep.Entries[0])
	}
	if s.opens[0] != GovListStartURL {
		t.Errorf("只允许打开官方起点: %v", s.opens)
	}
	if s.execs != 1 { // 仅一次翻页点击（两段式）
		t.Errorf("翻页点击次数错误: %d", s.execs)
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
	// MaxPages 钳制 + 空页即停 + delay 下限（默认值路径不 sleep 2s 即证明钳制生效）。
	s3 := &govListStub{pages: []string{"[]"}, snapNext: `- link "下一页" [ref=e1]`}
	start := time.Now()
	rep, err := RunGovListBrowse(context.Background(), s3, GovListOptions{MaxPages: 9999, Delay: time.Millisecond})
	if err != nil || rep.StoppedAt != "empty_page" || time.Since(start) > time.Second {
		t.Errorf("空页应即停且不等待: %+v %v", rep, err)
	}
}
