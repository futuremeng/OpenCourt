package plugins

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futuremeng/OpenLaw/store"
)

// 【M4】ElementsByCaseType：要件库封闭候选集读取（经 AnalyzeElements 空事实适配）。
func TestElementsByCaseType(t *testing.T) {
	dir := openlawTestDir(t)
	db, err := store.Open(filepath.Join(dir, "db", "openlaw.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO element(element_id, article_id, case_type, name, description, how_to_attack)
		VALUES('e1','l1_v1_第一条','公司纠纷','主体适格','公司为依法设立的公司','核查工商登记')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO element(element_id, article_id, case_type, name, description, how_to_attack)
		VALUES('e2','l1_v1_第一条','公司纠纷','滥用有限责任','存在滥用行为','核查财产混同')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	src, err := NewOpenLawSource(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	specs, err := src.ElementsByCaseType(context.Background(), "公司纠纷")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 {
		t.Fatalf("应取回 2 条要件，实际 %d: %+v", len(specs), specs)
	}
	if specs[0].ElementID != "e1" || specs[0].Name != "主体适格" || specs[0].HowToAttack != "核查工商登记" {
		t.Errorf("要件透传错误: %+v", specs[0])
	}
}

// 【M4】案由无要件 → 明确报错（无要件库），绝不返回空集冒充有库。
func TestElementsByCaseTypeNoElements(t *testing.T) {
	src, err := NewOpenLawSource(openlawTestDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	_, err = src.ElementsByCaseType(context.Background(), "不存在的案由")
	if err == nil {
		t.Fatal("案由无要件必须报错（显式降级，不静默空集）")
	}
	if !strings.Contains(err.Error(), "无要件库") {
		t.Errorf("错误信息应说明无要件库: %v", err)
	}
}
