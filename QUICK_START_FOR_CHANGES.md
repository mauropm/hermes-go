# Quick Start for Changes — Hermes-Go

## Where to Add New Features

### New Tool

1. Open `tools/builtin.go`
2. Add a new `registry.Register(&Tool{...})` block inside `RegisterBuiltinTools()`
3. Follow the existing pattern:
   - `Name`: lowercase with underscores
   - `Description`: clear, concise description for the LLM
   - `Schema`: use `DefaultSchema()` with properties and required fields
   - `Handler`: function that takes `map[string]interface{}` args, returns JSON string via `llm.ToolResultJSON()`
   - `Parallel`: `true` for read-only/safe tools, `false` for stateful tools

### New LLM Provider

1. Create `llm/<provider>.go`
2. Implement a struct with a `Chat(ctx, messages, tools, model) (*llm.Response, error)` method
3. Add a `New<Provider>Provider(cfg ProviderConfig)` constructor
4. Add a case in `llm/provider.go:NewProvider()` switch
5. Follow `llm/openai.go` or `llm/anthropic.go` as templates

### New Slash Command

1. Open `cli/cli.go`
2. Add a case in `handleCommand()` switch
3. Use `fmt.Println()` for output, `fmt.Fprintf(os.Stderr, ...)` for errors

### New API Endpoint

1. Open `api/server.go`
2. Add `mux.HandleFunc("/path", s.handler)` in `NewServer()`
3. Handler should: check method, authenticate, validate input, process, return JSON
4. The middleware already adds security headers and rate limiting

### New Configuration Option

1. Add field to the appropriate config struct in `config/config.go`
2. Set default in `defaultConfig()`
3. Add env override in `applyEnvOverrides()` (if applicable)
4. Add YAML tag for config file support
5. Add validation in `validateConfig()` (if needed)

### New Security Rule

1. Add regex pattern to the appropriate slice in `security/validator.go`:
   - `secretPatterns` — for detecting API keys/secrets
   - `exfilPatterns` — for detecting data exfiltration
   - `injectionPatterns` (inside `DetectPromptInjection`) — for prompt injection
2. Or add a new validation function and call it from `InputValidator.ValidateInput()` or `ValidateOutput()`

## Where to Fix Bugs

### Critical Bug: Anthropic Tool Arguments

**File:** `llm/anthropic.go:209-217`

The tool call `Arguments` field is hardcoded to `"{}"`. The actual arguments are in the `Input` struct but the `Arguments` json tag is `"-"` (ignored). Fix by properly serializing the `Input` field.

### Critical Bug: `/quit` Bypasses Cleanup

**File:** `cli/cli.go:109`

Replace `os.Exit(0)` with `return fmt.Errorf("user quit")` or similar, and let the `Run()` loop exit naturally so deferred cleanup runs.

### Medium Bug: UTF-8 Truncation

**File:** `security/validator.go:69`

Replace `input[:maxLen]` with proper UTF-8-aware truncation using `[]rune` or `utf8.DecodeRune`.

### Medium Bug: API Server Single-Agent Bottleneck

**File:** `api/server.go` + `main.go`

The API server shares one `Agent` instance. For concurrent requests, either:
- Create a new agent per request (expensive but isolated)
- Add a session map keyed by some identifier
- Document that the API is single-threaded

## Patterns to Follow

### Error Handling

- Use `fmt.Errorf("context: %w", err)` for wrapping
- Return errors up the stack; don't swallow them
- In the API server, return generic error messages to clients; log details server-side

### Thread Safety

- Use `sync.Mutex` or `sync.RWMutex` for shared state
- The `Agent` struct already has a mutex — use it
- The `Registry` and `Store` are already thread-safe

### Security

- Always validate input length before processing
- Sanitize Unicode before storing or sending to LLM
- Use `security.ConstantTimeCompare` for secret comparison
- Never log full prompts or API keys
- Use parameterized SQL queries (already done via `?` placeholders)

### Configuration

- New config values should have sensible defaults
- Allow env variable overrides for deployment flexibility
- Validate ranges/limits in `validateConfig()`

## Things to Avoid Breaking

### Do Not Break

| Area | Why |
|------|-----|
| **Provider interface** | `llm/provider.go:Provider` is the core abstraction. Changing it requires updating both OpenAI and Anthropic implementations. |
| **Tool result JSON format** | All tool handlers return JSON via `llm.ToolResultJSON()`. The agent expects this format. |
| **SQLite schema** | The migration system in `storage/session_db.go` is versioned. Changing the schema requires incrementing `currentSchemaVersion` and adding a migration. |
| **Security constants** | `MaxInputLength`, `MaxOutputLength`, etc. in `security/validator.go` are referenced across multiple packages. |
| **Bearer token auth** | The API server's auth mechanism is the only gate. Changing it without updating clients will break API consumers. |
| **Config loading order** | env → .env → YAML → defaults. Changing priority will break existing deployments. |

### Safe to Change

| Area | Notes |
|------|-------|
| Built-in tool implementations | Add/remove tools freely |
| CLI slash commands | Add new commands without affecting existing ones |
| Security regex patterns | Add new patterns; removing existing ones may reduce security |
| Default config values | Adjust as needed, but document changes |
| LLM provider implementations | Each provider is isolated |

## Build & Test Commands

```bash
# Build
go build -o hermes-go .

# Vet (catches common issues)
go vet ./...

# Run
ANTHROPIC_API_KEY=xxx ./hermes-go chat
ANTHROPIC_API_KEY=xxx API_SERVER_KEY=yyy ./hermes-go api

# When tests are added:
go test ./...
go test -race ./...    # Race detector
go test -cover ./...   # Coverage
```
