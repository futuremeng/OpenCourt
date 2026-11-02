package domain

import (
	"encoding/json"
	"testing"
	"time"
)

// Citation.Valid 是「无引用即无事实」的底线：必须同时有卷宗、页码（>=1）与原文片段。
func TestCitationValid(t *testing.T) {
	ok := Citation{SourceFile: "a.pdf", PageNum: 1, Snippet: "原文"}
	if !ok.Valid() {
		t.Error("完整引用应判定有效")
	}
	for name, c := range map[string]Citation{
		"无来源":  {PageNum: 1, Snippet: "x"},
		"无页码":  {SourceFile: "a.pdf", PageNum: 0, Snippet: "x"},
		"页码为负": {SourceFile: "a.pdf", PageNum: -1, Snippet: "x"},
		"无片段":  {SourceFile: "a.pdf", PageNum: 1},
	} {
		if c.Valid() {
			t.Errorf("%s 的引用必须判定无效", name)
		}
	}
}

// 五态是封闭集合：枚举、显示名与解析必须自洽（前端按状态名分列展示）。
func TestFactStateRoundTrip(t *testing.T) {
	states := AllStates()
	if len(states) != 5 {
		t.Fatalf("事实五态应为 5 个，实际 %d", len(states))
	}
	seen := map[string]bool{}
	for _, s := range states {
		if !s.Valid() {
			t.Errorf("%s 应为合法状态", s)
		}
		if seen[string(s)] {
			t.Errorf("状态重复: %s", s)
		}
		seen[string(s)] = true
		if got, err := ParseFactState(string(s)); err != nil || got != s {
			t.Errorf("按枚举码解析 %s 失败: %v %v", s, got, err)
		}
		if got, err := ParseFactState(s.DisplayName()); err != nil || got != s {
			t.Errorf("按中文名解析 %s 失败: %v %v", s.DisplayName(), got, err)
		}
	}
	if s, err := ParseFactState("已确认无误"); err == nil || s != "" {
		t.Errorf("非法状态必须报错且返回空值: %q %v", s, err)
	}
	if FactState("GUESSED").Valid() {
		t.Error("枚举外的状态不得判定合法")
	}
	if got := FactState("X").DisplayName(); got != "未知状态" {
		t.Errorf("未知状态显示名应为占位提示，实际 %q", got)
	}
}

func TestCaseStageValid(t *testing.T) {
	for _, s := range []CaseStage{StageConsult, StageFiling, StageFirstTrial, StageAppeal, StageSecondTrial} {
		if !s.Valid() {
			t.Errorf("阶段 %s 应合法", s)
		}
	}
	if CaseStage("EXECUTION").Valid() {
		t.Error("枚举外阶段必须判非法（案件状态层是封闭集合）")
	}
}

// 结论一旦继承假设，就必须带「基于假设」标签——这是状态机规则 2 的数据前提。
func TestConclusionBasedOnAssumption(t *testing.T) {
	if (Conclusion{ConclusionID: "C1", Text: "t"}).BasedOnAssumption() {
		t.Error("未挂假设的结论不应带假设标签")
	}
	c := Conclusion{ConclusionID: "C2", Text: "t", AssumptionIDs: []string{"A1"}}
	if !c.BasedOnAssumption() {
		t.Error("挂了假设的结论必须带假设标签")
	}
}

// Fact 的 JSON 键即前端绑定契约（wailsjs models.ts 用小写蛇形），改名即破前端。
func TestFactJSONContract(t *testing.T) {
	b, err := json.Marshal(Fact{
		FactID:      "F1",
		Description: "描述",
		State:       StateSingleSource,
		Citation:    &Citation{SourceFile: "a.pdf", PageNum: 3, Snippet: "片段"},
		Visibility:  VisBoth,
		UpdatedAt:   time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"fact_id", "description", "state", "citation", "visibility", "is_manual_confirmed", "updated_at"} {
		if _, ok := m[key]; !ok {
			t.Errorf("Fact JSON 缺少键 %q（前端绑定契约）: %s", key, b)
		}
	}
	// Citation 无 json tag（透传 Go 字段名），前端模板必须用 SourceFile 形式取值。
	cit, _ := m["citation"].(map[string]any)
	if _, ok := cit["SourceFile"]; !ok {
		t.Errorf("Citation JSON 键应为 Go 字段名（前端已按此取值）: %v", m["citation"])
	}
	if m["state"] != "SINGLE_SOURCE" {
		t.Errorf("state 应为枚举码，实际 %v", m["state"])
	}
}
