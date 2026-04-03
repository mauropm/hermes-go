package llm

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolDefinition struct {
	Type     string `json:"type"`
	Function struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Parameters  map[string]interface{} `json:"parameters"`
	} `json:"function"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type Response struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
	Usage        Usage
}

type Provider interface {
	Chat(ctx context.Context, messages []Message, tools []ToolDefinition, model string) (*Response, error)
}

type ProviderConfig struct {
	Provider           string
	APIKey             string
	BaseURL            string
	Model              string
	Timeout            time.Duration
	BedrockRegion      string
	BedrockProfile     string
	BedrockBearerToken string
	BedrockAccessKey   string
	BedrockSecretKey   string
	// Ollama-specific fields
	OllamaBaseURL string
	OllamaModel   string
	OllamaThink   string
}

func NewProvider(cfg ProviderConfig) (Provider, error) {
	provider := strings.ToLower(cfg.Provider)

	if provider == "auto" {
		provider = DetectProvider(cfg.Model)
	}

	switch provider {
	case "anthropic":
		return NewAnthropicProvider(cfg)
	case "bedrock":
		return NewBedrockProvider(cfg.BedrockRegion, cfg.BedrockProfile, cfg.BedrockBearerToken, cfg.BedrockAccessKey, cfg.BedrockSecretKey, cfg.Timeout)
	case "ollama":
		return NewOllamaProvider(OllamaConfig{
			BaseURL: cfg.OllamaBaseURL,
			Model:   cfg.OllamaModel,
			Timeout: cfg.Timeout,
			Think:   cfg.OllamaThink,
		})
	default:
		return NewOpenAICompatibleProvider(cfg)
	}
}

func DetectProvider(model string) string {
	lower := strings.ToLower(model)
	if strings.HasPrefix(lower, "anthropic/") || strings.HasPrefix(lower, "claude") {
		return "anthropic"
	}
	if strings.Contains(lower, "bedrock") || strings.Contains(lower, "amazon") || strings.Contains(lower, "llama") || strings.Contains(lower, "mistral") || strings.Contains(lower, "cohere") || strings.Contains(lower, "ai21") || strings.Contains(lower, "deepseek") {
		return "bedrock"
	}
	if strings.HasPrefix(lower, "ollama/") || strings.HasPrefix(lower, "ollama:") {
		return "ollama"
	}
	return "openai"
}

func ParseModel(model string) (provider, modelName string) {
	parts := strings.SplitN(model, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "auto", model
}

type ToolResult struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func ToolResultJSON(success bool, data interface{}, errMsg string) string {
	result := ToolResult{
		Success: success,
		Data:    data,
		Error:   errMsg,
	}
	if !success {
		result.Data = nil
	} else {
		result.Error = ""
	}
	enc, _ := json.Marshal(result)
	return string(enc)
}
