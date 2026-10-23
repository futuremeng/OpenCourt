package statemachine

import (
	"testing"

	"github.com/futuremeng/OpenCourt/core/domain"
)

func TestInitialState(t *testing.T) {
	withCite := domain.Fact{Citation: &domain.Citation{SourceFile: "a.pdf", PageNum: 3, Snippet: "片段"}}
	if got := InitialState(withCite, false); got != domain.StateSingleSource {
		t.Errorf("有引用应为单源未补强, got %s", got)
	}
	noSource := domain.Fact{}
	if got := InitialState(noSource, false); got != domain.StateUnsupported {
		t.Errorf("无来源应为无依据, got %s", got)
	}
	if got := InitialState(withCite, true); got != domain.StateOpponentClaim {
		t.Errorf("对方来源应为对方主张未验证, got %s", got)
	}
}

func TestUpgradeRequiresEvidence(t *testing.T) {
	// 规则 1：状态只能靠证据升级，不能靠推理升级。
	f := domain.Fact{FactID: "F1", State: domain.StateOpponentClaim}
	err := ArbitrateUpgrade(UpgradeRequest{
		Fact: f, Target: domain.StateTracedConfirmed,
		EvidenceImported: false, LocatedInSource: true, NoInternalConflict: true,
	})
	if err == nil {
		t.Fatal("未导入新证据时禁止升级到已溯源确认")
	}

	// 核验器无法定位原文 → 拒绝。
	err = ArbitrateUpgrade(UpgradeRequest{
		Fact: f, Target: domain.StateTracedConfirmed,
		EvidenceImported: true, LocatedInSource: false, NoInternalConflict: true,
	})
	if err == nil {
		t.Fatal("核验器定位失败时禁止升级")
	}

	// 存在内部矛盾 → 拒绝。
	err = ArbitrateUpgrade(UpgradeRequest{
		Fact: f, Target: domain.StateTracedConfirmed,
		EvidenceImported: true, LocatedInSource: true, NoInternalConflict: false,
	})
	if err == nil {
		t.Fatal("存在内部矛盾时禁止升级")
	}

	// 唯一合法路径：导入新材料 + 原文定位成功 + 无矛盾。
	updated, err := ApplyUpgrade(UpgradeRequest{
		Fact: f, Target: domain.StateTracedConfirmed,
		EvidenceImported: true, LocatedInSource: true, NoInternalConflict: true,
	})
	if err != nil {
		t.Fatalf("合法升级路径被拒绝: %v", err)
	}
	if updated.State != domain.StateTracedConfirmed {
		t.Fatalf("状态未更新, got %s", updated.State)
	}
}

func TestConflictNotAutoResolved(t *testing.T) {
	// 规则 3：冲突不自动消解。
	c := domain.Conflict{ConflictID: "C1", FactIDs: []string{"F1", "F2"}, ConflictPoint: "说法不一致"}
	if _, err := ResolveConflict(c, false); err == nil {
		t.Fatal("非人工消解冲突必须被拒绝")
	}
	updated, err := ResolveConflict(c, true)
	if err != nil || !updated.IsResolved {
		t.Fatal("人工确认后应可标记已解决")
	}
}

func TestAssumptionTagPropagation(t *testing.T) {
	// 规则 2：假设标签贯穿，推翻后可追溯受影响结论。
	c1 := domain.Conclusion{ConclusionID: "K1", Text: "结论一", AssumptionIDs: []string{"A1"}}
	c2 := domain.Conclusion{ConclusionID: "K2", Text: "结论二"}
	a := RejectAssumption(domain.Assumption{AssumptionID: "A1", Content: "假设合同已送达"}, []domain.Conclusion{c1, c2})
	if len(a.AffectedConclusions) != 1 || a.AffectedConclusions[0] != "K1" {
		t.Fatalf("受影响结论追溯错误: %v", a.AffectedConclusions)
	}
	if !a.IsRejected {
		t.Fatal("假设应标记为已推翻")
	}
}
