// 评测报告（Markdown）：所有指标必须带「目标 / 实测 / 判定」三列；
// 未测项判定列写「未运行 + 原因」，禁止留空或写 0。
package eval

import (
	"fmt"
	"strings"
)

// 验收目标（开发文档 11.1）。不降标准；未达标项如实保留。
const (
	targetEscapedRate = 0.01 // 逃逸幻觉率 < 1%
	targetTraceRate   = 0.98 // 溯源准确率 > 98%
)

func pct(v float64) string { return fmt.Sprintf("%.2f%%", v*100) }

func verdict(ok bool) string {
	if ok {
		return "✅ 达标"
	}
	return "❌ 未达标"
}

// RenderMarkdown 渲染评测报告。
func RenderMarkdown(r *Result) string {
	var sb strings.Builder
	sb.WriteString("# OpenCourt 评测报告（E0）\n\n")
	fmt.Fprintf(&sb, "- 生成时间：%s\n", r.GeneratedAt.Format("2006-01-02 15:04:05 UTC"))
	if !r.Corpus.ProbedAt.IsZero() {
		fmt.Fprintf(&sb, "- 语料规模：%d 份文书（探测于 %s）\n",
			r.Corpus.IndexedDocuments, r.Corpus.ProbedAt.Format("2006-01-02"))
	}
	if r.Corpus.Digest != "" {
		fmt.Fprintf(&sb, "- 语料快照：`corpus_digest=%s`\n", r.Corpus.Digest)
	}
	if r.Corpus.Version != "" {
		fmt.Fprintf(&sb, "- LocalCases 版本：%s\n", r.Corpus.Version)
	}
	if r.CorpusNote != "" {
		fmt.Fprintf(&sb, "- ⚠️ %s\n", r.CorpusNote)
	}

	sb.WriteString("\n## 门禁对抗集（离线确定性）\n\n")
	fmt.Fprintf(&sb, "- 样本：%d 条（必拦 %d / 必放 %d）\n", r.Gate.Total, r.Gate.MustBlock, r.Gate.MustPass)
	fmt.Fprintf(&sb, "- 误拦率：目标 0 · 实测 %s · %s\n", pct(r.Gate.FalseBlockRate()),
		map[bool]string{true: "✅ 达标", false: "❌ 未达标"}[len(r.Gate.FalseBlock) == 0])
	fmt.Fprintf(&sb, "- 漏拦率：目标 0 · 实测 %s · %s\n", pct(r.Gate.MissRate()),
		map[bool]string{true: "✅ 达标", false: "❌ 未达标"}[len(r.Gate.Missed) == 0])
	for _, f := range r.Gate.Missed {
		fmt.Fprintf(&sb, "  - 漏拦：「%s」（%s）\n", f.Sample.Text, f.Sample.Note)
	}
	for _, f := range r.Gate.FalseBlock {
		fmt.Fprintf(&sb, "  - 误拦：「%s」被 %s/%s 命中（%s）\n", f.Sample.Text, f.Rule, f.Phrase, f.Sample.Note)
	}

	sb.WriteString("\n## 模型阅卷指标\n\n")
	if !r.Model.Ran {
		fmt.Fprintf(&sb, "- **未运行**：%s\n", r.Model.SkipReason)
		fmt.Fprintf(&sb, "- 逃逸幻觉率：目标 <1%% · 实测 **未运行**\n")
		fmt.Fprintf(&sb, "- 溯源准确率：目标 >98%% · 实测 **未运行**\n")
	} else {
		a := r.Model.Aggregate
		fmt.Fprintf(&sb, "- 模型：%s · 有效评测 %d 份（失败/剔除 %d 份）\n",
			r.Model.ModelName, a.Docs, len(r.Excluded)+a.FailedDocs)
		fmt.Fprintf(&sb, "- 候选总数：%d · 入库 %d · 核验拒绝 %d · 门禁拦截 %d\n",
			a.Candidates, a.Accepted, a.RejectedVerify, a.GateBlocked)
		fmt.Fprintf(&sb, "- 候选拒绝率（核验+门禁拦下）：%s\n", pct(a.CandidateRejectRate()))
		fmt.Fprintf(&sb, "- 逃逸幻觉率：目标 <1%% · 实测 %s · %s\n",
			pct(a.EscapedRate()), verdict(a.Accepted > 0 && a.EscapedRate() < targetEscapedRate))
		fmt.Fprintf(&sb, "- 溯源准确率：目标 >98%% · 实测 %s · %s\n",
			pct(a.TraceAccuracy()), verdict(a.Accepted > 0 && a.TraceAccuracy() > targetTraceRate))
	}

	if len(r.Excluded) > 0 {
		sb.WriteString("\n## 剔除清单（如实计数）\n\n")
		for _, x := range r.Excluded {
			fmt.Fprintf(&sb, "- id=%d：%s\n", x.ID, x.Reason)
		}
	}

	sb.WriteString("\n## 已知限制\n\n")
	sb.WriteString("- 本报告不构成对任何真实案件的结论；评测语料为公开裁判文书。\n")
	sb.WriteString("- LC-8/LC-9/LC-10 未就绪时，复现性与完整性按上文标注降级。\n")
	sb.WriteString("- 门禁对抗集覆盖词表类检查；引用落地类门禁由 tests/compliance 用例锁定。\n")
	return sb.String()
}
