// 本案卷宗混合检索绑定（v0.3 M5）。范围严格限定本案卷宗（plan-v0.2 D0/V2）：
// 法条检索归 OpenLaw、类案检索归 LocalCases，本文件不得自称拥有后两者。
package main

import (
	"errors"
	"fmt"
	"sort"

	"github.com/futuremeng/OpenCourt/neutral"
)

// SearchCaseDocs 卷宗混合检索：关键词召回 + 向量召回（模型启用时）→ RRF 融合。
// 模型未启用自动降级为纯关键词并在 Note 显式标注——绝不静默冒充向量化过。
// 结果只是候选：本绑定没有任何写事实库路径，永不产生五态变化；进事实库
// 仍必须经阅卷/核验器定位。审计 DOC_SEARCH 记录实际召回路径与向量用量。
func (a *App) SearchCaseDocs(query string, topK int) (neutral.DocSearchReport, error) {
	sc := a.scope()
	docs, err := sc.ListDocuments()
	if err != nil {
		return neutral.DocSearchReport{}, err
	}
	pages := []neutral.PageText{}
	for _, d := range docs {
		doc, derr := sc.GetDocument(d.DocID)
		if derr != nil {
			return neutral.DocSearchReport{}, derr
		}
		nums := make([]int, 0, len(doc.Pages))
		for n := range doc.Pages {
			nums = append(nums, n)
		}
		sort.Ints(nums)
		for _, n := range nums {
			pages = append(pages, neutral.PageText{DocID: d.DocID, Page: n, Text: doc.Pages[n]})
		}
	}

	a.mu.RLock()
	emb := a.model
	embModel := a.settings.Model.EmbeddingModel
	a.mu.RUnlock()
	if emb != nil && emb.Enabled() && embModel == "" {
		embModel = emb.Name()
	}

	indexed, skipped := 0, 0
	var vecs map[string][]float64
	if emb != nil && emb.Enabled() {
		var ierr error
		indexed, skipped, ierr = neutral.IndexCasePages(a.ctx, emb, embModel, pages, sc)
		if ierr != nil && !errors.Is(ierr, neutral.ErrSemanticUnavailable) {
			_ = a.auditLog.Log(a.profile.Role, "DOC_SEARCH_FAIL", "", ierr.Error())
			return neutral.DocSearchReport{}, ierr
		}
		rows, lerr := sc.ListEmbeddings(embModel)
		if lerr != nil {
			_ = a.auditLog.Log(a.profile.Role, "DOC_SEARCH_FAIL", "", lerr.Error())
			return neutral.DocSearchReport{}, lerr
		}
		vecs = make(map[string][]float64, len(rows))
		for _, r := range rows {
			vecs[neutral.PageRef(r.DocID, r.Page)] = r.Vec
		}
	}

	report, err := neutral.SearchCaseDocs(a.ctx, emb, embModel, pages, vecs, query, topK)
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "DOC_SEARCH_FAIL", query, err.Error())
		return report, err
	}
	_ = a.auditLog.Log(a.profile.Role, "DOC_SEARCH", "",
		fmt.Sprintf("%s 命中 %d 条（%s）向量新建 %d 复用 %d",
			report.Mode, len(report.Hits), report.Note, indexed, skipped))
	return report, nil
}

// CaseIndexStatus 本案向量索引状态（前端徽章）：scope 恒为 case——本能力只
// 索引本案卷宗，向量不出本机、更不碰法条/类案语料。
func (a *App) CaseIndexStatus() (map[string]any, error) {
	a.mu.RLock()
	emb := a.model
	embModel := a.settings.Model.EmbeddingModel
	a.mu.RUnlock()
	if emb != nil && emb.Enabled() && embModel == "" {
		embModel = emb.Name()
	}
	out := map[string]any{
		"model_enabled":   emb != nil && emb.Enabled(),
		"embedding_model": embModel,
		"indexed_pages":   0,
		"total_pages":     0,
		"scope":           "case",
	}
	sc := a.scope()
	docs, err := sc.ListDocuments()
	if err != nil {
		return out, err
	}
	total := 0
	for _, d := range docs {
		total += d.PageCount
	}
	out["total_pages"] = total
	if emb != nil && emb.Enabled() {
		rows, err := sc.ListEmbeddings(embModel)
		if err != nil {
			return out, err
		}
		out["indexed_pages"] = len(rows)
	}
	return out, nil
}
