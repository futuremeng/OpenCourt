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

// TestRedactExtendedFields 补漏（review P1-5）：类案要旨与网页正文把 redact
// 当唯一强制关口，手机号 / 邮箱 / 统一社会信用代码 / 无市区县前缀的门牌地址
// 一旦漏网即直接泄露——必须有断言守着。
func TestRedactExtendedFields(t *testing.T) {
	r := New([]string{"张三"})
	text := "联系电话13800138000，邮箱 zhang.san@court.gov.cn，" +
		"统一社会信用代码 91110000MA01ABCD7X，住朝阳路88号院3号楼502室。"
	res := r.RedactText(text)

	for _, leak := range []string{"13800138000", "zhang.san@", "MA01ABCD7X", "88号院3号楼502室"} {
		if strings.Contains(res.Text, leak) {
			t.Errorf("敏感项泄露 %q: %s", leak, res.Text)
		}
	}
	if !strings.Contains(res.Text, "138****8000") {
		t.Errorf("手机号掩码格式错误: %s", res.Text)
	}
	if !strings.Contains(res.Text, "***@court.gov.cn") {
		t.Errorf("邮箱应保留域名: %s", res.Text)
	}
	if !strings.Contains(res.Text, "91") || !strings.Contains(res.Text, "D7X") {
		t.Errorf("信用代码应保留前2后4: %s", res.Text)
	}
	if !strings.Contains(res.Text, "朝阳路") {
		t.Errorf("门牌泛化应保留到路级: %s", res.Text)
	}
	want := map[string]bool{"USCC_MASK": true, "PHONE_MASK": true, "EMAIL_MASK": true,
		"ADDRESS_DETAIL_GENERALIZE": true}
	got := map[string]bool{}
	for _, a := range res.Actions {
		got[a.RuleID] = true
	}
	for w := range want {
		if !got[w] {
			t.Errorf("缺少脱敏动作 %s（审计需逐条留痕）: %+v", w, res.Actions)
		}
	}
}

// TestRedactNoFalsePositiveOnCaseNo 案号与年份不得被新规则误伤。
func TestRedactNoFalsePositiveOnCaseNo(t *testing.T) {
	r := New(nil)
	text := "案号 (2024)京01民初1号，2024年5月1日立案，金额 128000.50 元。"
	res := r.RedactText(text)
	for _, keep := range []string{"(2024)京01民初1号", "2024年5月1日", "128000.50"} {
		if !strings.Contains(res.Text, keep) {
			t.Errorf("不应误伤 %q: %s", keep, res.Text)
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
