package gate

import (
	"testing"

	"github.com/futuremeng/OpenCourt/core/domain"
)

func TestGuidanceBlocked(t *testing.T) {
	for _, text := range []string{
		"综合来看建议起诉。",
		"本案大概率胜诉。",
		"应当调解结案。",
		"胜诉率约为百分之八十。",
		// D6（plan-v0.2）：分布统计不得被换算成概率化表述。
		"这类案子我方胜算不小。",
		"胜诉大概有几成？",
		"法院判支持的可能性多大？",
		"该类案件法院倾向判决赔偿。",
	} {
		if v := CheckGuidance(text); len(v) == 0 {
			t.Errorf("引导性表述未被拦截: %s", text)
		}
	}
	if v := CheckGuidance("合同于 2024 年 3 月 1 日签订，卷宗第 5 页。"); len(v) != 0 {
		t.Errorf("中立陈述被误拦截: %v", v)
	}
}

// 组合规则（E0 对抗集暴露的间隔写法绕过）：「建议」+诉讼决策词同现即拦截；
// 补证方向等程序辅助建议与不带「建议」的叙述必须放行。
func TestGuidancePairRule(t *testing.T) {
	for _, text := range []string{
		"建议对被告提起诉讼追讨欠款。",
		"如协商不成，建议通过仲裁解决争议。",
		"证据不足时不建议贸然上诉。",
	} {
		if v := CheckGuidance(text); len(v) == 0 {
			t.Errorf("「建议」+决策词组合未被拦截: %s", text)
		}
	}
	for _, text := range []string{
		"补证方向建议：补充送货单与验收记录。",
		"原告于2023年6月20日提起诉讼。",
		"建议补充鉴定意见以补强交付事实。",
		// 构词排除：决策词+名词化后缀 = 中性法律术语，不是决策引导。
		"建议核实诉讼时效期间是否经过。",
		"建议补充仲裁协议签署过程的证据。",
		"建议提交诉讼费用缴纳凭证。",
		// 句子级同现：跨句叙述不误伤。
		"以上为补证建议。本案涉及诉讼时效争议。",
	} {
		if v := CheckGuidance(text); len(v) != 0 {
			t.Errorf("程序辅助/叙述性表述被误拦截: %s → %v", text, v)
		}
	}
}

func TestHallucinationGate(t *testing.T) {
	index := FactIndex{
		"F1": {FactID: "F1", State: domain.StateTracedConfirmed},
		"F2": {FactID: "F2", State: domain.StateUnsupported},
		"F3": {FactID: "F3", State: domain.StateOpponentClaim},
	}
	// 引用不存在的事实 → 拦截（注入虚假事实）。
	out := OutputDraft{Text: "陈述", FactIDs: []string{"F9"}}
	if v := CheckGrounding(out, index, RefIndex{}); len(v) == 0 || v[0].Rule != "HALLUCINATION" {
		t.Fatal("引用不存在的事实应被拦截")
	}
	// 引用无依据事实 → 拦截。
	out = OutputDraft{Text: "陈述", FactIDs: []string{"F2"}}
	if v := CheckGrounding(out, index, RefIndex{}); len(v) == 0 {
		t.Fatal("引用无依据事实应被拦截")
	}
	// 对方主张混入事实段落 → 拦截。
	out = OutputDraft{Text: "陈述", FactIDs: []string{"F3"}}
	if v := CheckGrounding(out, index, RefIndex{}); len(v) == 0 || v[0].Rule != "OPPONENT_CLAIM_IN_FACT" {
		t.Fatal("对方主张出现在事实段落应被拦截")
	}
	// 引用缺少页码/原文 → 拦截。
	out = OutputDraft{Text: "陈述", Citations: []domain.Citation{{SourceFile: "a.pdf"}}}
	if v := CheckGrounding(out, index, RefIndex{}); len(v) == 0 || v[0].Rule != "CITATION_INVALID" {
		t.Fatal("无效引用应被拦截")
	}
	// 已废止法条 → 标红提示。
	out = OutputDraft{Text: "陈述", Citations: []domain.Citation{
		{SourceFile: "a.pdf", PageNum: 1, Snippet: "片段", LawRepealed: true},
	}}
	if v := CheckGrounding(out, index, RefIndex{}); len(v) == 0 || v[0].Rule != "LAW_REPEALED" {
		t.Fatal("已废止法条应触发提示")
	}
}

func TestCheckCombined(t *testing.T) {
	index := FactIndex{"F1": {FactID: "F1", State: domain.StateTracedConfirmed}}
	out := OutputDraft{Text: "建议起诉", FactIDs: []string{"F1"}}
	v := Check(out, index, RefIndex{})
	if len(v) != 1 || v[0].Rule != "NO_GUIDANCE" {
		t.Fatalf("组合门禁结果错误: %+v", v)
	}
}
