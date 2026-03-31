package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	anthropicBaseURL    = "https://api.anthropic.com"
	anthropicAPIVersion = "2023-06-01"
)

type AnthropicProvider struct {
	client  *http.Client
	apiKey  string
	baseURL string
	timeout time.Duration
}

func NewAnthropicProvider(cfg ProviderConfig) (*AnthropicProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("anthropic API key is required")
	}

	baseURL := anthropicBaseURL
	if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	return &AnthropicProvider{
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 2,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		apiKey:  cfg.APIKey,
		baseURL: baseURL,
		timeout: timeout,
	}, nil
}

type anthropicRequest struct {
	Model       string                 `json:"model"`
	MaxTokens   int                    `json:"max_tokens"`
	Messages    []anthropicMessage     `json:"messages"`
	System      string                 `json:"system,omitempty"`
	Tools       []anthropicToolDef     `json:"tools,omitempty"`
	ToolChoice  map[string]interface{} `json:"tool_choice,omitempty"`
	Temperature float64                `json:"temperature,omitempty"`
}

type anthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type anthropicToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type anthropicResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type  string `json:"type"`
		Text  string `json:"text,omitempty"`
		ID    string `json:"id,omitempty"`
		Name  string `json:"name,omitempty"`
		Input struct {
			Arguments string `json:"-"`
		} `json:"input,omitempty"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (p *AnthropicProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, model string) (*Response, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	var systemPrompt string
	var chatMessages []anthropicMessage

	if messages[0].Role == "system" {
		systemPrompt = messages[0].Content
		messages = messages[1:]
	}

	for _, msg := range messages {
		am := anthropicMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			var content []interface{}
			if msg.Content != "" {
				content = append(content, map[string]interface{}{
					"type": "text",
					"text": msg.Content,
				})
			}
			for _, tc := range msg.ToolCalls {
				content = append(content, map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": json.RawMessage(tc.Function.Arguments),
				})
			}
			am.Content = content
		}
		if msg.Role == "tool" {
			am.Role = "user"
			am.Content = []interface{}{
				map[string]interface{}{
					"type":        "tool_result",
					"tool_use_id": msg.ToolCallID,
					"content":     msg.Content,
				},
			}
		}
		chatMessages = append(chatMessages, am)
	}

	var anthropicTools []anthropicToolDef
	for _, t := range tools {
		anthropicTools = append(anthropicTools, anthropicToolDef{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}

	reqBody := anthropicRequest{
		Model:     model,
		MaxTokens: 4096,
		Messages:  chatMessages,
		System:    systemPrompt,
		Tools:     anthropicTools,
	}

	reqData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/messages", bytes.NewReader(reqData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	result := &Response{
		FinishReason: anthropicResp.StopReason,
		Usage: Usage{
			InputTokens:  anthropicResp.Usage.InputTokens,
			OutputTokens: anthropicResp.Usage.OutputTokens,
		},
	}

	for _, c := range anthropicResp.Content {
		switch c.Type {
		case "text":
			result.Content = c.Text
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:   c.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      c.Name,
					Arguments: "{}",
				},
			})
		}
	}

	return result, nil
}
