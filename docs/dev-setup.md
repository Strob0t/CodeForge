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

**Infrastructure services start automatically** via `setup.sh` (`docker compose up -d`). The devcontainer is connected to the `codeforge` Docker network so the Go backend can reach services by container name (`codeforge-postgres`, `codeforge-nats`, `codeforge-litellm`). `devcontainer.json` sets `DATABASE_URL` and `NATS_URL` to these container names (the password in `DATABASE_URL` is hardcoded to `codeforge_dev`, so keep `POSTGRES_PASSWORD` unset or matching). `LITELLM_MASTER_KEY` is taken from the host environment (`${localEnv:LITELLM_MASTER_KEY}`): export it on the host or in the terminal (compose LiteLLM default: `sk-codeforge-dev`).

The devcontainer also sets `LITELLM_BASE_URL=http://codeforge-litellm:4000`.

**PostgreSQL 18 volume layout:** both compose files mount the data volume at `/var/lib/postgresql` (the PG 18 image keeps its cluster in `/var/lib/postgresql/18/docker`; the dev WAL archive is `/var/lib/postgresql/archive`). A volume created before 2026-09-30 holds a PG <= 17 cluster at its root, and the PG 18 entrypoint refuses to start with it ("there appears to be PostgreSQL data in /var/lib/postgresql"). Migrate it: start the old image on the old volume (`postgres:17-alpine`, mount at `/var/lib/postgresql/data`) and `pg_dumpall` it, remove the volume, start the new stack and restore the dump; or upgrade in place with `pg_upgrade --link`.

### Claude Code on the Web (SessionStart Hook)

Cloud sessions of Claude Code on the web run `.claude/hooks/session-start.sh` (registered in `.claude/settings.json`)
before the session starts. The hook only acts when `CLAUDE_CODE_REMOTE=true` and is idempotent:

- Go: `GOTOOLCHAIN=go1.25.14` (the CI toolchain), `go mod download`, golangci-lint v2.11.4 and goimports v0.42.0 in `$(go env GOPATH)/bin`
- Python: `poetry install` on Python 3.12 (like CI, including the pinned ruff)
- Frontend: `npm install --prefix frontend`
- pre-commit: `pre-commit install` and `pre-commit install-hooks`
- Test services (best effort): starts `dockerd` if needed, then PostgreSQL 18 (`codeforge-test-postgres`) and NATS JetStream
  (`codeforge-test-nats`) bound to `127.0.0.1:5432` / `127.0.0.1:4222` (or reuses services already listening there) and exports
  `DATABASE_URL` / `NATS_URL` for the session, so `go test -race ./...` and the `integration`-tagged tests run like in CI

Versions are kept in sync with `.github/workflows/ci.yml` and `.devcontainer/setup.sh`.

### Critical Startup Order (Manual / Outside Devcontainer)

When starting services manually (not via `setup.sh`), follow this **strict order**.
Violating the order causes NATS message drops: tool-call requests time out after 30s
and are denied; the worker only logs the warning "NATS response timeout waiting for
policy decision from Go control plane".

1. **Docker services:** `docker compose up -d postgres nats litellm`
2. **Purge NATS** (fresh test runs only): Kill Go backend + Python worker **first**,
   then purge the JetStream stream. Stale consumers from killed processes block new ones.
3. **Go backend:** `APP_ENV=development go run ./cmd/codeforge/`
   - MUST start **after** NATS purge -- creates fresh JetStream consumers on startup
   - Verify: `curl http://localhost:8080/health` returns `{"status":"ok","dev_mode":true,...}` (`dev_mode` is true only with `APP_ENV=development`)
4. **Python worker:** `source scripts/resolve-docker-ips.sh`, then `cd workers && poetry run python -m codeforge.consumer`
   (the script exports `NATS_URL`, `LITELLM_BASE_URL`, `DATABASE_URL`, ... with container IPs,
   needed on WSL2 where published `localhost` ports are not reachable)
   - MUST start **after** Go backend -- both sides need active consumers
5. **Frontend:** `cd frontend && npm run dev`

**Common pitfall:** A stale Go process (e.g. VSCode debug binary) holds old NATS
consumers that silently fail after a stream purge. Kill ALL Go processes before
purging: `ps aux | grep codeforge | grep -v grep`

