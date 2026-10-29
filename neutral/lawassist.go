// lawassist.go 法条辅助管线（plan-v0.2 4.1）：检索 + 文书引用确定性抽取 +
// 经 OpenLaw 的核验。三纪律：
//  1. 检索/核验全部经 plugins 适配层走 OpenLaw sdk（禁直连 DB；框架不内置
//     任何法条数据，模型不得生成法条）；
//  2. 引用抽取是确定性正则（无模型参与，D4）；
//  3. 核验结果原样透传 OpenLaw 错误码与 pending 状态——OpenCourt 不自行
//     判定效力、不自动确认。
package neutral

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/plugins"
)

// SearchLaw 法条关键词检索。法条源未配置时显式报错并给出原因——
// 绝不静默返回空列表冒充「语料里没有」。
func SearchLaw(ctx context.Context, src plugins.LawSourcePlugin, query string, topK int) ([]plugins.LawHit, error) {
	if src == nil {
		return nil, fmt.Errorf("法条源未配置（profile: openlaw.data_dir）：无法检索法条；框架不内置任何法条数据")
	}
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("检索词不能为空")
	}
	if topK <= 0 || topK > 50 {
		topK = 10
	}
	return src.SearchLaw(ctx, query, topK)
}

// LawCitationDraft 从文书文本确定性抽取的法条引用草稿（核验前状态）。
type LawCitationDraft struct {
	LawName   string `json:"law_name"`
	ArticleNo string `json:"article_no"` // 「第X条」形态（与 OpenLaw article_no 一致）
	Quote     string `json:"quote,omitempty"`
}

// citationRe 「《XX法》第X条（之Y）」——中文数字与阿拉伯数字均接受。
var citationRe = regexp.MustCompile(`《([^》]{2,60})》\s*第([0-9〇零一二三四五六七八九十百千两]+)条(?:之([0-9一二三四五六七八九十]+))?`)

// ExtractLawCitations 抽取文书中的法条引用（同法同条去重，保持出现顺序）。
// 抽取是纯确定性文本处理：不猜简称对应哪部法、不补全条号——核不上由
// OpenLaw 返回 OL-1001/OL-1003，界面原样展示。
func ExtractLawCitations(text string) []LawCitationDraft {
	matches := citationRe.FindAllStringSubmatch(text, -1)
	out := []LawCitationDraft{}
	seen := map[string]bool{}
	for _, m := range matches {
		name := strings.TrimSpace(m[1])
		no := "第" + strings.TrimSpace(m[2]) + "条"
		if m[3] != "" {
			no += "之" + strings.TrimSpace(m[3])
		}
		key := name + "\x00" + no
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, LawCitationDraft{LawName: name, ArticleNo: no})
	}
	return out
}

// LawVerifier 引用核验窄接口（OpenLawSource 实现；测试用 fake 实现）。
type LawVerifier interface {
	CheckCitation(ctx context.Context, lawName, articleNo, quote, atDate string) (plugins.CitationVerdict, error)
	FindRelation(ctx context.Context, articleID string) ([]plugins.Relation, error)
}

// VerifyCitations 逐条交 OpenLaw CheckCitation，产出引用留痕（law_refs）。
// VerifyResult 透传 OK / OL-1001..OL-1006；断链（OL-1006）数据照常返回，
// 界面必须标注「无法溯源」。pending 确认状态原样上浮（不自动确认）。
func VerifyCitations(ctx context.Context, v LawVerifier, atDate string, drafts []LawCitationDraft) ([]domain.LawRef, error) {
	if v == nil {
		return nil, fmt.Errorf("法条源未配置：引用核验不可用")
	}
	refs := make([]domain.LawRef, 0, len(drafts))
	for _, d := range drafts {
		vd, err := v.CheckCitation(ctx, d.LawName, d.ArticleNo, d.Quote, atDate)
		if err != nil {
			return nil, fmt.Errorf("引用核验调用失败（%s %s）: %w", d.LawName, d.ArticleNo, err)
		}
		refs = append(refs, domain.LawRef{
			ArticleID:     vd.ArticleID,
			LawID:         vd.LawID,
			LawName:       d.LawName,
			ArticleNo:     d.ArticleNo,
			Quote:         d.Quote,
			Status:        vd.Status,
			ConfirmStatus: vd.ConfirmStatus,
			VerifyResult:  vd.VerifyResult,
			AtDate:        atDate,
		})
	}
	return refs, nil
}
