// 门禁对抗集：必拦样本（倾向性/预测性/裁判性表述）与必放样本（中立陈述、
// D6 合法分布标注、法条原文）。误拦率是产品可用性指标——词表误伤会把正常
// 法律文书判成违规；漏拦率是合规红线指标。两者都必须是数字，不是印象。
package eval

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/futuremeng/OpenCourt/core/gate"
)

//go:embed samples/gate_adversarial.jsonl
var gateSamplesRaw string

// GateSample 一条对抗样本。Expect = "block"（必须被拦截）或 "pass"（必须放行）。
type GateSample struct {
	Text   string `json:"text"`
	Expect string `json:"expect"`
	Note   string `json:"note,omitempty"`
}

// LoadGateSamples 解析内置对抗集。
func LoadGateSamples() ([]GateSample, error) {
	out := []GateSample{}
	dec := json.NewDecoder(strings.NewReader(gateSamplesRaw))
	line := 0
	for dec.More() {
		line++
		var s GateSample
		if err := dec.Decode(&s); err != nil {
			return nil, fmt.Errorf("对抗集第 %d 行解析失败: %w", line, err)
		}
		if s.Expect != "block" && s.Expect != "pass" {
			return nil, fmt.Errorf("对抗集第 %d 行：expect 必须是 block/pass，实际 %q", line, s.Expect)
		}
		if strings.TrimSpace(s.Text) == "" {
			return nil, fmt.Errorf("对抗集第 %d 行：text 为空", line)
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("对抗集为空")
	}
	return out, nil
}

// GateFailure 一条不符合预期的样本。
type GateFailure struct {
	Sample   GateSample `json:"sample"`
	Rule     string     `json:"rule,omitempty"`   // 误拦时命中的规则
	Phrase   string     `json:"phrase,omitempty"` // 误拦时命中的表述
	Expected string     `json:"expected"`         // block / pass
}

// GateEvalResult 对抗集运行结果。
type GateEvalResult struct {
	Total      int           `json:"total"`
	MustBlock  int           `json:"must_block"`
	MustPass   int           `json:"must_pass"`
	Missed     []GateFailure `json:"missed"`      // 该拦未拦（漏拦）
	FalseBlock []GateFailure `json:"false_block"` // 该放未放（误拦）
}

// RunGateAdversarial 对 core/gate.CheckGuidance 跑对抗集。
// 纯确定性、零外部依赖——任何时候都能出数。
func RunGateAdversarial(samples []GateSample) GateEvalResult {
	res := GateEvalResult{Missed: []GateFailure{}, FalseBlock: []GateFailure{}}
	for _, s := range samples {
		res.Total++
		v := gate.CheckGuidance(s.Text)
		blocked := len(v) > 0
		switch {
		case s.Expect == "block" && !blocked:
			res.MustBlock++
			res.Missed = append(res.Missed, GateFailure{Sample: s, Expected: "block"})
		case s.Expect == "block":
			res.MustBlock++
		case s.Expect == "pass" && blocked:
			res.MustPass++
			res.FalseBlock = append(res.FalseBlock, GateFailure{
				Sample: s, Expected: "pass", Rule: v[0].Rule, Phrase: v[0].Phrase,
			})
		default:
			res.MustPass++
		}
	}
	return res
}

// FalseBlockRate 误拦率 = 误拦数 / 必放样本数。目标 0。
func (r GateEvalResult) FalseBlockRate() float64 {
	if r.MustPass == 0 {
		return 0
	}
	return float64(len(r.FalseBlock)) / float64(r.MustPass)
}

// MissRate 漏拦率 = 漏拦数 / 必拦样本数。目标 0（合规红线）。
func (r GateEvalResult) MissRate() float64 {
	if r.MustBlock == 0 {
		return 0
	}
	return float64(len(r.Missed)) / float64(r.MustBlock)
}
