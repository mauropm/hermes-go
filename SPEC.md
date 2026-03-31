# System Specification

## 1. Overview

**Hermes Agent** is a multi-platform AI agent framework that wraps large language models (LLMs) with tool-calling capabilities, persistent session management, and cross-platform messaging integration. It enables an LLM to interact with the real world through a extensible tool system (terminal execution, file operations, web search, browser automation, etc.) and deliver responses across 15+ messaging platforms, a CLI, an API server, and IDE integrations.

**Primary purpose:** Provide a self-contained, production-ready agent that can be deployed as an interactive terminal assistant, a messaging bot, a batch data generator, or an API service — all sharing the same core agent logic.

**Version:** 0.6.0

---

## 2. Architecture

### 2.1 Style

**Monolithic modular application** with layered architecture, registry-driven plugin system, and adapter-based external integrations.

```
┌─────────────────────────────────────────────────────────────────┐
│                        INTERFACE LAYER                           │
│  ┌──────────┐  ┌──────────────┐  ┌─────────┐  ┌─────────────┐  │
│  │   CLI    │  │   Gateway    │  │   ACP   │  │  Batch      │  │
│  │ (REPL)   │  │ (Messaging)  │  │ (IDE)   │  │  Runner     │  │
│  └────┬─────┘  └──────┬───────┘  └────┬────┘  └──────┬──────┘  │
├───────┼───────────────┼───────────────┼──────────────┼─────────┤
│                    ORCHESTRATION LAYER                              │
│  ┌──────────────────────────────────────────────────────────┐    │
│  │                      AIAgent                              │    │
│  │  Conversation loop │ Tool dispatch │ Context compression  │    │
│  │  Provider routing  │ Subagent delegation │ Budget tracking │    │
│  └──────────────────────────┬───────────────────────────────┘    │
├─────────────────────────────┼───────────────────────────────────┤
│                        TOOL LAYER                                 │
│  ┌─────────────┐  ┌──────────────────┐  ┌───────────────────┐  │
│  │ ToolRegistry │  │  Toolset Engine  │  │  Plugin System    │  │
│  │ (dispatch)   │  │ (composition)    │  │ (hooks, tools)    │  │
│  └──────┬──────┘  └────────┬─────────┘  └─────────┬─────────┘  │
│         │                  │                       │             │
│  ┌──────┴──────────────────┴───────────────────────┴─────────┐  │
│  │                    Tool Implementations                     │  │
│  │  Terminal │ Files │ Web │ Browser │ Vision │ Memory │ ... │  │
│  └──────────────────────────┬────────────────────────────────┘  │
├─────────────────────────────┼───────────────────────────────────┤
│                   INFRASTRUCTURE LAYER                            │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐           │
│  │ SQLite   │ │ Terminal │ │ External │ │  Cron    │           │
│  │ (state)  │ │ Envs     │ │ APIs     │ │Scheduler │           │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘           │
└─────────────────────────────────────────────────────────────────┘
```

### 2.2 Design Principles

1. **Registry-driven discovery** — Tools, commands, plugins, and skins self-register at module load time. No manual wiring.
2. **Adapter pattern for externals** — Terminal backends and messaging platforms are abstracted behind interfaces with swappable implementations.
3. **Synchronous core, async periphery** — The agent conversation loop is synchronous for simplicity and reliable prompt caching. Async is used only at boundaries (messaging platforms, streaming delivery).
4. **Profile isolation via environment variable** — Setting `HERMES_HOME` before imports creates fully isolated instances with separate config, state, and credentials.
5. **Prompt caching preservation** — Agent instances are cached per session. Configuration changes mid-conversation are restricted to avoid cache invalidation.

---

## 3. Components

### 3.1 AIAgent (Core Orchestrator)

**Responsibility:** Manage the LLM conversation loop, tool call dispatch, subagent delegation, context compression, and budget tracking.

**Inputs:**
- User message (string)
- Optional system message override
- Optional conversation history
- Configuration (model, provider, toolsets, callbacks)

**Outputs:**
- Final response text
- Conversation history (list of message objects)
- Session metadata (token counts, cost, tool calls)

**Lifecycle:**
1. **Startup:** Resolve provider credentials, create LLM client, discover available tools, build system prompt, create or resume session
2. **Runtime:** Execute conversation loop (LLM call → tool dispatch → repeat until final response or limits reached)
3. **Shutdown:** Flush messages to persistent storage, save trajectory if enabled, clean up child agents and resources

**Key behaviors:**
- Supports up to N iterations (default 90) with token budget tracking
- Interruptible from external threads
- Parallel tool execution for safe operations (max 8 workers)
- Automatic context compression when approaching model limits
- Subagent delegation with isolated context and separate budgets
- Multiple API modes: OpenAI Chat Completions, OpenAI Responses, Anthropic Messages

### 3.2 Tool System

**Responsibility:** Register, discover, filter, and dispatch tool implementations.

**Sub-components:**

#### ToolRegistry (Singleton)
- **Input:** Tool registration calls at module load time
- **Output:** Tool schemas (for LLM), dispatch results (JSON strings)
- **Behavior:** Stores tool metadata (name, toolset, schema, handler, availability check, environment requirements). Filters tools by toolset membership and availability. Dispatches tool calls to handlers.

#### Toolset Engine
- **Input:** Toolset name(s)
- **Output:** Resolved list of tool names
- **Behavior:** Resolves toolset definitions with recursive composition support and cycle detection. Each toolset declares which tools it includes and may reference other toolsets.

#### Plugin System
- **Input:** Plugin directories and entry points
- **Output:** Registered tools and hook callbacks
- **Behavior:** Discovers plugins from user directory (`~/.hermes/plugins/`), project directory (`./.hermes/plugins/`), and package entry points. Loads plugin manifests, registers tools and hooks.

**Valid plugin hooks:**
- `pre_tool_call` — Before tool execution
- `post_tool_call` — After tool execution
- `pre_llm_call` — Before LLM API call
- `post_llm_call` — After LLM API call
- `on_session_start` — When session begins
- `on_session_end` — When session ends

### 3.3 CLI (Interactive Terminal)

**Responsibility:** Provide an interactive terminal interface with rich formatting, slash commands, and session management.

**Inputs:** User text input and slash commands via REPL
**Outputs:** Formatted responses, tool previews, session information
**Dependencies:** AIAgent, SessionDB, ToolRegistry, Skin Engine

**Lifecycle:**
1. Load configuration and resolve credentials
2. Initialize TUI (terminal user interface) with input area and output panel
3. Enter REPL loop: read input → dispatch command or send to agent → display response
4. On exit: clean up background processes, browsers, MCP servers

