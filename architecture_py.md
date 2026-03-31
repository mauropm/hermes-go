# System Architecture

## Overview

Hermes Agent (v0.6.0) is a multi-platform AI agent framework by Nous Research. It provides a tool-calling agent that can run as an interactive CLI, a messaging bot across 15+ platforms, an OpenAI-compatible API server, and an editor plugin (ACP). The system features a synchronous conversation loop with parallel tool execution, persistent session storage with full-text search, a plugin/skill ecosystem, scheduled cron jobs, and parallel batch processing for training data generation.

## Architecture Style

**Monolithic modular application** with layered architecture and plugin-based extensibility.

- **Layered**: Clear separation between UI layer (CLI/Gateway/ACP), orchestration layer (AIAgent), tool layer (registry + handlers), and infrastructure layer (environments, storage, external APIs).
- **Registry-driven**: Tools, slash commands, platform adapters, and skins are all discovered and registered at import time via central registries.
- **Adapter-based**: Terminal execution and messaging platforms are abstracted behind interfaces with swappable implementations.
- **Event-driven (gateway)**: The gateway uses async message handling, streaming token delivery, and a hook system for extensibility.

## Components

### Core Agent (`run_agent.py`)

| Aspect | Detail |
|---|---|
| **Responsibility** | Central orchestrator — manages the LLM conversation loop, tool call dispatch, subagent delegation, context compression, and budget tracking |
| **Key class** | `AIAgent` |
| **Key interactions** | Calls LLM providers (OpenAI/Anthropic APIs), invokes `handle_function_call()` for tool dispatch, spawns child agents via `delegate_tool`, compresses context when approaching limits |
| **Behavior** | Synchronous loop; max 90 iterations by default; supports interrupt, parallel tool execution (up to 8 workers), and trajectory saving |

### Tool System (`tools/`, `model_tools.py`, `toolsets.py`)

| Aspect | Detail |
|---|---|
| **Responsibility** | Tool registration, schema collection, availability checking, and dispatch |
| **Layers** | 1. `tools/registry.py` — `ToolRegistry` singleton (register/dispatch). 2. `model_tools.py` — orchestration (discover, filter, bridge async). 3. `toolsets.py` — named toolset definitions with composition |
| **Key interactions** | Tool modules call `registry.register()` at import time. `get_tool_definitions()` filters by active toolset and returns OpenAI-format schemas. `handle_function_call()` dispatches to registry |
| **Tool count** | 40+ tools across web, terminal, file, browser, vision, memory, code, communication, and integration categories |

### CLI (`cli.py`, `hermes_cli/`)

| Aspect | Detail |
|---|---|
| **Responsibility** | Interactive terminal interface with Rich TUI, slash commands, session management, and configuration |
| **Key class** | `HermesCLI` |
| **Key interactions** | Instantiates `AIAgent`, renders output via Rich panels and KawaiiSpinner, dispatches slash commands via `COMMAND_REGISTRY`, persists config to `~/.hermes/config.yaml` |
| **Input** | `prompt_toolkit` REPL with autocomplete |

### Gateway (`gateway/`)

| Aspect | Detail |
|---|---|
| **Responsibility** | Multi-platform messaging controller — connects to 15+ platforms, routes messages to agents, delivers responses |
| **Key class** | `GatewayRunner` |
| **Key interactions** | Creates `AIAgent` instances per session (cached for prompt caching), routes via `DeliveryRouter`, manages `SessionStore` with reset policies, runs background cron scheduler and session expiry watcher |
| **Platform adapters** | Telegram, Discord, WhatsApp, Slack, Signal, Mattermost, Matrix, Home Assistant, Email, SMS, DingTalk, Feishu, WeCom, API Server (FastAPI), Webhook |

### State Storage (`hermes_state.py`)

| Aspect | Detail |
|---|---|
| **Responsibility** | SQLite-backed session and message persistence with full-text search |
| **Key class** | `SessionDB` |
| **Schema** | `sessions` table + `messages` table + FTS5 virtual table |
| **Features** | WAL mode, versioned migrations (v6), application-level write contention retry, FTS5 search, session export/prune |

### Cron System (`cron/`)

| Aspect | Detail |
|---|---|
| **Responsibility** | Scheduled task execution via cron expressions |
| **Components** | `jobs.py` (CRUD, due-job detection via croniter), `scheduler.py` (tick every 60s, file-based locking) |
| **Delivery** | Outputs to origin chat, home channels, or local files |

### Batch Runner (`batch_runner.py`)

