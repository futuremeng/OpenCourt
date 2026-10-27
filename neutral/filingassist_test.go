package neutral

import (
	"strings"
	"testing"
)

const sampleSnapshot = `- form "立案申请"
  - heading "当事人信息"
  - textbox "原告姓名" [ref=e12]
  - textbox "原告身份证号" [ref=e13]
  - textbox "送达手机号" [ref=e14]
  - combobox "管辖法院" [ref=e20]
  - textbox "短信验证码" [ref=e30]
  - button "下一步" [ref=e40]
  - button "提交" [ref=e99]`

func TestExtractFormFields(t *testing.T) {
	els := ExtractFormFields(sampleSnapshot)
	byRef := map[string]FormElement{}
	for _, e := range els {
		byRef[e.Ref] = e
	}
	for _, ref := range []string{"e12", "e13", "e14", "e20", "e30", "e40", "e99"} {
		if _, ok := byRef[ref]; !ok {
			t.Fatalf("缺失元素 %s: %+v", ref, els)
		}
	}
	if byRef["e12"].Role != "textbox" || !strings.Contains(byRef["e12"].Label, "原告姓名") {
		t.Fatalf("e12 解析: %+v", byRef["e12"])
	}
	if byRef["e99"].Role != "button" {
		t.Fatalf("e99 角色: %+v", byRef["e99"])
	}
}

func TestBuildFillPlanMapsAndSkips(t *testing.T) {
	els := ExtractFormFields(sampleSnapshot)
	values := map[string]string{
		"原告姓名":   "王五",
		"手机号":    "13800000000",
		"短信验证码":  "123456", // 敏感字段：应进入候选但会被 gate 拦截（此处仅映射）
		"不存在的字段": "x",
	}
	factStates := map[string]string{"原告姓名": "单源未补强"}
	specs := BuildFillPlan(els, values, factStates)
	byLabel := map[string]string{}
	for _, s := range specs {
		byLabel[s.Label] = s.Ref
	}
	if _, ok := byLabel["原告姓名"]; !ok {
		t.Fatalf("原告姓名未映射: %+v", specs)
	}
	// 按钮不参与填写；未提供值的字段不进计划。
	for _, s := range specs {
		if strings.Contains(s.Label, "提交") || strings.Contains(s.Label, "下一步") {
			t.Fatalf("按钮不应进入填写计划: %+v", s)
		}
		if strings.Contains(s.Label, "管辖法院") {
			t.Fatalf("未提供值的字段不应进计划: %+v", s)
		}
	}
	for _, s := range specs {
		if s.Label == "原告姓名" && s.FactState != "单源未补强" {
			t.Fatalf("弱态标记丢失: %+v", s)
		}
	}
}