### 3.4 Gateway (Messaging Platform Controller)

**Responsibility:** Connect to multiple messaging platforms, route messages to agents, and deliver responses.

**Inputs:** Messages from platform adapters (async events)
**Outputs:** Responses delivered to platform adapters
**Dependencies:** AIAgent, SessionStore, DeliveryRouter, Platform Adapters

**Lifecycle:**
1. Load gateway configuration
2. Create and connect all enabled platform adapters
3. Start background tasks (cron scheduler, session expiry watcher, reconnection queue)
4. Process incoming messages: resolve session → run agent → deliver response
5. On shutdown: disconnect all adapters, flush memories, save state

**Key behaviors:**
- Caches AIAgent instances per session to preserve prompt caching
- Evaluates session reset policies (daily, idle, both, none)
- Streams responses in real-time via message edits (where supported)
- Retries failed message sends with exponential backoff
- Queues failed platforms for automatic reconnection

### 3.5 SessionStore (Gateway Session Management)

**Responsibility:** Manage conversation sessions for the gateway, including creation, reset policies, and transcript management.

**Data structures:**
- **SessionSource:** Origin metadata (platform, chat_id, chat_type, user_id, thread_id)
- **SessionEntry:** Session state (session_key, session_id, timestamps, token counts, cost)
- **SessionContext:** Runtime context (source, connected platforms, home channels)

**Key behaviors:**
- Constructs session keys from platform + chat identifiers
- Evaluates reset policies: idle timeout, daily reset at configured hour
- Prevents reset when background processes are active
- Maintains transcripts in both SQLite and JSONL formats

### 3.6 SessionDB (SQLite Session Store)

**Responsibility:** Persistent storage for all sessions and messages with full-text search.

**Schema:**
```
sessions:
  id (TEXT, PK), source (TEXT), user_id (TEXT), model (TEXT),
  model_config (TEXT/JSON), system_prompt (TEXT),
  parent_session_id (TEXT, FK→sessions),
  started_at (REAL), ended_at (REAL), end_reason (TEXT),
  message_count (INT), tool_call_count (INT),
  input_tokens (INT), output_tokens (INT),
  cache_read_tokens (INT), cache_write_tokens (INT),
  reasoning_tokens (INT),
  billing_provider (TEXT), billing_base_url (TEXT), billing_mode (TEXT),
  estimated_cost_usd (REAL), actual_cost_usd (REAL),
  cost_status (TEXT), cost_source (TEXT), pricing_version (TEXT),
  title (TEXT, UNIQUE partial index)

messages:
  id (INT, PK AUTO), session_id (TEXT, FK→sessions),
  role (TEXT), content (TEXT), tool_call_id (TEXT),
  tool_calls (TEXT/JSON), tool_name (TEXT),
  timestamp (REAL), token_count (INT), finish_reason (TEXT),
  reasoning (TEXT), reasoning_details (TEXT/JSON),
  codex_reasoning_items (TEXT/JSON)

messages_fts: FTS5 virtual table for full-text search
```

**Key behaviors:**
- WAL mode for concurrent reads
- Application-level write contention retry (15 retries, 20-150ms jitter)
- Schema versioning with automatic migrations
- FTS5 full-text search across all messages

### 3.7 Cron Scheduler

**Responsibility:** Execute scheduled prompts at specified intervals.

**Inputs:** Job definitions with schedule expressions
**Outputs:** Agent responses delivered to configured targets
**Dependencies:** AIAgent, DeliveryRouter

**Schedule types:**
- Duration: one-shot after N minutes (e.g., "30m")
- Interval: recurring every N minutes (e.g., "every 30m")
- Cron expression: standard cron format (e.g., "0 9 * * *")
- Timestamp: one-shot at specific time (e.g., "2026-02-03T14:00")

**Lifecycle:**
1. Background thread ticks every 60 seconds
2. File-based lock prevents concurrent ticks
3. Gets due jobs, advances next_run before execution (at-most-once semantics)
4. Creates AIAgent with restricted toolsets, runs conversation
5. Delivers result to configured target (origin chat, home channel, local file)

### 3.8 Batch Runner

**Responsibility:** Parallel data generation for training datasets.

**Inputs:** JSONL dataset with prompt fields, toolset distribution configuration
**Outputs:** JSONL trajectory files with tool usage statistics
**Dependencies:** AIAgent, multiprocessing pool

**Key behaviors:**
- Processes prompts in parallel using worker processes
- Samples toolsets per prompt for varied tool exposure
- Checkpoint-based fault tolerance and resume
- Aggregates tool usage and reasoning coverage statistics

### 3.9 ACP Adapter (IDE Integration)

**Responsibility:** Expose the agent via the Agent Communication Protocol for IDE integration (VS Code, Zed, JetBrains).

**Inputs:** ACP protocol messages over stdio
**Outputs:** ACP protocol responses
**Dependencies:** AIAgent, SessionManager

**Key behaviors:**
- Session management (create, resume, fork, list)
- Tool definitions exposed to IDE
- Permission/approval system for tool execution
- Streaming token delivery to IDE

---

## 4. Execution Modes

### 4.1 Interactive CLI

```
hermes [chat] [--model X] [--provider Y] [--resume SESSION]
```

- **Runtime:** Single process, synchronous REPL
- **Input:** User types at terminal prompt
- **Output:** Rich-formatted text in terminal
- **State:** In-memory conversation history + SQLite session persistence
- **Termination:** User types `/quit` or sends SIGINT

### 4.2 Gateway (Messaging Bot)

```
hermes gateway start
```

- **Runtime:** Single process, async event loop with thread pool for agent execution
- **Input:** Messages from platform adapters (async events)
- **Output:** Responses delivered to platform adapters
- **State:** Agent instance cache per session, SQLite persistence
- **Termination:** SIGTERM/SIGINT, graceful disconnect of all platforms

### 4.3 API Server

```
hermes gateway start  # with api_server platform enabled
```

- **Runtime:** HTTP server (FastAPI-based) within the gateway process
- **Input:** HTTP POST requests (OpenAI-compatible format)
- **Output:** HTTP streaming responses (Server-Sent Events)
- **Authentication:** Bearer token via `API_SERVER_KEY`
- **Endpoint:** `/v1/chat/completions`

### 4.4 ACP Server

```
hermes acp
```

- **Runtime:** Single process, stdio-based protocol
- **Input:** ACP protocol messages over stdin
- **Output:** ACP protocol messages over stdout
- **Transport:** Stdio (JSON-RPC-like)

### 4.5 Batch Processing

