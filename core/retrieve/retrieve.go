// Package retrieve 检索层模块（开发文档 4.4、7.4）：
// “向量+关键词+重排”混合检索。本阶段实现关键词召回与重排接口；
// 向量检索由 plugins/model 的 embedding 适配器在后续阶段接入。
// 法条条目强制携带版本与废止状态，已废止法条在输出端标红。
package retrieve

import (
	"sort"
	"strings"
)

// LawEntry 法条条目：必须带 effective_date 与 status（开发文档 7.4 门禁 1）。
type LawEntry struct {
	LawID         string `json:"law_id"`
	Title         string `json:"title"`
	Article       string `json:"article"`
	EffectiveDate string `json:"effective_date"`
	Status        string `json:"status"` // EFFECTIVE / AMENDED / REPEALED
}

// IsRepealed 报告法条是否已废止。
func (l LawEntry) IsRepealed() bool { return l.Status == "REPEALED" }

// CaseEntry 类案条目：带裁判要旨。
type CaseEntry struct {
	CaseID  string `json:"case_id"`
	Title   string `json:"title"`
	Summary string `json:"summary"` // 裁判要旨
}

// Result 一条检索结果及重排得分。
type Result struct {
	Kind  string  `json:"kind"` // LAW / CASE / DOC
	Ref   any     `json:"ref"`
	Score float64 `json:"score"`
}

// KeywordSearch 关键词召回 + 简单重排（命中词数/文档长度），返回 topK。
// docs 为 文档ID→文本。
func KeywordSearch(query string, docs map[string]string, topK int) []Result {
	terms := strings.Fields(query)
	results := make([]Result, 0, len(docs))
	for id, text := range docs {
		score := 0.0
		for _, t := range terms {
			if t != "" {
				score += float64(strings.Count(text, t))
			}
		}
		if score > 0 {
			norm := score
			if n := len([]rune(text)); n > 0 {
				norm = score / float64(n) * 1000
			}
			results = append(results, Result{Kind: "DOC", Ref: id, Score: norm})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}
	return results
}

// FilterRepealed 从法条结果中分离出已废止条目（输出端必须标红提示）。
func FilterRepealed(entries []LawEntry) (active []LawEntry, repealed []LawEntry) {
	for _, e := range entries {
		if e.IsRepealed() {
			repealed = append(repealed, e)
		} else {
			active = append(active, e)
		}
	}
	return active, repealed
}
