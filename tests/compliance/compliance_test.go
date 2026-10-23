// Package compliance 合规测试（开发文档 7.6、12.3）：
// 验证脱敏、溯源、门禁等安全约束。对应合规检查清单中的：
//   - 溯源测试：注入虚假事实 → 系统拒绝输出或标记「无依据」
//   - 脱敏测试：导入含敏感信息文档 → 导出/展示内容中敏感字段已处理
//   - 引导性检测：倾向性表述必须被拦截
package compliance

import (
	"strings"
	"testing"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/gate"
	"github.com/futuremeng/OpenCourt/core/redact"
	"github.com/futuremeng/OpenCourt/core/statemachine"
	"github.com/futuremeng/OpenCourt/core/storage"
)

// 溯源测试：模型注入一个卷宗中不存在的事实并尝试引用，门禁必须拦截。
func TestFalseFactInjectionBlocked(t *testing.T) {
	index := gate.FactIndex{} // 空卷宗：任何引用都落不了地
	out := gate.OutputDraft{
		Text:    "被告于 2024 年 5 月 1 日收到催款函。",
		FactIDs: []string{"FACT-HALLUCINATED-001"},
	}
	violations := gate.CheckGrounding(out, index)
	if len(violations) == 0 {
		t.Fatal("注入虚假事实必须被拦截")
	}
	if violations[0].Rule != "HALLUCINATION" {
		t.Fatalf("拦截规则错误: %s", violations[0].Rule)
	}
}

// 状态机合规：仅凭模型推理（无新证据）不得将对方主张升格为已溯源确认。
func TestNoInferenceUpgrade(t *testing.T) {
	f := domain.Fact{FactID: "F1", State: domain.StateOpponentClaim}
	_, err := statemachine.ApplyUpgrade(statemachine.UpgradeRequest{
		Fact: f, Target: domain.StateTracedConfirmed,
		EvidenceImported: false, LocatedInSource: false, NoInternalConflict: true,
	})
	if err == nil {
		t.Fatal("无证据的状态升格必须被拒绝")
	}
}

// 脱敏测试：含敏感信息的文档经脱敏后，敏感字段不得出现在展示层。
func TestSensitiveFieldsRedacted(t *testing.T) {
	r := redact.New([]string{"张三"})
	doc := "当事人张三（身份证 110101199001011234，工资卡 6222020200112345678）" +
		"居住于北京市朝阳区朝阳路88号院。案号 (2024)京01民初1号。"
	res := r.RedactText(doc)
	for _, secret := range []string{"张三", "199001011234", "0200112345678", "朝阳路88号"} {
		if strings.Contains(res.Text, secret) {
			t.Errorf("敏感字段泄漏: %s -> %s", secret, res.Text)
		}
	}
	if !strings.Contains(res.Text, "(2024)京01民初1号") {
		t.Error("案号应保留")
	}
}

// 引导性检测：任何输出不得包含倾向性结论。
func TestNoGuidanceInOutputs(t *testing.T) {
	outputs := []string{
		"本院认为应当调解。",
		"评估认为原告大概率胜诉，建议尽快和解。",
		"根据类案，胜诉率超过 90%。",
	}
	for _, o := range outputs {
		if v := gate.CheckGuidance(o); len(v) == 0 {
			t.Errorf("倾向性输出未被拦截: %s", o)
		}
	}
}

// 冲突不自动消解：即使入库后，is_resolved 也必须保持 false。
func TestConflictsRemainUnresolved(t *testing.T) {
	s, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := domain.Conflict{ConflictID: "C1", FactIDs: []string{"F1", "F2"}, ConflictPoint: "两份合同金额不一致"}
	if err := s.CreateConflict(c); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListConflicts()
	if len(list) != 1 || list[0].IsResolved {
		t.Fatal("冲突入库后必须保持未消解状态")
	}
	if _, err := statemachine.ResolveConflict(list[0], false); err == nil {
		t.Fatal("自动消解冲突必须被拒绝")
	}
}
