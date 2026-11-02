// 本文件由 main.go 按领域拆分而来（同包 main：Wails 绑定方法必须
// 留在 main 包，这里只是物理分文件，便于按领域阅读与评审）。
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/neutral"
	"github.com/futuremeng/OpenCourt/plugins"
)

// LocalCasesStatus 类案源状态（前端面板用）：未启用时返回指引而非空列表。
// Health 探测失败不阻断——如实返回 ok=false 与错误文本。
func (a *App) LocalCasesStatus() map[string]any {
	out := map[string]any{
		"enabled":  a.caseSource != nil && a.caseSource.Enabled(),
		"base_url": a.settings.LocalCases.BaseURL,
		"token_set": a.settings.LocalCases.TokenEnv != "" &&
			os.Getenv(a.settings.LocalCases.TokenEnv) != "",
		"guidance":     plugins.LocalCasesGuidance,
		"temp_channel": true, // LC-6 未就绪：手动 base_url+token_env 为临时通道（UI 标注）
		"disclaimer":   neutral.CaseSearchDisclaimer,
		"indexed_docs": int64(0),
		"version":      "",
		"health_ok":    false,
		"health_error": "",
	}
	if hs, ok := a.caseSource.(interface {
		Health(context.Context) (plugins.LocalCasesHealth, error)
	}); ok && a.caseSource.Enabled() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		h, err := hs.Health(ctx)
		if err != nil {
			out["health_error"] = err.Error()
		} else {
			out["health_ok"] = h.OK
			out["version"] = h.Version
			out["indexed_docs"] = h.IndexedDocuments
		}
	}
	return out
}

// SearchSimilarCases 类案检索：检索 → 要旨强制脱敏 → CaseRef 留痕
// （不进 facts，永不进五态）。未启用直接报错带指引（不降级到其他来源）。
// facet 分布只在 structured=true 时随行，且必须附固定标注（D6）；
// 语料规模（indexed_documents）由 LocalCasesStatus 同时显示，防小样本误读。
func (a *App) SearchSimilarCases(search, court, cause string, yearFrom, yearTo, topK int, names []string) (neutral.CaseSearchReport, error) {
	if a.caseSource == nil || !a.caseSource.Enabled() {
		_ = a.auditLog.Log(a.profile.Role, "CASE_SEARCH_FAIL", "localcases", "未启用")
		return neutral.CaseSearchReport{}, fmt.Errorf("%s", plugins.LocalCasesGuidance)
	}
	q := plugins.CaseQuery{
		Search: search, Court: court, Cause: cause,
		YearFrom: yearFrom, YearTo: yearTo,
		GroupBy: []string{"year", "court"}, FacetSize: 10,
	}
	report, err := neutral.RunCaseSearch(a.ctx, a.caseSource, names, q)
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "CASE_SEARCH_FAIL", search, err.Error())
		return neutral.CaseSearchReport{}, err
	}
	if topK > 0 && topK < len(report.Refs) {
		report.Refs = report.Refs[:topK]
	}
	for _, r := range report.Refs {
		if err := a.scope().AddCaseRef(r); err != nil {
			return neutral.CaseSearchReport{}, err
		}
	}
	_ = a.auditLog.Log(a.profile.Role, "CASE_SEARCH", search,
		fmt.Sprintf("命中 %d（总 %d）；精确过滤 %v；脱敏 %d 项；留痕 %d 条（非事实）",
			len(report.Refs), report.Total, report.Structured, report.RedactCount, len(report.Refs)))
	return report, nil
}

// ListCaseRefs 列出本案类案引用留痕（检索时间倒序）。
func (a *App) ListCaseRefs() ([]domain.CaseRef, error) {
	return a.scope().ListCaseRefs()
}
