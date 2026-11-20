// 审校与报告导出绑定（plan-v0.4 M7/D3）：审校全部走 neutral.Review 确定性
// 检查；导出默认脱敏，原样导出需显式二次确认并写审计；报告全文过
// gate.CheckGuidance——倾向性表述在报告层二次拦截，即报告也是输出。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/futuremeng/OpenCourt/core/gate"
	"github.com/futuremeng/OpenCourt/core/redact"
	"github.com/futuremeng/OpenCourt/core/storage"
	"github.com/futuremeng/OpenCourt/core/verify"
	"github.com/futuremeng/OpenCourt/neutral"
)

// ReviewCase 对本案执行确定性审校（引用核验/回原文/五态/推理链/冲突）。
func (a *App) ReviewCase() (neutral.ReviewReport, error) {
	b, err := a.reportInput()
	if err != nil {
		return neutral.ReviewReport{}, err
	}
	return neutral.Review(neutral.ReviewInput{
		Facts:     b.input.Facts,
		Conflicts: b.input.Conflicts,
		LawRefs:   b.input.LawRefs,
		Documents: a.documentTexts(),
		SimRounds: simRoundViews(b.rounds),
	}), nil
}

// ExportReportResult 导出结果（报告即留痕）。
type ExportReportResult struct {
	Path     string `json:"path"`
	Redacted bool   `json:"redacted"`
	Red      int    `json:"red"`
	Warn     int    `json:"warn"`
}

// ExportReport 导出 Markdown 报告。redacted=false（原样导出）必须
// confirmRaw=true 显式二次确认——否则拒绝；两种导出都写审计，
// 原样导出记 REPORT_EXPORT_RAW。审校标红（Red>0）拒绝导出——审校是
// 代码仲裁，报告不得带红项出门（原样导出自身的红项标注除外，它发生在
// 阻断检查之后）。报告全文过 gate，违例即拒绝导出。
func (a *App) ExportReport(redacted bool, confirmRaw bool, names []string) (ExportReportResult, error) {
	if !redacted && !confirmRaw {
		return ExportReportResult{}, fmt.Errorf("原样导出包含未脱敏当事人信息：须显式确认（confirmRaw）方可执行")
	}
	b, err := a.reportInput()
	if err != nil {
		return ExportReportResult{}, err
	}
	in := b.input
	in.Review = neutral.Review(neutral.ReviewInput{
		Facts:     in.Facts,
		Conflicts: in.Conflicts,
		LawRefs:   in.LawRefs,
		Documents: a.documentTexts(),
		SimRounds: simRoundViews(b.rounds),
	})
	if in.Review.Red > 0 {
		_ = a.auditLog.Log(a.profile.Role, "REPORT_EXPORT_BLOCKED", a.caseID(),
			fmt.Sprintf("审校标红 %d 项，导出被阻断", in.Review.Red))
		return ExportReportResult{}, fmt.Errorf("审校标红 %d 项，导出被阻断：先处理标红项（补来源/重验引用/删除无依据事实）再导出", in.Review.Red)
	}
	var md string
	if redacted {
		// 顺序纪律：先试脱敏计数 → 把复查说明并入审校 → 重渲染 → 对最终
		// 全文（含说明）再脱敏——报告必带脱敏复查说明，且说明本身也过脱敏。
		r := redact.New(names)
		probe := r.RedactText(neutral.RenderMarkdown(in))
		note := fmt.Sprintf("脱敏复查：%d 处已替换", len(probe.Actions))
		if len(probe.Actions) == 0 {
			note = "脱敏复查未发现命中——请确认脱敏名单完整"
		}
		in.Review.AddRedactionIssue(neutral.LevelWarn, note)
		res := r.RedactText(neutral.RenderMarkdown(in))
		for _, act := range res.Actions {
			_ = a.auditLog.Log(a.profile.Role, "REDACT", act.RuleID, act.Strategy)
		}
		md = res.Text
	} else {
		in.Review.AddRedactionIssue(neutral.LevelRed, "原样导出：包含未脱敏信息")
		md = neutral.RenderMarkdown(in)
	}

	// 报告也是输出：全文过门禁，倾向性表述在报告层二次拦截。
	if v := gate.CheckGuidance(md); len(v) > 0 {
		_ = a.auditLog.Log(a.profile.Role, "REPORT_EXPORT_BLOCKED", a.caseID(),
			fmt.Sprintf("报告文本门禁拦截 %d 项", len(v)))
		return ExportReportResult{}, fmt.Errorf("报告文本未通过出报告前检查（%d 项），未导出", len(v))
	}

	dir := filepath.Join(a.dataDir, "reports")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ExportReportResult{}, err
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.md", a.caseID(), time.Now().Format("20060102-150405")))
	if err := os.WriteFile(path, []byte(md), 0o600); err != nil {
		return ExportReportResult{}, err
	}
	action, note := "REPORT_EXPORT", fmt.Sprintf("脱敏报告导出 %s（标红 %d / 提示 %d）", filepath.Base(path), in.Review.Red, in.Review.Warn)
	if !redacted {
		action, note = "REPORT_EXPORT_RAW", fmt.Sprintf("原样（未脱敏）报告导出 %s（标红 %d / 提示 %d）", filepath.Base(path), in.Review.Red, in.Review.Warn)
	}
	_ = a.auditLog.Log(a.profile.Role, action, a.caseID(), note)
	return ExportReportResult{Path: path, Redacted: redacted, Red: in.Review.Red, Warn: in.Review.Warn}, nil
}

