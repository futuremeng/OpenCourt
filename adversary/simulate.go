// M6 对方模拟（开发文档 2.2、3.6）：模型只基于「对方视角可见」的材料
// 生成攻击点候选。可见性过滤发生在进 prompt 之前——本方保密材料
// （仅本方可见标签）在数据层就进不了对方模拟，不是靠 UI 隐藏。
// 每条攻击点都是候选：必须带 fact_ids 且过 gate，违规条目 Blocked 待改写。
package adversary

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/gate"
	"github.com/futuremeng/OpenCourt/plugins"
)

// ErrModelDisabled 模型未启用：对方模拟必须显式失败——绝不用模板/规则
// 冒充「AI 模拟对方」，那是把生成内容伪装成推演结论。
var ErrModelDisabled = errors.New("模型未启用：对抗推演不可用")

// ViewFor 返回 role 视角可见的事实集合（Visibility 硬过滤）。
// 对侧角色模拟用本方对手视角：plaintiff 的对手看 BOTH+DEFENDANT_ONLY，
// defendant 的对手看 BOTH+PLAINTIFF_ONLY；COURT_ONLY 对任何一方模拟都不可见。
func ViewFor(role string, facts []domain.Fact) []domain.Fact {
	opponentVis := domain.VisDefendantOnly
	if role == "defendant" {
		opponentVis = domain.VisPlaintiffOnly
	}
	view := make([]domain.Fact, 0, len(facts))
	for _, f := range facts {
		if f.Visibility == domain.VisBoth || f.Visibility == opponentVis {
			view = append(view, f)
		}
	}
	return view
}

// Attack 一条对方攻击点候选（须经门禁；Blocked 即不得展示为可用攻击点）。
type Attack struct {
	Text       string           `json:"text"`
	FactIDs    []string         `json:"fact_ids,omitempty"`
	Violations []gate.Violation `json:"violations,omitempty"`
	Blocked    bool             `json:"blocked"`
}

// Simulator 模型驱动的对方模拟 Agent。
type Simulator struct {
	Model plugins.ModelAdapter
	Index gate.FactIndex // 本案事实索引（门禁落地检查用）
	Refs  gate.RefIndex  // 引用留痕索引（可空）
}

// Simulate 实现 OpponentSimulator 接口：返回通过门禁的攻击点文本。
func (s *Simulator) Simulate(visibleFacts []domain.Fact) ([]string, error) {
	attacks, err := s.generate(context.Background(), visibleFacts)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, a := range attacks {
		if !a.Blocked {
			out = append(out, a.Text)
		}
	}
	return out, nil
}

// SimulateRound 一轮对方模拟：可见性过滤 → 模型生成 → 逐条门禁。
// 返回全部候选（含 Blocked 条目——拦截必须可见，不得静默丢弃）。
func (s *Simulator) SimulateRound(ctx context.Context, role string, facts []domain.Fact) ([]Attack, error) {
	return s.generate(ctx, ViewFor(role, facts))
}

func (s *Simulator) generate(ctx context.Context, view []domain.Fact) ([]Attack, error) {
	if s.Model == nil || !s.Model.Enabled() {
		return nil, ErrModelDisabled
	}
	var sb strings.Builder
	sb.WriteString("你是庭审对抗推演中的对方代理人。仅基于下列对方可见的卷宗事实，" +
		"提出你方最可能的攻击点。每条一行，格式：FACT_ID|攻击点内容。" +
		"不得虚构事实，不得预测判决结果。\n")
	for _, f := range view {
		fmt.Fprintf(&sb, "%s: %s\n", f.FactID, f.Description)
	}
	raw, err := s.Model.Generate(ctx, sb.String())
	if err != nil {
		return nil, fmt.Errorf("对方模拟生成失败: %w", err)
	}
	attacks := []Attack{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		a := Attack{}
		if id, rest, ok := strings.Cut(line, "|"); ok {
			id = strings.TrimSpace(id)
			if strings.HasPrefix(id, "FACT-") {
				a.FactIDs = []string{id}
			}
			a.Text = strings.TrimSpace(rest)
		} else {
			a.Text = line
		}
		a.Violations = append(a.Violations, checkAttack(a, s.Index, s.Refs)...)
		a.Blocked = len(a.Violations) > 0
		attacks = append(attacks, a)
	}
	return attacks, nil
}

// checkAttack 攻击点门禁：无 fact_ids 即幻觉（发言必须可落地），
// 再走引用落地 + 倾向性检测。
func checkAttack(a Attack, index gate.FactIndex, refs gate.RefIndex) []gate.Violation {
	if len(a.FactIDs) == 0 {
		return []gate.Violation{{
			Rule:    "NO_FACT_REF",
			Phrase:  a.Text,
			Message: "攻击点未引用任何卷宗事实：无法落地的模拟发言直接拦截",
		}}
	}
	return gate.Check(gate.OutputDraft{Text: a.Text, FactIDs: a.FactIDs}, index, refs)
}
