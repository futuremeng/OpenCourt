package verify

import (
	"testing"

	"github.com/futuremeng/OpenCourt/core/domain"
)

func TestLocate(t *testing.T) {
	docs := map[string]*DocumentText{
		"contract.txt": {File: "contract.txt", Pages: map[int]string{
			1: "甲方与乙方于 2024 年 3 月 1 日签订本合同。",
			2: "付款方式：银行转账。",
		}},
	}
	ok := Locate(domain.Citation{SourceFile: "contract.txt", PageNum: 1, Snippet: "2024 年 3 月 1 日"}, docs)
	if !ok {
		t.Error("可定位引用应返回 true")
	}
	if Locate(domain.Citation{SourceFile: "contract.txt", PageNum: 3, Snippet: "片段"}, docs) {
		t.Error("不存在的页码应定位失败")
	}
	if Locate(domain.Citation{SourceFile: "other.txt", PageNum: 1, Snippet: "片段"}, docs) {
		t.Error("不存在的文件应定位失败")
	}
	if Locate(domain.Citation{SourceFile: "contract.txt", PageNum: 1, Snippet: "虚构的原文"}, docs) {
		t.Error("虚构片段应定位失败")
	}
	// 空白差异不应影响定位。
	if !Locate(domain.Citation{SourceFile: "contract.txt", PageNum: 1, Snippet: "签订本 合同"}, docs) {
		t.Error("空白差异应被规范化")
	}
}
