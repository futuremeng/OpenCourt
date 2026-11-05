package ingest

import (
	"context"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"

	"github.com/futuremeng/OpenCourt/core/verify"
)

// PDFParser 文本型 PDF 解析器：逐页抽取文本层，保留页码坐标。
// 扫描型 PDF（无文本层）：OCR 引擎已配置 → 抽内嵌图逐页 OCR（产出带 OCR
// 标记，封顶弱态）；未配置 → 明确拒绝并提示需经 OCR——绝不静默产出空文本，
// 也绝不猜测内容（开发文档 4.1：扫描件强制经过 OCR）。
type PDFParser struct {
	OCR           OCRAdapter     // nil = 扫描件维持拒绝（安全默认）
	ExtractImages ImageExtractor // nil = 已配引擎但无法取图，同样拒绝
}

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
		return p.parseScanned(path, total)
	}
	return &verify.DocumentText{File: path, Pages: pages}, nil
}

// parseScanned 扫描型 PDF：OCR 管道。任何一页无图像层、OCR 失败或空白
// 即整体拒绝——部分入库会让「回原文定位」出现无法解释的空页，比拒绝更危险。
// 同页多内嵌图按序拼接为一页文本；置信度取该页各图最低值（黄标保守）。
func (p *PDFParser) parseScanned(path string, total int) (*verify.DocumentText, error) {
	if p.OCR == nil {
		return nil, fmt.Errorf("PDF 无可抽取文本层（扫描文档需经 OCR，当前未配置 OCR 引擎，拒绝处理）: %s", path)
	}
	if p.ExtractImages == nil {
		return nil, fmt.Errorf("PDF 为扫描文档：OCR 引擎已配置（%s），但扫描取图组件未接入——拒绝处理（绝不猜测内容）", p.OCR.Name())
	}
	groups, cleanup, err := p.ExtractImages(path)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, fmt.Errorf("扫描 PDF 抽取内嵌图像失败: %w", err)
	}
	byPage := map[int][]string{}
	for _, g := range groups {
		if g.Page > total {
			return nil, fmt.Errorf("扫描 PDF 取图页码 %d 超出文档页数 %d，拒绝入库", g.Page, total)
		}
		byPage[g.Page] = append(byPage[g.Page], g.Paths...)
	}
	text := map[int]string{}
	conf := map[int]float64{}
	for page := 1; page <= total; page++ {
		imgs, ok := byPage[page]
		if !ok {
			return nil, fmt.Errorf("扫描 PDF 第 %d 页无图像层（也无文本层）——拒绝入库（绝不猜测内容）", page)
		}
		var sb strings.Builder
		pageConf := 0.0
		for _, img := range imgs {
			pages, err := p.OCR.Recognize(context.Background(), img)
			if err != nil {
				return nil, fmt.Errorf("扫描 PDF 第 %d 页 OCR 失败: %w", page, err)
			}
			if len(pages) == 0 {
				return nil, fmt.Errorf("扫描 PDF 第 %d 页 OCR 未产出文本，拒绝入库", page)
			}
			for _, pg := range pages {
				sb.WriteString(pg.Text)
				if pg.Confidence > 0 && (pageConf == 0 || pg.Confidence < pageConf) {
					pageConf = pg.Confidence
				}
			}
		}
		text[page] = sb.String()
		if pageConf > 0 {
			conf[page] = pageConf
		}
	}
	return &verify.DocumentText{
		File: path, Pages: text,
		OCR: true, OCREngine: p.OCR.Name(), OCRConf: conf,
	}, nil
}
