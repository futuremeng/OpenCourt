// action.go 浏览器辅助动作红线（docs/browseros-integration.md 5.3）：
// 「人做认证、人做决定、人做提交」不是提示词约束，而是代码级强制——
// 提交/支付/签名类动作与凭据/验证码类字段一律在此拦截，
// 插件的 ExecuteAction 只放行本门放行的候选。
package gate

import "regexp"

// reSubmitActions 必须由人亲自执行的最终动作（点击类）。
var reSubmitActions = regexp.MustCompile(`提\s*交|确认立案|申请立案|确认申请|确认提交|缴纳|支付|签名|签章|发送|删除`)

// reSensitiveFields 必须由人亲自填写的字段（填写类）：凭据/验证码/签名。
var reSensitiveFields = regexp.MustCompile(`密码|口令|验证码|校验码|动态码|短信|验证短信|签名|签章|人脸`)

// ActionVerdict 动作红线判定结果。
type ActionVerdict struct {
	Allowed bool   `json:"allowed"`
	Rule    string `json:"rule,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// CheckBrowserAction 判定浏览器辅助动作是否允许自动执行。
// kind: fill / click；label 为目标元素的可见标签。
func CheckBrowserAction(kind, label string) ActionVerdict {
	switch kind {
	case "click":
		if reSubmitActions.MatchString(label) {
			return ActionVerdict{Allowed: false, Rule: "AUTO_SUBMIT_FORBIDDEN",
				Reason: "提交/支付/签名类操作必须由当事人亲自执行，辅助工具不代做最终动作"}
		}
	case "fill":
		if reSensitiveFields.MatchString(label) {
			return ActionVerdict{Allowed: false, Rule: "SENSITIVE_FIELD_FORBIDDEN",
				Reason: "密码/验证码/签名类字段必须由当事人亲自填写，不代理凭据与实名环节"}
		}
	}
	return ActionVerdict{Allowed: true}
}
