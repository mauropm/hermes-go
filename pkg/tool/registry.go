package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/nousresearch/hermes-go/pkg/llm"
)

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
	sets  map[string]Toolset
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
		sets:  make(map[string]Toolset),
	}
}

func (r *Registry) Register(tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if tool.Name() == "" {
		return fmt.Errorf("tool name cannot be empty")
	}
	if _, exists := r.tools[tool.Name()]; exists {
		return fmt.Errorf("tool %q already registered", tool.Name())
	}

	r.tools[tool.Name()] = tool
	return nil
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

func (r *Registry) Definitions(names []string) []llm.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var defs []llm.ToolDefinition
	for _, name := range names {
		t, ok := r.tools[name]
		if !ok {
			continue
		}
		def := llm.ToolDefinition{
			Type: "function",
		}
		def.Function.Name = t.Name()
		def.Function.Description = t.Description()
		def.Function.Parameters = t.Schema()
		defs = append(defs, def)
	}
	return defs
}

func (r *Registry) AllDefinitions() []llm.ToolDefinition {
	return r.Definitions(r.Names())
}

func (r *Registry) RegisterToolset(ts Toolset) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sets[ts.Name] = ts
}

func (r *Registry) ResolveToolsets(names []string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	resolved := make(map[string]bool)
	var result []string

	var resolve func(name string) error
	resolve = func(name string) error {
		if resolved[name] {
			return nil
		}
		ts, ok := r.sets[name]
		if !ok {
			return fmt.Errorf("unknown toolset: %s", name)
		}
		for _, inc := range ts.Includes {
			if err := resolve(inc); err != nil {
				return err
			}
		}
		for _, t := range ts.Tools {
			if !resolved[t] {
				resolved[t] = true
				result = append(result, t)
			}
		}
		resolved[name] = true
		return nil
	}

	for _, name := range names {
		if err := resolve(name); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (r *Registry) Dispatch(name string, argsJSON string) string {
	r.mu.RLock()
	t, ok := r.tools[name]
	r.mu.RUnlock()

	if !ok {
		return toolResultJSON(false, nil, fmt.Sprintf("tool %q not found", name))
	}

	var args map[string]any
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return toolResultJSON(false, nil, fmt.Sprintf("invalid arguments: %v", err))
		}
	}

	ctx := context.Background()
	out, err := t.Execute(ctx, ToolInput{Arguments: args})
	if err != nil {
		return toolResultJSON(false, nil, err.Error())
	}
	return toolResultJSON(out.Success, out.Data, out.Error)
}

func toolResultJSON(success bool, data any, errMsg string) string {
	result := struct {
		Success bool   `json:"success"`
		Data    any    `json:"data,omitempty"`
		Error   string `json:"error,omitempty"`
	}{
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

func DefaultSchema(name, description string, properties map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
}
