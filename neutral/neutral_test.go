package neutral

import (
	"testing"
	"time"

	"github.com/futuremeng/OpenCourt/core/domain"
)

func TestAggregateDetectsConflicts(t *testing.T) {
	f1 := domain.Fact{FactID: "F1", RelatedElement: "付款时间",
		Citation: &domain.Citation{SourceFile: "a.txt", PageNum: 1, Snippet: "3月1日付款"}}
	f2 := domain.Fact{FactID: "F2", RelatedElement: "付款时间",
		Citation: &domain.Citation{SourceFile: "b.txt", PageNum: 2, Snippet: "5月1日付款"}}
	_, conflicts := Aggregate([]domain.Fact{f1, f2})
	if len(conflicts) != 1 {
		t.Fatalf("应检出 1 个冲突, got %d", len(conflicts))
	}
	if conflicts[0].IsResolved {
		t.Error("冲突不得被自动消解")
	}
}

func TestMatchAndBlindspots(t *testing.T) {
	facts := []domain.Fact{
		{FactID: "F1", RelatedElement: "合同成立", State: domain.StateTracedConfirmed},
		{FactID: "F2", RelatedElement: "违约行为", State: domain.StateSingleSource},
	}
	match := MatchElements(facts, []string{"合同成立", "违约行为", "损失金额"})
	if len(match.Covered) != 1 || match.Covered[0] != "合同成立" {
		t.Fatalf("要件覆盖错误: %+v", match)
	}
	if len(match.Missing) != 2 {
		t.Fatalf("缺失要件错误: %+v", match.Missing)
	}
	spot := GenerateBlindspots(match)
	if len(spot) != 2 || spot[0].Supplement == "" {
		t.Fatalf("盲区生成错误: %+v", spot)
	}
}

func TestProcedureGuard(t *testing.T) {
	now := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	rules := []DeadlineRule{
		{Name: "上诉期限", EventDate: now.AddDate(0, 0, -10), Days: 15, WarnDays: 7}, // 剩 5 天 → WARN
		{Name: "举证期限", EventDate: now.AddDate(0, 0, -20), Days: 15, WarnDays: 5}, // 已超期 → OVERDUE
		{Name: "送达回证", EventDate: now, Days: 30, WarnDays: 3},                    // 剩 30 天 → 无预警
	}
	warnings := CheckDeadlines(rules, now)
	if len(warnings) != 2 {
		t.Fatalf("应产生 2 条预警, got %d", len(warnings))
	}
	var overdue, warn int
	for _, w := range warnings {
		switch w.Level {
		case "OVERDUE":
			overdue++
		case "WARN":
			warn++
		}
	}
	if overdue != 1 || warn != 1 {
		t.Fatalf("预警级别分布错误: %+v", warnings)
	}
}
