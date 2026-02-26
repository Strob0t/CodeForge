<div align="center">

# CodeForge

**Orchestrate AI coding agents across multiple repositories with a single dashboard.**

[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)
[![Go 1.24](https://img.shields.io/badge/Go-1.24-00ADD8.svg?logo=go)](https://go.dev)
[![Python 3.12](https://img.shields.io/badge/Python-3.12-3776AB.svg?logo=python)](https://python.org)
[![TypeScript](https://img.shields.io/badge/TypeScript-5.x-3178C6.svg?logo=typescript)](https://typescriptlang.org)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED.svg?logo=docker)](https://docs.docker.com/compose/)

</div>

---

CodeForge is a self-hosted platform that combines **multi-repo project management**, **visual roadmapping**, **multi-LLM routing**, and **AI agent orchestration** into a single containerized service. Configure agent teams, set safety budgets, and let them work across your codebase — with full audit trails and cost tracking.

## Key Features

- **Multi-Repo Dashboard** — Manage Git, GitHub, GitLab, SVN, and local repositories from one place
- **Roadmap & Feature Map** — Visual planning with OpenSpec, Spec Kit, and PM tool sync (GitHub Issues, GitLab, Plane, Gitea)
- **Multi-LLM Routing** — OpenAI, Claude, Ollama, LM Studio, and more through LiteLLM with scenario-based model selection
- **Agent Orchestration** — Coordinate coding agents with 5 autonomy levels, 8 built-in modes, and DAG-based execution plans
- **Safety Layer** — Budget limits, command policies, branch isolation, test/lint gates, stall detection, and rollback
- **Code-RAG** — Hybrid retrieval (BM25 + semantic search), LLM-guided sub-agent search, and PostgreSQL-backed GraphRAG
- **Real-Time UI** — SolidJS frontend with WebSocket live updates, dark mode, i18n (EN/DE), and WCAG AA accessibility
- **Cost Tracking** — Per-run and per-project cost monitoring with budget alerts
- **Audit Trail** — Event sourcing, trajectory recording, replay, and inspection

## Architecture

```
┌─────────────────────────────┐
│   SolidJS Frontend (:3000)  │
│   Tailwind CSS, WebSocket   │
└──────────┬──────────────────┘
           │ REST / WebSocket
┌──────────▼──────────────────┐
│   Go Core Service (:8080)   │
│   HTTP, Policies, Lifecycle │
└──────────┬──────────────────┘
           │ NATS JetStream
┌──────────▼──────────────────┐
│   Python AI Workers         │
│   LLM Calls, RAG, Agents   │
└─────────────────────────────┘
```

| Layer | Stack | Purpose |
|-------|-------|---------|
| Frontend | TypeScript, SolidJS | Web GUI with real-time updates |
| Core | Go 1.24, chi, pgx | HTTP/WS server, scheduling, policies |
| Workers | Python 3.12, NATS | LLM integration, agent execution |
| Infra | Docker, PostgreSQL 18, NATS, LiteLLM | Containerization, storage, messaging, LLM proxy |

## Getting Started

### Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (WSL2 on Windows)
- [VS Code](https://code.visualstudio.com/) with [Dev Containers](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers) extension
- Git

### Quick Start (Dev Containers)

```bash
# 1. Clone
git clone https://github.com/your-org/CodeForge.git
cd CodeForge

# 2. Environment
cp .env.example .env
cp codeforge.yaml.example codeforge.yaml

# 3. Open in VS Code → "Reopen in Container"
#    (Go, Python, Node.js, Docker-in-Docker auto-installed)

# 4. Start infrastructure
docker compose up -d

# 5. Start services
go run ./cmd/codeforge &                              # API on :8080
cd workers && poetry run python -m codeforge.consumer & # AI workers
cd frontend && npm run dev                             # UI on :3000
```

Open [http://localhost:3000](http://localhost:3000) to access the dashboard.

### Production Deployment

```bash
docker compose -f docker-compose.prod.yml up -d
```

This starts all 6 services (core, worker, frontend, PostgreSQL, NATS, LiteLLM) with health checks, restart policies, and tuned resource limits.

## Testing

```bash
./scripts/test.sh              # All unit tests
./scripts/test.sh go           # Go tests only
./scripts/test.sh python       # Python tests only
./scripts/test.sh frontend     # Lint + type check + build
./scripts/test.sh integration  # Integration tests (requires services)
./scripts/test.sh e2e          # E2E browser tests (requires full stack)
./scripts/test.sh all          # Everything
```

## Documentation

| Document | Description |
|----------|-------------|
| [Architecture](docs/architecture.md) | System design, patterns, and component interactions |
| [Dev Setup](docs/dev-setup.md) | Development environment, ports, scripts, and tooling |
| [Tech Stack](docs/tech-stack.md) | Languages, libraries, and version requirements |
| [Project Status](docs/project-status.md) | Phase tracking and milestones |
| [TODO Tracker](docs/todo.md) | Current priorities and task backlog |

### Feature Specs

| Pillar | Document |
|--------|----------|
| Project Dashboard | [docs/features/01-project-dashboard.md](docs/features/01-project-dashboard.md) |
| Roadmap & Feature Map | [docs/features/02-roadmap-feature-map.md](docs/features/02-roadmap-feature-map.md) |
| Multi-LLM Provider | [docs/features/03-multi-llm-provider.md](docs/features/03-multi-llm-provider.md) |
| Agent Orchestration | [docs/features/04-agent-orchestration.md](docs/features/04-agent-orchestration.md) |

### Architecture Decision Records

| ADR | Decision |
|-----|----------|
| [ADR-001](docs/architecture/adr/001-nats-jetstream-message-queue.md) | NATS JetStream as message queue |
| [ADR-002](docs/architecture/adr/002-postgresql-database.md) | PostgreSQL 18 as primary database |
| [ADR-003](docs/architecture/adr/003-config-hierarchy.md) | Hierarchical configuration system |
| [ADR-004](docs/architecture/adr/004-async-logging.md) | Async logging with buffered channels |
| [ADR-005](docs/architecture/adr/005-docker-native-logging.md) | Docker-native logging (no ELK/Grafana) |
| [ADR-006](docs/architecture/adr/006-agent-execution-approach-c.md) | Go control plane + Python runtime |
| [ADR-007](docs/architecture/adr/007-policy-layer.md) | First-match-wins policy evaluation |

## Project Structure

```
CodeForge/
├── cmd/codeforge/          # Go entry point
├── internal/               # Go core (config, domain, adapters, services)
│   ├── adapter/            #   HTTP handlers, PostgreSQL, NATS, LiteLLM, Git
│   ├── domain/             #   Business entities (project, agent, run, plan, ...)
│   ├── port/               #   Interface definitions (database, cache, events)
│   └── service/            #   Business logic (runtime, orchestrator, RAG, ...)
├── workers/codeforge/      # Python AI workers (consumer, executor, retrieval, graphrag)
├── frontend/src/           # SolidJS frontend (features, components, api, i18n)
├── tests/                  # Integration and load tests
├── scripts/                # Build, test, backup, deployment scripts
├── docs/                   # Documentation, ADRs, feature specs, research
├── litellm/                # LiteLLM proxy configuration
├── docker-compose.yml      # Development infrastructure
└── docker-compose.prod.yml # Production deployment
```

## Services & Ports

| Port | Service | Description |
|------|---------|-------------|
| 3000 | Frontend | SolidJS dev server |
| 4000 | LiteLLM | LLM routing proxy |
| 4222 | NATS | Message queue |
| 5432 | PostgreSQL | Primary database |
| 8080 | Go Core | REST API + WebSocket |
| 8222 | NATS Monitor | Monitoring dashboard |

## Configuration

CodeForge uses a hierarchical config system: **defaults < YAML < env vars < CLI flags**.

```bash
# Copy the example config
cp codeforge.yaml.example codeforge.yaml

# Or use environment variables
export CODEFORGE_PORT=8080
export CODEFORGE_DSN="postgres://..."
export CODEFORGE_NATS_URL="nats://localhost:4222"

# Or use CLI flags
go run ./cmd/codeforge --port 8080 --log-level debug
```

Auth is disabled by default for development. Enable it in `codeforge.yaml`:

```yaml
auth:
  enabled: true
  jwt_secret: "your-secret-here"
```

## Contributing

1. Fork the repository
2. Create a feature branch from `staging`
3. Make your changes (all code, comments, and commits in English)
4. Run `pre-commit run --all-files` to verify linting
5. Run `./scripts/test.sh all` to verify tests
6. Submit a pull request to `staging`

Development happens on `staging`. The `main` branch receives merges only on explicit instruction.

## License

This project is licensed under the [GNU Affero General Public License v3.0](LICENSE).
