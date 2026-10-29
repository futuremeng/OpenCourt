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
		// V1 修法（plan-v0.2 D0）：效力状态不自行映射——OpenLaw 未给出
		// effect_status（OL-1 未就绪）前一律 UNKNOWN，绝不猜 EFFECTIVE/AMENDED。
		if h.Status != "UNKNOWN" {
			t.Errorf("效力状态必须透传（OL-1 未就绪时应 UNKNOWN）: %+v", h)
		}
		if h.ConfirmStatus != "confirmed" && h.ConfirmStatus != "pending" {
			t.Errorf("确认状态异常: %+v", h)
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

// --- M2 引用核验透传（plan-v0.2 4.1）：判定 = OpenLaw 错误码原样 ---

func TestOpenLawVerdictCheckCitation(t *testing.T) {
	src, err := NewOpenLawSource(openlawTestDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	ctx := context.Background()

	// 法规名不存在 → OL-1001
	v, err := src.CheckCitation(ctx, "不存在的法", "第一条", "", "2024-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if v.VerifyResult != "OL-1001" {
		t.Errorf("OL-1001 分支异常: %+v", v)
	}
	// 条款不存在 → OL-1003
	v, _ = src.CheckCitation(ctx, "测试法", "第九十九条", "", "2024-01-01")
	if v.VerifyResult != "OL-1003" {
		t.Errorf("OL-1003 分支异常: %+v", v)
	}
	// 最小数据目录无 manifest → 断链优先 OL-1006；数据不吞（条标识照常返回）
	v, err = src.CheckCitation(ctx, "测试法", "第一条",
		"公司股东滥用公司法人独立地位和股东有限责任，逃避债务，严重损害公司债权人利益的，应当对公司债务承担连带责任。", "2024-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if v.VerifyResult != "OL-1006" || v.ChainOK || v.ArticleID != "l1_v1_第一条" {
		t.Errorf("OL-1006 断链应数据照常返回: %+v", v)
	}
	if v.Status != "UNKNOWN" {
		t.Errorf("效力状态应保守 UNKNOWN: %+v", v)
	}
}

func TestOpenLawVerdictResolveAndRelation(t *testing.T) {
	dir := openlawTestDir(t)
	src, err := NewOpenLawSource(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	ctx := context.Background()

	info, err := src.ResolveVersion(ctx, "测试法", "2015-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if info.VersionNo != "v0" || info.ConfirmStatus != "confirmed" {
		t.Errorf("版本解析异常: %+v", info)
	}

	// 无关系 → 空列表不是错误
	rels, err := src.FindRelation(ctx, "l1")
	if err != nil || len(rels) != 0 {
		t.Fatalf("空关系应为空列表: %v %+v", err, rels)
	}
	// pending 关系原样透传（机器不自动判定废止）
	db, err := store.Open(filepath.Join(dir, "db", "openlaw.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO law(law_id, name) VALUES('l2','关联法')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO relation(src_law_id, dst_law_id, relation_type, confirm_status)
		VALUES('l1','l2','废止','pending')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	rels, err = src.FindRelation(ctx, "l1")
	if err != nil || len(rels) != 1 || rels[0].ConfirmStatus != "pending" {
		t.Fatalf("pending 关系必须原样上浮: %v %+v", err, rels)
	}
}
