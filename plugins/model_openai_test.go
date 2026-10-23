package plugins

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIAdapterDisabledByDefault(t *testing.T) {
	a := NewOpenAIAdapter("http://127.0.0.1:1", "", "m", "e", false)
	if a.Enabled() {
		t.Fatal("默认必须禁用（默认离线安全）")
	}
	if _, err := a.Generate(context.Background(), "hi"); err == nil {
		t.Fatal("未启用时禁止发起调用")
	}
}

func TestOpenAIAdapterNoKeyMeansDisabled(t *testing.T) {
	a := NewOpenAIAdapter("http://127.0.0.1:1", "", "m", "e", true)
	if a.Enabled() {
		t.Fatal("缺少 API Key 时必须视为未启用")
	}
}

func TestOpenAIAdapterChat(t *testing.T) {
	var gotAuth, gotNoTrain string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("路径错误: %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotNoTrain = r.Header.Get("X-OpenCourt-No-Training")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"候选事实"}}]}`))
	}))
	defer srv.Close()

	a := NewOpenAIAdapter(srv.URL+"/v1", "sk-test", "gpt-x", "emb-x", true)
	out, err := a.Generate(context.Background(), "prompt")
	if err != nil {
		t.Fatal(err)
	}
	if out != "候选事实" {
		t.Fatalf("输出解析错误: %q", out)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("Authorization 头错误: %q", gotAuth)
	}
	if gotNoTrain != "true" {
		t.Errorf("必须强制附加禁止训练标识头，实际: %q", gotNoTrain)
	}
	if gotBody["model"] != "gpt-x" {
		t.Errorf("model 字段错误: %v", gotBody["model"])
	}
}

func TestOpenAIAdapterEmbeddings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/embeddings") {
			t.Errorf("路径错误: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer srv.Close()

	a := NewOpenAIAdapter(srv.URL, "k", "m", "emb", true)
	vec, err := a.Embed(context.Background(), "text")
	if err != nil {
		t.Fatal(err)
	}
	if len(vec) != 3 {
		t.Fatalf("向量长度错误: %v", vec)
	}
}

func TestOpenAIAdapterErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer srv.Close()

	a := NewOpenAIAdapter(srv.URL, "bad", "m", "e", true)
	if _, err := a.Generate(context.Background(), "x"); err == nil {
		t.Fatal("非 200 状态必须报错")
	}
}
