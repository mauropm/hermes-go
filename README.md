# Hermes-Go

A secure, hardened Go reimagination of the Hermes AI agent framework. Built for deployment on cheap VPS instances with minimal dependencies and maximum security guarantees.

## Quick Start

```bash
# Build
make build

# First-time setup wizard
./hermes-go setup

# Run interactive CLI
ANTHROPIC_API_KEY=your-key ./hermes-go chat

# Run API server
ANTHROPIC_API_KEY=your-key API_SERVER_KEY=your-api-token ./hermes-go api
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        INTERFACE LAYER                           │
│  ┌──────────────────┐  ┌─────────────┐  ┌────────────────────┐  │
│  │   CLI (REPL)     │  │ Setup TUI   │  │ API Server (HTTP)  │  │
│  │   cli/cli.go     │  │ config_tui  │  │ api/server.go      │  │
│  └────────┬─────────┘  └─────────────┘  └────────┬───────────┘  │
├───────────┼───────────────────────────────────────┼─────────────┤
│                    ORCHESTRATION LAYER                             │
│  ┌──────────────────────────────────────────────────────────┐    │
│  │                    AIAgent                                │    │
│  │              core/agent.go                                │    │
│  │   Conversation loop │ Tool dispatch │ Context compression │    │
│  └──────────────────────────┬───────────────────────────────┘    │
├─────────────────────────────┼───────────────────────────────────┤
│                        TOOL LAYER                                 │
│  ┌──────────────────────────────────────────────────────────┐    │
│  │              ToolRegistry (tools/)                        │    │
│  │   Built-in tools only: get_time, calculator, help         │    │
│  │   Dangerous tools DISABLED by default                     │    │
│  └──────────────────────────┬───────────────────────────────┘    │
├─────────────────────────────┼───────────────────────────────────┤
│                   INFRASTRUCTURE LAYER                            │
│  ┌──────────┐ ┌──────────┐ ┌──────────────────┐ ┌────────────┐ │
│  │ SQLite   │ │ Memory   │ │ LLM Providers    │ │ Security   │ │
│  │ storage/ │ │ memory/  │ │ llm/             │ │ security/  │ │
│  │          │ │          │ │ • OpenAI         │ │            │ │
│  │          │ │          │ │ • Anthropic      │ │            │ │
│  │          │ │          │ │ • AWS Bedrock    │ │            │ │
│  └──────────┘ └──────────┘ └──────────────────┘ └────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

### Module Structure

| Package | Purpose |
|---------|---------|
| `config/` | Configuration loading from env, `.env`, YAML. Secrets management. Save & setters. |
| `security/` | Input validation, sanitization, LLM safety filters, secret redaction. |
| `storage/` | SQLite session store with WAL mode, migrations, FTS5 search. |
| `memory/` | Secure local memory with sanitization, TTL, deduplication, size limits. |
| `llm/` | Provider interface. OpenAI-compatible, Anthropic, and AWS Bedrock implementations. |
| `core/` | Agent orchestrator: conversation loop, tool dispatch, context compression, runtime model switching. |
| `tools/` | Tool registry. Only safe, read-only tools enabled by default. |
| `api/` | HTTP API server with auth, rate limiting, request validation. |
| `cli/` | Interactive REPL with slash commands, config TUI, and signal handling. |

## Security Model

### Zero Trust Input

- All inputs treated as untrusted (CLI, API, LLM responses, files)
- Strict length limits (100K input, 64K output, 4K memory entries)
- Unicode sanitization (control characters, invalid surrogates stripped)
- Prompt injection detection with pattern matching
- Unknown JSON fields rejected in API requests

### LLM Safety Layer

- **Immutable system prompt**: Cannot be overridden by user input
- **Output filtering**: Scans for exfiltration patterns, command injection, secret patterns
- **Context isolation**: Memory entries injected as user messages, never system instructions
- **Tool restrictions**: Only safe, read-only tools enabled. No terminal, file, or browser access.

### Memory Protection

- Write path: sanitize → redact secrets → deduplicate → store with TTL
- Read path: TTL check → re-scan for injections → wrap in trust markers
- Size limits: 4KB per entry, 10MB total store, 50 entries per key
- Default TTL: 30 days

### Secrets Management

- Loaded from environment variables only (never hardcoded)
- `.env` file permissions enforced to `0600`
- Secrets redacted in all logs
- Full prompts never logged
- API key patterns masked in output

### Network Security

- HTTPS only for all LLM calls
- TLS verification cannot be disabled
- Hard timeouts (60s default)
- No dynamic URLs from user input (SSRF prevention)
- API server: bearer token auth, rate limiting (100 req/min), 1MB request limit

### File System Safety

- SQLite database and memory store permissions: `0600`
- HERMES_HOME directories: `0700`
- No arbitrary file reads or writes
- Sensitive paths blocked: `/etc/`, `/proc/`, `/sys/`, `/boot/`, docker socket

## Threat Model

See [security.md](security.md) for the complete threat analysis including:

- STRIDE analysis (Spoofing, Tampering, Repudiation, Information Disclosure, DoS, Elevation of Privilege)
- Trust boundary definitions
- Attack vectors and mitigations
- Known limitations and deliberately excluded features

### Key Mitigations

| Threat | Mitigation |
|--------|-----------|
| Prompt injection | Pattern detection + immutable system prompt + tool restrictions |
| Credential exfiltration | Output filtering + secrets never in context + redaction |
| Memory poisoning | Sanitization on write/read + trust tagging + TTL |
| Arbitrary code execution | Dangerous tools disabled by default |
| DoS (token exhaustion) | Hard iteration limits + token budget tracking |
| API abuse | Bearer token auth + rate limiting + request size limits |
| Path traversal | Strict validation + allowlisted directories only |

## Configuration

See [setup.md](setup.md) for the complete setup guide.

### Quick Config

```bash
# Interactive wizard
./hermes-go setup

