package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultOllamaBaseURL = "http://localhost:11434"
	ollamaTimeout        = 300 * time.Second // 5 minutes for large models
)

// OllamaConfig holds configuration for the Ollama provider.
type OllamaConfig struct {
	BaseURL string
	Model   string
	Timeout time.Duration
}

// OllamaProvider implements the Provider interface for Ollama.
type OllamaProvider struct {
	client  *http.Client
	baseURL string
	model   string
	timeout time.Duration
}

// NewOllamaProvider creates a new Ollama provider.
// If BaseURL is empty, defaults to http://localhost:11434.
// If Timeout is 0, defaults to 120 seconds.
func NewOllamaProvider(cfg OllamaConfig) (*OllamaProvider, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL
	}

	// Ensure baseURL doesn't have trailing slash
	baseURL = strings.TrimRight(baseURL, "/")

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = ollamaTimeout
	}

	return &OllamaProvider{
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 2,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		baseURL: baseURL,
		model:   cfg.Model,
		timeout: timeout,
	}, nil
}

// ollamaGenerateRequest is the request body for /api/generate.
type ollamaGenerateRequest struct {
	Model     string  `json:"model"`
	Prompt    string  `json:"prompt"`
	Stream    bool    `json:"stream"`
	Options   ollamaOptions `json:"options,omitempty"`
}

// ollamaOptions holds generation options for Ollama.
type ollamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

// ollamaGenerateResponse is the response body for /api/generate.
type ollamaGenerateResponse struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	Response  string `json:"response"`
	Done      bool   `json:"done"`
	Context   []int  `json:"context,omitempty"`
}

// ollamaTagsResponse is the response body for /api/tags.
type ollamaTagsResponse struct {
	Models []ollamaTagModel `json:"models"`
}

// ollamaTagModel represents a model in the /api/tags response.
type ollamaTagModel struct {
	Name       string            `json:"name"`
	Model      string            `json:"model"`
	ModifiedAt time.Time         `json:"modified_at"`
	Size       int64             `json:"size"`
	Digest     string            `json:"digest"`
	Details    ollamaModelDetails `json:"details"`
}

// ollamaModelDetails contains details about a model.
type ollamaModelDetails struct {
	ParentModel       string   `json:"parent_model"`
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

// Chat sends a chat request to Ollama using the /api/generate endpoint.
// For Ollama, we treat the last user message as the prompt.
func (p *OllamaProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, model string) (*Response, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	// Extract the prompt from messages
	// For Ollama, we concatenate the conversation history into a single prompt
	var promptBuilder strings.Builder
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			promptBuilder.WriteString("System: ")
			promptBuilder.WriteString(msg.Content)
			promptBuilder.WriteString("\n\n")
		case "user":
			promptBuilder.WriteString("User: ")
			promptBuilder.WriteString(msg.Content)
			promptBuilder.WriteString("\n\n")
		case "assistant":
			promptBuilder.WriteString("Assistant: ")
			promptBuilder.WriteString(msg.Content)
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					promptBuilder.WriteString(fmt.Sprintf("[Tool: %s(%s)]", tc.Function.Name, tc.Function.Arguments))
				}
			}
			promptBuilder.WriteString("\n\n")
		}
	}
	promptBuilder.WriteString("Assistant: ")

	prompt := promptBuilder.String()

	// Use provided model or fallback to configured model
	requestModel := model
	if requestModel == "" {
		requestModel = p.model
	}
	if requestModel == "" {
		return nil, fmt.Errorf("no model specified for Ollama request")
	}

	reqBody := ollamaGenerateRequest{
		Model:  requestModel,
		Prompt: prompt,
		Stream: false,
		Options: ollamaOptions{
			Temperature: 0.7,
			TopP:        0.9,
			NumPredict:  4096,
		},
	}

	reqData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/generate", bytes.NewReader(reqData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

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
		return nil, fmt.Errorf("Ollama API error %d: %s", resp.StatusCode, string(body))
	}

	var ollamaResp ollamaGenerateResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &Response{
		Content:      ollamaResp.Response,
		FinishReason: "stop",
		Usage: Usage{
			InputTokens:  len(ollamaResp.Context),
			OutputTokens: 0, // Ollama doesn't provide token counts in generate API
		},
	}, nil
}

// ListModels returns a list of available models from the Ollama instance.
func (p *OllamaProvider) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		return nil, fmt.Errorf("Ollama API error %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var tagsResp ollamaTagsResponse
	if err := json.Unmarshal(body, &tagsResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	models := make([]string, 0, len(tagsResp.Models))
	for _, m := range tagsResp.Models {
		models = append(models, m.Name)
	}

	return models, nil
}

// Ping checks if the Ollama instance is reachable by making a simple request.
func (p *OllamaProvider) Ping(ctx context.Context) error {
	// Use a short timeout for ping
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(pingCtx, "GET", p.baseURL+"/api/tags", nil)
	if err != nil {
		return fmt.Errorf("create ping request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("Ollama not reachable at %s: %w", p.baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		return fmt.Errorf("Ollama not reachable at %s: status %d - %s", p.baseURL, resp.StatusCode, string(body))
	}

	return nil
}

// Stream sends a streaming chat request to Ollama.
// Note: This is a basic implementation. Full streaming support would require
// returning a channel and handling SSE-like responses.
func (p *OllamaProvider) Stream(ctx context.Context, messages []Message, tools []ToolDefinition, model string) (<-chan string, error) {
	ch := make(chan string, 1)

	// For now, we'll just send a non-streaming request and send the result
	go func() {
		defer close(ch)

		resp, err := p.Chat(ctx, messages, tools, model)
		if err != nil {
			ch <- fmt.Sprintf("Error: %v", err)
			return
		}

		// Send response in chunks to simulate streaming
		scanner := bufio.NewScanner(strings.NewReader(resp.Content))
		for scanner.Scan() {
			select {
			case ch <- scanner.Text() + "\n":
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}
