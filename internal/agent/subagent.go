package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nousresearch/hermes-go/pkg/llm"
	"github.com/nousresearch/hermes-go/pkg/tool"
)

type SubagentManager struct {
	mu            sync.Mutex
	active        map[string]*Subagent
	maxConcurrent int
	maxDepth      int
	config        SubagentConfig
}

type Subagent struct {
	ID       string
	ParentID string
	Depth    int
	Agent    *Agent
	Cancel   context.CancelFunc
	Done     chan struct{}
	Result   chan SubagentResult
}

type SubagentConfig struct {
	MaxConcurrent int
	MaxDepth      int
	BlockedTools  []string
	MaxTurns      int
	Model         string
	Provider      llm.Provider
	ToolRegistry  *tool.Registry
}

type SubagentResult struct {
	Status    string
	Summary   string
	Duration  time.Duration
	Tokens    llm.Usage
	ToolTrace []ToolTraceEntry
	Error     string
}

type ToolTraceEntry struct {
	Tool        string
	ArgsBytes   int
	ResultBytes int
	Status      string
}

type SubagentRequest struct {
	ParentID       string
	Depth          int
	Task           string
	RequestedTools []string
}

func NewSubagentManager(config SubagentConfig) *SubagentManager {
	if config.MaxConcurrent <= 0 {
		config.MaxConcurrent = 3
	}
	if config.MaxDepth <= 0 {
		config.MaxDepth = 2
	}
	if config.MaxTurns <= 0 {
		config.MaxTurns = 30
	}

	return &SubagentManager{
		active:        make(map[string]*Subagent),
		maxConcurrent: config.MaxConcurrent,
		maxDepth:      config.MaxDepth,
		config:        config,
	}
}

func (m *SubagentManager) Spawn(ctx context.Context, req SubagentRequest) (*Subagent, error) {
	m.mu.Lock()
	if len(m.active) >= m.maxConcurrent {
		m.mu.Unlock()
		return nil, fmt.Errorf("max concurrent subagents (%d) reached", m.maxConcurrent)
	}
	if req.Depth >= m.maxDepth {
		m.mu.Unlock()
		return nil, fmt.Errorf("max delegation depth (%d) reached", m.maxDepth)
	}
	m.mu.Unlock()

	childTools := m.filterTools(req.RequestedTools)
	childRegistry := tool.NewRegistry()
	for _, name := range childTools {
		t, ok := m.config.ToolRegistry.Get(name)
		if ok {
			_ = childRegistry.Register(t)
		}
	}

	childAgent, err := NewAgent(AgentConfig{
		Model:        m.config.Model,
		Provider:     m.config.Provider,
		ToolRegistry: childRegistry,
		MaxTurns:     m.config.MaxTurns,
		SessionID:    uuid.New().String(),
		Source:       "subagent",
	})
	if err != nil {
		return nil, fmt.Errorf("create subagent: %w", err)
	}

	childCtx, cancel := context.WithCancel(ctx)
	sub := &Subagent{
		ID:       uuid.New().String()[:12],
		ParentID: req.ParentID,
		Depth:    req.Depth + 1,
		Agent:    childAgent,
		Cancel:   cancel,
		Done:     make(chan struct{}),
		Result:   make(chan SubagentResult, 1),
	}

	m.mu.Lock()
	m.active[sub.ID] = sub
	m.mu.Unlock()

	go func() {
		defer close(sub.Done)
		defer func() {
			m.mu.Lock()
			delete(m.active, sub.ID)
			m.mu.Unlock()
		}()

		start := time.Now()
		response, err := childAgent.Run(childCtx, req.Task)

		result := SubagentResult{
			Duration: time.Since(start),
		}
		if err != nil {
			result.Status = "failed"
			result.Error = err.Error()
		} else {
			result.Status = "completed"
			result.Summary = response
			result.Tokens = childAgent.TokenUsage()
		}

		sub.Result <- result
	}()

	return sub, nil
}

func (m *SubagentManager) Wait(ctx context.Context, sub *Subagent) (SubagentResult, error) {
	select {
	case result := <-sub.Result:
		return result, nil
	case <-ctx.Done():
		sub.Cancel()
		return SubagentResult{}, ctx.Err()
	}
}

func (m *SubagentManager) Cancel(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sub, ok := m.active[id]; ok {
		sub.Cancel()
	}
}

func (m *SubagentManager) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.active)
}

func (m *SubagentManager) filterTools(requested []string) []string {
	blocked := make(map[string]bool)
	for _, t := range m.config.BlockedTools {
		blocked[t] = true
	}

	var result []string
	for _, t := range requested {
		if !blocked[t] {
			result = append(result, t)
		}
	}
	return result
}