```
hermes-agent --dataset file.jsonl --batch-size 10 --workers 4
```

- **Runtime:** Multiprocessing pool
- **Input:** JSONL dataset file
- **Output:** JSONL trajectory files + statistics
- **State:** Checkpoint file for resume
- **Termination:** After all prompts processed or interrupted

### 4.6 Cron Jobs

- **Runtime:** Background thread within gateway process
- **Input:** Job definitions from JSON file
- **Output:** Delivered messages or local files
- **Schedule:** Tick every 60 seconds

### 4.7 Standalone Agent (Programmatic)

```python
agent = AIAgent(model="...", api_key="...")
response = agent.chat("Hello")
```

- **Runtime:** Synchronous function call
- **Input:** Message string
- **Output:** Response string
- **Use case:** Embedding in other applications

---

## 5. Agents & Connectors

### 5.1 Platform Adapters (Messaging Connectors)

Each messaging platform is implemented as a connector that conforms to a standard interface.

**Interface: PlatformAdapter**

```
Methods (required):
  connect() → bool          # Establish connection, start receiving messages
  disconnect()              # Close connection, clean up resources
  send(chat_id, content, reply_to_message_id, metadata) → SendResult

Methods (optional, with defaults):
  edit_message(chat_id, message_id, content) → SendResult
  send_typing(chat_id, metadata)
  stop_typing(chat_id)
  send_image(chat_id, image_url, caption, reply_to, metadata) → SendResult
  send_voice(chat_id, audio_path, caption, reply_to, metadata) → SendResult
  send_video(chat_id, video_path, caption, reply_to, metadata) → SendResult
  send_document(chat_id, file_path, caption, filename, reply_to, metadata) → SendResult

Lifecycle hooks:
  on_processing_start(event)
  on_processing_complete(event, success)

Message handler:
  set_message_handler(callback)
  handle_message(event) → spawns background task
```

**MessageEvent structure:**
```
text: string
message_type: TEXT | LOCATION | PHOTO | VIDEO | AUDIO | VOICE | DOCUMENT | STICKER | COMMAND
source: SessionSource
message_id: string (optional)
media_urls: list[string]
media_types: list[string]
reply_to_message_id: string (optional)
reply_to_text: string (optional)
auto_skill: string (optional)
timestamp: datetime
```

**SendResult structure:**
```
success: boolean
message_id: string (optional)
error: string (optional)
retryable: boolean
```

**Supported platforms:**

| Platform | Protocol | Auth Method | Connection Type |
|----------|----------|-------------|-----------------|
| Telegram | Bot API (polling) | Bot token | Long-polling |
| Discord | Gateway WebSocket | Bot token | WebSocket |
| WhatsApp | Node.js bridge (baileys) | Session credentials | WebSocket bridge |
| Slack | Bolt SDK (Socket Mode) | Bot token + App token | WebSocket |
| Signal | signal-cli REST API | HTTP URL + account | HTTP polling |
| Mattermost | HTTP API | Bot token + server URL | HTTP polling |
| Matrix | matrix-nio sync | Access token | HTTP long-poll |
| Home Assistant | HTTP API | Token + URL | HTTP |
| Email | IMAP/SMTP | Credentials | IMAP polling + SMTP |
| SMS | Twilio API | Account SID + Auth token | HTTP webhook |
| DingTalk | WebSocket stream | Client ID + Secret | WebSocket |
| Feishu/Lark | WebSocket | App ID + Secret | WebSocket |
| WeCom | WebSocket | Bot ID + Secret | WebSocket |
| API Server | HTTP (FastAPI) | Bearer token | HTTP server |
| Webhook | HTTP POST | Per-route HMAC secret | HTTP server |

**Adding a new platform adapter:**
1. Implement the PlatformAdapter interface
2. Register the platform in the Platform enum
3. Add platform configuration schema
4. Implement message event creation from platform-specific events
5. Handle media caching (images, audio, documents)
6. Add to delivery router target resolution

### 5.2 Terminal Environment Connectors

Each terminal execution backend implements a standard interface.

**Interface: TerminalEnvironment**

```
Methods:
  execute(command, timeout, cwd) → {output: string, returncode: int, error: string}
  cleanup()
  spawn_background(command, cwd, env_vars) → ProcessSession
  poll(process_session) → {status: string, output: string, exit_code: int}
  kill(process_session)

Properties:
  env: dict (environment variables to pass through)
```

**Supported environments:**

| Environment | Isolation | Persistence | Use Case |
|-------------|-----------|-------------|----------|
| Local | None | Host filesystem | Development |
| Docker | Container | Optional volume | Sandboxed execution |
| SSH | Remote host | Remote filesystem | Remote servers |
| Modal | Cloud sandbox | Ephemeral | Cloud execution |
| Singularity | Container | Ephemeral | HPC environments |
| Daytona | Cloud sandbox | Ephemeral | Managed sandboxes |

### 5.3 LLM Provider Connectors

The system supports multiple LLM providers through a unified interface.

**Interface: LLMProvider**

```
Methods:
  chat_completions(messages, model, tools, max_tokens, temperature, stream) → Response
  responses_api(prompt, model, tools, stream) → Response  (OpenAI-specific)
  messages_api(messages, model, tools, max_tokens, stream) → Response  (Anthropic-specific)

Response structure:
  content: string
  tool_calls: list[{name: string, arguments: dict}]
  reasoning: string (optional)
  finish_reason: string
  usage: {input_tokens: int, output_tokens: int, ...}
  stream: iterator of {delta: string, done: bool}
```

**Provider resolution order (auto mode):**
1. Explicit provider configuration
2. Model name prefix matching (e.g., "anthropic/..." → Anthropic)
3. Environment variable detection (which API keys are set)
4. Fallback chain from configuration

**Supported providers:**

| Provider | API Format | Auth | Base URL |
|----------|-----------|------|----------|
| OpenRouter | Chat Completions | API key header | https://openrouter.ai/api/v1 |
| Nous Portal | Chat Completions | OAuth bearer token | https://inference-api.nousresearch.com/v1 |
| OpenAI Codex | Responses API | OAuth token | https://chatgpt.com/backend-api/codex |
| GitHub Copilot | Chat Completions | GitHub token | GitHub endpoint |
| Anthropic | Messages API | API key header | https://api.anthropic.com |
| Z.AI/GLM | Chat Completions | API key | Provider-specific |
| Kimi/Moonshot | Chat Completions | API key | https://api.kimi.com |
| MiniMax | Chat Completions | API key | Provider-specific |
| OpenCode Zen | Chat Completions | API key | Provider-specific |
| Hugging Face | Chat Completions | API key | Provider-specific |
| Custom | OpenAI-compatible | API key | User-configured URL |

