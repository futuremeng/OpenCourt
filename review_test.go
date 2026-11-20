// M7 审校与报告导出的 App 层测试：默认脱敏、原样导出二次确认 + 审计留痕、
// 报告全文过门禁（倾向性表述在报告层二次拦截）。
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readAudit(t *testing.T, a *App) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(a.dataDir, "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestExportReportRawRequiresConfirmation(t *testing.T) {
	a := newCaseApp(t, t.TempDir())
	if _, err := a.ExportReport(false, false, nil); err == nil ||
		!strings.Contains(err.Error(), "confirmRaw") {
		t.Fatalf("原样导出必须显式二次确认: %v", err)
	}
	if _, err := a.ExportReport(false, true, nil); err != nil {
		t.Fatalf("确认后原样导出应成功: %v", err)
	}
	auditText := readAudit(t, a)
	if !strings.Contains(auditText, "REPORT_EXPORT_RAW") {
		t.Error("原样导出必须写 REPORT_EXPORT_RAW 审计")
	}
}

func TestExportReportDefaultRedacted(t *testing.T) {
	a := newCaseApp(t, t.TempDir())
	if _, err := a.CreateCase("张某借贷案", "民间借贷", "FIRST_TRIAL"); err != nil {
		t.Fatal(err)
	}
	// 事实挂可定位引用（弱态）——审校无红项，导出才会真正走到脱敏环节。
	if err := a.scope().SaveDocument("DOC-R1", "loan.txt",
		map[int]string{1: "被告张某于3月借款10万元"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateFact("被告张某于3月借款10万元", "loan.txt", 1, "借款10万元", false); err != nil {
		t.Fatal(err)
	}
	res, err := a.ExportReport(true, false, []string{"张某"})
	if err != nil {
		t.Fatal(err)
	}
	md, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(md), "张某") {
		t.Error("默认导出必须已脱敏")
	}
	if !strings.Contains(string(md), "不构成裁判或法律意见") {
		t.Error("免责标注缺失")
	}
	if !strings.Contains(string(md), "脱敏复查") {
		t.Error("报告必须带脱敏复查说明")
	}
	if !strings.Contains(readAudit(t, a), "REPORT_EXPORT") {
		t.Error("导出动作必须写审计")
	}
}

// 审校标红即阻断导出：无依据事实（红项）存在时报告不得出门，且留审计。
func TestExportBlockedByReviewRed(t *testing.T) {
	a := newCaseApp(t, t.TempDir())
	if _, err := a.CreateCase("红项案", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateFact("无来源的陈述", "", 0, "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ExportReport(true, false, nil); err == nil ||
		!strings.Contains(err.Error(), "标红") {
		t.Fatalf("审校标红必须阻断导出: %v", err)
	}
	if !strings.Contains(readAudit(t, a), "REPORT_EXPORT_BLOCKED") {
		t.Error("审校阻断导出必须留审计")
	}
}

func TestReportLayerGateSecondIntercept(t *testing.T) {
	a := newCaseApp(t, t.TempDir())
	if _, err := a.CreateCase("测试案", "", ""); err != nil {
		t.Fatal(err)
	}
	// 事实挂可定位引用（审校无红项），描述携带倾向性表述——
	// 报告拼装后在报告层被门禁二次拦截。
	if err := a.scope().SaveDocument("DOC-G1", "note.txt",
		map[int]string{1: "原告称必然胜诉"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateFact("原告必然胜诉", "note.txt", 1, "必然胜诉", false); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ExportReport(true, false, nil); err == nil ||
		!strings.Contains(err.Error(), "门禁") {
		t.Fatalf("报告全文必须过 gate.Check: %v", err)
	}
	if !strings.Contains(readAudit(t, a), "REPORT_EXPORT_BLOCKED") {
		t.Error("报告层拦截必须留审计")
	}
}

func TestReviewCaseSurfacesBrokenLinks(t *testing.T) {
	a := newCaseApp(t, t.TempDir())
	if _, err := a.CreateCase("测试案", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateFact("有引用但原文缺失", "missing.pdf", 3, "不存在的片段", false); err != nil {
		t.Fatal(err)
	}
	rep, err := a.ReviewCase()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Red == 0 {
		t.Errorf("引用断链必须标红: %+v", rep.Issues)
	}
}
