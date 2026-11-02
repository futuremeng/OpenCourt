// migrate_caseid.go v0.2 M1 案件隔离幂等迁移（docs/plan-v0.2.md D1 与 §9）：
// 存量五表（facts/conflicts/assumptions/documents/elements）补 case_id 列，
// 存量数据一律归入 CASE-DEFAULT（不丢历史）；迁移前先备份 db 文件
// （opencourt.db.bak-<ts>）；已迁移的库重复打开为 no-op。
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/futuremeng/OpenCourt/core/domain"
)

// legacyTables 需要补 case_id 列的存量表。
var legacyTables = []string{"facts", "conflicts", "assumptions", "documents", "elements"}

// migrateCaseID 检查各表是否已有 case_id 列，缺失则先备份再 ALTER 补齐。
// 必须在业务表结构建立之后、cases 表建立之前执行。
func migrateCaseID(db *sql.DB, path string) error {
	missing := []string{}
	for _, t := range legacyTables {
		has, err := columnExists(db, t, "case_id")
		if err != nil {
			return err
		}
		if !has {
			missing = append(missing, t)
		}
	}
	if len(missing) == 0 {
		return nil // 新库（建表即含 case_id）或已迁移：no-op
	}
	if err := backupDatabase(db, path); err != nil {
		return err
	}
	for _, t := range missing {
		if _, err := db.Exec(fmt.Sprintf(
			"ALTER TABLE %s ADD COLUMN case_id TEXT NOT NULL DEFAULT '%s'",
			t, domain.DefaultCaseID)); err != nil {
			return fmt.Errorf("案件隔离迁移失败（%s 补 case_id）: %w", t, err)
		}
	}
	return nil
}

// columnExists 用 PRAGMA table_info 探测列是否存在（迁移幂等判定）。
func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, fmt.Errorf("探测表结构失败（%s）: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid     int
			name    string
			typ     string
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// backupKeep 迁移备份保留份数。备份里是未脱敏的案件数据，不得无限累积。
const backupKeep = 3

// backupDatabase 迁移前自动备份（plan-v0.2 §9：迁移前自动备份，失败即中止迁移）。
// 优先 VACUUM INTO——WAL 下的正确快照（已提交事务全部包含，不含半截 -wal）；
// 不支持时先 wal_checkpoint(TRUNCATE) 再整文件复制。内存库无备份对象。
func backupDatabase(db *sql.DB, path string) error {
	if path == "" || path == ":memory:" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	bak := fmt.Sprintf("%s.bak-%s", path, time.Now().UTC().Format("20060102T150405"))
	if _, err := db.Exec(`VACUUM INTO ?`, bak); err == nil {
		return pruneBackups(path)
	}
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("迁移前 WAL 检查点失败: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("迁移前备份数据库失败: %w", err)
	}
	if err := os.WriteFile(bak, data, 0o600); err != nil {
		return fmt.Errorf("迁移前备份数据库失败: %w", err)
	}
	return pruneBackups(path)
}

// pruneBackups 只保留最近 backupKeep 份 <db>.bak-<UTC 时间戳>（文件名定长，
// 字典序即时间序）。清理失败不阻断迁移。
func pruneBackups(path string) error {
	matches, err := filepath.Glob(path + ".bak-*")
	if err != nil || len(matches) <= backupKeep {
		return err
	}
	sort.Strings(matches)
	for _, m := range matches[:len(matches)-backupKeep] {
		if err := os.Remove(m); err != nil && !os.IsNotExist(err) {
			return nil
		}
	}
	return nil
}
