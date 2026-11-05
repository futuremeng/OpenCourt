// OCR 引擎适配（plan-v0.4 D1）：两个内置引擎均为本地 subprocess——
// 主进程零 CGO、零网络路径；引擎缺失即维持 ErrOCRNotConfigured 明确拒绝。
package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// VisionAdapter macOS 系统 Vision OCR（VNRecognizeTextRequest，zh-Hans 优先）。
// 经随 .app 打包的极小 Swift helper subprocess 调用——系统内置能力，
// 中文精度为免费方案最高，体积近零。仅 macOS 可用。
type VisionAdapter struct {
	// HelperPath 为 helper 可执行文件路径（探测顺序见 DetectOCR）。
	HelperPath string
}

func (v *VisionAdapter) Name() string { return "vision" }

// Available 报告 helper 是否存在可执行。
func (v *VisionAdapter) Available() bool {
	if v.HelperPath == "" {
		return false
	}
	_, err := exec.LookPath(v.HelperPath)
	return err == nil
}

// Recognize 调 helper（参数 = 图片路径），helper 输出
// {"text": "...", "confidence": 0.0-1.0}。识别失败原样上抛，绝不降级。
func (v *VisionAdapter) Recognize(ctx context.Context, path string) ([]OCRPage, error) {
	if !v.Available() {
		return nil, fmt.Errorf("Vision helper 不可用（%s）：请确认应用包内已打包 ocr_helper，或经 ingest.ocr.path 指定", v.HelperPath)
	}
	out, err := exec.CommandContext(ctx, v.HelperPath, path).Output()
	if err != nil {
		return nil, fmt.Errorf("Vision OCR 失败: %w", err)
	}
	var r struct {
		Text       string  `json:"text"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("Vision helper 输出解析失败: %w", err)
	}
	if strings.TrimSpace(r.Text) == "" {
		return nil, nil // 空白页：由上层按「未产出文本」拒绝入库
	}
	return []OCRPage{{PageNum: 1, Text: r.Text, Confidence: r.Confidence}}, nil
}
