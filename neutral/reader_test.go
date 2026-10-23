package neutral

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/extract"
	"github.com/futuremeng/OpenCourt/core/verify"
)

// fakeModel 测试用模型适配器：返回预设输出。
type fakeModel struct {
	out     string
	enabled bool
	err     error
}

func (f *fakeModel) Name() string  { return "fake" }
func (f *fakeModel) Enabled() bool { return f.enabled }
func (f *fakeModel) Embed(_ context.Context, _ string) ([]float64, error) {
	return nil, errors.New("unsupported")
}
func (f *fakeModel) Generate(_ context.Context, _ string) (string, error) {
	return f.out, f.err
}

func testDoc() *verify.DocumentText {
	return &verify.DocumentText{
		File: "judgment.txt",
		Pages: map[int]string{
			1: "2023年5月10日，原告向被告支付定金50000元。合同约定2023年6月1日交货。",
			2: "被告未按期交货，原告要求其承担违约责任。",
		},
	}
}

func TestReaderRejectsDisabledModel(t *testing.T) {
	_, err := RunReader(context.Background(), "DOC-1", testDoc(), &fakeModel{enabled: false})
	if err == nil {
		t.Fatal("默认离线：模型未启用时阅卷必须失败")
	}
}

func TestReaderVerifiesCandidates(t *testing.T) {
	out := `一些前言
` + "```json" + `
[
  {"description":"原告已支付定金","snippet":"原告向被告支付定金50000元","page_num":1,"from_opponent":false},
  {"description":"捏造的事实","snippet":"被告曾于2020年向原告借款100万元","page_num":1,"from_opponent":false},
  {"description":"对方声称货物已备好","snippet":"被告未按期交货","page_num":2,"from_opponent":true}
]
` + "```" + `
`
	report, err := RunReader(context.Background(), "DOC-1", testDoc(), &fakeModel{out: out, enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Accepted) != 2 {
		t.Fatalf("应接受 2 条已定位候选，实际 %d", len(report.Accepted))
	}
	if len(report.Rejected) != 1 {
		t.Fatalf("幻觉候选（回原文失败）应拒绝 1 条，实际 %d", len(report.Rejected))
	}
	if report.Rejected[0].Description != "捏造的事实" {
		t.Errorf("被拒绝的应是捏造候选: %+v", report.Rejected[0])
	}
	for _, f := range report.Accepted {
		switch f.State {
		case domain.StateSingleSource, domain.StateOpponentClaim:
		default:
			t.Errorf("入库事实状态应为核验器判定的弱态，实际: %s", f.State)
		}
		if f.Citation == nil || !verify.Locate(*f.Citation, map[string]*verify.DocumentText{"judgment.txt": testDoc()}) {
			t.Errorf("入库事实必须可回原文定位: %+v", f)
		}
	}
	// 确定性要素：两条日期 + 一个金额（第 1 页）。
	var times, amounts int
	for _, e := range report.Elements {
		if !e.Located {
			t.Fatal("要素必须已定位")
		}
		if e.Type == extract.ElementTime {
			times++
		}
		if e.Type == extract.ElementAmount {
			amounts++
		}
	}
	if times < 2 || amounts < 1 {
		t.Errorf("要素抽取不足: times=%d amounts=%d", times, amounts)
	}
}

func TestParseCandidatesGarbageFails(t *testing.T) {
	if _, err := ParseCandidates("模型没有输出 JSON"); err == nil {
		t.Fatal("无 JSON 数组必须报错")
	}
}

func TestBuildTimelineAnchorsAndSorts(t *testing.T) {
	doc := testDoc()
	facts := []domain.Fact{
		{FactID: "F3", Description: "无日期事实", State: domain.StateSingleSource},
		{FactID: "F1", Description: "付款", State: domain.StateSingleSource,
			Citation: &domain.Citation{SourceFile: doc.File, PageNum: 1, Snippet: "原告向被告支付定金50000元"}},
		{FactID: "F2", Description: "交货期", State: domain.StateSingleSource,
			Citation: &domain.Citation{SourceFile: doc.File, PageNum: 1, Snippet: "2023年6月1日交货"}},
	}
	elements := []extract.Element{
		{Type: extract.ElementTime, Value: "2023年5月10日", SourceFile: doc.File, PageNum: 1, Located: true},
	}
	tl := BuildTimeline(facts, elements)
	if len(tl) != 3 {
		t.Fatalf("时间线长度错误: %d", len(tl))
	}
	// 有锚点的在前，无锚点的排末尾。
	if tl[2].Date != "" {
		t.Errorf("无锚点事实应排末尾: %+v", tl[2])
	}
	if tl[0].Date != "2023年5月10日" {
		t.Errorf("锚定日期错误: %q", tl[0].Date)
	}
}

func TestParseDate(t *testing.T) {
	if _, ok := parseDate("2023-05-10"); !ok {
		t.Error("ISO 日期解析失败")
	}
	if _, ok := parseDate("2023年5月10日"); !ok {
		t.Error("中文日期解析失败")
	}
	if _, ok := parseDate("去年"); ok {
		t.Error("模糊表述不得被当作日期")
	}
	if _, ok := parseDate(strings.Repeat("x", 10)); ok {
		t.Error("乱码不得被当作日期")
	}
}
