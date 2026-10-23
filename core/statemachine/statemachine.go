// Package statemachine 实现事实状态机的三条硬规则（开发文档 3.2）。
// 这是“代码仲裁优先于模型判断”原则的核心落点：状态升级只能经由本包的
// ArbitrateUpgrade 函数，大语言模型仅能提交候选升级请求，无权做最终裁决。
package statemachine

import (
	"fmt"

	"github.com/futuremeng/OpenCourt/core/domain"
)

// InitialState 返回事实入库时的初始状态：
//   - 有可定位的原文引用 → 单源未补强（尚未经核验器定位验证）
//   - 来自对方文书/陈述 → 对方主张未验证
//   - 无任何来源 → 无依据
func InitialState(f domain.Fact, fromOpponent bool) domain.FactState {
	if fromOpponent {
		return domain.StateOpponentClaim
	}
	if f.Citation != nil && f.Citation.Valid() {
		return domain.StateSingleSource
	}
	return domain.StateUnsupported
}

// UpgradeRequest 一次状态升级候选，只能由确定性代码仲裁。
type UpgradeRequest struct {
	Fact               domain.Fact
	Target             domain.FactState
	EvidenceImported   bool // 本方是否导入了新材料
	LocatedInSource    bool // 核验器是否能在原文定位（verify 模块的确定性结果）
	NoInternalConflict bool // 汇聚器确认无内部矛盾
}

// ArbitrateUpgrade 按“证据驱动升级”规则仲裁状态变更（开发文档 3.2 规则 1）：
// 状态只能靠证据升级，不能靠推理升级。返回 nil 表示允许升级。
func ArbitrateUpgrade(req UpgradeRequest) error {
	if !req.Target.Valid() {
		return fmt.Errorf("目标状态非法: %q", req.Target)
	}
	from := req.Fact.State
	if from == req.Target {
		return fmt.Errorf("状态未变化: %s", from.DisplayName())
	}

	switch req.Target {
	case domain.StateTracedConfirmed:
		// 唯一路径：本方导入新材料 且 核验器能在原文定位 且 无内部矛盾。
		if !req.EvidenceImported {
			return fmt.Errorf("拒绝升级到「已溯源确认」：未导入新证据，状态只能靠证据升级，不能靠推理升级")
		}
		if !req.LocatedInSource {
			return fmt.Errorf("拒绝升级到「已溯源确认」：核验器无法在原文定位引用")
		}
		if !req.NoInternalConflict {
			return fmt.Errorf("拒绝升级到「已溯源确认」：存在内部矛盾")
		}
		return nil

	case domain.StateUnsupported:
		// 降级为“无依据”总是允许（更保守的方向不受限）。
		return nil

	case domain.StateConflicted:
		// 进入冲突态由汇聚器标记，属于事实描述，不是升级，允许。
		return nil

	case domain.StateSingleSource, domain.StateOpponentClaim:
		// 从已确认态退回弱态属于降级，允许；向上进入这两个态要求有对应来源。
		if from == domain.StateUnsupported && !req.EvidenceImported {
			return fmt.Errorf("拒绝进入 %s：无任何材料支持时不得进入弱事实态", req.Target.DisplayName())
		}
		return nil

	default:
		return fmt.Errorf("未知目标状态: %s", req.Target)
	}
}

// ApplyUpgrade 校验并执行状态变更，返回变更后的事实副本。
func ApplyUpgrade(req UpgradeRequest) (domain.Fact, error) {
	if err := ArbitrateUpgrade(req); err != nil {
		return domain.Fact{}, err
	}
	f := req.Fact
	f.State = req.Target
	return f, nil
}

// ResolveConflict 冲突消解（开发文档 3.2 规则 3）：冲突不自动消解。
// 只有人工确认（resolverIsHuman=true）才允许标记为已解决；
// 模型或自动逻辑调用一律被拒绝。
func ResolveConflict(c domain.Conflict, resolverIsHuman bool) (domain.Conflict, error) {
	if !resolverIsHuman {
		return domain.Conflict{}, fmt.Errorf("拒绝自动消解冲突 %s：冲突必须并列展示，交由用户自行判断", c.ConflictID)
	}
	c.IsResolved = true
	return c, nil
}

// RejectAssumption 假设被推翻时，回填所有依赖它的结论到 AffectedConclusions，
// 保证“假设标签贯穿、可追溯”（开发文档 3.2 规则 2）。
func RejectAssumption(a domain.Assumption, conclusions []domain.Conclusion) domain.Assumption {
	affected := make([]string, 0)
	for _, c := range conclusions {
		for _, id := range c.AssumptionIDs {
			if id == a.AssumptionID {
				affected = append(affected, c.ConclusionID)
				break
			}
		}
	}
	a.IsRejected = true
	a.AffectedConclusions = affected
	return a
}

// TagConclusionWithAssumptions 让结论继承假设标签。任何依赖假设的结论
// 在生成时必须经过此函数，禁止绕过。
func TagConclusionWithAssumptions(c domain.Conclusion, assumptionIDs []string) domain.Conclusion {
	c.AssumptionIDs = append(c.AssumptionIDs, assumptionIDs...)
	return c
}
