// Package storage 本地 SQLite 存储（开发文档 第六章数据模型）。
// 桌面端数据全部落在用户本机数据目录，默认不联网、不上传。
// v0.2 M1（docs/plan-v0.2.md D1/D5）：事实/冲突/假设/文档/要素全部挂 case_id；
// 业务读写一律经 Store.WithCase(caseID) 案件限定视图——
// 跨案件访问在 SQL 层不可能发生（WHERE 强制），不靠 UI 隐藏。
package storage

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/extract"
	"github.com/futuremeng/OpenCourt/core/verify"

	_ "modernc.org/sqlite"
)

// Store SQLite 仓储：连接管理与案件级操作（cases 表）。
// 业务数据读写不在本层——一律 WithCase 后经 CaseScope（编译期强制）。
type Store struct {
	db   *sql.DB
	path string
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
    updated_at          TIMESTAMP NOT NULL,
    case_id             TEXT NOT NULL DEFAULT 'CASE-DEFAULT'
);
CREATE TABLE IF NOT EXISTS conflicts (
    conflict_id       TEXT PRIMARY KEY,
    fact_ids_json     TEXT NOT NULL,
    conflict_point    TEXT NOT NULL,
    resolution_method TEXT,
    is_resolved       INTEGER NOT NULL DEFAULT 0,
    case_id           TEXT NOT NULL DEFAULT 'CASE-DEFAULT'
);
CREATE TABLE IF NOT EXISTS assumptions (
    assumption_id         TEXT PRIMARY KEY,
    content               TEXT NOT NULL,
    proposer              TEXT NOT NULL,
    based_on              TEXT NOT NULL,
    dependent_json        TEXT,
    affected_json         TEXT,
    is_rejected           INTEGER NOT NULL DEFAULT 0,
    case_id               TEXT NOT NULL DEFAULT 'CASE-DEFAULT'
);
CREATE TABLE IF NOT EXISTS documents (
    doc_id      TEXT PRIMARY KEY,
    file        TEXT NOT NULL,
    page_count  INTEGER NOT NULL,
    pages_json  TEXT NOT NULL,
    created_at  TIMESTAMP NOT NULL,
    case_id     TEXT NOT NULL DEFAULT 'CASE-DEFAULT',
    ocr_engine  TEXT NOT NULL DEFAULT '',
    annotations_json TEXT NOT NULL DEFAULT '[]'
);
CREATE TABLE IF NOT EXISTS elements (
    element_id  INTEGER PRIMARY KEY AUTOINCREMENT,
    type        TEXT NOT NULL,
    value       TEXT NOT NULL,
    source_file TEXT NOT NULL,
    page_num    INTEGER NOT NULL,
    case_id     TEXT NOT NULL DEFAULT 'CASE-DEFAULT'
);
-- v0.3 M5 混合检索：本案卷宗页级向量（纯 Go 暴力余弦，无外部向量库）。
-- 语料所有权（plan-v0.2 D0/V2）：只索引本案文档，法条/类案向量永不入库。
CREATE TABLE IF NOT EXISTS embeddings (
    emb_id     INTEGER PRIMARY KEY AUTOINCREMENT,
    doc_id     TEXT NOT NULL,
    page_num   INTEGER NOT NULL,
    model      TEXT NOT NULL,
    dim        INTEGER NOT NULL,
    vec        BLOB NOT NULL,
    text_hash  TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    case_id    TEXT NOT NULL DEFAULT 'CASE-DEFAULT',
    UNIQUE (case_id, doc_id, page_num, model)
);
CREATE INDEX IF NOT EXISTS idx_embeddings_case ON embeddings(case_id, model);
-- v0.3 M6 对抗推演：轮次留痕（round_id 与审计日志关联；被拦截轮次也入库，
-- 拦截必须可追溯，但 blocked=1 的轮次不计入阶段完成度）。
CREATE TABLE IF NOT EXISTS sim_rounds (
    round_id      TEXT PRIMARY KEY,
    phase         TEXT NOT NULL,
    role          TEXT NOT NULL,
    text          TEXT NOT NULL,
    fact_ids_json TEXT NOT NULL DEFAULT '[]',
    blocked       INTEGER NOT NULL DEFAULT 0,
    created_at    TIMESTAMP NOT NULL,
    case_id       TEXT NOT NULL DEFAULT 'CASE-DEFAULT'
);
CREATE INDEX IF NOT EXISTS idx_sim_rounds_case ON sim_rounds(case_id, phase);
`

// configure 连接与并发相关 PRAGMA。桌面端多 goroutine 读写同一库（Wails 每次
// 前端调用一个 goroutine），缺这几项会随机撞 SQLITE_BUSY：
//   - journal_mode=WAL：读写不互斥（内存库自动回落 memory，不报错）；
//   - busy_timeout：写-写冲突排队而不是立刻报错；
//   - foreign_keys：SQLite 默认关，显式打开（后续引用表约束生效）；
//   - synchronous=NORMAL：WAL 下兼顾断电安全与吞吐。
func configure(db *sql.DB) error {
	db.SetMaxOpenConns(4)
	for _, p := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA busy_timeout=5000`,
		`PRAGMA foreign_keys=ON`,
		`PRAGMA synchronous=NORMAL`,
	} {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("数据库参数设置失败（%s）: %w", p, err)
		}
	}
	return nil
}

