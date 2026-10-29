// cases.go 案件实体与引用仓储（v0.2 M1，docs/plan-v0.2.md §3.1/§3.3）。
// cases 表是一切案件边界；law_refs / case_refs 是法条与类案的引用留痕表——
// D2：法条不是事实、类案要旨不是本案事实，二者都不进 facts、不进五态、
// 不参与状态升级；留痕是「我引用了什么」的证据快照，不得用于再检索召回。
package storage

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/futuremeng/OpenCourt/core/domain"
)

// casesSchema 案件表与引用表结构（与 plan-v0.2 §3.1 DDL 一致）。
const casesSchema = `
CREATE TABLE IF NOT EXISTS cases (
    case_id     TEXT PRIMARY KEY,
    title       TEXT NOT NULL,
    case_type   TEXT,
    stage       TEXT NOT NULL,
    visibility  TEXT NOT NULL DEFAULT 'BOTH',
    created_at  TIMESTAMP NOT NULL
);
CREATE TABLE IF NOT EXISTS law_refs (
    ref_id         TEXT PRIMARY KEY,
    case_id        TEXT NOT NULL,
    article_id     TEXT NOT NULL,
    law_name       TEXT NOT NULL,
    article_no     TEXT,
    quote          TEXT,
    effective_date TEXT,
    status         TEXT NOT NULL,
    confirm_status TEXT NOT NULL,
    verify_result  TEXT,
    at_date        TEXT,
    created_at     TIMESTAMP NOT NULL
);
CREATE TABLE IF NOT EXISTS case_refs (
    ref_id         TEXT PRIMARY KEY,
    case_id        TEXT NOT NULL,
    source_case_id TEXT NOT NULL,
    source_url     TEXT,
    case_name      TEXT,
    court          TEXT,
    cause          TEXT,
    judgment_date  TEXT,
    legal_basis    TEXT,
    summary        TEXT,
    retrieved_at   TIMESTAMP NOT NULL,
    facets_json    TEXT
);
CREATE INDEX IF NOT EXISTS idx_law_refs_case ON law_refs (case_id);
CREATE INDEX IF NOT EXISTS idx_case_refs_case ON case_refs (case_id);
`

