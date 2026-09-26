# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

DLIA (Docker Log Intelligence Agent) is an AI-powered Docker log monitoring agent written in Go. It uses LLMs to analyze container logs, detect anomalies, and build a persistent knowledge base over time.

## Build Commands

All common commands are defined in `Taskfile.yml` — prefer `task <name>` over raw tool invocations.

```bash
# Build the binary
task build

# Run tests (set RACE=-race to enable race detection)
task test

# Run tests with per-file coverage enforcement (80% gate via scripts/check-coverage.sh)
task test:coverage

# Watch tests (requires gotestsum)
task test:watch

# Run the linter (uses golangci-lint with extensive config in .golangci.yml)
task lint            # CI installs it; locally run `task lint:install` first

# Format / check formatting
task fmt
task fmt:check

# Static analysis and security
task vet
task vulncheck       # locally run `task vulncheck:install` first

# Mutation testing (runs the official gremlins image via Docker, toolchain auto-selected)
task gremlins        # native binary alternative: `task gremlins:install` first

# Dependency management
task tidy
task deps:download

# Install git hooks (run once after cloning)
task install-hooks

# Run a single test (no task equivalent)
go test -v -run TestFunctionName ./path/to/package
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

#### Test-Driven Development (TDD)
- **Required**: Tests MUST be written BEFORE writing or modifying code
- **Workflow**:
  1. Write a test for the desired behavior (test fails - Red)
  2. Implement minimal code to make the test pass (Green)
  3. Refactor code while tests continue to pass (Refactor)
- Tests define expected behavior BEFORE implementation
- Code is iteratively adjusted until all tests pass

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

## Claude Code Hooks

This project uses Claude Code Hooks (`.claude/settings.json`) for automatic formatting:

- **PostToolUse Hook**: After every `Edit` or `Write` operation on `.go` files, `go fmt ./...` is automatically executed
- Timeout: 60 seconds
- The hook runs in the background and does not block the workflow

## Release Process

Uses goreleaser for cross-platform builds (Linux/Windows/macOS, amd64/arm64). Produces archives, native packages (deb, rpm, apk, archlinux), and Docker images. Version info injected via ldflags from `internal/version/`.

### Docker Images

Published to Docker Hub on release:
- `zorak1103/dlia:latest` - Multi-arch manifest (amd64 + arm64)
- `zorak1103/dlia:vX.Y.Z` - Version-specific multi-arch manifest

The Dockerfile uses the pre-built binary from goreleaser (not a multi-stage build from source). goreleaser's `dockers_v2` builds natively via buildx and lays out the build context per platform (`linux/amd64/dlia`, `linux/arm64/dlia`), so the Dockerfile copies via `ARG TARGETPLATFORM` + `COPY $TARGETPLATFORM/dlia` instead of a flat `COPY dlia`. Reproduce that layout for a local build:

```bash
# Local Docker build for testing
mkdir -p linux/amd64
GOOS=linux GOARCH=amd64 go build -o linux/amd64/dlia .
docker build --build-arg TARGETPLATFORM=linux/amd64 -t dlia:test .
docker run --rm dlia:test --version
rm -rf linux
```
