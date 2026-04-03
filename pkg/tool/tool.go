package tool

import (
	"context"
	"time"
)

type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Execute(ctx context.Context, input ToolInput) (ToolOutput, error)
	ParallelSafe() bool
}

type ToolInput struct {
	Arguments map[string]any
	Context   ToolContext
}

type ToolContext struct {
	SessionID   string
	WorkingDir  string
	Environment map[string]string
	Timeout     time.Duration
}

type ToolOutput struct {
	Success bool
	Data    string
	Error   string
	Meta    map[string]any
}

type ToolCategory string

const (
	CategoryUtility ToolCategory = "utility"
	CategoryWeb     ToolCategory = "web"
	CategoryCode    ToolCategory = "code"
	CategorySystem  ToolCategory = "system"
	CategoryMemory  ToolCategory = "memory"
	CategoryAgent   ToolCategory = "agent"
)

type Toolset struct {
	Name        string
	Description string
	Tools       []string
	Includes    []string
}
