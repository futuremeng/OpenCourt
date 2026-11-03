// 对抗推演与庭审流程绑定（v0.3 M6）。纪律：
//   - 这是模拟推演，不是审判结果预测——所有返回固定携带免责声明；
//   - 阶段推进由 core/flow 确定性硬守卫，不靠提示词；
//   - 对方模拟输入经 Visibility 硬过滤，本方保密材料进不了 prompt；
//   - 轨迹抖动且无新证据 → 停止自动续演，等待人工复核（SimReview）；
//   - 法院 Profile 禁用 defense_rehearsal（默认拒绝）。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/futuremeng/OpenCourt/adversary"
	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/flow"
	"github.com/futuremeng/OpenCourt/core/gate"
	"github.com/futuremeng/OpenCourt/core/storage"
	"github.com/futuremeng/OpenCourt/neutral"
)

// simDisclaimer 固定免责标注（roadmap M6：界面全程显示）。
const simDisclaimer = "这是模拟推演，不是审判结果预测；结论仅供参考，不构成对任何一方的倾向性评价"

// simState 每案推演运行态（内存态：轮次从库重放，监控器重启后重新累计）。
type simState struct {
	monitor   *adversary.TrajectoryMonitor
	seen      map[string]bool
	reviewed  bool
	needRev   bool
	reviewMsg string
}

var (
	simMu     sync.Mutex
	simStates = map[string]*simState{}
)

func getSimState(caseID string) *simState {
	simMu.Lock()
	defer simMu.Unlock()
	st, ok := simStates[caseID]
	if !ok {
		st = &simState{monitor: &adversary.TrajectoryMonitor{JitterThreshold: 3}, seen: map[string]bool{}}
		simStates[caseID] = st
	}
	return st
}

// SimRoundResult 一轮对抗推演结果（含全部拦截与复核信号）。
type SimRoundResult struct {
	RoundID     string             `json:"round_id"`
	Phase       string             `json:"phase"`
	Attacks     []adversary.Attack `json:"attacks"`
	Violations  []gate.Violation   `json:"violations,omitempty"`
	Blocked     bool               `json:"blocked"`
	Drift       bool               `json:"drift"`
	DriftNote   string             `json:"drift_note,omitempty"`
	NeedsReview bool               `json:"needs_review"`
	ReviewNote  string             `json:"review_note,omitempty"`
	Disclaimer  string             `json:"disclaimer"`
}

// refIndexes 本案事实索引 + 引用留痕索引（与 CheckOutput 同一落地口径）。
func (a *App) refIndexes() (gate.FactIndex, gate.RefIndex) {
	index := gate.FactIndex{}
	if facts, err := a.scope().ListFacts(); err == nil {
		for _, f := range facts {
			index[f.FactID] = f
		}
	}
	refs := gate.RefIndex{}
	if laws, err := a.scope().ListLawRefs(); err == nil {
		for _, r := range laws {
			refs[r.RefID] = gate.RefInfo{Kind: "LAW", Status: r.Status,
				ConfirmStatus: r.ConfirmStatus, VerifyResult: r.VerifyResult}
		}
	}
	if cases, err := a.scope().ListCaseRefs(); err == nil {
		for _, r := range cases {
			// 类案引用没有「效力」概念：留痕即溯源凭据，Status 记 UNKNOWN。
			refs[r.RefID] = gate.RefInfo{Kind: "CASE", Status: "UNKNOWN",
				ConfirmStatus: "confirmed", VerifyResult: "OK"}
		}
	}
	return index, refs
}

