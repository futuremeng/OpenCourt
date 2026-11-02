package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// TestConcurrentLogKeepsChainIntact 并发审计：Wails 每次前端调用都在独立
// goroutine 中执行绑定方法，审计写入必须串行——否则不仅数据竞争，还会写出
// 两条相同 PrevHash 的记录，把哈希链自己写断（Verify 从此报「疑似篡改」）。
func TestConcurrentLogKeepsChainIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	l, err := NewLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	const goroutines, perG = 8, 25
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				if err := l.Log("court", "QUERY", "CASE-DEFAULT", fmt.Sprintf("g%d-%d", g, i)); err != nil {
					t.Errorf("审计写入失败: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	if ok, _, err := Verify(path); err != nil || !ok {
		t.Errorf("并发写入后哈希链必须完整: ok=%v err=%v", ok, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(splitLines(strings.TrimSpace(string(data)))); n != goroutines*perG {
		t.Errorf("应有 %d 条审计记录，实际 %d（并发丢写）", goroutines*perG, n)
	}
}
