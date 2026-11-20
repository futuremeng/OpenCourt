// v0.4 P13：升级进强态的三条件由后端确定性重算（design-flow case-journey
// 批准项）——前端声明只能收紧不能放宽。生产入口 App.UpgradeFact 必须
// 回原文定位引用、核实来源文书仍在本案、检查未解决冲突；绕开录入
// 直接写库的坏数据也进不了强态。
package main

import (
	"strings"
	"testing"

	"github.com/futuremeng/OpenCourt/core/domain"
)

func TestUpgradeFactBackendDeterministic(t *testing.T) {
	a := newCaseApp(t, t.TempDir())
	if _, err := a.CreateCase("确定性复核案", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := a.scope().SaveDocument("DOC-N", "/x/native.pdf",
		map[int]string{1: "原生文本层原文"}); err != nil {
		t.Fatal(err)
	}

	// 1) 正常链路（录入定位过→升级）不得被重算误伤。
	fOK, err := a.CreateFact("原生事实", "/x/native.pdf", 1, "原生文本层原文", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.UpgradeFact(fOK.FactID, string(domain.StateTracedConfirmed), true, true, true); err != nil {
		t.Fatalf("引用可定位的正常升级不得被拒: %v", err)
	}

	// 2) 绕开 CreateFact 直接入库的坏引用（片段与原文对不上）：定位重算必须拒。
	fBad := domain.Fact{
		FactID: "FACT-BAD-CITE", Description: "串改引用", Visibility: domain.VisBoth,
		Citation: &domain.Citation{SourceFile: "/x/native.pdf", PageNum: 1, Snippet: "原文里不存在的句子"},
		State:    domain.StateSingleSource,
	}
	if err := a.scope().CreateFact(fBad); err != nil {
		t.Fatal(err)
	}
	if _, err := a.UpgradeFact(fBad.FactID, string(domain.StateTracedConfirmed), true, true, true); err == nil ||
		!strings.Contains(err.Error(), "找不到对应句子") {
		t.Fatalf("定位不上的引用升级必须被拒并说明原因: %v", err)
	}
	if got, _ := a.scope().GetFact(fBad.FactID); got.State != domain.StateSingleSource {
		t.Errorf("被拒后状态不得变化: %s", got.State)
	}

	// 3) 无来源事实（无引用）：按缺证据拒绝，补救指向阅卷。
	fNone, err := a.CreateFact("无来源事实", "", 0, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.UpgradeFact(fNone.FactID, string(domain.StateTracedConfirmed), true, true, true); err == nil ||
		!strings.Contains(err.Error(), "先在「阅卷」") {
		t.Fatalf("无证据升级必须被拒并指向阅卷接入: %v", err)
	}

	// 4) 涉及该事实的未解决冲突（存储层要求至少涉及两份事实）：
	// 并列展示、人工判断，不得进强态。人工消解后的复升路径当前无存储层
	// 更新接口（ResolveConflict 仅纯函数），本测试不覆盖，已在报告如实标注。
	fFresh, err := a.CreateFact("冲突下的事实", "/x/native.pdf", 1, "原生文本层原文", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.scope().CreateConflict(domain.Conflict{ConflictID: "C-P13",
		FactIDs: []string{fFresh.FactID, fOK.FactID}, ConflictPoint: "交付时间不一致"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.UpgradeFact(fFresh.FactID, string(domain.StateTracedConfirmed), true, true, true); err == nil ||
		!strings.Contains(err.Error(), "互相矛盾") {
		t.Fatalf("未解决冲突涉及该事实时升级必须被拒: %v", err)
	}
	if got, _ := a.scope().GetFact(fFresh.FactID); got.State != domain.StateSingleSource {
		t.Errorf("冲突拒绝后状态不得变化: %s", got.State)
	}
}
