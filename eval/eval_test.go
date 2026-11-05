package eval

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/futuremeng/OpenCourt/plugins"
)

// ---- 测试替身：与生产同一 RunReader/verify 路径，只替换模型与取文 ----

type fakeModel struct {
	out     string
	enabled bool
}

func (m fakeModel) Name() string                                     { return "fake-model" }
func (m fakeModel) Enabled() bool                                    { return m.enabled }
func (m fakeModel) Generate(context.Context, string) (string, error) { return m.out, nil }
func (m fakeModel) Embed(context.Context, string) ([]float64, error) {
	return nil, plugins.ErrNotSupported
}

type fakeFetch struct {
	text string
	err  error
}

func (f fakeFetch) GetCaseText(context.Context, int64) (plugins.CaseTextDoc, error) {
	if f.err != nil {
		return plugins.CaseTextDoc{}, f.err
	}
	return plugins.CaseTextDoc{ID: 42, Text: f.text, Source: "localcases"}, nil
}

type fakeProbe struct{ digest string }

func (p fakeProbe) Health(context.Context) (plugins.LocalCasesHealth, error) {
	return plugins.LocalCasesHealth{OK: true, IndexedDocuments: 137, CorpusDigest: p.digest}, nil
}

const sampleText = "2023年5月10日，原告甲向被告乙支付定金50000元。双方签订《设备买卖合同》。"

// 三个候选：1 条可定位、1 条幻觉片段、1 条引导性表述（门禁拦截）。
const fakeCandidates = `[
 {"description":"甲向乙支付了定金","snippet":"向被告乙支付定金50000元","page_num":1},
 {"description":"双方口头约定了违约金","snippet":"口头约定违约金百分之二十","page_num":1},
 {"description":"建议起诉追回定金","snippet":"支付定金50000元","page_num":1}
]`

// ---- 门禁对抗集 ----

func TestGateAdversarialCleanSweep(t *testing.T) {
	samples, err := LoadGateSamples()
	if err != nil {
		t.Fatalf("加载对抗集: %v", err)
	}
	res := RunGateAdversarial(samples)
	if len(res.Missed) > 0 {
		t.Errorf("存在漏拦（合规红线）: %+v", res.Missed)
	}
	if len(res.FalseBlock) > 0 {
		t.Errorf("存在误拦（可用性）: %+v", res.FalseBlock)
	}
	if res.MustBlock < 8 || res.MustPass < 6 {
		t.Errorf("对抗集样本量不足: block=%d pass=%d", res.MustBlock, res.MustPass)
	}
}

// ---- manifest ----

func TestLoadManifest(t *testing.T) {
	in := "{\"id\":1,\"query\":\"cause=民间借贷纠纷&sort=id\",\"sha256\":\"" +
		strings.Repeat("a", 64) + "\"}\n{\"id\":2}\n"
	ms, err := LoadManifest(strings.NewReader(in))
	if err != nil || len(ms) != 2 {
		t.Fatalf("合法清单应通过: %v %v", ms, err)
	}
	if _, err := LoadManifest(strings.NewReader("{\"id\":1}\n{\"id\":1}")); err == nil {
		t.Error("重复 id 必须报错——评测集必须是确定集合")
	}
	if _, err := LoadManifest(strings.NewReader("{\"id\":1,\"sha256\":\"zz\"}")); err == nil {
		t.Error("非法 sha256 必须报错")
	}
	if ms, err := LoadManifest(strings.NewReader("")); err != nil || len(ms) != 0 {
		t.Errorf("空清单应合法: %v %v", ms, err)
	}
}

// ---- runner ----

