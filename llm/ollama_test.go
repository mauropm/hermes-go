package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestOllamaProviderCreation tests that the provider can be created with various configs.
func TestOllamaProviderCreation(t *testing.T) {
	tests := []struct {
		name          string
		config        OllamaConfig
		wantBaseURL   string
		wantTimeout   time.Duration
		wantErr       bool
		errContains   string
	}{
		{
			name:        "default config",
			config:      OllamaConfig{},
			wantBaseURL: "http://localhost:11434",
			wantTimeout: 300 * time.Second,
			wantErr:     false,
		},
		{
			name: "custom base URL",
			config: OllamaConfig{
				BaseURL: "http://remote:11434",
			},
			wantBaseURL: "http://remote:11434",
			wantTimeout: 300 * time.Second,
			wantErr:     false,
		},
		{
			name: "custom timeout",
			config: OllamaConfig{
				BaseURL: "http://localhost:11434",
				Timeout: 60 * time.Second,
			},
			wantBaseURL: "http://localhost:11434",
			wantTimeout: 60 * time.Second,
			wantErr:     false,
		},
		{
			name: "URL with trailing slash",
			config: OllamaConfig{
				BaseURL: "http://localhost:11434/",
			},
			wantBaseURL: "http://localhost:11434",
			wantTimeout: 300 * time.Second,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewOllamaProvider(tt.config)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewOllamaProvider() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errContains != "" {
				if got := err.Error(); got != tt.errContains {
					t.Errorf("error = %v, want %v", got, tt.errContains)
				}
			}
			if provider != nil {
				if provider.baseURL != tt.wantBaseURL {
					t.Errorf("baseURL = %v, want %v", provider.baseURL, tt.wantBaseURL)
				}
				if provider.timeout != tt.wantTimeout {
					t.Errorf("timeout = %v, want %v", provider.timeout, tt.wantTimeout)
				}
			}
		})
	}
}

