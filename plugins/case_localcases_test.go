package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fakeLocalCases 模拟 LocalCases serve（回环 HTTP）：捕获请求参数与 token 头。
type fakeLocalCases struct {
	t        *testing.T
	query    url.Values
	token    string
	resp     map[string]any
	status   int
	respBody string
}

func (f *fakeLocalCases) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/health" {
		writeJSON(f.t, w, map[string]any{"ok": true, "version": "0.3.1", "indexed_documents": 12345})
		return
	}
	f.query = r.URL.Query()
	f.token = r.Header.Get("x-localcases-token")
	if f.status != 0 {
		http.Error(w, f.respBody, f.status)
		return
	}
	if f.respBody != "" {
		w.Write([]byte(f.respBody))
		return
	}
	writeJSON(f.t, w, f.resp)
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatal(err)
	}
}

func newLocalCasesTest(t *testing.T, fc *fakeLocalCases) (*LocalCasesSource, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(fc) // httptest 绑定 127.0.0.1——回环护栏放行
	return NewLocalCasesSource(srv.URL, "tok-123", true), srv
}

// 契约对齐：参数名 law（非 legal_basis）、无 limit、分页 offset/id_gt、
// token 走 x-localcases-token 头。
func TestLocalCasesSearchFilteredContract(t *testing.T) {
	fc := &fakeLocalCases{t: t, resp: map[string]any{
		"search_meta": map[string]any{"total": 137, "structured": true},
		"cases": []map[string]any{{
			"id": 42, "case_id": "（2020）川01民终1234号", "case_name": "某借贷纠纷",
			"court": "成都中院", "cause": "民间借贷纠纷", "judgment_date": "2020-05-11",
			"legal_basis": "民法典第675条", "preview": "原告张三向被告出借……",
		}},
		"facets": map[string]any{
			"year": map[string]any{"buckets": []map[string]any{{"key": 2020, "doc_count": 41}}},
		},
	}}
	src, srv := newLocalCasesTest(t, fc)
	defer srv.Close()

	res, err := src.SearchFiltered(context.Background(), CaseQuery{
		Search: "借款 合同", Cause: "民间借贷纠纷", Law: "中华人民共和国民法典",
		YearFrom: 2019, GroupBy: []string{"year", "court"}, FacetSize: 10,
		Offset: 20, IDAfter: 500,
	})
	if err != nil {
		t.Fatal(err)
	}
	if fc.token != "tok-123" {
		t.Errorf("token 头错误: %q", fc.token)
	}
	q := fc.query
	if q.Get("law") == "" || q.Get("legal_basis") != "" {
		t.Errorf("法条过滤参数必须是 law: %v", q)
	}
	if q.Has("limit") {
		t.Error("契约没有 limit 参数——不得发送")
	}
	if q.Get("id_gt") != "500" || q.Get("offset") != "20" || q.Get("year_from") != "2019" ||
		q.Get("group_by") != "year,court" || q.Get("size") != "10" {
		t.Errorf("参数透传错误: %v", q)
	}
	if res.Total != 137 || !res.Structured || len(res.Hits) != 1 {
		t.Fatalf("结果映射错误: %+v", res)
	}
	if res.Hits[0].SourceCaseID != "（2020）川01民终1234号" || res.Hits[0].Court != "成都中院" {
		t.Errorf("命中字段映射错误: %+v", res.Hits[0])
	}
	if len(res.Facets["year"]) != 1 || res.Facets["year"][0].Key != "2020" {
		t.Errorf("facet 解析错误: %+v", res.Facets)
	}
}

// structured=false：过滤被诚实降级——facet 不解析不呈现（绝不把降级当过滤）。
func TestLocalCasesStructuredFalseHidesFacets(t *testing.T) {
	fc := &fakeLocalCases{t: t, respBody: `{"search_meta":{"total":9,"structured":false},
		"cases":[],"facets":{"year":{"buckets":[{"key":2020,"doc_count":9}]}}}`}
	src, srv := newLocalCasesTest(t, fc)
	defer srv.Close()

	res, err := src.SearchFiltered(context.Background(), CaseQuery{Search: "借款", Court: "某法院"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Structured || res.Facets != nil {
		t.Errorf("structured=false 必须降级呈现且不回 facet: %+v", res)
	}
}

// structured 字段缺失（LC-7 未就绪）：保守按 false，不冒充过滤过。
func TestLocalCasesMissingStructuredConservative(t *testing.T) {
	fc := &fakeLocalCases{t: t, respBody: `{"search_meta":{"total":3},"cases":[]}`}
	src, srv := newLocalCasesTest(t, fc)
	defer srv.Close()
	res, err := src.SearchFiltered(context.Background(), CaseQuery{Search: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Structured {
		t.Error("structured 缺字段必须保守按 false")
	}
}

// 401 → token 指引；400 → 原样展示不改写重试。
func TestLocalCasesErrorHandling(t *testing.T) {
	fc := &fakeLocalCases{t: t, status: http.StatusUnauthorized, respBody: "unauthorized"}
	src, srv := newLocalCasesTest(t, fc)
	defer srv.Close()
	_, err := src.SearchFiltered(context.Background(), CaseQuery{Search: "x"})
	if err == nil || !strings.Contains(err.Error(), "token") {
		t.Errorf("401 必须提示 token: %v", err)
	}

	fc2 := &fakeLocalCases{t: t, status: http.StatusBadRequest, respBody: "unknown group_by: foo"}
	src2, srv2 := newLocalCasesTest(t, fc2)
	defer srv2.Close()
	_, err = src2.SearchFiltered(context.Background(), CaseQuery{Search: "x"})
	if err == nil || !strings.Contains(err.Error(), "unknown group_by: foo") {
		t.Errorf("400 必须原样展示: %v", err)
	}
}

// 未启用/非回环：一律报错带指引；绝不静默空列表。
func TestLocalCasesDisabledAndNonLoopback(t *testing.T) {
	off := NewLocalCasesSource("http://127.0.0.1:5010", "", false)
	if off.Enabled() {
		t.Error("默认必须禁用")
	}
	if _, err := off.SearchFiltered(context.Background(), CaseQuery{}); err == nil ||
		!strings.Contains(err.Error(), LocalCasesGuidance) {
		t.Errorf("未启用必须报错带指引: %v", err)
	}
	remote := NewLocalCasesSource("http://example.com:5010", "t", true)
	if remote.Enabled() {
		t.Error("非回环地址必须拒绝（类案语料含未脱敏文书）")
	}
	if _, err := remote.SearchCases(context.Background(), "x", 5); err == nil {
		t.Error("非回环检索必须报错")
	}
}

// Health 探测：ok/version/indexed_documents（语料规模必须显示，防小样本误读）。
func TestLocalCasesHealth(t *testing.T) {
	fc := &fakeLocalCases{t: t}
	src, srv := newLocalCasesTest(t, fc)
	defer srv.Close()
	h, err := src.Health(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !h.OK || h.Version != "0.3.1" || h.IndexedDocuments != 12345 {
		t.Errorf("health 解析错误: %+v", h)
	}
}