### 5.4 Subagent Delegation

The system can spawn child agents for parallel or isolated task execution.

**Delegation constraints:**
- Maximum depth: 2 (parent → child, no grandchild)
- Maximum concurrent children: 3
- Blocked tools for children: delegate_task, clarify, memory, send_message, execute_code
- Child toolsets are intersection of requested toolsets and parent's available tools

**Child agent construction:**
1. Inherits parent's credentials (or uses delegation config override)
2. Gets restricted toolsets (blocked tools stripped)
3. Fresh conversation (no parent history)
4. Isolated task_id (separate terminal session)
5. Focused system prompt with goal and context
6. Progress callbacks relay tool calls to parent display

**Result format:**
```json
{
  "results": [{
    "task_index": 0,
    "status": "completed|failed|error|interrupted",
    "summary": "string",
    "api_calls": 5,
    "duration_seconds": 12.34,
    "model": "string",
    "exit_reason": "completed|max_iterations|interrupted",
    "tokens": {"input": 1000, "output": 500},
    "tool_trace": [{"tool": "name", "args_bytes": 20, "result_bytes": 500, "status": "ok"}]
  }],
  "total_duration_seconds": 15.67
}
```

### 5.5 MCP (Model Context Protocol) Connector

Connects to external MCP servers for dynamic tool discovery.

**Behavior:**
- Discovers MCP servers from configuration
- Establishes stdio or HTTP connections to each server
- Imports server tools into the local registry
- Routes tool calls to appropriate MCP server
- Supports dynamic reload (`/reload-mcp` command)

---

## 6. Data Models

### 6.1 Core Entities

#### Session
```
id: string (UUID)
source: string ("cli", "telegram", "discord", etc.)
user_id: string (optional)
model: string
model_config: JSON object
system_prompt: string
parent_session_id: string (optional, FK to Session)
started_at: timestamp (epoch seconds)
ended_at: timestamp (epoch seconds, nullable)
end_reason: string (optional)
message_count: integer
tool_call_count: integer
input_tokens: integer
output_tokens: integer
cache_read_tokens: integer
cache_write_tokens: integer
reasoning_tokens: integer
title: string (unique, nullable)
estimated_cost_usd: float
actual_cost_usd: float
```

#### Message
```
id: integer (auto-increment)
session_id: string (FK to Session)
role: string ("system", "user", "assistant", "tool")
content: string (nullable)
tool_call_id: string (nullable)
tool_calls: JSON array (nullable)
tool_name: string (nullable)
timestamp: timestamp (epoch seconds)
token_count: integer (nullable)
finish_reason: string (nullable)
reasoning: string (nullable)
reasoning_details: JSON array (nullable)
```

#### ToolEntry
```
name: string
toolset: string
schema: JSON object (OpenAI function schema format)
handler: function reference
check_fn: function () → bool (availability check)
requires_env: list[string] (required environment variables)
is_async: boolean
description: string
emoji: string
```

#### Toolset
```
name: string
description: string
tools: list[string] (tool names)
includes: list[string] (other toolset names, optional)
requires_env: list[string] (optional)
```

#### PlatformConfig
```
enabled: boolean
token: string (optional)
api_key: string (optional)
home_channel: {platform: string, chat_id: string, name: string} (optional)
reply_to_mode: string ("off", "first", "all")
extra: map[string, any]
```

#### SessionResetPolicy
```
mode: string ("daily", "idle", "both", "none")
at_hour: integer (0-23)
idle_minutes: integer
notify: boolean
notify_exclude_platforms: list[string]
```

#### CronJob
```
id: string (12-char hex)
name: string
prompt: string
skills: list[string]
model: string (optional override)
provider: string (optional override)
schedule: {kind: string, minutes: integer, expression: string}
repeat: {times: integer (nullable), completed: integer}
enabled: boolean
state: string ("scheduled", "paused", "completed")
deliver: string ("origin", "local", "platform:chat_id")
origin: SessionSource (for "origin" delivery)
created_at: ISO timestamp
next_run_at: ISO timestamp
last_run_at: ISO timestamp (nullable)
last_status: string (nullable)
last_error: string (nullable)
```

#### ProcessSession
```
id: string ("proc_" + 12-char hex)
command: string
task_id: string
session_key: string
pid: integer (optional)
cwd: string (optional)
started_at: timestamp (epoch seconds)
exited: boolean
exit_code: integer (optional)
output_buffer: string (max 200,000 chars)
detached: boolean
watcher_platform: string (optional)
watcher_chat_id: string (optional)
watcher_thread_id: string (optional)
watcher_interval: integer (seconds)
```

### 6.2 Serialization Formats

- **Tool schemas:** OpenAI function calling format (JSON)
- **Messages:** OpenAI chat format (JSON)
- **Tool results:** JSON strings
- **Session transcripts:** JSONL (one JSON object per line)
- **Trajectories:** JSONL with conversation + metadata
- **Configuration:** YAML
- **Cron jobs:** JSON file
- **Process state:** JSON file

### 6.3 Validation Rules

- Tool handlers MUST return a JSON string
- Tool schemas MUST conform to OpenAI function calling format
- Session IDs MUST be valid UUIDs
- Session titles MUST be unique (per source, NULLs allowed)
- Cron schedule expressions MUST be parseable (duration, interval, cron, or timestamp)
- File tool paths MUST NOT access sensitive system directories
- Read file tool blocks after 4 consecutive identical reads (loop detection)

---

## 7. APIs / Interfaces

### 7.1 OpenAI-Compatible API (API Server Platform)

**Endpoint:** `POST /v1/chat/completions`

**Authentication:** `Authorization: Bearer <API_SERVER_KEY>`

**Request:**
```json
{
  "model": "string",
  "messages": [{"role": "user", "content": "string"}],
  "stream": false,
  "max_tokens": 4096,
  "temperature": 0.7
}
```

**Response (non-streaming):**
```json
{
  "id": "chatcmpl-xxx",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "string",
  "choices": [{
    "index": 0,
    "message": {"role": "assistant", "content": "string"},
    "finish_reason": "stop"
  }],
  "usage": {"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30}
}
```

**Response (streaming):** Server-Sent Events with `data: [DONE]` terminator

### 7.2 Tool Schema Format (OpenAI Function Calling)

```json
{
  "type": "function",
  "function": {
    "name": "tool_name",
    "description": "Human-readable description",
    "parameters": {
      "type": "object",
      "properties": {
        "param_name": {
          "type": "string",
          "description": "Parameter description"
        }
      },
      "required": ["param_name"]
    }
  }
}
```

