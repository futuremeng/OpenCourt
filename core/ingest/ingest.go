// Package ingest 文档接入模块（开发文档 4.1）：
// 支持 PDF、扫描件、图片、Word 及压缩包；扫描件与图片强制经过 OCR。
// 本阶段提供解析器接口与纯文本解析器实现；PDF/OCR 解析器由后续阶段接入。
package ingest

import (
	"fmt"
	"os"
	"strings"

	"github.com/futuremeng/OpenCourt/core/verify"
)

// Parser 文档解析器接口。
type Parser interface {
	// CanParse 按扩展名/MIME 判断是否支持该文件。
	CanParse(path string) bool
	// Parse 将文件解析为按页组织的可检索文本层。
	Parse(path string) (*verify.DocumentText, error)
}

// Registry 解析器注册表：按文件路径选择解析器。
type Registry struct {
	parsers []Parser
}

// NewRegistry 返回注册了内置解析器的注册表。
// OCRParser/DocxParser 默认未注入引擎：扫描件/图片会被显式拒绝（安全默认）；
// docx 正文无需引擎，仅内嵌图片需 OCR（未配置只拒绝该图，D2）。
func NewRegistry() *Registry {
	return &Registry{parsers: []Parser{
		&TextParser{},
		&PDFParser{},
		&DocxParser{},
		&OCRParser{},
	}}
}

// Register 追加解析器（如后续的 PDFParser、OCRParser）。
func (r *Registry) Register(p Parser) {
	r.parsers = append(r.parsers, p)
}

// SetOCR 向内置解析器注入 OCR 引擎（图片解析器直接注入；PDF 解析器获得
// 扫描页 OCR 能力）。engine 为 nil = 维持安全默认（扫描件明确拒绝）。
// 低置信阈值不在解析器上持有——由调用方经 Registry.LowConfidencePages
// 按配置判定（单一实现）。
func (r *Registry) SetOCR(engine OCRAdapter, extractImages ImageExtractor) {
	for _, p := range r.parsers {
		switch pp := p.(type) {
		case *OCRParser:
			pp.Adapter = engine
		case *PDFParser:
			pp.OCR = engine
			pp.ExtractImages = extractImages
		case *DocxParser:
			pp.OCR = engine
		}
	}
}

// LowConfidencePages 返回 OCR 文档中低于阈值的页码（供 UI 黄标与审计）。
func (r *Registry) LowConfidencePages(doc *verify.DocumentText, minConfidence float64) []int {
	if !doc.OCR {
		return nil
	}
	out := []int{}
	for page, c := range doc.OCRConf {
		if minConfidence > 0 && c < minConfidence {
			out = append(out, page)
		}
	}
	return out
}

// Parse 选择首个能处理该路径的解析器执行解析。
func (r *Registry) Parse(path string) (*verify.DocumentText, error) {
	for _, p := range r.parsers {
		if p.CanParse(path) {
			return p.Parse(path)
		}
	}
	return nil, fmt.Errorf("没有可用的解析器处理文件: %s", path)
}

// TextParser 纯文本/Markdown 解析器：整文件视为第 1 页；
// 以 "\f"（form feed）作为分页符。
type TextParser struct{}

// CanParse 实现 Parser 接口。
func (t *TextParser) CanParse(path string) bool {
	return strings.HasSuffix(path, ".txt") || strings.HasSuffix(path, ".md")
}

// Parse 实现 Parser 接口。
func (t *TextParser) Parse(path string) (*verify.DocumentText, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pages := map[int]string{}
	parts := strings.Split(string(data), "\f")
	for i, part := range parts {
		pages[i+1] = part
	}
	return &verify.DocumentText{File: path, Pages: pages}, nil
}
