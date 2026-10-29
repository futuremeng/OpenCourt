package plugins

import (
	"context"
	"fmt"

	"github.com/futuremeng/OpenLaw/sdk"
)

// law_openlaw_elements 要件库读取（plan-v0.2 M4）：按案由取 OpenLaw 要件库
// 的封闭候选集。实现走 AnalyzeElements(caseType, nil)——空事实集时全部要件
// 落入 missing_elements，即为完整要件清单（sdk 无独立列举 API，如实适配）。
// 案由无已录入要件 → 明确报错（element 表须经人工确认导入，绝不回退到
// 模型自由生成要件——V3）。

// ElementsByCaseType 按案由读取构成要件清单（ElementSource 实现）。
func (s *OpenLawSource) ElementsByCaseType(ctx context.Context, caseType string) ([]ElementSpec, error) {
	res := s.ol.AnalyzeElements(caseType, nil)
	if res.Status == sdk.StatusNotFound {
		msg := "案由无已录入构成要件"
		if len(res.Warnings) > 0 {
			msg = res.Warnings[0]
		}
		return nil, fmt.Errorf("无要件库（%s）: %s", caseType, msg)
	}
	raw, _ := res.Data["missing_elements"].([]map[string]any)
	out := make([]ElementSpec, 0, len(raw))
	for _, m := range raw {
		str := func(k string) string { v, _ := m[k].(string); return v }
		out = append(out, ElementSpec{
			ElementID:   str("element_id"),
			Name:        str("name"),
			HowToAttack: str("how_to_attack"),
		})
	}
	return out, nil
}
