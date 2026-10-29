// Package retrieve 检索层模块（开发文档 4.4、7.4；plan-v0.2 V2 收窄）：
// 仅负责「本案卷宗内」的混合检索（关键词召回 + 重排；向量由 plugins/model
// embedding 后续接入）。
// 【V2】法条/类案条目类型已移除：法条与类案是外部语料（OpenLaw /
// LocalCases），其检索归 plugins 适配层 + OpenLaw sdk，效力状态归 OpenLaw——
// OpenCourt 检索层不得自建语料类型、不得自行判定效力。
package retrieve

import (
	"sort"
	"strings"
)

// Result 一条卷宗检索结果及重排得分。
type Result struct {
	Ref   any     `json:"ref"` // 文档ID（卷宗内）
	Score float64 `json:"score"`
}

// KeywordSearch 关键词召回 + 简单重排（命中词数/文档长度），返回 topK。
// docs 为 文档ID→文本（必须来自当前案件的卷宗，跨案件在存储层即失败）。
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
			results = append(results, Result{Ref: id, Score: norm})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}
	return results
}
