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
	"time"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/gate"
	"github.com/futuremeng/OpenCourt/core/ingest"
	"github.com/futuremeng/OpenCourt/core/redact"
	"github.com/futuremeng/OpenCourt/core/statemachine"
	"github.com/futuremeng/OpenCourt/core/storage"
	"github.com/futuremeng/OpenCourt/neutral"
	"github.com/futuremeng/OpenCourt/plugins"
)

// 溯源测试：模型注入一个卷宗中不存在的事实并尝试引用，门禁必须拦截。
func TestFalseFactInjectionBlocked(t *testing.T) {
	index := gate.FactIndex{} // 空卷宗：任何引用都落不了地
	out := gate.OutputDraft{
		Text:    "被告于 2024 年 5 月 1 日收到催款函。",
		FactIDs: []string{"FACT-HALLUCINATED-001"},
	}
	violations := gate.CheckGrounding(out, index, gate.RefIndex{})
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
	sc := s.WithCase(domain.DefaultCaseID)
	c := domain.Conflict{ConflictID: "C1", FactIDs: []string{"F1", "F2"}, ConflictPoint: "两份合同金额不一致"}
	if err := sc.CreateConflict(c); err != nil {
		t.Fatal(err)
	}
	list, _ := sc.ListConflicts()
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
	report, err := neutral.RunReader(context.Background(), "DOC-1", doc, fakeModel{out: modelOut}, nil)
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
	sc := s.WithCase(domain.DefaultCaseID)
	for _, f := range report.Accepted {
		if err := sc.CreateFact(f); err != nil {
			t.Fatal(err)
		}
	}
	if err := sc.SaveElements(report.Elements); err != nil {
		t.Fatal(err)
	}
	facts, _ := sc.ListFacts()
	elements, _ := sc.ListElements()
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

// --- 网络查阅（BrowserOS 辅助）合规约束 ---

// webStub 网络查阅测试桩：模拟浏览器插件返回含 PII 的原始页面。
type webStub struct{ enabled bool }

func (s webStub) Name() string  { return "stub" }
func (s webStub) Enabled() bool { return s.enabled }
func (s webStub) Open(ctx context.Context, url string) (plugins.BrowserPage, error) {
	if !s.enabled {
		return plugins.BrowserPage{}, errors.New("插件未启用")
	}
	return plugins.BrowserPage{PageID: 1, URL: url}, nil
}
func (s webStub) ReadPage(ctx context.Context, pageID int) (plugins.BrowserPageContent, error) {
	md := "原告王五（身份证 110101199003078888）向上海市浦东新区人民法院提起诉讼。"
	return plugins.BrowserPageContent{URL: "https://court.example/filing", Markdown: md, ContentSHA: "x"}, nil
}
func (s webStub) SearchPage(ctx context.Context, pageID int, q string) ([]string, error) {
	return nil, nil
}
func (s webStub) ClosePage(ctx context.Context, pageID int) error { return nil }
func (s webStub) Snapshot(ctx context.Context, pageID int) (string, error) {
	return "", errors.New("stub 不支持")
}
func (s webStub) Evaluate(ctx context.Context, pageID int, code string) (string, error) {
	return "", errors.New("stub 不支持")
}
func (s webStub) ProposeFill(ctx context.Context, pageID int, fields []plugins.FormFieldSpec) ([]plugins.ActionCandidate, error) {
	return nil, errors.New("stub 不支持")
}
func (s webStub) ProposeClick(ctx context.Context, pageID int, ref, label string) (plugins.ActionCandidate, error) {
	return plugins.ActionCandidate{}, errors.New("stub 不支持")
}
func (s webStub) ExecuteAction(ctx context.Context, actionID string) (plugins.ActionReceipt, error) {
	return plugins.ActionReceipt{}, errors.New("stub 不支持")
}
func (s webStub) HandToHuman(ctx context.Context, pageID int, kind, reason, resumeHint string) (plugins.HumanHelpStatus, error) {
	return plugins.HumanHelpStatus{}, errors.New("stub 不支持")
}
func (s webStub) AwaitHuman(ctx context.Context) (plugins.HumanHelpStatus, error) {
	return plugins.HumanHelpStatus{}, errors.New("stub 不支持")
}

// 脱敏测试（网络源）：网页抓回的敏感信息，进入模型/UI 前必须已脱敏。
func TestWebContentRedactedBeforeModel(t *testing.T) {
	res, err := neutral.RunResearch(context.Background(), webStub{enabled: true}, []string{"王五"}, "https://court.example/filing")
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"王五", "110101199003078888"} {
		if strings.Contains(res.Text, leak) {
			t.Fatalf("联网内容未经脱敏流入模型入口: %s", leak)
		}
	}
	if res.Fact.State != domain.StateSingleSource {
		t.Fatalf("联网事实必须为弱态（单源未补强），实际 %s", res.Fact.State)
	}
}

