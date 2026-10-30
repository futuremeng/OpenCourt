package neutral

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/futuremeng/OpenCourt/plugins"
)

// fakeCaseSource 类案源测试替身（实现 FilteredCaseSource）。
type fakeCaseSource struct {
	res plugins.CaseSearchResult
	err error
	q   plugins.CaseQuery
}

func (f *fakeCaseSource) Name() string  { return "fake-cases" }
func (f *fakeCaseSource) Enabled() bool { return true }
func (f *fakeCaseSource) SearchCases(_ context.Context, _ string, _ int) ([]plugins.CaseHit, error) {
	return nil, nil
}
func (f *fakeCaseSource) SearchFiltered(_ context.Context, q plugins.CaseQuery) (plugins.CaseSearchResult, error) {
	f.q = q
	return f.res, f.err
}

func TestRunCaseSearchNilSourceGuidance(t *testing.T) {
	_, err := RunCaseSearch(context.Background(), nil, nil, plugins.CaseQuery{})
	if err == nil || !strings.Contains(err.Error(), plugins.LocalCasesGuidance) {
		t.Errorf("未配置类案源必须报错带指引（不降级）: %v", err)
	}
}

// 要旨/预览必须过 core/redact（唯一强制关口）；CaseRef 留痕字段完整。
func TestRunCaseSearchRedactsAndRefs(t *testing.T) {
	src := &fakeCaseSource{res: plugins.CaseSearchResult{
		Total: 137, Structured: true,
		Hits: []plugins.CaseDetailHit{{
			SourceCaseID: "（2020）川01民终1234号", CaseName: "某借贷纠纷",
			Court: "成都中院", Cause: "民间借贷纠纷", JudgmentDate: "2020-05-11",
			LegalBasis: "民法典第675条",
			Preview:    "原告张三向被告李四出借 6222021234567890 元",
		}},
		Facets: map[string][]plugins.FacetBucket{"year": {{Key: "2020", DocCount: 41}}},
	}}
	rep, err := RunCaseSearch(context.Background(), src, []string{"张三", "李四"}, plugins.CaseQuery{Search: "借款"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Refs) != 1 {
		t.Fatalf("应产 1 条 CaseRef: %+v", rep.Refs)
	}
	r := rep.Refs[0]
	if strings.Contains(r.Summary, "张三") || strings.Contains(r.Summary, "李四") {
		t.Errorf("要旨必须强制脱敏（姓名一致替换）: %s", r.Summary)
	}
	if !strings.Contains(r.Summary, "PERSON_001") {
		t.Errorf("脱敏应为可逆一致替换: %s", r.Summary)
	}
	if r.SourceCaseID == "" || r.Court != "成都中院" || !strings.HasPrefix(r.RefID, "CASE-") {
		t.Errorf("CaseRef 留痕字段错误: %+v", r)
	}
	if rep.Disclaimer != CaseSearchDisclaimer {
		t.Errorf("报告必须携带固定标注: %s", rep.Disclaimer)
	}
	if r.FacetsJSON == "" {
		t.Error("structured=true 时 facet 分布应随行留痕")
	}
	var fb map[string][]plugins.FacetBucket
	if err := json.Unmarshal([]byte(r.FacetsJSON), &fb); err != nil || len(fb["year"]) != 1 {
		t.Errorf("facets_json 解析错误: %v", err)
	}
}

// structured=false：facet 一律不呈现不留痕（降级结果不当过滤结果）。
func TestRunCaseSearchUnstructuredNoFacets(t *testing.T) {
	src := &fakeCaseSource{res: plugins.CaseSearchResult{
		Total: 9, Structured: false,
		Hits:   []plugins.CaseDetailHit{{SourceCaseID: "c1", Preview: "x"}},
		Facets: map[string][]plugins.FacetBucket{"year": {{Key: "2020", DocCount: 9}}},
	}}
	rep, err := RunCaseSearch(context.Background(), src, nil, plugins.CaseQuery{Search: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Facets != nil || rep.Refs[0].FacetsJSON != "" {
		t.Errorf("structured=false 不得呈现/留痕 facet: %+v", rep)
	}
}
