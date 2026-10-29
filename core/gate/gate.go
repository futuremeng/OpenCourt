// Package gate 实现幻觉门禁与引导性内容检测（开发文档 4.6、7.4）。
// 1. 引导性内容检测：拦截“建议起诉”“大概率胜诉”“应当调解”等倾向性表述。
// 2. 幻觉门禁：检查输出引用的事实是否可落地，无依据内容一律拦截。
package gate

import (
	"fmt"
	"strings"

	"github.com/futuremeng/OpenCourt/core/domain"
)

// Violation 一次门禁违规。
type Violation struct {
	Rule    string `json:"rule"`    // 触发的规则名
	Phrase  string `json:"phrase"`  // 命中的表述/实体
	Message string `json:"message"` // 违规说明
}

// blockedGuidance 引导性表述拦截词表（开发文档 1.1、7.4）。
// 命中即拒绝输出，强制改写为中立的事实陈述。
var blockedGuidance = []struct {
	phrase string
	rule   string
}{
	{"建议起诉", "NO_GUIDANCE"},
	{"不建议起诉", "NO_GUIDANCE"},
	{"建议调解", "NO_GUIDANCE"},
	{"应当调解", "NO_GUIDANCE"},
	{"不建议调解", "NO_GUIDANCE"},
	{"建议上诉", "NO_GUIDANCE"},
	{"不建议上诉", "NO_GUIDANCE"},
	{"建议撤诉", "NO_GUIDANCE"},
	{"大概率胜诉", "NO_PROBABILITY"},
	{"大概率败诉", "NO_PROBABILITY"},
	{"胜诉率", "NO_PROBABILITY"},
	{"败诉率", "NO_PROBABILITY"},
	{"胜诉概率", "NO_PROBABILITY"},
	{"量刑预测", "NO_PREDICTION"},
	{"判决预测", "NO_PREDICTION"},
	{"估计刑期", "NO_PREDICTION"},
	{"应当判决", "NO_VERDICT"},
	{"构成犯罪", "NO_VERDICT"},
	{"不构成犯罪", "NO_VERDICT"},
	{"成立正当防卫", "NO_VERDICT"},
}

// CheckGuidance 检测文本中的倾向性/预测性/裁判性表述。
func CheckGuidance(text string) []Violation {
	violations := make([]Violation, 0)
	for _, b := range blockedGuidance {
		if strings.Contains(text, b.phrase) {
			violations = append(violations, Violation{
				Rule:    b.rule,
				Phrase:  b.phrase,
				Message: fmt.Sprintf("检测到引导性表述「%s」：系统不提供倾向性结论、不预测判决结果、不引导用户决策", b.phrase),
			})
		}
	}
	return violations
}

// FactIndex 按 ID 索引的事实集合，供门禁做实体落地检查。
type FactIndex map[string]domain.Fact

// RefInfo 引用留痕（law_refs / case_refs）做门禁校验所需的最小信息（D2）。
type RefInfo struct {
	Kind          string // LAW / CASE
	Status        string // EFFECTIVE / AMENDED / UNVERIFIED / UNKNOWN
	ConfirmStatus string // confirmed / pending
	VerifyResult  string // OK 或 OL-1001..OL-1006
}

// RefIndex 按引用 ID（LAW-…/CASE-…）索引的引用留痕。
// 跨案件引用在此天然失败：scope 存储只装当前案件的 refs，A 案的 ID 在
// B 案的索引里不存在 → HALLUCINATION。
type RefIndex map[string]RefInfo

// OutputDraft 一次待检输出：文本 + 其引用的事实 ID 集合。
type OutputDraft struct {
	Text      string
	FactIDs   []string
	Citations []domain.Citation
}

// CheckGrounding 幻觉门禁：输出引用的实体必须可落地——
// FACT- 前缀走事实索引（不得引用「无依据」状态）；
// LAW- / CASE- 前缀走引用表校验（不存在即幻觉；UNVERIFIED 或 pending
// 即违规——未核验/待人工确认的引用不得静默通过，D2）。
// 引用必须满足最低溯源要求（页码+原文）。
func CheckGrounding(out OutputDraft, index FactIndex, refs RefIndex) []Violation {
	violations := make([]Violation, 0)
	for _, id := range out.FactIDs {
		if strings.HasPrefix(id, "LAW-") || strings.HasPrefix(id, "CASE-") {
			r, ok := refs[id]
			if !ok {
				violations = append(violations, Violation{
					Rule:    "HALLUCINATION",
					Phrase:  id,
					Message: fmt.Sprintf("引用的实体 %s 在当前案件的引用留痕中不存在（跨案件引用同样拦截），直接拦截", id),
				})
				continue
			}
			if r.Status == "UNVERIFIED" || r.ConfirmStatus == "pending" || r.VerifyResult != "OK" {
				violations = append(violations, Violation{
					Rule:    "REF_UNVERIFIED",
					Phrase:  id,
					Message: fmt.Sprintf("引用 %s 未核验或待人工确认（状态 %s / 确认 %s / 核验 %s）：不得静默通过，界面必须标注并转人工", id, r.Status, r.ConfirmStatus, r.VerifyResult),
				})
			}
			continue
		}
		f, ok := index[id]
		if !ok {
			violations = append(violations, Violation{
				Rule:    "HALLUCINATION",
				Phrase:  id,
				Message: fmt.Sprintf("引用的事实 %s 在卷宗索引中不存在，直接拦截", id),
			})
			continue
		}
		if f.State == domain.StateUnsupported {
			violations = append(violations, Violation{
				Rule:    "HALLUCINATION",
				Phrase:  id,
				Message: fmt.Sprintf("引用的事实 %s 状态为「无依据」，模型生成此类内容直接被门禁拦截", id),
			})
		}
		if f.State == domain.StateOpponentClaim {
			violations = append(violations, Violation{
				Rule:    "OPPONENT_CLAIM_IN_FACT",
				Phrase:  id,
				Message: fmt.Sprintf("事实 %s 为「对方主张未验证」，禁止出现在事实段落，必须显式标注「对方主张」", id),
			})
		}
	}
	for _, c := range out.Citations {
		if !c.Valid() {
			violations = append(violations, Violation{
				Rule:    "CITATION_INVALID",
				Phrase:  c.SourceFile,
				Message: "引用缺少卷宗页码或原文片段：无法定位到具体页码或原文的陈述将被替换为「[无法判断：缺乏原文依据]」",
			})
		}
		if c.LawRepealed {
			violations = append(violations, Violation{
				Rule:    "LAW_REPEALED",
				Phrase:  c.LawVersion,
				Message: "引用的法条已废止，输出端强制标红并提示「已废止」",
			})
		}
	}
	return violations
}

// Check 汇总执行全部输出门禁（开发文档 7.4 三层门禁）。
// 返回的 violations 非空即表示输出必须被拒绝或改写。
func Check(out OutputDraft, index FactIndex, refs RefIndex) []Violation {
	all := make([]Violation, 0)
	all = append(all, CheckGuidance(out.Text)...)
	all = append(all, CheckGrounding(out, index, refs)...)
	return all
}
