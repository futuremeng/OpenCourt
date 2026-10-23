package ingest

import (
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"

	"github.com/futuremeng/OpenCourt/core/verify"
)

// PDFParser 文本型 PDF 解析器：逐页抽取文本层，保留页码坐标。
// 扫描型 PDF（无文本层）被明确拒绝并提示需经 OCR——绝不静默产出空文本，
// 也绝不猜测内容（开发文档 4.1：扫描件强制经过 OCR）。
type PDFParser struct{}

// CanParse 实现 Parser 接口。
func (p *PDFParser) CanParse(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".pdf")
}

// Parse 实现 Parser 接口。
func (p *PDFParser) Parse(path string) (*verify.DocumentText, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("无法打开 PDF（可能已损坏或加密）: %w", err)
	}
	defer f.Close()

	total := r.NumPage()
	if total == 0 {
		return nil, fmt.Errorf("PDF 不含任何页面: %s", path)
	}
	pages := map[int]string{}
	nonEmpty := 0
	for i := 1; i <= total; i++ {
		text, err := r.Page(i).GetPlainText(nil)
		if err != nil {
			return nil, fmt.Errorf("PDF 第 %d 页文本抽取失败: %w", i, err)
		}
		pages[i] = text
		if strings.TrimSpace(text) != "" {
			nonEmpty++
		}
	}
	if nonEmpty == 0 {
		return nil, fmt.Errorf("PDF 无可抽取文本层（扫描文档需经 OCR，当前未配置 OCR 引擎，拒绝处理）: %s", path)
	}
	return &verify.DocumentText{File: path, Pages: pages}, nil
}