| Aspect | Detail |
|---|---|
| **Responsibility** | Parallel data generation for training — processes prompts through the agent with varied toolsets |
| **Features** | Multiprocessing pool, checkpoint-based fault tolerance, JSONL trajectory output, tool usage statistics |

### ACP Adapter (`acp_adapter/`)

| Aspect | Detail |
|---|---|
| **Responsibility** | Agent Communication Protocol server for IDE integration (VS Code, Zed, JetBrains) |
| **Components** | Session management, tool definitions, permission system, authentication |

### Agent Internals (`agent/`)

| Module | Responsibility |
|---|---|
| `prompt_builder.py` | System prompt assembly, context file scanning, prompt injection detection |
| `context_compressor.py` | Auto context compression when approaching model limits |
| `prompt_caching.py` | Anthropic prompt cache control headers |
| `auxiliary_client.py` | Secondary LLM client for vision, summarization, provider routing |
| `model_metadata.py` | Model context lengths, token estimation, pricing |
| `display.py` | KawaiiSpinner, tool preview formatting, skin integration |
| `skill_commands.py` | Skill slash commands (shared CLI/gateway) |
| `smart_model_routing.py` | Cheap-vs-strong model routing |
| `title_generator.py` | Auto session title generation |
| `redact.py` | Secret redacting for logs |

### Terminal Environments (`tools/environments/`)

| Backend | Description |
|---|---|
| Local | Direct execution on host |
| Docker | Containerized execution with configurable images |
| SSH | Remote execution over SSH |
| Modal | Cloud containers (Modal.com) |
| Singularity | HPC container execution |
| Daytona | Cloud sandboxes (Daytona.io) |

## Data Flow

### CLI Message Flow

```
User types message
  → HermesCLI.run() receives input
  → AIAgent.run_conversation(message)
    → Build system prompt (prompt_builder)
    → Append user message + conversation history
    → Loop:
        → LLM API call (OpenAI/Anthropic)
        → If tool_calls:
            → handle_function_call(name, args)
              → Check agent-loop tools (todo, memory, session_search, delegate)
              → registry.dispatch(name, args)
                → Tool handler executes (sync or async-bridged)
                → Plugin pre/post hooks fire
              → Append tool result message
            → Parallel batch for safe tools (up to 8 workers)
        → If final response:
            → Render via Rich panel
            → Save session to SQLite
            → Return to user
    → Context compression if approaching limit
    → Interrupt check between iterations
```

### Gateway Message Flow

```
Platform receives message (e.g., Telegram)
  → PlatformAdapter handler fires (async)
  → GatewayRunner._handle_message()
    → SessionStore.get_or_create_session()
    → Load conversation transcript
    → Build system prompt (with session context, memory, skills)
    → Get or create cached AIAgent (preserves prompt caching)
    → AIAgent.run_conversation()
      → Streaming: tokens pushed via stream_delta_callback
      → DeliveryRouter updates message in real-time (Telegram edits)
    → Final response → DeliveryRouter → PlatformAdapter.send()
    → Session saved to SQLite
  → Background: session expiry watcher, memory flush before expiry
```

### Tool Registration Flow

```
Process startup
  → import model_tools
  → _discover_tools()
    → Import all tools/*.py modules
      → Each module calls registry.register() at module level
    → MCP server discovery (if configured)
    → Plugin discovery (user/project/pip)
  → get_tool_definitions(toolsets)
    → Resolve toolset composition (recursive, cycle detection)
    → Filter by check_fn (API key availability)
    → Return OpenAI-format schemas
```

### Cron Job Flow

```
Gateway background thread
  → scheduler.tick() every 60s (file-based lock)
  → get_due_jobs() via croniter
  → For each due job:
    → Create AIAgent with limited iterations
    → Run conversation with job prompt
    → Save output
    → Deliver to target (origin chat / home channels / local file)
```

### Batch Processing Flow

```
batch_runner.py invoked with prompt list
  → Multiprocessing pool (N workers)
  → For each prompt:
    → Create AIAgent with toolset distribution
    → Run conversation
    → Save JSONL trajectory
    → Record tool usage + reasoning coverage
  → Checkpoint after each prompt (resume support)
  → Aggregate statistics
```

## External Dependencies

### LLM Providers

