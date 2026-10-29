package plugins

import (
	"context"
	"fmt"

	"github.com/futuremeng/OpenLaw/sdk"
)

// law_openlaw_verdict 引用校验透传层（plan-v0.2 4.1）：CheckCitation /
// FindRelation / ResolveVersion 三方法 + 三 DTO。
// 关键纪律：VerifyResult 直接采用 OpenLaw 错误码（OL-1001..OL-1006），
// 不另建同义枚举——避免语义漂移；断链（OL-1006）数据照常返回，
// 「无法溯源」由界面标注（OpenLaw 13.4）。

// CitationVerdict 文书引用核验判定（CheckCitation 透传）。
type CitationVerdict struct {
	// VerifyResult OK 或 OL-1001..OL-1006（OpenLaw 错误码原样）。
	VerifyResult string `json:"verify_result"`
	// Status 效力状态：透传 effect_status；OL-1 未就绪前一律 UNKNOWN（V1）。
	Status string `json:"status"`
	// ConfirmStatus confirmed / pending：pending 原样上浮，不得静默通过。
	ConfirmStatus string `json:"confirm_status"`
	ArticleID     string `json:"article_id"`
	// LawID OpenLaw 法规标识（relation 查询键）。SDK 现不返回 law_id：
	// 为空时上层关系查询必须 fail-closed 报错，绝不猜。
	LawID      string   `json:"law_id"`
	ArticleNo  string   `json:"article_no"`
	Content    string   `json:"content,omitempty"` // 原文（界面比对用；非语料副本，禁止再作召回源）
	Similarity float64  `json:"similarity"`
	Warnings   []string `json:"warnings,omitempty"`
	TraceDocID string   `json:"trace_doc_id,omitempty"`
	ChainOK    bool     `json:"chain_ok"`
}

// Relation 法条关系一条（FindRelation 透传；机器不自动判定废止）。
type Relation struct {
	SrcLawID      string `json:"src_law_id"`
	DstLawID      string `json:"dst_law_id"`
	Type          string `json:"relation_type"`
	BasedOnDocID  string `json:"based_on_doc_id,omitempty"`
	ConfirmStatus string `json:"confirm_status"` // pending → UI 必须标「待人工确认」
}

// VersionInfo 版本解析结果（ResolveVersion 透传）。
type VersionInfo struct {
	VersionID     string `json:"version_id"`
	VersionNo     string `json:"version_no"`
	EffectiveDate string `json:"effective_date"`
	ExpireDate    string `json:"expire_date"`
	ConfirmStatus string `json:"confirm_status"`
}

// CheckCitation 引用核验：法规名 + 条款号 + 引用文本 + 时间点。
// 六分支（OK / OL-1001 / OL-1002 / OL-1003 / OL-1004 / OL-1005 / OL-1006）
// 全部映射到 VerifyResult；仅传输层失败返回 error。
func (s *OpenLawSource) CheckCitation(ctx context.Context, lawName, articleNo, quote, atDate string) (CitationVerdict, error) {
	res := s.ol.CheckCitation(lawName, articleNo, quote, atDate)
	v := CitationVerdict{Status: "UNKNOWN", ConfirmStatus: "pending"}
	if res.Trace != nil {
		v.TraceDocID = res.Trace.DocID
		v.ChainOK = res.Trace.ChainOK
	}
	v.Warnings = res.Warnings
	if d := res.Data; d != nil {
		v.ArticleID, _ = d["article_id"].(string)
		v.LawID, _ = d["law_id"].(string) // SDK 暂未提供：留空，上层 fail-closed
		v.ArticleNo, _ = d["article_no"].(string)
		v.Content, _ = d["content"].(string)
		if sim, ok := d["similarity"].(float64); ok {
			v.Similarity = sim
		}
		if cs, _ := d["confirm_status"].(string); cs == "confirmed" {
			v.ConfirmStatus = "confirmed"
		}
		if es, _ := d["effect_status"].(string); es != "" {
			v.Status = es // 透传；无则保守 UNKNOWN
		}
	}
	switch res.Status {
	case sdk.StatusOK:
		if res.ErrorCode != "" {
			v.VerifyResult = res.ErrorCode // OL-1004 引用不一致 / OL-1005 等在 ok 族内的细分
		} else {
			v.VerifyResult = "OK"
		}
	case sdk.StatusExpired:
		v.VerifyResult = orCode(res.ErrorCode, sdk.CodeVersionExpired)
	case sdk.StatusBrokenChain:
		v.VerifyResult = orCode(res.ErrorCode, sdk.CodeBrokenChain)
	default: // not_found / need_manual
		if res.ErrorCode == "" {
			return v, fmt.Errorf("引用核验失败: %v", res.Warnings)
		}
		v.VerifyResult = res.ErrorCode
	}
	return v, nil
}

// FindRelation 法条关系查询（修改/废止/解释/配套）。pending 关系原样返回，
// 由界面标「待人工确认」——机器不自动判定废止。
func (s *OpenLawSource) FindRelation(ctx context.Context, articleID string) ([]Relation, error) {
	res := s.ol.FindRelation(articleID)
	if res.Status == sdk.StatusNotFound {
		return nil, fmt.Errorf("关系查询失败: %v", res.Warnings)
	}
	raw, _ := res.Data["relations"].([]map[string]any)
	out := make([]Relation, 0, len(raw))
	for _, m := range raw {
		str := func(k string) string { v, _ := m[k].(string); return v }
		out = append(out, Relation{
			SrcLawID:      str("src_law_id"),
			DstLawID:      str("dst_law_id"),
			Type:          str("relation_type"),
			BasedOnDocID:  str("based_on_doc_id"),
			ConfirmStatus: str("confirm_status"),
		})
	}
	return out, nil
}

// ResolveVersion 版本解析：按法规名与时间点取当时生效版本。
func (s *OpenLawSource) ResolveVersion(ctx context.Context, lawName, atDate string) (VersionInfo, error) {
	res := s.ol.ResolveVersion(lawName, atDate)
	if res.Status == sdk.StatusNotFound {
		return VersionInfo{}, fmt.Errorf("版本解析失败: %v", res.Warnings)
	}
	d := res.Data
	info := VersionInfo{ConfirmStatus: "pending"}
	info.VersionID, _ = d["version_id"].(string)
	info.VersionNo, _ = d["version_no"].(string)
	info.EffectiveDate, _ = d["effective_date"].(string)
	info.ExpireDate, _ = d["expire_date"].(string)
	if cs, _ := d["confirm_status"].(string); cs == "confirmed" {
		info.ConfirmStatus = "confirmed"
	}
	return info, nil
}

func orCode(code, def string) string {
	if code != "" {
		return code
	}
	return def
}
