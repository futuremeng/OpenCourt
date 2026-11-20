package neutral

import (
	"testing"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/verify"
)

func citeFact(id string, state domain.FactState, file string, page int, snippet string) domain.Fact {
	return domain.Fact{
		FactID: id, Description: "事实" + id, State: state,
		Citation: &domain.Citation{SourceFile: file, PageNum: page, Snippet: snippet},
	}
}

func doc(file, text string) *verify.DocumentText {
	return &verify.DocumentText{File: file, Pages: map[int]string{1: text}}
}

func countIssues(rep ReviewReport, check, level string) int {
	n := 0
	for _, iss := range rep.Issues {
		if iss.Check == check && iss.Level == level {
			n++
		}
	}
	return n
}

func TestReviewDeterministicChecks(t *testing.T) {
	in := ReviewInput{
		Facts: []domain.Fact{
			citeFact("FACT-1", domain.StateTracedConfirmed, "a.pdf", 1, "借款 10 万元"),
			citeFact("FACT-2", domain.StateTracedConfirmed, "a.pdf", 1, "原文里不存在的话"),
			citeFact("FACT-3", domain.StateSingleSource, "a.pdf", 1, "转账凭证载明"),
			{FactID: "FACT-4", Description: "无来源强态", State: domain.StateTracedConfirmed},
			{FactID: "FACT-5", Description: "无依据", State: domain.StateUnsupported},
		},
		LawRefs: []domain.LawRef{
			{RefID: "LAW-1", LawName: "民法典", ArticleNo: "第667条", VerifyResult: "OK"},
			{RefID: "LAW-2", LawName: "民法典", ArticleNo: "第680条", VerifyResult: ""},
			{RefID: "LAW-3", LawName: "民法典", ArticleNo: "第X条", VerifyResult: "OL-1002"},
		},
		Documents: map[string]*verify.DocumentText{"a.pdf": doc("a.pdf", "借款 10 万元，转账凭证载明到期归还")},
		SimRounds: []SimRoundView{
			{RoundID: "R-1", Phase: "法庭调查", Blocked: false, FactIDs: []string{"FACT-1"}},
			{RoundID: "R-2", Phase: "法庭辩论", Blocked: false, FactIDs: nil},
			{RoundID: "R-3", Phase: "最后陈述", Blocked: false, FactIDs: []string{"FACT-9"}},
			{RoundID: "R-4", Phase: "被拦轮", Blocked: true, FactIDs: nil},
		},
		Conflicts: []domain.Conflict{{ConflictID: "C-1", ConflictPoint: "利率", IsResolved: false}},
	}
	rep := Review(in)

	if countIssues(rep, CheckLawCitation, LevelWarn) != 1 {
		t.Errorf("未核验引用应 1 条 warn: %+v", rep.Issues)
	}
	if countIssues(rep, CheckLawCitation, LevelRed) != 1 {
		t.Errorf("核验失败引用应 1 条 red")
	}
	if countIssues(rep, CheckSourceLocate, LevelRed) != 2 { // FACT-2 定位失败 + FACT-4 强态缺坐标
		t.Errorf("回原文检查 red 应为 2: %+v", rep.Issues)
	}
	if countIssues(rep, CheckStateCompliance, LevelRed) != 1 { // FACT-5
		t.Errorf("无依据事实应标红")
	}
	if countIssues(rep, CheckReasoningChain, LevelRed) != 2 { // R-2 无事实 + R-3 幽灵引用
		t.Errorf("推理链断裂应 2 条 red: %+v", rep.Issues)
	}
	if countIssues(rep, CheckStateCompliance, LevelWarn) != 1 { // 未解决冲突
		t.Errorf("未解决冲突应 warn")
	}
	if rep.Red == 0 || rep.Warn == 0 {
		t.Errorf("统计缺失: %+v", rep)
	}
}

func TestReviewCleanCaseNoRed(t *testing.T) {
	rep := Review(ReviewInput{
		Facts:     []domain.Fact{citeFact("FACT-1", domain.StateTracedConfirmed, "a.pdf", 1, "借款")},
		LawRefs:   []domain.LawRef{{RefID: "LAW-1", VerifyResult: "OK"}},
		Documents: map[string]*verify.DocumentText{"a.pdf": doc("a.pdf", "借款 10 万元")},
		SimRounds: []SimRoundView{{RoundID: "R-1", FactIDs: []string{"FACT-1"}}},
	})
	if rep.Red != 0 {
		t.Errorf("干净案件不应有 red: %+v", rep.Issues)
	}
}
