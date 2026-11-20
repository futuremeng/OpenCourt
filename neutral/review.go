// 审校（plan-v0.4 D3/M7）：全部**确定性检查**优先——引用核验、回原文一致性、
// 五态合规、脱敏复查、推理链完整性。模型不参与审校判定；「风险说明」文本
// 只能补充在报告叙述层，且必过 gate。审校结论是代码仲裁，不是提示词自觉。
package neutral

import (
	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/verify"
)

// 审校检查项（封闭集）。
const (
	CheckLawCitation     = "law_citation"     // 法条引用核验（M2 LawVerifier 透传结果）
	CheckSourceLocate    = "source_locate"    // 事实引用回原文一致性（verify.Locate）
	CheckStateCompliance = "state_compliance" // 五态合规（无依据事实不得入库等）
	CheckRedaction       = "redaction"        // 脱敏复查
	CheckReasoningChain  = "reasoning_chain"  // 推理链完整性（结论可回溯 fact_ids）
)

// 审校级别：red = 报告显式标红并阻断导出；warn = 上浮提示，不阻断。
const (
	LevelRed  = "red"
	LevelWarn = "warn"
)

// ReviewIssue 单条审校发现。
type ReviewIssue struct {
	Check   string `json:"check"`
	Level   string `json:"level"`
	Ref     string `json:"ref,omitempty"`
	Message string `json:"message"`
}

// SimRoundView 模拟轮次的审校视图。
type SimRoundView struct {
	RoundID string
	Phase   string
	Blocked bool
	FactIDs []string
}

// ReviewInput 审校输入：全部来自本案存储与留痕，无模型参与。
type ReviewInput struct {
	Facts     []domain.Fact
	Conflicts []domain.Conflict
	LawRefs   []domain.LawRef
	Documents map[string]*verify.DocumentText // file → 文本层
	SimRounds []SimRoundView
}

// ReviewReport 审校结论。Red > 0 时导出必须被阻断。
type ReviewReport struct {
	Issues []ReviewIssue `json:"issues"`
	Red    int           `json:"red"`
	Warn   int           `json:"warn"`
}

// Review 执行全部确定性审校。
func Review(in ReviewInput) ReviewReport {
	rep := ReviewReport{Issues: []ReviewIssue{}}
	note := func(check, level, ref, msg string) {
		rep.Issues = append(rep.Issues, ReviewIssue{Check: check, Level: level, Ref: ref, Message: msg})
		if level == LevelRed {
			rep.Red++
		} else {
			rep.Warn++
		}
	}

	// 1. 法条引用核验：未核验/核验失败一律上浮；失败标红（fail-closed）。
	for _, lr := range in.LawRefs {
		switch {
		case lr.VerifyResult == "":
			note(CheckLawCitation, LevelWarn, lr.RefID, "法条引用未核验："+lr.LawName+lr.ArticleNo)
		case lr.VerifyResult != "OK":
			note(CheckLawCitation, LevelRed, lr.RefID, "法条引用核验失败（"+lr.VerifyResult+"）："+lr.LawName+lr.ArticleNo)
		}
		if lr.ConfirmStatus == "PENDING" {
			note(CheckLawCitation, LevelWarn, lr.RefID, "引用效力待人工确认")
		}
	}

	factIndex := map[string]domain.Fact{}
	for _, f := range in.Facts {
		factIndex[f.FactID] = f
	}

	// 2. 回原文一致性 + 3. 五态合规。
	for _, f := range in.Facts {
		if f.State == domain.StateUnsupported {
			note(CheckStateCompliance, LevelRed, f.FactID, "「无依据」事实不得进入卷宗——应删除或补来源")
			continue
		}
		if f.Citation == nil {
			if f.State == domain.StateTracedConfirmed {
				note(CheckSourceLocate, LevelRed, f.FactID, "强态事实缺引用坐标（已溯源确认必须可回原文）")
			}
			continue
		}
		if !verify.Locate(*f.Citation, in.Documents) {
			note(CheckSourceLocate, LevelRed, f.FactID, "引用回原文定位失败（原文已变更或坐标错误）："+f.Citation.SourceFile)
		}
	}

	// 4. 推理链完整性：非被拦截轮次的结论必须可回溯到存在的事实。
	for _, r := range in.SimRounds {
		if r.Blocked {
			continue
		}
		if len(r.FactIDs) == 0 {
			note(CheckReasoningChain, LevelRed, r.RoundID, "轮次结论未引用任何事实（推理链断裂）")
			continue
		}
		for _, id := range r.FactIDs {
			if _, ok := factIndex[id]; !ok {
				note(CheckReasoningChain, LevelRed, r.RoundID, "轮次引用了不存在的事实 "+id+"（推理链断裂）")
			}
		}
	}

	// 冲突并列是展示要求，未解决冲突上浮提示。
	for _, c := range in.Conflicts {
		if !c.IsResolved {
			note(CheckStateCompliance, LevelWarn, c.ConflictID, "存在未解决冲突，报告将原样并列："+c.ConflictPoint)
		}
	}
	return rep
}

// AddRedactionIssue 供导出层把脱敏复查/门禁结果并入审校报告。
func (r *ReviewReport) AddRedactionIssue(level, msg string) {
	r.Issues = append(r.Issues, ReviewIssue{Check: CheckRedaction, Level: level, Message: msg})
	if level == LevelRed {
		r.Red++
	} else {
		r.Warn++
	}
}
