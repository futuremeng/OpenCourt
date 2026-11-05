// 评测 runner：门禁对抗集永远运行；模型阅卷指标按「显式启用才运行」纪律执行。
// 未运行 = 报告如实标注原因，绝不静默出「0 幻觉」这类假数字。
package eval

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/futuremeng/OpenCourt/core/verify"
	"github.com/futuremeng/OpenCourt/neutral"
	"github.com/futuremeng/OpenCourt/plugins"
)

// Fetcher 评测取文通道（LocalCasesSource.GetCaseText 实现；LC-10）。
type Fetcher interface {
	GetCaseText(ctx context.Context, id int64) (plugins.CaseTextDoc, error)
}

// Prober 语料快照探测（LocalCasesSource.Health 实现；LC-8）。
type Prober interface {
	Health(ctx context.Context) (plugins.LocalCasesHealth, error)
}

// CorpusSnapshot 语料快照信息。Digest 为空 = LC-8 未就绪，报告必须标注
// 「语料快照标识缺失，复现性受限」，并记 indexed_documents + 探测时间。
type CorpusSnapshot struct {
	IndexedDocuments int64     `json:"indexed_documents"`
	Version          string    `json:"version,omitempty"`
	Digest           string    `json:"corpus_digest,omitempty"`
	ProbedAt         time.Time `json:"probed_at"`
}

// ModelSection 模型指标段。Ran=false 时 Reason 必填——「未运行」是结论不是空白。
type ModelSection struct {
	Ran        bool         `json:"ran"`
	SkipReason string       `json:"skip_reason,omitempty"`
	ModelName  string       `json:"model_name,omitempty"`
	Docs       []DocMetrics `json:"docs,omitempty"`
	Aggregate  Aggregate    `json:"aggregate"`
}

// ExcludedEntry 被剔除出本轮评测的文书（取文失败/正文变更），如实计数。
type ExcludedEntry struct {
	ID     int64  `json:"id"`
	Reason string `json:"reason"`
}

// Result 一次评测的完整结果。
type Result struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Corpus      CorpusSnapshot  `json:"corpus"`
	CorpusNote  string          `json:"corpus_note,omitempty"` // LC-8 缺失等降级说明
	Gate        GateEvalResult  `json:"gate"`
	Model       ModelSection    `json:"model"`
	Excluded    []ExcludedEntry `json:"excluded,omitempty"`
}

// Runner 评测执行器。零值不可用；Fetch/Probe 为 nil = 类案源未启用，
// 模型指标整体标注未运行（不是逐份报错）。
type Runner struct {
	Model        plugins.ModelAdapter
	Fetch        Fetcher
	Probe        Prober
	ElementNames []string // 要件封闭候选集（V3）；无要件库传 nil
}

var errNoManifest = errors.New("评测清单为空：模型指标未运行")

// Run 执行一轮评测。manifest 为空清单时只跑门禁对抗集并如实标注。
func (r *Runner) Run(ctx context.Context, manifest []ManifestEntry) (*Result, error) {
	samples, err := LoadGateSamples()
	if err != nil {
		return nil, err
	}
	res := &Result{
		GeneratedAt: time.Now().UTC(),
		Gate:        RunGateAdversarial(samples),
		Model:       ModelSection{Docs: []DocMetrics{}},
		Excluded:    []ExcludedEntry{},
	}
	if r.Probe != nil {
		if h, err := r.Probe.Health(ctx); err == nil {
			res.Corpus = CorpusSnapshot{
				IndexedDocuments: h.IndexedDocuments, Version: h.Version,
				Digest: h.CorpusDigest, ProbedAt: time.Now().UTC(),
			}
			if h.CorpusDigest == "" {
				res.CorpusNote = "语料快照标识缺失（LC-8 未就绪）：本报告以 indexed_documents + 探测时间代指语料快照，复现性受限"
			}
		} else {
			res.CorpusNote = fmt.Sprintf("语料探测失败：%v（快照信息缺失，如实标注）", err)
		}
	} else {
		res.CorpusNote = "类案源未启用：无法探测语料快照（LC-8 无从核验），报告不含语料规模"
	}

	switch {
	case r.Fetch == nil:
		res.Model.SkipReason = "类案源未启用：评测取文走 LocalCases 正常对接通道（/api/case/{id}），未启用无法取文——模型指标未运行"
	case r.Model == nil || !r.Model.Enabled():
		res.Model.SkipReason = "模型未启用：默认离线。阅卷指标需要显式启用 OpenAI 兼容模型后运行——模型指标未运行"
	case len(manifest) == 0:
		res.Model.SkipReason = errNoManifest.Error() + "（tests/eval/corpus_manifest.jsonl 不存在或为空）"
	default:
		res.Model.Ran = true
		res.Model.ModelName = r.Model.Name()
		r.runDocs(ctx, manifest, res)
	}
	res.Model.Aggregate = AggregateOf(res.Model.Docs)
	return res, nil
}

func (r *Runner) runDocs(ctx context.Context, manifest []ManifestEntry, res *Result) {
	for _, e := range manifest {
		doc, err := r.Fetch.GetCaseText(ctx, e.ID)
		if err != nil {
			res.Excluded = append(res.Excluded, ExcludedEntry{ID: e.ID, Reason: fmt.Sprintf("取文失败: %v", err)})
			continue
		}
		if e.SHA256 != "" && SHA256Hex(doc.Text) != e.SHA256 {
			res.Excluded = append(res.Excluded, ExcludedEntry{
				ID: e.ID, Reason: "正文与清单 sha256 不一致（语料已变更，评测集需显式改版 evalset vN）",
			})
			continue
		}
		docText := &verify.DocumentText{
			File:  fmt.Sprintf("localcases:%d", e.ID),
			Pages: map[int]string{1: doc.Text},
		}
		rep, err := neutral.RunReader(ctx, fmt.Sprintf("EVAL-%d", e.ID), docText, r.Model, r.ElementNames)
		if err != nil {
			res.Model.Docs = append(res.Model.Docs, DocMetrics{
				DocID: fmt.Sprintf("EVAL-%d", e.ID), Error: fmt.Sprintf("阅卷失败: %v", err),
			})
			continue
		}
		res.Model.Docs = append(res.Model.Docs, MetricsFromReport(rep, docText))
	}
}
