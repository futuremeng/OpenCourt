package extract

import "testing"

// 确定性抽取：只认能回原文定位的要素（Located=false 一律禁止入库），
// 且必须携带卷宗坐标——否则后续核验器无法证明要素不是幻觉。
func TestExtractTimeAndAmount(t *testing.T) {
	text := "2024-03-01 签订合同，约定 2024年5月1日 前交付；已付款 128,000.50 元，" +
		"违约金人民币 30万元，另涉 1.2亿元。无效日期 2024-13-45。"
	got := ExtractTimeAndAmount("合同.pdf", 7, text)

	count := map[ElementType]int{}
	for _, e := range got {
		if !e.Located {
			t.Errorf("抽取要素必须已定位: %+v", e)
		}
		if e.SourceFile != "合同.pdf" || e.PageNum != 7 {
			t.Errorf("要素缺少原文坐标: %+v", e)
		}
		count[e.Type]++
	}
	if count[ElementTime] < 2 {
		t.Errorf("应抽到 ISO 与中文两类日期，实际 %d", count[ElementTime])
	}
	if count[ElementAmount] < 3 {
		t.Errorf("应抽到千分位/人民币/万元/亿元金额，实际 %d", count[ElementAmount])
	}
	for _, e := range got {
		if e.Type == ElementTime && e.Value == "2024-13-45" {
			t.Error("非法 ISO 日期不应入库（time.Parse 必须拦住）")
		}
	}
}

func TestExtractEmptyText(t *testing.T) {
	got := ExtractTimeAndAmount("x.pdf", 1, "")
	if got == nil {
		t.Error("应返回空切片而非 nil（前端直接遍历）")
	}
	if len(got) != 0 {
		t.Errorf("空文本不应抽出要素: %+v", got)
	}
}

// 金额与日期不得互相吞并：日期里的数字不应被再抽成「元」。
func TestExtractDoesNotInventAmounts(t *testing.T) {
	got := ExtractTimeAndAmount("a.pdf", 1, "2024-03-01 交付")
	for _, e := range got {
		if e.Type == ElementAmount {
			t.Errorf("文本未出现金额单位，不得抽出金额: %+v", e)
		}
	}
}
