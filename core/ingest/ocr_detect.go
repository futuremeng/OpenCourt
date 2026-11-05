// 引擎探测与选择（plan-v0.4 D1 探测顺序）：显式指定 → Vision helper →
// PATH 中 tesseract → 皆无 = nil（扫描件维持明确拒绝，绝不静默）。
package ingest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// PageImages 一页的扫描图像：同页多内嵌图（彩色层/mask/分段扫描）按序归组，
// OCR 时按序拼接——不得因取图数与页数不等而误拒整文档。
type PageImages struct {
	Page  int
	Paths []string
}

// ImageExtractor 从扫描型 PDF 抽出内嵌图像并按页归组。返回的 cleanup
// 必须被调用方 defer——临时目录不得泄漏。取不到图即报错，绝不猜测内容。
type ImageExtractor func(path string) (pages []PageImages, cleanup func(), err error)

// OCRConfig 引擎选择配置（config/settings.yaml ingest.ocr）。
type OCRConfig struct {
	Engine string // none（默认）/ auto / vision / tesseract
	Path   string // 引擎二进制或 helper 路径（可选）
	Lang   string // tesseract 语言包（默认 chi_sim+eng）
}

// DetectOCR 按探测顺序返回引擎与人类可读说明。返回 nil 引擎时扫描件
// 维持 ErrOCRNotConfigured——指定引擎不可用也**绝不降级到其他引擎**。
func DetectOCR(c OCRConfig) (OCRAdapter, string) {
	switch strings.ToLower(strings.TrimSpace(c.Engine)) {
	case "", "none":
		return nil, ""
	case "vision":
		v := &VisionAdapter{HelperPath: resolveVisionHelper(c.Path)}
		if v.Available() {
			return v, ""
		}
		return nil, "OCR 指定 vision 但 helper 不可用：扫描件维持拒绝（不降级到其他引擎）"
	case "tesseract":
		t := &TesseractAdapter{Binary: c.Path, Lang: c.Lang}
		if t.Available() {
			return t, ""
		}
		return nil, "OCR 指定 tesseract 但二进制不可用：扫描件维持拒绝"
	case "auto":
		if runtime.GOOS == "darwin" {
			v := &VisionAdapter{HelperPath: resolveVisionHelper("")}
			if v.Available() {
				return v, ""
			}
		}
		t := &TesseractAdapter{Lang: c.Lang}
		if t.Available() {
			return t, ""
		}
		return nil, ""
	default:
		return nil, fmt.Sprintf("OCR 引擎 %q 未知：扫描件维持明确拒绝", c.Engine)
	}
}

// resolveVisionHelper helper 路径解析：显式配置 → .app 包内 Resources →
// 可执行文件同目录 → PATH。
func resolveVisionHelper(explicit string) string {
	if explicit != "" {
		return explicit
	}
	exe, err := os.Executable()
	if err == nil {
		if i := strings.Index(exe, ".app/Contents/MacOS/"); i >= 0 {
			if p := exe[:i+len(".app")] + "/Contents/Resources/ocr_helper"; exists(p) {
				return p
			}
		}
		if p := filepath.Join(filepath.Dir(exe), "ocr_helper"); exists(p) {
			return p
		}
	}
	if p, err := exec.LookPath("ocr_helper"); err == nil {
		return p
	}
	return ""
}

func exists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