The container automatically installs Go 1.25, Python 3.12, Node.js 22, Poetry, golangci-lint v2.11.4 (same version as CI), goimports, Claude Code CLI, Python dependencies (poetry install, including the pinned dev dependency ruff 0.15.1), Node dependencies (npm install), and Pre-commit Hooks.

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
├── data/                     # Runtime data of the Go core (gitignored): workspaces/ (cloned repos), initial_admin_password
├── cmd/
│   └── codeforge/
│       ├── admin.go          # Admin command entrypoints
│       ├── main.go           # Entry point, Dependency Injection
│       └── providers.go      # Blank imports of all active adapters
├── internal/
│   ├── config/               # Hierarchical config system (defaults < YAML < ENV < CLI)
│   ├── crypto/               # AES encryption, key derivation, random tokens/IDs
│   ├── domain/               # Core: Entities, Business Rules (40+ packages)
│   │   ├── a2a/              # A2A protocol types (AgentCard, Task, Message)
│   │   ├── agent/            # Agent + Team + Identity models
│   │   ├── artifact/         # Build artifacts
│   │   ├── autoagent/        # Automatic agent orchestration
│   │   ├── benchmark/        # Benchmark evaluation models
│   │   ├── boundary/         # Boundary detection models
│   │   ├── branchprotection/ # Branch protection rules
│   │   ├── channel/          # Real-time channel + thread models
│   │   ├── command/          # Slash command models
│   │   ├── context/          # Context pack (token budget management)
│   │   ├── conversation/     # Conversation + message models
│   │   ├── cost/             # Cost aggregation models
│   │   ├── dashboard/        # Dashboard models
│   │   ├── errors.go         # Sentinel errors (ErrNotFound, ErrConflict)
│   │   ├── event/            # Event types: agent events (22+ types), broadcast events (55 constants + 49 payloads), AG-UI events
│   │   ├── experience/       # Experience pool caching
│   │   ├── feedback/         # Human feedback provider protocol
│   │   ├── goal/             # Goal discovery models
│   │   ├── knowledgebase/    # Knowledge base models
│   │   ├── llmkey/           # Per-user LLM API key models
│   │   ├── lsp/              # LSP server lifecycle types
│   │   ├── mcp/              # MCP domain types (ServerDef, ServerTool)
│   │   ├── memory/           # Composite memory scoring
│   │   ├── microagent/       # Microagent trigger models
│   │   ├── mode/             # Agent specialization modes
│   │   ├── orchestration/    # Handoff, pipeline, DAG flow
│   │   ├── pipeline/         # Artifact-gated pipelines
│   │   ├── plan/             # Execution plans (DAG scheduling)
│   │   ├── policy/           # Policy profiles, presets, validation
│   │   ├── project/          # Project entity
│   │   ├── prompt/           # Prompt template models
│   │   ├── quarantine/       # Message quarantine + risk scoring
│   │   ├── resource/         # Resource limits (shared across layers)
│   │   ├── review/           # Periodic review models
│   │   ├── roadmap/          # Roadmap, Milestone, Feature
│   │   ├── routing/          # LLM routing + scenario models
│   │   ├── run/              # Run entity, ToolCall, Stall tracker
│   │   ├── settings/         # Project settings models
│   │   ├── skill/            # Agent skills (reusable snippets)
│   │   ├── task/             # Task entity
│   │   ├── tenant/           # Multi-tenancy
│   │   ├── trust/            # Trust annotations (4 levels)
│   │   ├── user/             # User + auth models
│   │   ├── vcsaccount/       # VCS account linking
│   │   └── webhook/          # Webhook models
│   ├── git/                  # Git worker pool (semaphore-bounded)
│   ├── logger/               # Async slog JSON logging
│   ├── middleware/            # HTTP middleware (request ID, tenant, rate limit, idempotency, deprecation)
│   ├── netutil/              # SSRF checks (private IP filter, safe HTTP transport)
│   ├── port/                 # Interfaces + Registries (21 packages)
│   │   ├── agentbackend/     # Agent backend interface + registry
│   │   ├── benchprovider/    # Benchmark provider interface
│   │   ├── broadcast/        # Broadcaster interface (WS events)
│   │   ├── cache/            # Cache interface (Get/Set/Delete)
│   │   ├── codeintel/        # Code intelligence interface (LSP abstraction)
│   │   ├── database/         # Store interface (80+ methods)
│   │   ├── eventstore/       # Event store interface + trajectory types
│   │   ├── feedback/         # Feedback provider interface
│   │   ├── filesystem/       # Filesystem port
│   │   ├── gitprovider/      # Git provider interface + registry
│   │   ├── lsp/              # LSP client port
│   │   ├── messagequeue/     # Message queue interface + schemas
│   │   ├── metrics/          # Metrics recorder interface (OTEL abstraction)
│   │   ├── notifier/         # Notification interface (Slack, Discord, Email)
│   │   ├── llm/              # LLM provider interface
│   │   ├── pmprovider/       # PM provider interface + registry
│   │   ├── shell/            # Shell command port
│   │   ├── specprovider/     # Spec provider interface + registry
│   │   ├── subscription/     # Subscription interface
│   │   ├── tokenexchange/    # Token exchange interface (Copilot abstraction)
│   │   └── wsticket/         # Single-use WebSocket ticket store
│   ├── adapter/              # Concrete Implementations (35 packages)
│   │   ├── a2a/              # A2A protocol server/client
│   │   ├── aider/            # Aider agent backend
│   │   ├── auth/             # Authentication adapter
│   │   ├── autospec/         # Autospec spec provider
│   │   ├── copilot/          # GitHub Copilot token exchange
│   │   ├── discord/          # Discord notification adapter
│   │   ├── email/            # Email notification + feedback adapter
│   │   ├── execshell/        # Shell commander (os/exec)
│   │   ├── gitea/            # Gitea/Forgejo adapter
│   │   ├── github/           # GitHub adapter
│   │   ├── githubpm/         # GitHub Issues PM provider (gh CLI)
│   │   ├── gitlab/           # GitLab adapter
│   │   ├── gitlocal/         # Local git CLI provider
│   │   ├── goose/            # Goose agent backend
│   │   ├── http/             # REST API handlers + routes (80+ endpoints)
│   │   ├── litellm/          # LiteLLM admin API client
│   │   ├── lsp/              # LSP server lifecycle management
│   │   ├── markdownspec/     # Markdown spec provider (ROADMAP.md)
│   │   ├── mcp/              # MCP server + client registry
│   │   ├── nats/             # NATS JetStream adapter
│   │   ├── natskv/           # NATS JetStream KV cache adapter (L2)
│   │   ├── opencode/         # OpenCode agent backend
│   │   ├── openhands/        # OpenHands agent backend
│   │   ├── openspec/         # OpenSpec spec provider (openspec/ dir)
│   │   ├── osfs/             # OS filesystem provider
│   │   ├── otel/             # OpenTelemetry tracing + metrics
│   │   ├── plandex/          # Plandex agent backend
│   │   ├── plane/            # Plane.so PM provider
│   │   ├── postgres/         # PostgreSQL store + 90 migrations
│   │   ├── ristretto/        # Ristretto in-process cache adapter (L1)
│   │   ├── slack/            # Slack notification + feedback adapter
│   │   ├── speckit/          # Spec Kit provider
│   │   ├── svn/              # SVN provider
│   │   ├── tiered/           # Tiered cache (L1 + L2)
│   │   └── ws/               # WebSocket hub + event broadcasting
│   ├── resilience/           # Circuit breaker
│   ├── secrets/              # Secrets vault with SIGHUP reload
│   ├── telemetry/            # OTEL span helpers (API-only, no SDK dependency)
│   ├── tenantctx/            # Tenant ID context helpers
│   ├── version/              # Build version
│   └── service/              # Use Cases (Runtime, Orchestrator, Policy, etc.)
├── workers/                  # Python AI Workers
│   └── codeforge/
│       ├── agent_loop.py     # Multi-turn agentic loop (LLM -> tools -> repeat)
│       ├── consumer/         # NATS queue consumer (modular subject handlers)
│       ├── executor.py       # Agent execution (runtime protocol)
│       ├── graphrag.py       # GraphRAG code graph builder + searcher
│       ├── llm.py            # LiteLLM async client (completions, embeddings)
│       ├── mcp_workbench.py  # MCP workbench (multi-server, BM25 recommender)
│       ├── models.py         # Pydantic data models
│       ├── retrieval.py      # Hybrid retrieval (BM25 + semantic + sub-agent)
│       ├── runtime.py        # Runtime client (Go <-> Python protocol)
│       ├── backends/         # Agent backend executors (Aider, Goose, OpenHands, etc.)
│       ├── evaluation/       # Benchmark evaluation (datasets, runners, metrics)
│       ├── memory/           # Composite memory scoring (semantic + recency)
│       ├── orchestration/    # Multi-agent orchestration helpers
│       ├── routing/          # Hybrid intelligent model routing (MAB, complexity)
│       ├── schemas/          # Pydantic schema models
│       ├── skills/           # Reusable agent skill snippets
│       ├── tools/            # Built-in agent tools (Read, Write, Edit, Bash, etc.)
│       ├── tracing/          # OpenTelemetry tracing
│       └── trust/            # Trust annotation helpers
├── frontend/                 # SolidJS Web GUI
│   ├── e2e/                  # Playwright E2E tests (83 browser/API specs + 11 LLM API specs; the default config also collects e2e/llm)
│   │   └── llm/              # LLM E2E test suite (88 tests, no browser needed)
│   ├── nginx.conf            # Production nginx config (SPA + API proxy)
│   ├── public/
│   │   ├── favicon.svg       # Anvil brand favicon
│   │   └── fonts/            # Self-hosted woff2 (Outfit, Source Sans 3 — 5 files)
│   ├── playwright.config.ts  # Playwright configuration (browser E2E)
│   ├── playwright.llm.config.ts  # Playwright configuration (LLM API E2E)
│   └── src/
│       ├── ui/               # Design system (Phase 16)
│       │   ├── tokens/       # ThemeDefinition, built-in themes (Nord, Solarized)
│       │   ├── primitives/   # Button, Input, Select, Badge, Alert, Spinner, etc.
│       │   ├── composites/   # Card, Modal, Table, Tabs, ConfirmDialog, etc.
│       │   ├── layout/       # Sidebar, NavLink, PageLayout, PageTransition, Section
│       │   ├── icons/        # CodeForgeLogo, EmptyStateIcons (SVG components)
│       │   ├── DESIGN-SYSTEM.md  # Design token documentation
│       │   └── index.ts      # Barrel: import { Button, Card } from "~/ui"
│       ├── features/
│       │   ├── a2a/          # Agent-to-Agent federation UI
│       │   ├── activity/     # Activity feed
│       │   ├── audit/        # Audit trail viewer
│       │   ├── auth/         # Login, auth guards
│       │   ├── benchmarks/   # BenchmarkPage (dev-mode evaluation dashboard)
│       │   ├── canvas/       # Visual design canvas (SVG, 7 tools, triple export)
│       │   ├── channels/     # Real-time channels with threads
│       │   ├── chat/         # Chat enhancements (slash commands, search, notifications)
│       │   ├── dev/          # DesignSystemPage (dev-mode living style guide)
│       │   ├── knowledge/    # Knowledge management
│       │   ├── onboarding/   # OnboardingWizard (3-step first-time user flow)
│       │   ├── costs/        # CostDashboardPage (global cost overview)
│       │   ├── dashboard/    # Project list, ProjectCard
│       │   ├── knowledgebases/ # Knowledge base management
│       │   ├── llm/          # ModelsPage (LLM model management)
│       │   ├── mcp/          # MCPServersPage (MCP server management)
│       │   ├── microagents/  # Microagents management UI
│       │   ├── modes/        # Agent modes management
│       │   ├── notifications/ # Notification center
│       │   ├── project/      # ProjectDetailPage, ChatPanel, WarRoom,
│       │   │                 # AgentPanel, RunPanel, PlanPanel, PolicyPanel,
│       │   │                 # RoadmapPanel, FeatureMapPanel, RepoMapPanel
│       │   ├── prompts/      # Prompt template management
│       │   ├── quarantine/   # Message quarantine admin UI
│       │   ├── routing/      # LLM routing configuration UI
│       │   ├── scopes/       # Scope/permissions management
│       │   ├── search/       # Conversation search UI
│       │   └── settings/     # Application settings
│       └── api/              # API Client, Types, WebSocket
├── scripts/
│   ├── test.sh               # Unified test runner (go/python/frontend/integration/e2e)
│   ├── logs.sh               # Docker log viewer helper
│   ├── resolve-docker-ips.sh       # Export container-IP env vars (WSL2), then start the worker
│   ├── backup-postgres.sh          # PostgreSQL backup script
│   ├── restore-postgres.sh         # PostgreSQL restore script
│   ├── cleanup-wal-archives.sh     # Remove old WAL archives
│   ├── generate-secrets.sh         # Generate production secret files
│   ├── validate-env.sh             # Pre-deploy env var check
│   ├── deploy-blue-green.sh        # Blue-green deployment
│   ├── run-agent-eval.sh           # Agent evaluation scenarios
│   ├── sync-version.sh             # Propagate VERSION to package manifests
│   ├── verify-features.sh          # Feature verification matrix (CI verify job)
│   ├── worker-healthcheck.py       # Worker container healthcheck (sentinel file)
│   └── setup-branch-protection.sh  # GitHub branch protection for main
├── configs/
│   ├── model_pricing.yaml    # Fallback LLM pricing table
│   ├── prometheus/           # Prometheus alert rules
│   └── benchmarks/           # Benchmark datasets (Phase 20): basic-coding, tool-use-basic, agent-coding, e2e-quick
├── tests/
│   └── integration/          # Integration tests (real PostgreSQL, build-tagged)
├── docs/                     # Documentation
├── litellm/
│   └── config.yaml           # LiteLLM Proxy Configuration
├── .env.example              # Environment Template
├── .dockerignore             # Docker build exclusions
├── .golangci.yml             # Go Linter Config (v2)
├── .mcp.json                 # MCP Server for Claude Code (local, gitignored, not in repo)
├── .pre-commit-config.yaml   # Pre-commit Hooks (15 hooks)
├── CLAUDE.md                 # Project Context for Claude Code
├── Dockerfile                # Go Core multi-stage build
├── Dockerfile.worker         # Python Worker image
├── Dockerfile.frontend       # Frontend nginx image
├── codeforge.example.yaml    # Config file template (main sections; full key list in internal/config/config.go)
├── docker-compose.yml        # Dev Services
├── docker-compose.prod.yml   # Production Services (6 containers)
├── LICENSE                   # AGPL-3.0
├── go.mod / go.sum           # Go module files
└── pyproject.toml            # Python: Poetry + Ruff + Pytest
```

PostgreSQL, NATS and docs-mcp data live in the named Docker volumes `codeforge-pgdata`, `codeforge-nats-data`, `codeforge-docs-mcp-data` and `codeforge-docs-mcp-config` (reset with `docker compose down -v`); LiteLLM bind-mounts the repo's `litellm/` directory.

### Ports

All ports published by `docker-compose.yml` bind to `127.0.0.1` (the dev services run without authentication or with
default credentials, KI-14); reach them from another machine through an SSH tunnel. Containers and the devcontainer use the
container names on the `codeforge` network.

| Port | Service              | Purpose                          |
|------|----------------------|----------------------------------|
| 3000 | Frontend Dev Server  | Web GUI                          |
| 4000 | LiteLLM Proxy        | LLM Routing (OpenAI-compatible)  |
| 5432 | PostgreSQL           | Primary Database (App + LiteLLM) |
| 4222 | NATS                 | Message Queue (client connections)|
| 6280 | docs-mcp-server      | MCP Endpoint (SSE/HTTP)          |
| 6281 | docs-mcp-server      | Web Dashboard                    |
| 8001 | playwright-mcp       | Browser Automation (profile `dev`) |
| 8080 | Go API               | Core Service REST/WebSocket      |
| 8222 | NATS Monitoring      | NATS HTTP monitoring dashboard   |
| 3001 | MCP Server           | MCP Streamable HTTP (when enabled)|
| 16686 | Jaeger              | Tracing UI (profile `dev`)       |
| 4317/4318 | Jaeger          | OTLP gRPC/HTTP receiver (profile `dev`) |

`docker compose up -d` (and `setup.sh`) does not start the `dev`-profile services; start them with `docker compose --profile dev up -d`.

### docs-mcp-server (Documentation Grounding)

Provides AI agents with up-to-date library documentation via MCP tools.

**Start:**

```bash
docker compose up -d docs-mcp
```

**Web Dashboard:** http://localhost:6281 (manage indexed libraries)

**MCP Endpoint:** http://localhost:6280/sse (for MCP client configuration)

**Index documentation (via Web UI or CLI):**

```bash
# Example: Index SolidJS docs
docker exec codeforge-docs-mcp npx docs-mcp-server scrape solidjs https://docs.solidjs.com

# Example: Index FastAPI docs
docker exec codeforge-docs-mcp npx docs-mcp-server scrape fastapi https://fastapi.tiangolo.com
```

**Assign to project:**

1. Open "MCP Servers" in the sidebar (`/mcp`) > register docs-mcp-server (type: SSE, URL: http://docs-mcp:6280/sse)
2. Open project > Settings (gear icon) > check "docs-mcp-server"
3. Agent now has `search_docs`, `scrape_docs`, `list_libraries` tools



**Embeddings:** `docker-compose.yml` points docs-mcp at LM Studio's OpenAI-compatible API on the host (`http://host.docker.internal:1234/v1`, model `text-embedding-nomic-embed-text-v1.5`). Load that embedding model in LM Studio, or edit the `OPENAI_API_BASE` / `DOCS_MCP_EMBEDDING_MODEL` values in `docker-compose.yml`. They are hardcoded there, so the `DOCS_MCP_*` entries in `.env` have no effect ([KI-51](todo.md#known-issues)).

**Ports:** 6280 (MCP), 6281 (Web UI)

### Playwright MCP Container

The `codeforge-playwright` container provides browser automation via Model Context Protocol. It is in the compose `dev` profile: start it with `docker compose --profile dev up -d playwright-mcp`.

**Important:** The MCP session is ephemeral -- if the container restarts, all active MCP
sessions become invalid ("Session not found"). You must reconnect from the MCP client
(e.g., restart Claude Code or the MCP client process) after a container restart.

### Design System Page (Dev-Mode Only)

The living design system page is available at `http://localhost:3000/design-system` when the backend runs with `APP_ENV=development`. It renders all design tokens, typography scale, color palette, component variants, and micro-interaction examples. Token documentation is maintained in `frontend/src/ui/DESIGN-SYSTEM.md`.

### Font Files

Self-hosted woff2 font files live in `frontend/public/fonts/` (5 files total). Outfit (display headings) and Source Sans 3 (body text, variable font) are loaded via `@font-face` in global CSS. No external CDN or npm font packages are used.

### Onboarding Wizard

A 3-step onboarding wizard is shown on first login when the user has 0 projects. The steps guide through: Connect Code (repository setup), Configure AI (LLM provider), Create Project. Completion is stored in `localStorage` under the key `codeforge-onboarding-completed`. The wizard does not appear again once completed. Implementation: `frontend/src/features/onboarding/OnboardingWizard.tsx` with 3 step components.

### Running Linting Manually

```bash
# All languages via pre-commit (15 hooks)
pre-commit run --all-files

# Python only (ruff 0.15.1, a pinned Poetry dev dependency matching the pre-commit rev;
# 21 rule groups including security, complexity, performance)
poetry run ruff check .
poetry run ruff format .           # CI runs `poetry run ruff format --check .`

# Go only (golangci-lint v2 with 17 linters including gosec, revive, errorlint; CI pins v2.11.4)
go build ./cmd/codeforge/
golangci-lint run ./...

# TypeScript only (ESLint strict + stylistic + import sorting)
npm run lint --prefix frontend
npm run format:check --prefix frontend
```

#### Linter Rule Summary

Python (ruff): F, E, W, I, N, UP, B, A (builtins), SIM, TCH (type-checking), RUF (ruff-specific), S (bandit security), C4, C90 (complexity 12), PERF, PIE, RET, FURB, LOG, T20, PT

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
# (the E2E suites log in as admin@localhost / Changeme123, so seed that admin)
docker compose up -d
APP_ENV=development CODEFORGE_AUTH_ADMIN_PASS=Changeme123 go run ./cmd/codeforge/ &
cd frontend && npm run dev &