// Open 打开（或创建）SQLite 数据库并完成全部迁移：
//  1. 业务表结构（新库建表即含 case_id）；
//  2. 存量库案件隔离幂等迁移（迁移前自动备份 <db>.bak-<ts>，plan-v0.2 §9）；
//  3. cases / law_refs / case_refs 表；
//  4. 确保默认案件存在（存量数据归 CASE-DEFAULT，不丢历史）。
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := configure(db); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}
	if err := migrateCaseID(db, path); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrateOCREngine(db); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(casesSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("案件表迁移失败: %w", err)
	}
	if err := migrateLawRefLawID(db, path); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(
		`INSERT OR IGNORE INTO cases (case_id, title, stage, visibility, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		domain.DefaultCaseID, "默认案件", string(domain.StageConsult), string(domain.VisBoth), time.Now().UTC(),
	); err != nil {
		db.Close()
		return nil, fmt.Errorf("默认案件确保失败: %w", err)
	}
	return &Store{db: db, path: path}, nil
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// --- 以下为案件限定仓储视图（CaseScope）上的业务方法（plan-v0.2 §3.3）：
// 旧的全局方法已从 Store 移除——编译期强制，杜绝跨案件泄漏。

// CreateFact 插入事实（挂当前案件）。状态必须合法。
func (sc *CaseScope) CreateFact(f domain.Fact) error {
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
	_, err := sc.db().Exec(
		`INSERT INTO facts (fact_id, description, state, citation_json, related_element,
			conflict_id, assumption_source, visibility, is_manual_confirmed, updated_at, case_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.FactID, f.Description, string(f.State), cj, f.RelatedElement,
		f.ConflictID, f.AssumptionSource, string(f.Visibility), f.IsManualConfirmed, f.UpdatedAt, sc.caseID,
	)
	return err
}

// UpdateFactState 更新事实状态（调用方必须已通过 statemachine 仲裁）。
// WHERE 带 case_id：跨案件更新在 SQL 层不可达。
func (sc *CaseScope) UpdateFactState(factID string, state domain.FactState) error {
	res, err := sc.db().Exec(
		`UPDATE facts SET state = ?, updated_at = ? WHERE fact_id = ? AND case_id = ?`,
		string(state), time.Now().UTC(), factID, sc.caseID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("事实不存在: %s", factID)
	}
	return nil
}