| Provider | Type | Auth |
|---|---|---|
| OpenRouter | Aggregator (100+ models) | API key |
| Nous Portal | Nous Research subscription | OAuth |
| OpenAI Codex | OpenAI direct | OAuth (chatgpt.com) |
| GitHub Copilot | GitHub | `GITHUB_TOKEN` or `gh auth` |
| Anthropic | Anthropic direct | API key or Claude Code OAuth |
| Z.AI / GLM | Zhipu AI direct | API key |
| Kimi / Moonshot | Moonshot AI direct | API key |
| MiniMax | MiniMax global/China | API key |
| OpenCode Zen | 35+ curated models | API key |
| OpenCode Go | Open models subscription | API key |
| AI Gateway (Vercel) | 200+ models | API key |
| Alibaba/DashScope | Qwen + multi-provider | API key |
| Hugging Face | 20+ open models | API key |
| Kilo Code | Kilo Gateway | API key |
| Custom | Any OpenAI-compatible endpoint | URL + API key |

### Tool Integrations

| Integration | Purpose | Dependency |
|---|---|---|
| Exa AI | Web search | `exa-py` |
| Firecrawl | Web extraction/scraping | `firecrawl-py` |
| Parallel Web | Parallel web search | `parallel-web` |
| Fal.ai | Image generation | `fal-client` |
| Browserbase | Browser automation | browser-provider SDK |
| Edge TTS | Free text-to-speech | `edge-tts` |
| ElevenLabs | Premium TTS | `elevenlabs` |
| Faster Whisper | Speech-to-text | `faster-whisper` |
| MCP | External tool servers | `mcp` |
| Honcho | AI-native memory | `honcho-ai` |
| Home Assistant | Smart home control | HTTP API |
| Twilio | SMS | Twilio HTTP API |

### Platform Messaging Libraries

| Platform | Library |
|---|---|
| Telegram | `python-telegram-bot` |
| Discord | `discord.py` |
| WhatsApp | Node.js (baileys bridge) |
| Slack | `slack-bolt` |
| Signal | signal-cli REST API |
| Matrix | `matrix-nio` (E2E encryption) |
| DingTalk | `dingtalk-stream` |
| Feishu/Lark | `lark-oapi` |
| WeCom | WebSocket |
| Mattermost | HTTP API |
| Email | `imaplib`/`smtplib` |

### Core Libraries

| Library | Purpose |
|---|---|
| `openai` | LLM client (Chat Completions + Responses API) |
| `anthropic` | Anthropic Messages API |
| `httpx` | HTTP client |
| `rich` | Terminal formatting |
| `prompt_toolkit` | Interactive CLI input |
| `fire` | CLI framework |
| `pyyaml` | YAML config parsing |
| `pydantic` | Data validation |
| `tenacity` | Retry logic |
| `jinja2` | Template rendering |
| `croniter` | Cron expression parsing |

## Interfaces & Boundaries

### Component Communication

| Boundary | Mechanism | Notes |
|---|---|---|
| CLI ↔ AIAgent | Synchronous method calls | `AIAgent.run_conversation()` returns dict |
| Gateway ↔ AIAgent | Synchronous method calls (in async context) | Agent cached per session |
| AIAgent ↔ LLM Providers | HTTP (OpenAI/Anthropic APIs) | Supports streaming, function calling |
| AIAgent ↔ Tools | Synchronous function dispatch via `ToolRegistry` | Async handlers bridged with persistent event loop |
| Gateway ↔ Platforms | Async event handlers (platform SDKs) | Each adapter implements `BasePlatformAdapter` |
| Gateway ↔ DeliveryRouter | Synchronous method calls | Routes responses to correct platform/chat |
| All components ↔ SQLite | Direct SQLite3 calls | WAL mode, application-level retry |
| CLI ↔ User | `prompt_toolkit` REPL + Rich rendering | Fixed input area, scrollable output |
| Gateway ↔ User | Platform-specific message types | Text, images, voice, documents |
| ACP ↔ Editor | Stdio (Agent Communication Protocol) | VS Code, Zed, JetBrains |
| Terminal tools ↔ Execution env | `BaseEnvironment` interface | Local, Docker, SSH, Modal, Singularity, Daytona |
| Plugins ↔ Core | Hook system (`pre_tool_call`, `post_tool_call`) | Dynamic slash command registration |

### Key Boundaries

```
+------------------------------------------------------------------+
|                        UI Layer                                   |
|  HermesCLI (CLI)  |  GatewayRunner (Messaging)  |  ACPServer     |
+-------------------+-----------------------------+-----------------+
|                     Orchestration Layer                            |
|                        AIAgent                                     |
|  (conversation loop, budget tracking, context compression)         |
+------------------------------------------------------------------+
|                        Tool Layer                                 |
|  model_tools.py  →  ToolRegistry  →  Individual tool handlers     |
+------------------------------------------------------------------+
|                     Infrastructure Layer                           |
|  Environments  |  SQLite  |  External APIs  |  Cron  |  Batch    |
+------------------------------------------------------------------+
```

