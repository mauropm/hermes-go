# Security Analysis & Threat Model — Hermes-Go

## 1. Architecture Summary

Hermes-Go is a secure, hardened Go reimagination of the Hermes Python AI agent framework (v0.6.0). It provides:

- **CLI**: Interactive terminal assistant with slash commands
- **API Server**: OpenAI-compatible HTTP API (`/v1/chat/completions`)
- **Core Agent**: Synchronous conversation loop with tool calling, context compression, budget tracking
- **Tool System**: Registry-driven tool discovery with strict allowlisting
- **Local Memory**: SQLite-backed learning system with sanitization and TTL
- **LLM Providers**: OpenAI-compatible and Anthropic Messages API support

**Key architectural difference from Python original**: All dangerous capabilities (terminal execution, arbitrary file access, browser automation, MCP servers) are **disabled by default** and require explicit opt-in via configuration. The Go version ships with only safe, read-only tools enabled.

---

## 2. Trust Boundaries

```
┌─────────────────────────────────────────────────────────────────┐
│  TRUSTED (originates from operator)                              │
│  • Environment variables                                         │
│  • config.yaml (written by operator)                             │
│  • CLI input (interactive user)                                  │
│  • API requests (authenticated with bearer token)                │
├─────────────────────────────────────────────────────────────────┤
│  SEMI-TRUSTED (may contain malicious content)                    │
│  • LLM responses (prompt injection vector)                       │
│  • Web search results (injection vector)                         │
│  • Memory store entries (may contain prior injections)           │
│  • Context files (AGENTS.md, etc. — may be modified by tools)    │
├─────────────────────────────────────────────────────────────────┤
│  UNTRUSTED (assume hostile)                                      │
│  • Network responses from external APIs                          │
│  • File contents from arbitrary paths                            │
│  • Any data that passed through an LLM                           │
└─────────────────────────────────────────────────────────────────┘
```

---

## 3. Threat Model (STRIDE)

### 3.1 Spoofing

| Threat | Attack Vector | Impact | Mitigation |
|--------|--------------|--------|------------|
| API impersonation | Attacker sends requests to API server without valid token | Unauthorized agent access, data exfiltration | Bearer token auth required; constant-time comparison |
| Platform impersonation | Fake messages injected into gateway | Agent acts on forged commands | HMAC verification for webhooks; platform-specific auth |
| Profile confusion | Attacker manipulates HERMES_HOME to load malicious config | Credential theft, tool injection | HERMES_HOME validated against realpath; no symlink following |

### 3.2 Tampering

| Threat | Attack Vector | Impact | Mitigation |
|--------|--------------|--------|------------|
| Memory poisoning | Malicious LLM response stored in memory, later retrieved as trusted context | Persistent prompt injection | All memory entries sanitized on write; tagged as untrusted; content filtered on read |
| Config tampering | Attacker modifies config.yaml to enable dangerous tools | Arbitrary code execution | File permissions 0600; config validated on load; dangerous tools disabled by default |
| Session hijacking | Attacker gains access to SQLite database | Conversation history theft | Database file permissions 0600; no network exposure |
| Tool result tampering | Compromised tool returns malicious JSON | Agent misdirection | All tool results validated against schema; length-limited |

### 3.3 Repudiation

| Threat | Attack Vector | Impact | Mitigation |
|--------|--------------|--------|------------|
| Action denial | User claims they didn't execute a command | Audit failure | All tool executions logged with timestamp, session ID, and arguments (redacted) |
| API request denial | Attacker denies making API request | Forensic gap | API requests logged with timestamp, source IP, and session ID (no secrets) |

### 3.4 Information Disclosure

| Threat | Attack Vector | Impact | Mitigation |
|--------|--------------|--------|------------|
| Credential exfiltration via LLM | LLM response contains instructions to exfiltrate API keys | Credential theft | LLM output filtered for secret patterns; secrets never in context; output scanner blocks known exfil patterns |
| Memory data leakage | Memory store contains sensitive data from prior conversations | Data exposure | PII detection on memory writes; size limits; optional TTL |
| Log leakage | Error logs contain secrets or full prompts | Credential exposure | Structured logging with secret redaction; prompts never logged in full |
| SSRF via tool | Tool fetches user-supplied URL, accesses internal services | Internal network scanning | No dynamic URLs from user input; allowlisted domains only |

