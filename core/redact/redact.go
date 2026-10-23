// Package redact 实现入库前强制脱敏（开发文档 4.2、7.3）。
// 支持一致替换（姓名→PERSON_001，附本地还原表）与不可逆脱敏（证件号/账号掩码、
// 地址泛化）。案号按文档要求保留。所有脱敏操作交由调用方写入 core/audit。
package redact

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Action 一次脱敏操作记录（审计用，含原始哈希、规则ID、时间戳）。
type Action struct {
	RuleID       string    `json:"rule_id"`
	OriginalHash string    `json:"original_hash"`
	Strategy     string    `json:"strategy"` // replace / mask / generalize / keep
	Restorable   bool      `json:"restorable"`
	Timestamp    time.Time `json:"timestamp"`
}

// Result 脱敏结果。
type Result struct {
	Text    string   `json:"text"`
	Actions []Action `json:"actions"`
}

// RestoreEntry 还原表条目（仅本地保存，授权用户在特定场景下还原）。
type RestoreEntry struct {
	Token    string `json:"token"`
	Original string `json:"original"`
}

var (
	reIDCard   = regexp.MustCompile(`\b\d{17}[\dXx]\b|\b\d{15}\b`)
	reBankCard = regexp.MustCompile(`\b\d{15,19}\b`)
	// 地址泛化：截取到首个 市/区/县 为止。
	reAddress = regexp.MustCompile(`([\p{Han}]{2,12}?(?:市|区|县))[\p{Han}0-9a-zA-Z\-—_（）()]{0,40}`)
)

// Redactor 脱敏器：names 为本方已知当事人姓名列表，做一致替换。
type Redactor struct {
	names   []string
	restore map[string]string // token -> original
	counter int
}

// New 创建脱敏器。names 中的姓名将按出现顺序替换为 PERSON_001、PERSON_002 …
func New(names []string) *Redactor {
	r := &Redactor{names: names, restore: map[string]string{}}
	for _, n := range names {
		r.counter++
		token := fmt.Sprintf("PERSON_%03d", r.counter)
		r.restore[token] = n
	}
	return r
}

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// RedactText 对文本执行强制脱敏。
func (r *Redactor) RedactText(text string) Result {
	res := Result{Text: text}
	now := time.Now()

	// 1. 姓名一致替换（可逆，依赖本地还原表）
	for _, name := range r.names {
		if name == "" || !strings.Contains(res.Text, name) {
			continue
		}
		token := r.tokenFor(name)
		res.Text = strings.ReplaceAll(res.Text, name, token)
		res.Actions = append(res.Actions, Action{
			RuleID: "NAME_REPLACE", OriginalHash: hash(name),
			Strategy: "replace", Restorable: true, Timestamp: now,
		})
	}

	// 2. 身份证号掩码（不可逆）：保留前 6 后 4
	res.Text = reIDCard.ReplaceAllStringFunc(res.Text, func(m string) string {
		res.Actions = append(res.Actions, Action{
			RuleID: "IDCARD_MASK", OriginalHash: hash(m),
			Strategy: "mask", Restorable: false, Timestamp: now,
		})
		return m[:6] + "****" + m[len(m)-4:]
	})

	// 3. 银行账号掩码（不可逆）：保留前 4 后 4
	res.Text = reBankCard.ReplaceAllStringFunc(res.Text, func(m string) string {
		res.Actions = append(res.Actions, Action{
			RuleID: "BANK_MASK", OriginalHash: hash(m),
			Strategy: "mask", Restorable: false, Timestamp: now,
		})
		return m[:4] + "****" + m[len(m)-4:]
	})

	// 4. 地址泛化（不可逆）：保留到市/区/县
	res.Text = reAddress.ReplaceAllStringFunc(res.Text, func(m string) string {
		g := reAddress.FindStringSubmatch(m)
		res.Actions = append(res.Actions, Action{
			RuleID: "ADDRESS_GENERALIZE", OriginalHash: hash(m),
			Strategy: "generalize", Restorable: false, Timestamp: now,
		})
		return g[1]
	})

	// 5. 案号保留（不脱敏），仅记录审计。
	if strings.Contains(res.Text, "民初") || strings.Contains(res.Text, "民终") {
		res.Actions = append(res.Actions, Action{
			RuleID: "CASE_NO_KEEP", OriginalHash: "",
			Strategy: "keep", Restorable: false, Timestamp: now,
		})
	}

	return res
}

func (r *Redactor) tokenFor(name string) string {
	for token, orig := range r.restore {
		if orig == name {
			return token
		}
	}
	r.counter++
	token := fmt.Sprintf("PERSON_%03d", r.counter)
	r.restore[token] = name
	return token
}

// RestoreTable 导出本地还原表（仅授权场景使用）。
func (r *Redactor) RestoreTable() []RestoreEntry {
	entries := make([]RestoreEntry, 0, len(r.restore))
	for token, orig := range r.restore {
		entries = append(entries, RestoreEntry{Token: token, Original: orig})
	}
	return entries
}

// Restore 按还原表还原 token。无对应条目时原样返回。
func (r *Redactor) Restore(token string) string {
	if orig, ok := r.restore[token]; ok {
		return orig
	}
	return token
}
