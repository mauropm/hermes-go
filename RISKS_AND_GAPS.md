# Risks & Gaps — Hermes-Go

## Security Risks

### Medium Risk

| Risk | Location | Description |
|------|----------|-------------|
| **Error message leakage in API** | `api/server.go:202` | Internal error messages (including stack traces) are returned directly to clients via `fmt.Sprintf` in error responses. Could leak implementation details. |
| **Truncate uses byte slicing** | `security/validator.go:69` | `input[:maxLen]` can split multi-byte UTF-8 characters, producing invalid UTF-8. Should use `utf8.DecodeRune` or `[]rune` slicing. |
| **Memory store not encrypted** | `memory/store.go:210-227` | Memory is persisted as plaintext JSON (`store.json`). If disk is compromised, all memory entries are readable. |
| **SQLite not encrypted** | `storage/session_db.go` | Session data (full conversation history) stored in plaintext. File permissions are `0600` but no encryption at rest. |
| **API key in process memory** | `config/config.go:224-238` | API keys are loaded into a `map[string]string` and kept in memory for the process lifetime. A memory dump could expose them. |
| **No TLS for API server** | `api/server.go:241` | `ListenAndServe()` is used (not `ListenAndServeTLS`). The API server transmits bearer tokens and conversation data in plaintext over the network. Only safe when bound to `127.0.0.1`. |

### Low Risk

| Risk | Location | Description |
|------|----------|-------------|
| **Rate limit state unbounded** | `api/server.go:132-154` | The `rateLimits` map grows indefinitely — entries are never cleaned up. Over long-running processes, this could cause memory growth. |
| **Scanner buffer limit** | `cli/cli.go:28` | `bufio.Scanner` has a default max line length of 64KB. Inputs longer than this will silently fail. Should use `scanner.Buffer()` to increase or handle the error. |
| **rand.Int63n for retry jitter** | `storage/session_db.go:195` | Uses `math/rand` (not `crypto/rand`) for retry delay jitter. Not a security issue in this context, but worth noting. |
| **Anthropic tool arguments hardcoded** | `llm/anthropic.go:213` | Tool call arguments are hardcoded to `"{}"` instead of being extracted from the response's `Input` field. Tool calls from Anthropic will always receive empty arguments. **This is a functional bug.** |
| **Memory persistence not atomic on crash** | `memory/store.go:210-227` | Uses tmp+rename pattern (good), but `store.json` is written on every `Store()` call. High-frequency writes could cause wear on SSDs and performance degradation. |

## Missing Features

| Feature | Impact | Notes |
|---------|--------|-------|
| **No tests** | Critical | Zero test files exist. No unit, integration, or e2e tests. |
| **No streaming support** | Medium | API server only supports non-streaming responses. The Python original supports SSE streaming. |
| **No conversation history loading** | Medium | The agent does not load prior conversation history from SQLite when resuming. Each session starts fresh. |
| **No memory integration in agent loop** | Medium | `memStore` is passed to the agent but never actually used in the `Chat()` or `runLoop()` methods. Memory entries are never retrieved or injected into the conversation. |
| **No session resumption** | Low | No `/resume` command or API parameter to continue a prior session. |
| **No metrics/observability** | Low | No Prometheus, structured logging, or request metrics. |
| **No CI/CD pipeline** | Low | No `.github/workflows/` or Makefile. |
| **No `.gitignore`** | Low | The compiled binary `hermes-go` is tracked in the repo. |
| **No graceful CLI cleanup** | Low | On `/quit`, `os.Exit(0)` is called directly, bypassing deferred cleanup in `main.go`. |

## Technical Debt

### Code Quality

| Issue | Location | Description |
|-------|----------|-------------|
| **`os.Exit(0)` in CLI** | `cli/cli.go:109` | Called inside `handleCommand`, which bypasses all deferred cleanup (session save, DB close). Should return an error and let `Run()` exit cleanly. |
| **Error wrapping inconsistency** | Multiple files | Some errors use `fmt.Errorf("...: %w", err)`, others use `fmt.Errorf("...: %v", err)`. The API server uses `%v` in JSON error responses, which could leak internal details. |
| **Hardcoded constants** | `core/agent.go:19` | `maxContextMessages = 50` and `recentCount = 10` are hardcoded. Should be configurable. |
| **Magic number in token estimation** | `api/server.go:223-224` | `len(response) / 4` is a rough token estimate. Should use a proper tokenizer or at least document the assumption. |
| **Unused `source` field** | `core/agent.go` | The `source` field is set but never used in the agent logic. |
| **Unused `startedAt`, `totalTokens`, `totalCost`** | `core/agent.go` | These fields are set but never read or persisted. |
| **`BuildSystemPrompt` appends to immutable prompt** | `core/agent.go:258-266` | The function concatenates memory context to the system prompt, but it's never called in the actual agent flow. The agent always uses the hardcoded `defaultSystemPrompt`. |
| **`SanitizeMessages` is unused** | `core/agent.go:268-276` | Exported function that is never called anywhere in the codebase. |

### Architecture

| Issue | Description |
|-------|-------------|
| **Single-agent API server** | The API server shares a single `Agent` instance across all requests. Since `Agent.Chat()` holds a mutex, concurrent API requests will block each other. This is a scalability bottleneck. |
| **No session management in API** | The API server does not create separate sessions per request. All API calls go through the same agent with the same message history. |
| **Memory store is unused** | The memory store is initialized in `main.go` but never integrated into the agent's conversation flow. |
| **No toolset filtering** | The Python version supports toolsets (named groups of tools). The Go version registers all tools without filtering. |

### Functional Bugs

| Bug | Location | Description |
|-----|----------|-------------|
| **Anthropic tool arguments lost** | `llm/anthropic.go:209-217` | When parsing Anthropic tool_use responses, the `Arguments` field is hardcoded to `"{}"` instead of extracting the actual `input` from the response. All tool calls to Anthropic models will receive empty arguments. |
| **`/quit` bypasses cleanup** | `cli/cli.go:109` | `os.Exit(0)` prevents `main.go` deferred `SaveSession()` and `EndSession()` from running. |
| **Context compression loses messages** | `core/agent.go:192-206` | The compression keeps system message + last 10 messages, but the math is off: `recentCount` can exceed available messages, and tool call/result pairs may be split. |
