package profiles

import "testing"

func TestLoadAllProfiles(t *testing.T) {
	for _, role := range []string{"court", "plaintiff", "defendant", "party"} {
		p, err := Load(".", role)
		if err != nil {
			t.Fatalf("加载 %s 失败: %v", role, err)
		}
		if p.Role != role {
			t.Fatalf("角色不一致: %s", p.Role)
		}
		if len(p.EnabledAgents) == 0 {
			t.Fatalf("%s 未启用任何 Agent", role)
		}
	}
}

func TestCourtDisablesStrategy(t *testing.T) {
	p, err := Load(".", "court")
	if err != nil {
		t.Fatal(err)
	}
	// 法院 Profile 禁用所有策略类插件（开发文档 5.2）。
	for _, tool := range []string{"strategy", "win_eval", "defense_rehearsal"} {
		if p.ToolEnabled(tool) {
			t.Errorf("法院 Profile 应禁用工具: %s", tool)
		}
	}
	if !p.ToolEnabled("aggregator") {
		t.Error("法院 Profile 应允许汇聚器")
	}
}

func TestPartyDisablesAllReasoning(t *testing.T) {
	p, err := Load(".", "party")
	if err != nil {
		t.Fatal(err)
	}
	if p.ToolEnabled("reasoning") || p.ToolEnabled("strategy") {
		t.Error("当事人轻量视图应禁用所有推演工具")
	}
	if !p.AgentEnabled("progress_query") {
		t.Error("当事人视图应启用进度查询")
	}
}

func TestInvalidRole(t *testing.T) {
	if _, err := Load(".", "judge"); err == nil {
		t.Error("非法角色应报错")
	}
}
