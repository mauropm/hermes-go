package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nousresearch/hermes-go/pkg/llm"
	"github.com/nousresearch/hermes-go/pkg/tool"
)

type Planner struct {
	provider llm.Provider
	model    string
	tools    *tool.Registry
}

func New(provider llm.Provider, model string, tools *tool.Registry) *Planner {
	return &Planner{
		provider: provider,
		model:    model,
		tools:    tools,
	}
}

type Plan struct {
	Goal             string  `json:"goal"`
	Steps            []Step  `json:"steps"`
	Strategy         string  `json:"strategy"`
	Confidence       float64 `json:"confidence"`
	RequiresSubagent bool    `json:"requires_subagent"`
	SubagentTask     string  `json:"subagent_task,omitempty"`
}

type Step struct {
	ID          string         `json:"id"`
	Description string         `json:"description"`
	Tool        string         `json:"tool,omitempty"`
	Input       map[string]any `json:"input,omitempty"`
	DependsOn   []string       `json:"depends_on,omitempty"`
	Parallel    bool           `json:"parallel"`
	MaxRetries  int            `json:"max_retries"`
}

type DecompositionRequest struct {
	Goal     string
	Context  string
	Tools    []string
	MaxSteps int
}

type DecompositionResult struct {
	Plan       Plan
	Reasoning  string
	Confidence float64
}

func (p *Planner) Decompose(ctx context.Context, req DecompositionRequest) (*DecompositionResult, error) {
	if p.isSimpleQuery(req.Goal) {
		return &DecompositionResult{
			Plan: Plan{
				Goal:     req.Goal,
				Steps:    []Step{{ID: "step_1", Description: req.Goal, MaxRetries: 1}},
				Strategy: "direct",
			},
			Reasoning:  "Simple query, no decomposition needed",
			Confidence: 0.9,
		}, nil
	}

	prompt := p.buildDecompositionPrompt(req)
	resp, err := p.provider.Chat(ctx, []llm.Message{
		{Role: "system", Content: plannerSystemPrompt},
		{Role: "user", Content: prompt},
	}, nil, p.model)
	if err != nil {
		return nil, fmt.Errorf("LLM decomposition: %w", err)
	}

	var plan Plan
	if err := json.Unmarshal([]byte(resp.Content), &plan); err != nil {
		plan = Plan{
			Goal:     req.Goal,
			Steps:    []Step{{ID: "step_1", Description: req.Goal, MaxRetries: 1}},
			Strategy: "direct",
		}
	}

	if plan.Goal == "" {
		plan.Goal = req.Goal
	}
	for i := range plan.Steps {
		if plan.Steps[i].ID == "" {
			plan.Steps[i].ID = fmt.Sprintf("step_%d", i+1)
		}
		if plan.Steps[i].MaxRetries == 0 {
			plan.Steps[i].MaxRetries = 1
		}
	}

	return &DecompositionResult{
		Plan:       plan,
		Reasoning:  resp.Content,
		Confidence: plan.Confidence,
	}, nil
}

func (p *Planner) isSimpleQuery(goal string) bool {
	words := strings.Fields(goal)
	if len(words) <= 5 {
		return true
	}

	simpleIndicators := []string{
		"what is", "what's", "who is", "who's", "when is", "how many",
		"define", "explain", "tell me", "what does",
	}
	lower := strings.ToLower(goal)
	for _, indicator := range simpleIndicators {
		if strings.HasPrefix(lower, indicator) {
			return true
		}
	}

	return false
}

func (p *Planner) buildDecompositionPrompt(req DecompositionRequest) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Goal: %s\n\n", req.Goal))

	if req.Context != "" {
		b.WriteString(fmt.Sprintf("Context:\n%s\n\n", req.Context))
	}

	b.WriteString(fmt.Sprintf("Available tools: %s\n\n", strings.Join(req.Tools, ", ")))

	if req.MaxSteps > 0 {
		b.WriteString(fmt.Sprintf("Maximum steps: %d\n\n", req.MaxSteps))
	}

	b.WriteString("Return a JSON plan with steps, tool assignments, and dependencies.\n")

	return b.String()
}

const plannerSystemPrompt = `You are a task planner for an autonomous AI agent. Your job is to decompose user goals into executable steps.

Rules:
1. Each step should use exactly one tool or be a direct action
2. Steps with no dependencies can run in parallel
3. Only use tools from the available tools list
4. Keep plans concise - prefer fewer steps over many
5. If the goal is complex, consider setting requires_subagent to true
6. Return ONLY valid JSON, no markdown formatting

JSON Schema:
{
  "goal": "string",
  "strategy": "direct|decompose|delegate|research",
  "steps": [
    {
      "id": "step_1",
      "description": "what this step does",
      "tool": "tool_name or empty for direct action",
      "input": {"arg": "value"},
      "depends_on": ["step_1"],
      "parallel": false,
      "max_retries": 1
    }
  ],
  "confidence": 0.8,
  "requires_subagent": false,
  "subagent_task": "task description if delegation needed"
}`
