# DLIA Architecture Documentation

This directory contains PlantUML diagrams documenting the architecture and design of the Docker Log Intelligence Agent (DLIA).

## Diagrams Overview

| Diagram | Description | Purpose |
|---------|-------------|---------|
| [01-component-diagram.puml](01-component-diagram.puml) | Component/Package Diagram | Shows high-level system architecture, packages, and their relationships |
| [02-scan-sequence-diagram.puml](02-scan-sequence-diagram.puml) | Sequence Diagram | Details the flow of the `scan` command from start to finish |
| [03-class-diagram.puml](03-class-diagram.puml) | Class/Structure Diagram | Documents Go structs, interfaces, and their relationships |
| [04-pipeline-activity-diagram.puml](04-pipeline-activity-diagram.puml) | Activity Diagram | Illustrates the log processing pipeline workflow |
| [05-container-state-diagram.puml](05-container-state-diagram.puml) | State Diagram | Shows container tracking states and transitions |
| [06-data-flow-diagram.puml](06-data-flow-diagram.puml) | Data Flow Diagram | Visualizes how data moves through the system |
| [07-deployment-diagram.puml](07-deployment-diagram.puml) | Deployment Diagram | Shows deployment topology and external integrations |

## Diagram Descriptions

### 1. Component Diagram
Provides a high-level view of the DLIA system architecture:
- **cmd package**: CLI entry points (root, scan, init, state commands)
- **internal packages**: Core functionality modules
- **External systems**: Docker daemon, LLM APIs, notification services
- **File system**: Configuration, state, reports, and knowledge base

### 2. Scan Sequence Diagram
Documents the complete flow of the `dlia scan` command:
- Initialization phase (config loading, Docker connection)
- Container discovery and filtering
- Per-container processing loop
- LLM analysis (with chunking if needed)
- Report generation and knowledge base updates
- Notification handling

### 3. Class Diagram
Details the Go types and their relationships:
- **Interfaces**: `ClientInterface` for Docker and LLM, `TokenizerInterface`
- **Core structs**: `Container`, `LogEntry`, `Config`, `State`
- **Processing types**: `Pipeline`, `AnalyzeResult`, `ChatMessage`
- Shows composition, implementation, and dependency relationships

### 4. Pipeline Activity Diagram
Illustrates the log processing workflow:
- Deduplication of repeated log entries
- Token counting and context window management
- Chunking strategy for large log batches
- Map-reduce pattern for chunked analysis
- Report and knowledge base updates

### 5. Container State Diagram
Shows the lifecycle states of container tracking:
- **Unknown**: First discovery, no previous scan
- **Tracked**: Has scan history with substates (Idle, Scanning, Updated)
- **Special modes**: Lookback mode, Dry-run mode
- **Transitions**: Reset, filter application, rediscovery

### 6. Data Flow Diagram
Visualizes data movement through the system:
- **Inputs**: Docker logs, configuration files, previous analyses
- **Processing**: Deduplication, tokenization, chunking, LLM calls
- **Outputs**: Reports, knowledge base updates, notifications
- **Feedback loop**: Historical context informs new analyses

### 7. Deployment Diagram
Shows deployment topology:
- Single binary with embedded dependencies
- Docker socket integration
- LLM provider options (OpenAI, OpenRouter, Ollama)
- Notification services (SMTP, Discord, Slack, etc.)
- Scheduling options (cron, systemd, Windows Task Scheduler)

## Rendering Diagrams

### Option 1: PlantUML Online Server
Visit [PlantUML Web Server](http://www.plantuml.com/plantuml/uml/) and paste the diagram content.

### Option 2: VS Code Extension
Install the [PlantUML extension](https://marketplace.visualstudio.com/items?itemName=jebbs.plantuml):
```
ext install jebbs.plantuml
```
Then use `Alt+D` to preview diagrams.

### Option 3: Command Line
Install PlantUML and run:
```bash
# Generate PNG
plantuml -tpng *.puml

# Generate SVG
plantuml -tsvg *.puml

# Generate PDF
plantuml -tpdf *.puml
```

### Option 4: Docker
```bash
docker run --rm -v $(pwd):/data plantuml/plantuml -tpng /data/*.puml
```

## Updating Diagrams

When modifying the codebase:

1. **Adding new packages**: Update `01-component-diagram.puml`
2. **Changing workflows**: Update `02-scan-sequence-diagram.puml` or `04-pipeline-activity-diagram.puml`
3. **Adding/modifying types**: Update `03-class-diagram.puml`
4. **Changing state management**: Update `05-container-state-diagram.puml`
5. **Changing data flows**: Update `06-data-flow-diagram.puml`
6. **Deployment changes**: Update `07-deployment-diagram.puml`

## PlantUML Resources

- [PlantUML Language Reference](https://plantuml.com/guide)
- [PlantUML Quick Start](https://plantuml.com/starting)
- [Real World PlantUML Examples](https://real-world-plantuml.com/)
