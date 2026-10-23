// Package audit 实现日志审计模块（开发文档 4.8、7.3）：
// 记录谁在什么时间查了什么、输出了什么。日志以 JSONL 追加写，
// 每条记录携带前一条记录的哈希（哈希链），任何篡改都可被 Verify 检出。
package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Entry 一条审计记录。
type Entry struct {
	Timestamp time.Time `json:"timestamp"`
	Actor     string    `json:"actor"`  // 操作者（角色）
	Action    string    `json:"action"` // 动作：QUERY / EXPORT / REDACT / GATE_BLOCK / STATE_UPGRADE ...
	Target    string    `json:"target"` // 对象：案件/事实/文档 ID
	Detail    string    `json:"detail,omitempty"`
	PrevHash  string    `json:"prev_hash"`
	EntryHash string    `json:"entry_hash"`
}

// Logger 追加式审计日志。
type Logger struct {
	path     string
	prevHash string
}

// NewLogger 打开（或创建）指定路径的审计日志并定位链尾。
func NewLogger(path string) (*Logger, error) {
	l := &Logger{path: path, prevHash: "GENESIS"}
	if _, err := os.Stat(path); err == nil {
		ok, lastHash, err := Verify(path)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("审计日志链校验失败，疑似被篡改: %s", path)
		}
		l.prevHash = lastHash
	}
	return l, nil
}

func computeHash(e Entry) string {
	e.EntryHash = ""
	b, _ := json.Marshal(e)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Log 追加一条审计记录（不可篡改：只追加，不修改既有行）。
func (l *Logger) Log(actor, action, target, detail string) error {
	e := Entry{
		Timestamp: time.Now().UTC(),
		Actor:     actor,
		Action:    action,
		Target:    target,
		Detail:    detail,
		PrevHash:  l.prevHash,
	}
	e.EntryHash = computeHash(e)

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	l.prevHash = e.EntryHash
	return nil
}

// Verify 校验日志哈希链，返回 (是否完整, 链尾哈希, 错误)。
func Verify(path string) (bool, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, "", err
	}
	lines := splitLines(string(data))
	prev := "GENESIS"
	for i, line := range lines {
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return false, "", fmt.Errorf("第 %d 行解析失败: %w", i+1, err)
		}
		if e.PrevHash != prev {
			return false, "", fmt.Errorf("第 %d 行哈希链断裂", i+1)
		}
		if computeHash(e) != e.EntryHash {
			return false, "", fmt.Errorf("第 %d 行内容被篡改", i+1)
		}
		prev = e.EntryHash
	}
	return true, prev, nil
}

func splitLines(s string) []string {
	out := []string{}
	cur := ""
	for _, ch := range s {
		if ch == '\n' {
			out = append(out, cur)
			cur = ""
		} else {
			cur += string(ch)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
