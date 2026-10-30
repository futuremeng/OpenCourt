package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// LocalCasesGuidance 类案源不可用时的固定指引（plan-v0.2 D3）：
// OpenCourt 绝不启动 LocalCases 进程、绝不降级到其他抓取方式，只给指引。
const LocalCasesGuidance = "类案源不可用：请自行启动 LocalCases（serve，回环地址）并在配置中启用 localcases（base_url + token_env）。" +
	"LC-6 发现文件未就绪前此为手动临时通道——OpenCourt 不会启动外部进程，也不会改用其他来源"

// CaseQuery 类案检索参数（契约见 docs/localcases-integration.md §4）。
// 注意：没有 Limit——LocalCases 每页固定 20 条，分页走 Offset 或 IDAfter 游标。
type CaseQuery struct {
	Search    string   `json:"search"`
	Court     string   `json:"court,omitempty"`
	Cause     string   `json:"cause,omitempty"`
	Law       string   `json:"law,omitempty"` // ⚠️ 过滤参数是 law（legal_basis 是响应字段名）
	Procedure string   `json:"procedure,omitempty"`
	YearFrom  int      `json:"year_from,omitempty"`
	YearTo    int      `json:"year_to,omitempty"`
	DateFrom  string   `json:"date_from,omitempty"`
	DateTo    string   `json:"date_to,omitempty"`
	GroupBy   []string `json:"group_by,omitempty"`
	FacetSize int      `json:"facet_size,omitempty"` // size，默认 20，上限 1000
	Offset    int      `json:"offset,omitempty"`
	IDAfter   int64    `json:"id_after,omitempty"` // id_gt 游标（深翻页推荐）
	Sort      string   `json:"sort,omitempty"`     // relevance / date / id
}

// FacetBucket 分布桶（D6：只呈现桶与计数，不换算百分比排名——LC-4 无误差上界）。
type FacetBucket struct {
	Key      string `json:"key"`
	DocCount int64  `json:"doc_count"`
}

// CaseDetailHit 类案命中完整字段（用于 case_refs 留痕；preview 必须过 redact 再入 UI/模型）。
type CaseDetailHit struct {
	SourceCaseID string `json:"source_case_id"`
	URL          string `json:"url,omitempty"`
	CaseName     string `json:"case_name,omitempty"`
	Court        string `json:"court,omitempty"`
	Cause        string `json:"cause,omitempty"`
	JudgmentDate string `json:"judgment_date,omitempty"`
	LegalBasis   string `json:"legal_basis,omitempty"`
	Preview      string `json:"preview,omitempty"` // 前 240 字
}

// CaseSearchResult 过滤检索结果。Structured=false 表示 LocalCases 索引缺
// 精确匹配层、过滤条件被诚实降级忽略——UI 必须提示并隐藏 facet，
// 绝不把降级结果当过滤结果呈现。
type CaseSearchResult struct {
	Total      int64
	Structured bool
	Hits       []CaseDetailHit
	Facets     map[string][]FacetBucket
}

// FilteredCaseSource 带过滤/分布能力的类案源（LocalCases 实现）。
type FilteredCaseSource interface {
	CaseSourcePlugin
	SearchFiltered(ctx context.Context, q CaseQuery) (CaseSearchResult, error)
}

// LocalCasesHealth 就绪探测结果（LC-7 未就绪时 Version 为空 = 版本未知，
// 功能可用但界面标注）。
type LocalCasesHealth struct {
	OK               bool
	Version          string
	IndexedDocuments int64
}

// LocalCasesSource 类案源插件（外部可选组件，默认禁用；与 BrowserOS 同纪律：
// 只出站连用户自装的回环 HTTP 服务，不拉起进程，连不上只给指引）。
type LocalCasesSource struct {
	baseURL string
	token   string
	enabled bool
	client  *http.Client
}

