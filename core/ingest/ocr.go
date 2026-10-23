package ingest

import (
	"context"
	"errors"
	"strings"

	"github.com/futuremeng/OpenCourt/core/verify"
)

// OCRAdapter 扫描件/图片的 OCR 引擎适配接口。
// 引擎实现（本地 Tesseract、PaddleOCR sidecar 等）由外部注入；
// 未注入前，OCRParser 一律拒绝处理——扫描文档绝不绕过 OCR 直接入库。
type OCRAdapter interface {
	Name() string
	// Recognize 识别图像文件，返回页码 → 文本。
	Recognize(ctx context.Context, path string) (map[int]string, error)
}

// ErrOCRNotConfigured OCR 引擎未配置。
var ErrOCRNotConfigured = errors.New("扫描件/图片必须经过 OCR：当前未配置 OCR 引擎，拒绝处理")

// OCRParser 图片与扫描件解析器：仅在注入 OCRAdapter 后生效。
type OCRParser struct {
	Adapter OCRAdapter
}

// CanParse 实现 Parser 接口。
func (o *OCRParser) CanParse(path string) bool {
	switch strings.ToLower(path[strings.LastIndexAny(path, ".")+1:]) {
	case "png", "jpg", "jpeg", "bmp", "tif", "tiff":
		return true
	}
	return false
}

// Parse 实现 Parser 接口。
func (o *OCRParser) Parse(path string) (*verify.DocumentText, error) {
	if o.Adapter == nil {
		return nil, ErrOCRNotConfigured
	}
	pages, err := o.Adapter.Recognize(context.Background(), path)
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, errors.New("OCR 未产出任何文本，拒绝入库")
	}
	return &verify.DocumentText{File: path, Pages: pages}, nil
}
