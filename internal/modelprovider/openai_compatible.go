package modelprovider

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	Endpoint  string
	Model     string
	APIKey    string
	Provider  string
	Client    *http.Client
	AllowHTTP bool
	Timeout   time.Duration
}

type Adapter struct {
	endpoint *url.URL
	model    string
	apiKey   string
	provider string
	client   *http.Client
}

type Capability struct {
	Provider         string   `json:"provider"`
	Model            string   `json:"model"`
	Endpoint         string   `json:"endpoint"`
	StructuredOutput bool     `json:"structured_output"`
	Streaming        bool     `json:"streaming"`
	ContextWindow    int      `json:"context_window,omitempty"`
	Capabilities     []string `json:"capabilities"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type CompletionRequest struct {
	Messages       []Message      `json:"messages"`
	Temperature    *float64       `json:"temperature,omitempty"`
	MaxTokens      int            `json:"max_tokens,omitempty"`
	ResponseSchema map[string]any `json:"response_schema,omitempty"`
	RequestID      string         `json:"request_id,omitempty"`
}

type CompletionResult struct {
	Provider     string          `json:"provider"`
	Model        string          `json:"model"`
	Content      string          `json:"content"`
	Structured   json.RawMessage `json:"structured,omitempty"`
	InputTokens  int64           `json:"input_tokens"`
	OutputTokens int64           `json:"output_tokens"`
	TotalTokens  int64           `json:"total_tokens"`
	RequestID    string          `json:"request_id,omitempty"`
	ReceivedAt   time.Time       `json:"received_at"`
}

func NewVLLM(config Config) (*Adapter, error) {
	config.Provider = "vllm"
	return New(config)
}

func NewSGLang(config Config) (*Adapter, error) {
	config.Provider = "sglang"
	return New(config)
}

func New(config Config) (*Adapter, error) {
	endpoint, err := url.Parse(strings.TrimRight(strings.TrimSpace(config.Endpoint), "/"))
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "https" && !(config.AllowHTTP && endpoint.Scheme == "http")) {
		return nil, errors.New("模型 Provider Endpoint 必须是 HTTPS URL")
	}
	if strings.TrimSpace(config.Model) == "" || strings.TrimSpace(config.Provider) == "" {
		return nil, errors.New("模型 Provider 必须声明 provider 和 model")
	}
	client := config.Client
	if client == nil {
		timeout := config.Timeout
		if timeout <= 0 {
			timeout = 45 * time.Second
		}
		client = &http.Client{Timeout: timeout, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}} // #nosec G402 -- explicit minimum TLS.
	}
	return &Adapter{endpoint: endpoint, model: strings.TrimSpace(config.Model), apiKey: strings.TrimSpace(config.APIKey), provider: strings.ToLower(strings.TrimSpace(config.Provider)), client: client}, nil
}

func (a *Adapter) Detect(ctx context.Context) (Capability, error) {
	if a == nil || a.endpoint == nil {
		return Capability{}, errors.New("模型 Provider 未配置")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.endpoint.ResolveReference(&url.URL{Path: "/v1/models"}).String(), nil)
	if err != nil {
		return Capability{}, err
	}
	a.authorize(request)
	response, err := a.client.Do(request)
	if err != nil {
		return Capability{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Capability{}, fmt.Errorf("model provider health returned HTTP %d", response.StatusCode)
	}
	return Capability{Provider: a.provider, Model: a.model, Endpoint: a.endpoint.String(), StructuredOutput: true, Streaming: true, Capabilities: []string{"chat.completions", "json_schema", "usage"}}, nil
}

func (a *Adapter) Complete(ctx context.Context, input CompletionRequest) (CompletionResult, error) {
	if a == nil || a.endpoint == nil {
		return CompletionResult{}, errors.New("模型 Provider 未配置")
	}
	if len(input.Messages) == 0 {
		return CompletionResult{}, errors.New("模型请求至少需要一条消息")
	}
	for _, message := range input.Messages {
		if message.Role != "system" && message.Role != "user" && message.Role != "assistant" {
			return CompletionResult{}, errors.New("模型消息 role 无效")
		}
	}
	payload := map[string]any{"model": a.model, "messages": input.Messages, "temperature": input.Temperature, "max_tokens": input.MaxTokens, "stream": false}
	if input.ResponseSchema != nil {
		payload["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "contentcloud_output", "strict": true, "schema": input.ResponseSchema}}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return CompletionResult{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint.ResolveReference(&url.URL{Path: "/v1/chat/completions"}).String(), bytes.NewReader(body))
	if err != nil {
		return CompletionResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if input.RequestID != "" {
		request.Header.Set("X-Request-ID", input.RequestID)
	}
	a.authorize(request)
	response, err := a.client.Do(request)
	if err != nil {
		return CompletionResult{}, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return CompletionResult{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return CompletionResult{}, fmt.Errorf("model provider completion returned HTTP %d", response.StatusCode)
	}
	var result struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			TotalTokens      int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &result); err != nil || len(result.Choices) == 0 {
		return CompletionResult{}, errors.New("模型 Provider 返回缺少 choices")
	}
	content := strings.TrimSpace(result.Choices[0].Message.Content)
	if content == "" {
		return CompletionResult{}, errors.New("模型 Provider 返回空内容")
	}
	completion := CompletionResult{Provider: a.provider, Model: firstNonEmpty(result.Model, a.model), Content: content, InputTokens: result.Usage.PromptTokens, OutputTokens: result.Usage.CompletionTokens, TotalTokens: result.Usage.TotalTokens, RequestID: firstNonEmpty(result.ID, input.RequestID), ReceivedAt: time.Now().UTC()}
	if input.ResponseSchema != nil {
		var structured map[string]any
		if err := json.Unmarshal([]byte(content), &structured); err != nil {
			return CompletionResult{}, errors.New("模型 Provider 结构化输出不是 JSON 对象")
		}
		completion.Structured, _ = json.Marshal(structured)
	}
	return completion, nil
}

func (a *Adapter) authorize(request *http.Request) {
	if a.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
