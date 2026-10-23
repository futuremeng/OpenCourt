package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrNotSupported 适配器不支持该能力。
var ErrNotSupported = errors.New("该模型适配器不支持此能力")

// OpenAIAdapter OpenAI 兼容接口适配器（/chat/completions 与 /embeddings）。
// 不假定具体服务商：base_url 可指向 OpenAI、任何云厂商兼容端点，
// 或本地 OpenAI 兼容服务（LM Studio、llama.cpp server、vLLM、Ollama /v1 等）。
//
// 安全约束（开发文档 7.2）：
//   - 默认禁用：未显式启用前，任何调用直接报错（默认离线安全）；
//   - API Key 只从环境变量读取，严禁写入配置文件；
//   - 每个请求强制附加「禁止用于模型训练」标识头。
type OpenAIAdapter struct {
	baseURL        string
	apiKey         string
	reasoningModel string
	embeddingModel string
	enabled        bool
	client         *http.Client
}

// NewOpenAIAdapter 创建适配器。baseURL 形如 https://api.openai.com/v1；
// apiKey 由调用方从环境变量注入（见 config 的 model.api_key_env）。
func NewOpenAIAdapter(baseURL, apiKey, reasoningModel, embeddingModel string, enabled bool) *OpenAIAdapter {
	return &OpenAIAdapter{
		baseURL:        strings.TrimRight(baseURL, "/"),
		apiKey:         apiKey,
		reasoningModel: reasoningModel,
		embeddingModel: embeddingModel,
		enabled:        enabled,
		client:         &http.Client{Timeout: 180 * time.Second},
	}
}

// Name 实现 ModelAdapter。
func (o *OpenAIAdapter) Name() string { return "openai-compatible" }

// Enabled 实现 ModelAdapter。
func (o *OpenAIAdapter) Enabled() bool { return o.enabled && o.apiKey != "" && o.baseURL != "" }

// ChatMessage OpenAI 兼容消息。
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Chat 以 messages 调用 /chat/completions，返回首个 choice 的文本。
func (o *OpenAIAdapter) Chat(ctx context.Context, messages []ChatMessage, temperature float64) (string, error) {
	if !o.Enabled() {
		return "", errors.New("模型适配器未启用：默认离线，需在配置中显式启用并通过环境变量注入 API Key")
	}
	payload := map[string]any{
		"model":       o.reasoningModel,
		"messages":    messages,
		"temperature": temperature,
		"stream":      false,
	}
	body, err := o.post(ctx, "/chat/completions", payload)
	if err != nil {
		return "", err
	}
	var resp struct {
		Choices []struct {
			Message ChatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("解析模型响应失败: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("模型未返回任何 choice")
	}
	return resp.Choices[0].Message.Content, nil
}

// Generate 实现 ModelAdapter：单轮生成候选文本（模型仅生成候选，无裁决权）。
func (o *OpenAIAdapter) Generate(ctx context.Context, prompt string) (string, error) {
	return o.Chat(ctx, []ChatMessage{{Role: "user", Content: prompt}}, 0.1)
}

// Embed 实现 ModelAdapter：调用 /embeddings 生成向量。
func (o *OpenAIAdapter) Embed(ctx context.Context, text string) ([]float64, error) {
	if !o.Enabled() {
		return nil, errors.New("模型适配器未启用：默认离线，需在配置中显式启用并通过环境变量注入 API Key")
	}
	if o.embeddingModel == "" {
		return nil, ErrNotSupported
	}
	payload := map[string]any{"model": o.embeddingModel, "input": text}
	body, err := o.post(ctx, "/embeddings", payload)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析 embedding 响应失败: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, errors.New("模型未返回 embedding")
	}
	return resp.Data[0].Embedding, nil
}

func (o *OpenAIAdapter) post(ctx context.Context, path string, payload map[string]any) ([]byte, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+path, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	// 开发文档 7.2：云端模型请求强制附加「禁止用于模型训练」标识。
	req.Header.Set("X-OpenCourt-No-Training", "true")
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("模型调用失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		snippet := string(body)
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		return nil, fmt.Errorf("模型服务返回状态 %d: %s", resp.StatusCode, snippet)
	}
	return body, nil
}
