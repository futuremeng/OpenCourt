package storage

import (
	"path/filepath"
	"testing"

	"github.com/futuremeng/OpenCourt/core/domain"
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

func TestFactCRUD(t *testing.T) {
	s := openTestStore(t)
	f := domain.Fact{
		FactID: "F1", Description: "合同签订于 2024-03-01",
		State: domain.StateSingleSource, Visibility: domain.VisBoth,
		Citation: &domain.Citation{SourceFile: "contract.txt", PageNum: 1, Snippet: "2024-03-01"},
	}
	if err := s.CreateFact(f); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetFact("F1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Citation == nil || got.Citation.PageNum != 1 {
		t.Fatalf("引用序列化错误: %+v", got.Citation)
	}
	if err := s.UpdateFactState("F1", domain.StateTracedConfirmed); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetFact("F1")
	if got.State != domain.StateTracedConfirmed {
		t.Fatalf("状态更新失败: %s", got.State)
	}
	if err := s.CreateFact(domain.Fact{FactID: "F2", State: "BOGUS"}); err == nil {
		t.Error("非法状态应被拒绝")
	}
}

func TestConflictCRUD(t *testing.T) {
	s := openTestStore(t)
	c := domain.Conflict{ConflictID: "C1", FactIDs: []string{"F1", "F2"}, ConflictPoint: "时间不一致"}
	if err := s.CreateConflict(c); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateConflict(domain.Conflict{ConflictID: "C2", FactIDs: []string{"F1"}, ConflictPoint: "x"}); err == nil {
		t.Error("单事实冲突应被拒绝")
	}
	list, err := s.ListConflicts()
	if err != nil || len(list) != 1 || list[0].IsResolved {
		t.Fatalf("冲突读取错误: %v %v", list, err)
	}
}
