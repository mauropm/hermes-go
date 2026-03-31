# Setup Guide — Hermes-Go

## First-Time Setup

### 1. Prerequisites

```bash
# Go 1.22+ required
go version

# CGO required for SQLite (usually pre-installed)
# macOS: sqlite3 comes pre-installed
# Ubuntu/Debian: sudo apt install libsqlite3-dev
# Fedora: sudo dnf install sqlite-devel
```

### 2. Build

```bash
# Using Make (recommended)
make build

# Or manually
go build -o hermes-go .
```

### 3. Run the Setup Wizard

```bash
# Interactive configuration wizard
./hermes-go setup

# Or with a specific profile
./hermes-go setup -p work
```

The wizard presents a menu of all configuration options with current values displayed. Changes are saved atomically to `~/.hermes/config.yaml` when you select "Save & Exit".

### 4. Set API Keys

API keys are loaded from environment variables or a `.env` file:

```bash
# Option A: Environment variable (one-time)
export ANTHROPIC_API_KEY=sk-ant-...

# Option B: .env file (persistent)
echo 'ANTHROPIC_API_KEY=sk-ant-...' >> ~/.hermes/.env
chmod 600 ~/.hermes/.env
```

Supported API key variables:
- `ANTHROPIC_API_KEY` — Anthropic direct
- `OPENAI_API_KEY` — OpenAI direct
- `OPENROUTER_API_KEY` — OpenRouter aggregator
- `GLM_API_KEY`, `KIMI_API_KEY`, `MINIMAX_API_KEY` — Chinese providers
- `OPENCODE_ZEN_API_KEY`, `HF_TOKEN`, `DASHSCOPE_API_KEY` — Other providers

For AWS Bedrock, no API key is needed — authentication uses your AWS credentials (see Bedrock section below).

### 5. Start Chatting

```bash
./hermes-go chat
```

## Configuration Methods

There are three ways to configure Hermes-Go, listed by priority:

### Method 1: Setup Wizard (Interactive)

```bash
./hermes-go setup           # Standalone wizard
```

Or from inside the chat:

```
/config                     # Opens config editor mid-session
```

The TUI displays all settings with current values and lets you edit each one:

```
╔══════════════════════════════════════════╗
║         Hermes Configuration             ║
╠══════════════════════════════════════════╣
║  1. Model          │ anthropic/claude-...║
║  2. Max Turns      │ 90                  ║
║  3. Memory         │ false               ║
║  4. Redact Secrets │ true                ║
║  5. Redact PII     │ false               ║
║  6. Bedrock Region │ us-east-1           ║
║  7. Bedrock Profile│                     ║
║  8. API Host       │ 127.0.0.1           ║
║  9. API Port       │ 8080                ║
║ 10. API Key Req'd  │ false               ║
╠══════════════════════════════════════════╣
║  0. Save & Exit                          ║
╚══════════════════════════════════════════╝
```

### Method 2: Direct YAML Edit

Edit `~/.hermes/config.yaml` directly:

```yaml
model: anthropic/claude-sonnet-4-20250514
provider: auto
agent:
  max_turns: 90
  tool_use_enforcement: auto
memory:
  memory_enabled: false
  user_profile_enabled: true
  memory_char_limit: 2200
  user_char_limit: 1375
security:
  redact_secrets: true
privacy:
  redact_pii: false
bedrock:
  region: us-east-1
  profile: ""
api_server:
  enabled: false
  host: 127.0.0.1
  port: 8080
```

### Method 3: Environment Variables

```bash
export HERMES_MAX_ITERATIONS=50
export API_SERVER_ENABLED=true
export API_SERVER_KEY=my-secret-token
export API_SERVER_PORT=9000
export API_SERVER_HOST=0.0.0.0
```

## AWS Bedrock Setup

### Prerequisites

1. AWS account with Bedrock access
2. AWS credentials configured (one of):
   - `~/.aws/credentials` file
   - Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`)
   - IAM role (EC2, ECS, Lambda)
   - AWS SSO

### Configure Bedrock

```bash
# Set the region in config
./hermes-go setup
# → Option 6: Bedrock Region → enter your region (e.g., us-east-1)

# Optional: use a specific AWS profile
# → Option 7: Bedrock Profile → enter profile name
```

Or via YAML:

```yaml
bedrock:
  region: us-west-2
  profile: my-aws-profile
