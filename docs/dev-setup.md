# CodeForge -- Development Setup

### Purpose

This document covers prerequisites, project structure, configuration, and daily workflows for developing CodeForge locally.

### Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (with WSL2 backend on Windows)
- [VS Code](https://code.visualstudio.com/) with Extension "Dev Containers" (`ms-vscode-remote.remote-containers`)
- Git

### Quick Start

Clone the repository:

```bash
git clone <repo-url> CodeForge
cd CodeForge
```

Configure the environment:

```bash
cp .env.example .env
# Edit .env (LM Studio / Ollama endpoint, API keys, etc.)
```

Start the devcontainer by opening VS Code (`code .`), then run `Ctrl+Shift+P` and select "Dev Containers: Reopen in Container". Wait until `setup.sh` has finished running.

**Infrastructure services start automatically** via `setup.sh`. The devcontainer is connected to the `codeforge` Docker network so the Go backend can reach services by container name (`codeforge-postgres`, `codeforge-nats`, `codeforge-litellm`). The env vars `DATABASE_URL`, `NATS_URL`, `LITELLM_URL`, and `LITELLM_MASTER_KEY` are pre-configured in `devcontainer.json` with no manual setup needed.

The container automatically installs Go 1.24, Python 3.12, Node.js 22, Poetry, golangci-lint v2, goimports, Claude Code CLI, Python dependencies (poetry install), Node dependencies (npm install), and Pre-commit Hooks.

### Project Structure

```text
CodeForge/
├── .claude/                  # Claude Code Config (gitignored)
│   ├── commands/             # Custom Slash Commands
│   ├── hooks/                # Pre/Post Tool-Use Hooks
│   └── settings.local.json   # Local Settings
├── .devcontainer/
│   ├── devcontainer.json     # Container Definition
│   └── setup.sh              # Post-Create Setup Script
├── .github/
│   └── workflows/
│       ├── ci.yml            # Go + Python + Frontend CI
│       └── docker-build.yml  # Docker image builds (ghcr.io)
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
│   ├── config/               # Hierarchical config system (defaults < YAML < ENV < CLI)
│   ├── domain/               # Core: Entities, Business Rules
│   │   ├── agent/            # Agent + Team models
│   │   ├── context/          # Context pack (token budget management)
│   │   ├── cost/             # Cost aggregation models
│   │   ├── errors.go         # Sentinel errors (ErrNotFound, ErrConflict)
│   │   ├── event/            # Agent event types (22+ types)
│   │   ├── mcp/             # MCP domain types (ServerDef, ServerTool, validation)
│   │   ├── plan/             # Execution plans (DAG scheduling)
│   │   ├── policy/           # Policy profiles, presets, validation
│   │   ├── project/          # Project entity
│   │   ├── resource/         # Resource limits (shared across layers)
│   │   ├── roadmap/          # Roadmap, Milestone, Feature
│   │   ├── run/              # Run entity, ToolCall, Stall tracker
│   │   └── task/             # Task entity
│   ├── git/                  # Git worker pool (semaphore-bounded)
│   ├── logger/               # Async slog JSON logging
│   ├── middleware/            # HTTP middleware (request ID, tenant, rate limit, idempotency, deprecation)
│   ├── port/                 # Interfaces + Registries
│   │   ├── agentbackend/     # Agent backend interface + registry
│   │   ├── broadcast/        # Broadcaster interface (WS events)
│   │   ├── cache/            # Cache interface (Get/Set/Delete)
│   │   ├── database/         # Store interface (80+ methods)
│   │   ├── eventstore/       # Event store interface + trajectory types
│   │   ├── gitprovider/      # Git provider interface + registry
│   │   ├── messagequeue/     # Message queue interface + schemas
│   │   ├── pmprovider/       # PM provider interface + registry
│   │   └── specprovider/     # Spec provider interface + registry
│   ├── adapter/              # Concrete Implementations
│   │   ├── aider/            # Aider agent backend
│   │   ├── githubpm/         # GitHub Issues PM provider (gh CLI)
│   │   ├── gitlocal/         # Local git CLI provider
│   │   ├── http/             # REST API handlers + routes (80+ endpoints)
│   │   ├── litellm/          # LiteLLM admin API client
│   │   ├── markdownspec/     # Markdown spec provider (ROADMAP.md)
│   │   ├── natskv/           # NATS JetStream KV cache adapter (L2)
│   │   ├── nats/             # NATS JetStream adapter
│   │   ├── openspec/         # OpenSpec spec provider (openspec/ dir)
│   │   ├── postgres/         # PostgreSQL store + 17 migrations
│   │   ├── ristretto/        # Ristretto in-process cache adapter (L1)
│   │   ├── tiered/           # Tiered cache (L1 + L2)
│   │   └── ws/               # WebSocket hub + event broadcasting
│   ├── resilience/           # Circuit breaker
│   ├── secrets/              # Secrets vault with SIGHUP reload
│   └── service/              # Use Cases (Runtime, Orchestrator, Policy, etc.)
├── workers/                  # Python AI Workers
│   └── codeforge/
│       ├── consumer.py       # NATS queue consumer (all subjects)
│       ├── executor.py       # Agent execution (runtime protocol)
│       ├── graphrag.py       # GraphRAG code graph builder + searcher
│       ├── llm.py            # LiteLLM async client (completions, embeddings)
│       ├── mcp_models.py     # Pydantic MCP models (MCPServerDef, MCPTool)
│       ├── mcp_workbench.py  # MCP workbench (multi-server, BM25 recommender)
│       ├── pricing.py        # Fallback model pricing table
│       ├── quality_gate.py   # Test/lint gate executor
│       ├── repo_map.py       # tree-sitter repo map generator
│       ├── retrieval.py      # Hybrid retrieval (BM25 + semantic + sub-agent)
│       ├── runtime.py        # Runtime client (Go <-> Python protocol)
│       └── models.py         # Pydantic data models
├── frontend/                 # SolidJS Web GUI
│   ├── e2e/                  # Playwright E2E tests (5 spec files)
│   ├── nginx.conf            # Production nginx config (SPA + API proxy)
│   ├── playwright.config.ts  # Playwright configuration
│   └── src/
│       ├── features/
│       │   ├── dashboard/    # Project list, ProjectCard
│       │   ├── project/      # ProjectDetailPage, AgentPanel, TaskPanel, RunPanel,
│       │   │                 # PlanPanel, PolicyPanel, RepoMapPanel, RetrievalPanel,
│       │   │                 # RoadmapPanel, TrajectoryPanel, CostSection, LiveOutput
│       │   ├── llm/          # ModelsPage (LLM model management)
│       │   ├── mcp/          # MCPServersPage (MCP server management)
│       │   └── cost/         # CostDashboardPage (global cost overview)
│       └── api/              # API Client, Types, WebSocket
├── scripts/
│   ├── test.sh               # Unified test runner (go/python/frontend/integration/e2e)
│   ├── logs.sh               # Docker log viewer helper
│   ├── backup-postgres.sh          # PostgreSQL backup script
│   ├── restore-postgres.sh         # PostgreSQL restore script
│   └── setup-branch-protection.sh  # GitHub branch protection for main
├── configs/
│   └── model_pricing.yaml    # Fallback LLM pricing table
├── tests/
│   └── integration/          # Integration tests (real PostgreSQL, build-tagged)
├── docs/                     # Documentation
├── litellm/
│   └── config.yaml           # LiteLLM Proxy Configuration
├── .env.example              # Environment Template
├── .dockerignore             # Docker build exclusions
├── .golangci.yml             # Go Linter Config (v2)
├── .mcp.json                 # MCP Server for Claude Code
├── .pre-commit-config.yaml   # Pre-commit Hooks (15 hooks)
├── CLAUDE.md                 # Project Context for Claude Code
├── Dockerfile                # Go Core multi-stage build
├── Dockerfile.worker         # Python Worker image
├── Dockerfile.frontend       # Frontend nginx image
├── codeforge.yaml.example    # Config file template (all fields documented)
├── docker-compose.yml        # Dev Services
├── docker-compose.prod.yml   # Production Services (6 containers)
├── LICENSE                   # AGPL-3.0
├── go.mod / go.sum           # Go module files
└── pyproject.toml            # Python: Poetry + Ruff + Pytest
```

### Ports

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
| 3001 | MCP Server           | MCP Streamable HTTP (when enabled)|

### Running Linting Manually

```bash
# All languages via pre-commit (15 hooks)
pre-commit run --all-files

# Python only (ruff with 17 rule groups including security, complexity, performance)
ruff check workers/
ruff format workers/

# Go only (golangci-lint v2 with 17 linters including gosec, revive, errorlint)
go build ./cmd/codeforge/
golangci-lint run ./...

# TypeScript only (ESLint strict + stylistic + import sorting)
npm run lint --prefix frontend
npm run format:check --prefix frontend
```

#### Linter Rule Summary

Python (ruff): F, E, W, I, N, UP, B, SIM, S (bandit security), C4, C90 (complexity 12), PERF, PIE, RET, FURB, LOG, T20, PT

Go (golangci-lint): errcheck, govet, staticcheck, unused, ineffassign, gocritic, misspell, unconvert, unparam, gosec, bodyclose, noctx, errorlint, revive (18 rules), fatcontext, dupword, durationcheck

**TypeScript** (ESLint): typescript-eslint strict + stylistic configs, simple-import-sort for imports/exports

### Running Tests

Use the central test runner script.

```bash
./scripts/test.sh              # Unit tests (Go + Python + Frontend)
./scripts/test.sh go           # Go unit tests only
./scripts/test.sh python       # Python unit tests only
./scripts/test.sh frontend     # Frontend lint + build
./scripts/test.sh integration  # Integration tests (requires docker compose services)
./scripts/test.sh migrations   # Migration rollback tests only (requires docker compose services)
./scripts/test.sh e2e          # E2E browser tests (requires full stack running)
./scripts/test.sh all          # Everything including integration and E2E
```

Or run each suite directly.

```bash
go test -race -count=1 ./...                              # Go unit tests
cd workers && poetry run pytest -v                         # Python unit tests
npm run lint --prefix frontend && npm run build --prefix frontend  # Frontend
```

#### E2E Browser Tests

E2E tests use Playwright and require the full stack to be running (Go backend + frontend dev server + infrastructure).

```bash
# One-time setup
cd frontend && npm install && npx playwright install --with-deps chromium

# Prerequisites: full stack running
docker compose up -d
go run ./cmd/codeforge/ &
cd frontend && npm run dev &

# Run tests
./scripts/test.sh e2e                           # Via test runner
cd frontend && npm run test:e2e                  # Directly
cd frontend && npm run test:e2e:headed           # See browser
cd frontend && npm run test:e2e:report           # View HTML report
```

Tests cover health checks (3), sidebar navigation (4), project CRUD (5), cost dashboard (2), and models page (3), totaling 17 tests.

#### Integration Tests

Integration tests run against real PostgreSQL (not mocked). They live in `tests/integration/` and use the `//go:build integration` build tag, so they are excluded from normal `go test ./...`.

```bash
# 1. Start required services
docker compose up -d postgres nats

# 2. Run integration tests
go test -race -count=1 -tags=integration ./tests/integration/...
```

The integration tests verify health/liveness endpoints, project CRUD lifecycle (create, get, list, delete), input validation (missing fields return 400), and task CRUD lifecycle (create, get, list within a project).

### Running the Project

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

### Configuration

CodeForge uses a hierarchical configuration system: defaults < YAML < environment variables < CLI flags.

#### Config File

Copy the example config and adjust as needed.

```bash
cp codeforge.yaml.example codeforge.yaml
```

The YAML file is optional. If missing, defaults are used. Environment variables override YAML, and CLI flags override everything.

#### CLI Flags

The Go Core binary accepts the following command-line flags (highest precedence).

| Flag | Shorthand | Description |
|---|---|---|
| `--config` | `-c` | Path to YAML config file (default: `codeforge.yaml`) |
| `--port` | `-p` | HTTP server port |
| `--log-level` | | Logging level (`debug`, `info`, `warn`, `error`) |
| `--dsn` | | PostgreSQL connection string |
| `--nats-url` | | NATS server URL |

Example:

```bash
./codeforge --port 9090 --log-level debug -c /etc/codeforge/config.yaml
```

#### Go Core Config (`internal/config/`)

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
| `litellm.conversation_model` | `CODEFORGE_CONVERSATION_MODEL` | `groq/llama-3.1-8b` | LLM model for chat conversations |
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
| `orchestrator.subagent_model` | `CODEFORGE_ORCH_SUBAGENT_MODEL` | `openai/gpt-4o-mini` | LLM model for sub-agent query expansion/rerank |
| `orchestrator.subagent_max_queries` | `CODEFORGE_ORCH_SUBAGENT_MAX_QUERIES` | `5` | Max expanded queries per sub-agent search |
| `orchestrator.subagent_rerank` | `CODEFORGE_ORCH_SUBAGENT_RERANK` | `true` | Enable LLM-based result reranking |
| `rate.cleanup_interval` | `CODEFORGE_RATE_CLEANUP_INTERVAL` | `5m` | Stale rate-limit bucket cleanup interval |
| `rate.max_idle_time` | `CODEFORGE_RATE_MAX_IDLE_TIME` | `10m` | Remove IP buckets idle longer than this |
| `orchestrator.graph_enabled` | `CODEFORGE_ORCH_GRAPH_ENABLED` | `false` | Enable GraphRAG structural code graph |
| `orchestrator.graph_max_hops` | `CODEFORGE_ORCH_GRAPH_MAX_HOPS` | `2` | Max BFS hops for graph traversal |
| `orchestrator.graph_top_k` | `CODEFORGE_ORCH_GRAPH_TOP_K` | `10` | Top-K results for graph search |
| `orchestrator.graph_hop_decay` | `CODEFORGE_ORCH_GRAPH_HOP_DECAY` | `0.7` | Score decay per hop (0.0-1.0) |
| `git.max_concurrent` | `CODEFORGE_GIT_MAX_CONCURRENT` | `5` | Max concurrent git CLI operations |
| `mcp.enabled` | `CODEFORGE_MCP_ENABLED` | `false` | Enable MCP integration |
| `mcp.servers_dir` | `CODEFORGE_MCP_SERVERS_DIR` | `` | Directory with MCP server YAML definitions |
| `mcp.server_port` | `CODEFORGE_MCP_SERVER_PORT` | `3001` | Port for built-in MCP server |
| `auth.enabled` | `CODEFORGE_AUTH_ENABLED` | `false` | Enable JWT authentication |
| `auth.jwt_secret` | `CODEFORGE_AUTH_JWT_SECRET` | (auto) | HMAC-SHA256 signing key |
| `auth.access_token_expiry` | `CODEFORGE_AUTH_ACCESS_TOKEN_EXPIRY` | `15m` | Access token lifetime |
| `auth.refresh_token_expiry` | `CODEFORGE_AUTH_REFRESH_TOKEN_EXPIRY` | `168h` | Refresh token lifetime (7d) |
| `auth.bcrypt_cost` | `CODEFORGE_AUTH_BCRYPT_COST` | `12` | Bcrypt work factor |
| `auth.default_admin_email` | `CODEFORGE_AUTH_DEFAULT_ADMIN_EMAIL` | `admin@localhost` | Seed admin email |
| `auth.default_admin_pass` | `CODEFORGE_AUTH_DEFAULT_ADMIN_PASS` | `changeme123` | Seed admin password |

#### Python Worker Config (`workers/codeforge/config.py`)

| ENV Variable | Default | Description |
|---|---|---|
| `NATS_URL` | `nats://localhost:4222` | NATS server URL |
| `LITELLM_URL` | `http://localhost:4000` | LiteLLM Proxy URL |
| `LITELLM_MASTER_KEY` | `` | LiteLLM API key |
| `CODEFORGE_WORKER_LOG_LEVEL` | `info` | Worker log level |
| `CODEFORGE_WORKER_LOG_SERVICE` | `codeforge-worker` | Worker service name |
| `CODEFORGE_WORKER_HEALTH_PORT` | `8081` | Worker health port |

### Health Endpoints

| Endpoint | Purpose | Response |
|---|---|---|
| `GET /health` | Liveness probe (Kubernetes) | Always `200 {"status":"ok"}` |
| `GET /health/ready` | Readiness probe | `200` if all services up, `503` if any down |

The readiness endpoint checks PostgreSQL (ping), NATS (connection status), and LiteLLM (health API) with per-service latency reporting.

### NATS Subjects

The Go Core and Python Workers communicate via NATS JetStream subjects.

#### Legacy Task Protocol (fire-and-forget)

| Subject | Direction | Purpose |
|---------|-----------|---------|
| `tasks.agent.*` | Go -> Python | Dispatch task to agent backend |
| `tasks.result` | Python -> Go | Task result from worker |
| `tasks.output` | Python -> Go | Streaming output line |
| `agents.status` | Go -> Frontend | Agent status update |

#### Run Protocol (Phase 4B, step-by-step)

| Subject | Direction | Purpose |
|---------|-----------|---------|
| `runs.start` | Go -> Python | Start a new run |
| `runs.toolcall.request` | Python -> Go | Request permission for tool call |
| `runs.toolcall.response` | Go -> Python | Permission decision (allow/deny/ask) |
| `runs.toolcall.result` | Python -> Go | Tool execution result |
| `runs.complete` | Python -> Go | Run finished |
| `runs.cancel` | Go -> Python | Cancel a running run |
| `runs.output` | Python -> Go | Streaming output line (run-scoped) |

The run protocol enables per-tool-call policy enforcement. Each tool call is individually approved by the Go control plane's policy engine before the Python worker executes it.

#### Retrieval Protocol (Phase 6B-6D)

| Subject | Direction | Purpose |
|---------|-----------|---------|
| `retrieval.build.request` | Go -> Python | Build retrieval index (BM25 + embeddings) |
| `retrieval.build.result` | Python -> Go | Index build result |
| `retrieval.search.request` | Go -> Python | Hybrid search query |
| `retrieval.search.result` | Python -> Go | Search results |
| `retrieval.agent.search.request` | Go -> Python | Sub-agent search (LLM query expansion + rerank) |
| `retrieval.agent.search.result` | Python -> Go | Sub-agent search results |
| `graph.build.request` | Go -> Python | Build structural code graph |
| `graph.build.result` | Python -> Go | Graph build result |
| `graph.search.request` | Go -> Python | BFS graph traversal from seed symbols |
| `graph.search.result` | Python -> Go | Graph search results |

#### MCP Protocol (Phase 15)

| Subject | Direction | Purpose |
|---------|-----------|---------|
| `mcp.server.status` | Python -> Go | MCP server connection status update |
| `mcp.tools.discovered` | Python -> Go | Tools discovered on MCP server |

### Logging

CodeForge uses structured JSON logging across all services with Docker-native log management.

#### Log Access

```bash
# Follow all service logs
docker compose logs -f

# Single service
docker compose logs -f codeforge

# Filter by level (requires jq)
docker compose logs codeforge 2>&1 | jq 'select(.level == "ERROR")'

# Filter by request ID across all services
docker compose logs 2>&1 | jq 'select(.request_id == "your-request-id")'
```

#### Helper Script

```bash
./scripts/logs.sh tail              # Follow all logs
./scripts/logs.sh errors            # Only ERROR level
./scripts/logs.sh service codeforge # Single service
./scripts/logs.sh request abc-123   # By request ID across services
```

#### Log Level Configuration

| Service | Config Key | Env Variable | Default |
|---|---|---|---|
| Go Core | `logging.level` | `CODEFORGE_LOG_LEVEL` | `info` |
| Python Workers | -- | `CODEFORGE_WORKER_LOG_LEVEL` | `info` |

Valid levels: `debug`, `info`, `warn`, `error`

#### Log Format

All services emit structured JSON to stdout.

```json
{"time":"2026-02-17T14:30:00Z","level":"INFO","service":"codeforge","msg":"request handled","request_id":"abc-123","method":"GET","path":"/api/v1/projects"}
```

#### Request ID Propagation

Every HTTP request gets a UUID (`X-Request-ID` header). This ID propagates through Go Core HTTP handler (logger context), NATS message headers (`X-Request-ID`), Python Worker (structlog context), and back to Go Core via NATS response. Use the request ID to trace a single operation across all services.

#### Log Rotation

Docker handles log rotation automatically via the `json-file` driver. Each service gets max 10 MB per log file with max 3 files (30 MB total per service). This is configured in `docker-compose.yml` via the `x-logging` anchor.

### Docker Production Build

CodeForge ships with multi-stage Dockerfiles for all three services.

#### Building Images

```bash
# Go Core (multi-stage: golang:1.24-alpine -> alpine:3.21)
docker build -t codeforge-core .

# Python Worker (python:3.12-slim, poetry, non-root user)
docker build -t codeforge-worker -f Dockerfile.worker .

# Frontend (node:22-alpine build -> nginx:alpine serve)
docker build -t codeforge-frontend -f Dockerfile.frontend .
```

#### Production Compose

```bash
# Start all 6 services (core, worker, frontend, postgres, nats, litellm)
docker compose -f docker-compose.prod.yml up -d

# View logs
docker compose -f docker-compose.prod.yml logs -f

# Stop
docker compose -f docker-compose.prod.yml down
```

Production compose differences from dev include named volumes for data persistence, health checks on all services, `restart: unless-stopped` for auto-recovery, tuned PostgreSQL (256MB shared_buffers, optimized WAL settings), and no dev-only services (docs-mcp, playwright).

#### CI/CD

GitHub Actions automatically builds and pushes Docker images to `ghcr.io` on push to `main`/`staging` and on version tags. See `.github/workflows/docker-build.yml`.

### Environment Variables

See `.env.example` for all configurable values.

| Variable                  | Default                                  | Description                     |
|---------------------------|------------------------------------------|---------------------------------|
| CODEFORGE_PORT            | 8080                                     | Go Core Service port            |
| CODEFORGE_CORS_ORIGIN     | http://localhost:3000                     | Allowed CORS origin             |
| DATABASE_URL              | postgres://...@codeforge-postgres:5432/codeforge (devcontainer) | PostgreSQL connection string |
| NATS_URL                  | nats://codeforge-nats:4222 (devcontainer) | NATS server URL                 |
| LITELLM_URL               | http://codeforge-litellm:4000 (devcontainer) | LiteLLM Proxy URL               |
| LITELLM_MASTER_KEY        | sk-codeforge-dev (devcontainer)          | Master Key for LiteLLM Proxy    |
| DOCS_MCP_API_BASE         | http://host.docker.internal:1234/v1      | Embedding API Endpoint          |
| DOCS_MCP_API_KEY          | lmstudio                                 | API Key for Embeddings          |
| DOCS_MCP_EMBEDDING_MODEL  | text-embedding-qwen3-embedding-8b        | Embedding Model Name            |
| OPENAI_API_KEY            | (optional)                               | OpenAI API Key (via LiteLLM)    |
| ANTHROPIC_API_KEY         | (optional)                               | Anthropic API Key (via LiteLLM) |
| GEMINI_API_KEY            | (optional)                               | Google Gemini API Key           |
| GROQ_API_KEY              | (optional)                               | Groq API Key (fast inference)   |
| MISTRAL_API_KEY           | (optional)                               | Mistral AI API Key              |
| OPENROUTER_API_KEY        | (optional)                               | OpenRouter API Key              |
| POSTGRES_PASSWORD         | (required)                               | PostgreSQL password              |
| OLLAMA_BASE_URL           | http://host.docker.internal:11434        | Ollama Endpoint (local)         |
| CODEFORGE_OTEL_ENABLED    | false                                    | Enable OpenTelemetry tracing    |
| CODEFORGE_OTEL_ENDPOINT   | localhost:4317                            | OTLP gRPC endpoint              |
| CODEFORGE_OTEL_SERVICE_NAME | codeforge-core                          | OTEL service name               |
| CODEFORGE_OTEL_SAMPLE_RATE | 1.0                                     | Trace sampling rate (0.0-1.0)   |
| CODEFORGE_A2A_ENABLED     | false                                    | Enable A2A protocol endpoints   |
| CODEFORGE_AGUI_ENABLED    | false                                    | Enable AG-UI event emission     |
| CODEFORGE_MCP_ENABLED     | false                                    | Enable MCP integration          |
| CODEFORGE_MCP_SERVERS_DIR |                                          | MCP server YAML definitions dir |
| CODEFORGE_MCP_SERVER_PORT | 3001                                     | Built-in MCP server port        |
| CODEFORGE_AUTH_ENABLED    | false                                    | Enable JWT authentication       |
| CODEFORGE_AUTH_JWT_SECRET | (auto-generated if empty)                | HMAC-SHA256 JWT signing key     |
| CODEFORGE_AUTH_ACCESS_TOKEN_EXPIRY | 15m                               | Access token lifetime           |
| CODEFORGE_AUTH_REFRESH_TOKEN_EXPIRY | 168h                             | Refresh token lifetime (7d)     |
| CODEFORGE_AUTH_BCRYPT_COST | 12                                      | Bcrypt work factor              |
| CODEFORGE_AUTH_DEFAULT_ADMIN_EMAIL | admin@localhost                  | Seed admin email                |
| CODEFORGE_AUTH_DEFAULT_ADMIN_PASS | changeme123                      | Seed admin password             |

### Backup and Restore

#### Manual Backup

```bash
# Set connection variables (or use .env)
export PGHOST=localhost PGPORT=5432 PGUSER=codeforge PGPASSWORD=codeforge_dev PGDATABASE=codeforge

# Run backup
./scripts/backup-postgres.sh

# Run backup with retention cleanup (removes backups older than 7 days)
./scripts/backup-postgres.sh --cleanup
```

Backups are stored in `./backups/postgres/` (gitignored) as compressed `pg_dump --format=custom` files.

#### Restore from Backup

```bash
# Restore from a specific file
./scripts/restore-postgres.sh ./backups/postgres/codeforge_20260218_120000.sql.gz

# Restore the most recent backup
./scripts/restore-postgres.sh latest
```

The restore script drops and recreates the database. Active connections are terminated automatically.

#### Scheduled Backups (cron)

```bash
# Daily backup at 3 AM UTC with 7-day retention
0 3 * * * cd /path/to/CodeForge && PGHOST=localhost PGUSER=codeforge PGPASSWORD=... ./scripts/backup-postgres.sh --cleanup >> /var/log/codeforge-backup.log 2>&1
```

#### WAL Archiving

The Docker Compose postgres service is configured with `wal_level=replica` and `archive_mode=on` for future Point-in-Time Recovery (PITR) support. WAL files are archived to `/var/lib/postgresql/data/archive/` inside the container.

#### Backup Environment Variables

| Variable | Default | Description |
|---|---|---|
| `BACKUP_DIR` | `./backups/postgres` | Directory for backup files |
| `BACKUP_RETAIN_DAYS` | `7` | Days to retain backups before cleanup |
