# Codebase Map — Hermes-Go

## Directory Structure

```
hermes-go/
├── main.go                    # Entry point — flag parsing, config loading, command dispatch
├── go.mod                     # Module definition, 4 dependencies
├── go.sum                     # Dependency checksums
├── hermes-go                  # Compiled binary (git-ignored)
│
├── config/
│   └── config.go              # Configuration system (334 lines)
│                              # - Load(): env → .env → YAML → defaults pipeline
│                              # - resolveHomeDir(): profile isolation, symlink safety
│                              # - loadEnvFile(): .env loading with permission enforcement
│                              # - loadAPIKeys(): collects all *_API_KEY env vars
│                              # - applyEnvOverrides(): HERMES_MAX_ITERATIONS, API_SERVER_*
│                              # - validateConfig(): bounds checking
│                              # - GetAPIKey(): provider → env var name mapping
│                              # - EnsureDirs(): creates ~/.hermes/{logs,sessions,memory}
│
├── core/
│   └── agent.go               # Core agent orchestrator (276 lines)
│                              # - NewAgent(): creates LLM provider, initializes messages
│                              # - Chat(): validates input, runs conversation loop
│                              # - runLoop(): turn-based LLM calls with tool dispatch
│                              # - compressContext(): keeps system msg + last 10 messages
│                              # - SaveSession(): persists messages to SQLite
│                              # - Interrupt(): thread-safe cancellation
│
├── cli/
│   └── cli.go                 # Interactive REPL (135 lines)
│                              # - Run(): main loop with signal handling
│                              # - handleCommand(): slash command dispatcher
│                              # Commands: /quit, /exit, /help, /session, /tools, /clear
│
├── api/
│   └── server.go              # HTTP API server (247 lines)
│                              # - NewServer(): sets up routes, middleware, http.Server
│                              # - withMiddleware(): security headers, rate limiting
│                              # - authenticate(): Bearer token with constant-time compare
│                              # - checkRateLimit(): per-IP sliding window (100 req/min)
│                              # - handleChat(): POST /v1/chat/completions
│                              # - handleHealth(): GET /health
│                              # - Start()/Shutdown(): lifecycle management
│
├── llm/
│   ├── provider.go            # Provider interface + factory (112 lines)
│   │                          # - Provider interface: Chat(ctx, messages, tools, model)
│   │                          # - NewProvider(): factory with auto-detection
│   │                          # - ParseModel(): splits "provider/model" format
│   │                          # - ToolResultJSON(): standard tool result format
│   │                          # Types: Message, ToolCall, ToolDefinition, Response, Usage
│   │
│   ├── openai.go              # OpenAI-compatible client (147 lines)
│   │                          # - HTTP POST to {baseURL}/v1/chat/completions
│   │                          # - Bearer token auth
│   │                          # - Response body limited to 1MB
│   │
│   └── anthropic.go           # Anthropic Messages API client (221 lines)
│                              # - HTTP POST to {baseURL}/v1/messages
│                              # - x-api-key + anthropic-version headers
│                              # - Converts OpenAI-format messages to Anthropic format
│                              # - Handles tool_use and tool_result content blocks
│
├── tools/
│   ├── registry.go            # Tool registry (113 lines)
│   │                          # - Registry: thread-safe map of name → Tool
│   │                          # - Register(): validates name, handler uniqueness
│   │                          # - Dispatch(): JSON arg parsing, handler invocation
│   │                          # - GetDefinitions(): converts to LLM tool schemas
│   │                          # - DefaultSchema(): helper for OpenAI function schema
│   │
│   └── builtin.go             # Built-in tool implementations (225 lines)
│                              # - get_time: returns current UTC time
│                              # - calculator: safe arithmetic parser (no eval!)
│                              #   - Recursive descent: expr → term → factor → number
│                              #   - Whitelist: only digits, +, -, *, /, (), .
│                              #   - Division by zero check
│                              #   - Max 1000 chars
│                              # - help: lists available tools
│
├── memory/
│   └── store.go               # Local memory store (261 lines)
│                              # - Store(): write with sanitization, dedup, TTL
│                              # - Retrieve(): read with TTL check
│                              # - RetrieveAll(): get all non-expired entries
│                              # - evictOldestLocked(): LRU-style eviction
│                              # - persist(): atomic write (tmp + rename)
│                              # - load(): read from store.json on startup
│                              # - FormatMemoryContext(): formats entries for LLM context
│                              # Constants: 4KB/entry, 10MB total, 50 entries/key, 30d TTL
│
├── storage/
│   └── session_db.go          # SQLite session store (383 lines)
│                              # - NewSessionDB(): opens DB with WAL mode, runs migrations
│                              # - migrate(): versioned schema migration (v0 → v1)
│                              # - retry(): application-level retry for "database is locked"
│                              # - CreateSession(), EndSession(), UpdateSessionMetrics()
│                              # - AddMessage(), GetMessages()
│                              # - SearchMessages(): FTS5 full-text search
│                              # - IncrementToolCalls()
│                              # Schema: sessions table, messages table, messages_fts virtual table
│
├── security/
│   └── validator.go           # Security utilities (210 lines)
│                              # - SanitizeUnicode(): strips control chars, invalid surrogates
│                              # - Truncate(): byte-level truncation
│                              # - ValidateLength(): length limit check
│                              # - ContainsSecrets(): regex scan for API key patterns
│                              # - RedactSecrets(): replaces secrets with [REDACTED]
│                              # - DetectExfiltration(): blocks curl/wget, base64, /etc/passwd
│                              # - DetectPromptInjection(): blocks "ignore previous", "you are now"
│                              # - IsSensitivePath(): blocks /etc/, /proc/, /sys/, etc.
│                              # - HasPathTraversal(): detects .. in paths
│                              # - ConstantTimeCompare(): timing-safe string comparison
│                              # - InputValidator: struct with ValidateInput/ValidateOutput
│
└── tools/                     # (see above)
```

