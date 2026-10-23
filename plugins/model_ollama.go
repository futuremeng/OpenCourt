package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrNotSupported 适配器不支持该能力。
var ErrNotSupported = errors.New("该模型适配器不支持此能力")

// OllamaAdapter 本地 Ollama 适配骨架（默认 127.0.0.1:11434）。
// 默认禁用：仅当用户在配置中显式启用后才会发起（仅限本机的）请求，
// 符合“默认离线安全”原则。
type OllamaAdapter struct {
	baseURL        string
	enabled        bool
	reasoningModel string
	embeddingModel string
	client         *http.Client
}

// NewOllamaAdapter 创建适配器。baseURL 为空时使用默认本机地址。
func NewOllamaAdapter(baseURL string, enabled bool, reasoningModel, embeddingModel string) *OllamaAdapter {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11434"
	}
	return &OllamaAdapter{
		baseURL:        baseURL,
		enabled:        enabled,
		reasoningModel: reasoningModel,
		embeddingModel: embeddingModel,
		client:         &http.Client{Timeout: 120 * time.Second},
	}
}

// Name 实现 ModelAdapter。
func (o *OllamaAdapter) Name() string { return "ollama-local" }

// Enabled 实现 ModelAdapter。
func (o *OllamaAdapter) Enabled() bool { return o.enabled }

// Generate 实现 ModelAdapter。未启用时拒绝任何调用。
func (o *OllamaAdapter) Generate(ctx context.Context, prompt string) (string, error) {
	if !o.enabled {
		return "", errors.New("模型适配器未启用：默认离线，需在配置中显式启用")
	}
	payload := map[string]any{
		"model":   o.reasoningModel,
		"prompt":  prompt,
		"stream":  false,
		"options": map[string]any{"temperature": 0.1},
	}
	body, err := o.post(ctx, "/api/generate", payload)
	if err != nil {
		return "", err
	}
	var resp struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	return resp.Response, nil
}

// Embed 实现 ModelAdapter。
func (o *OllamaAdapter) Embed(ctx context.Context, text string) ([]float64, error) {
	if !o.enabled {
		return nil, errors.New("模型适配器未启用：默认离线，需在配置中显式启用")
	}
	if o.embeddingModel == "" {
		return nil, ErrNotSupported
	}
	payload := map[string]any{"model": o.embeddingModel, "prompt": text}
	body, err := o.post(ctx, "/api/embeddings", payload)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return resp.Embedding, nil
}

func (o *OllamaAdapter) post(ctx context.Context, path string, payload map[string]any) ([]byte, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+path, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("本地模型调用失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("本地模型返回状态 %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