### 7.3 Tool Result Format

All tool handlers return a JSON string:

```json
{
  "success": true,
  "data": "...",
  "error": null
}
```

Or on failure:

```json
{
  "success": false,
  "error": "Error description",
  "data": null
}
```

### 7.4 Platform Adapter Interface

See Section 5.1 for the complete interface specification.

### 7.5 Terminal Environment Interface

See Section 5.2 for the complete interface specification.

### 7.6 ACP Protocol

The ACP adapter implements the Agent Communication Protocol over stdio:

```
Initialize → {protocol_version, client_capabilities}
NewSession → {session_id}
Prompt → {session_id, content_blocks}
  → Stream: SessionUpdate (text deltas, tool calls, thinking)
  → Final: PromptResponse
ForkSession → {session_id, new_session_id}
ListSessions → {sessions: [SessionInfo]}
```

### 7.7 Error Handling Patterns

**LLM API errors:**
- Rate limit (429): Wait and retry, or fall back to next provider
- Service unavailable (503): Fall back to next provider
- Connection failure: Fall back to next provider
- Invalid response: Return error message to agent

**Tool execution errors:**
- Handler exception: Return JSON error object to agent
- Timeout: Return timeout error to agent
- Permission denied: Return access denied error to agent

**Platform send errors:**
- Transient failure: Retry with exponential backoff (max 3 attempts)
- Permanent failure: Return error, log warning
- Message too long: Truncate or split (platform-dependent)

---

## 8. Core Workflows

### 8.1 Conversation Loop

```
1. BUILD SYSTEM PROMPT
   - Load identity, context files (AGENTS.md, SOUL.md, etc.)
   - Inject memory guidance, skill instructions, platform hints
   - Append user's ephemeral system prompt if provided

2. INITIALIZE MESSAGES
   - messages = [system_prompt]
   - Append conversation_history if resuming
   - Append user_message

3. CREATE/RESUME SESSION
   - Generate or reuse session_id
   - Create session record in SessionDB
   - Initialize token counters

4. CONVERSATION LOOP (while iterations < max AND budget > 0):
   a. CHECK INTERRUPT
      - If interrupt requested, break loop

   b. CHECK CONTEXT SIZE
      - If context exceeds threshold, run compression:
        i.   Prune old tool results (>200 chars → placeholder)
        ii.  Protect head messages (first N)
        iii. Find tail boundary by token budget
        iv.  Summarize middle turns via auxiliary LLM
        v.   Replace compressed section with summary

   c. SANITIZE MESSAGES
      - Fix orphaned tool_call/tool_result pairs
      - Remove invalid Unicode surrogates

   d. APPLY PROMPT CACHING (Anthropic only)
      - Mark system prompt and last 3 non-system messages

   e. CALL LLM API
      - Streaming or non-streaming based on configuration
      - Include tool schemas in request
      - Handle provider fallback on failure

   f. PROCESS RESPONSE
      - If tool_calls present:
        i.   Classify tools for parallel safety
        ii.  Execute safe tools in parallel (max 8 workers)
        iii. Execute unsafe tools sequentially
        iv.  Check command approval for terminal tools
        v.   Append tool results to messages
        vi.  Continue loop
      - If no tool_calls:
        i.   This is the final response
        ii.  Save trajectory if enabled
        iii. Flush messages to SessionDB
        iv.  Return result

5. HANDLE EXIT CONDITIONS
   - Max iterations reached: Return partial response
   - Interrupt: Return interrupt message
   - Context overflow: Compress and retry or return error
```

### 8.2 Gateway Message Handling

```
1. PLATFORM RECEIVES MESSAGE
   - Platform adapter creates MessageEvent
   - Spawns background task

2. RESOLVE SESSION
   - Build session_key from (platform, chat_id, thread_id, user_id)
   - Check reset policy:
     * Idle: Has session been inactive > idle_minutes?
     * Daily: Has the daily reset hour passed?
     * Active processes: Never reset if background processes running
   - If reset needed: Create new session, flush memories
   - If continuing: Load existing session

3. CHECK FOR RUNNING AGENT
   - If agent already running for this session:
     * Interrupt current agent
     * Queue new message

4. BUILD SESSION CONTEXT
   - Platform hints (behavioral instructions)
   - Delivery options (where to send response)
   - Home channels (where to send unsolicited messages)

5. RUN AGENT
   - Get or create cached AIAgent (preserves prompt caching)
   - Run conversation in executor thread
   - Stream tokens to delivery router (if streaming enabled)

6. DELIVER RESPONSE
   - Resolve delivery targets from session context
   - Send via platform adapter(s)
   - Handle send failures with retry

7. UPDATE SESSION
   - Update token counts in SessionDB
   - Update session metadata in SessionStore
```

### 8.3 Tool Registration Flow

```
1. PROCESS STARTUP
   - Import orchestration module

2. DISCOVER TOOLS
   - Import all tool modules
     → Each module calls registry.register() at module level
   - Discover MCP servers (if configured)
   - Discover plugins (user, project, pip entry points)

3. BUILD TOOL DEFINITIONS
   - Resolve toolset names to tool name lists (recursive composition)
   - Filter by check_fn (API key availability)
   - Rebuild dynamic schemas (e.g., execute_code available tools)
   - Strip cross-tool references when tools are unavailable

4. PASS TO LLM
   - Tool schemas included in API request
   - LLM selects tools to call
```

### 8.4 Command Approval Flow (Terminal Tools)

```
1. TERMINAL TOOL RECEIVES COMMAND
   - Check if command matches dangerous patterns (30+ regex patterns)

2. IF NOT DANGEROUS:
   - Execute immediately

3. IF DANGEROUS:
   a. Check approval mode:
      - "off": Execute immediately
      - "smart": Query auxiliary LLM for risk assessment
        * LLM returns: approve, deny, or escalate
        * If approve: Execute
        * If deny: Return error
        * If escalate: Fall through to manual
      - "manual": Prompt user

   b. Manual approval:
      - Create pending approval request
      - Wait for user response (once/session/always/deny)
      - If "always": Add to permanent allowlist
      - If "session": Add to session-level approvals
      - If "once": Execute this time only
      - If "deny": Return error

4. EXECUTE COMMAND
   - Run in configured environment (local, docker, ssh, etc.)
   - Capture stdout, stderr, return code
   - Return result as JSON
```

### 8.5 Session Reset Flow