## File Sizes

| File | Lines | Purpose |
|------|-------|---------|
| `main.go` | 198 | Entry point |
| `config/config.go` | 334 | Configuration |
| `core/agent.go` | 276 | Agent orchestrator |
| `cli/cli.go` | 135 | CLI REPL |
| `api/server.go` | 247 | HTTP API |
| `llm/provider.go` | 112 | Provider interface |
| `llm/openai.go` | 147 | OpenAI client |
| `llm/anthropic.go` | 221 | Anthropic client |
| `tools/registry.go` | 113 | Tool registry |
| `tools/builtin.go` | 225 | Built-in tools |
| `memory/store.go` | 261 | Memory store |
| `storage/session_db.go` | 383 | SQLite storage |
| `security/validator.go` | 210 | Security layer |
| **Total** | **~2,862** | |

## Key Types & Interfaces

| Type | Location | Purpose |
|------|----------|---------|
| `Config` | `config/config.go` | All configuration values |
| `Agent` | `core/agent.go` | Conversation orchestrator |
| `Provider` (interface) | `llm/provider.go` | LLM provider abstraction |
| `OpenAICompatibleProvider` | `llm/openai.go` | OpenAI-compatible HTTP client |
| `AnthropicProvider` | `llm/anthropic.go` | Anthropic HTTP client |
| `Registry` | `tools/registry.go` | Tool registration and dispatch |
| `Tool` | `tools/registry.go` | Tool metadata + handler |
| `Store` | `memory/store.go` | Local memory key-value store |
| `SessionDB` | `storage/session_db.go` | SQLite session/message storage |
| `InputValidator` | `security/validator.go` | Input/output validation |
| `Server` | `api/server.go` | HTTP API server |
| `CLI` | `cli/cli.go` | Interactive terminal interface |
