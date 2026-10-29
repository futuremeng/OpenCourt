package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/extract"
)

func TestDefaultCaseEnsured(t *testing.T) {
	s := openTestStore(t)
	list, err := s.ListCases()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].CaseID != domain.DefaultCaseID {
		t.Fatalf("默认案件必须自动确保存在: %+v", list)
	}
}

func TestCreateCaseValidation(t *testing.T) {
	s := openTestStore(t)
	if err := s.CreateCase(domain.Case{CaseID: "C-X", Title: ""}); err == nil {
		t.Error("空标题应被拒绝")
	}
	if err := s.CreateCase(domain.Case{CaseID: "C-X", Title: "x", Stage: "BOGUS"}); err == nil {
		t.Error("非法阶段应被拒绝")
	}
	c := domain.Case{CaseID: "C-A", Title: "民间借贷纠纷", CaseType: "民间借贷", Stage: domain.StageFirstTrial}
	if err := s.CreateCase(c); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetCase("C-A")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "民间借贷纠纷" || got.Stage != domain.StageFirstTrial || got.Visibility != domain.VisBoth {
		t.Errorf("案件读回不一致: %+v", got)
	}
	if _, err := s.GetCase("C-NOPE"); err == nil {
		t.Error("不存在的案件必须报错")
	}
}