### 3.5 Denial of Service

| Threat | Attack Vector | Impact | Mitigation |
|--------|--------------|--------|------------|
| Token budget exhaustion | LLM enters infinite tool-calling loop | Cost explosion, service unavailability | Hard iteration limit (default 90); token budget tracking; per-session cost limits |
| Memory bloat | Attacker floods memory store with large entries | Storage exhaustion, slow retrieval | Per-entry size limit (4KB); total store size limit (10MB); deduplication |
| Context overflow | Conversation history exceeds model context window | Agent failure, cost spike | Automatic context compression; hard message count limit |
| API rate abuse | Unauthenticated requests flood API server | Service degradation | Rate limiting per IP; authentication required; request size limits |

### 3.6 Elevation of Privilege

| Threat | Attack Vector | Impact | Mitigation |
|--------|--------------|--------|------------|
| Prompt injection → tool execution | LLM instructed to call dangerous tools | Arbitrary code execution | Dangerous tools disabled by default; tool allowlisting; LLM output validation |
| Subagent escape | Child agent inherits parent's full toolset | Privilege escalation | Child agents get restricted toolsets; max delegation depth 2 |
| Plugin injection | Malicious plugin loaded from plugin directory | Arbitrary code execution | Plugin loading disabled by default; no dynamic code loading in Go build |
| Path traversal | Tool accepts user path, accesses `/etc/shadow` | File system compromise | Strict path validation; allowlisted directories only; no `..` resolution |

---

## 4. LLM Safety Layer

### 4.1 Input Sanitization (Before LLM)

- **Length limits**: User input capped at 100,000 characters
- **Unicode sanitization**: Invalid surrogates and control characters stripped
- **Context isolation**: System prompt is immutable; user content never appended to system prompt
- **Memory tagging**: Retrieved memory entries wrapped in `<memory>` tags and explicitly labeled as potentially unreliable

### 4.2 Output Filtering (After LLM)

- **Secret pattern detection**: Regex scan for API key patterns, file paths to sensitive locations
- **Exfiltration pattern detection**: Blocks responses containing curl/wget with URLs, base64-encoded data with network instructions
- **Command injection detection**: Blocks shell command patterns in text responses
- **Length limit**: Response capped at configurable maximum (default 64KB)

### 4.3 System Prompt Protection

- System prompt is **immutable** after agent initialization
- User cannot override system prompt via input
- Memory content injected as **user messages**, not system instructions
- Tool results injected as **tool role** messages with strict schema

---

## 5. Local Memory Protection

### 5.1 Write Path

1. Content received → length check (max 4KB per entry)
2. Unicode sanitization → strip control characters, invalid surrogates
3. Secret redaction → mask API key patterns
4. Trust tag → mark as `untrusted` (from LLM) or `trusted` (from operator)
5. Deduplication → hash-based dedup against existing entries
6. Store → SQLite with TTL

### 5.2 Read Path

1. Query → retrieve entries
2. TTL check → expire entries past their TTL
3. Content filter → re-scan for injection patterns
4. Context assembly → wrap in explicit trust boundary markers
5. Inject → as user message, never system prompt

### 5.3 Size Constraints

- Max entry size: 4,096 bytes
- Max total store: 10 MB
- Max entries per key: 50
- Default TTL: 30 days
- Oldest entries evicted on overflow

---

## 6. Secrets Management

### 6.1 Loading

- Secrets loaded **only** from environment variables or `.env` file
- `.env` file permissions enforced to `0600`
- No secrets hardcoded in binary

### 6.2 Storage

- API keys stored in memory only (not persisted beyond process lifetime)
- SQLite database stores session metadata, **not** API keys
- OAuth tokens stored separately in `auth.json` with `0600` permissions

### 6.3 Logging

