package storage

import (
	"path/filepath"
	"testing"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/extract"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// openTestScope 默认案件视图（v0.2 M1：业务读写一律经 CaseScope）。
func openTestScope(t *testing.T) *CaseScope {
	t.Helper()
	return openTestStore(t).WithCase(domain.DefaultCaseID)
}

func TestFactCRUD(t *testing.T) {
	s := openTestStore(t)
	sc := s.WithCase(domain.DefaultCaseID)
	f := domain.Fact{
		FactID: "F1", Description: "合同签订于 2024-03-01",
		State: domain.StateSingleSource, Visibility: domain.VisBoth,
		Citation: &domain.Citation{SourceFile: "contract.txt", PageNum: 1, Snippet: "2024-03-01"},
	}
	if err := sc.CreateFact(f); err != nil {
		t.Fatal(err)
	}
	got, err := sc.GetFact("F1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Citation == nil || got.Citation.PageNum != 1 {
		t.Fatalf("引用序列化错误: %+v", got.Citation)
	}
	if err := sc.UpdateFactState("F1", domain.StateTracedConfirmed); err != nil {
		t.Fatal(err)
	}
	got, _ = sc.GetFact("F1")
	if got.State != domain.StateTracedConfirmed {
		t.Fatalf("状态更新失败: %s", got.State)
	}
	if err := sc.CreateFact(domain.Fact{FactID: "F2", State: "BOGUS"}); err == nil {
		t.Error("非法状态应被拒绝")
	}
}

func TestConflictCRUD(t *testing.T) {
	sc := openTestScope(t)
	c := domain.Conflict{ConflictID: "C1", FactIDs: []string{"F1", "F2"}, ConflictPoint: "时间不一致"}
	if err := sc.CreateConflict(c); err != nil {
		t.Fatal(err)
	}
	if err := sc.CreateConflict(domain.Conflict{ConflictID: "C2", FactIDs: []string{"F1"}, ConflictPoint: "x"}); err == nil {
		t.Error("单事实冲突应被拒绝")
	}
	list, err := sc.ListConflicts()
	if err != nil || len(list) != 1 || list[0].IsResolved {
		t.Fatalf("冲突读取错误: %v %v", list, err)
	}
}

func TestDocumentCRUD(t *testing.T) {
	sc := openTestScope(t)
	pages := map[int]string{1: "第一页原文", 2: "第二页原文 2023-05-10"}
	if err := sc.SaveDocument("DOC-1", "case.pdf", pages); err != nil {
		t.Fatal(err)
	}
	doc, err := sc.GetDocument("DOC-1")
	if err != nil {
		t.Fatal(err)
	}
	if doc.File != "case.pdf" || doc.Pages[2] != "第二页原文 2023-05-10" {
		t.Errorf("文档读回不一致: %+v", doc)
	}
	list, err := sc.ListDocuments()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].PageCount != 2 {
		t.Errorf("文档列表错误: %+v", list)
	}
	if _, err := sc.GetDocument("DOC-X"); err == nil {
		t.Error("不存在的文档必须报错")
	}
}

func TestElementStoreRejectsUnlocated(t *testing.T) {
	sc := openTestScope(t)
	// 未定位要素严禁入库（幻觉门禁前置）。
	err := sc.SaveElements([]extract.Element{
		{Type: extract.ElementTime, Value: "2023-05-10", SourceFile: "a.txt", PageNum: 1, Located: false},
	})
	if err == nil {
		t.Fatal("Located=false 的要素必须被拒绝入库")
	}
	list, _ := sc.ListElements()
	if len(list) != 0 {
		t.Errorf("被拒绝的要素不得残留: %+v", list)
	}
	if err := sc.SaveElements([]extract.Element{
		{Type: extract.ElementAmount, Value: "50000元", SourceFile: "a.txt", PageNum: 1, Located: true},
	}); err != nil {
		t.Fatal(err)
	}
	list, _ = sc.ListElements()
	if len(list) != 1 || !list[0].Located {
		t.Errorf("要素读回错误: %+v", list)
	}
}
