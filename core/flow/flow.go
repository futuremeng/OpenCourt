// Package flow 庭审流程引擎（v0.3 M6，开发文档 3.6、roadmap-next M6）：
// 阶段推进由确定性代码硬守卫，不靠提示词约定——
// 无证据事实不进质证、质证未完成不进辩论、辩论未完成不进最后陈述。
// 本包是纯函数层：不触存储、不调模型，阶段与轮次由调用方（App 绑定）持久化。
package flow

import (
	"errors"
	"fmt"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/gate"
)

// Phase 庭审阶段（法庭调查-陈述 → 质证 → 法庭辩论 → 最后陈述）。
type Phase string

const (
	PhaseStatement Phase = "STATEMENT" // 法庭调查：当事人陈述
	PhaseEvidence  Phase = "EVIDENCE"  // 法庭调查：举证质证
	PhaseDebate    Phase = "DEBATE"    // 法庭辩论
	PhaseFinal     Phase = "FINAL"     // 最后陈述
)

// order 阶段固定顺序：任何跳跃都必须被 AdvanceGuard 拒绝。
var order = []Phase{PhaseStatement, PhaseEvidence, PhaseDebate, PhaseFinal}

// ErrSkipPhase 试图跳跃阶段的错误 sentinel（前端据此提示先完成当前阶段）。
var ErrSkipPhase = errors.New("阶段不得跳跃")

// Valid 报告是否为合法阶段码。
func (p Phase) Valid() bool {
	for _, x := range order {
		if x == p {
			return true
		}
	}
	return false
}

// Next 返回下一阶段及其是否存在（FINAL 为终点）。
func (p Phase) Next() (Phase, bool) {
	for i, x := range order {
		if x == p && i+1 < len(order) {
			return order[i+1], true
		}
	}
	return "", false
}

// HasEvidenceBackedFact 事实库中是否存在「有证据支撑」状态的事实
// （已溯源确认或单源未补强）——进入质证的最低门槛。「无依据」「对方主张」
// 「存在冲突」均不算：证据缺位时质证就是空转（roadmap 硬守卫）。
func HasEvidenceBackedFact(facts []domain.Fact) bool {
	for _, f := range facts {
		if f.State == domain.StateTracedConfirmed || f.State == domain.StateSingleSource {
			return true
		}
	}
	return false
}

// AdvanceGuard 硬守卫：报告阶段 p 能否推进到下一阶段。
// done 为各阶段已完成的有效轮次数（被门禁拦截的轮次不得计数）。
func AdvanceGuard(p Phase, done map[Phase]int, facts []domain.Fact) error {
	if !p.Valid() {
		return fmt.Errorf("非法阶段: %q", p)
	}
	next, ok := p.Next()
	if !ok {
		return fmt.Errorf("阶段 %s 已是终点，不得再推进", p)
	}
	switch p {
	case PhaseStatement:
		if done[p] < 1 {
			return fmt.Errorf("当事人陈述阶段尚无有效轮次，不得进入质证")
		}
		if !HasEvidenceBackedFact(facts) {
			return fmt.Errorf("事实库没有任何「有证据支撑」状态的事实：无证据不进质证（当前 %d 条事实均无证据或仅对方主张）", len(facts))
		}
	case PhaseEvidence:
		if done[p] < 1 {
			return fmt.Errorf("质证阶段尚无有效轮次：质证未完成不得进入法庭辩论")
		}
	case PhaseDebate:
		if done[p] < 1 {
			return fmt.Errorf("辩论阶段尚无有效轮次：辩论未完成不得进入最后陈述")
		}
	}
	_ = next
	return nil
}

// CheckPhaseEntry 报告能否在阶段 p 发起新轮次：p 之前所有阶段都须有
// 有效轮次且各自守卫通过（跳跃进入后续阶段一律拒绝）。
func CheckPhaseEntry(p Phase, done map[Phase]int, facts []domain.Fact) error {
	if !p.Valid() {
		return fmt.Errorf("非法阶段: %q", p)
	}
	if p == PhaseStatement {
		return nil
	}
	cur := PhaseStatement
	for cur != p {
		if done[cur] < 1 {
			return fmt.Errorf("%w：阶段 %s 尚未完成（0 轮），不能进入 %s", ErrSkipPhase, cur, p)
		}
		if err := AdvanceGuard(cur, done, facts); err != nil {
			return fmt.Errorf("%w：%s", ErrSkipPhase, err)
		}
		nx, _ := cur.Next()
		cur = nx
	}
	return nil
}

// CheckUtterance 每轮发言门禁：必须带 fact_ids（无来源即无发言），
// 再走幻觉门禁（引用不存在/「无依据」事实即拦截）与倾向性检测。
func CheckUtterance(text string, factIDs []string, index gate.FactIndex, refs gate.RefIndex) []gate.Violation {
	if len(factIDs) == 0 {
		return []gate.Violation{{
			Rule:    "NO_FACT_REF",
			Phrase:  "",
			Message: "推演发言必须携带 fact_ids 指向卷宗事实：不带来源的发言直接拦截",
		}}
	}
	out := gate.OutputDraft{Text: text, FactIDs: factIDs}
	return gate.Check(out, index, refs)
}