// TestOllamaProviderChat tests the Chat method with a mock server.
func TestOllamaProviderChat(t *testing.T) {
	expectedResponse := "Hello! I am Ollama."
	expectedModel := "llama3"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req ollamaGenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Model != expectedModel {
			http.Error(w, "Wrong model", http.StatusBadRequest)
			return
		}

		resp := ollamaGenerateResponse{
			Model:     expectedModel,
			CreatedAt: time.Now().Format(time.RFC3339),
			Response:  expectedResponse,
			Done:      true,
			Context:   []int{1, 2, 3, 4, 5},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(OllamaConfig{
		BaseURL: server.URL,
		Model:   expectedModel,
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	messages := []Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Hello!"},
	}

	ctx := context.Background()
	resp, err := provider.Chat(ctx, messages, nil, "")
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if resp.Content != expectedResponse {
		t.Errorf("Content = %v, want %v", resp.Content, expectedResponse)
	}

	if resp.FinishReason != "stop" {
		t.Errorf("FinishReason = %v, want stop", resp.FinishReason)
	}

	if resp.Usage.InputTokens != 5 {
		t.Errorf("InputTokens = %v, want 5", resp.Usage.InputTokens)
	}
}

// TestOllamaProviderChatWithModelOverride tests that model override works.
func TestOllamaProviderChatWithModelOverride(t *testing.T) {
	configuredModel := "llama3"
	overrideModel := "mistral"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ollamaGenerateRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Model != overrideModel {
			http.Error(w, "Wrong model: got "+req.Model, http.StatusBadRequest)
			return
		}

		resp := ollamaGenerateResponse{
			Model:    overrideModel,
			Response: "Response from " + overrideModel,
			Done:     true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewOllamaProvider(OllamaConfig{
		BaseURL: server.URL,
		Model:   configuredModel,
	})

	messages := []Message{{Role: "user", Content: "Test"}}
	ctx := context.Background()
	_, err := provider.Chat(ctx, messages, nil, overrideModel)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
}

// TestOllamaProviderChatEmptyMessages tests error handling for empty messages.
func TestOllamaProviderChatEmptyMessages(t *testing.T) {
	provider, _ := NewOllamaProvider(OllamaConfig{
		BaseURL: "http://localhost:11434",
		Model:   "llama3",
	})

	ctx := context.Background()
	_, err := provider.Chat(ctx, []Message{}, nil, "")
	if err == nil {
		t.Error("Chat() expected error for empty messages")
	}
}

// TestOllamaProviderChatNoModel tests error handling when no model is specified.
func TestOllamaProviderChatNoModel(t *testing.T) {
	provider, _ := NewOllamaProvider(OllamaConfig{
		BaseURL: "http://localhost:11434",
		Model:   "", // No model configured
	})

	messages := []Message{{Role: "user", Content: "Test"}}
	ctx := context.Background()
	_, err := provider.Chat(ctx, messages, nil, "")
	if err == nil {
		t.Error("Chat() expected error for no model")
	}
}

// TestOllamaProviderPing tests the Ping method.
func TestOllamaProviderPing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}

		resp := ollamaTagsResponse{
			Models: []ollamaTagModel{
				{Name: "llama3", Model: "llama3:latest"},
				{Name: "mistral", Model: "mistral:latest"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewOllamaProvider(OllamaConfig{
		BaseURL: server.URL,
		Model:   "llama3",
	})

	ctx := context.Background()
	err := provider.Ping(ctx)
	if err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

// TestOllamaProviderPingFailure tests ping failure when server is unreachable.
func TestOllamaProviderPingFailure(t *testing.T) {
	provider, _ := NewOllamaProvider(OllamaConfig{
		BaseURL: "http://localhost:1", // Invalid port
		Model:   "llama3",
		Timeout: 1 * time.Second,
	})

	ctx := context.Background()
	err := provider.Ping(ctx)
	if err == nil {
		t.Error("Ping() expected error for unreachable server")
	}
}

// TestOllamaProviderListModels tests the ListModels method.
func TestOllamaProviderListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}

		resp := ollamaTagsResponse{
			Models: []ollamaTagModel{
				{Name: "llama3:latest", Model: "llama3:latest", Size: 4*1024*1024*1024},
				{Name: "mistral:latest", Model: "mistral:latest", Size: 4*1024*1024*1024},
				{Name: "codellama:latest", Model: "codellama:latest", Size: 4*1024*1024*1024},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewOllamaProvider(OllamaConfig{
		BaseURL: server.URL,
		Model:   "llama3",
	})

	ctx := context.Background()
	models, err := provider.ListModels(ctx)
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}

	expectedModels := []string{"llama3:latest", "mistral:latest", "codellama:latest"}
	if len(models) != len(expectedModels) {
		t.Errorf("ListModels() returned %d models, want %d", len(models), len(expectedModels))
	}

	for i, model := range models {
		if model != expectedModels[i] {
			t.Errorf("models[%d] = %v, want %v", i, model, expectedModels[i])
		}
	}
}

// TestOllamaProviderStream tests the Stream method.
func TestOllamaProviderStream(t *testing.T) {
	expectedResponse := "Line 1\nLine 2\nLine 3"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaGenerateResponse{
			Model:    "llama3",
			Response: expectedResponse,
			Done:     true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewOllamaProvider(OllamaConfig{
		BaseURL: server.URL,
		Model:   "llama3",
	})

	messages := []Message{{Role: "user", Content: "Test"}}
	ctx := context.Background()

	stream, err := provider.Stream(ctx, messages, nil, "")
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	var result string
	for chunk := range stream {
		result += chunk
	}

	// Stream should contain the response
	if result == "" {
		t.Error("Stream() returned empty result")
	}
}

// TestOllamaProviderChatHTTPError tests handling of HTTP errors.
func TestOllamaProviderChatHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer server.Close()

	provider, _ := NewOllamaProvider(OllamaConfig{
		BaseURL: server.URL,
		Model:   "llama3",
	})

	messages := []Message{{Role: "user", Content: "Test"}}
	ctx := context.Background()
	_, err := provider.Chat(ctx, messages, nil, "")
	if err == nil {
		t.Error("Chat() expected error for HTTP 500")
	}
}