# Run tests
./scripts/test.sh e2e                           # Via test runner
cd frontend && npm run test:e2e                  # Directly
cd frontend && npm run test:e2e:headed           # See browser
cd frontend && npm run test:e2e:report           # View HTML report
```

Tests span 83 spec files (plus the 11 LLM specs in `e2e/llm`, which the default config also picks up) covering health checks, navigation, auth, project CRUD, cost dashboard, models, modes, prompts, MCP, benchmarks, canvas, knowledge bases, settings, scopes, war room, accessibility, security, and more.

#### LLM E2E Tests (API-Level)

LLM E2E tests validate the full LLM integration stack via API calls (no browser needed). They require the backend + infrastructure but not the frontend dev server.

> **WARNING: `APP_ENV=development` is required.** Without it, dev-mode-only endpoints (benchmarks, agent features) return 403 and benchmark-related tests will fail. The `/health` endpoint exposes `dev_mode: true/false` so you can verify the mode.

```bash
# Prerequisites: backend + infrastructure running (tests log in as admin@localhost / Changeme123)
docker compose up -d
APP_ENV=development CODEFORGE_AUTH_ADMIN_PASS=Changeme123 go run ./cmd/codeforge/ &

# Run LLM E2E tests
cd frontend && npx playwright test --config=playwright.llm.config.ts
```

88 tests across 11 spec files covering: prerequisites (6), model management (7), simple conversation (11), agentic conversation (10), streaming AG-UI (10), multi-provider (5), routing (10), cost tracking (12), MCP tools (10), benchmarks (4), cleanup (3). Helper module: `frontend/e2e/llm/llm-helpers.ts`.

#### Integration Tests

Integration tests run against real PostgreSQL (not mocked). They live in `tests/integration/` and use the `//go:build integration` build tag, so they are excluded from normal `go test ./...`.

```bash
# 1. Start required services
docker compose up -d postgres nats

# 2. Run integration tests
go test -race -count=1 -tags=integration ./tests/integration/...
```