## Infrastructure & Deployment

### Deployment Models

| Model | Description |
|---|---|
| **Local installation** | `pip install` or `uv` from PyPI. Runs on user machine. `setup-hermes.sh` for guided setup |
| **Docker** | `Dockerfile` + `docker/entrypoint.sh`. Published via CI/CD to container registry |
| **Nix** | `flake.nix` with flake lock. Reproducible builds |

### Configuration Management

| File | Purpose |
|---|---|
| `~/.hermes/config.yaml` | Primary user configuration (model, terminal, display, platforms, memory, etc.) |
| `~/.hermes/.env` | API keys and secrets (dotenv format) |
| `./cli-config.yaml` | Project-level fallback config |

**Loading priority** (highest to lowest): Environment variables → `.env` → `config.yaml` → `cli-config.yaml` → built-in defaults.

### Profile System

Multi-instance support via `hermes -p <name>`. Each profile gets an isolated `HERMES_HOME` directory with its own config, API keys, memory, sessions, skills, and gateway. All code paths use `get_hermes_home()` to resolve the correct directory.

### CI/CD

| Pipeline | Description |
|---|---|
| `tests.yml` | Pytest suite (~3000 tests) |
| `docker-publish.yml` | Docker image build and publish |
| `deploy-site.yml` | Documentation website deployment |
| `supply-chain-audit.yml` | Dependency security audit |
| `nix.yml` | Nix flake CI |

### Runtime Assumptions

- **Python 3.10+** required
- **SQLite** is the sole database — no external DB server needed
- **No message queue** — cron jobs use file-based locking, batch processing uses multiprocessing
- **Single-process** for CLI and gateway (gateway uses async for concurrency)
- **State files** all live under `~/.hermes/` (or profile-specific directory)
- **Error logging** via rotating file handler (2MB, 2 backups)

## Key Design Decisions

### Synchronous Agent Loop

The core conversation loop is entirely synchronous (`run_conversation()`). This simplifies reasoning about state, prompt caching, and tool execution order. The gateway wraps this in async context using thread pools for concurrent message handling.

**Tradeoff**: Simpler code and reliable prompt caching vs. lower throughput for concurrent requests.

### Registry-Based Tool Discovery

Tools self-register at import time via `registry.register()`. This eliminates manual wiring and makes adding tools a 3-file change (tool file, import in `model_tools.py`, toolset in `toolsets.py`).

**Tradeoff**: Import-time side effects vs. automatic discovery and zero-config tool addition.

### Prompt Caching Preservation

The gateway caches `AIAgent` instances per session to preserve Anthropic prompt caching across messages. Model switching, toolset changes, and system prompt modifications are deliberately restricted mid-conversation to avoid cache invalidation.

**Tradeoff**: Higher cost efficiency vs. flexibility to change configuration mid-session.

### SQLite Over External Database

All persistent state (sessions, messages, cron jobs) uses a single SQLite file with WAL mode. No external database server is required.

**Tradeoff**: Zero operational overhead vs. single-writer limitation (mitigated by application-level retry with jitter).

### Profile Isolation via Environment Variable

The profile system works by setting `HERMES_HOME` before any module imports. All 119+ path references use `get_hermes_home()`, making profiles fully transparent to the codebase.

**Tradeoff**: Simple and robust vs. requires discipline to never hardcode `~/.hermes`.

### Parallel Tool Execution with Safety Classification

Tools are classified as never-parallel (interactive), parallel-safe (read-only), or path-scoped (independent file paths). Batches of safe tools execute in a thread pool (max 8 workers).

**Tradeoff**: Faster tool execution vs. complexity in safety classification.

### Skin/Theme System as Pure Data

Skins are YAML data files — no code changes needed to add or modify a theme. Missing values inherit from the default skin.

**Tradeoff**: High customizability with zero code changes vs. limited to predefined customization points.

## Risks & Gaps

### Scalability

- **Single-process gateway**: Cannot horizontally scale. Each gateway instance handles one set of platform connections.
- **SQLite single-writer**: Application-level retry handles contention, but high-concurrency scenarios (many simultaneous gateway sessions) could experience write delays.
- **No message queue**: Cron jobs and batch processing lack distributed coordination. File-based locking only works on a single machine.

### Security

