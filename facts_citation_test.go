// 手工录入引用的回原文定位纪律：与阅卷候选同一 fail-closed 标准——
// 定位失败/来源未接入/引用不完整一律拒绝入库并留审计；无来源原路径不变。
package main

import (
	"strings"
	"testing"

	"github.com/futuremeng/OpenCourt/core/domain"
)

func TestCreateFactCitationLocated(t *testing.T) {
	a := newCaseApp(t, t.TempDir())
	if _, err := a.CreateCase("引用定位案", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := a.scope().SaveDocument("DOC-1", "/x/起诉状.txt",
		map[int]string{1: "合同于2025年3月12日成立。"}); err != nil {
		t.Fatal(err)
	}
	// 可定位引用 → 单源未补强入库。
	f, err := a.CreateFact("合同成立", "/x/起诉状.txt", 1, "合同于2025年3月12日成立", false)
	if err != nil || f.State != domain.StateSingleSource {
		t.Fatalf("可定位引用应入库为单源未补强: %v state=%v", err, f.State)
	}
	// 片段不在原文 → 拒绝。
	if _, err := a.CreateFact("捏造引用", "/x/起诉状.txt", 1, "合同于2026年元旦解除", false); err == nil ||
		!strings.Contains(err.Error(), "定位") {
		t.Fatalf("不可定位引用必须拒绝: %v", err)
	}
	// 来源不在本案已接入文档 → 拒绝。
	if _, err := a.CreateFact("外部引用", "/x/未接入.txt", 1, "合同", false); err == nil ||
		!strings.Contains(err.Error(), "接入") {
		t.Fatalf("来源未接入必须拒绝: %v", err)
	}
	// 引用不完整（有来源缺片段）→ 拒绝。
	if _, err := a.CreateFact("缺片段", "/x/起诉状.txt", 1, "", false); err == nil ||
		!strings.Contains(err.Error(), "不完整") {
		t.Fatalf("引用不完整必须拒绝: %v", err)
	}
	// 无来源 → 无依据（原路径不变）。
	f2, err := a.CreateFact("传闻事实", "", 0, "", false)
	if err != nil || f2.State != domain.StateUnsupported {
		t.Fatalf("无来源应入无依据: %v state=%v", err, f2.State)
	}
	// 对方来源（无引用）→ 对方主张未验证。
	f3, err := a.CreateFact("对方称已退款", "", 0, "", true)
	if err != nil || f3.State != domain.StateOpponentClaim {
		t.Fatalf("对方来源应入对方主张: %v state=%v", err, f3.State)
	}
}
