package neutral

import (
	"strings"
	"testing"
	"time"

	"github.com/futuremeng/OpenCourt/core/domain"
)

func TestReportWeakStateNeverInFactSection(t *testing.T) {
	in := ReportInput{
		CaseTitle: "测试案", CaseID: "CASE-T", GeneratedAt: time.Unix(0, 0).UTC(),
		Facts: []domain.Fact{
			{FactID: "FACT-STRONG", Description: "强态事实A", State: domain.StateTracedConfirmed,
				Citation: &domain.Citation{SourceFile: "a.pdf", PageNum: 1, Snippet: "A"}},
			{FactID: "FACT-WEAK", Description: "弱态事实B", State: domain.StateSingleSource},
			{FactID: "FACT-CLAIM", Description: "对方主张C", State: domain.StateOpponentClaim},
			{FactID: "FACT-NOSRC", Description: "缺来源强态D", State: domain.StateTracedConfirmed},
		},
		Conflicts: []domain.Conflict{{ConflictID: "C-1", FactIDs: []string{"FACT-WEAK"}, ConflictPoint: "交付时间"}},
		Blindspots: []Blindspot{{Question: "是否交付", Supplement: "补充签收单"}},
		SimRounds: []SimRoundView{
			{RoundID: "R-1", Phase: "调查", FactIDs: []string{"FACT-STRONG"}},
			{RoundID: "R-2", Phase: "辩论", Blocked: true},
		},
	}
	md := RenderMarkdown(in)

	factSection := md[strings.Index(md, "## 一、事实清单"):strings.Index(md, "## 二、")]
	if strings.Contains(factSection, "弱态事实B") || strings.Contains(factSection, "对方主张C") {
		t.Errorf("弱态事实出现在报告事实段落（合规违例）:\n%s", factSection)
	}
	if !strings.Contains(md, "弱态事实B") || !strings.Contains(md, "待人工确认清单") {
		t.Error("弱态事实必须进待人工确认清单")
	}
	if !strings.Contains(md, Disclaimer) {
		t.Error("免责标注缺失")
	}
	if !strings.Contains(md, "**[缺来源]**") {
		t.Error("缺来源结论必须显式标红")
	}
	if !strings.Contains(md, "[已被门禁拦截]") {
		t.Error("被拦截轮次必须可见")
	}
	if !strings.Contains(md, "交付时间") || !strings.Contains(md, "补充签收单") {
		t.Error("冲突并列/盲区补证方向缺失")
	}
}

func TestReportDeterministic(t *testing.T) {
	in := ReportInput{CaseTitle: "X", CaseID: "CASE-1", GeneratedAt: time.Unix(1700000000, 0).UTC()}
	if RenderMarkdown(in) != RenderMarkdown(in) {
		t.Error("同一输入必须产出同一报告（确定性）")
	}
}