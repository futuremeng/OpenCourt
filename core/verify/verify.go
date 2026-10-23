// Package verify 实现引用校验（开发文档 4.5、7.4）：
// 每条结论必须挂载卷宗页码和原文片段；若无法在源文档对应页定位，直接拒绝。
package verify

import (
	"strings"

	"github.com/futuremeng/OpenCourt/core/domain"
)

// DocumentText 已解析的源文档：文件名 → 页码 → 该页文本。
// 由 core/ingest 解析（含 OCR）后产出。
type DocumentText struct {
	File  string
	Pages map[int]string
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
