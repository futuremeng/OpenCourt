// 阅卷 Agent（开发文档 4.8、11.2）：
// 模型只负责生成候选事实（描述 + 原文片段 + 页码），
// 状态由核验器回原文定位 + 状态机初始态规则确定性仲裁——模型无裁决权。
// 定位失败的候选一律拒绝入库，仅记审计。
package neutral

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/extract"
	"github.com/futuremeng/OpenCourt/core/statemachine"
	"github.com/futuremeng/OpenCourt/core/verify"
	"github.com/futuremeng/OpenCourt/plugins"
)

// CandidateFact 模型提交的候选事实（无状态字段——状态由代码判定）。
type CandidateFact struct {
	Description    string `json:"description"`
	Snippet        string `json:"snippet"`
	PageNum        int    `json:"page_num"`
	FromOpponent   bool   `json:"from_opponent"`
	RelatedElement string `json:"related_element,omitempty"`
}

// ReaderReport 阅卷结果。
type ReaderReport struct {
	DocID     string            `json:"doc_id"`
	ModelName string            `json:"model_name"`
	Accepted  []domain.Fact     `json:"accepted"` // 已入库事实（状态为核验器判定）
	Rejected  []CandidateFact   `json:"rejected"` // 回原文失败被拒绝的候选（仅审计）
	Elements  []extract.Element `json:"elements"` // 确定性时间/金额要素（均已定位）
}

// readerPrompt 构造阅卷提示：要求模型输出严格 JSON，片段必须逐字来自原文。
// 【V3，plan-v0.2】要件名是封闭候选集：有要件库时模型只能从要件库清单中
// 原样选择或留空；无要件库时禁止输出 related_element 字段（后置校验兜底）。
func readerPrompt(doc *verify.DocumentText, elementNames []string) string {
	var sb strings.Builder
	sb.WriteString(`你是法律文书阅卷助手。阅读以下带页码标记的文档，提取候选事实。

严格规则：
1. snippet 必须是原文中逐字连续出现的片段，禁止改写、概括或拼接；
2. page_num 必须是片段实际所在页码（对应页码标记）；
3. 无法在原文找到依据的内容一律不得输出；
4. from_opponent: 该事实若来自对方主张则 true；
`)
	if len(elementNames) > 0 {
		sb.WriteString("5. related_element 只能从以下要件清单中原样选择（不相关则留空），清单之外一律留空：\n   " +
			strings.Join(elementNames, "、") + "\n")
	} else {
		sb.WriteString("5. 不得输出 related_element 字段（一律留空）；\n")
	}
	sb.WriteString(`6. 只输出 JSON 数组，不要输出其他文字。

输出格式：
[{"description":"...","snippet":"...","page_num":1,"from_opponent":false,"related_element":"..."}]

文档内容：
`)
	for page := 1; page <= maxPage(doc); page++ {
		fmt.Fprintf(&sb, "\n=== 第 %d 页 ===\n%s\n", page, doc.Pages[page])
	}
	return sb.String()
}

func maxPage(doc *verify.DocumentText) int {
	m := 0
	for p := range doc.Pages {
		if p > m {
			m = p
		}
	}
	return m
}

// ParseCandidates 从模型输出中解析候选事实（容忍代码围栏与前后杂文）。
func ParseCandidates(out string) ([]CandidateFact, error) {
	s := strings.TrimSpace(out)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("模型输出中未找到 JSON 数组")
	}
	var cands []CandidateFact
	if err := json.Unmarshal([]byte(s[start:end+1]), &cands); err != nil {
		return nil, fmt.Errorf("解析候选事实失败: %w", err)
	}
	return cands, nil
}

// RunReader 执行阅卷：调用模型产候选 → 逐条回原文定位核验 →
// 定位成功者按状态机初始态规则入库候选态；定位失败者拒绝。
// 确定性要素（时间/金额）由正则抽取，不经模型。
// elementNames 是要件库封闭候选集（V3）：模型给出的要件名不在集内一律
// 置空（不匹配不入库）；无要件库传 nil——要件字段一律置空。
func RunReader(ctx context.Context, docID string, doc *verify.DocumentText, model plugins.ModelAdapter, elementNames []string) (ReaderReport, error) {
	if !model.Enabled() {
		return ReaderReport{}, fmt.Errorf("模型适配器 %s 未启用：默认离线，阅卷需要先在配置中显式启用模型", model.Name())
	}
	out, err := model.Generate(ctx, readerPrompt(doc, elementNames))
	if err != nil {
		return ReaderReport{}, err
	}
	cands, err := ParseCandidates(out)
	if err != nil {
		return ReaderReport{}, err
	}

	// V3 后置校验：要件名不信任模型输出——不在封闭集内的一律置空。
	allowed := make(map[string]bool, len(elementNames))
	for _, n := range elementNames {
		allowed[n] = true
	}

	docs := map[string]*verify.DocumentText{doc.File: doc}
	report := ReaderReport{
		DocID:     docID,
		ModelName: model.Name(),
		Accepted:  []domain.Fact{},
		Rejected:  []CandidateFact{},
	}
	for _, c := range cands {
		if strings.TrimSpace(c.Description) == "" {
			continue
		}
		if !allowed[c.RelatedElement] {
			c.RelatedElement = ""
		}
		f := domain.Fact{
			FactID:         newFactID(),
			Description:    c.Description,
			RelatedElement: c.RelatedElement,
			Visibility:     domain.VisBoth,
			UpdatedAt:      time.Now().UTC(),
		}
		if c.Snippet != "" && c.PageNum >= 1 {
			f.Citation = &domain.Citation{SourceFile: doc.File, PageNum: c.PageNum, Snippet: c.Snippet}
		}
		// 初始态由状态机规则判定；核验器不看结论，只回原文：
		// 定位失败直接打回 UNSUPPORTED → 拒绝入库。
		f.State = statemachine.InitialState(f, c.FromOpponent)
		f = VerifyFact(f, docs)
		if f.State == domain.StateUnsupported {
			report.Rejected = append(report.Rejected, c)
			continue
		}
		report.Accepted = append(report.Accepted, f)
	}

	// 确定性要素抽取：时间/金额走正则，均带原文坐标。
	for page, text := range doc.Pages {
		report.Elements = append(report.Elements, extract.ExtractTimeAndAmount(doc.File, page, text)...)
	}
	return report, nil
}

func newFactID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("FACT-%d", time.Now().UnixNano())
	}
	return "FACT-" + hex.EncodeToString(b)
}