// 跨案件隔离（plan-v0.2 §7）：WithCase 视图读不到、改不动他案数据。
func TestWithCaseIsolation(t *testing.T) {
	s := openTestStore(t)
	if err := s.CreateCase(domain.Case{CaseID: "CASE-A", Title: "A 案"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateCase(domain.Case{CaseID: "CASE-B", Title: "B 案"}); err != nil {
		t.Fatal(err)
	}
	a, b := s.WithCase("CASE-A"), s.WithCase("CASE-B")

	fa := domain.Fact{FactID: "FA", Description: "A 案事实", State: domain.StateSingleSource, Visibility: domain.VisBoth}
	fb := domain.Fact{FactID: "FB", Description: "B 案事实", State: domain.StateSingleSource, Visibility: domain.VisBoth}
	if err := a.CreateFact(fa); err != nil {
		t.Fatal(err)
	}
	if err := b.CreateFact(fb); err != nil {
		t.Fatal(err)
	}
	if facts, _ := a.ListFacts(); len(facts) != 1 || facts[0].FactID != "FA" {
		t.Errorf("A 案应只见自己的事实: %+v", facts)
	}
	if _, err := a.GetFact("FB"); err == nil {
		t.Error("跨案件 GetFact 必须 not-found（gate 溯源据此自动拦截）")
	}
	if err := a.UpdateFactState("FB", domain.StateTracedConfirmed); err == nil {
		t.Error("跨案件状态升级必须失败")
	}

	if err := a.SaveDocument("DOC-A", "a.pdf", map[int]string{1: "A 页"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.GetDocument("DOC-A"); err == nil {
		t.Error("跨案件文档读取必须失败")
	}
	if docs, _ := b.ListDocuments(); len(docs) != 0 {
		t.Errorf("B 案不得见 A 案文档: %+v", docs)
	}

	if err := a.SaveElements([]extract.Element{
		{Type: extract.ElementTime, Value: "2023-05-10", SourceFile: "a.pdf", PageNum: 1, Located: true},
	}); err != nil {
		t.Fatal(err)
	}
	if els, _ := b.ListElements(); len(els) != 0 {
		t.Errorf("B 案不得见 A 案要素: %+v", els)
	}

	if err := a.CreateConflict(domain.Conflict{ConflictID: "CA", FactIDs: []string{"FA", "FA2"}, ConflictPoint: "A 案冲突"}); err != nil {
		t.Fatal(err)
	}
	if cs, _ := b.ListConflicts(); len(cs) != 0 {
		t.Errorf("B 案不得见 A 案冲突: %+v", cs)
	}
}

// refs 表按案件隔离 + 未核验缺省保守（UNVERIFIED/pending）。
func TestRefsScoped(t *testing.T) {
	s := openTestStore(t)
	for _, id := range []string{"CASE-A", "CASE-B"} {
		if err := s.CreateCase(domain.Case{CaseID: id, Title: id}); err != nil {
			t.Fatal(err)
		}
	}
	a, b := s.WithCase("CASE-A"), s.WithCase("CASE-B")

	if err := a.AddLawRef(domain.LawRef{ArticleID: "mfd-577", LawName: "中华人民共和国民法典", ArticleNo: "第五百七十七条"}); err != nil {
		t.Fatal(err)
	}
	refs, _ := a.ListLawRefs()
	if len(refs) != 1 {
		t.Fatalf("A 案法条引用读回错误: %+v", refs)
	}
	if refs[0].Status != "UNVERIFIED" || refs[0].ConfirmStatus != "pending" || refs[0].VerifyResult != "" {
		t.Errorf("未核验引用必须保守取 UNVERIFIED/pending: %+v", refs[0])
	}
	if other, _ := b.ListLawRefs(); len(other) != 0 {
		t.Errorf("B 案不得见 A 案法条引用: %+v", other)
	}
	refA := refs[0].RefID
	if err := b.UpdateLawRefVerify(refA, "OL-1003"); err == nil {
		t.Error("跨案件更新核验结论必须失败")
	}
	if err := a.UpdateLawRefVerify(refA, "OL-1003"); err != nil {
		t.Fatal(err)
	}
	if refs, _ = a.ListLawRefs(); refs[0].VerifyResult != "OL-1003" {
		t.Errorf("核验结论应原样可读: %+v", refs[0])
	}

	if err := a.AddCaseRef(domain.CaseRef{SourceCaseID: "lc-1", CaseName: "类案甲", Summary: "已脱敏要旨"}); err != nil {
		t.Fatal(err)
	}
	if refs, _ := b.ListCaseRefs(); len(refs) != 0 {
		t.Errorf("B 案不得见 A 案类案引用: %+v", refs)
	}
	if err := a.AddCaseRef(domain.CaseRef{}); err == nil {
		t.Error("无源案件标识的类案引用应被拒绝")
	}
}

// 存量库迁移（plan-v0.2 §7/§9）：旧五表补 case_id、存量归 CASE-DEFAULT、
// 迁移前自动备份、重复打开幂等且不重复备份。
func TestMigrateLegacyDBIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencourt.db")

	// 造存量库：旧表结构（无 case_id）+ 存量数据
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TABLE facts (
    fact_id TEXT PRIMARY KEY, description TEXT NOT NULL, state TEXT NOT NULL,
    citation_json TEXT, related_element TEXT, conflict_id TEXT, assumption_source TEXT,
    visibility TEXT NOT NULL DEFAULT 'BOTH', is_manual_confirmed INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL);
INSERT INTO facts (fact_id, description, state, visibility, is_manual_confirmed, updated_at)
VALUES ('OLD-1', '存量事实', 'SINGLE_SOURCE', 'BOTH', 0, '2026-01-01T00:00:00Z');`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.WithCase(domain.DefaultCaseID).GetFact("OLD-1")
	if err != nil {
		t.Fatalf("存量数据必须归入 %s: %v", domain.DefaultCaseID, err)
	}
	if f.Description != "存量事实" {
		t.Errorf("存量数据不得丢失: %+v", f)
	}
	s.Close()

	entries, _ := os.ReadDir(dir)
	baks := 0
	for _, e := range entries {
		if strings.Contains(e.Name(), ".bak-") {
			baks++
		}
	}
	if baks != 1 {
		t.Fatalf("迁移前必须自动备份恰好一份: %d", baks)
	}

	// 重复打开：幂等 no-op，不再产生新备份，数据仍在
	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("重复打开必须幂等: %v", err)
	}
	defer s2.Close()
	if _, err := s2.WithCase(domain.DefaultCaseID).GetFact("OLD-1"); err != nil {
		t.Errorf("重复打开后存量数据必须仍在: %v", err)
	}
	entries, _ = os.ReadDir(dir)
	baks2 := 0
	for _, e := range entries {
		if strings.Contains(e.Name(), ".bak-") {
			baks2++
		}
	}
	if baks2 != 1 {
		t.Errorf("已迁移库重复打开不得再备份: %d", baks2)
	}
}
