package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/futuremeng/OpenLaw/store"
)

// openlawTestDir 最小 OpenLaw 数据目录（db/ + pdf/，哈希一致）。
func openlawTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{"db", "pdf"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	content := []byte("x")
	if err := os.WriteFile(filepath.Join(dir, "pdf", "d1.pdf"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	db, err := store.Open(filepath.Join(dir, "db", "openlaw.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO law(law_id, name) VALUES('l1', '测试法')`)
	must(`INSERT INTO document(doc_id, file_name, file_path, file_size, sha256, hash_algo, hash_computed_at, confirm_status)
		VALUES('d1','d1.pdf','pdf/d1.pdf',1,?,'SHA-256','2026-01-01T00:00:00Z','confirmed')`, hex.EncodeToString(sum[:]))
	must(`INSERT INTO law_version(version_id, law_id, version_no, effective_date, doc_id)
		VALUES('l1_v1','l1','v1','2020-01-01','d1')`)
	must(`INSERT INTO law_version(version_id, law_id, version_no, effective_date, expire_date, doc_id)
		VALUES('l1_v0','l1','v0','2010-01-01','2020-01-01','d1')`)
	must(`INSERT INTO article(article_id, version_id, article_no, content, md_path, doc_id, parser_version)
		VALUES('l1_v1_第一条','l1_v1','第一条','公司股东滥用公司法人独立地位和股东有限责任，逃避债务，严重损害公司债权人利益的，应当对公司债务承担连带责任。','md/d1.md','d1','test')`)
	must(`INSERT INTO article(article_id, version_id, article_no, content, md_path, doc_id, parser_version)
		VALUES('l1_v0_第一条','l1_v0','第一条','旧版条文：公司股东应当依法承担连带责任。','md/d0.md','d1','test')`)
	return dir
}

func TestOpenLawSourceSearchLaw(t *testing.T) {
	src, err := NewOpenLawSource(openlawTestDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if src.Name() != "openlaw" {
		t.Fatalf("Name = %s", src.Name())
	}
	hits, err := src.SearchLaw(context.Background(), "连带责任", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %d, 期望 2（新旧版本各 1）", len(hits))
	}
	for _, h := range hits {
		if h.Title != "测试法" || h.LawID == "" || h.EffectiveDate == "" {
			t.Errorf("LawHit 字段缺失: %+v", h)
		}
		switch h.Status {
		case "EFFECTIVE", "AMENDED":
		default:
			t.Errorf("状态映射异常（绝不出现 REPEALED）: %+v", h)
		}
	}
}

func TestOpenLawSourceGetLawDetail(t *testing.T) {
	src, err := NewOpenLawSource(openlawTestDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	hit, err := src.GetLawDetail(context.Background(), "l1_v1_第一条")
	if err != nil {
		t.Fatal(err)
	}
	// 最小数据目录无 manifest → 断链事实覆盖状态（溯源不可信优先于版本区间）
	if hit.Article == "" || hit.Status != "UNVERIFIED" {
		t.Errorf("详情异常: %+v", hit)
	}
	// 旧版本 → AMENDED（版本区间映射，绝不判废止）
	old, err := src.GetLawDetail(context.Background(), "l1_v0_第一条")
	if err != nil {
		t.Fatal(err)
	}
	if old.Status != "UNVERIFIED" {
		t.Errorf("断链数据目录应 UNVERIFIED: %+v", old)
	}
	if _, err := src.GetLawDetail(context.Background(), "不存在"); err == nil {
		t.Error("缺失条文应报错")
	}
}

func TestOpenLawSourceUnconfigured(t *testing.T) {
	src, err := NewOpenLawSource("")
	if src != nil || err != nil {
		t.Errorf("未配置应 (nil, nil)：调用方降级: %v %v", src, err)
	}
}