The integration tests verify health/liveness and API version, project CRUD lifecycle (create, get, list, delete), input validation (missing fields return 400), task CRUD lifecycle (create, get, list within a project), auth flows (login/logout, token refresh, password reset, API keys) and migration up/down. Smoke tests (`-tags=smoke`, `flows_test.go` / `smoke_test.go`) run against a running stack.

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
cp codeforge.example.yaml codeforge.yaml
```

The YAML file is optional. If missing, defaults are used. Environment variables override YAML, and CLI flags override everything. Unknown YAML keys are silently ignored, so check key names against the `yaml:"..."` tags in `internal/config/config.go`.

#### CLI Flags

The Go Core binary accepts the following command-line flags (highest precedence).

| Flag | Shorthand | Description |
|---|---|---|
| `--config` | `-c` | Path to YAML config file (default: `$CODEFORGE_CONFIG_FILE`, else `codeforge.yaml`; the worker also honours `CODEFORGE_CONFIG_FILE`) |
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
| `server.trusted_proxies` | `CODEFORGE_TRUSTED_PROXIES` | `[]` | Reverse proxies (IPs or CIDR prefixes, comma-separated in the env var) whose `X-Forwarded-For` / `X-Real-IP` headers identify the client for rate limiting, audit and consent records; empty = headers ignored; invalid entries fail startup |
| `postgres.dsn` | `DATABASE_URL` | `postgres://codeforge:...` | PostgreSQL DSN |
| `postgres.max_conns` | `CODEFORGE_PG_MAX_CONNS` | `50` | Max DB connections |
| `postgres.min_conns` | `CODEFORGE_PG_MIN_CONNS` | `10` | Min DB connections |
| `nats.url` | `NATS_URL` | `nats://localhost:4222` | NATS server URL (also `NATS_URL_FILE`) |
| `nats.stream_max_bytes` | `CODEFORGE_NATS_STREAM_MAX_BYTES` | `10737418240` (10 GiB) | Size limit of the `CODEFORGE` JetStream stream. JetStream reserves it against `max_file_store` (default 75% of free disk); lower it on small hosts, or the core exits with "insufficient storage resources" |
| `litellm.url` | `LITELLM_BASE_URL` | `http://localhost:4000` | LiteLLM Proxy URL |
| `litellm.master_key` | `LITELLM_MASTER_KEY` | `` | LiteLLM API key |
| `litellm.conversation_model` | `CODEFORGE_CONVERSATION_MODEL` | (auto-detect) | LLM model for chat conversations (empty = auto-select strongest) |
| `logging.level` | `CODEFORGE_LOG_LEVEL` | `info` | Log level |
| `breaker.max_failures` | `CODEFORGE_BREAKER_MAX_FAILURES` | `5` | Circuit breaker threshold |
| `breaker.timeout` | `CODEFORGE_BREAKER_TIMEOUT` | `30s` | Circuit breaker timeout |
| `rate.requests_per_second` | `CODEFORGE_RATE_RPS` | `10.0` | Rate limit RPS |
| `rate.burst` | `CODEFORGE_RATE_BURST` | `100` | Rate limit burst |
| `orchestrator.max_parallel` | `CODEFORGE_ORCH_MAX_PARALLEL` | `4` | Max parallel plan steps |
| `orchestrator.ping_pong_max_rounds` | `CODEFORGE_ORCH_PINGPONG_MAX_ROUNDS` | `3` | Ping-pong protocol max rounds |
| `orchestrator.consensus_quorum` | `CODEFORGE_ORCH_CONSENSUS_QUORUM` | `0` | Consensus quorum (0=majority) |
| `orchestrator.mode` | `CODEFORGE_ORCH_MODE` | `semi_auto` | Orchestrator mode (manual/semi_auto/full_auto) |
| `orchestrator.decompose_model` | `CODEFORGE_ORCH_DECOMPOSE_MODEL` | `""` | LLM model for feature decomposition (empty = auto-discover from LiteLLM) |
| `orchestrator.decompose_max_tokens` | `CODEFORGE_ORCH_DECOMPOSE_MAX_TOKENS` | `4096` | Max tokens for decomposition response |
| `orchestrator.max_team_size` | `CODEFORGE_ORCH_MAX_TEAM_SIZE` | `5` | Max agents per team |
| `orchestrator.subagent_model` | `CODEFORGE_ORCH_SUBAGENT_MODEL` | `""` | LLM model for sub-agent query expansion/rerank (empty = auto-discover from LiteLLM) |
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
| `auth.enabled` | `CODEFORGE_AUTH_ENABLED` | `true` | Enable JWT authentication |
| `auth.jwt_secret` | `CODEFORGE_AUTH_JWT_SECRET` | `` (random per start) | HMAC-SHA256 signing key; empty = auto-generated in memory at every start (sessions lost on restart). Must be >= 32 chars; well-known values are rejected unless `APP_ENV=development` |
| `auth.llm_key_encryption_secret` | `CODEFORGE_AUTH_LLM_KEY_ENCRYPTION_SECRET` | `` (falls back to the JWT secret) | Key material for encrypting stored LLM provider keys; set it to decouple them from the JWT secret (rotating it makes stored LLM keys unreadable). Every secret setting also accepts `<KEY>_FILE` |
| `auth.access_token_expiry` | `CODEFORGE_AUTH_ACCESS_EXPIRY` | `15m` | Access token lifetime |
| `auth.refresh_token_expiry` | `CODEFORGE_AUTH_REFRESH_EXPIRY` | `168h` | Refresh token lifetime (7d) |
| `auth.bcrypt_cost` | `CODEFORGE_AUTH_BCRYPT_COST` | `12` | Bcrypt work factor |
| `auth.default_admin_email` | `CODEFORGE_AUTH_ADMIN_EMAIL` | `admin@localhost` | Seed admin email |
| `auth.default_admin_pass` | `CODEFORGE_AUTH_ADMIN_PASS` | `` | Seed admin password |
| `auth.auto_generate_initial_password` | `CODEFORGE_AUTH_AUTO_GENERATE_PASSWORD` | `false` | Auto-generate admin password to `initial_password_file` |
| `auth.initial_password_file` | `CODEFORGE_AUTH_INITIAL_PASSWORD_FILE` | `data/initial_admin_password` | File path for generated password |
| `auth.setup_timeout_minutes` | `CODEFORGE_AUTH_SETUP_TIMEOUT_MINUTES` | `5` | Setup wizard timeout |
| `benchmark.datasets_dir` | (YAML only; `CODEFORGE_BENCHMARK_DATASETS_DIR` is read by the Python worker only) | `configs/benchmarks` | Directory with benchmark dataset YAML files |
| `benchmark.watchdog_timeout` | `CODEFORGE_BENCHMARK_WATCHDOG_TIMEOUT` | `2h` | Watchdog timeout for stuck benchmark runs |
| `github.client_id` | `GITHUB_CLIENT_ID` | `` | OAuth client ID for the GitHub device-flow subscription provider |
| `github.client_secret` | `GITHUB_CLIENT_SECRET` | `` | Read but currently unused: the GitHub OAuth web flow (`/api/v1/auth/github`) is not wired and returns 501 ([KI-55](todo.md#known-issues)) |
| `github.callback_url` | `GITHUB_CALLBACK_URL` | `` | Read but currently unused (see `github.client_secret`) |
| `postgres.max_conn_lifetime` | `CODEFORGE_PG_MAX_CONN_LIFETIME` | `30m` | Max connection lifetime |
| `postgres.max_conn_idle_time` | `CODEFORGE_PG_MAX_CONN_IDLE_TIME` | `5m` | Max connection idle time |
| `postgres.health_check` | `CODEFORGE_PG_HEALTH_CHECK` | `30s` | Health check interval |
| `logging.service` | `CODEFORGE_LOG_SERVICE` | `codeforge-core` | Service name in structured logs |
| `logging.async` | `CODEFORGE_LOG_ASYNC` | `true` | Enable async log buffering |
| `rate.auth_per_second` | `CODEFORGE_RATE_AUTH_RPS` | `0.167` | Auth endpoint rate limit (req/s) |
| `rate.auth_burst` | `CODEFORGE_RATE_AUTH_BURST` | `5` | Auth endpoint burst capacity |
| `policy.default_profile` | `CODEFORGE_POLICY_DEFAULT` | `headless-safe-sandbox` | Default policy preset |
| `policy.custom_dir` | `CODEFORGE_POLICY_DIR` | `data/policies` | Custom policy profiles (loaded at start; API-created profiles and Allow-Always clones are written back to their source file). `""` keeps them in memory only and disables Allow-Always (409) |
| `workspace.root` | `CODEFORGE_WORKSPACE_ROOT` | `data/workspaces` | Workspace root directory |
| `workspace.pipeline_dir` | `CODEFORGE_WORKSPACE_PIPELINE_DIR` | `` | Pipeline config directory |
| `runtime.stall_threshold` | `CODEFORGE_STALL_THRESHOLD` | `5` | Stall detection threshold (repeated actions) |
| `runtime.stall_max_retries` | `CODEFORGE_STALL_MAX_RETRIES` | `2` | Max stall recovery retries |
| `runtime.quality_gate_timeout` | `CODEFORGE_QG_TIMEOUT` | `60s` | Quality gate timeout (currently not applied; the worker uses a fixed 120 s per command, [KI-28](todo.md#known-issues)) |
| `runtime.default_deliver_mode` | `CODEFORGE_DELIVER_MODE` | `` | Default delivery mode |
| `runtime.default_test_command` | `CODEFORGE_TEST_COMMAND` | `go test ./...` | Default test command |
| `runtime.default_lint_command` | `CODEFORGE_LINT_COMMAND` | `golangci-lint run ./...` | Default lint command |
| `runtime.delivery_commit_prefix` | `CODEFORGE_COMMIT_PREFIX` | `codeforge:` | Git commit prefix |
| `runtime.heartbeat_interval` | `CODEFORGE_HEARTBEAT_INTERVAL` | `30s` | Agent heartbeat interval |
| `runtime.heartbeat_timeout` | `CODEFORGE_HEARTBEAT_TIMEOUT` | `120s` | Heartbeat timeout |
| `runtime.approval_timeout_seconds` | `CODEFORGE_APPROVAL_TIMEOUT_SECONDS` | `60` | HITL approval timeout (seconds) |
| `idempotency.bucket` | `CODEFORGE_IDEMPOTENCY_BUCKET` | `IDEMPOTENCY` | NATS KV bucket name |
| `idempotency.ttl` | `CODEFORGE_IDEMPOTENCY_TTL` | `24h` | Idempotency key TTL |
| `runtime.hybrid.command_image` | `CODEFORGE_HYBRID_IMAGE` | `` | Docker image for hybrid mode |
| `runtime.hybrid.mount_mode` | `CODEFORGE_HYBRID_MOUNT_MODE` | `rw` | Mount mode (rw/ro) |
| `runtime.sandbox.memory_mb` | `CODEFORGE_SANDBOX_MEMORY_MB` | `512` | Memory limit (MB) |
| `runtime.sandbox.cpu_quota` | `CODEFORGE_SANDBOX_CPU_QUOTA` | `1000` | CPU quota (millicores) |
| `runtime.sandbox.pids_limit` | `CODEFORGE_SANDBOX_PIDS_LIMIT` | `100` | Process limit |
| `runtime.sandbox.storage_gb` | `CODEFORGE_SANDBOX_STORAGE_GB` | `10` | Storage limit (GB) |
| `runtime.sandbox.network_mode` | `CODEFORGE_SANDBOX_NETWORK` | `none` | Network mode |
| `runtime.sandbox.image` | `CODEFORGE_SANDBOX_IMAGE` | `ubuntu:22.04` | Container image |
| `cache.l1_max_size_mb` | `CODEFORGE_CACHE_L1_SIZE_MB` | `100` | L1 in-memory cache size (MB) |
| `cache.l2_bucket` | `CODEFORGE_CACHE_L2_BUCKET` | `CACHE` | NATS KV cache bucket |
| `cache.l2_ttl` | `CODEFORGE_CACHE_L2_TTL` | `10m` | L2 cache TTL |
| `orchestrator.default_context_budget` | `CODEFORGE_ORCH_CONTEXT_BUDGET` | `4096` | Token budget for orchestrator context |
| `orchestrator.prompt_reserve` | `CODEFORGE_ORCH_PROMPT_RESERVE` | `1024` | Prompt token reserve |
| `orchestrator.subagent_enabled` | `CODEFORGE_ORCH_SUBAGENT_ENABLED` | `true` | Enable sub-agent search |
| `orchestrator.subagent_timeout` | `CODEFORGE_ORCH_SUBAGENT_TIMEOUT` | `60s` | Sub-agent request timeout |
| `orchestrator.context_rerank_enabled` | `CODEFORGE_CONTEXT_RERANK_ENABLED` | `false` | Enable LLM context reranking |
| `orchestrator.context_rerank_model` | `CODEFORGE_CONTEXT_RERANK_MODEL` | `` | Model for reranking |
| `webhook.github_secret` | `CODEFORGE_WEBHOOK_GITHUB_SECRET` | `` | GitHub webhook HMAC secret |
| `webhook.gitlab_token` | `CODEFORGE_WEBHOOK_GITLAB_TOKEN` | `` | GitLab webhook token |
| `webhook.plane_secret` | `CODEFORGE_WEBHOOK_PLANE_SECRET` | `` | Plane webhook secret |
| `notification.slack_webhook_url` | `CODEFORGE_NOTIFICATION_SLACK_WEBHOOK_URL` | `` | Slack webhook URL |
| `notification.discord_webhook_url` | `CODEFORGE_NOTIFICATION_DISCORD_WEBHOOK_URL` | `` | Discord webhook URL |
| `notification.smtp_host` | `CODEFORGE_SMTP_HOST` | `` | SMTP server hostname |
| `notification.smtp_port` | `CODEFORGE_SMTP_PORT` | `0` | SMTP server port; must be set (e.g. `587`), port 0 makes sends fail ([KI-51](todo.md#known-issues)) |
| `notification.smtp_from` | `CODEFORGE_SMTP_FROM` | `` | SMTP sender email |
| `notification.smtp_password` | `CODEFORGE_SMTP_PASSWORD` | `` | SMTP password |
| `a2a.base_url` | `CODEFORGE_A2A_BASE_URL` | `http://localhost:<CODEFORGE_PORT>` | Public URL for AgentCard |
| `a2a.api_keys` | `CODEFORGE_A2A_API_KEYS` | `` | Comma-separated API keys |
| `a2a.transport` | `CODEFORGE_A2A_TRANSPORT` | `jsonrpc` | Transport protocol (only `jsonrpc` is implemented; the value is informational) |
| `a2a.max_tasks` | `CODEFORGE_A2A_MAX_TASKS` | `100` | Max concurrent A2A tasks (not enforced yet) |
| `a2a.allow_open` | `CODEFORGE_A2A_ALLOW_OPEN` | `false` | Allow AgentCard discovery without A2A API key |
| `a2a.streaming` | `CODEFORGE_A2A_STREAMING` | `false` | Enable A2A streaming |
| `agent.default_model` | `CODEFORGE_AGENT_DEFAULT_MODEL` | `` | Default agent model (empty = auto-discover) |
| `agent.max_context_tokens` | `CODEFORGE_AGENT_MAX_CONTEXT_TOKENS` | `128000` | Max context window tokens |
| `agent.max_loop_iterations` | `CODEFORGE_AGENT_MAX_LOOP_ITERATIONS` | `50` | Max tool-use loop iterations |
| `agent.agentic_by_default` | `CODEFORGE_AGENT_AGENTIC_BY_DEFAULT` | `true` | Enable agentic mode by default |
| `agent.tool_output_max_chars` | `CODEFORGE_AGENT_TOOL_OUTPUT_MAX_CHARS` | `10000` | Max chars per tool output (currently not read; the worker uses its own default, [KI-38](todo.md#known-issues)) |
| `agent.conversation_rollout_count` | `CODEFORGE_AGENT_CONVERSATION_ROLLOUT_COUNT` | `1` | Conversation rollout count (1-8) |
| `agent.summarize_threshold` | `CODEFORGE_SUMMARIZE_THRESHOLD` | `0` | Message count to trigger summarization (0 = disabled) |
| `litellm.health_poll_interval` | `CODEFORGE_LITELLM_HEALTH_POLL_INTERVAL` | `60s` | LiteLLM health poll interval |
| `copilot.hosts_file_path` | `CODEFORGE_COPILOT_HOSTS_FILE` | `` (falls back to `~/.config/github-copilot/hosts.json`) | Copilot hosts file path |
| `experience.confidence_threshold` | `CODEFORGE_EXPERIENCE_CONFIDENCE_THRESHOLD` | `0.85` | Minimum confidence to use cached experience |
| `experience.max_entries` | `CODEFORGE_EXPERIENCE_MAX_ENTRIES` | `1000` | Max experience pool size |
| `app_env` | `APP_ENV` | `` | Application environment (`development`/`production`) |
| `internal_key` | `CODEFORGE_INTERNAL_KEY` | `` | Shared secret for worker-to-core API auth |
| `env_file` | `CODEFORGE_ENV_FILE` | `` | Path to .env file for OAuth device flow |

#### Python Worker Config (`workers/codeforge/config.py`, routing vars in `llm.py`, backend paths in `backends/*.py`)

| ENV Variable | Default | Description |
|---|---|---|
| `NATS_URL` | `nats://localhost:4222` | NATS server URL |
| `LITELLM_BASE_URL` | `http://localhost:4000` | LiteLLM Proxy URL |
| `LITELLM_MASTER_KEY` | `sk-codeforge-dev` | LiteLLM API key (dev default, matches the compose LiteLLM default; a warning is logged) |
| `CODEFORGE_WORKER_LOG_LEVEL` | `info` | Worker log level (falls back to `logging.level` in codeforge.yaml) |
| `CODEFORGE_WORKER_LOG_SERVICE` | `codeforge-worker` | Worker service name |
| `CODEFORGE_WORKER_HEALTH_PORT` | `8081` | Reserved: no worker HTTP health server is started; liveness uses the sentinel file `/tmp/codeforge-worker-healthy` ([KI-34](todo.md#known-issues)) |
| `CODEFORGE_AIDER_PATH` | `aider` | Path to Aider CLI binary |
| `CODEFORGE_GOOSE_PATH` | `goose` | Path to Goose CLI binary |
| `CODEFORGE_OPENCODE_PATH` | `opencode` | Path to OpenCode CLI binary |
| `CODEFORGE_PLANDEX_PATH` | `plandex` | Path to Plandex CLI binary |
| `CODEFORGE_OPENHANDS_URL` | `http://localhost:3000` | OpenHands service URL |
| `CODEFORGE_CLAUDECODE_ENABLED` | `false` | Enable Claude Code as routing target |
| `CODEFORGE_CLAUDECODE_PATH` | `claude` | Path to Claude Code CLI binary |
| `CODEFORGE_CLAUDECODE_MAX_TURNS` | `50` | Default max agentic turns per Claude Code run |
| `CODEFORGE_CLAUDECODE_TIMEOUT` | `300` | CLI subprocess timeout in seconds |
| `CODEFORGE_CLAUDECODE_TIERS` | `COMPLEX,REASONING` | Complexity tiers that include Claude Code (comma-separated) |
| `CODEFORGE_CLAUDECODE_MAX_CONCURRENT` | `5` | Max parallel Claude Code runs per worker |
| `CODEFORGE_ROUTING_COMPLEXITY_ENABLED` | `true` | Enable complexity analyzer layer |
| `CODEFORGE_ROUTING_MAB_ENABLED` | `true` | Enable MAB model selector |
| `CODEFORGE_ROUTING_LLM_META_ENABLED` | `true` | Enable LLM meta-router fallback |
| `CODEFORGE_ROUTING_MAB_MIN_TRIALS` | `10` | Min trials before MAB active |
| `CODEFORGE_ROUTING_MAB_EXPLORATION_RATE` | `1.414` | UCB1 exploration coefficient |
| `CODEFORGE_ROUTING_COST_WEIGHT` | `0.3` | Cost weight in routing |
| `CODEFORGE_ROUTING_QUALITY_WEIGHT` | `0.5` | Quality weight in routing |
| `CODEFORGE_ROUTING_LATENCY_WEIGHT` | `0.2` | Latency weight in routing |
| `CODEFORGE_ROUTING_META_MODEL` | `` | Model for meta-router |
| `CODEFORGE_ROUTING_STATS_INTERVAL` | `5m` | Stats refresh interval |
| `CODEFORGE_ROUTING_MAB_COST_PENALTY` | `0.0` | MAB cost penalty multiplier |
| `CODEFORGE_ROUTING_COST_PENALTY_MODE` | `linear` | Penalty mode (linear/exponential) |
| `CODEFORGE_ROUTING_MAX_COST_CEILING` | `0.10` | Max cost threshold (USD) |
| `CODEFORGE_ROUTING_MAX_LATENCY_CEILING` | `30000` | Max latency threshold (ms) |
| `CODEFORGE_ROUTING_CASCADE_ENABLED` | `false` | Enable cascade fallback |
| `CODEFORGE_ROUTING_CASCADE_CONFIDENCE` | `0.7` | Cascade confidence threshold |
| `CODEFORGE_ROUTING_CASCADE_MAX_STEPS` | `3` | Max cascade steps |
| `CODEFORGE_ROUTING_DIVERSITY_MODE` | `false` | Enable model diversity |
| `CODEFORGE_ROUTING_ENTROPY_WEIGHT` | `0.1` | Entropy weight in diversity mode |
| `CODEFORGE_EFFECTIVE_MODELS_CACHE_TTL` | `5.0` | Effective models cache TTL (seconds) |
| `CODEFORGE_DEFAULT_MODEL` | `` | Override default LLM model |
| `CODEFORGE_MODEL_BLOCK_TTL` | `300` | Default model block duration (seconds) |
| `CODEFORGE_MODEL_AUTH_BLOCK_TTL` | `86400` | Auth error block duration (24h) |
| `CODEFORGE_CONSUMER_MAX_ERRORS` | `10` | Max consecutive errors before backoff |
| `CODEFORGE_CONSUMER_BACKOFF_MULTIPLIER` | `0.5` | Backoff time multiplier |
| `CODEFORGE_CONSUMER_BACKOFF_MAX` | `5.0` | Max backoff interval (seconds) |
| `CODEFORGE_PLAN_ACT_MAX_ITERATIONS` | `10` | Max plan phase iterations |
| `CODEFORGE_CORE_URL` | `http://localhost:8080` | Go Core service HTTP endpoint |
| `CODEFORGE_TRUST_MIN_LEVEL` | `untrusted` | Minimum trust level |
| `CODEFORGE_WORKSPACE` | `/workspaces/CodeForge` | Default workspace path |
| `CODEFORGE_EARLY_STOP_THRESHOLD` | `0.9` | Early stopping score threshold |
| `CODEFORGE_EARLY_STOP_QUORUM` | `3` | Early stopping quorum size |
| `CODEFORGE_JUDGE_MODEL` | `openai/gpt-4o` | Model for evaluation judge |
| `CODEFORGE_BENCHMARK_MAX_PARALLEL` | `3` | Max parallel benchmarks |
| `CODEFORGE_BENCHMARK_DATASETS_DIR` | `configs/benchmarks` | Benchmark datasets directory |
| `CODEFORGE_SWEAGENT_PATH` | `sweagent` | Path to SWE-Agent CLI binary |
| `CODEFORGE_OPENHANDS_POLL_INTERVAL` | `2.0` | OpenHands poll interval (seconds) |
| `CODEFORGE_OPENHANDS_HTTP_TIMEOUT` | `30.0` | OpenHands HTTP timeout (seconds) |
| `CODEFORGE_OPENHANDS_HEALTH_TIMEOUT` | `5.0` | OpenHands health check timeout |
| `CODEFORGE_OPENHANDS_CANCEL_TIMEOUT` | `5.0` | OpenHands cancel timeout |

### Health Endpoints

| Endpoint | Purpose | Response |
|---|---|---|
| `GET /health` | Liveness probe (Kubernetes) | Always `200 {"status":"ok","dev_mode":<bool>,"dropped_logs":<int>}` |
| `GET /health/ready` | Readiness probe | `200` if all services up, `503` if any down |

The readiness endpoint checks PostgreSQL (ping), NATS (connection status), and LiteLLM (health API), with latency reporting for PostgreSQL and LiteLLM (NATS reports up/down only).

### NATS Subjects

The Go Core and Python Workers communicate via NATS JetStream subjects. The tables below are a subset; the authoritative lists are `internal/port/messagequeue/queue.go` (Go) and `workers/codeforge/nats_subjects.py` (Python). Not listed here: `runs.heartbeat`, `runs.qualitygate.*`, `runs.trajectory.event`, `benchmark.task.*`, `context.shared.updated`, `context.rerank.*`, `repomap.generate.*`, `conversation.run.*`, `conversation.compact.*`, `evaluation.gemmas.*`, `a2a.task.*`, `memory.*`, `handoff.request`, `backends.health.*`, `review.*`, `prompt.evolution.*`.

#### Legacy Task Protocol (fire-and-forget)

| Subject | Direction | Purpose |
|---------|-----------|---------|
| `tasks.agent.<name>` | Go -> Python | Dispatch task to agent backend (name = aider/goose/openhands/opencode/plandex) |
| `tasks.result` | Python -> Go | Task result from worker |
| `tasks.output` | Python -> Go | Streaming output line |
| `tasks.cancel` | Go -> Python | Cancel a running task (does not stop a running backend process yet, [KI-22](todo.md#known-issues)) |
| `agents.output` | Python -> Go | Per-line backend output (re-broadcast over WebSocket) |

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
| `retrieval.index.request` | Go -> Python | Build retrieval index (BM25 + embeddings) |
| `retrieval.index.result` | Python -> Go | Index build result |
| `retrieval.search.request` | Go -> Python | Hybrid search query |
| `retrieval.search.result` | Python -> Go | Search results |
| `retrieval.subagent.request` | Go -> Python | Sub-agent search (LLM query expansion + rerank) |
| `retrieval.subagent.result` | Python -> Go | Sub-agent search results |
| `graph.build.request` | Go -> Python | Build structural code graph |
| `graph.build.result` | Python -> Go | Graph build result |
| `graph.search.request` | Go -> Python | BFS graph traversal from seed symbols |
| `graph.search.result` | Python -> Go | Graph search results |

#### MCP Protocol (Phase 15)

Reserved/planned subjects: the `CODEFORGE` stream already captures `mcp.>`, but no code publishes or consumes these yet.

| Subject | Direction | Purpose |
|---------|-----------|---------|
| `mcp.server.status` | Python -> Go | MCP server connection status update |
| `mcp.tools.discovered` | Python -> Go | Tools discovered on MCP server |

#### Benchmark Protocol (Phase 20, Dev-Only)

| Subject | Direction | Purpose |
|---------|-----------|---------|
| `benchmark.run.request` | Go -> Python | Start benchmark run (dataset, model, metrics) |
| `benchmark.run.result` | Python -> Go | Benchmark run results (scores, costs, duration) |

### Logging

CodeForge uses structured JSON logging across all services with Docker-native log management.

#### Log Access

In development the Go Core and the worker run on the host and log to their own terminals; `docker compose logs` covers only the infrastructure services. In production the Go Core and worker are the `core` and `worker` services of `docker-compose.prod.yml`.

```bash
# Follow all service logs
docker compose logs -f

# Single service (production)
docker compose -f docker-compose.prod.yml logs -f core

# Filter by level (requires jq; --no-log-prefix strips the "<container> |" prefix,
# fromjson? skips non-JSON lines, ascii_upcase also matches the worker's lowercase levels)
docker compose -f docker-compose.prod.yml logs --no-log-prefix core worker \
  | jq -R 'fromjson? | select(.level | ascii_upcase == "ERROR")'

# Filter by request ID across all services
docker compose -f docker-compose.prod.yml logs --no-log-prefix \
  | jq -R 'fromjson? | select(.request_id == "your-request-id")'
```

#### Helper Script

`scripts/logs.sh` only covers the dev compose (`docker-compose.yml`) infrastructure services.

```bash
./scripts/logs.sh tail              # Follow all logs
./scripts/logs.sh errors            # Only ERROR level
./scripts/logs.sh service litellm   # Single service (postgres, nats, litellm, docs-mcp, ...)
./scripts/logs.sh request abc-123   # By request ID across services
```

#### Log Level Configuration

| Service | Config Key | Env Variable | Default |
|---|---|---|---|
| Go Core | `logging.level` | `CODEFORGE_LOG_LEVEL` | `info` |
| Python Workers | `logging.level` (shared via codeforge.yaml) | `CODEFORGE_WORKER_LOG_LEVEL` | `info` |

Valid levels: `debug`, `info`, `warn`, `error`

#### Log Format

All services emit structured JSON to stdout. Go Core example:

```json
{"time":"2026-02-17T14:30:00Z","level":"INFO","service":"codeforge-core","msg":"request handled","request_id":"abc-123","method":"GET","path":"/api/v1/projects"}
```

The Python worker emits `timestamp`, `level` (lowercase), `event`, `logger` and `service: codeforge-worker` instead, and stdlib loggers (e.g. httpx) write plain-text lines ([KI-35](todo.md#known-issues)).

#### Request ID Propagation

Every HTTP request gets a request ID (`X-Request-ID` header): the client-supplied value, or a random 32-character hex ID. This ID propagates through Go Core HTTP handler (logger context), NATS message headers (`X-Request-ID`), Python Worker (structlog context), and back to Go Core via NATS response. Use the request ID to trace a single operation across all services.

#### Log Rotation

Docker handles log rotation automatically via the `json-file` driver. Each service gets max 50 MB per log file with max 10 files (500 MB total per service). This is configured in `docker-compose.yml` and `docker-compose.prod.yml` via the `x-logging` anchor.

### Docker Production Build

CodeForge ships with multi-stage Dockerfiles for all three services.

#### Building Images

```bash
# Go Core (multi-stage: golang:1.25-alpine -> alpine:3.21)
docker build -t codeforge-core .

# Python Worker (python:3.12-slim, poetry, non-root user)
docker build -t codeforge-worker -f Dockerfile.worker .

# Frontend (node:22-alpine build -> nginxinc/nginx-unprivileged:1.27-alpine serve on port 8080)
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

Production layout (since 2026-09-30): PostgreSQL 18 with TLS (self-signed certificate from `generate-secrets.sh`, copied to a tmpfs by an entrypoint wrapper; clients use `sslmode=require`), the core with a read-only root filesystem plus volumes `core_data` (`/data`, holds `data/policies`, `data/initial_admin_password`) and `workspaces` (`/data/workspaces`, shared with the worker at the same path), tmpfs `/tmp` for core and worker, images running as UID/GID 10001, LiteLLM `v1.103.1` on the `internal` and `egress` networks with `host.docker.internal` mapped to the host gateway (local model servers). All credentials come from Docker secret files, see [Secret Management](#secret-management). Remaining gap: the worker healthcheck only tests importability ([KI-34](todo.md#known-issues)); the blue-green overlay does not work ([KI-70](todo.md#known-issues)).

#### CI/CD

CI (`.github/workflows/ci.yml`) runs on pushes to `main`/`staging` and on pull requests to `main` and `staging`: Go build, `go vet -tags=integration`, unit tests with `-race`, `integration`-tagged tests against PostgreSQL + NATS, golangci-lint v2.11.4; Python (`poetry run ruff check .`, `poetry run ruff format --check .` with the Poetry-pinned ruff 0.15.1, `poetry run pytest`); frontend lint, format check, type check (`npm run typecheck`), unit tests (`npm run test`, vitest) and build; Lighthouse CI, contract tests and security scanning; smoke tests and feature verification run only on pushes to `staging`/`main`.

GitHub Actions automatically builds and pushes Docker images to `ghcr.io` on push to `main`/`staging` and on version tags. See `.github/workflows/docker-build.yml`. Each build job records the pushed image by digest and the Grype scan job scans exactly those references.

### Environment Variables

See `.env.example` for the most common values; the full lists are in `internal/config/loader.go` (Go) and `workers/codeforge/config.py` (Python).

| Variable                  | Default                                  | Description                     |
|---------------------------|------------------------------------------|---------------------------------|
| CODEFORGE_PORT            | 8080                                     | Go Core Service port            |
| CODEFORGE_CORS_ORIGIN     | http://localhost:3000                     | Allowed CORS origin             |
| CODEFORGE_TRUSTED_PROXIES | (empty)                                   | Trusted reverse proxies (IPs/CIDRs) for client IP headers |
| DATABASE_URL              | postgres://...@codeforge-postgres:5432/codeforge (devcontainer) | PostgreSQL connection string |
| NATS_URL                  | nats://codeforge-nats:4222 (devcontainer) | NATS server URL                 |
| LITELLM_BASE_URL          | http://localhost:4000                    | LiteLLM Proxy URL (the devcontainer sets `http://codeforge-litellm:4000`) |
| LITELLM_MASTER_KEY        | empty (Go Core); sk-codeforge-dev (worker, dev LiteLLM container) | Master Key for LiteLLM Proxy (the devcontainer forwards the host value) |
| DOCS_MCP_API_BASE         | http://host.docker.internal:1234/v1      | Embedding API Endpoint (currently ignored: docker-compose.yml hardcodes the docs-mcp embedding settings, [KI-51](todo.md#known-issues)) |
| DOCS_MCP_API_KEY          | lmstudio                                 | API Key for Embeddings (currently ignored) |
| DOCS_MCP_EMBEDDING_MODEL  | text-embedding-qwen3-embedding-8b        | Embedding Model Name (currently ignored; compose uses `text-embedding-nomic-embed-text-v1.5`) |
| OPENAI_API_KEY            | (optional)                               | OpenAI API Key (via LiteLLM)    |
| ANTHROPIC_API_KEY         | (optional)                               | Anthropic API Key (via LiteLLM) |
| GEMINI_API_KEY            | (optional)                               | Google Gemini API Key           |
| GROQ_API_KEY              | (optional)                               | Groq API Key (fast inference)   |
| MISTRAL_API_KEY           | (optional)                               | Mistral AI API Key              |
| OPENROUTER_API_KEY        | (optional)                               | OpenRouter API Key              |
| POSTGRES_PASSWORD         | (required)                               | PostgreSQL password              |
| OLLAMA_BASE_URL           | http://host.docker.internal:11434        | Ollama Endpoint (local); used by Go model discovery, while LiteLLM's `ollama/*` entry hardcodes its `api_base` ([KI-51](todo.md#known-issues)) |
| CODEFORGE_OTEL_ENABLED    | false                                    | Enable OpenTelemetry tracing    |
| CODEFORGE_OTEL_ENDPOINT   | localhost:4317                              | OTLP gRPC endpoint              |
| CODEFORGE_OTEL_SERVICE_NAME | codeforge-core                          | OTEL service name               |
| CODEFORGE_OTEL_SAMPLE_RATE | 1.0                                     | Trace sampling rate (0.0-1.0)   |
| CODEFORGE_A2A_ENABLED     | false                                    | Enable A2A protocol endpoints   |
| CODEFORGE_AGUI_ENABLED    | false                                    | Reserved, currently has no effect (AG-UI run events are always emitted) |
| CODEFORGE_MCP_ENABLED     | false                                    | Enable MCP integration          |
| CODEFORGE_MCP_SERVERS_DIR |                                          | MCP server YAML definitions dir |
| CODEFORGE_MCP_SERVER_PORT | 3001                                     | Built-in MCP server port        |
| CODEFORGE_AUTH_ENABLED    | true                                     | Enable JWT authentication       |
| CODEFORGE_AUTH_JWT_SECRET | (empty: random secret per start)         | HMAC-SHA256 JWT signing key; if unset, a random secret is generated at startup and lost on restart. Must be >= 32 chars; well-known values such as `codeforge-dev-jwt-secret-change-in-production` are rejected unless `APP_ENV=development` |
| CODEFORGE_AUTH_ACCESS_EXPIRY | 15m                                   | Access token lifetime           |
| CODEFORGE_AUTH_REFRESH_EXPIRY | 168h                                  | Refresh token lifetime (7d)     |
| CODEFORGE_AUTH_BCRYPT_COST | 12                                      | Bcrypt work factor              |
| CODEFORGE_AUTH_ADMIN_EMAIL | admin@localhost                          | Seed admin email                |
| CODEFORGE_AUTH_ADMIN_PASS |                                          | Seed admin password             |
| CODEFORGE_LLM_MAX_RETRIES  | 2                                        | Max retry attempts per LLM call |
| CODEFORGE_LLM_BACKOFF_BASE | 2.0                                      | Exponential backoff base (sec)  |
| CODEFORGE_LLM_BACKOFF_MAX  | 60.0                                     | Maximum backoff cap (sec)       |
| CODEFORGE_LLM_CONNECT_TIMEOUT | 10.0                                  | HTTP connect timeout (sec)      |
| CODEFORGE_LLM_READ_TIMEOUT | 300.0                                    | HTTP read timeout (sec)         |
| CODEFORGE_AGENT_CONTEXT_ENABLED | true                                 | Enable context optimizer for conversations |
| CODEFORGE_AGENT_CONTEXT_BUDGET | 2048                                  | Token budget for context entries |
| CODEFORGE_AGENT_CONTEXT_PROMPT_RESERVE | 512                           | Tokens reserved for prompt       |
| CODEFORGE_QUARANTINE_ENABLED | false                                   | Enable message quarantine system |
| CODEFORGE_QUARANTINE_THRESHOLD | 0.7                                  | Risk score for quarantine hold   |
| CODEFORGE_QUARANTINE_BLOCK_THRESHOLD | 0.95                           | Risk score for immediate block   |
| CODEFORGE_QUARANTINE_MIN_TRUST_BYPASS | verified                       | Min trust level to bypass quarantine |
| CODEFORGE_QUARANTINE_EXPIRY_HOURS | 72                                | Hours until unreviewed messages expire |
| CODEFORGE_LSP_ENABLED       | false                                    | Enable LSP integration           |
| CODEFORGE_ORCH_REVIEW_ROUTER_ENABLED | false                          | Enable confidence-based review routing |
| CODEFORGE_ORCH_REVIEW_CONFIDENCE_THRESHOLD | 0.7                      | Steps below this get routed to review |
| CODEFORGE_ORCH_REVIEW_ROUTER_MODEL |                                  | LLM model for review evaluation  |
| CODEFORGE_COPILOT_ENABLED   | false                                    | Enable GitHub Copilot token exchange |
| CODEFORGE_ROUTING_ENABLED   | true                                     | Enable hybrid intelligent routing |
| CODEFORGE_EXPERIENCE_ENABLED | false                                   | Enable experience pool caching (currently ignored: the pool is always active, [KI-16](todo.md#known-issues)) |
| CODEFORGE_A2A_BASE_URL     | `http://localhost:<CODEFORGE_PORT>`      | Public URL for AgentCard         |
| CODEFORGE_A2A_API_KEYS     |                                          | Comma-separated API keys         |
| CODEFORGE_A2A_TRANSPORT    | jsonrpc                                  | Transport protocol (only `jsonrpc` is implemented) |
| CODEFORGE_A2A_MAX_TASKS    | 100                                      | Max concurrent A2A tasks (not enforced yet) |
| CODEFORGE_A2A_ALLOW_OPEN   | false                                    | Allow AgentCard discovery without A2A API key |
| CODEFORGE_OTEL_INSECURE    | false (Go Core) / true (worker)          | Use insecure gRPC; the Go exporter currently always uses TLS ([KI-36](todo.md#known-issues)) |
| DEEPSEEK_API_KEY            | (optional)                               | DeepSeek API Key                 |
| COHERE_API_KEY              | (optional)                               | Cohere API Key                   |
| TOGETHERAI_API_KEY          | (optional)                               | Together AI API Key              |
| FIREWORKS_API_KEY           | (optional)                               | Fireworks AI API Key             |
| HF_TOKEN                    | (optional)                               | HuggingFace API token            |
| LM_STUDIO_API_BASE          | (optional)                               | LM Studio API base URL           |
| AIHUBMIX_API_KEY            | (optional)                               | AIHubMix API Key                 |
| CEREBRAS_API_KEY            | (optional)                               | Cerebras API Key                 |
| CHUTES_API_KEY              | (optional)                               | Chutes API Key                   |
| GITHUB_TOKEN                | (optional)                               | GitHub personal access token     |
| CODEFORGE_CONVERSATION_TIMEOUT | 3600                                  | Max wall-clock seconds per conversation run |
| CODEFORGE_WORKER_MEMORY_THRESHOLD_MB | 3500                            | Worker RSS abort threshold (MB)  |
| DOCKER_SECRETS_DIR          | /run/secrets                              | Docker Secrets directory override |
| DEEPEVAL_TELEMETRY_OPT_OUT  | (unset)                                  | Set to `YES` to disable deepeval telemetry |

**Note on deepeval telemetry:** The `deepeval` Python dependency (used by the benchmark
evaluation system) sends anonymous usage telemetry by default. In deployment and CI
environments, set `DEEPEVAL_TELEMETRY_OPT_OUT=YES` to disable this. Add it to your
`.env` file or Docker Compose environment block.

### Secret Management

In development, secrets are loaded from environment variables (`.env` file).

In production, every secret comes from a Docker secret file. The Go Core reads `<KEY>_FILE` for its secret settings
(`CODEFORGE_INTERNAL_KEY`, `DATABASE_URL`, `NATS_URL`, `LITELLM_MASTER_KEY`, `CODEFORGE_AUTH_JWT_SECRET`,
`CODEFORGE_AUTH_LLM_KEY_ENCRYPTION_SECRET`, the admin password, webhook secrets, GitHub client secret, SMTP password,
Plane token, A2A keys; `internal/secrets.LookupFileEnv`). Setting both `KEY` and `KEY_FILE` stops the core at startup;
a missing or empty file is an error; a trailing newline is trimmed; list settings (A2A keys) split on commas and
newlines. PostgreSQL uses `POSTGRES_PASSWORD_FILE`, NATS a generated `nats-auth.conf`, and LiteLLM and the worker export
their values from the files in an entrypoint wrapper. Agent tool subprocesses never inherit these values (scrubbed
environment, `workers/codeforge/subprocess_env.py`). URL credentials are redacted in all logs.

```bash
./scripts/generate-secrets.sh        # writes ./secrets next to docker-compose.prod.yml (or SECRETS_DIR from env/.env)
./scripts/validate-env.sh            # checks the files the compose file mounts
docker compose -f docker-compose.prod.yml up -d
```

`generate-secrets.sh` creates missing secrets as hex values, a PostgreSQL TLS pair and the derived `database-url`,
`nats-url` and `nats-auth.conf` (written only when missing or when their inputs were just generated, so edits such as
`sslmode=verify-full` survive). It reads `POSTGRES_USER`/`POSTGRES_DB` like compose (environment, then `.env`).
Rotation: `codeforge-internal-key`, `nats-user` and `nats-pass` rotate by deleting the file and re-running;
`postgres-password` must be changed in the database first (`ALTER USER`), then in `postgres-password` and
`database-url`; the JWT secret (logs everyone out, VCS tokens become unreadable), the LLM key encryption secret and the
LiteLLM master key are never regenerated for a directory in use (the script stops and explains). A directory counts as in use once any file only the script creates exists (derived files, JWT or LLM key secret, TLS pair); a directory with only operator pre-seeded files (e.g. your own `postgres-password`) is treated as new. For an existing
installation the LLM key encryption secret is created with the JWT secret's value, so stored LLM keys stay readable.
`validate-env.sh` checks presence, readability by the non-root containers, length, known dev defaults, that
`database-url` matches `POSTGRES_USER`/`POSTGRES_DB` and does not disable TLS, and that `nats-url` matches
`nats-auth.conf`.

See `docs/SECURITY.md` for the full secret management policy.

### Distributed Tracing (OpenTelemetry)

CodeForge supports distributed tracing across Go Core, Python Workers, and NATS messaging using OpenTelemetry. Go injects W3C `traceparent` headers into NATS messages and Python extracts them on incoming messages. Injection on the worker's outgoing messages is not implemented yet, so Python -> Go hops start new traces ([KI-36](todo.md#known-issues)).

> **Known issue ([KI-36](todo.md#known-issues)):** the Go exporter ignores `CODEFORGE_OTEL_INSECURE` and always dials TLS, so the Go Core cannot export to a plaintext collector such as the dev Jaeger; only worker spans arrive there until this is fixed.

#### Quick Start

```bash
# 1. Start Jaeger (OTLP collector + UI)
docker compose --profile dev up -d jaeger

# 2. Enable OTEL on Go Core (plaintext collector; see the KI-36 note above)
CODEFORGE_OTEL_ENABLED=true CODEFORGE_OTEL_INSECURE=true go run ./cmd/codeforge/

# 3. Enable OTEL on Python Worker
cd workers && CODEFORGE_OTEL_ENABLED=true poetry run python -m codeforge.consumer

# 4. Open Jaeger UI
open http://localhost:16686
```

Select service `codeforge-core` or `codeforge-worker` in Jaeger to see traces.

#### Configuration

Both Go Core and Python Workers share the same environment variables:

| ENV Variable | Default | Description |
|---|---|---|
| `CODEFORGE_OTEL_ENABLED` | `false` | Master switch for tracing + metrics |
| `CODEFORGE_OTEL_ENDPOINT` | `localhost:4317` | OTLP gRPC endpoint |
| `CODEFORGE_OTEL_SERVICE_NAME` | `codeforge-core` / `codeforge-worker` | Service name in traces |
| `CODEFORGE_OTEL_INSECURE` | `false` (Go Core) / `true` (Python worker) | Use insecure gRPC; set `true` for a local plaintext collector such as Jaeger |
| `CODEFORGE_OTEL_SAMPLE_RATE` | `1.0` | Trace sampling rate (0.0-1.0) |

Or use the YAML config file (`codeforge.yaml`):

```yaml
otel:
  enabled: true
  endpoint: "localhost:4317"
  service_name: "codeforge-core"
  insecure: true
  sample_rate: 1.0
```

#### Jaeger Ports

| Port | Protocol | Purpose |
|---|---|---|
| 16686 | HTTP | Jaeger UI |
| 4317 | gRPC | OTLP trace + metric receiver |
| 4318 | HTTP | OTLP HTTP receiver |

#### What Gets Traced

**Go Core:** HTTP requests (middleware), run lifecycle (start/complete), tool call approval, delivery, conversation messages.

**Python Workers:** Agent execution (`agent_loop`, `executor`), tool calls (`mcp_workbench`), incoming NATS messages (trace context extraction).

**Metrics (Python):** 6 instruments -- `codeforge.agent.loop.iterations`, `codeforge.agent.loop.duration`, `codeforge.llm.call.duration`, `codeforge.llm.tokens.used`, `codeforge.tool.execution.duration`, `codeforge.nats.message.processing.duration`. Not exported yet: the worker configures no MeterProvider, so they are no-ops even when OTEL is enabled ([KI-36](todo.md#known-issues)).

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

The restore script asks for confirmation, then drops and recreates the database.

Stop the core, worker and LiteLLM first (`docker compose -f docker-compose.prod.yml stop core worker litellm`): the script refuses to run while other sessions use the database, checks again after `createdb` (a client reconnecting to the empty database aborts the restore before `pg_restore`), and drops the database with `dropdb --force` (PostgreSQL 13+).

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

### Benchmark System (Phase 20 + 26)

The benchmark evaluation framework measures agent quality with configurable metrics, providers, and evaluator plugins. Requires `APP_ENV=development`.

#### Architecture

```
Dataset YAML  -->  BenchmarkRunner (Go/Python)  -->  Evaluator Pipeline  -->  Results DB
                                                         |
                                                   LLMJudge / FunctionalTest / SPARC
```

Three benchmark types:
- **Simple**: Direct LLM prompt/response scoring (correctness, faithfulness)
- **Tool-Use**: LLM calls with tool invocation validation
- **Agent**: Full workspace lifecycle (clone, edit, test, evaluate)

Ten external providers: HumanEval, MBPP, BigCodeBench, CRUXEval, LiveCodeBench, SWE-bench (full/lite/verified), SPARCBench, Aider Polyglot, DPAI Arena, Terminal-Bench.

#### API Endpoints

All endpoints under `/api/v1/benchmarks` (dev-mode only).

```bash
# --- Run CRUD ---
curl -X POST http://localhost:8080/api/v1/benchmarks/runs \
  -H "Content-Type: application/json" \
  -d '{"dataset": "basic-coding", "model": "openai/gpt-4o", "metrics": ["correctness"]}'

curl http://localhost:8080/api/v1/benchmarks/runs
curl http://localhost:8080/api/v1/benchmarks/runs/{run_id}
curl http://localhost:8080/api/v1/benchmarks/runs/{run_id}/results
curl -X DELETE http://localhost:8080/api/v1/benchmarks/runs/{run_id}

# --- Suite CRUD ---
curl -X POST http://localhost:8080/api/v1/benchmarks/suites \
  -H "Content-Type: application/json" \
  -d '{"name": "Code Quality", "type": "simple", "provider_name": "codeforge_simple"}'

curl http://localhost:8080/api/v1/benchmarks/suites
curl http://localhost:8080/api/v1/benchmarks/suites/{suite_id}
curl -X DELETE http://localhost:8080/api/v1/benchmarks/suites/{suite_id}

# --- Comparison ---
# Two-run comparison
curl -X POST http://localhost:8080/api/v1/benchmarks/compare \
  -H "Content-Type: application/json" \
  -d '{"run_id_a": "...", "run_id_b": "..."}'

# Multi-run comparison (N runs)
curl -X POST http://localhost:8080/api/v1/benchmarks/compare-multi \
  -H "Content-Type: application/json" \
  -d '{"run_ids": ["id1", "id2", "id3"]}'

# --- Analysis ---
curl http://localhost:8080/api/v1/benchmarks/runs/{run_id}/cost-analysis
curl "http://localhost:8080/api/v1/benchmarks/leaderboard?suite_id=optional"
curl "http://localhost:8080/api/v1/benchmarks/runs/{run_id}/export/training?format=json"

# --- Datasets ---
curl http://localhost:8080/api/v1/benchmarks/datasets
```

#### Dashboard

The frontend Benchmarks page (`/benchmarks`) has 5 tabs:
- **Runs** — Create/delete runs, view results, two-run comparison
- **Leaderboard** — Model ranking by avg score, cost efficiency, token efficiency
- **Cost Analysis** — Per-run cost breakdown with task-level detail, training data export
- **Multi-Compare** — Side-by-side comparison of N runs with metric highlighting
- **Suites** — Benchmark suite management (CRUD)

#### Dataset Directory

Benchmark datasets are YAML files in `configs/benchmarks/` (configurable via `benchmark.datasets_dir` in `codeforge.yaml`). See `configs/benchmarks/README.md` for the YAML schema.

Available metrics/evaluators: `llm_judge`, `functional_test`, `sparc`, `trajectory_verifier`, `correctness`, `faithfulness`, `relevance`, `coherence`, `fluency`, `tool_correctness`, `answer_relevancy`, `contextual_precision` (other names are rejected with 400). `trajectory_verifier` currently always scores 0.0 because the worker lacks `litellm` ([KI-37](todo.md#known-issues)).

#### Configuration

| YAML Key | ENV Variable | Default | Description |
|---|---|---|---|
| `benchmark.datasets_dir` | `CODEFORGE_BENCHMARK_DATASETS_DIR` (Python worker only) | `configs/benchmarks` | Directory with benchmark dataset YAML files. The Go Core (dataset listing and path resolution) reads only the YAML key |
| `benchmark.watchdog_timeout` | `CODEFORGE_BENCHMARK_WATCHDOG_TIMEOUT` | `2h` | Watchdog timeout for stuck runs (Go duration: `30m`, `4h`). Agent runs with local models can take 60+ min. |
| — | `HF_TOKEN` | — | HuggingFace API token for gated datasets. Required for CRUXEval (`cruxeval/cruxeval`). Optional for other external suites. Get a token at https://huggingface.co/settings/tokens |

#### Interactive E2E Testing Guide

This section provides a step-by-step walkthrough for manually testing the benchmark system end-to-end, covering infrastructure verification, API-level testing, frontend dashboard usage, and advanced features.

##### Prerequisites

1. **Start infrastructure services:**

```bash
docker compose up -d postgres nats litellm
```

2. **Start the Go backend in dev mode** (required for benchmark endpoints; the password seeds the `admin@localhost` account used below):

```bash
APP_ENV=development CODEFORGE_AUTH_ADMIN_PASS=Changeme123 go run ./cmd/codeforge/
```

3. **Start the frontend dev server** (for dashboard testing):

```bash
cd frontend && npm run dev
```

4. **Verify dev mode is active:**

```bash
curl -s http://localhost:8080/health | jq '.dev_mode'
# Must return: true
```

Without `APP_ENV=development`, all `/api/v1/benchmarks/*` endpoints return 403.

5. **Log in and export the auth token** (all API calls require auth). A seeded admin must change the password first (otherwise every call returns 403 "password change required"); the E2E helpers simply set it to the same value and log in again:

```bash
login() {
  curl -s -X POST http://localhost:8080/api/v1/auth/login \
    -H "Content-Type: application/json" \
    -d '{"email":"admin@localhost","password":"Changeme123"}' | jq -r '.access_token'
}
TOKEN=$(login)

# First login only: clear must_change_password, then log in again
curl -s -X POST http://localhost:8080/api/v1/auth/change-password \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"old_password":"Changeme123","new_password":"Changeme123"}'
TOKEN=$(login)

# Verify token works
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/health | jq
```

##### Step 1: Verify Infrastructure Health

```bash
# Backend liveness (use /health/ready for PostgreSQL/NATS/LiteLLM checks)
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/health | jq
# Expected: {"status":"ok","dev_mode":true,...}

# LiteLLM proxy health
curl -s http://codeforge-litellm:4000/health/liveliness
# Expected: "I'm alive!"

# List available LLM models
curl -s -H "Authorization: Bearer sk-codeforge-dev" \
  http://codeforge-litellm:4000/v1/models | jq '.data[].id'
# Should list your configured models (e.g. lm_studio/*, openai/*, etc.)
```

##### Step 2: List Available Datasets and Suites

```bash
# List built-in benchmark datasets (auto-discovered from configs/benchmarks/)
curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/benchmarks/datasets | jq
```

**Expected datasets** (4 built-in):

| Dataset | Description | Tasks |
|---|---|---|
| `basic-coding` | FizzBuzz, bug fix, refactor, binary search, TS interface | 5 |
| `tool-use-basic` | File read, web search, multi-tool operations | 3 |
| `agent-coding` | FizzBuzz impl, bug fix, add tests, refactor, REST handler | 5 |
| `e2e-quick` | Minimal hello-world + add (fast E2E validation) | 2 |

```bash
# List seeded benchmark suites (13 pre-registered providers)
curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/benchmarks/suites | jq '.[].provider_name'
```

**Expected suites:** `codeforge_simple`, `codeforge_agent`, `codeforge_tool_use`, `humaneval`, `mbpp`, `swebench`, `bigcodebench`, `cruxeval`, `livecodebench`, `sparcbench`, `aider_polyglot`, `dpai_arena`, `terminal_bench`.

##### Step 3: Run a Simple Benchmark (Fastest Path)

Use the `e2e-quick` dataset (2 trivial tasks) for the fastest E2E validation:

```bash
# Create a simple benchmark run
RUN_ID=$(curl -s -X POST http://localhost:8080/api/v1/benchmarks/runs \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "dataset": "e2e-quick",
    "model": "openai/gpt-4o",
    "metrics": ["llm_judge"],
    "benchmark_type": "simple",
    "exec_mode": "mount"
  }' | jq -r '.id')

echo "Run ID: $RUN_ID"
```

Replace the model name with your available model (e.g. `lm_studio/qwen3-30b-a3b` for local models).

**Poll for completion:**

```bash
# Check run status (poll every 5 seconds until completed/failed)
watch -n 5 "curl -s -H 'Authorization: Bearer $TOKEN' \
  http://localhost:8080/api/v1/benchmarks/runs/$RUN_ID | jq '{status,total_cost,error_message}'"
```

**Get results when completed:**

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/benchmarks/runs/$RUN_ID/results | jq
```

Each result contains: `task_id`, `task_name`, `scores` (e.g. `{"llm_judge": 0.85}`), `cost_usd`, `tokens_in`, `tokens_out`, `duration_ms`.

##### Step 4: Run All Three Benchmark Types

Test each benchmark type to verify the full pipeline:

```bash
# A) Simple benchmark (direct prompt/response scoring)
curl -s -X POST http://localhost:8080/api/v1/benchmarks/runs \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"dataset":"basic-coding","model":"openai/gpt-4o","metrics":["llm_judge"],"benchmark_type":"simple","exec_mode":"mount"}'

# B) Tool-use benchmark (validates tool invocation)
curl -s -X POST http://localhost:8080/api/v1/benchmarks/runs \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"dataset":"tool-use-basic","model":"openai/gpt-4o","metrics":["llm_judge"],"benchmark_type":"tool_use","exec_mode":"mount"}'

# C) Agent benchmark (full workspace lifecycle: file creation, editing, test execution)
curl -s -X POST http://localhost:8080/api/v1/benchmarks/runs \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"dataset":"agent-coding","model":"openai/gpt-4o","metrics":["llm_judge","functional_test"],"benchmark_type":"agent","exec_mode":"mount"}'
```

Agent benchmarks take significantly longer (up to 5 min per task with local models).

##### Step 5: Test Evaluator Combinations

The system supports multiple evaluator plugins that can be combined:

```bash
# LLM Judge only (semantic scoring via a second LLM call)
curl -s -X POST http://localhost:8080/api/v1/benchmarks/runs \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"dataset":"e2e-quick","model":"openai/gpt-4o","metrics":["llm_judge"],"benchmark_type":"simple"}'

# Functional Test only (runs test_command from dataset, agent type only)
curl -s -X POST http://localhost:8080/api/v1/benchmarks/runs \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"dataset":"agent-coding","model":"openai/gpt-4o","metrics":["functional_test"],"benchmark_type":"agent"}'

# Combined: LLM Judge + SPARC + Trajectory Verifier (agent type)
curl -s -X POST http://localhost:8080/api/v1/benchmarks/runs \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"dataset":"agent-coding","model":"openai/gpt-4o","metrics":["llm_judge","sparc","trajectory_verifier"],"benchmark_type":"agent"}'
```

Note: `functional_test` on a `simple` benchmark returns score 0 (graceful degradation, no crash).

##### Step 6: Compare Runs and Analyze Costs

After running at least 2 benchmarks, test the comparison and analysis features:

```bash
# List all runs (grab IDs of completed runs)
curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/benchmarks/runs | jq '.[].id'

# Two-run side-by-side comparison
curl -s -X POST http://localhost:8080/api/v1/benchmarks/compare \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"run_id_a":"<RUN_ID_1>","run_id_b":"<RUN_ID_2>"}'

# Multi-run comparison (3+ runs)
curl -s -X POST http://localhost:8080/api/v1/benchmarks/compare-multi \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"run_ids":["<ID1>","<ID2>","<ID3>"]}'

# Cost analysis for a specific run
curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/benchmarks/runs/<RUN_ID>/cost-analysis | jq

# Leaderboard (all models ranked by avg score)
curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/benchmarks/leaderboard | jq

# Run analysis report (failure rate, model family detection)
curl -s -X POST -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/benchmarks/runs/<RUN_ID>/analyze | jq
```

##### Step 7: Export Results

```bash
# Export results as JSON
curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/benchmarks/runs/<RUN_ID>/export/results" | jq

# Export results as CSV
curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/benchmarks/runs/<RUN_ID>/export/results?format=csv"

# Export DPO training pairs (JSONL format, for multi-rollout runs)
curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/benchmarks/runs/<RUN_ID>/export/training"

# Export training pairs as JSON
curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/benchmarks/runs/<RUN_ID>/export/training?format=json" | jq
```

##### Step 8: Test the Frontend Dashboard

1. Open the browser at `http://localhost:3000/benchmarks` (dev-mode only).

2. **Runs tab** — Create a new run:
   - Select a dataset from the dropdown (e.g. `basic-coding`)
   - Select a model (must be available in LiteLLM)
   - Pick metrics (e.g. `correctness`)
   - Select benchmark type (`simple`, `tool_use`, or `agent`)
   - Click "Start Run" and watch the live progress feed

3. **Runs tab** — Inspect results:
   - Click on a completed run to see per-task scores, costs, and duration
   - Use the "Compare" button to compare two runs side-by-side

4. **Leaderboard tab** — View model rankings:
   - Shows avg score, total cost, cost-per-score-point, token efficiency
   - Filter by suite for provider-specific leaderboards

5. **Cost Analysis tab** — Drill into costs:
   - Select a run to see task-level cost breakdown
   - View tokens-in/out per task and total cost

6. **Multi-Compare tab** — Compare N runs:
   - Select 2+ completed runs from the list
   - View side-by-side metric comparison with highlighting

7. **Suites tab** — Manage benchmark suites:
   - View all 13 seeded suites
   - Create/edit/delete custom suites

##### Step 9: Test Error Scenarios

Verify the system handles invalid inputs gracefully:

```bash
# Invalid dataset name -> should fail with clear error
curl -s -X POST http://localhost:8080/api/v1/benchmarks/runs \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"dataset":"nonexistent","model":"openai/gpt-4o","metrics":["llm_judge"],"benchmark_type":"simple"}' | jq

# Invalid model -> run created but transitions to "failed" with error_message
curl -s -X POST http://localhost:8080/api/v1/benchmarks/runs \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"dataset":"e2e-quick","model":"nonexistent/model","metrics":["llm_judge"],"benchmark_type":"simple"}' | jq

# Missing required field (no model) -> 400 Bad Request
curl -s -X POST http://localhost:8080/api/v1/benchmarks/runs \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"dataset":"e2e-quick","metrics":["llm_judge"]}' | jq

# Cancel a running benchmark
curl -s -X PATCH -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/benchmarks/runs/<RUNNING_RUN_ID> | jq
```

##### Step 10: Run Automated E2E Validation Suite

The project includes a comprehensive automated test suite (22 tests across 6 blocks):

```bash
cd frontend

# Run the full benchmark validation suite (requires backend + LiteLLM running)
npx playwright test --config=e2e/benchmark-validation/playwright.validation.config.ts
```

**Test blocks:**
- Block 0: Prerequisites (7 tests) — health checks, datasets, suites
- Block 1: Simple benchmarks (3 tests) — llm_judge, functional_test, combined
- Block 2: Tool-use benchmarks (3 tests) — same evaluator combinations
- Block 3: Agent benchmarks (3 tests) — with trajectory_verifier and sparc
- Block 4: Routing (1 test) — `model=auto` with intelligent routing
- Block 5: Error scenarios (5 tests) — invalid dataset/model, empty dataset, unknown evaluator, duplicates

Run individual blocks:

```bash
npx playwright test --config=e2e/benchmark-validation/playwright.validation.config.ts \
  e2e/benchmark-validation/block-0-prerequisites.spec.ts

npx playwright test --config=e2e/benchmark-validation/playwright.validation.config.ts \
  e2e/benchmark-validation/block-1-simple.spec.ts
```

##### Troubleshooting

| Problem | Cause | Fix |
|---|---|---|
| All benchmark endpoints return 403 | Missing `APP_ENV=development` | Restart backend with `APP_ENV=development go run ./cmd/codeforge/` |
| Run stays "running" forever | Python worker not connected to NATS, or LiteLLM unreachable | Check `docker compose logs nats litellm`, verify NATS_URL and LITELLM_BASE_URL |
| All scores are 0 | Local model context too small for LLM Judge | Expected with small local models; use a model with 32K+ context or a cloud provider |
| "model not found" error | Model name not registered in LiteLLM | Check `litellm/config.yaml`, run `curl http://codeforge-litellm:4000/v1/models` |
| "dataset not found" error | YAML file not in `configs/benchmarks/` | Verify the file exists and is valid YAML |
| Benchmarks page not visible in UI | Frontend not detecting dev mode | Verify `/health` returns `dev_mode: true`, hard-refresh the browser |
| `model=auto` fails | Routing not enabled | Set `CODEFORGE_ROUTING_ENABLED=true` or use an explicit model name |

##### Creating Custom Datasets

1. Create a new YAML file in `configs/benchmarks/`:

```yaml
name: My Custom Tasks
description: Domain-specific code generation tests.

tasks:
  - id: custom-001
    name: Generate REST handler
    input: "Write a Go HTTP handler that returns JSON."
    expected_output: |
      func handler(w http.ResponseWriter, r *http.Request) {
          w.Header().Set("Content-Type", "application/json")
          json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
      }
    difficulty: easy
```

2. The file is auto-discovered (no restart needed).

3. Verify it appears: `curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/benchmarks/datasets | jq '.[].name'`

4. Run a benchmark against it using the file name (without `.yaml` extension) as the dataset name.

See `configs/benchmarks/README.md` for the full YAML schema and available fields.

### A2A Protocol (Phase 27)

The A2A (Agent-to-Agent) protocol enables CodeForge to communicate with external AI agents. When enabled, CodeForge exposes an AgentCard at `/.well-known/agent-card.json` and can delegate tasks to remote A2A agents. With auth enabled (default), the AgentCard and `/a2a` currently also sit behind the global JWT middleware, so they need a CodeForge JWT or API key; A2A API keys alone are not accepted ([KI-15](todo.md#known-issues)).

#### Configuration

| YAML Key | ENV Variable | Default | Description |
|---|---|---|---|
| `a2a.enabled` | `CODEFORGE_A2A_ENABLED` | `false` | Enable A2A endpoints |
| `a2a.base_url` | `CODEFORGE_A2A_BASE_URL` | `http://localhost:<CODEFORGE_PORT>` | Public URL for AgentCard |
| `a2a.api_keys` | `CODEFORGE_A2A_API_KEYS` | (empty) | Comma-separated API keys for inbound auth |
| `a2a.transport` | `CODEFORGE_A2A_TRANSPORT` | `jsonrpc` | Transport protocol (only `jsonrpc` is implemented; the value is informational) |
| `a2a.max_tasks` | `CODEFORGE_A2A_MAX_TASKS` | `100` | Max concurrent A2A tasks (not enforced yet) |
| `a2a.allow_open` | `CODEFORGE_A2A_ALLOW_OPEN` | `false` | Allow AgentCard discovery without A2A API key |

#### Quick Test

```bash
# Enable A2A and start the server
CODEFORGE_A2A_ENABLED=true go run ./cmd/codeforge/

# Fetch the AgentCard ($TOKEN: CodeForge access token, see the benchmark guide above)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/.well-known/agent-card.json

# Register a remote agent (admin or editor role)
curl -X POST http://localhost:8080/api/v1/a2a/agents \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "remote-coder", "url": "https://remote-agent.example.com"}'
```

#### Database

A2A uses 3 PostgreSQL tables (migration `054_a2a_protocol.sql`): `a2a_tasks`, `a2a_remote_agents`, `a2a_push_configs`.

### Intelligent Model Routing (Phase 29)

Three-layer intelligent model routing that replaces manual tag-based LiteLLM routing. When enabled, the Python HybridRouter selects the exact model name and LiteLLM routes directly via provider wildcards.

#### Configuration

| ENV Variable | Default | Description |
|---|---|---|
| `CODEFORGE_ROUTING_ENABLED` | `true` | Master switch for intelligent routing |
| `CODEFORGE_ROUTING_COMPLEXITY_ENABLED` | `true` | Enable Layer 1 (rule-based complexity analysis) |
| `CODEFORGE_ROUTING_MAB_ENABLED` | `true` | Enable Layer 2 (UCB1 multi-armed bandit) |
| `CODEFORGE_ROUTING_LLM_META_ENABLED` | `true` | Enable Layer 3 (LLM-as-router cold-start) |
| `CODEFORGE_ROUTING_MAB_MIN_TRIALS` | `10` | Minimum observations before MAB trusts data |
| `CODEFORGE_ROUTING_MAB_EXPLORATION_RATE` | `1.414` | UCB1 exploration parameter |
| `CODEFORGE_ROUTING_COST_WEIGHT` | `0.3` | Weight for cost in reward function |
| `CODEFORGE_ROUTING_QUALITY_WEIGHT` | `0.5` | Weight for quality in reward function |
| `CODEFORGE_ROUTING_LATENCY_WEIGHT` | `0.2` | Weight for latency in reward function |
| `CODEFORGE_ROUTING_META_MODEL` | `""` | Model for Layer 3 LLM classification (empty = disabled) |

#### LiteLLM Config

The `litellm/config.yaml` uses provider-level wildcards instead of individual model entries:

```yaml
model_list:
  - model_name: "openai/*"       # All OpenAI models
  - model_name: "anthropic/*"    # All Anthropic models
  - model_name: "groq/*"         # All Groq models
  - model_name: "gemini/*"       # All Google Gemini models
  - model_name: "ollama/*"       # Local Ollama models
  - model_name: "mistral/*"      # All Mistral AI models
```

When routing is disabled (`CODEFORGE_ROUTING_ENABLED=false`), the system falls back to scenario-based tag routing.

#### Key Files

| File | Purpose |
|---|---|
| `workers/codeforge/routing/` | Routing package (10 modules) |
| `workers/codeforge/routing/complexity.py` | Layer 1: rule-based prompt analysis |
| `workers/codeforge/routing/mab.py` | Layer 2: UCB1 bandit model selection |
| `workers/codeforge/routing/meta_router.py` | Layer 3: LLM classification fallback |
| `workers/codeforge/routing/router.py` | HybridRouter cascade orchestrator |
| `workers/codeforge/llm.py` | `resolve_model_with_routing()` integration |
| `litellm/config.yaml` | Provider wildcard configuration |

### Goal Discovery (Phase 30)

Auto-detection of project vision, requirements, constraints, and state from workspace files. Goals are injected into agent system prompts and available as ContextPack entries.

#### API Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/projects/{id}/goals` | List goals for a project |
| `POST` | `/api/v1/projects/{id}/goals` | Create a goal |
| `POST` | `/api/v1/projects/{id}/goals/detect` | Trigger auto-detection from workspace |
| `POST` | `/api/v1/projects/{id}/goals/ai-discover` | LLM-assisted goal discovery |
| `GET` | `/api/v1/goals/{id}` | Get a single goal |
| `PUT` | `/api/v1/goals/{id}` | Update a goal |
| `DELETE` | `/api/v1/goals/{id}` | Delete a goal |

#### Database

Goal Discovery uses 1 PostgreSQL table (migration `056_project_goals.sql`): `project_goals`.

#### Key Files

| File | Purpose |
|---|---|
| `internal/domain/goal/goal.go` | Domain model (5 kinds, validation) |
| `internal/service/goal_discovery.go` | Three-tier detection, context rendering, CRUD |
| `internal/adapter/postgres/store_project_goal.go` | PostgreSQL persistence |
| `internal/adapter/http/handlers_goals.go` | REST API handlers |

### Branch Protection (Recommended)

For PRs to `main` (CI also runs for PRs to `staging`), configure these required status checks in GitHub. Checks match the job display names in `.github/workflows/ci.yml`; `scripts/setup-branch-protection.sh` applies `Go`, `Python` and `Frontend`:

- `Go` -- Go build, unit and integration tests, golangci-lint v2.11.4
- `Python` -- Python tests + ruff lint/format check
- `Frontend` -- Frontend lint + format check + build
- `Contract Tests` -- NATS payload contract validation
- `Feature Verification` -- Critical feature verification gate (runs only on pushes to `staging`/`main`; skipped on PRs)
