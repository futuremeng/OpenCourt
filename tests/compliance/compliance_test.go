// Package compliance 合规测试（开发文档 7.6、12.3）：
// 验证脱敏、溯源、门禁等安全约束。对应合规检查清单中的：
//   - 溯源测试：注入虚假事实 → 系统拒绝输出或标记「无依据」
//   - 脱敏测试：导入含敏感信息文档 → 导出/展示内容中敏感字段已处理
//   - 引导性检测：倾向性表述必须被拦截
package compliance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/gate"
	"github.com/futuremeng/OpenCourt/core/ingest"
	"github.com/futuremeng/OpenCourt/core/redact"
	"github.com/futuremeng/OpenCourt/core/statemachine"
	"github.com/futuremeng/OpenCourt/core/storage"
	"github.com/futuremeng/OpenCourt/neutral"
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

// fakeModel 端到端测试用模型：只产候选，无裁决权。
type fakeModel struct{ out string }

func (f fakeModel) Name() string  { return "fake" }
func (f fakeModel) Enabled() bool { return true }
func (f fakeModel) Embed(_ context.Context, _ string) ([]float64, error) {
	return nil, errors.New("unsupported")
}
func (f fakeModel) Generate(_ context.Context, _ string) (string, error) { return f.out, nil }

// 阶段二验收链路：文档接入 → 阅卷（含幻觉候选）→ 核验器仲裁 →
// 时间线 → 要件匹配 → 盲区。幻觉候选必须被拒绝入库。
func TestPhase2ReaderPipeline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "judgment.txt")
	text := "2023年5月10日，原告向被告支付定金50000元。\f被告未按期交货，双方对交货时间表述不一致。\f"
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}

	doc, err := ingest.NewRegistry().Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Pages) != 3 {
		t.Fatalf("分页错误: %d", len(doc.Pages))
	}

	modelOut := `[
	 {"description":"原告已付定金","snippet":"原告向被告支付定金50000元","page_num":1},
	 {"description":"捏造：被告曾书面承诺赔偿","snippet":"被告书面承诺赔偿三倍定金","page_num":2},
	 {"description":"对方主张已发货","snippet":"被告未按期交货","page_num":2,"from_opponent":true}
	]`
	report, err := neutral.RunReader(context.Background(), "DOC-1", doc, fakeModel{out: modelOut})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Accepted) != 2 || len(report.Rejected) != 1 {
		t.Fatalf("仲裁结果错误: accepted=%d rejected=%d", len(report.Accepted), len(report.Rejected))
	}
	if report.Rejected[0].Description != "捏造：被告曾书面承诺赔偿" {
		t.Errorf("被拒绝的必须是幻觉候选: %+v", report.Rejected)
	}

	// 事实入库后：时间线锚定 + 要件匹配 + 盲区。
	s, err := storage.Open(filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, f := range report.Accepted {
		if err := s.CreateFact(f); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SaveElements(report.Elements); err != nil {
		t.Fatal(err)
	}
	facts, _ := s.ListFacts()
	elements, _ := s.ListElements()
	tl := neutral.BuildTimeline(facts, elements)
	if len(tl) != 2 {
		t.Fatalf("时间线长度错误: %d", len(tl))
	}

	// 阅卷产生的事实均为弱态（单源/对方主张），不能覆盖要件——
	// 要件匹配必须报缺，盲区必须给出补证方向，绝不幻觉填补。
	match := neutral.MatchElements(facts, []string{"合同成立", "违约行为"})
	if len(match.Covered) != 0 || len(match.Missing) != 2 {
		t.Errorf("弱态事实不得覆盖要件: %+v", match)
	}
	blind := neutral.GenerateBlindspots(match)
	if len(blind) != 2 || !strings.Contains(blind[0].Supplement, "补证方向") {
		t.Errorf("盲区输出错误: %+v", blind)
	}
}
