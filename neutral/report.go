// 报告导出（plan-v0.4 D3/M7）：确定性拼装 Markdown，不依赖模型。
// 固定免责头部；强态事实进事实段落，弱态一律进「待人工确认清单」——
// 弱态事实出现在事实段落属实现违例（合规测试锁定）；冲突原样并列；
// 盲区只给「无法判断 + 补证方向」；模拟轨迹含被拦截轮。
package neutral

import (
	"fmt"
	"strings"
	"time"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/flow"
)

// Disclaimer 固定免责标注——每份报告头部必带，不可由调用方移除。
const Disclaimer = "> **免责标注**：本报告由 AI 辅助整理，不构成裁判或法律意见。"

// ReportInput 报告拼装输入。
type ReportInput struct {
	CaseTitle   string
	CaseID      string
	GeneratedAt time.Time
	Facts       []domain.Fact
	Conflicts   []domain.Conflict
	LawRefs     []domain.LawRef
	Blindspots  []Blindspot
	SimRounds   []SimRoundView
	Review      ReviewReport
}

// RenderMarkdown 确定性拼装报告全文。同一输入必得同一文本（除时间戳）——
// 报告即留痕，禁止模型参与拼装。
func RenderMarkdown(in ReportInput) string {
	var b strings.Builder
	b.WriteString("# 案件工作报告：" + in.CaseTitle + "\n\n")
	b.WriteString(Disclaimer + "\n\n")
	fmt.Fprintf(&b, "- 案件：%s\n- 生成时间：%s\n\n", in.CaseID, in.GeneratedAt.Format("2006-01-02 15:04:05 MST"))

	b.WriteString("## 一、事实清单\n\n")
	strong := 0
	for _, f := range in.Facts {
		if f.State != domain.StateTracedConfirmed {
			continue // 弱态不进事实段落——由「待人工确认清单」承载
		}
		strong++
		fmt.Fprintf(&b, "- [%s] %s", f.FactID, f.Description)
		if f.Citation != nil && f.Citation.Valid() {
			fmt.Fprintf(&b, "（来源：%s 第 %d 页，片段「%s」）", f.Citation.SourceFile, f.Citation.PageNum, f.Citation.Snippet)
		} else {
			b.WriteString(" **[缺来源]**")
		}
		b.WriteString("\n")
	}
	if strong == 0 {
		b.WriteString("（无已溯源确认的强态事实）\n")
	}

	b.WriteString("\n## 二、争点与冲突（原样并列，不自动消解）\n\n")
	if len(in.Conflicts) == 0 {
		b.WriteString("（无登记冲突）\n")
	}
	for _, c := range in.Conflicts {
		state := "未解决"
		if c.IsResolved {
			state = "已人工解决"
		}
		fmt.Fprintf(&b, "- [%s] %s（%s）· 涉及事实：%s", c.ConflictID, c.ConflictPoint, state, strings.Join(c.FactIDs, "、"))
		if c.ResolutionMethod != "" {
			fmt.Fprintf(&b, " · 可能解决方式（仅提示）：%s", c.ResolutionMethod)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n## 三、盲区（无法判断 + 补证方向）\n\n")
	if len(in.Blindspots) == 0 {
		b.WriteString("（要件匹配暂无盲区）\n")
	}
	for _, sp := range in.Blindspots {
		fmt.Fprintf(&b, "- 无法判断：%s\n  - 补证方向：%s\n", sp.Question, sp.Supplement)
	}

	b.WriteString("\n## 四、模拟轮次轨迹\n\n")
	if len(in.SimRounds) == 0 {
		b.WriteString("（未进行对抗推演）\n")
	}
	for _, r := range in.SimRounds {
		if r.Blocked {
			fmt.Fprintf(&b, "- [%s] %s：**[已被检查拦截]**（内容未采信入库）\n", r.RoundID, flow.PhaseName(flow.Phase(r.Phase)))
			continue
		}
		fmt.Fprintf(&b, "- [%s] %s · 依据事实：%s\n", r.RoundID, flow.PhaseName(flow.Phase(r.Phase)), strings.Join(r.FactIDs, "、"))
	}

	b.WriteString("\n## 五、法条引用留痕\n\n")
	if len(in.LawRefs) == 0 {
		b.WriteString("（无法条引用）\n")
	}
	for _, lr := range in.LawRefs {
		fmt.Fprintf(&b, "- [%s] %s%s（效力：%s · 核验：%s）\n", lr.RefID, lr.LawName, lr.ArticleNo, lr.Status, lawVerifyText(lr))
	}

	b.WriteString("\n## 六、待人工确认清单\n\n")
	weak := 0
	for _, f := range in.Facts {
		if f.State == domain.StateTracedConfirmed {
			continue
		}
		weak++
		fmt.Fprintf(&b, "- [%s] %s —— 状态：**%s**（弱态，不得进入事实段落）", f.FactID, f.Description, f.State.DisplayName())
		if f.Citation != nil && f.Citation.Valid() {
			fmt.Fprintf(&b, "（线索来源：%s 第 %d 页）", f.Citation.SourceFile, f.Citation.PageNum)
		}
		b.WriteString("\n")
	}
	for _, iss := range in.Review.Issues {
		if iss.Level == LevelRed {
			fmt.Fprintf(&b, "- **[标红]** %s（%s：%s）\n", iss.Message, iss.Check, iss.Ref)
		} else {
			fmt.Fprintf(&b, "- [提示] %s（%s）\n", iss.Message, iss.Check)
		}
	}
	if weak == 0 && len(in.Review.Issues) == 0 {
		b.WriteString("（无待确认项）\n")
	}
	return b.String()
}

func lawVerifyText(lr domain.LawRef) string {
	switch lr.VerifyResult {
	case "":
		return "未核验"
	case "OK":
		return "通过"
	default:
		return "失败（" + lr.VerifyResult + "）"
	}
}
