package plugins

import (
	"context"
	"fmt"

	"github.com/futuremeng/OpenLaw/sdk"
)

// law_openlaw OpenCourt 法条源适配器（M4 对接）：进程内调用 OpenLaw sdk
// （import 对接面，禁止直连数据库——architecture 纪律）。OpenLaw 数据目录
// （db/ pdf/ md/ provenance/ manifest/，签名可校验）随应用安装/迁移，完全离线。

// OpenLawSource 实现 LawSourcePlugin：检索与详情均携带版本区间与生效状态。
type OpenLawSource struct {
	ol *sdk.OpenLaw
}

// NewOpenLawSource 打开 OpenLaw 数据目录；dataDir 为空返回 (nil, nil)——
// 未配置法条源时调用方降级（框架不内置任何法条数据）。
func NewOpenLawSource(dataDir string) (*OpenLawSource, error) {
	if dataDir == "" {
		return nil, nil
	}
	ol, err := sdk.New(dataDir)
	if err != nil {
		return nil, fmt.Errorf("打开 OpenLaw 数据目录失败: %w", err)
	}
	return &OpenLawSource{ol: ol}, nil
}

// Name 插件唯一名。
func (s *OpenLawSource) Name() string { return "openlaw" }

// Close 释放底层数据目录。
func (s *OpenLawSource) Close() error { return s.ol.Close() }

// SearchLaw 关键词检索法条：结果携带版本与状态（LawHit 契约）。
// OpenLaw 断链（OL-1006）命中透传为 error 前先取数据：断链条文可展示但必须
// 由上层标注「无法溯源」（sdk 已在 warnings/trace 表达，这里保留 warnings）。
func (s *OpenLawSource) SearchLaw(ctx context.Context, query string, topK int) ([]LawHit, error) {
	res := s.ol.Search(query, topK)
	if res.Status == sdk.StatusNotFound {
		if len(res.Warnings) > 0 {
			return nil, fmt.Errorf("%s", res.Warnings[0])
		}
		return nil, fmt.Errorf("检索失败")
	}
	raw, _ := res.Data["hits"].([]map[string]any)
	hits := make([]LawHit, 0, len(raw))
	for _, h := range raw {
		hits = append(hits, lawHitFrom(h))
	}
	return hits, nil
}

// GetLawDetail 按条文标识取详情（lawID = OpenLaw article_id，稳定标识约定）。
func (s *OpenLawSource) GetLawDetail(ctx context.Context, lawID string) (LawHit, error) {
	res := s.ol.GetArticle(lawID)
	if res.Status == sdk.StatusNotFound {
		return LawHit{}, fmt.Errorf("条文不存在: %s", lawID)
	}
	hit := lawHitFrom(res.Data)
	if content, ok := res.Data["content"].(string); ok && content != "" {
		hit.Article = content // 详情给全文而非摘要
	}
	if res.Status == sdk.StatusBrokenChain {
		// OL-1006：数据照常返回，断链事实由 Trace/warnings 上浮到界面标注
		hit.Status = "UNVERIFIED"
	}
	return hit, nil
}

// lawHitFrom sdk 命中 map → LawHit。
func lawHitFrom(m map[string]any) LawHit {
	str := func(k string) string { v, _ := m[k].(string); return v }
	return LawHit{
		LawID:         str("article_id"),
		Title:         str("law_name"),
		Article:       str("snippet"),
		EffectiveDate: str("effective_date"),
		Status:        openlawStatus(str("expire_date")),
	}
}

// openlawStatus 状态映射：OpenLaw 版本区间 → LawHit.Status。
// 被后续版本接续（expire_date 非空）→ AMENDED；否则 EFFECTIVE。
// 绝不输出 REPEALED：机器不自动判定废止（OpenLaw 10 章纪律）——废止须
// confirmed 的 relation 证据，M4 未接 relation 查询，宁可少判。
func openlawStatus(expireDate string) string {
	if expireDate != "" {
		return "AMENDED"
	}
	return "EFFECTIVE"
}
