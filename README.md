# DLIA - Docker Log Intelligence Agent

[![CI](https://github.com/zorak1103/dlia/actions/workflows/ci.yml/badge.svg)](https://github.com/zorak1103/dlia/actions/workflows/ci.yml)
[![Release](https://github.com/zorak1103/dlia/actions/workflows/release.yml/badge.svg)](https://github.com/zorak1103/dlia/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/zorak1103/dlia?style=flat)](https://goreportcard.com/report/github.com/zorak1103/dlia)
[![Go Version](https://img.shields.io/github/go-mod/go-version/zorak1103/dlia)](https://go.dev/)
[![Go Reference](https://pkg.go.dev/badge/github.com/zorak1103/dlia.svg)](https://pkg.go.dev/github.com/zorak1103/dlia)
[![License](https://img.shields.io/github/license/zorak1103/dlia)](LICENSE)
[![GitHub release](https://img.shields.io/github/v/release/zorak1103/dlia)](https://github.com/zorak1103/dlia/releases/latest)

**DLIA** is an AI-powered Docker log monitoring agent that uses Large Language Models (LLMs) to intelligently analyze container logs, detect anomalies, and provide contextual insights over time.

## ✨ Features

- 🤖 **Semantic Log Analysis** - Uses LLMs to understand log context, not just keyword matching.
- 📊 **Historical Context** - Tracks trends over time to detect gradual degradation.
- 🤫 **Natural Language Filtering** - Ignore routine errors or expected noise by providing instructions in plain English (e.g., "Ignore 'connection refused' during nightly backups").
- 🧠 **Self-Cleaning Knowledge Base** - Automatically "forgets" issues based on a configurable retention period (default: 30 days), keeping the knowledge base relevant.
- 🔧 **Customizable AI Prompts** - Override the default AI instructions to tune the analysis process for your specific needs.
- 🔒 **Privacy-First** - Automatic anonymization of IPs, secrets, and sensitive data.
- 🔌 **Flexible LLM Backend** - Works with OpenAI, OpenRouter, Ollama, or any OpenAI-compatible API.
- 📝 **Markdown Reports** - Human-readable persistent knowledge base.
- 🔔 **Universal Notifications** - Email, Discord, Slack, and more via Shoutrrr.
- 🐳 **Docker Native** - Direct Docker socket integration.
- ⚡ **Single Binary** - No runtime dependencies except Docker.

## 🚀 Quick Start

### Prerequisites

- Go 1.23 or higher
- Docker installed and running
- LLM API access (OpenAI, OpenRouter, or local via Ollama)

### Installation

```bash
# Clone the repository
git clone https://github.com/zorak1103/dlia.git
cd dlia

# Install dependencies
go mod download

# Build the binary
go build -o dlia.exe .

# Initialize configuration (creates config.yaml, .env, and default dirs)
./dlia.exe init

# Edit .env to add your API key
# DLIA_LLM_API_KEY=your-key-here

# Edit config.yaml with your LLM API settings
notepad config.yaml

# (Optional) Create custom prompts or ignore files (see Configuration section)

# Test with a dry run
./dlia.exe scan --dry-run

# Perform your first scan
./dlia.exe scan
```

## 📖 Usage

### Commands

#### `scan` - One-time Log Scan
Analyzes container logs once and exits. Perfect for cron jobs.

```bash
# Scan all containers
dlia scan

# Scan specific containers
dlia scan --filter "nginx.*"

# Analyze last 24 hours (ignore state)
dlia scan --lookback 24h

# Test without calling LLM
dlia scan --dry-run

# Enable LLM conversation logging for debugging
dlia scan --llmlog
```



#### `init` - Initialize Configuration
Creates default `config.yaml`, `.env` file, and the `reports` and `knowledge_base` directory structure (including `knowledge_base/services/` subdirectory). It uses embedded templates, so the binary is fully self-contained.

```bash
# Create config and directories
dlia init

# Force overwrite existing configs
dlia init --force
```

#### `state` - State Management
Manage log scan cursors.

```bash
# View current state
dlia state list

# Reset all containers
dlia state reset --force

# Reset specific containers
dlia state reset nginx --force
```

#### `cleanup` - Remove Obsolete Container Data
Clean up storage for containers that no longer exist in Docker.

```bash
# List obsolete container data
dlia cleanup list

# Preview what would be deleted (dry-run)
dlia cleanup execute --dry-run

# Remove obsolete data with confirmation
dlia cleanup execute

# Remove obsolete data without confirmation
dlia cleanup execute --force
```

**What gets cleaned:**
- State file entries (`state.json`)
- Knowledge base files (`knowledge_base/services/*.md`)
- Report directories (`reports/*/`)
- LLM log directories (`logs/llm/*/`)

**⚠️ Warning**: The cleanup command permanently deletes data. Always review the list with `cleanup list` or use `--dry-run` before executing. Use `--force` only when you're certain.

### Global Flags

- `--config` - Path to config file (default: `./config.yaml`)
- `--verbose`, `-v` - Enable verbose logging

## ⚙️ Configuration

DLIA uses a `config.yaml` file with environment variable overrides.

### config.yaml

```yaml
llm:
  base_url: "https://api.openai.com/v1"  # or OpenRouter, Ollama, etc.
  api_key: ""  # Set via DLIA_LLM_API_KEY
  model: "gpt-4o-mini"
  max_tokens: 128000

docker:
  socket_path: "" # Auto-detects for Linux, macOS, and Windows

notification:
  shoutrrr_url: ""  # smtp://, discord://, slack://, etc.
  enabled: false

output:
  reports_dir: "./reports"
  knowledge_base_dir: "./knowledge_base"
  state_file: "./state.json"
  ignore_dir: "./config/ignore"  # Directory for per-container ignore rules
  llm_log_dir: "./logs/llm"  # Directory for LLM request/response logs (--llmlog flag)
  knowledge_retention_days: 30  # Retention period for knowledge base entries (1-365 days)

privacy:
  anonymize_ips: true
  anonymize_secrets: true

# Optional: Paths to custom prompt templates.
# Leave empty to use the built-in defaults.
prompts:
  system_prompt: ""
  analysis_prompt: ""
  chunk_summary_prompt: ""
  synthesis_prompt: ""
  executive_summary_prompt: ""
```

### Environment Variables

All config options can be overridden with environment variables:

```bash
DLIA_LLM_API_KEY=sk-xxx
DLIA_LLM_MODEL=gpt-4o
DLIA_LLM_BASE_URL=https://openrouter.ai/api/v1
DLIA_NOTIFICATION_SHOUTRRR_URL=smtp://user:pass@smtp.gmail.com:587/?from=x@y.com&to=a@b.com
DLIA_PROMPTS_SYSTEM_PROMPT=./config/prompts/my_system_prompt.md
DLIA_OUTPUT_KNOWLEDGE_RETENTION_DAYS=90
```

### Advanced Filtering (Natural Language)

You can instruct the AI to ignore specific, known issues for a container by creating a Markdown file with natural language rules. This is more flexible than simple keyword or regex filtering.

1.  Create a directory named `config/ignore`.
2.  Inside, create a file named `{container_name}.md` (e.g., `my-app.md`).
3.  Write your instructions in the file.

**Example: `config/ignore/backup-service.md`**
```markdown
- Ignore any "connection refused" errors that happen between 2 AM and 4 AM, as this is the expected maintenance window.
- Disregard warnings about "disk space low" if the usage is below 95%.
```
The agent will automatically load these instructions and use them during analysis.

### Cost Optimization with Regexp Filters

DLIA supports **pre-LLM filtering** using regular expression patterns to reduce token costs by excluding irrelevant log entries before they reach the LLM. This is particularly useful for filtering out routine debug messages, health checks, or other high-volume noise.

#### Purpose

Regexp filtering happens **before** logs are sent to the LLM, providing:
- **Direct cost reduction** - Fewer tokens = lower API bills
- **Faster analysis** - Less data to process
- **Focused insights** - AI concentrates on meaningful logs

#### Configuration

Add `regexp_filters` to your `config.yaml`, with container-specific pattern lists:

```yaml
regexp_filters:
  my-app:
    enabled: true
    patterns:
      - "^DEBUG:"           # Exclude lines starting with "DEBUG:"
      - "healthcheck"       # Exclude lines containing "healthcheck"
      - "GET /metrics"      # Exclude metrics endpoint calls
  
  nginx:
    enabled: true
    patterns:
      - "\\[info\\]"        # Exclude info-level logs
      - "GET /health"       # Exclude health check requests
```

Each pattern uses **Go regexp syntax** ([documentation](https://pkg.go.dev/regexp/syntax)). Common examples:
- `^pattern` - Match at start of line
- `pattern$` - Match at end of line
- `.*pattern.*` - Match anywhere in line (implicit in substring matches)
- `\\[info\\]` - Match literal brackets (escape with `\\`)

#### Monitoring Effectiveness

Use the `--filter-stats` flag to see filtering statistics:

```bash
dlia scan --filter-stats
```

Output shows lines filtered per container:
```
Container: my-app
  Filtered: 1,234/5,000 lines (24.7%)
  
Container: nginx
  Filtered: 890/2,100 lines (42.4%)
```

This helps you:
- Verify patterns are working correctly
- Estimate cost savings (fewer lines = fewer tokens)
- Tune patterns for optimal filtering

#### Difference from Semantic Filtering

DLIA supports **two complementary filtering mechanisms**:

| Feature | Regexp Filters (This Section) | Natural Language Filtering (`config/ignore/`) |
|---------|-------------------------------|----------------------------------------------|
| **When Applied** | Before LLM processing | During AI analysis |
| **Purpose** | Cost reduction (exclude logs) | Context refinement (ignore known issues) |
| **Syntax** | Regular expressions | Plain English instructions |
| **Best For** | High-volume noise (debug logs, health checks) | Contextual patterns (maintenance windows, expected errors) |
| **Cost Impact** | Reduces tokens sent to LLM | Logs still sent, AI instructed to ignore |

**Best Practice**: Use regexp filters for volume reduction, then use semantic filtering for nuanced context-aware filtering of remaining logs.

**Example**: Filter out debug logs with regexp (`^DEBUG:`), then use semantic filtering to ignore "connection timeout during nightly backup window."

### Customizing AI Prompts

You can override any of the default prompts the AI uses for its analysis. This allows you to fine-tune its behavior, focus, and output format.

1.  Create a directory (e.g., `config/prompts`).
2.  Create a new Markdown file for the prompt you want to override (e.g., `custom_system_prompt.md`).
3.  Update `config.yaml` to point to your new file.

**Example: `config.yaml`**
```yaml
prompts:
  system_prompt: "./config/prompts/custom_system_prompt.md"
  analysis_prompt: "./config/prompts/custom_analysis.md"
```
If a path is specified but the file is not found, DLIA will log a warning and fall back to the internal default prompt.

### Knowledge Base Retention

DLIA automatically manages the knowledge base by removing old entries based on a configurable retention period. This keeps the knowledge base relevant and focused on recent issues.

#### Configuration

Set the retention period in `config.yaml`:

```yaml
output:
  knowledge_retention_days: 30  # Keep entries for 30 days (default)
```

Or via environment variable:

```bash
export DLIA_OUTPUT_KNOWLEDGE_RETENTION_DAYS=90
```

**Valid range:** 1-365 days

#### How It Works

- Each knowledge base entry includes a timestamp
- During every scan, entries older than the retention period are automatically removed
- Only affects service-specific knowledge base files (`knowledge_base/services/*.md`)
- Global summaries and reports are not affected

#### Use Cases

**Short retention (7-14 days):**
- Rapidly changing environments
- Development/staging systems
- Focus on very recent issues

**Medium retention (30-60 days):**
- Production systems (default)
- Balance between history and relevance
- Good for most use cases

**Long retention (90-365 days):**
- Compliance or audit requirements
- Long-term trend analysis
- Infrequent issues that need longer context

**Example:**

```yaml
# Development environment - keep only recent issues
output:
  knowledge_retention_days: 7

# Production - standard retention
output:
  knowledge_retention_days: 30

# Compliance - long-term retention
output:
  knowledge_retention_days: 180
```

## 🤝 Contributing

Contributions are welcome! Please feel free to submit issues and pull requests.

## 📚 References

- [Cobra CLI Framework](https://github.com/spf13/cobra)
- [Viper Configuration](https://github.com/spf13/viper)
- [Shoutrrr Notifications](https://containrrr.dev/shoutrrr/)
