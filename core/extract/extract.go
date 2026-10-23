// Package extract 要素抽取模块（开发文档 4.3）：
// 抽取主体信息、时间节点、金额数值、关键行为、合同条款、送达信息。
// 本阶段实现确定性的时间/金额正则抽取；主体/行为等语义要素由模型候选 +
// 核验器落地检查在后续阶段补全（模型仅生成候选，不入库未落地要素）。
package extract

import (
	"regexp"
	"time"
)

// ElementType 要素类型。
type ElementType string

const (
	ElementSubject ElementType = "SUBJECT" // 主体信息
	ElementTime    ElementType = "TIME"    // 时间节点
	ElementAmount  ElementType = "AMOUNT"  // 金额数值
	ElementAction  ElementType = "ACTION"  // 关键行为
	ElementClause  ElementType = "CLAUSE"  // 合同条款
	ElementService ElementType = "SERVICE" // 送达信息
)

// Element 一条抽取结果。Located=false 的要素禁止入库（幻觉门禁前置）。
type Element struct {
	Type       ElementType `json:"type"`
	Value      string      `json:"value"`
	SourceFile string      `json:"source_file"`
	PageNum    int         `json:"page_num"`
	Located    bool        `json:"located"` // 是否已回原文定位
}

var (
	reDateISO = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
	reDateCN  = regexp.MustCompile(`\d{4}年\d{1,2}月\d{1,2}日`)
	reAmount  = regexp.MustCompile(`(?:人民币)?\d+(?:,\d{3})*(?:\.\d+)?\s*(?:元|万元|亿元)`)
)

// ExtractTimeAndAmount 确定性抽取时间节点与金额。返回的要素均带原文定位坐标。
func ExtractTimeAndAmount(file string, pageNum int, text string) []Element {
	out := []Element{}
	for _, m := range reDateISO.FindAllString(text, -1) {
		if _, err := time.Parse("2006-01-02", m); err == nil {
			out = append(out, Element{Type: ElementTime, Value: m, SourceFile: file, PageNum: pageNum, Located: true})
		}
	}
	for _, m := range reDateCN.FindAllString(text, -1) {
		out = append(out, Element{Type: ElementTime, Value: m, SourceFile: file, PageNum: pageNum, Located: true})
	}
	for _, m := range reAmount.FindAllString(text, -1) {
		out = append(out, Element{Type: ElementAmount, Value: m, SourceFile: file, PageNum: pageNum, Located: true})
	}
	return out
}