func TestRunnerModelDisabledHonestSkip(t *testing.T) {
	r := &Runner{
		Model: fakeModel{enabled: false},
		Fetch: fakeFetch{text: sampleText},
		Probe: fakeProbe{digest: "abc"},
	}
	res, err := r.Run(context.Background(), []ManifestEntry{{ID: 42}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Model.Ran {
		t.Fatal("模型未启用绝不能 Ran")
	}
	if !strings.Contains(res.Model.SkipReason, "模型未启用") {
		t.Errorf("必须给出未运行原因: %q", res.Model.SkipReason)
	}
	if len(res.Gate.Missed)+len(res.Gate.FalseBlock) != 0 || res.Gate.Total == 0 {
		t.Error("门禁对抗集必须照常运行且全对")
	}
	md := RenderMarkdown(res)
	if !strings.Contains(md, "**未运行**") {
		t.Error("报告必须显式标注模型指标未运行")
	}
	if !strings.Contains(md, "corpus_digest=abc") {
		t.Error("LC-8 就绪时应记语料快照")
	}
}

func TestRunnerNoFetchSkipAndLC8Degrade(t *testing.T) {
	r := &Runner{Model: fakeModel{enabled: true, out: fakeCandidates}}
	res, err := r.Run(context.Background(), []ManifestEntry{{ID: 42}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Model.Ran || !strings.Contains(res.Model.SkipReason, "类案源未启用") {
		t.Errorf("类案源未启用应整体标注未运行: %+v", res.Model)
	}
	if !strings.Contains(res.CorpusNote, "LC-8") {
		t.Errorf("LC-8 缺失应标注复现性受限: %q", res.CorpusNote)
	}
}

func TestRunnerHappyPathMetrics(t *testing.T) {
	r := &Runner{
		Model: fakeModel{enabled: true, out: fakeCandidates},
		Fetch: fakeFetch{text: sampleText},
		Probe: fakeProbe{},
	}
	res, err := r.Run(context.Background(), []ManifestEntry{{ID: 42}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Model.Ran || len(res.Model.Docs) != 1 {
		t.Fatalf("应产出 1 份文书指标: %+v", res.Model)
	}
	m := res.Model.Docs[0]
	if m.Candidates != 3 || m.Accepted != 1 || m.RejectedVerify != 1 || m.GateBlocked != 1 {
		t.Errorf("候选分解错误: %+v", m)
	}
	if m.TracedOK != 1 || m.EscapedVerify != 0 {
		t.Errorf("复验指标错误: %+v", m)
	}
}

func TestRunnerShaMismatchExcluded(t *testing.T) {
	r := &Runner{
		Model: fakeModel{enabled: true, out: fakeCandidates},
		Fetch: fakeFetch{text: sampleText},
		Probe: fakeProbe{},
	}
	res, err := r.Run(context.Background(), []ManifestEntry{{ID: 42, SHA256: strings.Repeat("0", 64)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Excluded) != 1 || !strings.Contains(res.Excluded[0].Reason, "sha256") {
		t.Errorf("正文变更必须剔除并计数: %+v", res.Excluded)
	}
	if len(res.Model.Docs) != 0 {
		t.Error("剔除的文书不得计入指标")
	}
	if md := RenderMarkdown(res); !strings.Contains(md, "剔除清单") {
		t.Error("报告必须含剔除清单")
	}
}

func TestRunnerFetchFailureExcluded(t *testing.T) {
	r := &Runner{
		Model: fakeModel{enabled: true, out: fakeCandidates},
		Fetch: fakeFetch{err: fmt.Errorf("LC-10 未就绪")},
		Probe: fakeProbe{},
	}
	res, err := r.Run(context.Background(), []ManifestEntry{{ID: 42}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Excluded) != 1 || !strings.Contains(res.Excluded[0].Reason, "LC-10") {
		t.Errorf("取文失败必须如实剔除: %+v", res.Excluded)
	}
}

func TestEmptyManifestGateOnly(t *testing.T) {
	r := &Runner{Model: fakeModel{enabled: true, out: fakeCandidates}, Fetch: fakeFetch{}}
	res, err := r.Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Model.Ran || !strings.Contains(res.Model.SkipReason, "清单") {
		t.Errorf("空清单应只跑门禁并标注: %+v", res.Model)
	}
}
