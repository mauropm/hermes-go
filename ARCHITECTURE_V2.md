# Hermes-Go v2 Architecture

> A production-grade autonomous AI agent in Go, architected for modularity, observability, and self-improvement.

## Table of Contents

1. [Design Philosophy](#1-design-philosophy)
2. [System Architecture](#2-system-architecture)
3. [Package Structure](#3-package-structure)
4. [Core Agent Loop](#4-core-agent-loop)
5. [Memory System](#5-memory-system)
6. [Skill System](#6-skill-system)
7. [Tool System](#7-tool-system)
8. [Planner](#8-planner)
9. [Scheduler](#9-scheduler)
10. [Subagent System](#10-subagent-system)
11. [Event Bus](#11-event-bus)
12. [Proactive Behavior](#12-proactive-behavior)
13. [Learning Loop](#13-learning-loop)
14. [Storage Layout](#14-storage-layout)
15. [Extensibility](#15-extensibility)
16. [Safety & Security](#16-safety--security)
17. [LLM Integration](#17-llm-integration)
18. [Multi-Tenancy](#18-multi-tenancy)
19. [Observability](#19-observability)
20. [Implementation Roadmap](#20-implementation-roadmap)

---

## 1. Design Philosophy

### Guiding Principles

| Principle | Rationale |
|-----------|-----------|
| **Small interfaces** | Go's strength is composition via small interfaces (1-2 methods). Each package defines only what it needs. |
| **Explicit over implicit** | No magic registration via `init()`. Tools, skills, and plugins are wired explicitly at startup. |
| **Context everywhere** | All blocking operations accept `context.Context` for cancellation, timeouts, and tracing. |
| **Channels for coordination, mutexes for state** | Use channels to orchestrate goroutines; use `sync.Mutex` only for protecting shared data structures. |
| **Local-first storage** | All state lives on disk under `~/.hermes/`. No external database required. SQLite for structured data, JSON for documents. |
| **Fail-fast, recover-gracefully** | Panics are bugs. Errors are expected. Each subsystem handles its own errors and reports via events. |
| **Composability** | Subagents, tools, and skills compose like LEGO bricks. No inheritance hierarchies. |

### What This Is NOT

- Not a Python wrapper or port — idiomatic Go from the ground up
- Not a chatbot — an autonomous agent that acts proactively
- Not a framework — a concrete application with extension points

---

## 2. System Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         INTERFACE LAYER                              │
│  ┌──────────┐  ┌──────────────┐  ┌─────────┐  ┌─────────────────┐  │
│  │   CLI    │  │   Gateway    │  │   API   │  │   Cron/Daemon   │  │
│  │ (REPL)   │  │ (Messaging)  │  │ Server  │  │   (Proactive)   │  │
│  └────┬─────┘  └──────┬───────┘  └────┬────┘  └────────┬────────┘  │
│       └───────────────┴───────────────┴────────────────┘            │
│                              │                                       │
├──────────────────────────────┼───────────────────────────────────────┤
│                    ORCHESTRATION LAYER                                │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                     Agent Core                                │   │
│  │  ┌─────────┐ ┌────────┐ ┌──────────┐ ┌────────┐ ┌────────┐  │   │
│  │  │ Observe │→│ Decide │→│   Act    │→│ Learn  │→│ Recall │  │   │
│  │  └─────────┘ └────────┘ └──────────┘ └────────┘ └────────┘  │   │
│  └──────────────────────────┬───────────────────────────────────┘   │
│                             │                                        │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐               │
│  │ Planner  │ │ EventBus │ │Scheduler │ │Proactive │               │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘               │
│                                                                      │
├──────────────────────────────┼───────────────────────────────────────┤
│                        CAPABILITY LAYER                               │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐               │
│  │  Tools   │ │  Skills  │ │ Subagents│ │ Learning │               │
│  │ Registry │ │ Manager  │ │ Spawner  │ │  Loop    │               │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘               │
│                                                                      │
├──────────────────────────────┼───────────────────────────────────────┤
│                     INFRASTRUCTURE LAYER                              │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐               │
│  │  Memory  │ │ Sessions │ │   LLM    │ │  Config  │               │
│  │  System  │ │  (SQLite)│ │Providers │ │  System  │               │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘               │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### Data Flow: Single User Request

```
User input
  → CLI/Gateway/API receives message
  → Creates Agent instance (or reuses cached)
  → Agent.Observe():
      - Load relevant memory entries
      - Load applicable skills
      - Check recent activity patterns
  → Agent.Decide():
      - Planner decomposes intent into steps
      - Select tools for each step
  → Agent.Act():
      - Execute tool calls (sequential or parallel)
      - Subagent delegation for complex subtasks
  → Agent.Learn():
      - Store outcome in episodic memory
      - Create/update skill if successful
      - Emit learning event
  → Response delivered to user
```

### Data Flow: Proactive Cron Loop

```
Scheduler tick (every 60s)
  → Check due jobs
  → For each job:
      ProactiveEngine.Evaluate():
        - Check time patterns
        - Check memory for recurring needs
        - Check external signals (repo status, new data, etc.)
      If action warranted:
        → Create proactive task
        → Run through agent loop
        → Deliver to configured channel
        → Store outcome in memory
```

---

## 3. Package Structure

```
hermes-go/
├── cmd/
│   └── hermes/
│       └── main.go                     # Entry point: flag parsing, command dispatch
│
├── internal/                           # Private application code
│   ├── agent/
│   │   ├── agent.go                    # Core agent orchestrator
│   │   ├── loop.go                     # Observe-Decide-Act-Learn loop
│   │   ├── context.go                  # Agent context builder (memory + skills injection)
│   │   └── subagent.go                 # Subagent spawning and management
│   │
│   ├── planner/
│   │   ├── planner.go                  # Task decomposition engine
│   │   ├── step.go                     # Step definition and execution
│   │   └── graph.go                    # Dependency graph for parallel steps
│   │
│   ├── scheduler/
│   │   ├── scheduler.go                # Cron-based task scheduler
│   │   ├── job.go                      # Job definition and CRUD
│   │   └── ticker.go                   # Background tick loop
│   │
│   ├── proactive/
│   │   ├── engine.go                   # Proactive behavior engine
│   │   ├── patterns.go                 # Time/activity pattern detection
│   │   └── suggestions.go              # Action suggestion generator
│   │
│   ├── learning/
│   │   ├── loop.go                     # Learning loop orchestrator
│   │   ├── episodic.go                 # Episodic memory writer
│   │   └── skillgen.go                 # Auto-skill generation from successes
│   │
│   ├── cli/
│   │   ├── cli.go                      # Interactive REPL
│   │   ├── commands.go                 # Slash command registry
│   │   └── config_tui.go               # Configuration TUI
│   │
│   ├── api/
│   │   └── server.go                   # OpenAI-compatible HTTP API
│   │
│   └── gateway/                        # (Future) Multi-platform messaging
│       ├── gateway.go
│       └── platforms/
│
├── pkg/                                # Reusable packages (could be extracted)
│   ├── memory/
│   │   ├── store.go                    # Episodic memory (JSON-backed, TTL, trust levels)
│   │   ├── semantic.go                 # Semantic memory (tagged, indexed facts)
│   │   ├── working.go                  # Short-term working memory (session-scoped)
│   │   └── indexer.go                  # Simple inverted index for retrieval
│   │
│   ├── skill/
│   │   ├── skill.go                    # Skill definition and loader
│   │   ├── registry.go                 # Skill registry with tag-based retrieval
│   │   └── generator.go                # Auto-generate SKILL.md from task traces
│   │
│   ├── tool/
│   │   ├── tool.go                     # Tool interface definition
│   │   ├── registry.go                 # Thread-safe tool registry
│   │   ├── builtin.go                  # Built-in tools (time, calculator, etc.)
│   │   ├── web_search.go               # Web search tool
│   │   └── parallel.go                 # Parallel tool execution engine
│   │
│   ├── eventbus/
│   │   ├── bus.go                      # Event bus with typed channels
│   │   └── events.go                   # Event type definitions
│   │
│   ├── llm/
│   │   ├── provider.go                 # Provider interface + factory
│   │   ├── types.go                    # Shared types (Message, ToolCall, etc.)
│   │   ├── openai.go                   # OpenAI-compatible client
│   │   ├── anthropic.go                # Anthropic client
│   │   ├── bedrock.go                  # AWS Bedrock client
│   │   └── ollama.go                   # Ollama client
│   │
│   ├── storage/
│   │   ├── session.go                  # SQLite session store
│   │   └── migrations.go               # Schema migrations
│   │
│   ├── config/
│   │   ├── config.go                   # Multi-source configuration
│   │   └── save.go                     # Atomic config persistence
│   │
│   └── security/
│       ├── validator.go                # Input/output validation
│       └── sandbox.go                  # Execution sandboxing (future)
│
├── skills/                             # Bundled skills (SKILL.md files)
│   └── examples/
│
├── examples/
│   ├── skill.md                        # Example skill file
│   └── memory_record.json              # Example memory record
│
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

### Package Responsibilities

| Package | Responsibility | Dependencies |
|---------|---------------|--------------|
| `internal/agent` | Core orchestration, agent loop lifecycle | All other internal packages |
| `internal/planner` | Task decomposition, step selection | `pkg/tool`, `pkg/memory` |
| `internal/scheduler` | Cron job management, background execution | `internal/agent`, `pkg/eventbus` |
| `internal/proactive` | Pattern detection, suggestion generation | `pkg/memory`, `pkg/eventbus` |
| `internal/learning` | Post-task analysis, skill generation | `pkg/memory`, `pkg/skill` |
| `pkg/memory` | Episodic + semantic + working memory | `pkg/security` |
| `pkg/skill` | Skill loading, registry, auto-generation | None |
| `pkg/tool` | Tool interface, registry, dispatch | `pkg/llm` |
| `pkg/eventbus` | Type-safe event routing via channels | None |
| `pkg/llm` | Provider abstraction, API clients | None |
| `pkg/storage` | SQLite session persistence | None |
| `pkg/config` | Configuration loading and persistence | None |
| `pkg/security` | Input validation, sanitization | None |

---

## 4. Core Agent Loop

### The Observe-Decide-Act-Learn Cycle

```go
// loop.go — The core agent loop
func (a *Agent) Run(ctx context.Context, input string) (string, error) {
    // === OBSERVE ===
    observation := a.observe(ctx, input)

    // === DECIDE ===
    plan, err := a.decide(ctx, observation)
    if err != nil {
        return "", err
    }

    // === ACT ===
    result, err := a.act(ctx, plan)
    if err != nil {
        a.learn(ctx, TaskOutcome{
            Goal:    input,
            Plan:    plan,
            Success: false,
            Error:   err,
        })
        return "", err
    }

    // === LEARN ===
    a.learn(ctx, TaskOutcome{
        Goal:    input,
        Plan:    plan,
        Result:  result,
        Success: true,
    })

    return result.Response, nil
}
```

### Observe Phase

Gathers all relevant context before making decisions:

```go
type Observation struct {
    UserInput      string
    SessionHistory []llm.Message
    Memory         MemoryContext    // Relevant episodic + semantic entries
    Skills         []skill.Skill    // Applicable skills for this task
    ActiveTools    []tool.Tool      // Available tools (filtered by toolset)
    TimeContext    TimeContext      // Hour, day, timezone, patterns
    RecentActivity []ActivityRecord // Last N actions by this agent
    Signals        []Signal         // External signals (cron, proactive triggers)
}

func (a *Agent) observe(ctx context.Context, input string) Observation {
    obs := Observation{
        UserInput:   input,
        TimeContext: a.timePatterns.Now(),
    }

    // Load working memory (current session)
    obs.SessionHistory = a.messages

    // Retrieve relevant long-term memory
    if a.memStore != nil {
        entries, _ := a.memStore.RetrieveByRelevance(ctx, input, 10)
        obs.Memory = MemoryContext{Entries: entries}
    }

    // Load applicable skills
    if a.skillRegistry != nil {
        obs.Skills = a.skillRegistry.Match(input)
    }

    // Get available tools
    obs.ActiveTools = a.toolRegistry.List()

    // Check for external signals
    select {
    case signal := <-a.signalCh:
        obs.Signals = append(obs.Signals, signal)
    default:
    }

    return obs
}
```

### Decide Phase

The planner decomposes intent into executable steps:

```go
type Plan struct {
    Goal        string
    Steps       []Step
    Strategy    string       // "direct", "decompose", "delegate", "research"
    Confidence  float64      // 0.0-1.0, how confident the planner is
    RequiresSubagent bool
}

type Step struct {
    ID          string
    Description string
    Tool        string         // Tool name to use
    Input       map[string]any // Tool arguments
    DependsOn   []string       // Step IDs this depends on
    Parallel    bool           // Can run in parallel with siblings
    MaxRetries  int
}

func (a *Agent) decide(ctx context.Context, obs Observation) (Plan, error) {
    // For simple queries, create direct plan
    if a.isSimpleQuery(obs) {
        return Plan{
            Goal:     obs.UserInput,
            Steps:    []Step{{Description: "Answer directly", Tool: ""}},
            Strategy: "direct",
        }, nil
    }

    // For complex tasks, use LLM-assisted planning
    return a.planner.Decompose(ctx, obs)
}
```

### Act Phase

Executes the plan, handling parallel steps and subagent delegation:

```go
type ActionResult struct {
    Response  string
    ToolCalls []ToolCallRecord
    Duration  time.Duration
    Tokens    TokenUsage
}

func (a *Agent) act(ctx context.Context, plan Plan) (*ActionResult, error) {
    start := time.Now()
    result := &ActionResult{}

    // Build execution graph
    graph := buildExecutionGraph(plan.Steps)

    for _, level := range graph.TopologicalLevels() {
        // Parallel execution for independent steps
        if len(level) > 1 {
            results := a.executeParallel(ctx, level)
            result.ToolCalls = append(result.ToolCalls, results...)
        } else {
            r, err := a.executeStep(ctx, level[0])
            if err != nil {
                return nil, fmt.Errorf("step %s: %w", level[0].ID, err)
            }
            result.ToolCalls = append(result.ToolCalls, r)
        }
    }

    result.Duration = time.Since(start)
    result.Response = a.formatResult(result.ToolCalls)
    return result, nil
}
```

### Learn Phase

Stores outcomes and generates skills:

```go
type TaskOutcome struct {
    Goal      string
    Plan      Plan
    Result    *ActionResult
    Success   bool
    Error     error
    Timestamp time.Time
}

func (a *Agent) learn(ctx context.Context, outcome TaskOutcome) {
    // 1. Store in episodic memory
    a.memStore.StoreEpisodic(ctx, EpisodicRecord{
        Goal:      outcome.Goal,
        Steps:     outcome.Plan.Steps,
        Success:   outcome.Success,
        Duration:  outcome.Result.Duration,
        Timestamp: outcome.Timestamp,
    })

    // 2. If successful, consider skill generation
    if outcome.Success && a.learningLoop != nil {
        a.learningLoop.RecordSuccess(ctx, outcome)
    }

    // 3. If failed, store for analysis
    if !outcome.Success {
        a.learningLoop.RecordFailure(ctx, outcome)
    }

    // 4. Emit event
    a.eventBus.Publish(EventTaskCompleted{
        TaskID:  outcome.Goal,
        Success: outcome.Success,
    })
}
```

---

## 5. Memory System

### Three-Tier Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Working Memory                            │
│  - Session-scoped, in-memory                                 │
│  - Current conversation context                              │
│  - Cleared on session reset                                  │
│  - Access: O(1) map lookup                                   │
├─────────────────────────────────────────────────────────────┤
│                    Episodic Memory                           │
│  - Task outcomes, conversation summaries                     │
│  - Persistent (JSON files), TTL-based expiration             │
│  - Indexed by tags, time, success/failure                    │
│  - Access: Inverted index + relevance scoring                │
├─────────────────────────────────────────────────────────────┤
│                    Semantic Memory                           │
│  - Facts, preferences, user profile                          │
│  - Persistent (JSON files), no automatic expiration          │
│  - Tagged, searchable, confidence-scored                     │
│  - Access: Tag-based retrieval + keyword matching            │
└─────────────────────────────────────────────────────────────┘
```

### Working Memory

```go
// pkg/memory/working.go
type WorkingMemory struct {
    mu       sync.RWMutex
    facts    map[string]Fact       // Key → fact
    context  map[string]string     // Key → value (arbitrary context)
    sessionID string
    maxSize  int
}

type Fact struct {
    Key       string
    Value     string
    Source    string    // "user", "inferred", "tool"
    Confidence float64  // 0.0-1.0
    CreatedAt time.Time
}

func (wm *WorkingMemory) Set(key, value string, source string)
func (wm *WorkingMemory) Get(key string) (Fact, bool)
func (wm *WorkingMemory) List() []Fact
func (wm *WorkingMemory) Clear()
```

### Episodic Memory

```go
// pkg/memory/store.go
type EpisodicStore struct {
    mu       sync.RWMutex
    records  []EpisodicRecord
    index    *InvertedIndex
    storeDir string
    ttl      time.Duration
    maxSize  int
}

type EpisodicRecord struct {
    ID        string    `json:"id"`
    Goal      string    `json:"goal"`
    Summary   string    `json:"summary"`
    Steps     []Step    `json:"steps,omitempty"`
    ToolsUsed []string  `json:"tools_used"`
    Success   bool      `json:"success"`
    Duration  time.Duration `json:"duration"`
    Tags      []string  `json:"tags"`
    CreatedAt time.Time `json:"created_at"`
    ExpiresAt time.Time `json:"expires_at"`
    Hash      string    `json:"hash"`
}

func (es *EpisodicStore) Store(ctx context.Context, record EpisodicRecord) error
func (es *EpisodicStore) RetrieveByTags(ctx context.Context, tags []string, limit int) ([]EpisodicRecord, error)
func (es *EpisodicStore) RetrieveByRelevance(ctx context.Context, query string, limit int) ([]EpisodicRecord, error)
func (es *EpisodicStore) Recent(ctx context.Context, since time.Time, limit int) ([]EpisodicRecord, error)
func (es *EpisodicStore) Statistics(ctx context.Context) (StoreStats, error)
```

### Semantic Memory

```go
// pkg/memory/semantic.go
type SemanticStore struct {
    mu       sync.RWMutex
    facts    map[string]SemanticFact
    index    *InvertedIndex
    storeDir string
}

type SemanticFact struct {
    ID         string    `json:"id"`
    Category   string    `json:"category"`  // "user_preference", "fact", "pattern", "rule"
    Key        string    `json:"key"`
    Value      string    `json:"value"`
    Tags       []string  `json:"tags"`
    Confidence float64   `json:"confidence"`
    Source     string    `json:"source"`    // "user_stated", "inferred", "learned"
    CreatedAt  time.Time `json:"created_at"`
    UpdatedAt  time.Time `json:"updated_at"`
}

func (ss *SemanticStore) Store(ctx context.Context, fact SemanticFact) error
func (ss *SemanticStore) RetrieveByCategory(ctx context.Context, category string) ([]SemanticFact, error)
func (ss *SemanticStore) RetrieveByTags(ctx context.Context, tags []string) ([]SemanticFact, error)
func (ss *SemanticStore) UpdateConfidence(ctx context.Context, id string, delta float64) error
```

### Inverted Index

```go
// pkg/memory/indexer.go
type InvertedIndex struct {
    mu       sync.RWMutex
    postings map[string]map[string]int  // term → {recordID → frequency}
    docFreq  map[string]int             // term → document frequency
}

func (idx *InvertedIndex) Index(id string, terms []string)
func (idx *InvertedIndex) Remove(id string)
func (idx *InvertedIndex) Search(query []string, topK int) []ScoredResult
func (idx *InvertedIndex) RelevanceScore(query []string, docTerms []string) float64
```

---

## 6. Skill System

### Skill Definition

Skills are stored as `SKILL.md` files with YAML frontmatter:

```yaml
---
name: "deploy-go-service"
description: "Deploy a Go service to a Linux server via SSH"
version: "1.0.0"
tags: ["deployment", "go", "ssh", "linux"]
tools_required: ["ssh_execute", "file_write"]
confidence: 0.95
created_at: "2026-04-01T10:00:00Z"
updated_at: "2026-04-01T10:00:00Z"
usage_count: 5
success_rate: 1.0
---
```

```markdown
# Deploy Go Service

## When to Use
When the user asks to deploy a Go application to a remote server.

## Prerequisites
- SSH access to target server
- Go binary already compiled
- Target server running Linux

## Steps
1. Verify binary exists and is executable
2. SSH to target server
3. Stop existing service (systemctl stop)
4. Upload new binary via SCP
5. Set permissions (chmod +x)
6. Start service (systemctl start)
7. Verify health endpoint responds

## Common Pitfalls
- Binary architecture mismatch (GOOS/GOARCH)
- Port already in use
- Missing environment variables

## Example
User: "Deploy my service to prod"
→ Compile: GOOS=linux GOARCH=amd64 go build
→ Upload and restart via SSH
```

### Skill Interface

```go
// pkg/skill/skill.go
type Skill struct {
    Name          string    `yaml:"name"`
    Description   string    `yaml:"description"`
    Version       string    `yaml:"version"`
    Tags          []string  `yaml:"tags"`
    ToolsRequired []string  `yaml:"tools_required"`
    Confidence    float64   `yaml:"confidence"`
    CreatedAt     time.Time `yaml:"created_at"`
    UpdatedAt     time.Time `yaml:"updated_at"`
    UsageCount    int       `yaml:"usage_count"`
    SuccessRate   float64   `yaml:"success_rate"`
    Content       string    `yaml:"-"` // Markdown body (not in frontmatter)
    FilePath      string    `yaml:"-"` // Source file path
}

type Registry struct {
    mu       sync.RWMutex
    skills   map[string]*Skill
    index    *InvertedIndex
    homeDir  string
}

func (r *Registry) LoadAll() error                              // Load all SKILL.md from skills/
func (r *Registry) Load(path string) error                      // Load single skill
func (r *Registry) Match(query string) []Skill                  // Tag + keyword matching
func (r *Registry) Get(name string) (*Skill, bool)              // Get by name
func (r *Registry) RecordUsage(name string, success bool)       // Update usage stats
func (r *Registry) FormatForPrompt(skills []Skill) string       // Format for LLM context
```

### Auto-Skill Generation

```go
// pkg/skill/generator.go
type Generator struct {
    skillsDir string
    minSuccesses int  // Minimum successes before auto-generating
}

func (g *Generator) ShouldGenerate(outcome TaskOutcome) bool {
    // Check if similar tasks succeeded multiple times
    // Check if no existing skill covers this pattern
    return g.similarSuccessCount(outcome) >= g.minSuccesses
}

func (g *Generator) Generate(outcome TaskOutcome) (*Skill, error) {
    skill := &Skill{
        Name:          slugify(outcome.Goal),
        Description:   fmt.Sprintf("Auto-generated: %s", outcome.Goal),
        Tags:          extractTags(outcome),
        ToolsRequired: unique(outcome.Result.ToolCalls),
        Confidence:    0.5, // Start conservative
        Content:       generateMarkdown(outcome),
    }
    return skill, g.save(skill)
}
```

---

## 7. Tool System

### Tool Interface

```go
// pkg/tool/tool.go
type Tool interface {
    Name() string
    Description() string
    Schema() map[string]any       // JSON Schema for parameters
    Execute(ctx context.Context, input ToolInput) (ToolOutput, error)
    ParallelSafe() bool           // Can this tool run in parallel?
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
    Meta    map[string]any  // Optional metadata (duration, tokens, etc.)
}

type ToolCategory string

const (
    CategoryUtility    ToolCategory = "utility"
    CategoryWeb        ToolCategory = "web"
    CategoryCode       ToolCategory = "code"
    CategorySystem     ToolCategory = "system"
    CategoryMemory     ToolCategory = "memory"
    CategoryAgent      ToolCategory = "agent"
)
```

### Tool Registry

```go
// pkg/tool/registry.go
type Registry struct {
    mu     sync.RWMutex
    tools  map[string]Tool
    sets   map[string]Toolset   // Named toolsets
}

type Toolset struct {
    Name        string
    Description string
    Tools       []string   // Tool names included
    Includes    []string   // Other toolset names (recursive composition)
}

func (r *Registry) Register(tool Tool) error
func (r *Registry) Deregister(name string)
func (r *Registry) Get(name string) (Tool, bool)
func (r *Registry) List() []Tool
func (r *Registry) Definitions(toolsets []string) []llm.ToolDefinition
func (r *Registry) RegisterToolset(ts Toolset)
func (r *Registry) ResolveToolsets(names []string) ([]string, error)  // Flatten with cycle detection
```

### Parallel Execution Engine

```go
// pkg/tool/parallel.go
type ParallelExecutor struct {
    maxWorkers int
    registry   *Registry
}

type ExecutionPlan struct {
    Sequential []string   // Must run in order
    Parallel   [][]string // Groups that can run in parallel
}

func (pe *ParallelExecutor) Execute(ctx context.Context, plan ExecutionPlan, inputs map[string]ToolInput) ([]ToolOutput, error) {
    var results []ToolOutput

    // Run sequential steps
    for _, name := range plan.Sequential {
        out, err := pe.registry.Get(name).Execute(ctx, inputs[name])
        if err != nil {
            return results, err
        }
        results = append(results, out)
    }

    // Run parallel groups
    for _, group := range plan.Parallel {
        type result struct {
            idx   int
            out   ToolOutput
            err   error
        }
        ch := make(chan result, len(group))

        var wg sync.WaitGroup
        sem := make(chan struct{}, pe.maxWorkers)

        for i, name := range group {
            wg.Add(1)
            go func(idx int, toolName string) {
                defer wg.Done()
                sem <- struct{}{}
                defer func() { <-sem }()

                tool, _ := pe.registry.Get(toolName)
                out, err := tool.Execute(ctx, inputs[toolName])
                ch <- result{idx, out, err}
            }(i, name)
        }

        wg.Wait()
        close(ch)

        // Collect results in order
        ordered := make([]ToolOutput, len(group))
        for r := range ch {
            if r.err != nil {
                return results, fmt.Errorf("parallel tool %s: %w", group[r.idx], r.err)
            }
            ordered[r.idx] = r.out
        }
        results = append(results, ordered...)
    }

    return results, nil
}
```

---

## 8. Planner

### Task Decomposition

```go
// internal/planner/planner.go
type Planner struct {
    provider  llm.Provider
    model     string
    tools     *tool.Registry
    memory    *memory.EpisodicStore
}

type DecompositionRequest struct {
    Goal        string
    Context     string       // Relevant memory + skills
    Tools       []string     // Available tool names
    MaxSteps    int
}

type DecompositionResult struct {
    Plan       Plan
    Reasoning  string       // Why this plan was chosen
    Confidence float64
}

func (p *Planner) Decompose(ctx context.Context, req DecompositionRequest) (*DecompositionResult, error) {
    // Use LLM to decompose complex goals into steps
    prompt := p.buildDecompositionPrompt(req)
    resp, err := p.provider.Chat(ctx, []llm.Message{
        {Role: "system", Content: plannerSystemPrompt},
        {Role: "user", Content: prompt},
    }, nil, p.model)
    if err != nil {
        return nil, err
    }

    var plan Plan
    if err := json.Unmarshal([]byte(resp.Content), &plan); err != nil {
        // Fallback: single-step plan
        plan = Plan{
            Goal:     req.Goal,
            Steps:    []Step{{Description: req.Goal}},
            Strategy: "direct",
        }
    }

    return &DecompositionResult{
        Plan:       plan,
        Reasoning:  "LLM decomposition",
        Confidence: 0.8,
    }, nil
}
```

### Execution Graph

```go
// internal/planner/graph.go
type ExecutionGraph struct {
    nodes map[string]*StepNode
    edges map[string][]string  // from → to
}

type StepNode struct {
    Step    Step
    Ready   bool
    Done    bool
    Result  *tool.ToolOutput
}

func (g *ExecutionGraph) TopologicalLevels() [][]Step {
    // Kahn's algorithm, returning levels (not just ordering)
    // Each level contains steps that can run in parallel
}

func (g *ExecutionGraph) AddStep(step Step)
func (g *ExecutionGraph) MarkDone(stepID string, result *tool.ToolOutput)
func (g *ExecutionGraph) IsComplete() bool
```

---

## 9. Scheduler

### Cron-Based Scheduling

```go
// internal/scheduler/scheduler.go
type Scheduler struct {
    mu       sync.RWMutex
    jobs     map[string]*Job
    ticker   *time.Ticker
    bus      *eventbus.Bus
    storeDir string
    running  bool
}

type Job struct {
    ID          string        `json:"id"`
    Name        string        `json:"name"`
    Prompt      string        `json:"prompt"`
    Schedule    Schedule      `json:"schedule"`
    Enabled     bool          `json:"enabled"`
    State       JobState      `json:"state"`
    Deliver     DeliveryTarget `json:"deliver"`
    Skills      []string      `json:"skills,omitempty"`
    Toolsets    []string      `json:"toolsets,omitempty"`
    MaxTurns    int           `json:"max_turns,omitempty"`
    CreatedAt   time.Time     `json:"created_at"`
    NextRunAt   time.Time     `json:"next_run_at"`
    LastRunAt   time.Time     `json:"last_run_at,omitempty"`
    LastStatus  string        `json:"last_status,omitempty"`
    LastError   string        `json:"last_error,omitempty"`
    RunCount    int           `json:"run_count"`
}

type Schedule struct {
    Kind       string `json:"kind"`       // "cron", "interval", "once", "at"
    Expression string `json:"expression"` // cron expr or duration
}

type DeliveryTarget struct {
    Kind   string `json:"kind"`   // "log", "memory", "channel"
    Target string `json:"target"` // channel ID or file path
}

type JobState string

const (
    StateScheduled JobState = "scheduled"
    StateRunning   JobState = "running"
    StatePaused    JobState = "paused"
    StateCompleted JobState = "completed"
    StateFailed    JobState = "failed"
)

func (s *Scheduler) Start(ctx context.Context)
func (s *Scheduler) Stop()
func (s *Scheduler) AddJob(job *Job) error
func (s *Scheduler) RemoveJob(id string) error
func (s *Scheduler) PauseJob(id string) error
func (s *Scheduler) ResumeJob(id string) error
func (s *Scheduler) ListJobs() []*Job
func (s *Scheduler) tick(ctx context.Context)  // Called every 60s
```

### Ticker Loop

```go
func (s *Scheduler) tick(ctx context.Context) {
    s.mu.RLock()
    var due []*Job
    now := time.Now()
    for _, job := range s.jobs {
        if job.Enabled && job.State == StateScheduled && !job.NextRunAt.After(now) {
            due = append(due, job)
        }
    }
    s.mu.RUnlock()

    for _, job := range s.jobs {
        s.mu.Lock()
        job.NextRunAt = job.Schedule.Next(now)  // Advance before execution (at-most-once)
        job.State = StateRunning
        s.mu.Unlock()

        // Run job in goroutine
        go s.runJob(ctx, job)
    }
}

func (s *Scheduler) runJob(ctx context.Context, job *Job) {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
    defer cancel()

    start := time.Now()
    err := s.executeJob(ctx, job)

    s.mu.Lock()
    job.LastRunAt = start
    job.RunCount++
    if err != nil {
        job.LastStatus = "failed"
        job.LastError = err.Error()
        job.State = StateFailed
    } else {
        job.LastStatus = "completed"
        job.State = StateScheduled
    }
    s.mu.Unlock()

    s.bus.Publish(EventJobCompleted{
        JobID:   job.ID,
        Success: err == nil,
    })
}
```

---

## 10. Subagent System

### Subagent Spawning

```go
// internal/agent/subagent.go
type SubagentManager struct {
    mu          sync.Mutex
    active      map[string]*Subagent
    maxConcurrent int
    maxDepth    int
    config      SubagentConfig
}

type Subagent struct {
    ID        string
    ParentID  string
    Depth     int
    Agent     *Agent
    Cancel    context.CancelFunc
    Done      chan struct{}
    Result    chan SubagentResult
}

type SubagentConfig struct {
    MaxConcurrent int
    MaxDepth      int
    BlockedTools  []string
    MaxTurns      int
    Model         string
}

type SubagentResult struct {
    Status   string
    Summary  string
    Duration time.Duration
    Tokens   llm.Usage
    ToolTrace []ToolTraceEntry
    Error    string
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

    // Create child agent with restricted toolset
    childTools := m.filterTools(req.RequestedTools)
    childAgent, err := NewAgent(AgentConfig{
        Model:        m.config.Model,
        ToolRegistry: tool.NewFilteredRegistry(childTools),
        MaxTurns:     m.config.MaxTurns,
        SessionID:    uuid.New().String(),
        Source:       "subagent",
        ParentID:     req.ParentID,
    })
    if err != nil {
        return nil, err
    }

    childCtx, cancel := context.WithCancel(ctx)
    sub := &Subagent{
        ID:       uuid.New().String(),
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

    // Run in goroutine
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
```

---

## 11. Event Bus

### Type-Safe Event Routing

```go
// pkg/eventbus/bus.go
type Bus struct {
    mu       sync.RWMutex
    handlers map[EventType][]Handler
    closed   bool
}

type EventType string

const (
    EventTaskCompleted  EventType = "task.completed"
    EventToolCalled     EventType = "tool.called"
    EventMemoryStored   EventType = "memory.stored"
    EventSkillCreated   EventType = "skill.created"
    EventJobCompleted   EventType = "job.completed"
    EventError          EventType = "error"
    EventProactive      EventType = "proactive.suggestion"
    EventSessionStart   EventType = "session.start"
    EventSessionEnd     EventType = "session.end"
)

type Handler func(Event)

type Event interface {
    Type() EventType
    Timestamp() time.Time
}

func (b *Bus) Subscribe(eventType EventType, handler Handler)
func (b *Bus) Unsubscribe(eventType EventType, handler Handler)
func (b *Bus) Publish(event Event)
func (b *Bus) Close()

// Publish dispatches events to handlers in separate goroutines
func (b *Bus) Publish(event Event) {
    b.mu.RLock()
    handlers := b.handlers[event.Type()]
    b.mu.RUnlock()

    for _, h := range handlers {
        go h(event)  // Non-blocking delivery
    }
}
```

### Event Definitions

```go
// pkg/eventbus/events.go
type EventTaskCompleted struct {
    TaskID    string
    Success   bool
    Duration  time.Duration
    Error     string
    Timestamp time.Time
}

func (e EventTaskCompleted) Type() EventType { return EventTaskCompleted }
func (e EventTaskCompleted) Timestamp() time.Time { return e.Timestamp }

type EventToolCalled struct {
    ToolName string
    Args     map[string]any
    Success  bool
    Duration time.Duration
    Timestamp time.Time
}

func (e EventToolCalled) Type() EventType { return EventToolCalled }
func (e EventToolCalled) Timestamp() time.Time { return e.Timestamp }

type EventProactive struct {
    Suggestion  string
    Reason      string
    Confidence  float64
    Timestamp   time.Time
}

func (e EventProactive) Type() EventType { return EventProactive }
func (e EventProactive) Timestamp() time.Time { return e.Timestamp }
```

---

## 12. Proactive Behavior

### Pattern Detection Engine

```go
// internal/proactive/engine.go
type Engine struct {
    memory     *memory.EpisodicStore
    patterns   *PatternDetector
    bus        *eventbus.Bus
    config     ProactiveConfig
}

type ProactiveConfig struct {
    Enabled         bool
    CheckInterval   time.Duration
    MinConfidence   float64
    QuietHours      QuietHours
    MaxSuggestionsPerHour int
}

type QuietHours struct {
    Start string  // "22:00"
    End   string  // "07:00"
}

func (e *Engine) Start(ctx context.Context) {
    ticker := time.NewTicker(e.config.CheckInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            e.evaluate(ctx)
        }
    }
}

func (e *Engine) evaluate(ctx context.Context) {
    if e.config.isQuietHours(time.Now()) {
        return
    }

    suggestions := e.patterns.Detect(ctx)
    for _, s := range suggestions {
        if s.Confidence >= e.config.MinConfidence {
            e.bus.Publish(eventbus.EventProactive{
                Suggestion: s.Message,
                Reason:     s.Reason,
                Confidence: s.Confidence,
            })
        }
    }
}
```

### Pattern Detector

```go
// internal/proactive/patterns.go
type PatternDetector struct {
    memory *memory.EpisodicStore
}

type Pattern struct {
    Type        string    // "time_based", "frequency", "sequence", "anomaly"
    Description string
    Confidence  float64
    Message     string
    Reason      string
}

func (pd *PatternDetector) Detect(ctx context.Context) []Pattern {
    var patterns []Pattern

    // Time-based patterns: "You usually check X at this hour"
    patterns = append(patterns, pd.detectTimePatterns(ctx)...)

    // Frequency patterns: "You've done X 5 times this week"
    patterns = append(patterns, pd.detectFrequencyPatterns(ctx)...)

    // Sequence patterns: "After X, you usually do Y"
    patterns = append(patterns, pd.detectSequencePatterns(ctx)...)

    // Anomaly detection: "This repo has failing tests"
    patterns = append(patterns, pd.detectAnomalies(ctx)...)

    return patterns
}

func (pd *PatternDetector) detectTimePatterns(ctx context.Context) []Pattern {
    now := time.Now()
    hour := now.Hour()

    // Look for tasks commonly done at this hour
    records, _ := pd.memory.Recent(ctx, now.AddDate(0, 0, -7), 100)

    hourCounts := make(map[string]int)
    for _, r := range records {
        h := r.CreatedAt.Hour()
        hourCounts[r.Goal]++
    }

    var patterns []Pattern
    for goal, count := range hourCounts {
        if count >= 3 {
            patterns = append(patterns, Pattern{
                Type:       "time_based",
                Confidence: float64(count) / 7.0,
                Message:    fmt.Sprintf("You usually %s around this time", goal),
                Reason:     fmt.Sprintf("Done %d times at hour %d in the past week", count, hour),
            })
        }
    }
    return patterns
}
```

---

## 13. Learning Loop

### Learning Orchestrator

```go
// internal/learning/loop.go
type LearningLoop struct {
    episodic    *memory.EpisodicStore
    semantic    *memory.SemanticStore
    skillGen    *skill.Generator
    bus         *eventbus.Bus
    failures    []TaskOutcome  // Rolling window of failures
    maxFailures int
}

func (ll *LearningLoop) RecordSuccess(ctx context.Context, outcome TaskOutcome) {
    // 1. Store in episodic memory
    record := memory.EpisodicRecord{
        Goal:      outcome.Goal,
        Summary:   outcome.Result.Response,
        ToolsUsed: extractToolNames(outcome.Result.ToolCalls),
        Success:   true,
        Duration:  outcome.Result.Duration,
        Tags:      autoTag(outcome),
        CreatedAt: outcome.Timestamp,
    }
    ll.episodic.Store(ctx, record)

    // 2. Consider skill generation
    if ll.skillGen.ShouldGenerate(outcome) {
        skill, err := ll.skillGen.Generate(outcome)
        if err == nil {
            ll.bus.Publish(eventbus.EventSkillCreated{
                SkillName: skill.Name,
            })
        }
    }

    // 3. Extract semantic facts
    facts := extractFacts(outcome)
    for _, f := range facts {
        ll.semantic.Store(ctx, f)
    }
}

func (ll *LearningLoop) RecordFailure(ctx context.Context, outcome TaskOutcome) {
    // Store failure for pattern analysis
    ll.failures = append(ll.failures, outcome)
    if len(ll.failures) > ll.maxFailures {
        ll.failures = ll.failures[1:]
    }

    // Store in episodic memory (marked as failed)
    record := memory.EpisodicRecord{
        Goal:      outcome.Goal,
        Summary:   outcome.Error.Error(),
        ToolsUsed: extractToolNames(outcome.Result.ToolCalls),
        Success:   false,
        Duration:  outcome.Result.Duration,
        Tags:      append(autoTag(outcome), "failed"),
        CreatedAt: outcome.Timestamp,
    }
    ll.episodic.Store(ctx, record)
}

func (ll *LearningLoop) AnalyzeFailures() []Insight {
    // Group failures by pattern
    // Identify common failure modes
    // Suggest improvements
}
```

---

## 14. Storage Layout

```
~/.hermes/
├── config.yaml                   # Main configuration
├── .env                          # API keys (0600 permissions)
├── sessions.db                   # SQLite session store
│
├── memory/
│   ├── episodic/
│   │   ├── store.json            # All episodic records
│   │   └── index.json            # Inverted index
│   ├── semantic/
│   │   ├── store.json            # All semantic facts
│   │   └── index.json            # Inverted index
│   └── working/                  # Session-scoped (not persisted)
│
├── skills/
│   ├── SKILL.md                  # Bundled skills
│   ├── deploy-go-service/
│   │   └── SKILL.md
│   └── auto-generated/
│       └── <slug>/
│           └── SKILL.md
│
├── scheduler/
│   └── jobs.json                 # Cron job definitions
│
├── sessions/
│   └── session_<id>.jsonl        # Full transcript per session
│
├── logs/
│   ├── errors.log                # Rotating error log
│   └── activity.log              # Structured activity log
│
├── learning/
│   └── failures.json             # Rolling failure window
│
└── profiles/                     # Multi-profile support
    └── <name>/
        └── (same structure as above)
```

---

## 15. Extensibility

### Plugin System

```go
// Plugin interface for external tool providers
type Plugin interface {
    Name() string
    Version() string
    Tools() []tool.Tool
    Hooks() map[eventbus.EventType][]eventbus.Handler
    Init(ctx context.Context, config map[string]any) error
    Close() error
}
```

### Plugin Loading

Plugins are loaded from `~/.hermes/plugins/` as compiled Go plugins or as separate processes communicating via stdio (similar to MCP):

```
~/.hermes/plugins/
├── my-plugin/
│   ├── plugin.so      # Go plugin (compiled)
│   └── manifest.yaml  # Plugin metadata
└── remote-tool/
    └── manifest.yaml  # Stdio-based tool server
```

### WASM Sandbox (Future)

Untrusted code execution via WebAssembly:

```go
type WASMExecutor struct {
    engine    *wazero.Runtime
    memoryLimit uint64
    timeout   time.Duration
}

func (we *WASMExecutor) Execute(ctx context.Context, wasm []byte, input []byte) ([]byte, error)
```

### gRPC Remote Tools

```go
// Remote tool implementation via gRPC
type RemoteToolClient struct {
    conn   *grpc.ClientConn
    client pb.ToolServiceClient
}

func (rtc *RemoteToolClient) Execute(ctx context.Context, input tool.ToolInput) (tool.ToolOutput, error)
```

---

## 16. Safety & Security

### Execution Guards

| Guard | Implementation |
|-------|---------------|
| **Tool allowlist** | Only registered tools can be dispatched. No dynamic tool creation from LLM output. |
| **Input validation** | All user input passes through `InputValidator` (length, injection detection, Unicode sanitization). |
| **Output validation** | LLM output checked for exfiltration patterns, secret leakage, and length limits. |
| **Resource limits** | Per-tool timeouts, max output size, max concurrent subagents, max turns. |
| **Sandboxed execution** | Terminal tools can run in Docker containers (future). WASM for untrusted code. |
| **Command approval** | Dangerous patterns require user confirmation (manual/smart/off modes). |
| **Path restrictions** | File tools block sensitive paths (`/etc/`, `/boot/`, etc.). |
| **Rate limiting** | API server rate-limits per IP. LLM calls respect provider rate limits. |

### Trust Levels

```go
type TrustLevel string

const (
    TrustTrusted   TrustLevel = "trusted"    // User-provided, verified
    TrustInferred  TrustLevel = "inferred"   // Agent-derived, moderate confidence
    TrustUntrusted TrustLevel = "untrusted"  // External sources, web content
)
```

---

## 17. LLM Integration

### Provider Interface (Existing, Extended)

```go
// pkg/llm/provider.go — Extended
type Provider interface {
    Chat(ctx context.Context, messages []Message, tools []ToolDefinition, model string) (*Response, error)
    Stream(ctx context.Context, messages []Message, tools []ToolDefinition, model string) (<-chan StreamChunk, error)
    EstimateTokens(messages []Message) int
    MaxContextLength(model string) int
}

type StreamChunk struct {
    Delta       string
    ToolCall    *ToolCall
    FinishReason string
    Done        bool
}
```

### Supported Providers

| Provider | Chat | Stream | Tools | Notes |
|----------|------|--------|-------|-------|
| OpenAI-compatible | ✅ | ✅ | ✅ | Default for most providers |
| Anthropic | ✅ | ✅ | ✅ | Prompt caching support |
| AWS Bedrock | ✅ | ✅ | ✅ | 21+ models cataloged |
| Ollama | ✅ | ✅ | ⚠️ | No native tool calling (prompt-based) |

### Model Routing

```go
// Smart model routing: cheap model for simple tasks, strong model for complex
type ModelRouter struct {
    cheap     llm.Provider
    strong    llm.Provider
    cheapModel  string
    strongModel string
    threshold   int  // Character count threshold
}

func (mr *ModelRouter) Select(query string) (llm.Provider, string) {
    if len(query) <= mr.threshold && !containsComplexIndicators(query) {
        return mr.cheap, mr.cheapModel
    }
    return mr.strong, mr.strongModel
}
```

---

## 18. Multi-Tenancy

### Profile System

Each profile is a fully isolated instance:

```
hermes-go -p work chat     # Uses ~/.hermes/profiles/work/
hermes-go -p personal chat # Uses ~/.hermes/profiles/personal/
```

### Agent Factory for Multi-Tenant API

```go
// internal/agent/factory.go
type AgentFactory struct {
    mu       sync.Mutex
    cache    map[string]*Agent  // tenantID → cached agent
    config   map[string]AgentConfig
    maxCache int
}

func (f *AgentFactory) GetOrCreate(tenantID string) (*Agent, error) {
    f.mu.Lock()
    defer f.mu.Unlock()

    if agent, ok := f.cache[tenantID]; ok {
        return agent, nil
    }

    cfg, ok := f.config[tenantID]
    if !ok {
        return nil, fmt.Errorf("unknown tenant: %s", tenantID)
    }

    agent, err := NewAgent(cfg)
    if err != nil {
        return nil, err
    }

    // Evict oldest if cache full
    if len(f.cache) >= f.maxCache {
        // LRU eviction
    }

    f.cache[tenantID] = agent
    return agent, nil
}
```

---

## 19. Observability

### Structured Logging

```go
// All log entries use structured format
log.Structured("tool.executed",
    "tool", "web_search",
    "duration_ms", 234,
    "success", true,
    "session_id", sessionID,
)
```

### Metrics

| Metric | Type | Labels |
|--------|------|--------|
| `agent_turns_total` | Counter | `session_id`, `source` |
| `tool_calls_total` | Counter | `tool_name`, `success` |
| `tool_duration_seconds` | Histogram | `tool_name` |
| `llm_tokens_total` | Counter | `type` (input/output) |
| `llm_cost_usd_total` | Counter | `provider`, `model` |
| `memory_entries_total` | Gauge | `type` (episodic/semantic) |
| `skills_total` | Gauge | `source` (manual/auto) |
| `subagents_active` | Gauge | — |
| `scheduler_jobs_total` | Gauge | `state` |
| `proactive_suggestions_total` | Counter | `accepted`, `dismissed` |

### Health Endpoint

```
GET /health
{
    "status": "healthy",
    "uptime_seconds": 86400,
    "active_sessions": 3,
    "memory_entries": 1247,
    "skills": 12,
    "scheduler_jobs": 5,
    "subagents_active": 0
}
```

---

## 20. Implementation Roadmap

### Phase 1: Foundation (Week 1-2)

| Task | Files | Priority |
|------|-------|----------|
| Restructure to `cmd/` + `internal/` + `pkg/` layout | All | High |
| Implement event bus | `pkg/eventbus/bus.go`, `events.go` | High |
| Enhance tool interface with context | `pkg/tool/tool.go` | High |
| Integrate memory into agent loop | `internal/agent/context.go` | High |
| Fix existing bugs (Anthropic tool args, UTF-8 truncation) | `llm/anthropic.go`, `security/validator.go` | High |
| Add session resumption | `internal/agent/agent.go` | High |

### Phase 2: Memory & Skills (Week 3-4)

| Task | Files | Priority |
|------|-------|----------|
| Implement episodic memory with indexing | `pkg/memory/store.go`, `indexer.go` | High |
| Implement semantic memory | `pkg/memory/semantic.go` | High |
| Implement working memory | `pkg/memory/working.go` | Medium |
| Implement skill system (load, match, registry) | `pkg/skill/skill.go`, `registry.go` | High |
| Auto-skill generation | `pkg/skill/generator.go` | Medium |
| Integrate memory + skills into system prompt | `internal/agent/context.go` | High |

### Phase 3: Planning & Subagents (Week 5-6)

| Task | Files | Priority |
|------|-------|----------|
| Implement planner with LLM decomposition | `internal/planner/planner.go` | High |
| Implement execution graph | `internal/planner/graph.go` | Medium |
| Implement subagent manager | `internal/agent/subagent.go` | High |
| Parallel tool execution | `pkg/tool/parallel.go` | High |
| Refactor core agent loop | `internal/agent/loop.go` | High |

### Phase 4: Scheduling & Proactive (Week 7-8)

| Task | Files | Priority |
|------|-------|----------|
| Implement cron scheduler | `internal/scheduler/scheduler.go`, `job.go` | High |
| Implement pattern detector | `internal/proactive/patterns.go` | Medium |
| Implement proactive engine | `internal/proactive/engine.go` | Medium |
| Add proactive suggestions to CLI | `internal/cli/cli.go` | Medium |

### Phase 5: Learning & Observability (Week 9-10)

| Task | Files | Priority |
|------|-------|----------|
| Implement learning loop | `internal/learning/loop.go` | High |
| Add structured logging | All packages | Medium |
| Add metrics collection | `internal/metrics/` | Medium |
| Add health endpoint | `internal/api/server.go` | Medium |

### Phase 6: Extensibility & Safety (Week 11-12)

| Task | Files | Priority |
|------|-------|----------|
| Plugin system (stdio-based) | `internal/plugin/` | Medium |
| gRPC remote tools | `pkg/tool/remote.go` | Low |
| WASM sandbox | `pkg/security/sandbox.go` | Low |
| Docker sandbox for terminal tools | `pkg/tool/environments/docker.go` | Low |
| Model routing | `pkg/llm/router.go` | Medium |
| Multi-tenant agent factory | `internal/agent/factory.go` | Medium |

### Phase 7: Polish & Production (Week 13+)

| Task | Priority |
|------|----------|
| Comprehensive test suite | High |
| Streaming support (SSE) | High |
| Context compression via LLM summarization | Medium |
| Provider fallback chains | Medium |
| Token budget enforcement | Medium |
| Prompt caching (Anthropic) | Medium |
| Trajectory saving (JSONL) | Low |
| Gateway (messaging platforms) | Low |
| ACP adapter | Low |

---

## Appendix A: Example Skill File

See `examples/skill.md`

## Appendix B: Example Memory Record

See `examples/memory_record.json`

## Appendix C: Comparison with Python Hermes

| Feature | Python Hermes | Hermes-Go v2 |
|---------|--------------|--------------|
| Core loop | Synchronous | Synchronous (with goroutine parallelism) |
| Memory | File-based + Honcho | 3-tier (working + episodic + semantic) |
| Skills | SKILL.md + auto-gen | SKILL.md + auto-gen + tag-based retrieval |
| Tools | 40+ via registry | Extensible interface + plugin system |
| Subagents | Max depth 2, max 3 concurrent | Configurable depth + concurrency |
| Scheduler | Cron + interval + once | Same + event-triggered |
| Proactive | Limited | Pattern detection + time-based suggestions |
| Learning | Implicit (memory) | Explicit (episodic + skill generation) |
| Multi-tenant | Profile system | Profile + agent factory |
| Messaging | 15+ platforms | CLI + API (gateway future) |
| Streaming | Yes | Planned |
| Observability | Basic logging | Structured logs + metrics + health |
