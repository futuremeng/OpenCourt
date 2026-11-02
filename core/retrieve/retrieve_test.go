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
