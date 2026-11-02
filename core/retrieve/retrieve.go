// Package retrieve 检索层模块（开发文档 4.4、7.4；plan-v0.2 V2 收窄；v0.3 M5）。
// 仅负责「本案卷宗内」的混合检索：关键词召回 + 向量召回（暴力余弦，
// 万级页量足够，不引外部向量库）+ RRF 融合。检索结果是候选——进入事实
// 库仍必须经 core/verify 定位，命中本身永不产生任何五态变化。
// 【V2】法条/类案条目类型已移除：法条与类案是外部语料（OpenLaw /
// LocalCases），其检索归 plugins 适配层 + OpenLaw sdk，效力状态归 OpenLaw——
// OpenCourt 检索层不得自建语料类型、不得自行判定效力。
package retrieve

import (
	"math"
	"sort"
	"strings"
)

// Result 一条卷宗检索结果及重排得分。
type Result struct {
	Ref   any     `json:"ref"` // 文档定位（卷宗内：docID#page）
	Score float64 `json:"score"`
}

// KeywordSearch 关键词召回 + 简单重排（命中词数/文档长度），返回 topK。
// docs 为 定位→文本（必须来自当前案件的卷宗，跨案件在存储层即失败）。
// 同分按 Ref 升序——结果必须可复现（M5 验收），不得依赖 map 遍历顺序。
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
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return refKey(results[i].Ref) < refKey(results[j].Ref)
	})
	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}
	return results
}

// Cosine 余弦相似度。维度不符或零向量返回 0（不猜方向）。
func Cosine(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// VectorSearch 暴力余弦向量召回（纯 Go，无外部向量库），返回 topK。
// 同分按 Ref 升序，保证可复现。
func VectorSearch(queryVec []float64, vecs map[string][]float64, topK int) []Result {
	results := make([]Result, 0, len(vecs))
	for ref, v := range vecs {
		if s := Cosine(queryVec, v); s > 0 {
			results = append(results, Result{Ref: ref, Score: s})
		}
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return refKey(results[i].Ref) < refKey(results[j].Ref)
	})
	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}
	return results
}

// FuseRRF Reciprocal Rank Fusion（k=60 惯例值）：只取各召回列表的排名，
// 对分数尺度天然免疫（关键词与余弦分数量纲不同不可直接相加）。
// lists 为多条已排序的 Ref 列表（各自降序）；融合分 = Σ 1/(k+rank)，rank 从 1 起。
// 同分按 Ref 升序——融合结果必须可复现（M5 验收）。
func FuseRRF(lists [][]string, k, topK int) []Result {
	if k <= 0 {
		k = 60
	}
	scores := map[string]float64{}
	for _, list := range lists {
		for rank, ref := range list {
			scores[ref] += 1.0 / float64(k+rank+1)
		}
	}
	results := make([]Result, 0, len(scores))
	for ref, s := range scores {
		results = append(results, Result{Ref: ref, Score: s})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Ref.(string) < results[j].Ref.(string)
	})
	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}
	return results
}

// RankedRefs 把 Result 列表转为 Ref 字符串列表（供 FuseRRF）。
func RankedRefs(rs []Result) []string {
	refs := make([]string, 0, len(rs))
	for _, r := range rs {
		refs = append(refs, refKey(r.Ref))
	}
	return refs
}

func refKey(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