```

### Select a Bedrock Model

From inside the chat:

```
/models              # List all available models with pricing
/models use 1        # Switch to model #1 (Claude Sonnet 4)
/models use 14       # Switch to model #14 (Nova Micro — free tier)
```

Available models include:

| # | Model | Input $/MTok | Output $/MTok | Cost Tier |
|---|-------|-------------|--------------|-----------|
| 1 | Claude Sonnet 4 | $3.00 | $15.00 | $$$ |
| 2 | Claude Opus 4 | $15.00 | $75.00 | $$$ |
| 3 | Claude 3.5 Sonnet v2 | $3.00 | $15.00 | $$$ |
| 4 | Claude 3.5 Haiku | $0.80 | $4.00 | $$ |
| 5 | Claude 3 Haiku | $0.25 | $1.25 | $ |
| 6 | Llama 4 Scout | $0.15 | $0.60 | $ |
| 7 | Llama 4 Maverick | $0.20 | $0.80 | $ |
| 8 | Llama 3.3 70B | $0.72 | $0.72 | $$ |
| 9 | Llama 3.2 11B | $0.16 | $0.16 | $ |
| 10 | Llama 3.2 3B | $0.10 | $0.10 | $ |
| 11 | Llama 3.2 1B | $0.10 | $0.10 | $ |
| 12 | Amazon Nova Pro | $0.80 | $3.20 | $$ |
| 13 | Amazon Nova Lite | $0.06 | $0.24 | $ |
| 14 | Amazon Nova Micro | $0.035 | $0.14 | FREE |
| 15 | Command R+ | $2.50 | $10.00 | $$$ |
| 16 | Command R | $0.50 | $1.50 | $ |
| 17 | Mistral Large 2 | $2.00 | $6.00 | $$$ |
| 18 | Mistral Small | $0.20 | $0.60 | $ |
| 19 | Jamba 1.5 Large | $2.00 | $8.00 | $$$ |
| 20 | Jamba 1.5 Mini | $0.20 | $0.40 | $ |
| 21 | DeepSeek R1 | $1.35 | $5.40 | $$ |

### Model Auto-Detection

When you select a Bedrock model, the provider is automatically detected from the model ID. No manual provider switching needed. Supported model ID patterns:
- `anthropic.claude-*` → Anthropic via Bedrock
- `amazon.nova-*` → Amazon
- `meta.llama*` → Meta
- `mistral.*` → Mistral
- `cohere.*` → Cohere
- `ai21.*` → AI21
- `deepseek.*` → DeepSeek

## Profiles

Profiles provide fully isolated configurations:

```bash
# Create a work profile
./hermes-go setup -p work

# Chat with work profile
./hermes-go chat -p work

# Each profile has its own:
# - ~/.hermes/profiles/work/config.yaml
# - ~/.hermes/profiles/work/.env
# - ~/.hermes/profiles/work/sessions.db
# - ~/.hermes/profiles/work/memory/
```

## Slash Commands

| Command | Description |
|---------|-------------|
| `/quit`, `/exit` | Exit the application |
| `/help` | Show available commands |
| `/session` | Show current session ID |
| `/tools` | List available tools |
| `/models` | List Bedrock models with pricing |
| `/models use <n>` | Switch to Bedrock model #n |
| `/config` | Open configuration editor |
| `/clear` | Clear the screen |

## Makefile Targets

```bash
make build          # Build the binary (runs vuln check first)
make build-static   # Build stripped static binary
make clean          # Remove binary
make test           # Run tests with race detection
make vet            # Run go vet
make run-chat       # Build and run CLI
make run-api        # Build and run API server
make run-setup      # Build and run setup wizard
make audit          # Full security audit (version + modules + vulns)
make deps           # Check for outdated dependencies
make govulncheck    # Scan for known vulnerabilities
make help           # Show all targets
```

## Directory Structure

After first run, `~/.hermes/` contains:

```
~/.hermes/
├── config.yaml       # User configuration
├── .env              # API keys (permissions: 0600)
├── sessions.db       # SQLite session store (permissions: 0600)
├── sessions.db-wal   # SQLite WAL file
├── sessions.db-shm   # SQLite shared memory
├── logs/             # Log files
├── sessions/         # Session exports
└── memory/
    └── store.json    # Local memory store (permissions: 0600)
```

## Troubleshooting

### "No API key found for provider"

Set the appropriate environment variable or add it to `~/.hermes/.env`:
```bash
export ANTHROPIC_API_KEY=sk-ant-...
# or
echo 'ANTHROPIC_API_KEY=sk-ant-...' >> ~/.hermes/.env
```

### "Failed to initialize session database"

Ensure SQLite is available and the directory is writable:
```bash
# Check CGO/SQLite
go env CGO_ENABLED  # Should be 1

# Fix permissions
chmod 700 ~/.hermes
```

### Bedrock: "load AWS config" error

Ensure AWS credentials are configured:
```bash
# Check credentials
aws sts get-caller-identity

# Or set manually
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
export AWS_REGION=us-east-1
```

### Build fails with CGO errors

```bash
# macOS (usually works out of the box)
xcode-select --install

# Ubuntu/Debian
sudo apt install gcc libsqlite3-dev

# Fedora
sudo dnf install gcc sqlite-devel
```
