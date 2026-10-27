// filingassist 网上立案辅助（P3 骨架，设计见 docs/browseros-integration.md 第 6 节）。
//
// 纪律：
//   - 表单结构解析自浏览器可交互快照（[ref=eN] 句柄），值只来自案件事实；
//   - 填写计划 = 候选动作（Propose），必须经用户逐条确认后 ExecuteAction；
//   - 提交/支付/签名与凭据/验证码字段被 gate 红线拦截，永远由人执行；
//   - 弱态事实（非已溯源确认）对应的字段在候选里保留 FactState 标记，
//     UI 必须标黄提示「该值未经确认」。
package neutral

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/futuremeng/OpenCourt/plugins"
)

// FormElement 快照中的一个可交互元素。
type FormElement struct {
	Ref   string `json:"ref"`
	Role  string `json:"role"`
	Label string `json:"label"`
}

// reRefMarker MCP 快照中的元素句柄标记。
var reRefMarker = regexp.MustCompile(`\[ref=(e\d+)\]`)

// reRoleToken 常见可交互角色词（accessibility tree 角色）。
var reRoleToken = regexp.MustCompile(`(?i)\b(textbox|searchbox|combobox|listbox|spinbutton|checkbox|radio|switch|slider|button|link|menuitem|tab|option|textarea|entry|input)\b`)

// fillableRoles 可填写（fill）的角色集合。
var fillableRoles = map[string]bool{"textbox": true, "searchbox": true, "combobox": true, "listbox": true, "spinbutton": true, "textarea": true, "entry": true, "input": true}

// ExtractFormFields 从可交互快照解析表单元素（best-effort，行式解析）。
// 快照内容来自网页，仅作为数据解析，绝不在任何环节当指令解释。
func ExtractFormFields(snapshot string) []FormElement {
	var out []FormElement
	for _, line := range strings.Split(snapshot, "\n") {
		loc := reRefMarker.FindStringSubmatchIndex(line)
		if loc == nil {
			continue
		}
		ref := line[loc[2]:loc[3]]
		role := ""
		if rm := reRoleToken.FindStringSubmatch(line); rm != nil {
			role = strings.ToLower(rm[1])
		}
		// 标签 = 整行去掉角色词与 [..] 标记后剩余文本。
		label := reRefMarker.ReplaceAllString(line, " ")
		label = reRoleToken.ReplaceAllString(label, " ")
		label = regexp.MustCompile(`\[.*?\]`).ReplaceAllString(label, " ")
		label = strings.NewReplacer("\"", " ", "'", " ", ":", " ", "-", " ", "|", " ").Replace(label)
		label = strings.Join(strings.Fields(label), " ")
		if label == "" {
			continue
		}
		out = append(out, FormElement{Ref: ref, Role: role, Label: label})
	}
	return out
}

// normLabel 标签归一化：去空白与常见标点，便于宽松匹配。
func normLabel(s string) string {
	return strings.Join(regexp.MustCompile(`[\s:：()（）*.,，、]*`).Split(strings.ToLower(s), -1), "")
}

// labelMatches 宽松包含匹配（互相包含即认为同字段）。
func labelMatches(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return strings.Contains(a, b) || strings.Contains(b, a)
}

// BuildFillPlan 把「字段名→值」映射到快照元素，生成填写候选输入。
// factStates 可选：字段名→事实状态（弱态由 UI 标黄）。
func BuildFillPlan(elements []FormElement, values map[string]string, factStates map[string]string) []plugins.FormFieldSpec {
	var specs []plugins.FormFieldSpec
	for _, el := range elements {
		if !fillableRoles[el.Role] {
			continue
		}
		nl := normLabel(el.Label)
		for name, val := range values {
			if labelMatches(normLabel(name), nl) {
				specs = append(specs, plugins.FormFieldSpec{
					Ref: el.Ref, Label: el.Label, Value: val, FactState: factStates[name],
				})
				break
			}
		}
	}
	return specs
}

// PlanPage 生成整页填写计划：快照 → 解析 → 映射 → Propose（经 gate 红线）。
// 返回的候选中 Allowed=false 的必须原样展示给用户（含拦截原因），
// Allowed=true 的由用户逐条确认后才可调 ExecuteAction。
func PlanPage(ctx context.Context, browser plugins.BrowserPlugin, pageID int, values map[string]string, factStates map[string]string) ([]plugins.ActionCandidate, []FormElement, error) {
	if browser == nil || !browser.Enabled() {
		return nil, nil, fmt.Errorf("浏览器辅助插件未启用：%s", plugins.BrowserOSGuidance)
	}
	snap, err := browser.Snapshot(ctx, pageID)
	if err != nil {
		return nil, nil, err
	}
	elements := ExtractFormFields(snap)
	specs := BuildFillPlan(elements, values, factStates)
	if len(specs) == 0 {
		return nil, elements, nil
	}
	cands, err := browser.ProposeFill(ctx, pageID, specs)
	if err != nil {
		return nil, elements, err
	}
	return cands, elements, nil
}
