package domain

import "time"

// DefaultCaseID 存量数据与未显式建案场景所属案件（v0.2 M1 迁移语义，
// 见 docs/plan-v0.2.md D1：存量事实/冲突/假设/文档/要素一律归入本案，不丢历史）。
const DefaultCaseID = "CASE-DEFAULT"

// Valid 报告案件阶段是否在枚举内（开发文档 6.6）。
func (s CaseStage) Valid() bool {
	switch s {
	case StageConsult, StageFiling, StageFirstTrial, StageAppeal, StageSecondTrial:
		return true
	}
	return false
}

// Case 案件实体（v0.2 M1）：一切事实、冲突、文档、要素与引用的边界。
// case_type 为自由文本（案由不自动判定）；stage 必须是 CaseStage 枚举码。
type Case struct {
	CaseID     string     `json:"case_id"`
	Title      string     `json:"title"`
	CaseType   string     `json:"case_type,omitempty"`
	Stage      CaseStage  `json:"stage"`
	Visibility Visibility `json:"visibility"`
	CreatedAt  time.Time  `json:"created_at"`
}

// LawRef 法条引用留痕（D2：法条不是事实，不进五态，不入 facts）。
// VerifyResult 为空 = 尚未核验；Status/ConfirmStatus 一律由 OpenLaw 原样透传，
// OpenCourt 不自行推导效力状态（D0 语料所有权 / 违例 V1 修法）。
type LawRef struct {
	RefID     string `json:"ref_id"` // LAW-<hex>
	CaseID    string `json:"case_id"`
	ArticleID string `json:"article_id"` // OpenLaw 稳定标识
	// LawID OpenLaw 法规标识（relation 查询键）。SDK 现不返回 law_id——
	// 为空时关系查询 fail-closed 报错，绝不猜（缺口见 CHANGELOG）。
	LawID         string    `json:"law_id,omitempty"`
	LawName       string    `json:"law_name"`
	ArticleNo     string    `json:"article_no,omitempty"`
	Quote         string    `json:"quote,omitempty"` // 文书中的引文（核验用，非语料副本）
	EffectiveDate string    `json:"effective_date,omitempty"`
	Status        string    `json:"status"` // EFFECTIVE / AMENDED / UNVERIFIED（由 OpenLaw 透传）
	ConfirmStatus string    `json:"confirm_status"`
	VerifyResult  string    `json:"verify_result,omitempty"` // 空=未核验；否则 OK 或 OpenLaw 错误码（OL-1001..）
	AtDate        string    `json:"at_date,omitempty"`       // 行为发生时点（版本回溯基准）
	CreatedAt     time.Time `json:"created_at"`
}

// CaseRef 类案引用留痕（D2：类案要旨不是本案事实，永不进五态、不可被质证升级）。
type CaseRef struct {
	RefID        string    `json:"ref_id"` // CASE-<hex>
	CaseID       string    `json:"case_id"`
	SourceCaseID string    `json:"source_case_id"` // LocalCases 内部 id（字符串化）
	SourceURL    string    `json:"source_url,omitempty"`
	CaseName     string    `json:"case_name,omitempty"`
	Court        string    `json:"court,omitempty"`
	Cause        string    `json:"cause,omitempty"`
	JudgmentDate string    `json:"judgment_date,omitempty"`
	LegalBasis   string    `json:"legal_basis,omitempty"`
	Summary      string    `json:"summary,omitempty"` // 裁判要旨（已强制脱敏）
	RetrievedAt  time.Time `json:"retrieved_at"`
	FacetsJSON   string    `json:"facets_json,omitempty"` // 检索分布（呈现为分布统计，非裁判概率，D6）
}
