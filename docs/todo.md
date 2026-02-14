# CodeForge — TODO Tracker

> **LLM Agents: This is your primary task reference.**
> Always read this file before starting work to understand current priorities.

## How to Use This File

- **Before starting work:** Read this file to understand what needs to be done
- **After completing a task:** Mark it `[x]`, add completion date, move to "Recently Completed" if needed
- **When discovering new work:** Add items to the appropriate section with context
- **Format:** `- [ ]` for open, `- [x]` for done, `- [-]` for cancelled/deferred
- **Cross-reference:** Link to feature docs, architecture.md sections, or issues where relevant

---

## Current Priority: Phase 2 MVP Features

> Phase 0 and Phase 1 are complete. See "Recently Completed" below.

### Infrastructure

- [x] (2026-02-14) Build and test devcontainer
  - Go 1.23.12, Python 3.12.12, Node.js 22.22.0, Docker 29.2.1, pre-commit 4.5.1
  - Docker-in-Docker verified
  - Pre-commit hooks pass

### Go Core

- [x] (2026-02-14) Initialize Go module (`go mod init github.com/Strob0t/CodeForge`)
- [x] (2026-02-14) Create directory structure: `cmd/codeforge/`, `internal/domain/`, `internal/port/`, `internal/adapter/`, `internal/service/`, `migrations/`
- [x] (2026-02-14) Create `cmd/codeforge/main.go` — chi v5 HTTP server, health endpoint, graceful shutdown
- [x] (2026-02-14) Create `cmd/codeforge/providers.go` (blank imports placeholder)

### Python Workers

- [x] (2026-02-14) Poetry environment verified (`poetry install`, `poetry run pytest` — 3/3 pass)
- [x] (2026-02-14) Create `workers/codeforge/` with `__init__.py`, `consumer.py`, `health.py`
- [x] (2026-02-14) Create `workers/tests/test_health.py` (3 tests)

### Frontend

- [x] (2026-02-14) Initialize SolidJS + TypeScript project in `frontend/`
- [x] (2026-02-14) Configure Tailwind CSS v4 (via @tailwindcss/vite)
- [x] (2026-02-14) Configure ESLint 9 (flat config) + Prettier + eslint-plugin-solid
- [x] (2026-02-14) Create minimal app shell with @solidjs/router, sidebar layout

---

## Phase 1 Backlog: Foundation (COMPLETED)

### Go Core Service — Scaffold

- [x] (2026-02-14) HTTP Router setup (chi v5)
- [x] (2026-02-14) WebSocket server setup (coder/websocket)
- [x] (2026-02-14) Health endpoint (`GET /health`) with service status
- [x] (2026-02-14) Graceful shutdown handling (run() pattern)
- [x] (2026-02-14) Basic middleware (logging, CORS, recovery)
- [x] (2026-02-14) Provider Registry (`port/gitprovider/registry.go` + tests)
- [x] (2026-02-14) Agent Backend Registry (`port/agentbackend/registry.go` + tests)
- [x] (2026-02-14) Domain entities (project, agent, task)
- [x] (2026-02-14) Database port interface (`port/database/store.go`)
- [x] (2026-02-14) Message queue port interface (`port/messagequeue/queue.go`)
- [x] (2026-02-14) PostgreSQL store adapter (CRUD for projects + tasks)
- [x] (2026-02-14) NATS adapter (JetStream publish/subscribe)
- [x] (2026-02-14) REST API routes + handlers (projects, tasks, providers)
- [x] (2026-02-14) Services layer (ProjectService, TaskService)
- [x] (2026-02-14) HTTP handler tests (5 tests)

### Python Worker — Scaffold

- [x] (2026-02-14) NATS JetStream queue consumer (real implementation)
- [x] (2026-02-14) Health check endpoint (NATS + LiteLLM status)
- [x] (2026-02-14) LiteLLM client integration (httpx async)
- [x] (2026-02-14) Agent executor stub
- [x] (2026-02-14) Pydantic models (TaskMessage, TaskResult, TaskStatus)
- [x] (2026-02-14) Tests: models (5), llm (5), consumer (3) — 16 total

### Frontend — Scaffold

- [x] (2026-02-14) SolidJS app with @solidjs/router (routes for / and /projects)
- [x] (2026-02-14) API client module (typed fetch wrapper)
- [x] (2026-02-14) WebSocket client (@solid-primitives/websocket, auto-reconnect)
- [x] (2026-02-14) Sidebar with health indicators (WS + API)
- [x] (2026-02-14) Project Dashboard page with CRUD

### LiteLLM Proxy

- [x] (2026-02-14) Add litellm service to `docker-compose.yml`
- [x] (2026-02-14) Create `litellm_config.yaml` (Ollama, OpenAI, Anthropic)
- [x] (2026-02-14) Health check integration in /health endpoint

