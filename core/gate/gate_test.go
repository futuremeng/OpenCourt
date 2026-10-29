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
	} {
		if v := CheckGuidance(text); len(v) == 0 {
			t.Errorf("引导性表述未被拦截: %s", text)
		}
	}
	if v := CheckGuidance("合同于 2024 年 3 月 1 日签订，卷宗第 5 页。"); len(v) != 0 {
		t.Errorf("中立陈述被误拦截: %v", v)
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