```
1. TRIGGER: New message arrives at gateway

2. EVALUATE RESET POLICY:
   a. Check mode:
      - "none": Never reset
      - "daily": Check if current hour >= at_hour AND last activity was before today
      - "idle": Check if (now - last_activity) > idle_minutes
      - "both": Reset if EITHER daily OR idle condition met

   b. Check exceptions:
      - Session has active background processes → never reset
      - Session is a DM that requires pairing → check pairing status

3. IF RESET NEEDED:
   a. Flush memories (if memory system enabled)
   b. Create new session_id
   c. Notify user (if notify enabled and platform not excluded)
   d. Mark old session as auto-reset
   e. Return new session entry

4. IF NO RESET:
   - Return existing session entry
```

---

## 9. Configuration

### 9.1 Configuration Sources (Priority Order)

1. **Environment variables** (highest priority)
2. **`.env` file** at `~/.hermes/.env` (dotenv format)
3. **`config.yaml`** at `~/.hermes/config.yaml` (primary YAML config)
4. **Project config** at `./cli-config.yaml` (project-level fallback)
5. **Built-in defaults** (lowest priority)

### 9.2 Configuration Structure

```yaml
# Model configuration
model: "anthropic/claude-opus-4.6"
fallback_providers: []                    # Provider failover chain
toolsets: ["hermes-cli"]                  # Active toolsets

# Agent behavior
agent:
  max_turns: 90                           # Max tool-calling iterations
  tool_use_enforcement: "auto"            # "auto", "on", "off"

# Terminal execution
terminal:
  backend: "local"                        # local, docker, ssh, modal, singularity, daytona
  cwd: "."                                # Working directory
  timeout: 180                            # Command timeout (seconds)
  docker_image: "nikolaik/python-nodejs:python3.11-nodejs20"
  container_cpu: 1
  container_memory: 5120                  # MB
  container_disk: 51200                   # MB
  container_persistent: true
  docker_volumes: []
  persistent_shell: true

# Browser automation
browser:
  inactivity_timeout: 120                 # seconds
  command_timeout: 30                     # seconds
  record_sessions: false

# Checkpoints
checkpoints:
  enabled: true
  max_snapshots: 50

# Context compression
compression:
  enabled: true
  threshold: 0.50                         # Compress when context exceeds this ratio
  target_ratio: 0.20                      # Target size after compression
  protect_last_n: 20                      # Protected tail messages
  summary_model: ""                       # Override model for summarization
  summary_provider: "auto"

# Smart model routing
smart_model_routing:
  enabled: false
  max_simple_chars: 160
  max_simple_words: 28

# Auxiliary LLM providers (for vision, compression, approval, etc.)
auxiliary:
  vision:
    provider: "auto"
    model: ""
    timeout: 30
  web_extract:
    provider: "auto"
  compression:
    provider: "auto"
    timeout: 120
  approval:
    provider: "auto"
    model: ""
  # ... additional auxiliary configs

# Display
display:
  compact: false
  personality: "kawaii"
  streaming: false
  show_reasoning: false
  show_cost: false
  skin: "default"
  tool_progress_command: false
  tool_preview_length: 0

# Voice
tts:
  provider: "edge"
stt:
  enabled: true
  provider: "local"
voice:
  record_key: "ctrl+b"
  max_recording_seconds: 120
  auto_tts: false
  silence_threshold: 200
  silence_duration: 3.0

# Human-like pacing
human_delay:
  mode: "off"
  min_ms: 800
  max_ms: 2500

# Memory
memory:
  memory_enabled: true
  user_profile_enabled: true
  memory_char_limit: 2200
  user_char_limit: 1375

# Delegation
delegation:
  model: ""
  provider: ""
  base_url: ""
  api_key: ""
  max_iterations: 50

# Approvals
approvals:
  mode: "manual"                          # "manual", "smart", "off"
  timeout: 60

# Security
security:
  redact_secrets: true
  tirith_enabled: true
  tirith_path: "tirith"
  tirith_timeout: 5
  tirith_fail_open: true
  website_blocklist:
    enabled: false

# Privacy
privacy:
  redact_pii: false

# Cron
cron:
  wrap_response: true

# Platform-specific
discord:
  require_mention: true
  auto_thread: true

# Command allowlist
command_allowlist: []                     # Permanently allowed command patterns

# Quick commands
quick_commands: {}                        # User-defined quick commands

# Personalities
personalities: {}                         # Custom personality definitions

# Skills
skills:
  external_dirs: []                       # External skill directories

# Honcho (external memory)
honcho: {}                                # Honcho overrides

# Timezone
timezone: ""                              # IANA timezone
```

### 9.3 Environment Variables

**Core:**
| Variable | Purpose | Default |
|----------|---------|---------|
| `HERMES_HOME` | Override home directory | `~/.hermes` |
| `HERMES_MAX_ITERATIONS` | Max tool-calling iterations | 90 |
| `HERMES_TIMEZONE` | IANA timezone | System timezone |
| `HERMES_YOLO_MODE` | Skip all approval prompts | false |
| `HERMES_STREAM_RETRIES` | Max stream retries | 3 |
| `MESSAGING_CWD` | Gateway working directory | Home directory |
| `SUDO_PASSWORD` | Sudo password for terminal | (none) |

