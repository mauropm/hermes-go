# System Overview — Hermes-Go

## Purpose

Hermes-Go is a **secure, hardened Go reimagination** of the Hermes AI agent framework (originally Python). It provides a minimal, auditable AI assistant that wraps LLMs (OpenAI-compatible and Anthropic) with tool-calling, persistent session storage, and a local memory system. It is designed for deployment on cheap VPS instances with minimal dependencies and maximum security guarantees.

## Core Features

| Feature | Description |
|---------|-------------|
| **Interactive CLI** | REPL with slash commands, signal handling, session management |
| **API Server** | OpenAI-compatible HTTP endpoint (`/v1/chat/completions`) with auth, rate limiting |
| **Conversation Loop** | Synchronous turn-based LLM interaction with tool dispatch and context compression |
| **Tool System** | Registry-driven, allowlisted tools only (get_time, calculator, help) |
| **Session Storage** | SQLite with WAL mode, FTS5 full-text search, migrations |
| **Local Memory** | JSON-backed key-value store with TTL, dedup, trust tagging, size limits |
| **Security Layer** | Input/output validation, prompt injection detection, secret redaction, Unicode sanitization |

## Components & Interactions

```
┌──────────────────────────────────────────────────────────────┐
│                     INTERFACE LAYER                           │
│  ┌────────────────┐            ┌──────────────────────────┐   │
│  │  CLI (REPL)    │            │  API Server (HTTP)       │   │
│  │  cli/cli.go    │            │  api/server.go           │   │
│  └───────┬────────┘            └───────────┬──────────────┘   │
├──────────┼─────────────────────────────────┼──────────────────┤
│                  ORCHESTRATION LAYER                            │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │                     AIAgent                              │  │
│  │   core/agent.go                                          │  │
│  │   Conversation loop │ Tool dispatch │ Context compression │  │
│  └──────────────────────────┬──────────────────────────────┘  │
├─────────────────────────────┼─────────────────────────────────┤
│                       TOOL LAYER                               │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │              ToolRegistry (tools/)                       │  │
│  │   Built-in tools: get_time, calculator, help             │  │
│  └──────────────────────────┬──────────────────────────────┘  │
├─────────────────────────────┼─────────────────────────────────┤
│                  INFRASTRUCTURE LAYER                          │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────────┐ │
│  │ SQLite   │ │ Memory   │ │ LLM      │ │ Security Layer   │ │
│  │ storage/ │ │ memory/  │ │ llm/     │ │ security/        │ │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

## Data Flow

### CLI Request Lifecycle

```
User types message
  → CLI validates length
  → Agent.Chat(userInput)
    → InputValidator: length + injection check
    → SanitizeUnicode + Truncate
    → Append to messages slice
    → Compress if > 50 messages
    → runLoop():
      → LLM Provider.Chat(messages, tools)
        → OpenAI or Anthropic HTTP call
      → If tool_calls:
        → ToolRegistry.Dispatch(name, args)
        → Append tool result to messages
        → Continue loop
      → If final response:
        → OutputValidator: length + exfil check
        → Return to user
  → Save messages to SQLite
```

### API Request Lifecycle

```
HTTP POST /v1/chat/completions
  → Middleware: security headers, rate limit check
  → Auth: Bearer token (constant-time compare)
  → Parse JSON (DisallowUnknownFields)
  → Validate input (length + injection)
  → Agent.Chat(userInput)
    → Same flow as CLI
  → Build OpenAI-compatible response JSON
  → Return to client
```

## External Dependencies

| Dependency | Purpose |
|------------|---------|
| `github.com/mattn/go-sqlite3` | SQLite session storage (CGO required) |
| `gopkg.in/yaml.v3` | YAML config parsing |
| `github.com/joho/godotenv` | `.env` file loading |
| `github.com/google/uuid` | UUID generation for session IDs |

All other functionality uses only the Go standard library.

## LLM Providers

- **Anthropic** — Direct Messages API (`api.anthropic.com`)
- **OpenAI-compatible** — Works with OpenAI, OpenRouter, GLM, Kimi, MiniMax, HuggingFace, etc.

Provider is auto-detected from model name prefix (e.g., `anthropic/claude-...` → Anthropic, everything else → OpenAI-compatible).
