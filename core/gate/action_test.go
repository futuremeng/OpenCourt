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
