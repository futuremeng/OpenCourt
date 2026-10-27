// 网络查阅管道（P1，设计见 docs/browseros-integration.md）：
// 浏览器插件（BrowserOS）抓取 → 强制脱敏 → 以「单源未补强」事实入库
// （Citation 携带 URL/抓取时间/快照哈希）→ 仅脱敏后文本对外返回。
// 原始页面内容（不可信外部数据）绝不回流模型；其哈希入审计供回查。
// 联网内容永远不能直接进入「已溯源确认」态——升级只能靠导入证据。
package neutral

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/redact"
	"github.com/futuremeng/OpenCourt/core/statemachine"
	"github.com/futuremeng/OpenCourt/plugins"
)

// ResearchResult 一次网络查阅的结果。
type ResearchResult struct {
	URL           string          `json:"url"`
	FetchedAt     time.Time       `json:"fetched_at"`
	ContentSHA    string          `json:"content_sha256"` // 原始快照（脱敏前）哈希
	Text          string          `json:"text"`           // 脱敏后正文——模型唯一安全入口
	Fact          domain.Fact     `json:"fact"`           // 状态由状态机判定（联网源至多单源未补强）
	RedactActions []redact.Action `json:"redact_actions"`
}

// RunResearch 执行网络查阅：抓取 → 脱敏 → 产候选事实（弱态）。
// names 为已知当事人姓名（一致替换用）。插件未启用时直接报错，
// 绝不降级到其他抓取方式。
func RunResearch(ctx context.Context, plugin plugins.BrowserPlugin, names []string, url string) (ResearchResult, error) {
	if plugin == nil {
		return ResearchResult{}, fmt.Errorf("浏览器辅助插件未配置：%s", plugins.BrowserOSGuidance)
	}
	page, err := plugin.Open(ctx, url)
	if err != nil {
		return ResearchResult{}, err
	}
	content, readErr := plugin.ReadPage(ctx, page.PageID)
	_ = plugin.ClosePage(ctx, page.PageID) // 尽力关闭，失败不影响结果
	if readErr != nil {
		return ResearchResult{}, readErr
	}

	// 强制脱敏：脱敏后文本是模型/UI 的唯一安全入口。
	red := redact.New(names)
	redRes := red.RedactText(content.Markdown)

	source := content.URL
	if source == "" {
		source = page.URL
	}
	snippet := excerpt(redRes.Text, 500)
	f := domain.Fact{
		FactID:      newFactID(),
		Description: excerpt(redRes.Text, 200),
		// 联网源坐标：URL + 抓取时间 + 快照哈希（审计与回查用）。
		Citation: &domain.Citation{
			SourceFile: fmt.Sprintf("web:%s@%s#%s", source,
				content.FetchedAt.UTC().Format(time.RFC3339), content.ContentSHA),
			PageNum: 1,
			Snippet: snippet,
		},
		Visibility: domain.VisBoth,
		UpdatedAt:  time.Now().UTC(),
	}
	// 初始态由状态机确定性判定：有可定位引用 → 单源未补强。
	f.State = statemachine.InitialState(f, false)

	return ResearchResult{
		URL:           source,
		FetchedAt:     content.FetchedAt,
		ContentSHA:    content.ContentSHA,
		Text:          redRes.Text,
		Fact:          f,
		RedactActions: redRes.Actions,
	}, nil
}

func excerpt(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(string(r[:maxRunes])) + "…"
}
