package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditChain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	l, err := NewLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := l.Log("court", "QUERY", "facts", "test"); err != nil {
			t.Fatal(err)
		}
	}
	ok, _, err := Verify(path)
	if err != nil || !ok {
		t.Fatal("完整日志链校验应通过")
	}
}

func TestTamperDetected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	l, _ := NewLogger(path)
	_ = l.Log("court", "QUERY", "facts", "first")
	_ = l.Log("court", "EXPORT", "report", "second")

	data, _ := os.ReadFile(path)
	tampered := strings.Replace(string(data), "QUERY", "HACKED", 1)
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	ok, _, err := Verify(path)
	if ok || err == nil {
		t.Fatal("篡改日志应被检出")
	}
}
