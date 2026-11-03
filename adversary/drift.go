// M6 立场漂移检测（开发文档 3.6）：每轮发言前重新注入本方立场，
// 用确定性极性词对比对本轮发言与历史立场——明显转向立即报警。
// 规则式而非模型式：漂移报警不能依赖另一个会漂移的模型。
package adversary

import (
	"fmt"
	"strings"
)

// polarityPairs 成对的立场极性词（词对间无子串包含，避免误伤）：
// 立场取一侧、发言取另一侧即判翻转。
var polarityPairs = [][2]string{
	{"支持", "反对"},
	{"认可", "否认"},
	{"合理", "过高"},
	{"应当调整", "无需调整"},
	{"有权", "无权"},
	{"应当支付", "无需支付"},
}

// LexicalDrift 确定性词汇漂移检测器。
type LexicalDrift struct{}

// Detect 比对本轮发言与本方立场：任一极性词对出现「立场在一侧、
// 发言在另一侧」即判漂移并说明翻转点。history 仅用于报警说明。
func (LexicalDrift) Detect(stance string, utterance string, history []string) (bool, string) {
	for _, p := range polarityPairs {
		pos, neg := p[0], p[1]
		var flipFrom, flipTo string
		switch {
		case strings.Contains(stance, pos) && !strings.Contains(stance, neg) && strings.Contains(utterance, neg):
			flipFrom, flipTo = pos, neg
		case strings.Contains(stance, neg) && !strings.Contains(stance, pos) && strings.Contains(utterance, pos):
			flipFrom, flipTo = neg, pos
		default:
			continue
		}
		msg := fmt.Sprintf("立场漂移报警：本方立场在「%s」侧，本轮发言出现「%s」侧表述", flipFrom, flipTo)
		if len(history) > 0 {
			msg += fmt.Sprintf("（历史立场 %d 轮，已重新注入核对）", len(history))
		}
		return true, msg
	}
	return false, ""
}
