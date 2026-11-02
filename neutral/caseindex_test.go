package neutral

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futuremeng/OpenCourt/core/storage"
	"github.com/futuremeng/OpenCourt/plugins"
)

// fakeEmbed 确定性向量适配器：按关键词给二维向量——向量结果完全可预期。
type fakeEmbed struct {
	enabled bool
	err     error
	calls   int
}

func (f *fakeEmbed) Name() string  { return "fake-embed" }
func (f *fakeEmbed) Enabled() bool { return f.enabled }
func (f *fakeEmbed) Generate(context.Context, string) (string, error) {
	return "", plugins.ErrNotSupported
}
func (f *fakeEmbed) Embed(_ context.Context, text string) ([]float64, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	switch {
	case strings.Contains(text, "违约"):
		return []float64{1, 0}, nil
	case strings.Contains(text, "送达"):
		return []float64{0, 1}, nil
	default:
		return []float64{1, 1}, nil
	}
}

func indexTestStore(t *testing.T) *storage.CaseScope {
	t.Helper()
	st, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st.WithCase("CASE-DEFAULT")
}

// 建索引：缺页嵌入入库、text_hash 未变跳过、改文重嵌。
func TestIndexCasePages(t *testing.T) {
	sc := indexTestStore(t)
	fe := &fakeEmbed{enabled: true}
	pages := []PageText{
		{DocID: "d1", Page: 1, Text: "违约金条款"},
		{DocID: "d1", Page: 2, Text: "送达地址确认书"},
	}
	indexed, skipped, err := IndexCasePages(context.Background(), fe, "m1", pages, sc)
	if err != nil {
		t.Fatal(err)
	}
	if indexed != 2 || skipped != 0 || fe.calls != 2 {
		t.Errorf("首建应嵌 2 页: indexed=%d skipped=%d calls=%d", indexed, skipped, fe.calls)
	}
	rows, err := sc.ListEmbeddings("m1")
	if err != nil || len(rows) != 2 {
		t.Fatalf("应入库 2 行: %v %+v", err, rows)
	}

	indexed, skipped, err = IndexCasePages(context.Background(), fe, "m1", pages, sc)
	if err != nil || indexed != 0 || skipped != 2 {
		t.Errorf("未变应全跳过: indexed=%d skipped=%d err=%v", indexed, skipped, err)
	}

	pages[0].Text = "违约金条款（修订稿）"
	indexed, skipped, err = IndexCasePages(context.Background(), fe, "m1", pages, sc)
	if err != nil || indexed != 1 || skipped != 1 {
		t.Errorf("改文应只重嵌 1 页: indexed=%d skipped=%d err=%v", indexed, skipped, err)
	}
}

// 模型未启用 / 不支持 embedding → ErrSemanticUnavailable（降级信号，不是失败）。
func TestIndexCasePagesUnavailable(t *testing.T) {
	sc := indexTestStore(t)
	pages := []PageText{{DocID: "d1", Page: 1, Text: "x"}}
	if _, _, err := IndexCasePages(context.Background(), nil, "m1", pages, sc); !errors.Is(err, ErrSemanticUnavailable) {
		t.Errorf("nil 模型应报语义不可用: %v", err)
	}
	fe := &fakeEmbed{enabled: false}
	if _, _, err := IndexCasePages(context.Background(), fe, "m1", pages, sc); !errors.Is(err, ErrSemanticUnavailable) {
		t.Errorf("未启用应报语义不可用: %v", err)
	}
	fe2 := &fakeEmbed{enabled: true, err: plugins.ErrNotSupported}
	if _, _, err := IndexCasePages(context.Background(), fe2, "m1", pages, sc); !errors.Is(err, ErrSemanticUnavailable) {
		t.Errorf("不支持 embedding 应报语义不可用: %v", err)
	}
}

var hybridPages = []PageText{
	{DocID: "d1", Page: 1, Text: "合同约定违约金为合同金额的百分之二十"},
	{DocID: "d2", Page: 1, Text: "双方就违约责任另有口头约定"},
	{DocID: "d3", Page: 1, Text: "送达地址确认书一份"},
}

