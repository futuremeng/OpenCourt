// 本案卷宗混合检索（v0.3 M5，roadmap-next M5）：关键词召回 + 向量召回
// （暴力余弦）→ RRF 融合。
// 边界（plan-v0.2 D0/V2）：本管道只搜本案文档——法条归 OpenLaw、类案归
// LocalCases，本层不得自称拥有后两者。检索结果只是候选：没有任何写事实库
// 路径，永不产生五态变化；进事实库仍必须经 core/verify 定位。
package neutral

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/futuremeng/OpenCourt/core/retrieve"
	"github.com/futuremeng/OpenCourt/core/storage"
	"github.com/futuremeng/OpenCourt/plugins"
)

// PageText 本案文档的一页（检索单元 = 页，与 verify 的页码引用坐标一致）。
type PageText struct {
	DocID string
	Page  int
	Text  string
}

// PageVecStore 本案向量存储面（storage.CaseScope 实现；模型隔离由存储层 WHERE 强制）。
type PageVecStore interface {
	ListEmbeddings(model string) ([]storage.EmbeddingInfo, error)
	UpsertEmbedding(docID string, page int, model, textHash string, vec []float64) error
}

// DocSearchHit 卷宗检索一条命中（页级）。是候选，不是事实。
type DocSearchHit struct {
	Ref     string   `json:"ref"` // docID#page
	DocID   string   `json:"doc_id"`
	Page    int      `json:"page"`
	Score   float64  `json:"score"`
	Sources []string `json:"sources"` // keyword / vector 命中来源组合
	Snippet string   `json:"snippet"`
}

// DocSearchReport 卷宗检索报告。Mode/Note 必须如实声明实际召回路径——
// 语义召回未启用时绝不冒充向量化过。
type DocSearchReport struct {
	Query string         `json:"query"`
	Mode  string         `json:"mode"` // hybrid | keyword_only
	Note  string         `json:"note"`
	Hits  []DocSearchHit `json:"hits"`
}

const (
	SearchModeHybrid      = "hybrid"
	SearchModeKeywordOnly = "keyword_only"
	rrfK                  = 60
)

// ErrSemanticUnavailable 语义召回不可用（模型未启用 / 不支持 embedding）。
// 调用方据此降级为纯关键词并显式标注——这是降级，不是失败。
var ErrSemanticUnavailable = errors.New("语义召回不可用")

// PageRef 页定位坐标 docID#page。
func PageRef(docID string, page int) string { return fmt.Sprintf("%s#%d", docID, page) }

func pageTextHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// IndexCasePages 补建本案页向量：text_hash 未变跳过，缺页调 Embed 后入库。
// emb 未启用 / 不支持 → ErrSemanticUnavailable 包装返回（已建部分保留）。
func IndexCasePages(ctx context.Context, emb plugins.ModelAdapter, model string,
	pages []PageText, store PageVecStore) (indexed, skipped int, err error) {
	if emb == nil || !emb.Enabled() {
		return 0, 0, fmt.Errorf("%w：模型未启用", ErrSemanticUnavailable)
	}
	if model == "" {
		model = emb.Name()
	}
	existing := map[string]string{}
	if store != nil {
		rows, lerr := store.ListEmbeddings(model)
		if lerr != nil {
			return 0, 0, lerr
		}
		for _, r := range rows {
			existing[PageRef(r.DocID, r.Page)] = r.TextHash
		}
	}
	for _, p := range pages {
		h := pageTextHash(p.Text)
		if existing[PageRef(p.DocID, p.Page)] == h {
			skipped++
			continue
		}
		vec, eerr := emb.Embed(ctx, p.Text)
		if eerr != nil {
			if errors.Is(eerr, plugins.ErrNotSupported) {
				return indexed, skipped, fmt.Errorf("%w：%v", ErrSemanticUnavailable, eerr)
			}
			return indexed, skipped, eerr
		}
		if store != nil {
			if serr := store.UpsertEmbedding(p.DocID, p.Page, model, h, vec); serr != nil {
				return indexed, skipped, serr
			}
		}
		indexed++
	}
	return indexed, skipped, nil
}

