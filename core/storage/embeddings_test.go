package storage

import (
	"math"
	"testing"

	"github.com/futuremeng/OpenCourt/core/domain"
)

// 向量往返：BLOB 编解码必须逐位无损（暴力余弦的正确性前提）。
func TestEmbeddingRoundTrip(t *testing.T) {
	sc := openTestStore(t).WithCase(domain.DefaultCaseID)
	vec := []float64{1.5, -2.25, 0, math.Pi}
	if err := sc.UpsertEmbedding("d1", 1, "m1", "hash1", vec); err != nil {
		t.Fatal(err)
	}
	rows, err := sc.ListEmbeddings("m1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].DocID != "d1" || rows[0].Page != 1 {
		t.Fatalf("应恰有一条: %+v", rows)
	}
	if len(rows[0].Vec) != len(vec) {
		t.Fatalf("维度丢失: %v", rows[0].Vec)
	}
	for i := range vec {
		if rows[0].Vec[i] != vec[i] {
			t.Fatalf("分量 %d 失真: %v != %v", i, rows[0].Vec[i], vec[i])
		}
	}
	if rows[0].TextHash != "hash1" {
		t.Errorf("text_hash 应随行保存: %+v", rows[0])
	}
}

// 幂等覆盖：同 (case, doc, page, model) 再写只更新不重复。
func TestEmbeddingUpsertIdempotent(t *testing.T) {
	sc := openTestStore(t).WithCase(domain.DefaultCaseID)
	if err := sc.UpsertEmbedding("d1", 1, "m1", "h1", []float64{1, 0}); err != nil {
		t.Fatal(err)
	}
	if err := sc.UpsertEmbedding("d1", 1, "m1", "h2", []float64{0, 1}); err != nil {
		t.Fatal(err)
	}
	rows, err := sc.ListEmbeddings("m1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].TextHash != "h2" || rows[0].Vec[1] != 1 {
		t.Errorf("应覆盖为一行: %+v", rows)
	}
}

// 模型隔离：不同 embedding 模型的向量空间不可比，读取必须按模型过滤。
func TestEmbeddingModelIsolation(t *testing.T) {
	sc := openTestStore(t).WithCase(domain.DefaultCaseID)
	if err := sc.UpsertEmbedding("d1", 1, "model-a", "h1", []float64{1, 0}); err != nil {
		t.Fatal(err)
	}
	if err := sc.UpsertEmbedding("d1", 1, "model-b", "h1", []float64{0, 1, 2}); err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"model-a", "model-b"} {
		rows, err := sc.ListEmbeddings(m)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].Model != m {
			t.Errorf("模型 %s 应只见自己: %+v", m, rows)
		}
	}
	if rows, _ := sc.ListEmbeddings("model-c"); len(rows) != 0 {
		t.Errorf("未知模型应为空: %+v", rows)
	}
}

// 案件隔离：向量与其余表同样挂 case_id，跨案件互不可见。
func TestEmbeddingCaseIsolation(t *testing.T) {
	s := openTestStore(t)
	if err := s.CreateCase(domain.Case{CaseID: "CASE-A", Title: "A 案"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateCase(domain.Case{CaseID: "CASE-B", Title: "B 案"}); err != nil {
		t.Fatal(err)
	}
	a, b := s.WithCase("CASE-A"), s.WithCase("CASE-B")
	if err := a.UpsertEmbedding("da", 1, "m", "h", []float64{1}); err != nil {
		t.Fatal(err)
	}
	if rows, _ := b.ListEmbeddings("m"); len(rows) != 0 {
		t.Errorf("B 案不得见 A 案向量: %+v", rows)
	}
	if err := b.UpsertEmbedding("da", 1, "m", "h", []float64{2}); err != nil {
		t.Errorf("同坐标不同案件必须可共存: %v", err)
	}
	if rows, _ := a.ListEmbeddings("m"); len(rows) != 1 || rows[0].Vec[0] != 1 {
		t.Errorf("A 案向量不应被 B 案覆盖: %+v", rows)
	}
}

// 文档删除时向量随行清除，不留悬空向量。
func TestEmbeddingDeleteWithDocument(t *testing.T) {
	sc := openTestStore(t).WithCase(domain.DefaultCaseID)
	if err := sc.UpsertEmbedding("d1", 1, "m", "h", []float64{1}); err != nil {
		t.Fatal(err)
	}
	if err := sc.DeleteDocumentEmbeddings("d1"); err != nil {
		t.Fatal(err)
	}
	if rows, _ := sc.ListEmbeddings("m"); len(rows) != 0 {
		t.Errorf("删除后不得残留: %+v", rows)
	}
}

// 空模型名/零维向量必须显式拒绝——宁缺毋滥。
func TestEmbeddingRejectsGarbage(t *testing.T) {
	sc := openTestStore(t).WithCase(domain.DefaultCaseID)
	if err := sc.UpsertEmbedding("d1", 1, "", "h", []float64{1}); err == nil {
		t.Error("空模型名应拒绝")
	}
	if err := sc.UpsertEmbedding("d1", 1, "m", "h", nil); err == nil {
		t.Error("零维向量应拒绝")
	}
}
