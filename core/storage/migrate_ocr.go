package storage

import (
	"database/sql"
	"fmt"
)

// migrateOCREngine 存量库补 documents.ocr_engine / annotations_json 列（幂等，O1/O2）。
// 列缺失 = 存量文档全部按「非 OCR、无标注」处理——这些标记是新增语义，
// 不允许对存量文档做「看起来像 OCR/有标注」的猜测回填。
var ocrMigrations = []struct {
	column string
	ddl    string
}{
	{"ocr_engine", `ALTER TABLE documents ADD COLUMN ocr_engine TEXT NOT NULL DEFAULT ''`},
	{"annotations_json", `ALTER TABLE documents ADD COLUMN annotations_json TEXT NOT NULL DEFAULT '[]'`},
}

func migrateOCREngine(db *sql.DB) error {
	for _, m := range ocrMigrations {
		has, err := columnExists(db, "documents", m.column)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := db.Exec(m.ddl); err != nil {
			return fmt.Errorf("文档标记迁移失败（documents 补 %s）: %w", m.column, err)
		}
	}
	return nil
}