// GetFact 按 ID 读取事实（限本案；查不到即 not-found——跨案件引用天然失败，
// gate.CheckGrounding 因此自动拦截跨案件引用）。
func (sc *CaseScope) GetFact(factID string) (domain.Fact, error) {
	row := sc.db().QueryRow(
		`SELECT fact_id, description, state, citation_json, related_element,
			conflict_id, assumption_source, visibility, is_manual_confirmed, updated_at
		 FROM facts WHERE fact_id = ? AND case_id = ?`, factID, sc.caseID)
	return scanFact(row)
}

// ListFacts 列出本案全部事实（按更新时间倒序）。
func (sc *CaseScope) ListFacts() ([]domain.Fact, error) {
	rows, err := sc.db().Query(
		`SELECT fact_id, description, state, citation_json, related_element,
			conflict_id, assumption_source, visibility, is_manual_confirmed, updated_at
		 FROM facts WHERE case_id = ? ORDER BY updated_at DESC`, sc.caseID)
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

// CreateConflict 插入冲突记录（挂当前案件；冲突不自动消解，is_resolved 默认 false）。
func (sc *CaseScope) CreateConflict(c domain.Conflict) error {
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
	_, err = sc.db().Exec(
		`INSERT INTO conflicts (conflict_id, fact_ids_json, conflict_point, resolution_method, is_resolved, case_id)
		 VALUES (?, ?, ?, ?, 0, ?)`,
		c.ConflictID, string(fj), c.ConflictPoint, c.ResolutionMethod, sc.caseID,
	)
	return err
}

// ListConflicts 列出本案全部冲突。
func (sc *CaseScope) ListConflicts() ([]domain.Conflict, error) {
	rows, err := sc.db().Query(
		`SELECT conflict_id, fact_ids_json, conflict_point, resolution_method, is_resolved
		 FROM conflicts WHERE case_id = ?`, sc.caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Conflict{}
	for rows.Next() {
		var (
			id, fj, point string
			method        sql.NullString
			resolved      int
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
			ResolutionMethod: method.String, IsResolved: resolved != 0,
		})
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanFact(row scanner) (domain.Fact, error) {
	var (
		f                  domain.Fact
		state, vis         string
		cj, rel, cid, asum sql.NullString
		manual             int
		updatedAt          time.Time
	)
	if err := row.Scan(&f.FactID, &f.Description, &state, &cj, &rel, &cid, &asum, &vis, &manual, &updatedAt); err != nil {
		return domain.Fact{}, err
	}
	f.State = domain.FactState(state)
	f.Visibility = domain.Visibility(vis)
	f.RelatedElement, f.ConflictID, f.AssumptionSource = rel.String, cid.String, asum.String
	f.IsManualConfirmed = manual != 0
	f.UpdatedAt = updatedAt
	if cj.String != "" {
		var c domain.Citation
		if err := json.Unmarshal([]byte(cj.String), &c); err != nil {
			return domain.Fact{}, err
		}
		f.Citation = &c
	}
	return f, nil
}

// DocumentInfo 文档接入概况（不含全文，供列表展示）。
// OCREngine 非空 = 文本来自 OCR（UI 必须显示 OCR 徽章；五态封顶弱态由
// 状态机按文档 OCR 标记强制）。
type DocumentInfo struct {
	DocID     string    `json:"doc_id"`
	File      string    `json:"file"`
	PageCount int       `json:"page_count"`
	CreatedAt time.Time `json:"created_at"`
	OCREngine string    `json:"ocr_engine,omitempty"`
}

// SaveDocument 保存已解析文档（页码 → 文本，挂当前案件），供核验器与阅卷 Agent 使用。
func (sc *CaseScope) SaveDocument(docID, file string, pages map[int]string) error {
	return sc.saveDocument(docID, file, pages, "", nil)
}

// SaveDocumentOCR 保存 OCR 产出文档：ocr_engine 落列，GetDocument 读回时
// 带 OCR 标记——状态机据此拒绝进入强态（D1）。
func (sc *CaseScope) SaveDocumentOCR(docID, file, ocrEngine string, pages map[int]string) error {
	if ocrEngine == "" {
		return fmt.Errorf("OCR 文档保存必须声明引擎名（留空 = 伪装非 OCR）")
	}
	return sc.saveDocument(docID, file, pages, ocrEngine, nil)
}

// SaveDocumentFull 保存解析产出的全部语义（OCR 标记 + 标注区块，D2）：
// docx 等「正文 + 修订/批注线索」文档走此入口，读回不丢标注。
func (sc *CaseScope) SaveDocumentFull(docID string, doc *verify.DocumentText) error {
	engine := ""
	if doc.OCR {
		engine = doc.OCREngine
		if engine == "" {
			engine = "ocr" // 不允许 OCR 文档以空引擎名入库
		}
	}
	return sc.saveDocument(docID, doc.File, doc.Pages, engine, doc.Annotations)
}

func (sc *CaseScope) saveDocument(docID, file string, pages map[int]string, ocrEngine string, annotations []verify.Annotation) error {
	pj, err := json.Marshal(pages)
	if err != nil {
		return err
	}
	aj := []byte("[]")
	if len(annotations) > 0 {
		if aj, err = json.Marshal(annotations); err != nil {
			return err
		}
	}
	_, err = sc.db().Exec(
		`INSERT INTO documents (doc_id, file, page_count, pages_json, created_at, case_id, ocr_engine, annotations_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		docID, file, len(pages), string(pj), time.Now().UTC(), sc.caseID, ocrEngine, string(aj),
	)
	return err
}

// GetDocument 读取本案文档全文（返回 core/verify 的可检索文本层；OCR 文档带标记，
// 标注区块读回——标注不进 Pages，回原文定位对其一律失败）。
func (sc *CaseScope) GetDocument(docID string) (*verify.DocumentText, error) {
	var file, pj, ocrEngine, aj string
	row := sc.db().QueryRow(
		`SELECT file, pages_json, ocr_engine, annotations_json FROM documents WHERE doc_id = ? AND case_id = ?`, docID, sc.caseID)
	if err := row.Scan(&file, &pj, &ocrEngine, &aj); err != nil {
		return nil, fmt.Errorf("文档不存在: %s", docID)
	}
	pages := map[int]string{}
	if err := json.Unmarshal([]byte(pj), &pages); err != nil {
		return nil, err
	}
	var annotations []verify.Annotation
	if aj != "" && aj != "[]" {
		if err := json.Unmarshal([]byte(aj), &annotations); err != nil {
			return nil, fmt.Errorf("文档标注解析失败: %w", err)
		}
	}
	return &verify.DocumentText{
		File: file, Pages: pages,
		OCR: ocrEngine != "", OCREngine: ocrEngine,
		Annotations: annotations,
	}, nil
}

// GetDocumentInfo 读取本案单个文档的概况。
func (sc *CaseScope) GetDocumentInfo(docID string) (DocumentInfo, error) {
	var d DocumentInfo
	row := sc.db().QueryRow(
		`SELECT doc_id, file, page_count, created_at, ocr_engine FROM documents WHERE doc_id = ? AND case_id = ?`,
		docID, sc.caseID)
	err := row.Scan(&d.DocID, &d.File, &d.PageCount, &d.CreatedAt, &d.OCREngine)
	if err != nil {
		return DocumentInfo{}, fmt.Errorf("文档不存在: %s", docID)
	}
	return d, nil
}

// IsOCRSource 报告本案内该来源文件是否存在任何 OCR 产出文档（D1 接线）：
// 状态机升级仲裁据此置 SourceFromOCR。同名文件只要有一份经 OCR 入库即按
// OCR 来源处理——fail-closed，宁可封顶弱态，不可漏判强态。
func (sc *CaseScope) IsOCRSource(sourceFile string) (bool, error) {
	var n int
	err := sc.db().QueryRow(
		`SELECT COUNT(*) FROM documents WHERE case_id = ? AND file = ? AND ocr_engine <> ''`,
		sc.caseID, sourceFile).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("来源 OCR 标记查询失败: %w", err)
	}
	return n > 0, nil
}

// ListDocuments 列出本案全部已接入文档。
func (sc *CaseScope) ListDocuments() ([]DocumentInfo, error) {
	rows, err := sc.db().Query(
		`SELECT doc_id, file, page_count, created_at, ocr_engine FROM documents
		 WHERE case_id = ? ORDER BY created_at DESC`, sc.caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DocumentInfo{}
	for rows.Next() {
		var d DocumentInfo
		if err := rows.Scan(&d.DocID, &d.File, &d.PageCount, &d.CreatedAt, &d.OCREngine); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SaveElements 批量保存本案已定位要素。未定位要素（Located=false）严禁入库。
func (sc *CaseScope) SaveElements(elements []extract.Element) error {
	tx, err := sc.db().Begin()
	if err != nil {
		return err
	}
	for _, e := range elements {
		if !e.Located {
			if err := tx.Rollback(); err != nil {
				return err
			}
			return fmt.Errorf("未定位要素禁止入库: %s %q", e.Type, e.Value)
		}
		if _, err := tx.Exec(
			`INSERT INTO elements (type, value, source_file, page_num, case_id) VALUES (?, ?, ?, ?, ?)`,
			string(e.Type), e.Value, e.SourceFile, e.PageNum, sc.caseID,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// ListElements 列出本案全部已入库要素。
func (sc *CaseScope) ListElements() ([]extract.Element, error) {
	rows, err := sc.db().Query(
		`SELECT type, value, source_file, page_num FROM elements
		 WHERE case_id = ? ORDER BY element_id`, sc.caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []extract.Element{}
	for rows.Next() {
		var (
			typ, value, file string
			page             int
		)
		if err := rows.Scan(&typ, &value, &file, &page); err != nil {
			return nil, err
		}
		out = append(out, extract.Element{
			Type: extract.ElementType(typ), Value: value,
			SourceFile: file, PageNum: page, Located: true,
		})
	}
	return out, rows.Err()
}

// ---------- v0.3 M5 本案卷宗向量（embeddings 表） ----------

// EmbeddingInfo 本案某页的向量行（已解码）。vec 仅在本机进程内使用，不出域。
type EmbeddingInfo struct {
	DocID    string
	Page     int
	Model    string
	Dim      int
	TextHash string
	Vec      []float64
}

// encodeVec 向量 → 小端 float64 BLOB（纯 Go，无序列化依赖）。
func encodeVec(vec []float64) []byte {
	buf := make([]byte, 8*len(vec))
	for i, f := range vec {
		binary.LittleEndian.PutUint64(buf[8*i:], math.Float64bits(f))
	}
	return buf
}

// decodeVec BLOB → 向量。dim 与 BLOB 长度不符即报错——宁缺毋滥，不猜半截向量。
func decodeVec(b []byte, dim int) ([]float64, error) {
	if len(b) != 8*dim || dim <= 0 {
		return nil, fmt.Errorf("向量存储损坏：dim=%d 与 BLOB %d 字节不符", dim, len(b))
	}
	vec := make([]float64, dim)
	for i := range vec {
		vec[i] = math.Float64frombits(binary.LittleEndian.Uint64(b[8*i:]))
	}
	return vec, nil
}

// UpsertEmbedding 写入/更新本案某页向量（同 case/doc/page/model 幂等覆盖）。
// textHash 为页文本 sha256，供「未变即跳过重嵌」判定。
func (sc *CaseScope) UpsertEmbedding(docID string, page int, model, textHash string, vec []float64) error {
	if model == "" || len(vec) == 0 {
		return fmt.Errorf("向量入库缺少 model 或维度为空")
	}
	_, err := sc.db().Exec(
		`INSERT INTO embeddings (doc_id, page_num, model, dim, vec, text_hash, updated_at, case_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (case_id, doc_id, page_num, model)
		 DO UPDATE SET dim = excluded.dim, vec = excluded.vec,
		                   text_hash = excluded.text_hash, updated_at = excluded.updated_at`,
		docID, page, model, len(vec), encodeVec(vec), textHash, time.Now().UTC(), sc.caseID,
	)
	return err
}

// ListEmbeddings 列出本案指定 embedding 模型的全部页向量。
// 模型隔离是正确性要求：不同模型的向量空间不可比，绝不混用。
func (sc *CaseScope) ListEmbeddings(model string) ([]EmbeddingInfo, error) {
	rows, err := sc.db().Query(
		`SELECT doc_id, page_num, model, dim, vec, text_hash FROM embeddings
		 WHERE case_id = ? AND model = ? ORDER BY doc_id, page_num`, sc.caseID, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EmbeddingInfo{}
	for rows.Next() {
		var (
			info EmbeddingInfo
			blob []byte
		)
		if err := rows.Scan(&info.DocID, &info.Page, &info.Model, &info.Dim, &blob, &info.TextHash); err != nil {
			return nil, err
		}
		if info.Vec, err = decodeVec(blob, info.Dim); err != nil {
			return nil, fmt.Errorf("%s#%d: %w", info.DocID, info.Page, err)
		}
		out = append(out, info)
	}
	return out, rows.Err()
}

// DeleteDocumentEmbeddings 删除本案某文档的全部向量（文档删除/重接入时调用）。
func (sc *CaseScope) DeleteDocumentEmbeddings(docID string) error {
	_, err := sc.db().Exec(`DELETE FROM embeddings WHERE case_id = ? AND doc_id = ?`, sc.caseID, docID)
	return err
}

// SimRound 对抗推演一轮留痕（v0.3 M6）。被门禁拦截的轮次同样入库
// （拦截必须可追溯），但 Blocked 轮不计入阶段完成度。
type SimRound struct {
	RoundID   string    `json:"round_id"`
	Phase     string    `json:"phase"`
	Role      string    `json:"role"`
	Text      string    `json:"text"`
	FactIDs   []string  `json:"fact_ids,omitempty"`
	Blocked   bool      `json:"blocked"`
	CreatedAt time.Time `json:"created_at"`
}

// AddSimRound 写入一轮推演（round_id 冲突即报错——轮次号由绑定层生成，不得覆盖）。
func (sc *CaseScope) AddSimRound(r SimRound) error {
	if r.RoundID == "" || r.Phase == "" {
		return fmt.Errorf("推演轮次缺少 round_id 或 phase")
	}
	ids, err := json.Marshal(r.FactIDs)
	if err != nil {
		return err
	}
	blocked := 0
	if r.Blocked {
		blocked = 1
	}
	ts := r.CreatedAt
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	_, err = sc.db().Exec(
		`INSERT INTO sim_rounds (round_id, phase, role, text, fact_ids_json, blocked, created_at, case_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.RoundID, r.Phase, r.Role, r.Text, string(ids), blocked, ts, sc.caseID)
	return err
}

// ListSimRounds 本案全部推演轮次（时间正序——轨迹必须按发生顺序重放）。
func (sc *CaseScope) ListSimRounds() ([]SimRound, error) {
	rows, err := sc.db().Query(
		`SELECT round_id, phase, role, text, fact_ids_json, blocked, created_at
		 FROM sim_rounds WHERE case_id = ? ORDER BY created_at, round_id`, sc.caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SimRound{}
	for rows.Next() {
		var (
			r   SimRound
			ids string
			bl  int
		)
		if err := rows.Scan(&r.RoundID, &r.Phase, &r.Role, &r.Text, &ids, &bl, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Blocked = bl != 0
		if ids != "" && ids != "[]" {
			if err := json.Unmarshal([]byte(ids), &r.FactIDs); err != nil {
				return nil, fmt.Errorf("轮次 %s fact_ids 损坏: %w", r.RoundID, err)
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
