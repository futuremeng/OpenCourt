package retrieve

import "testing"

// 卷宗内检索：命中排序按归一化得分，topK 截断，未命中不得出现在结果里
// （「卷宗里没有」必须区别于「检索失败」）。
func TestKeywordSearchRankingAndTopK(t *testing.T) {
	docs := map[string]string{
		"d1": "违约金条款约定违约金为合同金额的百分之二十",
		"d2": "送达地址确认书一份",
		"d3": "违约金 违约金 违约金 交付与验收",
	}
	res := KeywordSearch("违约金", docs, 2)
	if len(res) != 2 {
		t.Fatalf("topK=2 应返回 2 条: %+v", res)
	}
	if res[0].Ref != "d3" {
		t.Errorf("命中最多的文档应排第一，实际 %v", res[0].Ref)
	}
	if res[0].Score < res[1].Score {
		t.Errorf("得分必须降序: %+v", res)
	}
	for _, r := range res {
		if r.Score <= 0 {
			t.Errorf("未命中不得进入结果: %+v", r)
		}
	}
}

func TestKeywordSearchNoHit(t *testing.T) {
	res := KeywordSearch("不可抗力", map[string]string{"d1": "合同解除通知"}, 10)
	if len(res) != 0 {
		t.Errorf("无命中应返回空: %+v", res)
	}
	if res == nil {
		t.Error("应返回空切片而非 nil")
	}
}

// topK<=0 表示不截断（调用方全量取回时的语义）。
func TestKeywordSearchTopKNonPositive(t *testing.T) {
	docs := map[string]string{"a": "违约金", "b": "违约金", "c": "违约金"}
	if got := KeywordSearch("违约金", docs, 0); len(got) != 3 {
		t.Errorf("topK=0 应返回全部命中: %+v", got)
	}
}

// 同分必须按 Ref 升序——结果可复现是 M5 验收项，不得随 map 遍历顺序漂移。
func TestKeywordSearchDeterministicTieBreak(t *testing.T) {
	docs := map[string]string{"d3": "违约金", "d1": "违约金", "d2": "违约金"}
	first := KeywordSearch("违约金", docs, 0)
	for i := 0; i < 20; i++ {
		got := KeywordSearch("违约金", docs, 0)
		for j := range got {
			if got[j].Ref != first[j].Ref {
				t.Fatalf("同分排序不可复现: %v vs %v", got, first)
			}
		}
	}
	if first[0].Ref != "d1" || first[2].Ref != "d3" {
		t.Errorf("同分应按 Ref 升序: %v %v %v", first[0].Ref, first[1].Ref, first[2].Ref)
	}
}

func TestCosine(t *testing.T) {
	if s := Cosine([]float64{1, 0}, []float64{1, 0}); s != 1 {
		t.Errorf("同向量余弦应为 1，实际 %v", s)
	}
	if s := Cosine([]float64{1, 0}, []float64{0, 1}); s != 0 {
		t.Errorf("正交向量余弦应为 0，实际 %v", s)
	}
	if s := Cosine([]float64{1, 2}, []float64{1}); s != 0 {
		t.Errorf("维度不符应返回 0（不猜方向），实际 %v", s)
	}
	if s := Cosine([]float64{0, 0}, []float64{1, 1}); s != 0 {
		t.Errorf("零向量余弦应为 0，实际 %v", s)
	}
}

// 向量召回：按余弦降序、topK 截断、非正相似进不了结果。
func TestVectorSearchRankingAndTopK(t *testing.T) {
	vecs := map[string][]float64{
		"d1#1": {1, 0},
		"d2#1": {0, 1},
		"d3#1": {0, 2},
	}
	res := VectorSearch([]float64{0, 1}, vecs, 2)
	if len(res) != 2 {
		t.Fatalf("topK=2 应返回 2 条: %+v", res)
	}
	if res[0].Ref != "d2#1" && res[0].Ref != "d3#1" {
		t.Errorf("与查询同向的应排前: %+v", res)
	}
	if res[0].Score < res[1].Score {
		t.Errorf("得分必须降序: %+v", res)
	}
}

// RRF 融合：双榜同现 > 单榜；只排名参与、分数尺度免疫；可复现。
func TestFuseRRF(t *testing.T) {
	kw := []string{"a", "b", "c"}
	vec := []string{"b", "c", "d"}
	res := FuseRRF([][]string{kw, vec}, 60, 4)
	if len(res) != 4 {
		t.Fatalf("并集应 4 条: %+v", res)
	}
	score := map[string]float64{}
	for _, r := range res {
		score[r.Ref.(string)] = r.Score
	}
	// b 在两个榜都排第 2，必须高于任何单榜第 1（a）
	if score["b"] <= score["a"] {
		t.Errorf("双榜同现必须高于单榜首名: %v", score)
	}
	if score["d"] >= score["c"] {
		t.Errorf("单榜靠后者得分应更低: %v", score)
	}
	for i := 0; i < 20; i++ {
		again := FuseRRF([][]string{kw, vec}, 60, 4)
		for j := range again {
			if again[j].Ref != res[j].Ref {
				t.Fatalf("融合结果不可复现: %v vs %v", again, res)
			}
		}
	}
}

// 同分（同排名结构）必须按 Ref 升序，保证可复现。
func TestFuseRRFDeterministicTieBreak(t *testing.T) {
	res := FuseRRF([][]string{{"z"}, {"a"}, {"m"}}, 60, 0)
	if res[0].Ref != "a" || res[2].Ref != "z" {
		t.Errorf("同分应按 Ref 升序: %+v", res)
	}
}
