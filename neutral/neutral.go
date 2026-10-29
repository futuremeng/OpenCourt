// Package neutral 中立逻辑层（开发文档 2.2、4.9-4.13）：
// 所有人共用同一套代码，但数据各自独立。包含汇聚器、核验器、
// 要件匹配器、盲区生成器与程序守护。
// 依赖方向：neutral -> core（禁止 core 反向依赖本包）。
package neutral

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/verify"
	"github.com/futuremeng/OpenCourt/plugins"
)

// TimelineItem 时间线条目。
type TimelineItem struct {
	Date string // ISO 或中文日期
	Fact domain.Fact
}

// Aggregate 汇聚器（开发文档 4.9）：编排时间线、标记内部矛盾与证据冲突。
// 冲突不自动消解：发现的冲突以 Conflict 记录并列输出。
func Aggregate(facts []domain.Fact) (timeline []TimelineItem, conflicts []domain.Conflict) {
	timeline = make([]TimelineItem, 0, len(facts))
	for _, f := range facts {
		timeline = append(timeline, TimelineItem{Fact: f})
	}
	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i].Fact.FactID < timeline[j].Fact.FactID
	})

	// 同一要件下存在不同原文表述的事实 → 标记为冲突候选（并列展示，不采信任何一方）。
	byElement := map[string][]domain.Fact{}
	for _, f := range facts {
		if f.RelatedElement == "" {
			continue
		}
		byElement[f.RelatedElement] = append(byElement[f.RelatedElement], f)
	}
	idx := 0
	for element, group := range byElement {
		if len(group) < 2 {
			continue
		}
		diff := false
		for i := 0; i < len(group) && !diff; i++ {
			for j := i + 1; j < len(group); j++ {
				a, b := group[i], group[j]
				if a.Citation != nil && b.Citation != nil &&
					a.Citation.Snippet != b.Citation.Snippet {
					diff = true
					break
				}
			}
		}
		if diff {
			idx++
			ids := make([]string, 0, len(group))
			for _, f := range group {
				ids = append(ids, f.FactID)
			}
			conflicts = append(conflicts, domain.Conflict{
				ConflictID:       formatConflictID(idx),
				FactIDs:          ids,
				ConflictPoint:    "同一要件「" + element + "」下存在不一致的材料表述",
				ResolutionMethod: "补强证据或庭审质证后由用户人工判断",
			})
		}
	}
	return timeline, conflicts
}

func formatConflictID(i int) string {
	return "CONF-" + pad(i)
}

func pad(i int) string {
	digits := ""
	for {
		digits = string(rune('0'+i%10)) + digits
		i /= 10
		if i == 0 {
			break
		}
	}
	return digits
}

// VerifyFact 核验器（开发文档 4.10）：不看结论，只回原文。
// 定位失败的事实直接打回为「无依据」。
func VerifyFact(f domain.Fact, docs map[string]*verify.DocumentText) domain.Fact {
	if f.Citation == nil || !verify.Locate(*f.Citation, docs) {
		f.State = domain.StateUnsupported
		return f
	}
	if f.State == domain.StateUnsupported {
		f.State = domain.StateSingleSource
	}
	return f
}

// MatchResult 要件匹配结果（开发文档 4.11）。
type MatchResult struct {
	Covered []string // 已有事实支撑的要件
	Missing []string // 缺失要件（直接列出，不自动补全）
}

// MatchElements 要件匹配器：将事实与请求权基础要件逐条对照。
// 只有「已溯源确认」的事实才能覆盖要件；弱态事实记入待补强。
func MatchElements(facts []domain.Fact, requirements []string) MatchResult {
	res := MatchResult{Covered: []string{}, Missing: []string{}}
	for _, req := range requirements {
		covered := false
		for _, f := range facts {
			if f.RelatedElement == req && f.State == domain.StateTracedConfirmed {
				covered = true
				break
			}
		}
		if covered {
			res.Covered = append(res.Covered, req)
		} else {
			res.Missing = append(res.Missing, req)
		}
	}
	return res
}

// ElementSource 要件来源（plan-v0.2 M4）：OpenLaw 要件库适配器实现；
// nil = 无要件库。要件数据归 OpenLaw（人工确认导入），OpenCourt 只读取
// 封闭候选集——绝不生成或补全要件（V3）。
type ElementSource interface {
	ElementsByCaseType(ctx context.Context, caseType string) ([]plugins.ElementSpec, error)
}

// MatchElementsFrom 按案由的要件匹配：要件清单只从要件库取（封闭集），
// 匹配语义与 MatchElements 一致（仅「已溯源确认」事实可覆盖要件）。
// src == nil、案由为空或案由无要件 → 明确报错并给出出路，
// 绝不回退到模型自由生成要件。
func MatchElementsFrom(facts []domain.Fact, src ElementSource, caseType string) (MatchResult, error) {
	if src == nil {
		return MatchResult{}, fmt.Errorf("无要件库：请手输要件，或经 OpenLaw（openlaw-element）人工确认导入要件")
	}
	caseType = strings.TrimSpace(caseType)
	if caseType == "" {
		return MatchResult{}, fmt.Errorf("案由为空：无法按案由取要件（案由是自由文本，须与要件库录入一致）")
	}
	specs, err := src.ElementsByCaseType(context.Background(), caseType)
	if err != nil {
		return MatchResult{}, err
	}
	if len(specs) == 0 {
		return MatchResult{}, fmt.Errorf("案由「%s」无已录入构成要件：请手输要件或经 OpenLaw 人工确认导入", caseType)
	}
	reqs := make([]string, 0, len(specs))
	for _, sp := range specs {
		reqs = append(reqs, sp.Name)
	}
	return MatchElements(facts, reqs), nil
}

// Blindspot 盲区条目（开发文档 4.12）。
type Blindspot struct {
	Question   string `json:"question"`   // 无法判断的问题
	Supplement string `json:"supplement"` // 补证方向
}

// GenerateBlindspots 盲区生成器：基于要件匹配结果输出“无法判断”的问题清单
// 与“补证方向”。绝不通过模型幻觉填补事实空白。
func GenerateBlindspots(match MatchResult) []Blindspot {
	out := make([]Blindspot, 0, len(match.Missing))
	for _, m := range match.Missing {
		out = append(out, Blindspot{
			Question:   "要件「" + m + "」目前无法判断",
			Supplement: "补证方向：导入能直接证明「" + m + "」的原始材料",
		})
	}
	return out
}