// RunSimRound 发起一轮推演：阶段守卫 → 发言门禁 → 对方模拟 → 轨迹监控。
// stance 为本方立场（每轮重新注入并做漂移比对）；被拦截轮次也入库留痕，
// 但不计入阶段完成度。
func (a *App) RunSimRound(phase, stance, conclusion string, factIDs []string) (SimRoundResult, error) {
	if !a.profile.ToolEnabled("defense_rehearsal") {
		return SimRoundResult{}, fmt.Errorf("当前立场（%s）已禁用对抗推演（defense_rehearsal）", a.profile.Role)
	}
	p := flow.Phase(strings.ToUpper(strings.TrimSpace(phase)))
	if !p.Valid() {
		return SimRoundResult{}, fmt.Errorf("非法庭审阶段: %q（须为 STATEMENT/EVIDENCE/DEBATE/FINAL）", phase)
	}
	sc := a.scope()
	rounds, err := sc.ListSimRounds()
	if err != nil {
		return SimRoundResult{}, err
	}
	done := map[flow.Phase]int{}
	history := []string{}
	for _, r := range rounds {
		if !r.Blocked {
			done[flow.Phase(r.Phase)]++
			history = append(history, r.Text)
		}
	}
	facts := mustFacts(sc)
	if err := flow.CheckPhaseEntry(p, done, facts); err != nil {
		_ = a.auditLog.Log(a.profile.Role, "SIM_GUARD_BLOCK", string(p), err.Error())
		return SimRoundResult{}, err
	}
	st := getSimState(a.caseID())
	simMu.Lock()
	held, msg := st.needRev && !st.reviewed, st.reviewMsg
	simMu.Unlock()
	if held {
		return SimRoundResult{}, fmt.Errorf("轨迹抖动待人工复核，自动续演已停止：%s（人工确认请调用 SimReview）", msg)
	}

	index, refs := a.refIndexes()
	violations := flow.CheckUtterance(conclusion, factIDs, index, refs)
	drift, driftNote := adversary.LexicalDrift{}.Detect(stance, conclusion, history)
	if drift {
		_ = a.auditLog.Log(a.profile.Role, "SIM_DRIFT", string(p), driftNote)
	}
	roundID := fmt.Sprintf("SIM-%s-%03d", p, len(rounds)+1)
	if len(violations) > 0 {
		if err := sc.AddSimRound(storage.SimRound{
			RoundID: roundID, Phase: string(p), Role: a.profile.Role,
			Text: conclusion, FactIDs: factIDs, Blocked: true,
		}); err != nil {
			return SimRoundResult{}, err
		}
		_ = a.auditLog.Log(a.profile.Role, "SIM_ROUND_BLOCK", roundID,
			fmt.Sprintf("%s 阶段发言被门禁拦截 %d 条", p, len(violations)))
		return SimRoundResult{RoundID: roundID, Phase: string(p), Violations: violations,
			Blocked: true, Drift: drift, DriftNote: driftNote, Disclaimer: simDisclaimer}, nil
	}

	a.mu.RLock()
	model := a.model
	a.mu.RUnlock()
	sim := &adversary.Simulator{Model: model, Index: index, Refs: refs}
	attacks, err := sim.SimulateRound(a.ctx, a.profile.Role, facts)
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "SIM_FAIL", roundID, err.Error())
		return SimRoundResult{}, err
	}
	if err := sc.AddSimRound(storage.SimRound{
		RoundID: roundID, Phase: string(p), Role: a.profile.Role,
		Text: conclusion, FactIDs: factIDs,
	}); err != nil {
		return SimRoundResult{}, err
	}
	_ = a.auditLog.Log(a.profile.Role, "SIM_ROUND", roundID,
		fmt.Sprintf("%s 阶段有效轮次，攻击点候选 %d 条（拦截 %d 条）漂移=%v",
			p, len(attacks), countBlocked(attacks), drift))

	newEvidence := []string{}
	simMu.Lock()
	for _, id := range factIDs {
		if !st.seen[id] {
			st.seen[id] = true
			newEvidence = append(newEvidence, id)
		}
	}
	st.monitor.Record(len(rounds)+1, hashText(conclusion), newEvidence)
	need, note := st.monitor.NeedsHumanReview()
	st.needRev, st.reviewMsg = need, note
	if st.reviewed {
		st.reviewed = false // 复核只对当次抖动生效，新抖动再停
	}
	simMu.Unlock()

	return SimRoundResult{RoundID: roundID, Phase: string(p), Attacks: attacks,
		Drift: drift, DriftNote: driftNote, NeedsReview: need, ReviewNote: note,
		Disclaimer: simDisclaimer}, nil
}

// SimReview 人工复核确认轨迹抖动（恢复自动续演的唯一出口）。
func (a *App) SimReview() error {
	st := getSimState(a.caseID())
	simMu.Lock()
	st.reviewed = true
	simMu.Unlock()
	return a.auditLog.Log(a.profile.Role, "SIM_REVIEW", a.caseID(), "人工复核确认轨迹抖动，恢复续演")
}

// SimStatus 推演面板状态：阶段完成度、待复核标志、模型可用性。
func (a *App) SimStatus() (map[string]any, error) {
	sc := a.scope()
	rounds, err := sc.ListSimRounds()
	if err != nil {
		return nil, err
	}
	done := map[string]int{}
	for _, r := range rounds {
		if !r.Blocked {
			done[r.Phase]++
		}
	}
	st := getSimState(a.caseID())
	simMu.Lock()
	need, msg := st.needRev, st.reviewMsg
	simMu.Unlock()
	a.mu.RLock()
	modelOK := a.model != nil && a.model.Enabled()
	a.mu.RUnlock()
	return map[string]any{
		"rounds":         len(rounds),
		"done":           done,
		"role":           a.profile.Role,
		"tool_enabled":   a.profile.ToolEnabled("defense_rehearsal"),
		"model_enabled":  modelOK,
		"needs_review":   need,
		"review_note":    msg,
		"evidence_ready": flow.HasEvidenceBackedFact(mustFacts(sc)),
		"disclaimer":     simDisclaimer,
	}, nil
}

// ListSimRounds 本案推演轮次（时间正序，含被拦截轮——拦截必须可追溯）。
func (a *App) ListSimRounds() ([]storage.SimRound, error) {
	return a.scope().ListSimRounds()
}

// SummarizeCaseIssues 本案争点归纳：事实按要件分组 + 冲突并列 + 证据缺口。
// 要件清单取本案已导入要素（封闭集，不自动扩）；纯离线确定性代码，不调模型。
func (a *App) SummarizeCaseIssues() (neutral.IssueReport, error) {
	sc := a.scope()
	facts, err := sc.ListFacts()
	if err != nil {
		return neutral.IssueReport{}, err
	}
	conflicts, err := sc.ListConflicts()
	if err != nil {
		return neutral.IssueReport{}, err
	}
	elems, err := sc.ListElements()
	if err != nil {
		return neutral.IssueReport{}, err
	}
	seen := map[string]bool{}
	reqs := []string{}
	for _, e := range elems {
		v := strings.TrimSpace(e.Value)
		if v != "" && !seen[v] {
			seen[v] = true
			reqs = append(reqs, v)
		}
	}
	return neutral.SummarizeIssues(facts, conflicts, neutral.MatchElements(facts, reqs)), nil
}

func mustFacts(sc *storage.CaseScope) []domain.Fact {
	facts, err := sc.ListFacts()
	if err != nil {
		return nil
	}
	return facts
}

func countBlocked(attacks []adversary.Attack) int {
	n := 0
	for _, a := range attacks {
		if a.Blocked {
			n++
		}
	}
	return n
}

func hashText(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}
