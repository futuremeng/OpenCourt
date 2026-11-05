package ingest

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// TesseractAdapter 本机 tesseract 二进制（subprocess，Apache-2.0，跨平台兜底）。
// 输出走 --tsv：自带词级置信度，页级置信度取词均值（忽略 conf=-1 的非文本行）。
type TesseractAdapter struct {
	// Binary tesseract 可执行文件（空 = PATH 中查找）。
	Binary string
	// Lang 语言包（默认 chi_sim+eng；模型 tessdata_fast 数 MB，Apache-2.0）。
	Lang string
}

func (t *TesseractAdapter) Name() string { return "tesseract" }

// Available 报告二进制是否可执行。
func (t *TesseractAdapter) Available() bool {
	bin := t.Binary
	if bin == "" {
		bin = "tesseract"
	}
	_, err := exec.LookPath(bin)
	return err == nil
}

// Recognize 单图 → 单页。失败原样上抛，绝不静默产出空文本。
func (t *TesseractAdapter) Recognize(ctx context.Context, path string) ([]OCRPage, error) {
	bin := t.Binary
	if bin == "" {
		bin = "tesseract"
	}
	lang := t.Lang
	if lang == "" {
		lang = "chi_sim+eng"
	}
	out, err := exec.CommandContext(ctx, bin, path, "stdout", "-l", lang, "--tsv").Output()
	if err != nil {
		return nil, fmt.Errorf("tesseract 识别失败: %w", err)
	}
	text, conf, ok := parseTesseractTSV(string(out))
	if !ok {
		return nil, nil // 无文本：由上层按「未产出文本」拒绝入库
	}
	return []OCRPage{{PageNum: 1, Text: text, Confidence: conf}}, nil
}

// parseTesseractTSV 解析 TSV 输出的词行（level=5）：conf 为 0–100，
// -1 表示非文本占位。返回页文本与均值置信（0–1）。
func parseTesseractTSV(out string) (text string, conf float64, ok bool) {
	var words []string
	var sum float64
	var n int
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		cols := strings.Split(line, "\t")
		if i == 0 || len(cols) < 12 {
			continue // 表头或残行
		}
		if cols[0] != "5" { // 只取词级行
			continue
		}
		c, err := strconv.ParseFloat(strings.TrimSpace(cols[10]), 64)
		if err != nil || c < 0 {
			continue
		}
		w := cols[11]
		if w == "" || w == " " {
			continue
		}
		words = append(words, w)
		sum += c
		n++
	}
	if n == 0 {
		return "", 0, false
	}
	return joinTSVWords(words), sum / float64(n) / 100.0, true
}

// joinTSVWords 拼接词序列：相邻两侧均为 ASCII 字母/数字时补空格，
// 否则直接相接——中文逐词不加空格（chi_sim 逐字产出），英文不粘连。
// verify.Locate 归一化会去空白，两种接法都不影响回原文定位。
func joinTSVWords(words []string) string {
	var sb strings.Builder
	for _, w := range words {
		if sb.Len() > 0 && isASCIIAlnum(lastRune(sb.String())) && isASCIIAlnum(firstRune(w)) {
			sb.WriteByte(' ')
		}
		sb.WriteString(w)
	}
	return sb.String()
}

func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}

func lastRune(s string) rune {
	var r rune
	for _, c := range s {
		r = c
	}
	return r
}

func isASCIIAlnum(r rune) bool {
	return r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}