// 默认离线测试：浏览器插件未启用时网络查阅必须直接报错，绝不降级抓取。
func TestWebResearchDisabledByDefault(t *testing.T) {
	if _, err := neutral.RunResearch(context.Background(), webStub{enabled: false}, nil, "https://x"); err == nil {
		t.Fatal("未启用必须报错")
	}
	if _, err := neutral.RunResearch(context.Background(), nil, nil, "https://x"); err == nil {
		t.Fatal("未配置必须报错")
	}
}

// --- v0.2 合规用例（plan-v0.2 §7）---

// 合规用例 1：跨案件引用必须被拦截。存储层隔离（B 案索引装不进 A 案实体）
// 与门禁校验（索引查不到即幻觉）双重防线，任何一层失守另一层兜底。
func TestCrossCaseReferenceBlocked(t *testing.T) {
	dir := t.TempDir()
	st, err := storage.Open(filepath.Join(dir, "case.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, c := range []domain.Case{
		{CaseID: "CASE-A", Title: "甲案", Stage: domain.StageConsult, Visibility: domain.VisBoth},
		{CaseID: "CASE-B", Title: "乙案", Stage: domain.StageConsult, Visibility: domain.VisBoth},
	} {
		if err := st.CreateCase(c); err != nil {
			t.Fatal(err)
		}
	}
	sa, sb := st.WithCase("CASE-A"), st.WithCase("CASE-B")
	if err := sa.CreateFact(domain.Fact{FactID: "FACT-A1", Description: "甲案事实", State: domain.StateSingleSource, Visibility: domain.VisBoth}); err != nil {
		t.Fatal(err)
	}
	if err := sa.AddLawRef(domain.LawRef{LawName: "民法典", ArticleNo: "第五百七十七条", VerifyResult: "OK", Status: "UNKNOWN", ConfirmStatus: "confirmed"}); err != nil {
		t.Fatal(err)
	}

	// 存储层：乙案视野内没有甲案的事实与法条留痕。
	factsB, err := sb.ListFacts()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range factsB {
		if f.FactID == "FACT-A1" {
			t.Fatal("存储层隔离失守：乙案读到了甲案事实")
		}
	}
	refsB, err := sb.ListLawRefs()
	if err != nil {
		t.Fatal(err)
	}
	if len(refsB) != 0 {
		t.Fatalf("存储层隔离失守：乙案读到了 %d 条甲案法条留痕", len(refsB))
	}

	// 门禁层：即便模型把甲案 ID 塞进乙案输出，也必须按幻觉拦截。
	index := gate.FactIndex{}
	for _, f := range factsB {
		index[f.FactID] = f
	}
	out := gate.OutputDraft{Text: "参照甲案事实认定。", FactIDs: []string{"FACT-A1"}}
	v := gate.CheckGrounding(out, index, gate.RefIndex{})
	if len(v) == 0 || v[0].Rule != "HALLUCINATION" {
		t.Fatalf("跨案件事实引用必须拦截: %+v", v)
	}
	out = gate.OutputDraft{Text: "援引甲案法条。", FactIDs: []string{refsA0(t, sa)}}
	v = gate.CheckGrounding(out, index, gate.RefIndex{})
	if len(v) == 0 || v[0].Rule != "HALLUCINATION" {
		t.Fatalf("跨案件法条引用必须拦截: %+v", v)
	}
}

// refsA0 取甲案首条法条留痕 ID。
func refsA0(t *testing.T, sc *storage.CaseScope) string {
	t.Helper()
	refs, err := sc.ListLawRefs()
	if err != nil || len(refs) == 0 {
		t.Fatal("甲案法条留痕缺失")
	}
	return refs[0].RefID
}

// complianceVerifier 合规测试用假核验器：按预置判定透传（模拟 OpenLaw 六分支）。
type complianceVerifier struct {
	verdicts map[string]plugins.CitationVerdict
}

func (v complianceVerifier) CheckCitation(_ context.Context, lawName, articleNo, _, _ string) (plugins.CitationVerdict, error) {
	if d, ok := v.verdicts[lawName+" "+articleNo]; ok {
		return d, nil
	}
	return plugins.CitationVerdict{VerifyResult: "OL-1001", Status: "UNKNOWN", ConfirmStatus: "pending"}, nil
}

func (v complianceVerifier) FindRelation(_ context.Context, _ string) ([]plugins.Relation, error) {
	return nil, nil
}

// 合规用例 3：未核验/断链的法条引用必须显式上浮——留痕带 OL 码入库，
// 门禁对 UNVERIFIED/pending 引用强制违规，禁止静默通过。
func TestLawRefUnverifiedSurfaces(t *testing.T) {
	cv := complianceVerifier{verdicts: map[string]plugins.CitationVerdict{
		"民法典 第五百七十七条": {VerifyResult: "OK", Status: "UNKNOWN", ConfirmStatus: "confirmed", ArticleID: "art-ok"},
		"民法典 第九百九十九条": {VerifyResult: "OL-1003", Status: "UNVERIFIED", ConfirmStatus: "pending"},
		"劳动合同法 第八十七条": {VerifyResult: "OL-1006", Status: "UNVERIFIED", ConfirmStatus: "pending", ArticleID: "art-broken"},
	}}
	refs, err := neutral.VerifyCitations(context.Background(), cv, "2024-01-01", []neutral.LawCitationDraft{
		{LawName: "民法典", ArticleNo: "第五百七十七条"},
		{LawName: "民法典", ArticleNo: "第九百九十九条"},
		{LawName: "劳动合同法", ArticleNo: "第八十七条"},
	})
	if err != nil {
		t.Fatal(err)
	}
	refsIndex := gate.RefIndex{}
	for i := range refs {
		r := refs[i]
		r.RefID = "LAW-" + r.VerifyResult
		refsIndex[r.RefID] = gate.RefInfo{Kind: "LAW", Status: r.Status, ConfirmStatus: r.ConfirmStatus, VerifyResult: r.VerifyResult}
	}
	for _, id := range []string{"LAW-OL-1003", "LAW-OL-1006"} {
		out := gate.OutputDraft{FactIDs: []string{id}}
		v := gate.CheckGrounding(out, gate.FactIndex{}, refsIndex)
		if len(v) == 0 || v[0].Rule != "REF_UNVERIFIED" {
			t.Fatalf("未核验引用 %s 必须违规上浮: %+v", id, v)
		}
	}
	out := gate.OutputDraft{FactIDs: []string{"LAW-OK"}}
	if v := gate.CheckGrounding(out, gate.FactIndex{}, refsIndex); len(v) != 0 {
		t.Fatalf("已核验引用不应被拦截: %+v", v)
	}
}

// ---------- v0.3 M5 混合检索合规 ----------

// embedStub 确定性向量适配器（合规用例只验纪律，不验召回质量）。
type embedStub struct{}

func (embedStub) Name() string  { return "stub" }
func (embedStub) Enabled() bool { return true }
func (embedStub) Generate(context.Context, string) (string, error) {
	return "", plugins.ErrNotSupported
}
func (embedStub) Embed(_ context.Context, text string) ([]float64, error) {
	if strings.Contains(text, "违约") {
		return []float64{1, 0}, nil
	}
	return []float64{0, 1}, nil
}

// 检索命中只是候选：跑完整索引+检索管道后，事实表必须一字未动，
// 状态机与五态不得被检索结果触碰（roadmap M5 验收「检索命中不能直接成为强态事实」）。
func TestHybridSearchNeverWritesFacts(t *testing.T) {
	dir := t.TempDir()
	st, err := storage.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	sc := st.WithCase(domain.DefaultCaseID)
	if err := sc.CreateFact(domain.Fact{
		FactID: "F1", Description: "既有事实", State: domain.StateUnsupported,
	}); err != nil {
		t.Fatal(err)
	}
	pages := []neutral.PageText{
		{DocID: "d1", Page: 1, Text: "合同约定违约金为合同金额的百分之二十"},
	}
	if _, _, err := neutral.IndexCasePages(context.Background(), embedStub{}, "m", pages, sc); err != nil {
		t.Fatal(err)
	}
	rows, err := sc.ListEmbeddings("m")
	if err != nil {
		t.Fatal(err)
	}
	vecs := map[string][]float64{}
	for _, r := range rows {
		vecs[neutral.PageRef(r.DocID, r.Page)] = r.Vec
	}
	rep, err := neutral.SearchCaseDocs(context.Background(), embedStub{}, "m", pages, vecs, "违约金", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Hits) == 0 {
		t.Fatal("命中为空说明管道断了")
	}
	facts, err := sc.ListFacts()
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 || facts[0].FactID != "F1" || facts[0].State != domain.StateUnsupported {
		t.Errorf("检索必须零写事实、零改状态: %+v", facts)
	}
}

// 向量按案件隔离：跨案件向量不得进入本案召回（可见性在存储层前置过滤，
// 与 roadmap M1「先过滤再召回，不靠 UI 隐藏」同一纪律）。
func TestEmbeddingsCaseIsolated(t *testing.T) {
	dir := t.TempDir()
	st, err := storage.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.CreateCase(domain.Case{CaseID: "CASE-A", Title: "A 案"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateCase(domain.Case{CaseID: "CASE-B", Title: "B 案"}); err != nil {
		t.Fatal(err)
	}
	a, b := st.WithCase("CASE-A"), st.WithCase("CASE-B")
	if err := a.UpsertEmbedding("da", 1, "m", "h", []float64{1, 0}); err != nil {
		t.Fatal(err)
	}
	rows, err := b.ListEmbeddings("m")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("B 案不得见 A 案向量: %+v", rows)
	}
	rep, err := neutral.SearchCaseDocs(context.Background(), embedStub{}, "m",
		[]neutral.PageText{{DocID: "db", Page: 1, Text: "与 A 案同词的文本"}}, nil, "违约金", 5)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Mode != neutral.SearchModeKeywordOnly {
		t.Errorf("无本案向量必须降级关键词模式，不得借他案向量: %+v", rep)
	}
}

// 模型未启用必须显式降级标注——离线默认值下链路可用但绝不冒充语义召回。
func TestHybridSearchDegradesExplicitly(t *testing.T) {
	rep, err := neutral.SearchCaseDocs(context.Background(), nil, "",
		[]neutral.PageText{{DocID: "d1", Page: 1, Text: "违约金条款"}}, nil, "违约金", 5)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Mode != neutral.SearchModeKeywordOnly || !strings.Contains(rep.Note, "未启用语义召回") {
		t.Errorf("离线默认必须显式标注降级: %+v", rep)
	}
}

// OCR 纪律（plan-v0.4 D1/O1）：
//   - 扫描件绝不绕过 OCR 直接入库（引擎缺失 = 明确拒绝）；
//   - OCR 文本入库上限弱态（状态机拒绝进入已溯源确认）；
//   - 引擎不可用不得静默降级到其他引擎。
func TestOCRCompliance(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "scan.png")
	if err := os.WriteFile(img, []byte("fake-png"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 1) 引擎缺失：图片必须被拒绝，错误必须指向 OCR。
	r := ingest.NewRegistry()
	if _, err := r.Parse(img); err == nil || !strings.Contains(err.Error(), "OCR") {
		t.Fatalf("未配置引擎时扫描件必须明确拒绝: %v", err)
	}
	// 2) 指定引擎不可用：维持拒绝，不降级。
	if eng, note := ingest.DetectOCR(ingest.OCRConfig{
		Engine: "tesseract", Path: filepath.Join(dir, "missing-bin"),
	}); eng != nil || note == "" {
		t.Errorf("引擎不可用必须维持拒绝并说明: %v %q", eng, note)
	}
	// 3) OCR 文本封顶弱态：三条件全满足也进不了强态。
	err := statemachine.ArbitrateUpgrade(statemachine.UpgradeRequest{
		Fact:               domain.Fact{FactID: "F1", State: domain.StateSingleSource},
		Target:             domain.StateTracedConfirmed,
		EvidenceImported:   true,
		LocatedInSource:    true,
		NoInternalConflict: true,
		SourceFromOCR:      true,
	})
	if err == nil {
		t.Fatal("OCR 来源事实禁止进入已溯源确认")
	}
}

// TestReportCompliance 报告导出合规（plan-v0.4 M7/D3）：弱态事实出现在
// 报告事实段落即失败；免责头部固定；报告全文必过门禁。
func TestReportCompliance(t *testing.T) {
	in := neutral.ReportInput{
		CaseTitle: "合规案", CaseID: "CASE-C", GeneratedAt: time.Unix(0, 0).UTC(),
		Facts: []domain.Fact{
			{FactID: "F1", Description: "强态：合同已签订", State: domain.StateTracedConfirmed,
				Citation: &domain.Citation{SourceFile: "a.txt", PageNum: 1, Snippet: "签订"}},
			{FactID: "F2", Description: "弱态：单方陈述的交付", State: domain.StateSingleSource},
			{FactID: "F3", Description: "对方主张：已还款", State: domain.StateOpponentClaim},
		},
	}
	md := neutral.RenderMarkdown(in)
	factSection := md[strings.Index(md, "## 一、事实清单"):strings.Index(md, "## 二、")]
	for _, weak := range []string{"单方陈述的交付", "已还款"} {
		if strings.Contains(factSection, weak) {
			t.Fatalf("弱态事实进入报告事实段落（合规违例）：%s", weak)
		}
	}
	if !strings.Contains(md, neutral.Disclaimer) {
		t.Fatal("报告免责头部缺失")
	}
	// 报告也是输出：拼装文本含倾向性表述必须被 gate 拦截。
	tainted := md + "\n综上，被告必然败诉。\n"
	if v := gate.CheckGuidance(tainted); len(v) == 0 {
		t.Fatal("报告层门禁未拦截倾向性表述")
	}
	if v := gate.CheckGuidance(md); len(v) != 0 {
		t.Fatalf("合规报告文本不应被门禁误拦: %+v", v)
	}
}
