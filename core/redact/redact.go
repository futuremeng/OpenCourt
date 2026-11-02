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
	// 手机号（11 位）：卷宗与网页正文里最常见的直接标识符之一，
	// 15–19 位的卡号规则覆盖不到，必须单列。
	rePhone = regexp.MustCompile(`\b1[3-9]\d{9}\b`)
	// 统一社会信用代码（18 位，字符集排除 I/O/Z/S/V/U 易混字母）。
	reUSCC = regexp.MustCompile(`\b[0-9A-HJ-NP-RTUWXY]{2}\d{6}[0-9A-HJ-NP-RTUWXY]{10}\b`)
	// 邮箱：保留域名（机构归属常是必要信息），本地部分一律抹掉。
	reEmail = regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@([A-Za-z0-9.\-]+\.[A-Za-z]{2,})\b`)
	// 地址泛化：截取到首个 市/区/县 为止。
	reAddress = regexp.MustCompile(`([\p{Han}]{2,12}?(?:市|区|县))[\p{Han}0-9a-zA-Z\-—_（）()]{0,40}`)
	// 门牌级地址（补漏）：不含 市/区/县 前缀的「××路88号院3号楼」「××村7组」
	// 形态——reAddress 匹配不到，此前会整段漏网。保留到路/街/村级，其后全抹。
	// 第二组字符集刻意只含门牌/楼栋用字与数字，不含「路/线/和/平」等，
	// 避免把「路上行人」「村里说法」这类普通词吞掉。
	reAddressDetail = regexp.MustCompile(`([\p{Han}]{1,12}?(?:大道|路|街|道|巷|里|村|屯|寨))([0-9A-Za-z\-—（）()号楼单元室栋幢院组层弄排期甲乙丙丁东南西北]{1,30})`)
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

	// 2. 身份证号掩码（不可逆）：保留前 6 后 4。
	//    必须先于信用代码——18 位纯数字身份证同样落进信用代码的字符集，
	//    先掩码可彻底排除互相误伤。
	res.Text = reIDCard.ReplaceAllStringFunc(res.Text, func(m string) string {
		res.Actions = append(res.Actions, Action{
			RuleID: "IDCARD_MASK", OriginalHash: hash(m),
			Strategy: "mask", Restorable: false, Timestamp: now,
		})
		return m[:6] + "****" + m[len(m)-4:]
	})

	// 3. 统一社会信用代码掩码（不可逆）：保留前 2 后 4。
	//    含字母才算信用代码：纯 18 位数字串（身份证残留、批量编号）一律不动。
	res.Text = reUSCC.ReplaceAllStringFunc(res.Text, func(m string) string {
		if !strings.ContainsFunc(m, func(r rune) bool { return r >= 'A' && r <= 'Z' }) {
			return m
		}
		res.Actions = append(res.Actions, Action{
			RuleID: "USCC_MASK", OriginalHash: hash(m),
			Strategy: "mask", Restorable: false, Timestamp: now,
		})
		return m[:2] + strings.Repeat("*", len(m)-6) + m[len(m)-4:]
	})

	// 4. 手机号掩码（不可逆）：保留前 3 后 4
	res.Text = rePhone.ReplaceAllStringFunc(res.Text, func(m string) string {
		res.Actions = append(res.Actions, Action{
			RuleID: "PHONE_MASK", OriginalHash: hash(m),
			Strategy: "mask", Restorable: false, Timestamp: now,
		})
		return m[:3] + "****" + m[len(m)-4:]
	})

	// 5. 银行账号掩码（不可逆）：保留前 4 后 4
	res.Text = reBankCard.ReplaceAllStringFunc(res.Text, func(m string) string {
		res.Actions = append(res.Actions, Action{
			RuleID: "BANK_MASK", OriginalHash: hash(m),
			Strategy: "mask", Restorable: false, Timestamp: now,
		})
		return m[:4] + "****" + m[len(m)-4:]
	})

	// 6. 邮箱本地部分抹除（不可逆）：保留域名
	res.Text = reEmail.ReplaceAllStringFunc(res.Text, func(m string) string {
		g := reEmail.FindStringSubmatch(m)
		res.Actions = append(res.Actions, Action{
			RuleID: "EMAIL_MASK", OriginalHash: hash(m),
			Strategy: "mask", Restorable: false, Timestamp: now,
		})
		return "***@" + g[1]
	})

	// 7. 地址泛化（不可逆）：保留到市/区/县
	res.Text = reAddress.ReplaceAllStringFunc(res.Text, func(m string) string {
		g := reAddress.FindStringSubmatch(m)
		res.Actions = append(res.Actions, Action{
			RuleID: "ADDRESS_GENERALIZE", OriginalHash: hash(m),
			Strategy: "generalize", Restorable: false, Timestamp: now,
		})
		return g[1]
	})

	// 8. 门牌级地址（不可逆）：无 市/区/县 前缀的「××路88号院3号楼」形态，
	//    保留到路/街/村级，门牌与楼栋单元一律抹除。
	res.Text = reAddressDetail.ReplaceAllStringFunc(res.Text, func(m string) string {
		g := reAddressDetail.FindStringSubmatch(m)
		res.Actions = append(res.Actions, Action{
			RuleID: "ADDRESS_DETAIL_GENERALIZE", OriginalHash: hash(m),
			Strategy: "generalize", Restorable: false, Timestamp: now,
		})
		return g[1]
	})

	// 9. 案号保留（不脱敏），仅记录审计。
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
