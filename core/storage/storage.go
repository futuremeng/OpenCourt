// Package storage 本地 SQLite 存储（开发文档 第六章数据模型）。
// 桌面端数据全部落在用户本机数据目录，默认不联网、不上传。
package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/futuremeng/OpenCourt/core/domain"

	_ "modernc.org/sqlite"
)

// Store SQLite 仓储。
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS facts (
    fact_id             TEXT PRIMARY KEY,
    description         TEXT NOT NULL,
    state               TEXT NOT NULL,
    citation_json       TEXT,
    related_element     TEXT,
    conflict_id         TEXT,
    assumption_source   TEXT,
    visibility          TEXT NOT NULL DEFAULT 'BOTH',
    is_manual_confirmed INTEGER NOT NULL DEFAULT 0,
    updated_at          TIMESTAMP NOT NULL
);
CREATE TABLE IF NOT EXISTS conflicts (
    conflict_id       TEXT PRIMARY KEY,
    fact_ids_json     TEXT NOT NULL,
    conflict_point    TEXT NOT NULL,
    resolution_method TEXT,
    is_resolved       INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS assumptions (
    assumption_id         TEXT PRIMARY KEY,
    content               TEXT NOT NULL,
    proposer              TEXT NOT NULL,
    based_on              TEXT NOT NULL,
    dependent_json        TEXT,
    affected_json         TEXT,
    is_rejected           INTEGER NOT NULL DEFAULT 0
);
`

// Open 打开（或创建）SQLite 数据库并完成迁移。
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}
	return &Store{db: db}, nil
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// CreateFact 插入事实。状态必须合法。
func (s *Store) CreateFact(f domain.Fact) error {
	if !f.State.Valid() {
		return fmt.Errorf("非法事实状态: %q", f.State)
	}
	if f.UpdatedAt.IsZero() {
		f.UpdatedAt = time.Now().UTC()
	}
	cj := ""
	if f.Citation != nil {
		b, err := json.Marshal(f.Citation)
		if err != nil {
			return err
		}
		cj = string(b)
	}
	_, err := s.db.Exec(
		`INSERT INTO facts (fact_id, description, state, citation_json, related_element,
			conflict_id, assumption_source, visibility, is_manual_confirmed, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.FactID, f.Description, string(f.State), cj, f.RelatedElement,
		f.ConflictID, f.AssumptionSource, string(f.Visibility), f.IsManualConfirmed, f.UpdatedAt,
	)
	return err
}

// UpdateFactState 更新事实状态（调用方必须已通过 statemachine 仲裁）。
func (s *Store) UpdateFactState(factID string, state domain.FactState) error {
	res, err := s.db.Exec(
		`UPDATE facts SET state = ?, updated_at = ? WHERE fact_id = ?`,
		string(state), time.Now().UTC(), factID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("事实不存在: %s", factID)
	}
	return nil
}

// GetFact 按 ID 读取事实。
func (s *Store) GetFact(factID string) (domain.Fact, error) {
	row := s.db.QueryRow(
		`SELECT fact_id, description, state, citation_json, related_element,
			conflict_id, assumption_source, visibility, is_manual_confirmed, updated_at
		 FROM facts WHERE fact_id = ?`, factID)
	return scanFact(row)
}

// ListFacts 列出全部事实（按更新时间倒序）。
func (s *Store) ListFacts() ([]domain.Fact, error) {
	rows, err := s.db.Query(
		`SELECT fact_id, description, state, citation_json, related_element,
			conflict_id, assumption_source, visibility, is_manual_confirmed, updated_at
		 FROM facts ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Fact{}
	for rows.Next() {
		f, err := scanFact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// CreateConflict 插入冲突记录（冲突不自动消解，is_resolved 默认 false）。
func (s *Store) CreateConflict(c domain.Conflict) error {
	if len(c.FactIDs) < 2 {
		return fmt.Errorf("冲突至少涉及两份事实: %v", c.FactIDs)
	}
	if c.ConflictPoint == "" {
		return fmt.Errorf("冲突点描述不能为空")
	}
	fj, err := json.Marshal(c.FactIDs)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO conflicts (conflict_id, fact_ids_json, conflict_point, resolution_method, is_resolved)
		 VALUES (?, ?, ?, ?, 0)`,
		c.ConflictID, string(fj), c.ConflictPoint, c.ResolutionMethod,
	)
	return err
}

// ListConflicts 列出全部冲突。
func (s *Store) ListConflicts() ([]domain.Conflict, error) {
	rows, err := s.db.Query(
		`SELECT conflict_id, fact_ids_json, conflict_point, resolution_method, is_resolved FROM conflicts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Conflict{}
	for rows.Next() {
		var (
			id, fj, point, method string
			resolved              int
		)
		if err := rows.Scan(&id, &fj, &point, &method, &resolved); err != nil {
			return nil, err
		}
		var ids []string
		if err := json.Unmarshal([]byte(fj), &ids); err != nil {
			return nil, err
		}
		out = append(out, domain.Conflict{
			ConflictID: id, FactIDs: ids, ConflictPoint: point,
			ResolutionMethod: method, IsResolved: resolved != 0,
		})
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanFact(row scanner) (domain.Fact, error) {
	var (
		f              domain.Fact
		state, vis, cj string
		rel, cid, asum string
		manual         int
		updatedAt      time.Time
	)
	if err := row.Scan(&f.FactID, &f.Description, &state, &cj, &rel, &cid, &asum, &vis, &manual, &updatedAt); err != nil {
		return domain.Fact{}, err
	}
	f.State = domain.FactState(state)
	f.Visibility = domain.Visibility(vis)
	f.RelatedElement, f.ConflictID, f.AssumptionSource = rel, cid, asum
	f.IsManualConfirmed = manual != 0
	f.UpdatedAt = updatedAt
	if cj != "" {
		var c domain.Citation
		if err := json.Unmarshal([]byte(cj), &c); err != nil {
			return domain.Fact{}, err
		}
		f.Citation = &c
	}
	return f, nil
}