- Structured logging with explicit secret redaction
- Known secret patterns masked: `sk-*`, `Bearer *`, `key=...`
- Full prompts **never** logged
- API request/response bodies **never** logged

### 6.4 Transmission

- All LLM API calls use HTTPS only
- TLS verification **cannot** be disabled
- Timeouts enforced (default 60s)

---

## 7. File System Safety

### 7.1 Path Validation

- All file paths resolved to absolute paths
- Path traversal (`..`) detected and rejected
- Access restricted to explicitly allowlisted directories
- Sensitive paths blocked: `/etc/`, `/boot/`, `/proc/`, `/sys/`, `/var/run/docker.sock`

### 7.2 Directory Constraints

- Default working directory: current directory at startup
- File read/write restricted to HERMES_HOME and explicitly configured directories
- No arbitrary file reads from user input

---

## 8. Network Security

### 8.1 Outbound Calls

- HTTPS only for all LLM provider calls
- Hardcoded base URLs (no dynamic URLs from user input)
- Connection timeouts: 10s connect, 60s read
- Retry with exponential backoff (max 3 retries)

### 8.2 Inbound (API Server)

- Bearer token authentication required
- Rate limiting: 100 requests per minute per IP
- Request body size limit: 1MB
- Unknown JSON fields rejected
- CORS disabled by default

---

## 9. Known Limitations

### 9.1 Unavoidable Risks

1. **LLM responses are inherently untrusted**: No filter is perfect. A sufficiently sophisticated LLM could encode exfiltration instructions in ways that bypass pattern matching. Mitigation: defense in depth — disable dangerous tools by default, so even if injection succeeds, impact is limited.

2. **API keys in memory**: Keys are loaded into process memory and could be read by a compromised host. Mitigation: use short-lived tokens where possible; restrict file permissions.

3. **SQLite not encrypted at rest**: Session data (including conversation history) is stored in plaintext. Mitigation: file permissions `0600`; deploy on encrypted filesystem if required.

### 9.2 Deliberately Excluded (vs Python Original)

The following capabilities from the Python original are **not implemented** in this Go version due to security risk:

| Feature | Reason for Exclusion |
|---------|---------------------|
| Terminal command execution | Arbitrary code execution risk; requires sandboxing infrastructure |
| Browser automation | High attack surface; requires isolated environment |
| MCP server connections | Dynamic tool discovery from untrusted servers |
| Plugin system (dynamic loading) | Go doesn't support safe dynamic code loading |
| Docker/SSH/Modal execution environments | Complex attack surface; out of scope for secure baseline |
| 15+ messaging platform adapters | Each adapter is a potential attack surface; implement only what you need |

These can be added later with appropriate security controls, but the baseline ships with a **minimal, auditable surface area**.

### 9.3 Dependencies

| Dependency | Justification |
|-----------|--------------|
| `github.com/mattn/go-sqlite3` | SQLite support for session storage and memory. CGO required. |
| `gopkg.in/yaml.v3` | YAML config parsing. Minimal, well-audited. |
| `github.com/joho/godotenv` | `.env` file loading. Minimal, widely used. |

All other functionality uses the Go standard library.

---

## 10. Security Checklist

- [x] No hardcoded secrets
- [x] Secrets loaded from environment only
- [x] Secrets redacted in logs
- [x] HTTPS-only for external calls
- [x] TLS verification enforced
- [x] Input length limits enforced
- [x] Unknown JSON fields rejected
- [x] Path traversal prevented
- [x] Sensitive paths blocked
- [x] Dangerous tools disabled by default
- [x] LLM output filtering implemented
- [x] Memory sanitization on write and read
- [x] Memory TTL and size limits
- [x] Token budget tracking
- [x] Iteration limits enforced
- [x] Rate limiting on API server
- [x] Bearer token authentication
- [x] File permissions enforced (0600)
- [x] Structured logging without secrets
- [x] System prompt immutable
- [x] Context isolation for memory
- [x] Subagent tool restrictions
- [x] Unicode sanitization
- [x] SQL injection prevented (parameterized queries)
