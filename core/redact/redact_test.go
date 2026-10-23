package redact

import (
	"strings"
	"testing"
)

func TestRedactText(t *testing.T) {
	r := New([]string{"张三", "李四"})
	text := "张三的身份证号是 110101199001011234，银行账号 6222020200112345678，" +
		"住址为北京市朝阳区朝阳路88号院3号楼。李四于 2024 年起诉，案号 (2024)京01民初1号。"
	res := r.RedactText(text)

	if strings.Contains(res.Text, "张三") || strings.Contains(res.Text, "李四") {
		t.Errorf("姓名未被替换: %s", res.Text)
	}
	if !strings.Contains(res.Text, "PERSON_001") || !strings.Contains(res.Text, "PERSON_002") {
		t.Errorf("一致替换失败: %s", res.Text)
	}
	if !strings.Contains(res.Text, "110101****1234") {
		t.Errorf("身份证掩码错误: %s", res.Text)
	}
	if !strings.Contains(res.Text, "6222****5678") {
		t.Errorf("银行账号掩码错误: %s", res.Text)
	}
	if strings.Contains(res.Text, "朝阳路88号") {
		t.Errorf("地址未泛化: %s", res.Text)
	}
	if !strings.Contains(res.Text, "(2024)京01民初1号") {
		t.Errorf("案号应保留: %s", res.Text)
	}
	if len(res.Actions) == 0 {
		t.Fatal("脱敏操作未记录审计动作")
	}
	for _, a := range res.Actions {
		if a.RuleID == "NAME_REPLACE" && a.OriginalHash == "" {
			t.Error("脱敏审计缺少原始哈希")
		}
	}
}

func TestRestoreTable(t *testing.T) {
	r := New([]string{"王五"})
	res := r.RedactText("王五签署了合同。")
	if !strings.Contains(res.Text, "PERSON_001") {
		t.Fatalf("替换失败: %s", res.Text)
	}
	if r.Restore("PERSON_001") != "王五" {
		t.Error("还原表失败")
	}
	if r.Restore("PERSON_999") != "PERSON_999" {
		t.Error("未知 token 应原样返回")
	}
	if len(r.RestoreTable()) != 1 {
		t.Error("还原表条目数错误")
	}
}
