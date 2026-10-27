package neutral

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/plugins"
)

// stubBrowser 内存版 BrowserPlugin（管道测试用）。
type stubBrowser struct {
	enabled  bool
	markdown string
	closed   bool
	lastURL  string
}

func (s *stubBrowser) Name() string  { return "stub" }
func (s *stubBrowser) Enabled() bool { return s.enabled }
func (s *stubBrowser) Open(ctx context.Context, url string) (plugins.BrowserPage, error) {
	if !s.enabled {
		return plugins.BrowserPage{}, fmt.Errorf("插件未启用")
	}
	s.lastURL = url
	return plugins.BrowserPage{PageID: 1, URL: url}, nil
}
func (s *stubBrowser) ReadPage(ctx context.Context, pageID int) (plugins.BrowserPageContent, error) {
	sum := sha256.Sum256([]byte(s.markdown))
	return plugins.BrowserPageContent{
		URL:        s.lastURL,
		Markdown:   s.markdown,
		ContentSHA: hex.EncodeToString(sum[:]),
		FetchedAt:  time.Now().UTC(),
	}, nil
}
func (s *stubBrowser) SearchPage(ctx context.Context, pageID int, q string) ([]string, error) {
	return nil, nil
}
func (s *stubBrowser) ClosePage(ctx context.Context, pageID int) error {
	s.closed = true
	return nil
}

func (s *stubBrowser) Snapshot(ctx context.Context, pageID int) (string, error) {
	return "", fmt.Errorf("stub 不支持")
}
func (s *stubBrowser) Evaluate(ctx context.Context, pageID int, code string) (string, error) {
	return "", fmt.Errorf("stub 不支持")
}
func (s *stubBrowser) ProposeFill(ctx context.Context, pageID int, fields []plugins.FormFieldSpec) ([]plugins.ActionCandidate, error) {
	return nil, fmt.Errorf("stub 不支持")
}
func (s *stubBrowser) ProposeClick(ctx context.Context, pageID int, ref, label string) (plugins.ActionCandidate, error) {
	return plugins.ActionCandidate{}, fmt.Errorf("stub 不支持")
}
func (s *stubBrowser) ExecuteAction(ctx context.Context, actionID string) (plugins.ActionReceipt, error) {
	return plugins.ActionReceipt{}, fmt.Errorf("stub 不支持")
}
func (s *stubBrowser) HandToHuman(ctx context.Context, pageID int, kind, reason, resumeHint string) (plugins.HumanHelpStatus, error) {
	return plugins.HumanHelpStatus{}, fmt.Errorf("stub 不支持")
}
func (s *stubBrowser) AwaitHuman(ctx context.Context) (plugins.HumanHelpStatus, error) {
	return plugins.HumanHelpStatus{}, fmt.Errorf("stub 不支持")
}

func TestRunResearchRedactsBeforeReturn(t *testing.T) {
	raw := "当事人张三（身份证号 110101199003078888）应在北京市海淀区人民法院立案。"
	sb := &stubBrowser{enabled: true, markdown: raw}
	res, err := RunResearch(context.Background(), sb, []string{"张三"}, "https://court.example.gov/filing")
	if err != nil {
		t.Fatal(err)
	}
	// 合规核心：返回文本（模型唯一入口）不得含未脱敏 PII。
	for _, leak := range []string{"张三", "110101199003078888"} {
		if strings.Contains(res.Text, leak) {
			t.Fatalf("脱敏后仍含原文敏感内容: %s", leak)
		}
	}
	if !strings.Contains(res.Text, "PERSON_001") {
		t.Fatalf("姓名未一致替换: %q", res.Text)
	}
	if len(res.RedactActions) == 0 {
		t.Fatal("脱敏动作未记录")
	}
}

func TestRunResearchFactIsWeakStateWithWebCitation(t *testing.T) {
	sb := &stubBrowser{enabled: true, markdown: "诉讼费：财产案件根据诉讼请求金额按比例分段累计交纳。"}
	res, err := RunResearch(context.Background(), sb, nil, "https://court.example.gov/fees")
	if err != nil {
		t.Fatal(err)
	}
	f := res.Fact
	// 联网源至多「单源未补强」，绝不可直接进已溯源确认/无依据之外的强态。
	if f.State != domain.StateSingleSource {
		t.Fatalf("联网事实状态 = %s，应为单源未补强", f.State)
	}
	if f.Citation == nil || !f.Citation.Valid() {
		t.Fatalf("联网事实必须携带可定位引用: %+v", f.Citation)
	}
	if !strings.HasPrefix(f.Citation.SourceFile, "web:https://court.example.gov/fees@") {
		t.Fatalf("引用应含 URL+抓取时间: %q", f.Citation.SourceFile)
	}
	if !strings.Contains(f.Citation.SourceFile, "#"+res.ContentSHA) {
		t.Fatal("引用应含快照哈希")
	}
	if !sb.closed {
		t.Fatal("页面应被关闭")
	}
}

func TestRunResearchDisabledPluginNoFallback(t *testing.T) {
	sb := &stubBrowser{enabled: false}
	_, err := RunResearch(context.Background(), sb, nil, "https://example.com")
	if err == nil {
		t.Fatal("插件未启用必须报错，绝不降级抓取")
	}
	_, err = RunResearch(context.Background(), nil, nil, "https://example.com")
	if err == nil {
		t.Fatal("插件未配置必须报错")
	}
}