// CreateCase 新建案件（case_id/title 必填；stage 缺省 CONSULT，必须合法）。
func (s *Store) CreateCase(c domain.Case) error {
	if c.CaseID == "" || c.Title == "" {
		return fmt.Errorf("案件 ID 与标题不能为空")
	}
	if c.Stage == "" {
		c.Stage = domain.StageConsult
	}
	if !c.Stage.Valid() {
		return fmt.Errorf("非法案件阶段: %q（允许: CONSULT/FILING/FIRST_TRIAL/APPEAL/SECOND_TRIAL）", c.Stage)
	}
	if c.Visibility == "" {
		c.Visibility = domain.VisBoth
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(
		`INSERT INTO cases (case_id, title, case_type, stage, visibility, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		c.CaseID, c.Title, c.CaseType, string(c.Stage), string(c.Visibility), c.CreatedAt,
	)
	return err
}

// ListCases 列出全部案件（创建时间倒序）。
func (s *Store) ListCases() ([]domain.Case, error) {
	rows, err := s.db.Query(
		`SELECT case_id, title, case_type, stage, visibility, created_at
		 FROM cases ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Case{}
	for rows.Next() {
		c, err := scanCase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCase 按 ID 读取案件。
func (s *Store) GetCase(caseID string) (domain.Case, error) {
	row := s.db.QueryRow(
		`SELECT case_id, title, case_type, stage, visibility, created_at FROM cases WHERE case_id = ?`,
		caseID)
	c, err := scanCase(row)
	if err != nil {
		return domain.Case{}, fmt.Errorf("案件不存在: %s", caseID)
	}
	return c, nil
}

func scanCase(row scanner) (domain.Case, error) {
	var (
		c         domain.Case
		stage     string
		vis       string
		caseType  sql.NullString
		createdAt time.Time
	)
	if err := row.Scan(&c.CaseID, &c.Title, &caseType, &stage, &vis, &createdAt); err != nil {
		return domain.Case{}, err
	}
	c.CaseType = caseType.String
	c.Stage = domain.CaseStage(stage)
	c.Visibility = domain.Visibility(vis)
	c.CreatedAt = createdAt
	return c, nil
}

// WithCase 返回限定案件的仓储视图（plan-v0.2 D5）：所有读写自动带
// WHERE case_id = ?，跨案件访问在 SQL 层不可能发生——不靠 UI 隐藏。
// 旧的全局仓储方法已从 Store 移除（编译期强制，§9），业务读写一律经本视图。
func (s *Store) WithCase(caseID string) *CaseScope {
	return &CaseScope{store: s, caseID: caseID}
}

// CaseScope 案件限定仓储视图：事实/冲突/文档/要素与引用留痕的唯一读写入口。
type CaseScope struct {
	store  *Store
	caseID string
}

// CaseID 本视图限定的案件 ID。
func (sc *CaseScope) CaseID() string { return sc.caseID }

// db 底层连接（所有语句强制带 case_id 条件）。
func (sc *CaseScope) db() *sql.DB { return sc.store.db }

// newRefID 生成随机引用 ID（LAW-<hex> / CASE-<hex>）。
func newRefID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b)
}

// AddLawRef 登记法条引用留痕（挂当前案件）。RefID 缺省自动生成；
// Status/ConfirmStatus 缺省保守取 UNVERIFIED/pending——未核验不得显示为已确认。
func (sc *CaseScope) AddLawRef(r domain.LawRef) error {
	if r.LawName == "" {
		return fmt.Errorf("法条引用必须含法名")
	}
	if r.RefID == "" {
		r.RefID = newRefID("LAW")
	}
	if r.Status == "" {
		r.Status = "UNVERIFIED"
	}
	if r.ConfirmStatus == "" {
		r.ConfirmStatus = "pending"
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	_, err := sc.db().Exec(
		`INSERT INTO law_refs (ref_id, case_id, article_id, law_name, article_no, quote,
			effective_date, status, confirm_status, verify_result, at_date, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.RefID, sc.caseID, r.ArticleID, r.LawName, r.ArticleNo, r.Quote,
		r.EffectiveDate, r.Status, r.ConfirmStatus, r.VerifyResult, r.AtDate, r.CreatedAt,
	)
	return err
}

// ListLawRefs 列出本案全部法条引用（创建时间倒序）。
func (sc *CaseScope) ListLawRefs() ([]domain.LawRef, error) {
	rows, err := sc.db().Query(
		`SELECT ref_id, case_id, article_id, law_name, article_no, quote, effective_date,
			status, confirm_status, verify_result, at_date, created_at
		 FROM law_refs WHERE case_id = ? ORDER BY created_at DESC`, sc.caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.LawRef{}
	for rows.Next() {
		var (
			r         domain.LawRef
			articleID sql.NullString
			lawName   sql.NullString
			articleNo sql.NullString
			quote     sql.NullString
			eff       sql.NullString
			at        sql.NullString
			verifyRes sql.NullString
			status    string
			confirm   string
			createdAt time.Time
		)
		if err := rows.Scan(&r.RefID, &r.CaseID, &articleID, &lawName, &articleNo, &quote,
			&eff, &status, &confirm, &verifyRes, &at, &createdAt); err != nil {
			return nil, err
		}
		r.ArticleID, r.LawName, r.ArticleNo = articleID.String, lawName.String, articleNo.String
		r.Quote, r.EffectiveDate = quote.String, eff.String
		r.AtDate, r.VerifyResult = at.String, verifyRes.String
		r.Status, r.ConfirmStatus = status, confirm
		r.CreatedAt = createdAt
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpdateLawRefVerify 更新核验结论（verify_result 采用 OpenLaw 错误码原样，不做二次翻译）。
// 跨案件 ref 在 SQL 层不可达。
func (sc *CaseScope) UpdateLawRefVerify(refID, result string) error {
	res, err := sc.db().Exec(
		`UPDATE law_refs SET verify_result = ? WHERE ref_id = ? AND case_id = ?`,
		result, refID, sc.caseID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("法条引用不存在: %s", refID)
	}
	return nil
}

// AddCaseRef 登记类案引用留痕（挂当前案件；summary 必须已由调用方强制脱敏）。
func (sc *CaseScope) AddCaseRef(r domain.CaseRef) error {
	if r.SourceCaseID == "" {
		return fmt.Errorf("类案引用必须含源案件标识")
	}
	if r.RefID == "" {
		r.RefID = newRefID("CASE")
	}
	if r.RetrievedAt.IsZero() {
		r.RetrievedAt = time.Now().UTC()
	}
	_, err := sc.db().Exec(
		`INSERT INTO case_refs (ref_id, case_id, source_case_id, source_url, case_name, court,
			cause, judgment_date, legal_basis, summary, retrieved_at, facets_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.RefID, sc.caseID, r.SourceCaseID, r.SourceURL, r.CaseName, r.Court,
		r.Cause, r.JudgmentDate, r.LegalBasis, r.Summary, r.RetrievedAt, r.FacetsJSON,
	)
	return err
}

// ListCaseRefs 列出本案全部类案引用（检索时间倒序）。
func (sc *CaseScope) ListCaseRefs() ([]domain.CaseRef, error) {
	rows, err := sc.db().Query(
		`SELECT ref_id, case_id, source_case_id, source_url, case_name, court, cause,
			judgment_date, legal_basis, summary, retrieved_at, facets_json
		 FROM case_refs WHERE case_id = ? ORDER BY retrieved_at DESC`, sc.caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.CaseRef{}
	for rows.Next() {
		var (
			r           domain.CaseRef
			src         sql.NullString
			url         sql.NullString
			name        sql.NullString
			court       sql.NullString
			cause       sql.NullString
			jdate       sql.NullString
			basis       sql.NullString
			summary     sql.NullString
			facets      sql.NullString
			retrievedAt time.Time
		)
		if err := rows.Scan(&r.RefID, &r.CaseID, &src, &url, &name, &court, &cause,
			&jdate, &basis, &summary, &retrievedAt, &facets); err != nil {
			return nil, err
		}
		r.SourceCaseID, r.SourceURL, r.CaseName = src.String, url.String, name.String
		r.Court, r.Cause = court.String, cause.String
		r.JudgmentDate, r.LegalBasis = jdate.String, basis.String
		r.Summary, r.FacetsJSON = summary.String, facets.String
		r.RetrievedAt = retrievedAt
		out = append(out, r)
	}
	return out, rows.Err()
}
