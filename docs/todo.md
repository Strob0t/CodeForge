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

## Current Priority: Phase 0 Completion

These tasks must be completed before moving to Phase 1.

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

## Phase 1 Backlog: Foundation

> Start these after Phase 0 is complete.

### Go Core Service — Scaffold

- [ ] HTTP Router setup (chi v5)
  - See: [features/01-project-dashboard.md](features/01-project-dashboard.md)
- [ ] WebSocket server setup (coder/websocket)
- [ ] Health endpoint (`GET /health`)
- [ ] Graceful shutdown handling
- [ ] Basic middleware (logging, CORS, recovery)
- [ ] Provider Registry skeleton (`port/gitprovider/registry.go`)
- [ ] Agent Backend Registry skeleton (`port/agentbackend/registry.go`)

### Python Worker — Scaffold

- [ ] NATS JetStream queue consumer
- [ ] Health check endpoint
- [ ] LiteLLM client integration (against LiteLLM Proxy)
- [ ] Basic agent execution framework
  - See: [features/04-agent-orchestration.md](features/04-agent-orchestration.md)

### Frontend — Scaffold

- [ ] SolidJS app with solid-router
- [ ] API client module (REST + WebSocket)
- [ ] Basic layout (sidebar, main content area)
- [ ] Project Dashboard placeholder page
  - See: [features/01-project-dashboard.md](features/01-project-dashboard.md)

### LiteLLM Proxy

- [ ] Add litellm service to `docker-compose.yml`
  - See: [features/03-multi-llm-provider.md](features/03-multi-llm-provider.md)
- [ ] Create initial `litellm_config.yaml`
- [ ] Health check integration
- [ ] Verify routing with at least one provider (Ollama or OpenAI)

### Message Queue (NATS JetStream)

- [x] (2026-02-14) Decision: NATS JetStream — [ADR-001](architecture/adr/001-nats-jetstream-message-queue.md)
- [ ] Add NATS service to `docker-compose.yml`
- [ ] Go producer integration (`nats.go` + `nats.go/jetstream`)
- [ ] Python consumer integration (`nats-py`)
- [ ] Define subject hierarchy (`tasks.agent.{backend}`, `results.{task_id}`, etc.)

### Database (PostgreSQL)

- [x] (2026-02-14) Decision: PostgreSQL 17 + pgx + goose — [ADR-002](architecture/adr/002-postgresql-database.md)
- [ ] Add PostgreSQL service to `docker-compose.yml`
- [ ] Go database client setup (pgx v5 + pgxpool)
- [ ] Migration tool setup (goose, SQL files in `migrations/`)
- [ ] LiteLLM shared instance configuration (`?schema=litellm`)
- [ ] Initial schema: projects, agents, tasks

### Protocols (Phase 1)

- [ ] MCP server in Go Core (expose CodeForge tools to agents)
- [ ] MCP client registry in Go Core (connect to external MCP servers)
- [ ] LSP client in Go Core (manage LSP server lifecycle per project language)
- [ ] OpenTelemetry SDK setup (Go: `go.opentelemetry.io/otel`, Python: `opentelemetry-sdk`)
- [ ] OTEL collector service in `docker-compose.yml`

### CI/CD

- [ ] GitHub Actions workflow: lint + test (Go, Python, TypeScript)
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