// reportBundle 报告装配输入 + 原始轮次行。
type reportBundle struct {
	input  neutral.ReportInput
	rounds []storage.SimRound
}

// reportInput 从本案存储收集报告拼装输入（全部确定性数据）。
func (a *App) reportInput() (reportBundle, error) {
	c, err := a.store.GetCase(a.caseID())
	if err != nil {
		return reportBundle{}, err
	}
	facts, err := a.scope().ListFacts()
	if err != nil {
		return reportBundle{}, err
	}
	conflicts, err := a.scope().ListConflicts()
	if err != nil {
		return reportBundle{}, err
	}
	lawRefs, err := a.scope().ListLawRefs()
	if err != nil {
		return reportBundle{}, err
	}
	rounds, err := a.scope().ListSimRounds()
	if err != nil {
		return reportBundle{}, err
	}
	var blindspots []neutral.Blindspot
	if reqs := a.elementNamesForCurrentCase(); len(reqs) > 0 {
		if match, err := a.RunElementMatch(reqs); err == nil {
			blindspots = neutral.GenerateBlindspots(match)
		}
	}
	return reportBundle{
		input: neutral.ReportInput{
			CaseTitle: c.Title, CaseID: a.caseID(), GeneratedAt: time.Now().UTC(),
			Facts: facts, Conflicts: conflicts, LawRefs: lawRefs,
			Blindspots: blindspots, SimRounds: simRoundViews(rounds),
		},
		rounds: rounds,
	}, nil
}

func simRoundViews(rounds []storage.SimRound) []neutral.SimRoundView {
	out := make([]neutral.SimRoundView, 0, len(rounds))
	for _, r := range rounds {
		out = append(out, neutral.SimRoundView{RoundID: r.RoundID, Phase: r.Phase + " · " + r.Role, Blocked: r.Blocked, FactIDs: r.FactIDs})
	}
	return out
}

// documentTexts 本案文档文本层（file → DocumentText），供回原文一致性审校。
func (a *App) documentTexts() map[string]*verify.DocumentText {
	docs := map[string]*verify.DocumentText{}
	list, err := a.scope().ListDocuments()
	if err != nil {
		return docs
	}
	for _, d := range list {
		if doc, err := a.scope().GetDocument(d.DocID); err == nil {
			docs[d.File] = doc
		}
	}
	return docs
}