# Or from inside the chat
/config
```

### Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `HERMES_HOME` | Override home directory | `~/.hermes` |
| `HERMES_MAX_ITERATIONS` | Max conversation turns | 90 |
| `ANTHROPIC_API_KEY` | Anthropic API key | (required) |
| `OPENAI_API_KEY` | OpenAI API key | (required if using OpenAI) |
| `AWS_ACCESS_KEY_ID` | AWS access key (Bedrock) | (from ~/.aws/credentials) |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key (Bedrock) | (from ~/.aws/credentials) |
| `API_SERVER_KEY` | API server bearer token | (required for API mode) |
| `API_SERVER_ENABLED` | Enable API server | false |
| `API_SERVER_PORT` | API server port | 8080 |
| `API_SERVER_HOST` | API server host | 127.0.0.1 |

### Configuration Files

Priority order (highest to lowest):
1. Environment variables
2. `~/.hermes/.env` (dotenv format)
3. `~/.hermes/config.yaml` (YAML config)
4. Built-in defaults

## Usage

### Interactive CLI

```bash
# With Anthropic
ANTHROPIC_API_KEY=sk-ant-... ./hermes-go chat

# With OpenAI
OPENAI_API_KEY=sk-... ./hermes-go chat

# With AWS Bedrock (uses AWS credentials)
./hermes-go chat

# With profile
ANTHROPIC_API_KEY=sk-ant-... ./hermes-go chat -p myprofile
```

**Slash commands:**
- `/quit` or `/exit` — Exit
- `/help` — Show commands
- `/session` — Show session ID
- `/tools` — List available tools
- `/models` — List Bedrock models with pricing
- `/models use <n>` — Switch to Bedrock model #n
- `/config` — Open configuration editor
- `/clear` — Clear screen

### Setup Wizard

```bash
# Run the interactive configuration wizard
./hermes-go setup
```

The wizard displays all current settings and lets you edit each one. Changes are saved atomically to `~/.hermes/config.yaml`.

### AWS Bedrock

```bash
# List available models with pricing
/models

# Switch to a model
/models use 1        # Claude Sonnet 4
/models use 14       # Nova Micro (free tier)
```

Bedrock uses your AWS credentials (from `~/.aws/credentials`, environment variables, or IAM roles). No API key needed. Configure the region via `/config` or `~/.hermes/config.yaml`:

```yaml
bedrock:
  region: us-east-1
  profile: ""
```

See [setup.md](setup.md) for the full model catalog with pricing.

### API Server

```bash
ANTHROPIC_API_KEY=sk-ant-... API_SERVER_KEY=my-secret-token ./hermes-go api
```

The API server exposes an OpenAI-compatible endpoint at `http://127.0.0.1:8080/v1/chat/completions`.

```bash
curl -X POST http://127.0.0.1:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer my-secret-token" \
  -d '{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"Hello"}]}'
```

Health check: `http://127.0.0.1:8080/health`

### Profiles

```bash
# Create isolated profile
ANTHROPIC_API_KEY=sk-ant-... ./hermes-go chat -p work

# Each profile has separate:
# - Configuration
# - API keys
# - Sessions
# - Memory store
```

## Dependencies

| Dependency | Justification |
|-----------|--------------|
| `github.com/mattn/go-sqlite3` | SQLite for session storage and FTS5 search. CGO required. |
| `gopkg.in/yaml.v3` | YAML config parsing. Minimal, well-audited. |
| `github.com/joho/godotenv` | `.env` file loading. Minimal, widely used. |
| `github.com/google/uuid` | UUID generation for session IDs. |
| `github.com/aws/aws-sdk-go-v2` | AWS SDK for Bedrock provider (config, credentials, bedrockruntime). |

All other functionality uses the Go standard library.

## Building

```bash
# Using Make (recommended — includes vulnerability checks)
make build

# Manual build (requires Go 1.22+ and CGO for SQLite)
go build -o hermes-go .

# Static build (requires musl-gcc)
CGO_ENABLED=1 go build -ldflags="-s -w" -o hermes-go .

# Security audit before building
make audit
```

## Deliberately Excluded Features

The following capabilities from the Python original are **not implemented** due to security risk:

- Terminal command execution
- Browser automation
- MCP server connections
- Dynamic plugin loading
- Docker/SSH/Modal execution environments
- 15+ messaging platform adapters

These can be added later with appropriate security controls, but the baseline ships with a minimal, auditable surface area.

## License

MIT
