// 本文件由 main.go 按领域拆分而来（同包 main：Wails 绑定方法必须
// 留在 main 包，这里只是物理分文件，便于按领域阅读与评审）。
package main

import (
	"fmt"
	"path/filepath"
	"strings"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/futuremeng/OpenCourt/core/storage"
	"github.com/futuremeng/OpenCourt/core/verify"
	"github.com/futuremeng/OpenCourt/neutral"
)

// PickDocumentFile 弹出系统文件选择框（PDF/txt/md；图片与扫描件需 OCR 引擎，
// 未配置引擎时选择后会被明确拒绝——入口不撒谎）。
func (a *App) PickDocumentFile() (string, error) {
	return wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择卷宗文档",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "卷宗文档（PDF/txt/md/docx）", Pattern: "*.pdf;*.txt;*.md;*.docx;*.doc"},
			{DisplayName: "图片/扫描件（需 OCR 引擎）", Pattern: "*.png;*.jpg;*.jpeg;*.bmp;*.tif;*.tiff"},
		},
	})
}

// IngestDocument 解析并入库文档（保留页码坐标，供核验器回原文定位）。
// 扫描型文档：OCR 引擎已配置 → 走 OCR 管道（带引擎与置信度审计，五态封顶
// 弱态）；未配置 → 明确拒绝。
func (a *App) IngestDocument(path string) (storage.DocumentInfo, error) {
	if path == "" {
		return storage.DocumentInfo{}, fmt.Errorf("未选择文件")
	}
	doc, err := a.ingest.Parse(path)
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "INGEST_REJECTED", path, err.Error())
		return storage.DocumentInfo{}, err
	}
	docID := newID("DOC")
	if err := a.scope().SaveDocumentFull(docID, doc); err != nil {
		return storage.DocumentInfo{}, err
	}
	if doc.OCR {
		note := fmt.Sprintf("%s 共 %d 页（OCR 引擎 %s，入库封顶弱态）", filepath.Base(path), len(doc.Pages), doc.OCREngine)
		if low := a.ingest.LowConfidencePages(doc, a.settings.Ingest.OCR.MinConfidence); len(low) > 0 {
			note += fmt.Sprintf("；低置信页: %v（需人工校对）", low)
		}
		_ = a.auditLog.Log(a.profile.Role, "INGEST_OCR", docID, note)
	} else {
		note := fmt.Sprintf("%s 共 %d 页", filepath.Base(path), len(doc.Pages))
		if n := len(doc.Annotations); n > 0 {
			note += fmt.Sprintf("；标注区块 %d 条（修订/批注/图片线索，不进正文）", n)
		}
		_ = a.auditLog.Log(a.profile.Role, "INGEST", docID, note)
	}
	return a.scope().GetDocumentInfo(docID)
}

// ListDocuments 列出已接入文档。
func (a *App) ListDocuments() ([]storage.DocumentInfo, error) {
	return a.scope().ListDocuments()
}

// DocumentAnnotations 返回文档的独立标注区块（修订痕迹/批注/内嵌图线索，D2）。
// 标注是**证据线索**不是正文：不进 Pages，回原文定位对其一律失败。
func (a *App) DocumentAnnotations(docID string) ([]verify.Annotation, error) {
	doc, err := a.scope().GetDocument(docID)
	if err != nil {
		return nil, err
	}
	return doc.Annotations, nil
}

// RunReader 对指定文档执行阅卷：模型产候选事实 → 核验器回原文定位仲裁。
// 定位失败的候选被拒绝入库并记审计；时间/金额要素由正则确定性抽取。
// 【V3】要件名封闭候选集由当前案件案由从 OpenLaw 要件库注入；无要件库
// 传 nil——related_element 一律置空，绝不回退到模型自由生成要件。
func (a *App) RunReader(docID string) (neutral.ReaderReport, error) {
	doc, err := a.scope().GetDocument(docID)
	if err != nil {
		return neutral.ReaderReport{}, err
	}
	report, err := neutral.RunReader(a.ctx, docID, doc, a.model, a.elementNamesForCurrentCase())
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "READER_FAILED", docID, err.Error())
		return neutral.ReaderReport{}, err
	}
	for _, f := range report.Accepted {
		if err := a.scope().CreateFact(f); err != nil {
			return neutral.ReaderReport{}, err
		}
	}
	if err := a.scope().SaveElements(report.Elements); err != nil {
		return neutral.ReaderReport{}, err
	}
	_ = a.auditLog.Log(a.profile.Role, "READER", docID,
		fmt.Sprintf("模型 %s：候选入库 %d 条，回原文失败拒绝 %d 条，门禁拦截 %d 条，要素 %d 个",
			report.ModelName, len(report.Accepted), len(report.Rejected), report.GateBlocked, len(report.Elements)))
	for _, r := range report.Rejected {
		if strings.HasPrefix(r.RejectReason, "GATE:") {
			_ = a.auditLog.Log(a.profile.Role, "GATE_BLOCK", docID,
				fmt.Sprintf("阅卷候选被拦截（%s）：%s", r.RejectReason, r.Description))
		}
	}
	return report, nil
}

// --- 网络资料查阅（BrowserOS 辅助，默认禁用；docs/browseros-integration.md） ---