// NewLocalCasesSource 构造类案源。baseURL 必须是回环地址（护栏）；
// token 只能经 token_env 注入（严禁写入配置文件）。
func NewLocalCasesSource(baseURL, token string, enabled bool) *LocalCasesSource {
	return &LocalCasesSource{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		enabled: enabled,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *LocalCasesSource) Name() string { return "localcases" }

// Enabled 启用 = 显式 enabled 且 base_url 已填且为回环地址。
func (s *LocalCasesSource) Enabled() bool {
	return s.enabled && s.baseURL != "" && s.loopback()
}

// loopback 回环护栏：类案语料含未脱敏裁判文书，绝不允许指向远程地址。
func (s *LocalCasesSource) loopback() bool {
	u, err := url.Parse(s.baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Health 就绪探测（GET /api/health）。未启用直接报错带指引。
func (s *LocalCasesSource) Health(ctx context.Context) (LocalCasesHealth, error) {
	if !s.Enabled() {
		return LocalCasesHealth{}, fmt.Errorf("%s", LocalCasesGuidance)
	}
	var resp struct {
		OK               bool   `json:"ok"`
		Version          string `json:"version"`
		IndexedDocuments int64  `json:"indexed_documents"`
	}
	if err := s.getJSON(ctx, s.baseURL+"/api/health", &resp); err != nil {
		return LocalCasesHealth{}, err
	}
	return LocalCasesHealth{OK: resp.OK, Version: resp.Version, IndexedDocuments: resp.IndexedDocuments}, nil
}

// SearchCases 实现 CaseSourcePlugin：关键词检索（映射到 CaseHit 三字段）。
// 每页固定 20 条（LocalCases PAGE_SIZE），topK 只截断本页。
func (s *LocalCasesSource) SearchCases(ctx context.Context, query string, topK int) ([]CaseHit, error) {
	res, err := s.SearchFiltered(ctx, CaseQuery{Search: query})
	if err != nil {
		return nil, err
	}
	if topK <= 0 || topK > len(res.Hits) {
		topK = len(res.Hits)
	}
	out := make([]CaseHit, 0, topK)
	for _, h := range res.Hits[:topK] {
		out = append(out, CaseHit{CaseID: h.SourceCaseID, Title: h.CaseName, Summary: h.Preview})
	}
	return out, nil
}

// SearchFiltered 过滤检索（GET /api/search）。参数名严格对齐 LocalCases 契约：
// 法条过滤是 law；没有 limit，每页固定 20 条，分页 offset / id_gt。
// 坏参数 400 原样上抛（不猜、不自动改写重试）。
func (s *LocalCasesSource) SearchFiltered(ctx context.Context, q CaseQuery) (CaseSearchResult, error) {
	if !s.Enabled() {
		return CaseSearchResult{}, fmt.Errorf("%s", LocalCasesGuidance)
	}
	params := url.Values{}
	set := func(k, v string) {
		if strings.TrimSpace(v) != "" {
			params.Set(k, v)
		}
	}
	set("search", q.Search)
	set("court", q.Court)
	set("cause", q.Cause)
	set("law", q.Law)
	set("procedure", q.Procedure)
	if q.YearFrom > 0 {
		params.Set("year_from", strconv.Itoa(q.YearFrom))
	}
	if q.YearTo > 0 {
		params.Set("year_to", strconv.Itoa(q.YearTo))
	}
	set("date_from", q.DateFrom)
	set("date_to", q.DateTo)
	if len(q.GroupBy) > 0 {
		params.Set("group_by", strings.Join(q.GroupBy, ","))
	}
	if q.FacetSize > 0 {
		params.Set("size", strconv.Itoa(q.FacetSize))
	}
	if q.Offset > 0 {
		params.Set("offset", strconv.Itoa(q.Offset))
	}
	if q.IDAfter > 0 {
		params.Set("id_gt", strconv.FormatInt(q.IDAfter, 10))
	}
	set("sort", q.Sort)

	var resp struct {
		SearchMeta struct {
			Total      int64 `json:"total"`
			Structured *bool `json:"structured"` // 缺字段 = 版本未知：保守按 false（不冒充过滤过）
		} `json:"search_meta"`
		Cases []struct {
			ID           int64  `json:"id"`
			URL          string `json:"url"`
			CaseID       string `json:"case_id"`
			CaseName     string `json:"case_name"`
			Court        string `json:"court"`
			Cause        string `json:"cause"`
			JudgmentDate string `json:"judgment_date"`
			LegalBasis   string `json:"legal_basis"`
			Preview      string `json:"preview"`
		} `json:"cases"`
		Facets map[string]struct {
			Buckets []struct {
				Key      json.RawMessage `json:"key"`
				DocCount int64           `json:"doc_count"`
			} `json:"buckets"`
		} `json:"facets"`
	}
	if err := s.getJSON(ctx, s.baseURL+"/api/search?"+params.Encode(), &resp); err != nil {
		return CaseSearchResult{}, err
	}
	out := CaseSearchResult{
		Total:      resp.SearchMeta.Total,
		Structured: resp.SearchMeta.Structured != nil && *resp.SearchMeta.Structured,
		Hits:       make([]CaseDetailHit, 0, len(resp.Cases)),
	}
	for _, c := range resp.Cases {
		out.Hits = append(out.Hits, CaseDetailHit{
			SourceCaseID: c.CaseID,
			URL:          c.URL,
			CaseName:     c.CaseName,
			Court:        c.Court,
			Cause:        c.Cause,
			JudgmentDate: c.JudgmentDate,
			LegalBasis:   c.LegalBasis,
			Preview:      c.Preview,
		})
	}
	if out.Structured { // 无精确层时聚合不可信——不解析 facet（§5/D6）
		out.Facets = map[string][]FacetBucket{}
		for name, f := range resp.Facets {
			bs := make([]FacetBucket, 0, len(f.Buckets))
			for _, b := range f.Buckets {
				key := strings.Trim(string(b.Key), `"`) // 数字 key 的 JSON 引号
				bs = append(bs, FacetBucket{Key: key, DocCount: b.DocCount})
			}
			out.Facets[name] = bs
		}
	}
	return out, nil
}

// getJSON 出站 GET（带 x-localcases-token）。错误处置按 §9：
// 连接失败 → 指引；401 → token 缺失/不匹配；400 → 原样展示；不猜不重试。
func (s *LocalCasesSource) getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("类案请求构造失败: %w", err)
	}
	if s.token != "" {
		req.Header.Set("x-localcases-token", s.token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("连接 LocalCases 失败: %v（%s）", err, LocalCasesGuidance)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("读取 LocalCases 响应失败: %v", err)
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("LocalCases 拒绝（401）：token 缺失或不匹配——请检查 token_env 指向的环境变量")
	case http.StatusBadRequest:
		return fmt.Errorf("LocalCases 拒绝（400，坏参数不自动改写重试）: %s", strings.TrimSpace(string(body)))
	case http.StatusOK:
	default:
		return fmt.Errorf("LocalCases 返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("解析 LocalCases 响应失败（契约不匹配不猜）: %v", err)
	}
	return nil
}
