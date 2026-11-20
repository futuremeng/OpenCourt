package profiles

import (
	"encoding/json"
	"strings"
	"testing"
)

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

// TestCourtGuideFourSteps 法院场景包（v0.4 P1）：court Profile 声明四步
// 引导动线，动作标识不得重复（前端按标识映射落点）。
func TestCourtGuideFourSteps(t *testing.T) {
	p, err := Load(".", "court")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Guide) != 4 {
		t.Fatalf("court 引导应为 4 步，实得 %d", len(p.Guide))
	}
	seen := map[string]bool{}
	for i, s := range p.Guide {
		if s.Title == "" || s.Text == "" || s.Action == "" {
			t.Errorf("引导第 %d 步字段不全: %+v", i+1, s)
		}
		if seen[s.Action] {
			t.Errorf("引导动作重复: %s", s.Action)
		}
		seen[s.Action] = true
	}
}

// TestGuideJSONKeys 引导步骤经 Wails 绑定到前端时的序列化契约：
// 前端向导按 title/text/action 小写键读取；缺 json 标签会序列化成
// Title/Text/Action，向导四步全部渲染成空壳且状态永不完成（GUI 实测暴露）。
func TestGuideJSONKeys(t *testing.T) {
	p, err := Load(".", "court")
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(p.Guide)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"title":`, `"text":`, `"action":`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("Guide JSON 缺小写键 %s: %s", want, b)
		}
	}
	for _, bad := range []string{`"Title":`, `"Text":`, `"Action":`} {
		if strings.Contains(string(b), bad) {
			t.Errorf("Guide JSON 不应含大写字段 %s: %s", bad, b)
		}
	}
}

// TestOtherProfilesNoGuide 其余三个 Profile 保持自由动线（plan-v0.4 D4）：
// 引导是数据，不是代码分支；未声明即无向导。
func TestOtherProfilesNoGuide(t *testing.T) {
	for _, role := range []string{"plaintiff", "defendant", "party"} {
		p, err := Load(".", role)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Guide) != 0 {
			t.Errorf("%s 不应声明引导动线，实得 %d 步", role, len(p.Guide))
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

// TestEmbeddedFallback 磁盘缺目录（双击 .app 工作目录非仓库）→ 内置兜底。
func TestEmbeddedFallback(t *testing.T) {
	for _, role := range []string{"court", "plaintiff", "defendant", "party"} {
		p, err := Load(t.TempDir(), role)
		if err != nil {
			t.Fatalf("内置兜底失败 %s: %v", role, err)
		}
		if p.Role != role {
			t.Errorf("内置 %s Role 异常: %+v", role, p)
		}
	}
}
