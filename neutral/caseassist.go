// caseassist 类案检索管道（plan-v0.2 §4.3，M3）：
// 检索 → 要旨/预览强制脱敏（core/redact 是唯一强制关口，LocalCases 不做脱敏）
// → 产 CaseRef 留痕（不进 facts，永不进五态）。
// 未启用 → 直接报错带指引，绝不降级到其他抓取方式（D3）。
package neutral

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/redact"
	"github.com/futuremeng/OpenCourt/plugins"
)

// CaseSearchDisclaimer 分布统计固定标注（D6，唯一合法概率类输出的强制附注）。
const CaseSearchDisclaimer = "以上为本地语料检索结果的分布，不是裁判概率或胜诉可能性"

// CaseSearchReport 类案检索报告。Structured=false 时 UI 必须显示
// 「当前索引不支持精确过滤，以下结果未按法院/案由等条件筛选」并隐藏 facet。
type CaseSearchReport struct {
	Refs        []domain.CaseRef                 `json:"refs"`
	Facets      map[string][]plugins.FacetBucket `json:"facets,omitempty"`
	Structured  bool                             `json:"structured"`
	Total       int64                            `json:"total"`
	Disclaimer  string                           `json:"disclaimer"`
	RedactCount int                              `json:"redact_count"` // 脱敏动作数（审计/提示）
}

// RunCaseSearch 类案检索管道：src 必须实现 FilteredCaseSource（LocalCases）。
// names 为已知当事人姓名（脱敏一致替换用）。返回的 Summary 已强制脱敏；
// 未脱敏原文不回流模型、不入 UI。
func RunCaseSearch(ctx context.Context, src plugins.CaseSourcePlugin, names []string, q plugins.CaseQuery) (CaseSearchReport, error) {
	if src == nil {
		return CaseSearchReport{}, fmt.Errorf("%s", plugins.LocalCasesGuidance)
	}
	fs, ok := src.(plugins.FilteredCaseSource)
	if !ok {
		return CaseSearchReport{}, fmt.Errorf("类案源 %s 不支持过滤检索（需要 LocalCases）", src.Name())
	}
	res, err := fs.SearchFiltered(ctx, q)
	if err != nil {
		return CaseSearchReport{}, err
	}
	red := redact.New(names)
	report := CaseSearchReport{
		Refs:       make([]domain.CaseRef, 0, len(res.Hits)),
		Structured: res.Structured,
		Total:      res.Total,
		Disclaimer: CaseSearchDisclaimer,
	}
	for _, h := range res.Hits {
		// 脱敏唯一强制关口在 OpenCourt（§脱敏纪律）：预览/要旨进 UI 前必过 redact。
		rr := red.RedactText(h.Preview)
		report.RedactCount += len(rr.Actions)
		report.Refs = append(report.Refs, domain.CaseRef{
			RefID:        newRefID(),
			SourceCaseID: h.SourceCaseID,
			SourceURL:    h.URL,
			CaseName:     h.CaseName,
			Court:        h.Court,
			Cause:        h.Cause,
			JudgmentDate: h.JudgmentDate,
			LegalBasis:   h.LegalBasis,
			Summary:      rr.Text,
			RetrievedAt:  time.Now().UTC(),
		})
	}
	// facet 只在 structured=true 时随行（无精确层时聚合不可信——不呈现）。
	if res.Structured && len(res.Facets) > 0 {
		report.Facets = res.Facets
		if fj, err := json.Marshal(res.Facets); err == nil {
			for i := range report.Refs {
				report.Refs[i].FacetsJSON = string(fj)
			}
		}
	}
	return report, nil
}

func newRefID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("CASE-%d", time.Now().UnixNano())
	}
	return "CASE-" + hex.EncodeToString(b)
}
