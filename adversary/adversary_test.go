package adversary

import (
	"testing"

	"github.com/futuremeng/OpenCourt/core/domain"
)

// 轨迹监控：结论翻转且无新证据支撑才触发人工复核——有新证据的翻转是正常推进，
// 不得报警（否则人工复核会被噪声淹没）。
func TestTrajectoryNeedsHumanReview(t *testing.T) {
	m := &TrajectoryMonitor{}
	m.Record(1, "h1", nil)
	for i, h := range []string{"h2", "h1", "h2"} {
		m.Record(i+2, h, nil)
	}
	ok, reason := m.NeedsHumanReview()
	if !ok {
		t.Error("3 次无证据翻转应触发人工复核")
	}
	if reason == "" {
		t.Error("触发时必须给出理由")
	}

	withEvidence := &TrajectoryMonitor{}
	withEvidence.Record(1, "h1", nil)
	for i, h := range []string{"h2", "h1", "h2"} {
		withEvidence.Record(i+2, h, []string{"EV-1"})
	}
	if ok, _ := withEvidence.NeedsHumanReview(); ok {
		t.Error("翻转均有新证据，不应触发人工复核")
	}

	stable := &TrajectoryMonitor{}
	for i := 1; i <= 4; i++ {
		stable.Record(i, "h1", nil)
	}
	if ok, _ := stable.NeedsHumanReview(); ok {
		t.Error("结论稳定不应触发人工复核")
	}

	custom := &TrajectoryMonitor{JitterThreshold: 2}
	custom.Record(1, "h1", nil)
	custom.Record(2, "h2", nil)
	custom.Record(3, "h1", nil)
	if ok, _ := custom.NeedsHumanReview(); !ok {
		t.Error("自定义阈值 2 应被遵守（2 次翻转即触发）")
	}
	below := &TrajectoryMonitor{JitterThreshold: 2}
	below.Record(1, "h1", nil)
	below.Record(2, "h2", nil)
	if ok, _ := below.NeedsHumanReview(); ok {
		t.Error("翻转次数未达阈值不应触发")
	}
}

// 首轮无上一轮可比，不得判为翻转。
func TestTrajectoryFirstRoundNotFlipped(t *testing.T) {
	m := &TrajectoryMonitor{}
	m.Record(1, "h1", nil)
	if m.Rounds[0].Flipped {
		t.Error("首轮不应判为翻转")
	}
	m.Record(2, "h1", nil)
	if m.Rounds[1].Flipped {
		t.Error("结论未变不应判为翻转")
	}
	if m.Rounds[1].Round != 2 {
		t.Errorf("轮次应如实记录: %+v", m.Rounds[1])
	}
}

// 接口存在性守卫：对方模拟与漂移检测的契约不得被无声改掉
// （阶段三接入实现时，签名漂移会在这里暴露）。
func TestInterfacesSatisfied(t *testing.T) {
	var _ OpponentSimulator = stubSimulator{}
	var _ DriftDetector = stubDetector{}
}

type stubSimulator struct{}

func (stubSimulator) Simulate(visibleFacts []domain.Fact) ([]string, error) {
	return nil, nil
}

type stubDetector struct{}

func (stubDetector) Detect(stance, utterance string, history []string) (bool, string) {
	return false, ""
}
