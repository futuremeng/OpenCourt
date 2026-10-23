// Package adversary 对抗推演层（开发文档 2.2、3.6、3.7）：
// 对方模拟、立场漂移检测、轨迹监控。三方均可开启，但输入不同。
// 本阶段提供接口与可运行的轨迹监控实现；对方模拟 Agent 依赖模型插件，
// 在阶段三接入。
package adversary

import (
	"fmt"

	"github.com/futuremeng/OpenCourt/core/domain"
)

// OpponentSimulator 对方模拟 Agent 接口：基于本方可见材料推演对方主攻方向。
// 输入严格受可见性标签过滤（仅本方可见的材料不得进入对方模拟）。
type OpponentSimulator interface {
	// Simulate 返回对方可能的攻击点列表（候选，须经门禁过滤）。
	Simulate(visibleFacts []domain.Fact) ([]string, error)
}

// DriftDetector 立场漂移检测（开发文档 3.6）：每轮发言前重新注入本方立场，
// 独立的漂移检测比对本轮发言与历史立场，明显转向立即报警。
type DriftDetector interface {
	// Detect 返回 (是否漂移, 说明)。
	Detect(stance string, utterance string, history []string) (bool, string)
}

// TrajectoryMonitor 辩论轨迹监控（开发文档 3.7）：记录每轮结论变化。
// 轨迹剧烈抖动且缺乏新证据支撑时触发人工复核。
type TrajectoryMonitor struct {
	Rounds []TrajectoryRound
	// JitterThreshold 连续轮次中结论翻转次数达到该值即判定剧烈抖动。
	JitterThreshold int
}

// TrajectoryRound 一轮推演记录。
type TrajectoryRound struct {
	Round          int      `json:"round"`
	ConclusionHash string   `json:"conclusion_hash"` // 结论摘要哈希
	NewEvidenceIDs []string `json:"new_evidence_ids,omitempty"`
	Flipped        bool     `json:"flipped"` // 与上一轮结论方向相反
}

// Record 记录一轮推演。
func (m *TrajectoryMonitor) Record(round int, conclusionHash string, newEvidenceIDs []string) {
	flipped := len(m.Rounds) > 0 && m.Rounds[len(m.Rounds)-1].ConclusionHash != conclusionHash
	m.Rounds = append(m.Rounds, TrajectoryRound{
		Round: round, ConclusionHash: conclusionHash,
		NewEvidenceIDs: newEvidenceIDs, Flipped: flipped,
	})
}

// NeedsHumanReview 判定是否需要触发人工复核：
// 最近窗口内翻转次数达到阈值且这些轮次均无新证据支撑。
func (m *TrajectoryMonitor) NeedsHumanReview() (bool, string) {
	threshold := m.JitterThreshold
	if threshold <= 0 {
		threshold = 3
	}
	flips, noEvidence := 0, 0
	for _, r := range m.Rounds {
		if r.Flipped {
			flips++
			if len(r.NewEvidenceIDs) == 0 {
				noEvidence++
			}
		}
	}
	if flips >= threshold && noEvidence == flips {
		return true, fmt.Sprintf("轨迹剧烈抖动（%d 次翻转且均无新证据支撑），触发人工复核", flips)
	}
	return false, ""
}
