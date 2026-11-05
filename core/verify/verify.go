// Package verify 实现引用校验（开发文档 4.5、7.4）：
// 每条结论必须挂载卷宗页码和原文片段；若无法在源文档对应页定位，直接拒绝。
package verify

import (
	"strings"

	"github.com/futuremeng/OpenCourt/core/domain"
)

// DocumentText 已解析的源文档：文件名 → 页码 → 该页文本。
// 由 core/ingest 解析（含 OCR）后产出。
// OCR 标记（D1 纪律）：文本来自 OCR 引擎的文档，五态升级**永久封顶弱态**——
// 状态机拒绝其事实进入「已溯源确认」；OCRConf 为引擎逐页置信度（0–1，
// 缺 key = 引擎未报告），低置信页由 UI 黄标上浮，纠正只允许人工编辑后重验。
type DocumentText struct {
	File      string
	Pages     map[int]string
	OCR       bool
	OCREngine string
	OCRConf   map[int]float64
	// Annotations 证据线索（D2）：修订痕迹、批注、未识别内嵌图等。
	// **不进 Pages**——回原文定位对标注文本一律失败，标注只能作线索上浮，
	// 不能作事实的正文引用来源（不自动采信）。
	Annotations []Annotation
}

// Annotation 独立标注区块条目（Kind 封闭集：revision_insert /
// revision_delete / comment / image）。
type Annotation struct {
	Kind   string `json:"kind"`
	Author string `json:"author,omitempty"`
	Text   string `json:"text"`
	Meta   string `json:"meta,omitempty"`
}

// Locate 核验器核心：只回原文，不看结论。
// 检查引用片段能否在指定卷宗的指定页定位。定位失败返回 false，
// 调用方（neutral/verifier 与状态机）据此打回或拒绝状态升级。
func Locate(c domain.Citation, docs map[string]*DocumentText) bool {
	if !c.Valid() {
		return false
	}
	doc, ok := docs[c.SourceFile]
	if !ok {
		return false
	}
	pageText, ok := doc.Pages[c.PageNum]
	if !ok {
		return false
	}
	return strings.Contains(normalize(pageText), normalize(c.Snippet))
}

// normalize 去除空白差异，避免排版导致的定位失败。
func normalize(s string) string {
	return strings.Join(strings.Fields(s), "")
}
