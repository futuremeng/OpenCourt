package neutral

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/plugins"
)

// fakeElementSource 要件库测试替身。
type fakeElementSource struct {
	specs []plugins.ElementSpec
	err   error
	calls []string
}

func (f *fakeElementSource) ElementsByCaseType(_ context.Context, caseType string) ([]plugins.ElementSpec, error) {
	f.calls = append(f.calls, caseType)
	return f.specs, f.err
}

// 【M4】MatchElementsFrom：要件清单来自要件库（封闭集），匹配语义同 MatchElements。
func TestMatchElementsFromClosedSet(t *testing.T) {
	src := &fakeElementSource{specs: []plugins.ElementSpec{
		{ElementID: "e1", Name: "主体适格"},
		{ElementID: "e2", Name: "滥用行为"},
	}}
	facts := []domain.Fact{
		{FactID: "F1", RelatedElement: "主体适格", State: domain.StateTracedConfirmed},
		{FactID: "F2", RelatedElement: "滥用行为", State: domain.StateSingleSource}, // 弱态不覆盖
	}
	res, err := MatchElementsFrom(facts, src, "公司纠纷")
	if err != nil {
		t.Fatal(err)
	}
	if len(src.calls) != 1 || src.calls[0] != "公司纠纷" {
		t.Errorf("要件库查询错误: %v", src.calls)
	}
	if len(res.Covered) != 1 || res.Covered[0] != "主体适格" {
		t.Errorf("Covered 错误: %+v", res.Covered)
	}
	if len(res.Missing) != 1 || res.Missing[0] != "滥用行为" {
		t.Errorf("Missing 错误: %+v", res.Missing)
	}
}

// 【M4】无要件库/案由为空/案由无要件/取数失败 → 一律显式报错，绝不静默。
func TestMatchElementsFromExplicitDegradation(t *testing.T) {
	if _, err := MatchElementsFrom(nil, nil, "公司纠纷"); err == nil || !strings.Contains(err.Error(), "无要件库") {
		t.Errorf("src=nil 必须报「无要件库」: %v", err)
	}
	src := &fakeElementSource{}
	if _, err := MatchElementsFrom(nil, src, "  "); err == nil || !strings.Contains(err.Error(), "案由为空") {
		t.Errorf("案由为空必须显式报错: %v", err)
	}
	if _, err := MatchElementsFrom(nil, src, "公司纠纷"); err == nil || !strings.Contains(err.Error(), "无已录入构成要件") {
		t.Errorf("案由无要件必须显式报错（不返回空结果冒充）: %v", err)
	}
	src.err = errors.New("openlaw 打开失败")
	if _, err := MatchElementsFrom(nil, src, "公司纠纷"); err == nil || !strings.Contains(err.Error(), "openlaw 打开失败") {
		t.Errorf("取数失败必须透传: %v", err)
	}
}
