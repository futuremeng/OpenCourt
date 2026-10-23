package domain

import "time"

// Visibility 数据可见性标签（开发文档 6.4 权限与可见性层）。
// 所有检索先过权限过滤再召回，确保数据隔离。
type Visibility string

const (
	VisPlaintiffOnly Visibility = "PLAINTIFF_ONLY" // 仅原告可见
	VisDefendantOnly Visibility = "DEFENDANT_ONLY" // 仅被告可见
	VisBoth          Visibility = "BOTH"           // 双方可见
	VisCourtOnly     Visibility = "COURT_ONLY"     // 仅法院可见
)

// CaseStage 案件状态层（开发文档 6.6）：咨询、起诉、一审、上诉、二审。
type CaseStage string

const (
	StageConsult     CaseStage = "CONSULT"      // 咨询
	StageFiling      CaseStage = "FILING"       // 起诉
	StageFirstTrial  CaseStage = "FIRST_TRIAL"  // 一审
	StageAppeal      CaseStage = "APPEAL"       // 上诉
	StageSecondTrial CaseStage = "SECOND_TRIAL" // 二审
)

// Citation 引用溯源（开发文档 6.5）：每条结论必须挂到具体证据编号、卷宗页码、
// 原文片段、法条版本。无法定位到页码与原文片段的引用视为无效。
type Citation struct {
	SourceFile  string // 来源文件（卷宗）
	PageNum     int    // 页码（>=1 才有效）
	Snippet     string // 原文片段
	LawVersion  string // 关联法条版本（可空）
	LawRepealed bool   // 关联法条是否已废止
}

// Valid 报告引用是否满足最低溯源要求（有页码且有原文片段）。
func (c Citation) Valid() bool {
	return c.SourceFile != "" && c.PageNum >= 1 && c.Snippet != ""
}

// Fact 事实表（开发文档 6.1）。
type Fact struct {
	FactID            string     `json:"fact_id"`
	Description       string     `json:"description"`
	State             FactState  `json:"state"`
	Citation          *Citation  `json:"citation,omitempty"`
	RelatedElement    string     `json:"related_element,omitempty"` // 关联要件
	ConflictID        string     `json:"conflict_id,omitempty"`
	AssumptionSource  string     `json:"assumption_source,omitempty"`
	Visibility        Visibility `json:"visibility"`
	IsManualConfirmed bool       `json:"is_manual_confirmed"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// Conflict 冲突表（开发文档 6.2）。冲突不自动消解，必须并列展示。
type Conflict struct {
	ConflictID       string   `json:"conflict_id"`
	FactIDs          []string `json:"fact_ids"`
	ConflictPoint    string   `json:"conflict_point"`
	ResolutionMethod string   `json:"resolution_method,omitempty"` // 可能解决方式（仅提示，不代做决定）
	IsResolved       bool     `json:"is_resolved"`
}

// Assumption 假设表（开发文档 6.3）。基于假设的结论必须继承“基于假设”标签，
// 假设被推翻时需能追溯所有受影响结论。
type Assumption struct {
	AssumptionID         string   `json:"assumption_id"`
	Content              string   `json:"content"`
	Proposer             string   `json:"proposer"`
	BasedOn              string   `json:"based_on"` // 基于哪个事实或主张
	DependentConclusions []string `json:"dependent_conclusions,omitempty"`
	AffectedConclusions  []string `json:"affected_conclusions,omitempty"` // 被推翻后受影响的结论列表
	IsRejected           bool     `json:"is_rejected"`
}

// Conclusion 推演结论。基于假设的结论必须继承 AssumptionIDs 标签（状态机规则 2）。
type Conclusion struct {
	ConclusionID  string   `json:"conclusion_id"`
	Text          string   `json:"text"`
	FactIDs       []string `json:"fact_ids,omitempty"`
	AssumptionIDs []string `json:"assumption_ids,omitempty"`
	HasSource     bool     `json:"has_source"` // 输出必须带来源
}

// BasedOnAssumption 报告结论是否继承了“基于假设”标签。
func (c Conclusion) BasedOnAssumption() bool {
	return len(c.AssumptionIDs) > 0
}