### Message Queue (NATS JetStream)

- [x] (2026-02-14) Decision: NATS JetStream — [ADR-001](architecture/adr/001-nats-jetstream-message-queue.md)
- [x] (2026-02-14) Add NATS service to `docker-compose.yml`
- [x] (2026-02-14) Go producer integration (`nats.go` + JetStream)
- [x] (2026-02-14) Python consumer integration (`nats-py`)
- [x] (2026-02-14) Subject hierarchy defined (tasks.agent.*, agents.*)

### Database (PostgreSQL)

- [x] (2026-02-14) Decision: PostgreSQL 17 + pgx + goose — [ADR-002](architecture/adr/002-postgresql-database.md)
- [x] (2026-02-14) PostgreSQL in `docker-compose.yml`
- [x] (2026-02-14) Go database client (pgx v5 + pgxpool)
- [x] (2026-02-14) Migration tool (goose, embedded SQL via go:embed)
- [x] (2026-02-14) Initial schema: projects, agents, tasks (001_initial_schema.sql)

### Protocols (Phase 1 Stubs)

- [x] (2026-02-14) MCP server stub (`internal/adapter/mcp/server.go`)
- [x] (2026-02-14) MCP client stub (`internal/adapter/mcp/client.go`)
- [x] (2026-02-14) LSP client stub (`internal/adapter/lsp/client.go`)
- [x] (2026-02-14) OpenTelemetry stub (`internal/adapter/otel/setup.go`)

### CI/CD

- [x] (2026-02-14) GitHub Actions workflow: lint + test (Go, Python, TypeScript)
- [ ] GitHub Actions workflow: build Docker images
- [ ] Branch protection rules for `main`

---

## Phase 2 Backlog: MVP Features

> High-level items — will be broken down into granular tasks when Phase 1 is complete.

- [ ] Project management (add/remove repos, display status)
- [ ] Git integration (Clone, Pull, Branch, Diff)
- [ ] LLM provider management (API keys, model selection)
- [ ] Simple agent execution (single task to single agent)
- [ ] Basic Web GUI for all features above

See feature specs for detailed breakdown:
- [features/01-project-dashboard.md](features/01-project-dashboard.md)
- [features/03-multi-llm-provider.md](features/03-multi-llm-provider.md)
- [features/04-agent-orchestration.md](features/04-agent-orchestration.md)

---

## Phase 3 Backlog: Advanced Features

> Long-term items — will be broken down when Phase 2 is complete.

- [ ] Roadmap/Feature Map Editor (Auto-Detection, Multi-Format SDD, bidirectional PM sync)
- [ ] OpenSpec/Spec Kit/Autospec integration
- [ ] SVN integration
- [ ] Multi-agent orchestration (pipelines, DAGs)
- [ ] A2A protocol integration (agent discovery, task delegation, Agent Cards)
- [ ] AG-UI protocol integration (agent ↔ frontend streaming, replace custom WS events)
- [ ] GitHub/GitLab Webhook integration
- [ ] Cost tracking dashboard for LLM usage
- [ ] Multi-tenancy / user management

See feature specs for detailed breakdown:
- [features/02-roadmap-feature-map.md](features/02-roadmap-feature-map.md)

---

## Recently Completed

> Move items here after completion for context. Periodically archive old items.

- [x] (2026-02-14) Phase 1 completed: Infrastructure, Go Core, Python Workers, Frontend, CI/CD
  - Docker Compose: PostgreSQL, NATS JetStream, LiteLLM Proxy
  - Go: Hexagonal architecture, REST API, WebSocket, NATS, PostgreSQL
  - Python: NATS consumer, LiteLLM client, Pydantic models, 16 tests
  - Frontend: SolidJS dashboard, API client, WebSocket, health indicators
  - CI: GitHub Actions (Go + Python + Frontend)
- [x] (2026-02-14) Phase 0 completed: devcontainer, Go scaffold, Python Workers, SolidJS frontend
- [x] (2026-02-14) Protocol support decided: MCP, LSP, OpenTelemetry (Tier 1), A2A, AG-UI (Tier 2)
- [x] (2026-02-14) Library decisions: chi (router), coder/websocket (WS), git exec wrapper, SolidJS minimal stack
- [x] (2026-02-14) PostgreSQL 17 chosen as database — [ADR-002](architecture/adr/002-postgresql-database.md)
- [x] (2026-02-14) NATS JetStream chosen as message queue — [ADR-001](architecture/adr/001-nats-jetstream-message-queue.md)
- [x] (2026-02-14) Documentation structure created (docs/README.md, docs/todo.md, feature specs)
- [x] (2026-02-14) Architecture harmony audit: all docs synchronized
- [x] (2026-02-14) All documentation translated from German to English
- [x] (2026-02-14) Coding agent insights integrated into architecture.md

For full completion history, see [project-status.md](project-status.md).
