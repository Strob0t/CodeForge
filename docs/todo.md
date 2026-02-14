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

- [ ] Build and test devcontainer for the first time
  - Verify all tools install correctly (Go 1.23, Python 3.12, Node.js 22)
  - Verify Docker-in-Docker works
  - Verify pre-commit hooks run

### Go Core

- [ ] Initialize Go module (`go mod init github.com/Strob0t/CodeForge`)
- [ ] Create basic directory structure: `cmd/codeforge/`, `internal/domain/`, `internal/port/`, `internal/adapter/`, `internal/service/`
- [ ] Create `cmd/codeforge/main.go` with minimal HTTP server (health endpoint)
- [ ] Create `cmd/codeforge/providers.go` (blank imports placeholder)

### Python Workers

- [ ] Verify Poetry environment works (`poetry install`, `poetry run pytest`)
- [ ] Create basic directory structure: `workers/codeforge/` with `__init__.py`
- [ ] Create placeholder consumer module

### Frontend

- [ ] Initialize SolidJS project in `frontend/` (`npm create solid`)
- [ ] Configure Tailwind CSS
- [ ] Configure ESLint + Prettier for TypeScript
- [ ] Create minimal app shell with routing placeholder

---

## Phase 1 Backlog: Foundation

> Start these after Phase 0 is complete.

### Go Core Service — Scaffold

- [ ] HTTP Router setup (choose: chi, fiber, or echo)
  - See: [features/01-project-dashboard.md](features/01-project-dashboard.md)
- [ ] WebSocket server setup
- [ ] Health endpoint (`GET /health`)
- [ ] Graceful shutdown handling
- [ ] Basic middleware (logging, CORS, recovery)
- [ ] Provider Registry skeleton (`port/gitprovider/registry.go`)
- [ ] Agent Backend Registry skeleton (`port/agentbackend/registry.go`)

### Python Worker — Scaffold

- [ ] NATS or Redis queue consumer
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

### Message Queue

- [ ] Decision: NATS vs Redis (document as ADR)
- [ ] Docker Compose service for chosen queue
- [ ] Go producer library integration
- [ ] Python consumer library integration

### Database

- [ ] Decision: PostgreSQL setup (document as ADR)
- [ ] Docker Compose service for PostgreSQL
- [ ] Go database client (pgx or sqlx)
- [ ] Migration tool setup (golang-migrate or goose)
- [ ] Initial schema: projects, agents, tasks

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
- [ ] GitHub/GitLab Webhook integration
- [ ] Cost tracking dashboard for LLM usage
- [ ] Multi-tenancy / user management

See feature specs for detailed breakdown:
- [features/02-roadmap-feature-map.md](features/02-roadmap-feature-map.md)

---

## Recently Completed

> Move items here after completion for context. Periodically archive old items.

- [x] (2026-02-14) Documentation structure created (docs/README.md, docs/todo.md, feature specs)
- [x] (2026-02-14) Architecture harmony audit: all docs synchronized
- [x] (2026-02-14) All documentation translated from German to English
- [x] (2026-02-14) Coding agent insights integrated into architecture.md

For full completion history, see [project-status.md](project-status.md).