// 混合检索：关键词榜 + 向量榜 RRF 融合；命中来源如实标注；结果可复现。
func TestSearchCaseDocsHybrid(t *testing.T) {
	fe := &fakeEmbed{enabled: true}
	vecs := map[string][]float64{
		PageRef("d1", 1): {1, 0},
		PageRef("d2", 1): {1, 0}, // 无关键词命中但语义相近——只有向量榜能召回
		PageRef("d3", 1): {0, 1},
	}
	rep, err := SearchCaseDocs(context.Background(), fe, "m1", hybridPages, vecs, "违约金", 5)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Mode != SearchModeHybrid || rep.Note != "" {
		t.Errorf("双榜可用必须标 hybrid 且无降级注: %+v", rep)
	}
	if len(rep.Hits) != 2 {
		t.Fatalf("d1/d2 应命中，d3 不得混入: %+v", rep.Hits)
	}
	if rep.Hits[0].Ref != PageRef("d1", 1) {
		t.Errorf("双榜同现必须居首: %+v", rep.Hits)
	}
	if strings.Join(rep.Hits[0].Sources, "+") != "keyword+vector" {
		t.Errorf("来源标注错误: %v", rep.Hits[0].Sources)
	}
	if rep.Hits[1].Ref != PageRef("d2", 1) || strings.Join(rep.Hits[1].Sources, "+") != "vector" {
		t.Errorf("纯向量召回必须只标 vector: %+v", rep.Hits[1])
	}
	if !strings.Contains(rep.Hits[0].Snippet, "违约金") {
		t.Errorf("snippet 应含命中词: %q", rep.Hits[0].Snippet)
	}
	// 可复现（M5 验收）
	for i := 0; i < 10; i++ {
		again, _ := SearchCaseDocs(context.Background(), fe, "m1", hybridPages, vecs, "违约金", 5)
		for j := range again.Hits {
			if again.Hits[j].Ref != rep.Hits[j].Ref {
				t.Fatalf("结果不可复现: %+v vs %+v", again.Hits, rep.Hits)
			}
		}
	}
}

// 降级三态必须 Mode=keyword_only 并显式说明——绝不静默冒充向量化过。
func TestSearchCaseDocsDegradations(t *testing.T) {
	// 1) 模型未启用
	rep, err := SearchCaseDocs(context.Background(), &fakeEmbed{enabled: false}, "m1",
		hybridPages, nil, "违约金", 5)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Mode != SearchModeKeywordOnly || !strings.Contains(rep.Note, "未启用语义召回") {
		t.Errorf("未启用必须显式标注: %+v", rep)
	}
	if len(rep.Hits) == 0 {
		t.Error("降级后关键词召回必须照常可用")
	}
	// 2) 不支持 embedding
	fe := &fakeEmbed{enabled: true, err: plugins.ErrNotSupported}
	rep, _ = SearchCaseDocs(context.Background(), fe, "m1", hybridPages, nil, "违约金", 5)
	if rep.Mode != SearchModeKeywordOnly || !strings.Contains(rep.Note, "不支持") {
		t.Errorf("不支持必须显式标注: %+v", rep)
	}
	// 3) 启用但索引为空
	fe2 := &fakeEmbed{enabled: true}
	rep, _ = SearchCaseDocs(context.Background(), fe2, "m1", hybridPages, map[string][]float64{}, "违约金", 5)
	if rep.Mode != SearchModeKeywordOnly || !strings.Contains(rep.Note, "索引尚未建立") {
		t.Errorf("空索引必须显式标注: %+v", rep)
	}
	// 4) 嵌入运行期失败——降级但如实注明原因
	fe3 := &fakeEmbed{enabled: true, err: errors.New("connection refused")}
	rep, _ = SearchCaseDocs(context.Background(), fe3, "m1", hybridPages, nil, "违约金", 5)
	if rep.Mode != SearchModeKeywordOnly || !strings.Contains(rep.Note, "connection refused") {
		t.Errorf("运行期失败必须如实注明: %+v", rep)
	}
}

// 检索管道没有任何写事实库路径：跑完索引+检索，事实表必须一根毫毛都不动。
func TestSearchPipelineNeverWritesFacts(t *testing.T) {
	sc := indexTestStore(t)
	if err := sc.SaveDocument("d1", "合同.pdf", map[int]string{
		1: "合同约定违约金为合同金额的百分之二十",
	}); err != nil {
		t.Fatal(err)
	}
	pages := []PageText{{DocID: "d1", Page: 1, Text: "合同约定违约金为合同金额的百分之二十"}}
	fe := &fakeEmbed{enabled: true}
	if _, _, err := IndexCasePages(context.Background(), fe, "m1", pages, sc); err != nil {
		t.Fatal(err)
	}
	rows, err := sc.ListEmbeddings("m1")
	if err != nil {
		t.Fatal(err)
	}
	vecs := map[string][]float64{}
	for _, r := range rows {
		vecs[PageRef(r.DocID, r.Page)] = r.Vec
	}
	if _, err := SearchCaseDocs(context.Background(), fe, "m1", pages, vecs, "违约金", 5); err != nil {
		t.Fatal(err)
	}
	facts, err := sc.ListFacts()
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 0 {
		t.Errorf("检索命中不得成为事实: %+v", facts)
	}
}

func TestSearchCaseDocsEmptyQuery(t *testing.T) {
	if _, err := SearchCaseDocs(context.Background(), nil, "", hybridPages, nil, "  ", 5); err == nil {
		t.Error("空检索词必须报错，不得空转冒充无命中")
	}
}