**Provider keys (all optional, at least one required):**
`OPENROUTER_API_KEY`, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GLM_API_KEY`, `KIMI_API_KEY`, `MINIMAX_API_KEY`, `OPENCODE_ZEN_API_KEY`, `HF_TOKEN`, `DASHSCOPE_API_KEY`

**Tool keys (optional, enable specific tools):**
`EXA_API_KEY`, `FIRECRAWL_API_KEY`, `TAVILY_API_KEY`, `FAL_KEY`, `BROWSERBASE_API_KEY`, `BROWSERBASE_PROJECT_ID`, `HONCHO_API_KEY`, `GITHUB_TOKEN`, `ELEVENLABS_API_KEY`, `TINKER_API_KEY`, `WANDB_API_KEY`

**Messaging keys (optional, enable specific platforms):**
`TELEGRAM_BOT_TOKEN`, `DISCORD_BOT_TOKEN`, `SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN`, `MATTERMOST_URL`, `MATTERMOST_TOKEN`, `MATRIX_HOMESERVER`, `MATRIX_ACCESS_TOKEN`, `MATRIX_USER_ID`, `GATEWAY_ALLOW_ALL_USERS`

**API Server:**
`API_SERVER_ENABLED`, `API_SERVER_KEY`, `API_SERVER_PORT`, `API_SERVER_HOST`

**Webhook:**
`WEBHOOK_ENABLED`, `WEBHOOK_PORT`, `WEBHOOK_SECRET`

### 9.4 Secrets Handling

- API keys and tokens are stored in `~/.hermes/.env` (dotenv format)
- OAuth tokens are stored in `~/.hermes/auth.json`
- Secrets are redacted from logs and output when `security.redact_secrets` is enabled
- Messaging platforms do not support secure secret entry (users must configure via CLI or `.env`)

### 9.5 Profile System

Profiles allow multiple fully isolated instances:

```
hermes -p <profile_name> chat
```

**Mechanism:**
1. Parse `--profile/-p` flag before any module imports
2. Set `HERMES_HOME` to `~/.hermes/profiles/<name>`
3. All state (config, API keys, sessions, memory, skills) is scoped to this directory
4. Sticky default via `~/.hermes/active_profile`

---

## 10. External Integrations

### 10.1 LLM Providers

All providers are accessed via HTTP APIs. The system normalizes differences through:
- API mode selection (Chat Completions, Responses, Messages)
- Provider-specific parameter mapping (e.g., `max_tokens` vs `max_completion_tokens`)
- Streaming protocol adaptation
- Error handling and fallback chains

### 10.2 Web Search & Extraction

| Service | Purpose | SDK/Protocol |
|---------|---------|--------------|
| Exa | AI-native web search | Python SDK |
| Parallel | Web search and extract | Python SDK |
| Firecrawl | Web scraping | Python SDK |
| Tavily | AI-native web search | Python SDK |

### 10.3 Browser Automation

| Service | Protocol |
|---------|----------|
| Browserbase | Cloud API (WebSocket CDP) |
| Browser Use | Cloud API |

### 10.4 Image Generation

| Service | Protocol |
|---------|----------|
| Fal.ai | Python SDK (REST) |

### 10.5 Text-to-Speech / Speech-to-Text

| Service | Purpose | Protocol |
|---------|---------|----------|
| Edge TTS | Free text-to-speech | WebSocket |
| ElevenLabs | Premium TTS | REST API |
| OpenAI | TTS and STT | REST API |
| Faster Whisper | Local STT | Local library |

### 10.6 Memory Systems

| Service | Purpose | Protocol |
|---------|---------|----------|
| SQLite (built-in) | Session storage | Local file |
| Honcho | AI-native cross-session memory | REST API |
| MEMORY.md / USER.md | File-based persistent memory | Local files |

### 10.7 Smart Home

| Service | Purpose | Protocol |
|---------|---------|----------|
| Home Assistant | Smart home control | REST API |

### 10.8 External Tool Servers

| Protocol | Purpose |
|----------|---------|
| MCP (Model Context Protocol) | Dynamic tool discovery from external servers |

---

## 11. Observability

### 11.1 Logging

- **Error log:** Rotating file at `~/.hermes/logs/errors.log` (2MB, 2 backups)
- **Session logs:** JSON files at `~/.hermes/sessions/session_<id>.json`
- **Trajectory logs:** JSONL files (when enabled) for training data
- **Process logs:** Background process output buffered in memory (200KB max per process)

**Log levels:** DEBUG, INFO, WARNING, ERROR

### 11.2 Metrics

**Per-session metrics tracked:**
- Input tokens, output tokens, cache read/write tokens, reasoning tokens
- API call count
- Tool call count
- Estimated cost (USD)
- Duration

**Per-tool metrics (batch mode):**
- Call count, success count, failure count
- Error type distribution

**Reasoning metrics:**
- Turns with reasoning vs. without
- Reasoning token count

### 11.3 Health Checks

- **Gateway status file:** `~/.hermes/gateway_status.json` tracks runtime state
- **Platform connection status:** Tracked per adapter with reconnection state
- **Process registry:** Tracks active background processes with checkpoint recovery

### 11.4 Tracing

- **Tool trace:** Each subagent delegation records tool calls with argument/result sizes
- **Session lineage:** Parent-child session relationships tracked via `parent_session_id`
- **Title lineage:** Session title variants tracked for resolution (e.g., "my session", "my session #2")

---

## 12. Security Model

### 12.1 Authentication

**LLM Providers:**
- API key in HTTP header (most providers)
- OAuth bearer token (Nous Portal, OpenAI Codex, GitHub Copilot)
- OAuth token refresh handled automatically

**Messaging Platforms:**
- Bot tokens (Telegram, Discord, Slack, Mattermost)
- Access tokens (Matrix)
- Session credentials (WhatsApp via QR pairing)
- HMAC secrets (Webhook per-route)

**API Server:**
- Bearer token authentication via `API_SERVER_KEY`

**ACP:**
- Provider detection and authentication via configured credentials

**Pairing (Gateway DMs):**
- Code-based authorization for new DM conversations
- Configurable behavior: "pair" (require pairing) or "ignore" (reject unknown)

### 12.2 Authorization

**Tool-level:**
- Tools check API key availability before registration
- Terminal commands checked against dangerous pattern list
- Command approval system (manual/smart/off modes)
- Permanent allowlist for trusted commands

**File-level:**
- Sensitive path prefixes blocked (`/etc/`, `/boot/`, `/usr/lib/systemd/`)
- Sensitive exact paths blocked (`/var/run/docker.sock`)
- Internal Hermes cache/index files blocked (prompt injection prevention)
- Read loop detection (blocks after 4 consecutive identical reads)

**Platform-level:**
- Allowed user lists for Telegram and Discord
- `GATEWAY_ALLOW_ALL_USERS` for open access
- Unauthorized DM behavior configurable

**Subagent-level:**
- Children inherit restricted toolsets (dangerous tools blocked)
- Maximum delegation depth prevents infinite nesting
- Maximum concurrent children prevents resource exhaustion

### 12.3 Sensitive Data Handling

- Secrets redacted from output when `security.redact_secrets` is enabled
- PII redaction available via `privacy.redact_pii` (hashes user IDs in LLM context)
- OAuth tokens stored in separate `auth.json` file
- API keys stored in `.env` file (plaintext on disk — no encryption at rest)
- Session transcripts contain full conversation history including any sensitive data shared

### 12.4 Trust Boundaries

```
┌─────────────────────────────────────────────────────┐
│                    TRUSTED                           │
│  User input (CLI)                                   │
│  Configuration files                                │
│  Plugin code (user-installed)                       │
├─────────────────────────────────────────────────────┤
│                    SEMI-TRUSTED                      │
│  LLM responses (may contain malicious instructions) │
│  Skill content (community-authored)                 │
│  MCP server tools (externally defined)              │
│  Context files (AGENTS.md, etc. — may be modified)  │
├─────────────────────────────────────────────────────┤
│                    UNTRUSTED                         │
│  Web content (search results, scraped pages)         │
│  Platform messages (from external users)            │
│  File contents from unknown sources                 │
└─────────────────────────────────────────────────────┘
```

**Mitigations:**
- Prompt injection detection in context file loading
- Skills injected as user messages (not system prompt)
- File path validation for sensitive directories
- Command approval for dangerous terminal operations
- Website blocklist (configurable)

---

## 13. Reimplementation Guidelines

### 13.1 Language-Agnostic Design Rules

1. **Registry pattern is fundamental.** Tools, commands, and plugins must self-register. Do not manually wire components. Use a central registry that components add themselves to at load time.

2. **All tool handlers return JSON strings.** This is the contract between the agent core and tool implementations. The agent parses the JSON and includes it in the conversation.

3. **The conversation loop is synchronous by design.** Async should only be used at I/O boundaries (messaging platforms, streaming delivery). The core loop should be simple and predictable.

4. **Prompt caching is a first-class concern.** Cache invalidation dramatically increases costs. Cache agent instances per session. Do not modify system prompts, toolsets, or model configuration mid-conversation.

5. **Profile isolation via environment variable.** Set the home directory variable before any module loads. All path resolution must read from this variable, never hardcode paths.

6. **Toolsets compose recursively.** A toolset can include other toolsets. Resolution must handle cycles and produce a flat list of tool names.

7. **SQLite is the persistence layer.** Use WAL mode. Handle write contention with application-level retry. No external database server is needed.

### 13.2 Suggested Abstractions

```
AgentCore:
  - run_conversation(messages, tools, config) → Response
  - interrupt()
  - get_session_metrics() → Metrics

