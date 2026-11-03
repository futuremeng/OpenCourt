// M6 争点归纳（开发文档 3.6、roadmap-next M6）：从中立视角把事实按要件
// 分组、并列展示未消解冲突、直陈证据缺口。归纳只呈现结构，不评哪方占优——
// 任何倾向性表述都会被门禁在输出端拦截。
package neutral

import (
	"sort"
	"strings"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/gate"
)

// Issue 一个争点：要件 + 涉及事实 + 并列的冲突点。
type Issue struct {
	Element        string   `json:"element"`
	FactIDs        []string `json:"fact_ids"`
	ConflictPoints []string `json:"conflict_points,omitempty"` // 冲突并列展示，不代作取舍
	HasOpponent    bool     `json:"has_opponent"`              // 争点内含「对方主张」事实
}

// IssueReport 争点归纳结果。Note 固定中立声明；Gaps 为证据缺口（缺失要件）。
type IssueReport struct {
	Issues []Issue  `json:"issues"`
	Gaps   []string `json:"gaps"` // 证据缺口：要件清单中无已确认事实支撑的项
	Note   string   `json:"note"`
}

// SummarizeIssues 按关联要件分组归纳争点：
//   - 同要件下的事实并列，冲突（未消解）的冲突点原样并列挂到争点下；
//   - 「对方主张」状态事实只标注不采信；
//   - match.Missing 直通为证据缺口，不自动补全。
func SummarizeIssues(facts []domain.Fact, conflicts []domain.Conflict, match MatchResult) IssueReport {
	byElement := map[string]*Issue{}
	order := []string{}
	for _, f := range facts {
		el := strings.TrimSpace(f.RelatedElement)
		if el == "" {
			continue
		}
		it, ok := byElement[el]
		if !ok {
			it = &Issue{Element: el, FactIDs: []string{}}
			byElement[el] = it
			order = append(order, el)
		}
		it.FactIDs = append(it.FactIDs, f.FactID)
		if f.State == domain.StateOpponentClaim {
			it.HasOpponent = true
		}
	}
	factEl := map[string]string{}
	for el, it := range byElement {
		for _, id := range it.FactIDs {
			factEl[id] = el
		}
	}
	for _, c := range conflicts {
		if c.IsResolved {
			continue
		}
		for _, id := range c.FactIDs {
			el, ok := factEl[id]
			if !ok {
				continue
			}
			it := byElement[el]
			cp := c.ConflictPoint + "（" + strings.Join(c.FactIDs, "、") + "）"
			found := false
			for _, x := range it.ConflictPoints {
				if x == cp {
					found = true
					break
				}
			}
			if !found {
				it.ConflictPoints = append(it.ConflictPoints, cp)
			}
			break
		}
	}
	sort.Strings(order)
	rep := IssueReport{
		Issues: []Issue{},
		Gaps:   append([]string{}, match.Missing...),
		Note:   "争点归纳为中立整理：冲突并列展示、不代作取舍，不构成对任何一方的倾向性评价",
	}
	for _, el := range order {
		it := byElement[el]
		sort.Strings(it.FactIDs)
		if v := gate.CheckGuidance(it.Element); len(v) > 0 {
			// 要件名本身出现倾向词（异常数据）——原样保留但绝不加工。
			continue
		}
		rep.Issues = append(rep.Issues, *it)
	}
	return rep
}
