package ingest

import (
	"context"
	"errors"
	"strings"

	"github.com/futuremeng/OpenCourt/core/verify"
)

// OCRPage 引擎对一页的产出。Confidence 为 0–1；0 = 引擎不报告置信度
// （此时该页不参与低置信黄标判定，宁缺毋假）。
type OCRPage struct {
	PageNum    int
	Text       string
	Confidence float64
}

// OCRAdapter 扫描件/图片的 OCR 引擎适配接口。
// 引擎实现（本机 Vision helper、tesseract subprocess 等）由外部注入；
// 未注入前，OCRParser 一律拒绝处理——扫描文档绝不绕过 OCR 直接入库。
// 纪律（plan-v0.4 D1）：本地进程、永不联网；产出的文本入库封顶弱态。
type OCRAdapter interface {
	Name() string
	// Recognize 识别图像文件，返回按页组织的识别结果。
	Recognize(ctx context.Context, path string) ([]OCRPage, error)
}

// ErrOCRNotConfigured OCR 引擎未配置。
var ErrOCRNotConfigured = errors.New("扫描件/图片必须经过 OCR：当前未配置 OCR 引擎，拒绝处理")

// OCRParser 图片与扫描件解析器：仅在注入 OCRAdapter 后生效。
// 低置信黄标阈值不在本类型上持有——统一由 Registry.LowConfidencePages
// 按配置判定（单一实现，避免近重复逻辑漂移）。
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

// Parse 实现 Parser 接口。产出带 OCR 标记的文档层（D1：封顶弱态的依据）。
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
	text := map[int]string{}
	conf := map[int]float64{}
	for _, p := range pages {
		if p.PageNum < 1 {
			return nil, errors.New("OCR 页码非法（必须 ≥1），拒绝入库")
		}
		text[p.PageNum] = p.Text
		if p.Confidence > 0 {
			conf[p.PageNum] = p.Confidence
		}
	}
	return &verify.DocumentText{
		File: path, Pages: text,
		OCR: true, OCREngine: o.Adapter.Name(), OCRConf: conf,
	}, nil
}

// LowConfidencePages 判定统一收敛在 Registry.LowConfidencePages（单一实现，
// 阈值来自配置）——此处不再放第二份近重复逻辑。
