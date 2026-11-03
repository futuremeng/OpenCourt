package compliance

import (
	"context"
	"strings"
	"testing"

	"github.com/futuremeng/OpenCourt/adversary"
	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/flow"
	"github.com/futuremeng/OpenCourt/core/gate"
	"github.com/futuremeng/OpenCourt/neutral"
	"github.com/futuremeng/OpenCourt/plugins"
	"github.com/futuremeng/OpenCourt/profiles"
)

// ---------- v0.3 M6 对抗推演与庭审流程合规（roadmap「合规测试先行」）----------

// 对方模拟输入严格受可见性过滤：本方保密材料（仅本方可见标签）不得
// 进入对方模拟——可见性在进 prompt 前过滤，不是 UI 隐藏。
func TestOpponentSimExcludesOwnConfidential(t *testing.T) {
	facts := []domain.Fact{
		{FactID: "FACT-1", Description: "双方签订借款合同", Visibility: domain.VisBoth},
		{FactID: "FACT-2", Description: "本方和解底线为十二万元", Visibility: domain.VisPlaintiffOnly},
		{FactID: "FACT-3", Description: "对方声称已现金清偿", Visibility: domain.VisDefendantOnly},
	}
	view := adversary.ViewFor("plaintiff", facts)
	for _, f := range view {
		if f.FactID == "FACT-2" {
			t.Error("仅原告可见的本方材料不得进入原告方视角的对方（被告）模拟")
		}
	}
	var captured string
	sim := &adversary.Simulator{
		Model: &captureEmbed{enabled: true, capture: &captured, reply: "FACT-1|合同真实性无异议"},
		Index: gate.FactIndex{},
	}
	if _, err := sim.SimulateRound(context.Background(), "plaintiff", facts); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(captured, "十二万") || strings.Contains(captured, "FACT-2") {
		t.Errorf("对方模拟 prompt 泄漏本方保密材料: %s", captured)
	}
	if !strings.Contains(captured, "FACT-1") {
		t.Errorf("对方模拟应基于双方可见材料: %s", captured)
	}
}

// captureEmbed 捕获 prompt 的模型适配器（只验纪律不验生成质量）。
type captureEmbed struct {
	enabled bool
	capture *string
	reply   string
	err     error
}

func (m *captureEmbed) Name() string  { return "capture" }
func (m *captureEmbed) Enabled() bool { return m.enabled }
func (m *captureEmbed) Generate(_ context.Context, prompt string) (string, error) {
	if m.capture != nil {
		*m.capture = prompt
	}
	return m.reply, m.err
}
func (m *captureEmbed) Embed(context.Context, string) ([]float64, error) {
	return nil, plugins.ErrNotSupported
}

// 每轮发言必须带 fact_ids：空引用即拦截；引用「无依据」事实同样拦截。
func TestSimUtteranceGrounding(t *testing.T) {
	index := gate.FactIndex{
		"FACT-1": {FactID: "FACT-1", State: domain.StateTracedConfirmed},
		"FACT-9": {FactID: "FACT-9", State: domain.StateUnsupported},
	}
	if v := flow.CheckUtterance("本方陈述", nil, index, nil); len(v) == 0 {
		t.Error("不带 fact_ids 的发言必须被拦截（无来源即无发言）")
	}
	if v := flow.CheckUtterance("本方陈述", []string{"FACT-9"}, index, nil); len(v) == 0 {
		t.Error("引用「无依据」事实的发言必须被拦截")
	}
	if v := flow.CheckUtterance("本方陈述", []string{"FACT-1"}, index, nil); len(v) != 0 {
		t.Errorf("合规发言不应被拦截: %+v", v)
	}
}

// 倾向性发言必须被拦截并要求改写：对方攻击点里出现引导/预测表述即 Blocked。
func TestSimAttacksGuidanceBlocked(t *testing.T) {
	var captured string
	sim := &adversary.Simulator{
		Model: &captureEmbed{enabled: true, capture: &captured,
			reply: "FACT-1|对方大概率胜诉，建议撤诉"},
		Index: gate.FactIndex{"FACT-1": {FactID: "FACT-1", State: domain.StateSingleSource}},
	}
	attacks, err := sim.SimulateRound(context.Background(), "plaintiff",
		[]domain.Fact{{FactID: "FACT-1", Description: "双方签订合同", Visibility: domain.VisBoth}})
	if err != nil {
		t.Fatal(err)
	}
	if len(attacks) != 1 || !attacks[0].Blocked {
		t.Fatalf("倾向性攻击点必须被拦截待改写: %+v", attacks)
	}
}