// SearchCaseDocs 混合检索管道：关键词召回（永远可用）+ 向量召回（模型启用
// 且有已索引页时）→ RRF 融合。降级三态（未启用 / 不支持 / 索引为空）一律
// Mode=keyword_only 并在 Note 显式说明原因，绝不静默冒充混合。
func SearchCaseDocs(ctx context.Context, emb plugins.ModelAdapter, embModel string,
	pages []PageText, vecs map[string][]float64, query string, topK int) (DocSearchReport, error) {
	if strings.TrimSpace(query) == "" {
		return DocSearchReport{}, fmt.Errorf("检索词不能为空")
	}
	if topK <= 0 || topK > 50 {
		topK = 10
	}
	texts := make(map[string]string, len(pages))
	loc := make(map[string]PageText, len(pages))
	for _, p := range pages {
		ref := PageRef(p.DocID, p.Page)
		texts[ref] = p.Text
		loc[ref] = p
	}
	kw := retrieve.KeywordSearch(query, texts, topK*2)
	report := DocSearchReport{
		Query: query, Mode: SearchModeKeywordOnly,
		Note: "未启用语义召回：结果仅来自关键词召回",
		Hits: []DocSearchHit{},
	}
	lists := [][]string{retrieve.RankedRefs(kw)}

	if emb != nil && emb.Enabled() {
		qv, err := emb.Embed(ctx, query)
		switch {
		case err != nil && errors.Is(err, plugins.ErrNotSupported):
			report.Note = "当前模型不支持 embedding：结果仅来自关键词召回"
		case err != nil:
			report.Note = fmt.Sprintf("向量召回不可用（%v）：本次仅关键词召回结果", err)
		case len(vecs) == 0:
			report.Note = "向量索引尚未建立（当前 embedding 模型下本案无已索引页）：结果仅来自关键词召回"
		default:
			vec := retrieve.VectorSearch(qv, vecs, topK*2)
			lists = append(lists, retrieve.RankedRefs(vec))
			report.Mode = SearchModeHybrid
			report.Note = ""
		}
	}

	fused := retrieve.FuseRRF(lists, rrfK, topK)
	terms := strings.Fields(query)
	inList := func(list []string, ref string) bool {
		for _, r := range list {
			if r == ref {
				return true
			}
		}
		return false
	}
	var vecList []string
	if report.Mode == SearchModeHybrid {
		vecList = lists[1]
	}
	for _, f := range fused {
		ref := f.Ref.(string)
		p := loc[ref]
		src := []string{}
		if inList(lists[0], ref) {
			src = append(src, "keyword")
		}
		if inList(vecList, ref) {
			src = append(src, "vector")
		}
		if len(src) == 0 { // 理论不可达（融合项必来自某榜）——保守标 keyword 不冒充 vector
			src = append(src, "keyword")
		}
		report.Hits = append(report.Hits, DocSearchHit{
			Ref: ref, DocID: p.DocID, Page: p.Page, Score: f.Score,
			Sources: src, Snippet: snippetAround(p.Text, terms),
		})
	}
	return report, nil
}

// snippetAround 命中词附近摘录（确定性：取首个命中词位置，前后共 120 rune）。
// 无命中词（纯向量召回命中）取页首——不猜内容。
func snippetAround(text string, terms []string) string {
	runes := []rune(text)
	pos := -1
	for _, t := range terms {
		if t == "" {
			continue
		}
		if i := strings.Index(text, t); i >= 0 {
			c := len([]rune(text[:i]))
			if pos < 0 || c < pos {
				pos = c
			}
		}
	}
	start := 0
	if pos > 60 {
		start = pos - 60
	}
	end := start + 120
	if end > len(runes) {
		end = len(runes)
	}
	sn := string(runes[start:end])
	if end < len(runes) {
		sn += "…"
	}
	return sn
}
