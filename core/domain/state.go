// Package domain 定义 OpenCourt 的核心数据模型：事实五态、事实、冲突、假设、
// 可见性标签与案件状态层（开发文档 第三章 3.1、第六章）。
// 本包不依赖任何内部模块，是四层架构依赖链的最底层。
package domain

import "fmt"

// FactState 事实五态（开发文档 3.1）。
// 系统不将文本视为绝对真理，所有推理严格受限于事实的当前状态。
type FactState string

const (
	// StateTracedConfirmed 已溯源确认：本方卷宗有原文定位到页码和片段，且无内部矛盾。
	// 可直接用于推理，输出时必须带来源。
	StateTracedConfirmed FactState = "TRACED_CONFIRMED"

	// StateSingleSource 单源未补强：仅有一份材料支持。
	// 不能直接下结论，需提示补强方向。
	StateSingleSource FactState = "SINGLE_SOURCE"

	// StateOpponentClaim 对方主张未验证：来自对方文书或陈述，本方无法证实或证伪。
	// 显式标注“对方主张”，禁止出现在“事实”段落。
	StateOpponentClaim FactState = "OPPONENT_CLAIM"

	// StateConflicted 存在冲突：两份以上材料对同一事件说法不一致。
	// 不自动选择，并列列出并标出冲突点。
	StateConflicted FactState = "CONFLICTED"

	// StateUnsupported 无依据：无任何材料支持。
	// 模型生成此类内容直接被门禁拦截。
	StateUnsupported FactState = "UNSUPPORTED"
)

// DisplayName 返回状态的中文显示名。
func (s FactState) DisplayName() string {
	switch s {
	case StateTracedConfirmed:
		return "已溯源确认"
	case StateSingleSource:
		return "单源未补强"
	case StateOpponentClaim:
		return "对方主张未验证"
	case StateConflicted:
		return "存在冲突"
	case StateUnsupported:
		return "无依据"
	default:
		return "未知状态"
	}
}

// Valid 报告 s 是否为合法的五态之一。
func (s FactState) Valid() bool {
	switch s {
	case StateTracedConfirmed, StateSingleSource, StateOpponentClaim, StateConflicted, StateUnsupported:
		return true
	default:
		return false
	}
}

// ParseFactState 解析状态字符串（接受枚举码或中文名），非法时返回错误。
func ParseFactState(s string) (FactState, error) {
	switch FactState(s) {
	case StateTracedConfirmed, StateSingleSource, StateOpponentClaim, StateConflicted, StateUnsupported:
		return FactState(s), nil
	}
	for _, st := range AllStates() {
		if st.DisplayName() == s {
			return st, nil
		}
	}
	return "", fmt.Errorf("非法事实状态: %q", s)
}

// AllStates 返回全部五态，顺序与开发文档 3.1 表格一致。
func AllStates() []FactState {
	return []FactState{
		StateTracedConfirmed,
		StateSingleSource,
		StateOpponentClaim,
		StateConflicted,
		StateUnsupported,
	}
}