// 流程硬守卫（确定性代码，非提示词）：无证据事实不进质证、质证未完成
// 不进辩论、辩论未完成不进最后陈述；阶段不得跳跃。
func TestFlowHardGuards(t *testing.T) {
	facts := []domain.Fact{
		{FactID: "FACT-1", State: domain.StateUnsupported},
	}
	done := map[flow.Phase]int{flow.PhaseStatement: 1}
	if err := flow.AdvanceGuard(flow.PhaseStatement, done, facts); err == nil {
		t.Error("事实库只有「无依据」状态时不得进入质证")
	}
	facts[0].State = domain.StateSingleSource
	if err := flow.AdvanceGuard(flow.PhaseStatement, done, facts); err != nil {
		t.Errorf("有证据支撑事实且陈述完成应可进质证: %v", err)
	}
	doneE := map[flow.Phase]int{flow.PhaseStatement: 1}
	if err := flow.AdvanceGuard(flow.PhaseEvidence, doneE, facts); err == nil {
		t.Error("质证未完成不得进入法庭辩论")
	}
	doneE[flow.PhaseEvidence] = 1
	if err := flow.AdvanceGuard(flow.PhaseEvidence, doneE, facts); err != nil {
		t.Errorf("质证完成应可进辩论: %v", err)
	}
	if err := flow.AdvanceGuard(flow.PhaseDebate, doneE, facts); err == nil {
		t.Error("辩论未完成不得进入最后陈述")
	}
	if err := flow.AdvanceGuard(flow.PhaseFinal, doneE, facts); err == nil {
		t.Error("最后陈述即终点，不得再推进")
	}
}

// 轨迹抖动且无新证据 → 必须触发人工复核（自动续演停止的判据）。
func TestTrajectoryJitterRequiresReview(t *testing.T) {
	m := &adversary.TrajectoryMonitor{JitterThreshold: 3}
	m.Record(1, "A", nil)
	m.Record(2, "B", nil) // 翻转、无新证据
	m.Record(3, "A", nil) // 再翻转
	m.Record(4, "B", nil) // 三度翻转仍无新证据
	need, note := m.NeedsHumanReview()
	if !need || note == "" {
		t.Errorf("连续翻转且均无新证据必须触发人工复核: %v %q", need, note)
	}
	m2 := &adversary.TrajectoryMonitor{JitterThreshold: 3}
	m2.Record(1, "A", nil)
	m2.Record(2, "B", []string{"FACT-NEW"}) // 有新增证据的立场变化不触发
	m2.Record(3, "A", []string{"FACT-NEW2"})
	if need, _ := m2.NeedsHumanReview(); need {
		t.Error("有新证据支撑的立场变化不应触发人工复核")
	}
}

// 立场漂移：本轮发言与本方立场极性相反 → 立即报警。
func TestStanceDriftAlarm(t *testing.T) {
	d := adversary.LexicalDrift{}
	stance := "本方立场：违约金约定过高，应当调减"
	if drift, _ := d.Detect(stance, "我方认为违约金约定合理，无需调整", nil); !drift {
		t.Error("立场极性翻转必须报警")
	}
	if drift, _ := d.Detect(stance, "补充提交转账凭证一份", nil); drift {
		t.Error("与立场无关的材料提交不应误报漂移")
	}
}

// 争点归纳保持中立：冲突必须并列展示双方事实引用，不得出现倾向性表述。
func TestIssueSummaryNeutral(t *testing.T) {
	facts := []domain.Fact{
		{FactID: "FACT-1", Description: "已付款", State: domain.StateTracedConfirmed, RelatedElement: "付款义务"},
		{FactID: "FACT-2", Description: "对方称已现金清偿", State: domain.StateOpponentClaim, RelatedElement: "付款义务"},
	}
	conflicts := []domain.Conflict{{
		ConflictID: "C-1", FactIDs: []string{"FACT-1", "FACT-2"},
		ConflictPoint: "是否已清偿", IsResolved: false,
	}}
	rep := neutral.SummarizeIssues(facts, conflicts,
		neutral.MatchResult{Covered: []string{"付款义务"}, Missing: []string{"催告"}})
	if len(rep.Issues) == 0 {
		t.Fatal("争点为空说明归纳断了")
	}
	for _, i := range rep.Issues {
		if len(i.ConflictPoints) == 0 {
			t.Errorf("未消解冲突必须在争点上并列展示: %+v", i)
		}
		if v := gate.CheckGuidance(i.Element + strings.Join(i.ConflictPoints, "")); len(v) != 0 {
			t.Errorf("争点归纳出现倾向性表述: %+v", v)
		}
	}
	if len(rep.Gaps) != 1 || rep.Gaps[0] != "催告" {
		t.Errorf("证据缺口必须直接列出缺失要件: %+v", rep.Gaps)
	}
}

// 法院 Profile 必须禁用对抗推演类工具（默认拒绝在绑定层生效）。
func TestCourtProfileDisablesRehearsal(t *testing.T) {
	p, err := profiles.Load("../../profiles", "court")
	if err != nil {
		t.Fatal(err)
	}
	if p.ToolEnabled("defense_rehearsal") || p.ToolEnabled("win_eval") || p.ToolEnabled("verdict_tendency") {
		t.Error("法院 Profile 必须禁用策略/胜算/裁判倾向工具")
	}
	if !p.ToolEnabled("aggregator") {
		t.Error("法院 Profile 的中立能力不得误禁")
	}
}
