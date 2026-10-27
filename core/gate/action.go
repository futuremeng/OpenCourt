// action.go 浏览器辅助动作红线（docs/browseros-integration.md 5.3）：
// 「人做认证、人做决定、人做提交」不是提示词约束，而是代码级强制——
// 提交/支付/签名类动作与凭据/验证码类字段一律在此拦截，
// 插件的 ExecuteAction 只放行本门放行的候选。
package gate

import "regexp"

// reSubmitActions 必须由人亲自执行的最终动作（点击类）。
// 立案最终提交为绝对红线：无论当事人是否在界面上二次确认，本门一律拦截，
// 插件 ExecuteAction 永不可执行被本门拦截的候选（两段式确认也不能绕过）。
// 词表覆盖 zxfw（人民法院在线服务网）实测按钮词汇（docs/zxfw-sitemap.md §6-§7）。
var reSubmitActions = regexp.MustCompile(`(?i)提\s*交|递交|重新提起|提起申请|确认立案|申请立案|确认申请|正式申请|缴纳|支付|缴费|\bpay\b|\bsubmit\b|\bdelete\b|签名|签章|签署|\bsign\b|发送|\bsend\b|删除|撤回申请`)

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
