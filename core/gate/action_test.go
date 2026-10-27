package gate

import "testing"

func TestCheckBrowserActionRedlines(t *testing.T) {
	cases := []struct {
		kind, label string
		allowed     bool
		rule        string
	}{
		{"click", "提交", false, "AUTO_SUBMIT_FORBIDDEN"},
		{"click", "提交申请", false, "AUTO_SUBMIT_FORBIDDEN"},
		{"click", "确认立案", false, "AUTO_SUBMIT_FORBIDDEN"},
		{"click", "在线支付", false, "AUTO_SUBMIT_FORBIDDEN"},
		{"click", "电子签名", false, "AUTO_SUBMIT_FORBIDDEN"},
		{"click", "下一步", true, ""},
		{"click", "选择管辖法院", true, ""},
		// zxfw 实测词汇（docs/zxfw-sitemap.md §6-§7）：最终提交绝对红线
		{"click", "预览和提交", false, "AUTO_SUBMIT_FORBIDDEN"},
		{"click", "正式提交", false, "AUTO_SUBMIT_FORBIDDEN"},
		{"click", "重新提起", false, "AUTO_SUBMIT_FORBIDDEN"},
		{"click", "提起申请", false, "AUTO_SUBMIT_FORBIDDEN"},
		{"click", "递交起诉状", false, "AUTO_SUBMIT_FORBIDDEN"},
		{"click", "Submit", false, "AUTO_SUBMIT_FORBIDDEN"},
		{"click", "Pay Fees", false, "AUTO_SUBMIT_FORBIDDEN"},
		{"click", "Delete", false, "AUTO_SUBMIT_FORBIDDEN"},
		// 流程内非提交动作允许（仍需两段式人工确认）
		{"click", "生成起诉状", true, ""},
		{"click", "选择要素式立案", true, ""},
		{"click", "制作示范文本", true, ""},
		{"click", "上一步", true, ""},
		{"fill", "短信验证码", false, "SENSITIVE_FIELD_FORBIDDEN"},
		{"fill", "登录密码", false, "SENSITIVE_FIELD_FORBIDDEN"},
		{"fill", "原告姓名", true, ""},
		{"fill", "诉讼请求金额", true, ""},
	}
	for _, c := range cases {
		v := CheckBrowserAction(c.kind, c.label)
		if v.Allowed != c.allowed {
			t.Errorf("%s %q: allowed=%v 期望 %v (%s)", c.kind, c.label, v.Allowed, c.allowed, v.Reason)
		}
		if !c.allowed && v.Rule != c.rule {
			t.Errorf("%s %q: rule=%q 期望 %q", c.kind, c.label, v.Rule, c.rule)
		}
	}
}
