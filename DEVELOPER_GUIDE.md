# Developer Guide — Hermes-Go

## Prerequisites

- **Go 1.22+**
- **CGO enabled** (required for `go-sqlite3`)
- On macOS: `brew install sqlite3` (usually pre-installed)
- On Linux: `libsqlite3-dev` package

## How to Build

```bash
# Standard build
go build -o hermes-go .

# Static build (requires musl-gcc)
CGO_ENABLED=1 go build -ldflags="-s -w" -o hermes-go .
```

## How to Run

### Interactive CLI

```bash
# With Anthropic
ANTHROPIC_API_KEY=sk-ant-... ./hermes-go chat

# With OpenAI
OPENAI_API_KEY=sk-... ./hermes-go chat

# With a named profile (isolated config/sessions/memory)
ANTHROPIC_API_KEY=sk-ant-... ./hermes-go chat -p work
```

### API Server

```bash
ANTHROPIC_API_KEY=sk-ant-... API_SERVER_KEY=my-secret-token ./hermes-go api
```

Test it:

```bash
curl -X POST http://127.0.0.1:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer my-secret-token" \
  -d '{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"Hello"}]}'
```

Health check: `curl http://127.0.0.1:8080/health`

## How to Test

**No test files currently exist in the repository.** This is a significant gap. To add tests:

```bash
# Standard Go test
go test ./...

# With coverage
go test -cover ./...

# Verbose
go test -v ./...
```

## Key Areas of the Codebase

### Entry Point

- `main.go` — Parses flags (`-p`, `-cmd`, `-version`), loads config, dispatches to `runChat()` or `runAPI()`

### Configuration

- `config/config.go` — Multi-source config loading (env → `.env` → YAML → defaults), profile resolution, API key management, directory creation

### Core Agent

- `core/agent.go` — `Agent` struct with `Chat()` method, `runLoop()` for conversation turns, `compressContext()`, `SaveSession()`, `Interrupt()`

### LLM Providers

- `llm/provider.go` — `Provider` interface, factory `NewProvider()`, model parsing, shared types (`Message`, `ToolCall`, `Response`)
- `llm/openai.go` — OpenAI-compatible HTTP client
- `llm/anthropic.go` — Anthropic Messages API client with message format conversion

### Tools

- `tools/registry.go` — `Registry` with `Register()`, `Dispatch()`, `GetDefinitions()`
- `tools/builtin.go` — Built-in tools: `get_time`, `calculator` (with safe arithmetic parser), `help`

### Storage

- `storage/session_db.go` — SQLite wrapper: schema migrations, session/message CRUD, FTS5 search, write retry with jitter

### Memory

- `memory/store.go` — JSON-backed key-value store: `Store()`, `Retrieve()`, TTL, dedup, eviction, atomic persistence

### Security

- `security/validator.go` — Input/output validation, prompt injection detection, exfiltration patterns, secret redaction, Unicode sanitization, path traversal checks, constant-time comparison

### CLI

- `cli/cli.go` — REPL loop with `bufio.Scanner`, signal handling (SIGINT/SIGTERM), slash commands (`/quit`, `/help`, `/session`, `/tools`, `/clear`)

### API Server

- `api/server.go` — HTTP server with middleware (security headers, rate limiting), bearer token auth, OpenAI-compatible request/response format, unknown field rejection

## Common Workflows

### Adding a New Tool

1. Define the tool in `tools/builtin.go` (or create a new file)
2. Create a `Tool` struct with `Name`, `Description`, `Schema`, `Handler`, `Parallel`
3. Register it via `registry.Register()` in `RegisterBuiltinTools()`
4. The tool will automatically appear in `GetDefinitions()` and be available to the LLM

### Adding a New LLM Provider

1. Create a new file in `llm/` (e.g., `llm/groq.go`)
2. Implement the `Provider` interface: `Chat(ctx, messages, tools, model) (*Response, error)`
3. Add a case in `llm/provider.go:NewProvider()` switch statement

### Adding a New Slash Command

1. Add a case in `cli/cli.go:handleCommand()` switch
2. Follow existing patterns for output formatting

### Modifying Security Rules

1. Edit regex patterns in `security/validator.go`
2. Adjust constants (`MaxInputLength`, `MaxOutputLength`, etc.)
3. Update `InputValidator` methods as needed

### Changing Configuration Defaults

1. Edit `config/config.go:defaultConfig()`
2. Add new env override in `applyEnvOverrides()`
3. Add validation in `validateConfig()`
