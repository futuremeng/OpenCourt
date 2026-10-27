package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeMCPServer 模拟 MCP Streamable HTTP server（SSE 响应 + 会话头），
// 行为对齐实测 BrowserOS neo 0.0.80。
type fakeMCPServer struct {
	mu        sync.Mutex
	session   string
	calls     []string                    // 依次记录收到的 method
	toolArgs  map[string][]map[string]any // 按工具名记录收到的 arguments
	tools     []mcpTool
	toolText  map[string]string
	toolError map[string]bool
	sseMode   bool
}

func (f *fakeMCPServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var env mcpEnvelope
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.calls = append(f.calls, env.Method)
		f.mu.Unlock()

		writeResult := func(result any) {
			payload, _ := json.Marshal(mcpEnvelope{JSONRPC: "2.0", ID: env.ID, Result: mustRaw(result)})
			if f.sseMode {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Mcp-Session-Id", "sess-123")
				fmt.Fprintf(w, "data:\nid: 0\nretry: 3000\n\ndata: %s\n\n", payload)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", "sess-123")
			w.Write(payload)
		}

		switch env.Method {
		case "initialize":
			writeResult(map[string]any{
				"protocolVersion": mcpProtocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]string{"name": "fake-neo", "version": "0"},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeResult(map[string]any{"tools": f.tools})
		case "tools/call":
			params := env.Params.(map[string]any)
			name := params["name"].(string)
			if args, ok := params["arguments"].(map[string]any); ok {
				f.mu.Lock()
				if f.toolArgs == nil {
					f.toolArgs = map[string][]map[string]any{}
				}
				f.toolArgs[name] = append(f.toolArgs[name], args)
				f.mu.Unlock()
			}
			text := f.toolText[name]
			writeResult(map[string]any{
				"content": []map[string]string{{"type": "text", "text": text}},
				"isError": f.toolError[name],
			})
		default:
			http.Error(w, "unknown method", http.StatusBadRequest)
		}
	}
}

func mustRaw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func newFakeClient(t *testing.T, f *fakeMCPServer) *mcpClient {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	return newMCPClient(srv.URL + "/mcp")
}

func TestMCPClientHandshakeAndCalls(t *testing.T) {
	f := &fakeMCPServer{
		sseMode: true,
		tools: []mcpTool{
			{Name: "tabs"}, {Name: "read"}, {Name: "grep"},
		},
		toolText: map[string]string{"tabs": "opened page 42", "grep": "line1\nline2"},
	}
	c := newFakeClient(t, f)
	ctx := context.Background()

	if err := c.initialize(ctx, "test"); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if c.sessionID != "sess-123" {
		t.Fatalf("会话 ID 未记录: %q", c.sessionID)
	}
	tools, err := c.listTools(ctx)
	if err != nil {
		t.Fatalf("listTools: %v", err)
	}
	if len(tools) != 3 {
		t.Fatalf("tools = %d", len(tools))
	}
	res, err := c.callTool(ctx, "tabs", map[string]any{"action": "new"})
	if err != nil {
		t.Fatalf("callTool: %v", err)
	}
	if res.Text != "opened page 42" || res.IsError {
		t.Fatalf("callTool result: %+v", res)
	}
	f.mu.Lock()
	calls := strings.Join(f.calls, ",")
	f.mu.Unlock()
	if calls != "initialize,notifications/initialized,tools/list,tools/call" {
		t.Fatalf("调用序列异常: %s", calls)
	}
}

func TestMCPClientJSONMode(t *testing.T) {
	f := &fakeMCPServer{
		tools:    []mcpTool{{Name: "read"}},
		toolText: map[string]string{"read": "hello"},
	}
	c := newFakeClient(t, f)
	ctx := context.Background()
	if err := c.initialize(ctx, "test"); err != nil {
		t.Fatal(err)
	}
	res, err := c.callTool(ctx, "read", map[string]any{"page": 1})
	if err != nil || res.Text != "hello" {
		t.Fatalf("JSON 模式失败: %v %+v", err, res)
	}
}

func TestMCPClientToolError(t *testing.T) {
	f := &fakeMCPServer{
		tools:     []mcpTool{{Name: "read"}},
		toolText:  map[string]string{"read": "boom"},
		toolError: map[string]bool{"read": true},
	}
	c := newFakeClient(t, f)
	ctx := context.Background()
	if err := c.initialize(ctx, "test"); err != nil {
		t.Fatal(err)
	}
	res, err := c.callTool(ctx, "read", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("isError 应透传")
	}
}

func TestMCPClientConnectionRefused(t *testing.T) {
	c := newMCPClient("http://127.0.0.1:1/mcp")
	if err := c.initialize(context.Background(), "test"); err == nil {
		t.Fatal("连接失败应报错")
	}
}
