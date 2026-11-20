// D1 生产接线测试：OCR 来源封顶弱态必须经由 App.UpgradeFact（生产唯一
// 升级入口）验证——只测 ArbitrateUpgrade 纯函数会给出货架上的假安全感。
package main

import (
	"strings"
	"testing"

	"github.com/futuremeng/OpenCourt/core/domain"
)

func TestUpgradeFactOCRCapped(t *testing.T) {
	a := newCaseApp(t, t.TempDir())
	if _, err := a.CreateCase("OCR 封顶案", "", ""); err != nil {
		t.Fatal(err)
	}
	// OCR 产出文档（引擎名落列）与原生文档各一份。
	if err := a.scope().SaveDocumentOCR("DOC-OCR", "/x/scan.pdf", "vision",
		map[int]string{1: "扫描识别文本"}); err != nil {
		t.Fatal(err)
	}
	if err := a.scope().SaveDocument("DOC-NATIVE", "/x/native.pdf",
		map[int]string{1: "原生文本层"}); err != nil {
		t.Fatal(err)
	}
	fOCR, err := a.CreateFact("扫描件事实", "/x/scan.pdf", 1, "扫描识别文本", false)
	if err != nil {
		t.Fatal(err)
	}
	fNat, err := a.CreateFact("原生文档事实", "/x/native.pdf", 1, "原生文本层", false)
	if err != nil {
		t.Fatal(err)
	}
	// 三条件全满足也进不了强态——OCR 来源被状态机拒绝。
	if _, err := a.UpgradeFact(fOCR.FactID, string(domain.StateTracedConfirmed), true, true, true); err == nil ||
		!strings.Contains(err.Error(), "OCR") {
		t.Fatalf("OCR 来源事实经生产入口升级必须被拒: %v", err)
	}
	if got, _ := a.scope().GetFact(fOCR.FactID); got.State != domain.StateSingleSource {
		t.Errorf("被拒后状态不得变化: %s", got.State)
	}
	// 非 OCR 来源照常走证据驱动升级——接线不得误伤。
	if _, err := a.UpgradeFact(fNat.FactID, string(domain.StateTracedConfirmed), true, true, true); err != nil {
		t.Fatalf("非 OCR 来源三条件满足应可升级: %v", err)
	}
	if !strings.Contains(readAudit(t, a), "STATE_UPGRADE_BLOCKED") {
		t.Error("OCR 升级被拒必须留审计")
	}
}