ToolRegistry:
  - register(name, toolset, schema, handler, check_fn)
  - get_definitions(toolsets) → [Schema]
  - dispatch(name, args) → JSON string

PlatformAdapter:
  - connect() → bool
  - disconnect()
  - send(chat_id, content, metadata) → SendResult
  - set_message_handler(callback)

TerminalEnvironment:
  - execute(command, timeout, cwd) → {output, returncode, error}
  - spawn_background(command, cwd) → ProcessSession
  - poll(session) → {status, output, exit_code}
  - kill(session)

SessionStore:
  - get_or_create_session(source) → SessionEntry
  - reset_session(session_key) → SessionEntry
  - append_message(session_id, message)
  - load_transcript(session_id) → [Message]

DeliveryRouter:
  - resolve_targets(spec, origin) → [Target]
  - deliver(content, targets) → {success, targets}
```

### 13.3 Portability Considerations

1. **File system paths:** Use the home directory variable for all state files. Support `~` expansion for user-facing paths.

2. **Process management:** The process registry and background process system assume POSIX-like process semantics. On Windows, adapt using job objects or equivalent.

3. **Terminal control:** The CLI uses terminal control libraries for rich formatting. These are UI concerns and can be replaced with platform-appropriate alternatives.

4. **SQLite:** Available in most languages via bindings. If unavailable, any ACID-compliant embedded database works (the schema is simple).

5. **Async/sync bridge:** If the target language has a different async model, ensure tool handlers can be called from the conversation loop regardless of their internal async nature.

6. **HTTP clients:** All external integrations use HTTP. Use the language's standard HTTP client with support for streaming responses.

7. **JSON:** All data interchange is JSON. Use the language's standard JSON library.

### 13.4 Implementation Order (Suggested)

1. **ToolRegistry** — Foundation for everything else
2. **Core tool implementations** — Start with file tools, terminal, web search
3. **AIAgent conversation loop** — The orchestrator
4. **SessionDB** — Persistence layer
5. **CLI** — Interactive interface
6. **Platform adapters** — Start with one (Telegram is simplest)
7. **Gateway** — Multi-platform controller
8. **Cron scheduler** — Background jobs
9. **Plugin system** — Extensibility
10. **Batch runner** — Data generation
11. **ACP adapter** — IDE integration

### 13.5 What NOT to Reimplement Initially

- Skin/theme engine (UI concern, replace with native theming)
- Kawaii spinner (UI concern)
- WhatsApp bridge (requires Node.js, complex setup)
- RL training environments (specialized use case)
- Honcho integration (optional external service)
- Mixture of Agents tool (advanced feature)
- Smart model routing (optimization, not core)

---

## 14. Open Questions / Assumptions

### Assumptions

1. **LLM API compatibility:** The system assumes all LLM providers support function/tool calling. Providers without this capability cannot be used as the main model (though they can serve as auxiliary models for specific tasks).

2. **Single-writer SQLite:** The application-level retry mechanism assumes write contention is occasional, not constant. High-concurrency scenarios (many simultaneous gateway sessions writing to the same database) may experience degraded performance.

3. **Process-local state:** All state is process-local. There is no distributed state sharing. Running multiple gateway processes requires separate profiles or separate machines.

4. **Synchronous tool execution:** Tool handlers are expected to complete within reasonable time (seconds to minutes). Very long-running tools should use the background process mechanism.

5. **Memory availability:** The system assumes sufficient memory to hold conversation history in memory. Context compression mitigates this but requires an auxiliary LLM call.

### Open Questions

1. **Plugin API stability:** The plugin hook interface (`pre_tool_call`, `post_tool_call`, etc.) is not versioned. Breaking changes to hook signatures would break existing plugins. No formal plugin API versioning exists.

2. **ACP protocol version:** The ACP adapter implements an internal protocol that may evolve independently of the agent core. Compatibility guarantees between agent core versions and IDE plugin versions are not documented.

3. **Trajectory format specification:** The JSONL trajectory format is described as "HuggingFace-compatible" but no formal schema exists. The exact fields and their types should be documented for reproducibility.

4. **Provider fallback semantics:** When a provider fails and falls back to the next, the conversation state is preserved but the model change may produce inconsistent behavior (different tool-calling patterns, different response styles). The impact on conversation quality is not measured.

5. **Session reset edge cases:** The interaction between session reset policies and active background processes is defined (never reset), but the behavior when a reset is triggered during a long-running tool execution is not fully specified.

6. **Media caching lifecycle:** Platform media (images, audio, documents) is cached with a 24-hour auto-cleanup. If a session references cached media after cleanup, the reference becomes invalid. No mechanism exists to re-fetch expired media.

7. **Scalability limits:** The system is designed for single-user or small-team use. No load testing data exists for:
   - Maximum concurrent gateway sessions
   - Maximum number of platform connections
   - Maximum conversation length before compression becomes necessary
   - Maximum tool execution throughput

8. **Error recovery:** If the gateway process crashes, in-flight conversations are lost (not persisted mid-turn). Background processes can be recovered from checkpoint, but agent state cannot.