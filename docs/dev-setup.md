# CodeForge — Development Setup

## Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (with WSL2 backend on Windows)
- [VS Code](https://code.visualstudio.com/) with Extension "Dev Containers" (`ms-vscode-remote.remote-containers`)
- Git

## Quick Start

1. **Clone the repository:**
   ```bash
   git clone <repo-url> CodeForge
   cd CodeForge
   ```

2. **Configure environment:**
   ```bash
   cp .env.example .env
   # Edit .env (LM Studio / Ollama endpoint, API keys, etc.)
   ```

3. **Start devcontainer:**
   - Open VS Code: `code .`
   - `Ctrl+Shift+P` → "Dev Containers: Reopen in Container"
   - Wait until `setup.sh` has finished running

4. **Start infrastructure services:**
   ```bash
   docker compose up -d
   ```
   This starts PostgreSQL, NATS JetStream, LiteLLM Proxy, docs-mcp, and playwright-mcp.

5. **Done.** The container automatically installs:
   - Go 1.24, Python 3.12, Node.js 22
   - Poetry, golangci-lint v2, goimports, Claude Code CLI
   - Python dependencies (poetry install)
   - Node dependencies (npm install)
   - Pre-commit Hooks

## Project Structure (Planned)

> **Note:** This shows the target directory structure. Directories not yet created
> are marked with `(planned)` comments or will be scaffolded in Phase 1.

```
CodeForge/
├── .claude/                  # Claude Code Config (gitignored)
│   ├── commands/             # Custom Slash Commands
│   ├── hooks/                # Pre/Post Tool-Use Hooks
│   └── settings.local.json   # Local Settings
├── .devcontainer/
│   ├── devcontainer.json     # Container Definition
│   └── setup.sh              # Post-Create Setup Script
├── data/                     # Persistent data (gitignored, auto-created by docker compose)
│   ├── docs_mcp/             # Docs MCP Index
│   ├── litellm/              # LiteLLM Runtime Data
│   ├── nats/                 # NATS JetStream Data
│   ├── playwright/           # Playwright Config
│   └── postgres/             # PostgreSQL Data
├── cmd/
│   └── codeforge/
│       ├── main.go           # Entry point, Dependency Injection
│       └── providers.go      # Blank imports of all active adapters
├── internal/
│   ├── domain/               # Core: Entities, Business Rules
│   │   ├── project/
│   │   ├── agent/
│   │   └── roadmap/
│   ├── port/                 # Interfaces + Registries
│   │   ├── gitprovider/
│   │   ├── agentbackend/
│   │   ├── specprovider/    # Spec Detection (OpenSpec, Spec Kit, Autospec)
│   │   ├── pmprovider/      # PM Sync (Plane, OpenProject, GitHub/GitLab)
│   │   ├── broadcast/       # Broadcaster interface (WS events)
│   │   ├── database/
│   │   └── messagequeue/
│   ├── adapter/              # Concrete Implementations
│   │   ├── aider/           # Aider agent backend (async NATS dispatch)
│   │   ├── gitlocal/        # Local git CLI provider
│   │   ├── http/            # REST API handlers + routes
│   │   ├── litellm/         # LiteLLM admin API client
│   │   ├── lsp/             # LSP client stub
│   │   ├── mcp/             # MCP server/client stubs
│   │   ├── nats/            # NATS JetStream adapter
│   │   ├── otel/            # OpenTelemetry stub
│   │   ├── postgres/        # PostgreSQL store + migrations
│   │   ├── ws/              # WebSocket hub + event broadcasting
│   │   └── ...              # (planned: github, gitlab, svn, goose, etc.)
│   └── service/              # Use Cases
├── workers/                  # Python AI Workers
│   └── codeforge/
│       ├── consumer/         # Queue Consumer
│       ├── agents/           # Agent Backends
│       ├── llm/              # LLM Client via LiteLLM
│       └── models/           # Data Models
├── frontend/                 # SolidJS Web GUI
│   └── src/
│       ├── features/
│       │   ├── dashboard/   # Project list, ProjectCard
│       │   ├── project/     # ProjectDetailPage, AgentPanel, TaskPanel, RunPanel, PlanPanel, LiveOutput
│       │   └── llm/         # ModelsPage (LLM model management)
│       └── api/              # API Client, Types, WebSocket
├── docs/
│   ├── README.md             # Documentation index (start here)
│   ├── todo.md               # TODO tracker for LLM agents
│   ├── architecture.md       # System Architecture + Hexagonal + Provider Registry
│   ├── dev-setup.md          # This file
│   ├── project-status.md     # Project Status & Roadmap
│   ├── tech-stack.md         # Tech Stack Details
│   ├── features/             # Feature specifications
│   │   ├── 01-project-dashboard.md
│   │   ├── 02-roadmap-feature-map.md
│   │   ├── 03-multi-llm-provider.md
│   │   └── 04-agent-orchestration.md
│   ├── architecture/         # Detailed architecture docs
│   │   └── adr/              # Architecture Decision Records
│   └── research/
│       ├── market-analysis.md# Market Research & Competitors
│       └── aider-deep-analysis.md # Aider Architecture Deep-Dive
├── litellm/
│   └── config.yaml           # LiteLLM Proxy Configuration
├── .env.example              # Environment Template
├── .gitignore
├── .golangci.yml             # Go Linter Config
├── .mcp.json                 # MCP Server for Claude Code
├── .pre-commit-config.yaml   # Pre-commit Hooks (Python, Go, TS)
├── CLAUDE.md                 # Project Context for Claude Code
├── docker-compose.yml        # Dev Services
├── LICENSE                   # AGPL-3.0
└── pyproject.toml            # Python: Poetry + Ruff + Pytest
```

## Ports

| Port | Service              | Purpose                          |
|------|----------------------|----------------------------------|
| 3000 | Frontend Dev Server  | Web GUI                          |
| 4000 | LiteLLM Proxy        | LLM Routing (OpenAI-compatible)  |
| 5432 | PostgreSQL           | Primary Database (App + LiteLLM) |
| 4222 | NATS                 | Message Queue (client connections)|
| 5173 | Vite HMR             | Hot Module Replacement           |
| 6280 | docs-mcp-server      | Documentation Indexing           |
| 8001 | playwright-mcp       | Browser Automation               |
| 8080 | Go API               | Core Service REST/WebSocket      |
| 8222 | NATS Monitoring      | NATS HTTP monitoring dashboard   |

## Running Linting Manually

```bash
# All languages via pre-commit
pre-commit run --all-files

# Python only
ruff check workers/
ruff format workers/

# Go only
go build ./cmd/codeforge/
golangci-lint run ./...

# TypeScript only
npm run lint --prefix frontend
npm run format:check --prefix frontend
```

## Running Tests

Use the central test runner script:

```bash
./scripts/test.sh              # Unit tests (Go + Python + Frontend)
./scripts/test.sh go           # Go unit tests only
./scripts/test.sh python       # Python unit tests only
./scripts/test.sh frontend     # Frontend lint + build
./scripts/test.sh integration  # Integration tests (requires docker compose services)
./scripts/test.sh all          # Everything including integration
```

Or run each suite directly:

```bash
go test -race -count=1 ./...                              # Go unit tests
cd workers && poetry run pytest -v                         # Python unit tests
npm run lint --prefix frontend && npm run build --prefix frontend  # Frontend
```

### Integration Tests

Integration tests run against real PostgreSQL (not mocked). They live in `tests/integration/` and use the `//go:build integration` build tag, so they are excluded from normal `go test ./...`.

```bash
# 1. Start required services
docker compose up -d postgres nats

# 2. Run integration tests
go test -race -count=1 -tags=integration ./tests/integration/...
```

The integration tests verify:
- Health/liveness endpoints
- Project CRUD lifecycle (create, get, list, delete)
- Input validation (missing fields return 400)
- Task CRUD lifecycle (create, get, list within a project)

## Running the Project

```bash
# 1. Start infrastructure (PostgreSQL, NATS, LiteLLM)
docker compose up -d

# 2. Go Core Service (port 8080)
go run ./cmd/codeforge/

# 3. Python Worker (connects to NATS)
cd workers && poetry run python -m codeforge.consumer

# 4. Frontend Dev Server (port 3000, proxies /api and /ws to Go Core)
npm run dev --prefix frontend
```

## Configuration

CodeForge uses a hierarchical configuration system: **defaults < YAML < environment variables**.

### Config File

Copy the example config and adjust as needed:
```bash
cp codeforge.yaml.example codeforge.yaml
```

The YAML file is optional. If missing, defaults are used. Environment variables always take precedence.

### Go Core Config (`internal/config/`)

| YAML Key | ENV Variable | Default | Description |
|---|---|---|---|
| `server.port` | `CODEFORGE_PORT` | `8080` | HTTP server port |
| `server.cors_origin` | `CODEFORGE_CORS_ORIGIN` | `http://localhost:3000` | Allowed CORS origin |
| `postgres.dsn` | `DATABASE_URL` | `postgres://codeforge:...` | PostgreSQL DSN |
| `postgres.max_conns` | `CODEFORGE_PG_MAX_CONNS` | `15` | Max DB connections |
| `postgres.min_conns` | `CODEFORGE_PG_MIN_CONNS` | `2` | Min DB connections |
| `nats.url` | `NATS_URL` | `nats://localhost:4222` | NATS server URL |
| `litellm.url` | `LITELLM_URL` | `http://localhost:4000` | LiteLLM Proxy URL |
| `litellm.master_key` | `LITELLM_MASTER_KEY` | `` | LiteLLM API key |
| `logging.level` | `CODEFORGE_LOG_LEVEL` | `info` | Log level |
| `breaker.max_failures` | `CODEFORGE_BREAKER_MAX_FAILURES` | `5` | Circuit breaker threshold |
| `breaker.timeout` | `CODEFORGE_BREAKER_TIMEOUT` | `30s` | Circuit breaker timeout |
| `rate.requests_per_second` | `CODEFORGE_RATE_RPS` | `10.0` | Rate limit RPS |
| `rate.burst` | `CODEFORGE_RATE_BURST` | `100` | Rate limit burst |
| `orchestrator.max_parallel` | `CODEFORGE_ORCH_MAX_PARALLEL` | `4` | Max parallel plan steps |
| `orchestrator.ping_pong_max_rounds` | `CODEFORGE_ORCH_PINGPONG_MAX_ROUNDS` | `3` | Ping-pong protocol max rounds |
| `orchestrator.consensus_quorum` | `CODEFORGE_ORCH_CONSENSUS_QUORUM` | `0` | Consensus quorum (0=majority) |
| `orchestrator.mode` | `CODEFORGE_ORCH_MODE` | `semi_auto` | Orchestrator mode (manual/semi_auto/full_auto) |
| `orchestrator.decompose_model` | `CODEFORGE_ORCH_DECOMPOSE_MODEL` | `openai/gpt-4o-mini` | LLM model for feature decomposition |
| `orchestrator.decompose_max_tokens` | `CODEFORGE_ORCH_DECOMPOSE_MAX_TOKENS` | `4096` | Max tokens for decomposition response |
| `orchestrator.max_team_size` | `CODEFORGE_ORCH_MAX_TEAM_SIZE` | `5` | Max agents per team |

### Python Worker Config (`workers/codeforge/config.py`)

| ENV Variable | Default | Description |
|---|---|---|
| `NATS_URL` | `nats://localhost:4222` | NATS server URL |
| `LITELLM_URL` | `http://localhost:4000` | LiteLLM Proxy URL |
| `LITELLM_MASTER_KEY` | `` | LiteLLM API key |
| `CODEFORGE_WORKER_LOG_LEVEL` | `info` | Worker log level |
| `CODEFORGE_WORKER_LOG_SERVICE` | `codeforge-worker` | Worker service name |
| `CODEFORGE_WORKER_HEALTH_PORT` | `8081` | Worker health port |

## Health Endpoints

| Endpoint | Purpose | Response |
|---|---|---|
| `GET /health` | Liveness probe (Kubernetes) | Always `200 {"status":"ok"}` |
| `GET /health/ready` | Readiness probe | `200` if all services up, `503` if any down |

The readiness endpoint checks PostgreSQL (ping), NATS (connection status), and LiteLLM (health API) with per-service latency reporting.

## NATS Subjects

The Go Core and Python Workers communicate via NATS JetStream subjects:

### Legacy Task Protocol (fire-and-forget)

| Subject | Direction | Purpose |
|---------|-----------|---------|
| `tasks.agent.*` | Go → Python | Dispatch task to agent backend |
| `tasks.result` | Python → Go | Task result from worker |
| `tasks.output` | Python → Go | Streaming output line |
| `agents.status` | Go → Frontend | Agent status update |

### Run Protocol (Phase 4B, step-by-step)

| Subject | Direction | Purpose |
|---------|-----------|---------|
| `runs.start` | Go → Python | Start a new run |
| `runs.toolcall.request` | Python → Go | Request permission for tool call |
| `runs.toolcall.response` | Go → Python | Permission decision (allow/deny/ask) |
| `runs.toolcall.result` | Python → Go | Tool execution result |
| `runs.complete` | Python → Go | Run finished |
| `runs.cancel` | Go → Python | Cancel a running run |
| `runs.output` | Python → Go | Streaming output line (run-scoped) |

The run protocol enables per-tool-call policy enforcement. Each tool call is individually approved by the Go control plane's policy engine before the Python worker executes it.

## Environment Variables

See `.env.example` for all configurable values.

| Variable                  | Default                                  | Description                     |
|---------------------------|------------------------------------------|---------------------------------|
| CODEFORGE_PORT            | 8080                                     | Go Core Service port            |
| CODEFORGE_CORS_ORIGIN     | http://localhost:3000                     | Allowed CORS origin             |
| DATABASE_URL              | postgres://codeforge:...@localhost:5432/codeforge | PostgreSQL connection string |
| NATS_URL                  | nats://localhost:4222                     | NATS server URL                 |
| LITELLM_URL               | http://localhost:4000                     | LiteLLM Proxy URL               |
| LITELLM_MASTER_KEY        | (required)                               | Master Key for LiteLLM Proxy    |
| DOCS_MCP_API_BASE         | http://host.docker.internal:1234/v1      | Embedding API Endpoint          |
| DOCS_MCP_API_KEY          | lmstudio                                 | API Key for Embeddings          |
| DOCS_MCP_EMBEDDING_MODEL  | text-embedding-qwen3-embedding-8b        | Embedding Model Name            |
| OPENAI_API_KEY            | (optional)                               | OpenAI API Key (via LiteLLM)    |
| ANTHROPIC_API_KEY         | (optional)                               | Anthropic API Key (via LiteLLM) |
| GEMINI_API_KEY            | (optional)                               | Google Gemini API Key           |
| OPENROUTER_API_KEY        | (optional)                               | OpenRouter API Key              |
| POSTGRES_PASSWORD         | (required)                               | PostgreSQL password              |
| OLLAMA_BASE_URL           | http://host.docker.internal:11434        | Ollama Endpoint (local)         |