- **API keys in `.env` file**: Stored as plaintext on disk. No encryption at rest.
- **Tool execution sandboxing**: Terminal tools run on the host by default. Docker/Modal backends provide isolation but are opt-in.
- **Prompt injection**: `prompt_builder.py` includes injection detection, but skill content is injected as user messages, not system prompts — partially mitigating but not eliminating risk.
- **OAuth token storage**: Nous Portal and Copilot OAuth tokens stored in `auth.json` without encryption.

### Reliability

- **No health checks**: Gateway has no built-in health endpoint (except the API server platform).
- **No graceful shutdown**: Gateway relies on signal handling but doesn't drain in-flight requests.
- **Cron scheduler single point of failure**: If the gateway process dies, cron jobs don't fire until restart.

### Ambiguities

- **Plugin API stability**: Plugin hooks (`pre_tool_call`, `post_tool_call`) are documented but versioning/compatibility guarantees are unclear.
- **ACP protocol version**: The ACP adapter implements an internal protocol — compatibility with future IDE versions is not guaranteed.
- **Batch runner output format**: JSONL trajectory format is described as "HuggingFace-compatible" but exact schema is not formally specified.

### Missing Pieces

- **No rate limiting** at the gateway level — relies on provider-side rate limits.
- **No audit logging** — tool executions are saved in session transcripts but not independently auditable.
- **No metrics/observability** — no Prometheus, OpenTelemetry, or structured metrics export.
- **No database migration tooling** — schema migrations are hardcoded in `SessionDB` with no CLI for manual migration.

## Appendix

### File Structure Summary

```
hermes-agent/
├── run_agent.py              # AIAgent — core conversation loop
├── model_tools.py            # Tool orchestration layer
├── toolsets.py               # Toolset definitions
├── cli.py                    # HermesCLI — interactive CLI
├── hermes_state.py           # SessionDB — SQLite session store
├── hermes_constants.py       # Shared constants, HERMES_HOME resolution
├── batch_runner.py           # Parallel batch processing
├── agent/                    # Agent internals (prompt, caching, display, skills)
├── tools/                    # Tool implementations (40+ tools)
│   ├── registry.py           # Central ToolRegistry singleton
│   ├── terminal_tool.py      # Terminal command execution
│   ├── environments/         # Terminal backends (local, docker, ssh, modal, etc.)
│   └── browser_providers/    # Browser automation providers
├── hermes_cli/               # CLI subcommands (35 files)
│   ├── main.py               # Entry point — all `hermes` subcommands
│   ├── config.py             # DEFAULT_CONFIG, OPTIONAL_ENV_VARS
│   ├── commands.py           # COMMAND_REGISTRY
│   ├── skin_engine.py        # Skin/theme engine
│   ├── setup.py              # Interactive setup wizard
│   └── platforms/            # (none — platforms are in gateway/)
├── gateway/                  # Messaging platform gateway
│   ├── run.py                # GatewayRunner — main loop
│   ├── session.py            # SessionStore, reset policies
│   ├── delivery.py           # DeliveryRouter
│   ├── config.py             # GatewayConfig, Platform enum
│   └── platforms/            # 15+ platform adapters
├── cron/                     # Scheduler (jobs.py, scheduler.py)
├── acp_adapter/              # ACP server (VS Code / Zed / JetBrains)
├── environments/             # RL training environments (Atropos)
├── skills/                   # Bundled skills
├── tests/                    # Pytest suite (~3000 tests)
├── docker/                   # Docker configuration
├── .github/workflows/        # CI/CD pipelines
├── pyproject.toml            # Python project definition
├── Dockerfile                # Docker build
├── flake.nix                 # Nix flake
└── setup-hermes.sh           # Installation script
```

### Notable Configuration Defaults

| Setting | Default |
|---|---|
| Model | `anthropic/claude-opus-4.6` |
| Max iterations | 90 |
| Terminal backend | `local` |
| Session reset mode | `both` (daily + idle) |
| Idle timeout | 1440 minutes (24h) |
| Reset hour | 04:00 |
| Streaming | Enabled |
| Memory | Disabled by default |
| Parallel tool workers | 8 |
| Cron tick interval | 60 seconds |
| SQLite write retries | 15 (20–150ms jitter) |
| Error log rotation | 2MB, 2 backups |

### Entry Points

| Command | Module | Description |
|---|---|---|
| `hermes` | `hermes_cli/main.py` | Main CLI — subcommands: chat, gateway, setup, model, doctor, cron, acp, sessions, etc. |
| `hermes-agent` | `run_agent.py` | Standalone agent runner (via `fire`) |
| `hermes-acp` | `acp_adapter/entry.py` | ACP server for editor integration |
| `hermes-mcp` | `mcp_serve.py` | MCP server entry point |
