package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nousresearch/hermes-go/core"
	"github.com/nousresearch/hermes-go/security"
)

const (
	maxRequestBodySize = 1 * 1024 * 1024
	defaultRateLimit   = 100
	rateLimitWindow    = 60 * time.Second
)

type Server struct {
	server    *http.Server
	agent     *core.Agent
	apiKey    string
	validator *security.InputValidator

	mu         sync.Mutex
	rateLimits map[string]*rateInfo
	rateLimit  int
}

type rateInfo struct {
	count   int
	resetAt time.Time
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Stream      bool      `json:"stream"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature"`
}

type message struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

type chatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   usage    `json:"usage"`
}

type choice struct {
	Index        int     `json:"index"`
	Message      message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func NewServer(agent *core.Agent, apiKey string, host string, port int) *Server {
	mux := http.NewServeMux()

	s := &Server{
		agent:      agent,
		apiKey:     apiKey,
		validator:  security.NewInputValidator(),
		rateLimits: make(map[string]*rateInfo),
		rateLimit:  defaultRateLimit,
	}

	mux.HandleFunc("/v1/chat/completions", s.handleChat)
	mux.HandleFunc("/health", s.handleHealth)

	s.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", host, port),
		Handler:      s.withMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return s
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if !s.checkRateLimit(r.RemoteAddr) {
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) authenticate(r *http.Request) bool {
	if s.apiKey == "" {
		return true
	}

	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}

	token := strings.TrimPrefix(auth, "Bearer ")
	return security.ConstantTimeCompare(token, s.apiKey)
}

func (s *Server) checkRateLimit(addr string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	host := extractHost(addr)
	info, exists := s.rateLimits[host]
	now := time.Now()

	if !exists || now.After(info.resetAt) {
		s.rateLimits[host] = &rateInfo{
			count:   1,
			resetAt: now.Add(rateLimitWindow),
		}
		return true
	}

	if info.count >= s.rateLimit {
		return false
	}

	info.count++
	return true
}

func extractHost(addr string) string {
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if !s.authenticate(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req chatRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid request: %v"}`, err), http.StatusBadRequest)
		return
	}

	if len(req.Messages) == 0 {
		http.Error(w, `{"error":"messages array is required"}`, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	userInput := req.Messages[len(req.Messages)-1].Content

	if err := s.validator.ValidateInput(userInput); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"input validation failed: %v"}`, err), http.StatusBadRequest)
		return
	}

	response, err := s.agent.Chat(ctx, userInput)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}

	resp := chatResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []choice{
			{
				Index: 0,
				Message: message{
					Role:    "assistant",
					Content: response,
				},
				FinishReason: "stop",
			},
		},
		Usage: usage{
			PromptTokens:     0,
			CompletionTokens: len(response) / 4,
			TotalTokens:      len(response) / 4,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := map[string]interface{}{
		"status": "healthy",
		"time":   time.Now().UTC().Format(time.RFC3339),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) Start() error {
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}
