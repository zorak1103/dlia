# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

DLIA (Docker Log Intelligence Agent) is an AI-powered Docker log monitoring agent written in Go. It uses LLMs to analyze container logs, detect anomalies, and build a persistent knowledge base over time.

## Build Commands

```bash
# Install dependencies
go mod download

# Build the binary
go build -o dlia.exe .

# Run tests with race detection and coverage
go test -v -race -coverprofile=coverage.out -covermode=atomic ./...

# Run linter (uses golangci-lint with extensive config in .golangci.yml)
golangci-lint run -v --timeout=5m

# Run a single test
go test -v -run TestFunctionName ./path/to/package

# Check for security vulnerabilities
govulncheck ./...
```

## Architecture Overview

### Data Flow

1. **Docker Client** (`internal/docker/`) - Connects to Docker socket, lists containers, reads logs since last scan
2. **State Management** (`internal/state/`) - Tracks scan timestamps per container in `state.json` for incremental processing
3. **Log Processing Pipeline** (`internal/chunking/`) - Deduplicates logs, applies regexp filters, chunks large log batches
4. **LLM Analysis** (`internal/llm/`) - OpenAI-compatible API client with retry logic, handles chunked summarization
5. **Knowledge Base** (`internal/knowledge/`) - Persists analysis results as Markdown files with automatic retention-based pruning
6. **Reporting** (`internal/reporting/`) - Generates timestamped Markdown reports per container
7. **Notifications** (`internal/notification/`) - Sends alerts via Shoutrrr (email, Discord, Slack, etc.)

### Key Components

**CLI Commands** (`cmd/`):
- `scan` - Main command: reads logs, analyzes with LLM, updates knowledge base
- `init` - Creates config.yaml, .env, and directory structure from embedded templates
- `state` - View/reset scan state
- `cleanup` - Remove data for deleted containers

**Configuration** (`internal/config/`):
- Uses Viper with YAML config + environment variable overrides (prefix: `DLIA_`)
- Auto-detects Docker socket (Unix socket or Windows named pipe)
- Validates regexp filter patterns at load time

**Prompt System** (`internal/prompts/`):
- Default prompts embedded in binary (`internal/prompts/defaults/*.md`)
- Custom prompts override via config paths
- Supports container-specific ignore patterns (`config/ignore/{container}.md`)

**Chunking Strategy** (`internal/chunking/pipeline.go`):
- Token counting via tiktoken for accurate budget calculation
- If logs fit in context: single LLM call
- If logs exceed context: chunk → summarize each → synthesize final analysis

### Design Patterns

- **Interfaces for testability**: `docker.Client`, `llm.Client`, `chunking.TokenizerInterface` enable mocking
- **Dependency injection**: Config passed through function parameters, not globals
- **Thread-safe state**: `state.State` uses `sync.RWMutex` for concurrent access
- **Atomic file writes**: State uses temp file + rename for crash safety

## Testing Patterns

- Unit tests use `*_test.go` alongside source files
- Integration tests have `_integration_test.go` suffix
- Mock implementations via interfaces (no external mocking frameworks)
- Test helper functions should call `t.Helper()`

## Code Conventions

- Standard Cobra pattern with `init()` for command registration (suppressed via `nolint:gochecknoinits`)
- Error wrapping with `fmt.Errorf("context: %w", err)` including source location info
- Domain-specific error types in `internal/errors/` (`ConfigurationError`, `DockerConnectionError`, `LLMAPIError`) with `Unwrap()` support for proper error chain handling
- Complexity limits (enforced by golangci-lint):
  - Cyclomatic complexity: 15 (gocyclo)
  - Cognitive complexity: 23 (gocognit)
  - Function length: 60 lines / 40 statements (funlen)
- Exit codes: 0 = success, 1 = general error/panic, 2 = config error

## Development/Debugging

- `--dry-run` flag on scan - Test log processing pipeline without calling LLM
- `--llmlog` flag on scan - Log LLM conversations to `logs/llm/` for prompt debugging (`internal/llmlogger/`)
- `--filter-stats` flag on scan - Show regexp filter statistics
- `internal/sanitize/` package - Utility for filesystem-safe container names

## Release Process

Uses goreleaser for cross-platform builds (Linux/Windows/macOS, amd64/arm64). Produces archives and native packages (deb, rpm, apk, archlinux). Version info injected via ldflags from `internal/version/`.
