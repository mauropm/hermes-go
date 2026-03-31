package tools

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/nousresearch/hermes-go/llm"
)

type Handler func(args map[string]interface{}) string

type Tool struct {
	Name        string
	Description string
	Schema      map[string]interface{}
	Handler     Handler
	Parallel    bool
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]*Tool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]*Tool),
	}
}

func (r *Registry) Register(tool *Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[tool.Name]; exists {
		return fmt.Errorf("tool %q already registered", tool.Name)
	}

	if tool.Name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}

	if tool.Handler == nil {
		return fmt.Errorf("tool %q has no handler", tool.Name)
	}

	r.tools[tool.Name] = tool
	return nil
}

func (r *Registry) GetDefinitions() []llm.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var defs []llm.ToolDefinition
	for _, tool := range r.tools {
		def := llm.ToolDefinition{
			Type: "function",
		}
		def.Function.Name = tool.Name
		def.Function.Description = tool.Description
		def.Function.Parameters = tool.Schema
		defs = append(defs, def)
	}

	return defs
}

func (r *Registry) Dispatch(name string, argsJSON string) string {
	r.mu.RLock()
	tool, exists := r.tools[name]
	r.mu.RUnlock()

	if !exists {
		return llm.ToolResultJSON(false, nil, fmt.Sprintf("tool %q not found", name))
	}

	var args map[string]interface{}
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return llm.ToolResultJSON(false, nil, fmt.Sprintf("invalid arguments: %v", err))
		}
	}

	return tool.Handler(args)
}

func (r *Registry) GetTool(name string) (*Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

func (r *Registry) ListTools() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var names []string
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

func DefaultSchema(name, description string, properties map[string]interface{}, required []string) map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
}
