// 本文件由 main.go 按领域拆分而来（同包 main：Wails 绑定方法必须
// 留在 main 包，这里只是物理分文件，便于按领域阅读与评审）。
package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/neutral"
	"github.com/futuremeng/OpenCourt/plugins"
)

// SearchLaw 法条关键词检索。法条源未配置显式报错（不静默冒充空结果）。
func (a *App) SearchLaw(query string, topK int) ([]plugins.LawHit, error) {
	var hits []plugins.LawHit
	err := a.withLaw(func(src plugins.LawSourcePlugin) error {
		var err error
		hits, err = neutral.SearchLaw(context.Background(), src, query, topK)
		return err
	})
	if err != nil {
		return nil, err
	}
	_ = a.auditLog.Log(a.profile.Role, "LAW_SEARCH", "", query)
	return hits, nil
}

// VerifyDocLawCitations 文书法条引用核验：确定性抽取「《法名》第X条」→ 逐条
// 交 OpenLaw CheckCitation → 引用留痕入当前案件 law_refs（跨案件天然隔离）。
// 断链（OL-1006）留痕照常入库，界面必须标注「无法溯源」。
func (a *App) VerifyDocLawCitations(docID, atDate string) ([]domain.LawRef, error) {
	doc, err := a.scope().GetDocument(docID)
	if err != nil {
		return nil, err
	}
	pages := make([]int, 0, len(doc.Pages))
	for p := range doc.Pages {
		pages = append(pages, p)
	}
	sort.Ints(pages)
	var sb strings.Builder
	for _, p := range pages {
		sb.WriteString(doc.Pages[p])
		sb.WriteString("\n")
	}
	drafts := neutral.ExtractLawCitations(sb.String())
	if len(drafts) == 0 {
		return []domain.LawRef{}, nil
	}
	var refs []domain.LawRef
	err = a.withLaw(func(src plugins.LawSourcePlugin) error {
		verifier, ok := src.(neutral.LawVerifier)
		if !ok {
			return fmt.Errorf("法条源未配置（profile: openlaw.data_dir）：引用核验不可用")
		}
		var verr error
		refs, verr = neutral.VerifyCitations(context.Background(), verifier, atDate, drafts)
		return verr
	})
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "LAW_VERIFY_FAIL", docID, err.Error())
		return nil, err
	}
	sc := a.scope()
	for _, r := range refs {
		if err := sc.AddLawRef(r); err != nil {
			return nil, err
		}
		if r.VerifyResult != "OK" {
			_ = a.auditLog.Log(a.profile.Role, "LAW_VERIFY_FAIL", docID,
				fmt.Sprintf("%s %s → %s", r.LawName, r.ArticleNo, r.VerifyResult))
		}
	}
	_ = a.auditLog.Log(a.profile.Role, "LAW_VERIFY", docID, fmt.Sprintf("%d 条引用", len(refs)))
	return refs, nil
}

// ListLawRefs 本案法条引用留痕（创建时间倒序）。
func (a *App) ListLawRefs() ([]domain.LawRef, error) {
	return a.scope().ListLawRefs()
}

// GetLawRelations 法条关系（修改/废止/解释/配套）：pending 关系原样返回，
// 界面必须标「待人工确认」——机器不自动判定废止。
func (a *App) GetLawRelations(refID string) ([]plugins.Relation, error) {
	refs, err := a.scope().ListLawRefs()
	if err != nil {
		return nil, err
	}
	var lawID string
	found := false
	for _, r := range refs {
		if r.RefID == refID {
			lawID, found = r.LawID, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("法条引用不存在: %s", refID)
	}
	// relation 表以 law_id 为键；SDK 现不返回 law_id——为空时 fail-closed
	// 报错并给出出路，绝不拿 article_id 去猜。
	if lawID == "" {
		return nil, fmt.Errorf("引用 %s 缺法规标识（law_id 待 OpenLaw SDK 透传，见桥接清单缺口）：关系查询不可用", refID)
	}
	var rel []plugins.Relation
	err = a.withLaw(func(src plugins.LawSourcePlugin) error {
		verifier, ok := src.(neutral.LawVerifier)
		if !ok {
			return fmt.Errorf("法条源未配置：法条关系查询不可用")
		}
		var verr error
		rel, verr = verifier.FindRelation(context.Background(), lawID)
		return verr
	})
	if err != nil {
		return nil, err
	}
	return rel, nil
}

// --- 类案检索（v0.2 M3，plan-v0.2 §4.2/§4.3；docs/localcases-integration.md）---
