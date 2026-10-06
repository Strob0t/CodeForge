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

The devcontainer also sets `APP_ENV=development`, `LITELLM_BASE_URL=http://codeforge-litellm:4000`, `LITELLM_MASTER_KEY` (the host's value or `sk-codeforge-dev`) and `POSTGRES_PASSWORD` (the host's value or `codeforge_dev`), from which it builds `DATABASE_URL`; `setup.sh` starts the compose services with these values (compose prefers them over `.env`). `.devcontainer/dev-env.sh`, sourced by `setup.sh` and `~/.bashrc`, exports one `CODEFORGE_INTERNAL_KEY` for the Core and the worker, generated once per container in `~/.config/codeforge/internal_key`; export your own before it to override (KI-124).

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
├── data/                     # Runtime data of the Go core (gitignored): workspaces/ (cloned repos), knowledge/<tenant_id>/ (knowledge-base content, KI-105), initial_admin_password
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
│   ├── git/                  # Hardened git for workspaces (sanitised environment, config allowlist, no nested repositories) + worker pool
│   ├── logger/               # Async slog JSON logging
│   ├── middleware/            # HTTP middleware (request ID, tenant, rate limit, idempotency, deprecation)
│   ├── netutil/              # SSRF checks (private IP filter, safe HTTP transport, OutboundPolicy for MCP URLs)
│   ├── port/                 # Interfaces + Registries (20 packages)
│   │   ├── agentbackend/     # Agent backend interface + registry
│   │   ├── benchprovider/    # Benchmark provider interface
│   │   ├── broadcast/        # Broadcaster interface (WS events)
│   │   ├── codeintel/        # Code intelligence interface (LSP abstraction)
│   │   ├── database/         # Store interface (80+ methods)
│   │   ├── eventstore/       # Event store interface + trajectory types
│   │   ├── feedback/         # Feedback provider interface
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
│   ├── adapter/              # Concrete Implementations (32 packages)
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
│   │   ├── opencode/         # OpenCode agent backend
│   │   ├── openhands/        # OpenHands agent backend
│   │   ├── openspec/         # OpenSpec spec provider (openspec/ dir)
│   │   ├── otel/             # OpenTelemetry tracing + metrics
│   │   ├── plandex/          # Plandex agent backend
│   │   ├── plane/            # Plane.so PM provider
│   │   ├── postgres/         # PostgreSQL store + 113 migrations
│   │   ├── slack/            # Slack notification + feedback adapter
│   │   ├── speckit/          # Spec Kit provider
│   │   ├── svn/              # SVN provider
│   │   └── ws/               # WebSocket hub + event broadcasting
│   ├── proctemp/             # Per-process temporary directory (stale ones removed at startup)
│   ├── resilience/           # Circuit breaker
│   ├── secrets/              # Secrets vault (reloaded on SIGHUP)
│   ├── telemetry/            # OTEL span helpers (API-only, no SDK dependency)
│   ├── tenantctx/            # Tenant ID context helpers
│   ├── version/              # Build version
│   ├── workspacefs/          # Symlink-safe file access below a directory (os.Root): workspaces, knowledge areas, benchmark datasets (KI-95)
│   └── service/              # Use Cases (Runtime, Orchestrator, Policy, etc.)
├── workers/                  # Python AI Workers
│   └── codeforge/
│       ├── agent_loop.py     # Multi-turn agentic loop (LLM -> tools -> repeat)
│       ├── consumer/         # NATS queue consumer (modular subject handlers)
│       ├── executor.py       # Agent execution (runtime protocol)
│       ├── graphrag.py       # GraphRAG code graph builder + searcher
│       ├── llm.py            # LiteLLM async client (completions, embeddings)
│       ├── mcp_outbound.py   # Outbound policy + guarded transport for sse/streamable_http MCP servers
│       ├── mcp_workbench.py  # MCP workbench (multi-server, BM25 recommender)
│       ├── models.py         # Pydantic data models
│       ├── retrieval.py      # Hybrid retrieval (BM25 + semantic + sub-agent)
│       ├── runtime.py        # Runtime client (Go <-> Python protocol)
│       ├── workspace_fs.py   # Symlink-safe workspace file access (O_NOFOLLOW descriptor walks, KI-95)
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
│   ├── live-e2e/                   # Live end-to-end stack (Core, worker, LiteLLM, frontend with a local model; README inside)
│   ├── run-agent-eval.sh           # Agent evaluation scenarios
│   ├── sync-version.sh             # Propagate VERSION to package manifests
│   ├── verify-features.sh          # Feature verification matrix (CI verify job)
│   ├── worker-healthcheck.py       # Worker container healthcheck (GET /health/ready)
│   ├── worker-entrypoint.sh        # Worker image entrypoint: runs the worker as uid 10001 with ambient SETUID/SETGID/KILL (KI-71)
│   ├── check-tool-isolation.sh     # Checks tool isolation of two tenants in the worker image with the production settings (KI-71, KI-96)
│   ├── check-host.sh               # Preflight of a host for per-tenant tool isolation before an upgrade (KI-96)
│   └── setup-branch-protection.sh  # GitHub branch protection for main
├── configs/
│   ├── model_pricing.yaml    # Fallback LLM pricing table
│   ├── nats/                 # nats-server.conf of the production NATS: users core and worker, permissions (KI-71)
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
├── AGENTS.md                 # Instructions for coding agents (AGENTS.md convention)
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

**Outbound policy:** the Core's connection test and the worker refuse private and loopback addresses unless the operator allowlists them (`mcp.allowed_private_hosts` / `CODEFORGE_MCP_ALLOWED_PRIVATE_HOSTS`). docs-mcp needs an entry in dev:

- Core and worker on the host, docs-mcp published on `127.0.0.1:6280`: allowlist `127.0.0.1` and register `http://127.0.0.1:6280/sse` (`localhost` may also resolve to `::1`, which that entry does not open; the entry `localhost` opens both).
- Core and worker in the compose network: register `http://docs-mcp:6280/sse` and allowlist `docs-mcp`, or the container address or the network's CIDR (for example `172.18.0.0/16`).
- A `servers_dir` YAML definition needs no entry in the worker (operator config).

PM syncs: a self-hosted GitLab on a private network or on the host needs an entry in `pm.allowed_private_hosts` (for example `gitlab.corp.internal`, `10.20.0.0/16`, or `127.0.0.1` in dev); the list is separate from `mcp.allowed_private_hosts`.



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

#### Running a Single Test

```bash
go test -race -count=1 ./internal/service/ -run 'TestStartRun_'           # Go: one package, tests matching a regexp
DATABASE_URL=postgres://codeforge:codeforge_dev@localhost:5432/<private_db> \
  go test -race -count=1 ./internal/adapter/postgres/ -run TestStore_X    # store tests skip without DATABASE_URL (they run the migrations)
go test -race -count=1 -tags=integration ./tests/integration/ -run TestX  # files with //go:build integration
poetry run pytest workers/tests/test_text_protocol_parse.py -q -k repair  # Python, from the repo root (testpaths = workers/tests)
cd frontend && npx vitest run src/features/project/WebhooksPanel.test.tsx -t "rotate"   # one vitest file / test name
cd frontend && npx playwright test e2e/activity.spec.ts -g "title"        # one E2E spec (full stack running)
```

Use a private database for store and integration tests that add migrations, and drop it afterwards.

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

#### Tests That Need Root, Docker or a nats-server Binary (KI-71, KI-96)

A few tests skip with a reason unless their prerequisite is present, and CI provides both:

- `workers/tests/test_tool_isolation_integration.py`, `test_tool_exec.py` and `test_landlock.py` have tests that start real `setpriv` tool processes as tenant tool UIDs under Landlock; they need root (CI runs them with `sudo -E`), setpriv, POSIX ACLs on `/tmp` and Landlock.
- `CODEFORGE_ISOLATION_TESTS=required` turns every such skip (no root, setpriv, ACLs, Landlock or Docker) into a failure; CI sets it on the isolation steps.
- These root tests use fixed tenant tool UIDs (`TENANTS` in `workers/tests/tool_isolation_check.py`), and ending a tenant's last work item kills every process of its tool UID (the leftover reaper). Never run two such test runs on one host at the same time: each would kill the other's tool processes.
- `workers/tests/test_nats_permissions.py`, `workers/tests/test_deployment_isolation.py` (`nats-server -t` on the generated config) and `internal/adapter/nats/auth_test.go` start a real `nats-server` with `configs/nats/nats-server.conf`. They need the binary: `NATS_SERVER_BIN=/path/to/nats-server` or `nats-server` on `PATH` (CI copies it from `nats:2.15-alpine`: `docker create --name nats-bin nats:2.15-alpine && docker cp nats-bin:/usr/local/bin/nats-server ./nats-server`). Use nats-server 2.11 or newer; the notification read-back needs batched direct get.
- `./scripts/check-tool-isolation.sh [image]` runs the isolation check of two tenants in the built worker image with the production worker settings (default image `$WORKER_IMAGE`, else the image `docker-compose.prod.yml` runs; needs Docker).
- `workers/tests/test_tenant_isolation_docker.py` (marked `docker`) runs the tenant-isolation suite on the built image with the production service definition: `CODEFORGE_TEST_WORKER_IMAGE` (the worker image), `CODEFORGE_TEST_BATTERY_IMAGE` (an image built from `workers/tests/docker/Dockerfile.battery` with node, go and a JDK), `CODEFORGE_TEST_MIGRATION_ENTRIES` (default 100000; CI uses 1000000) and `CODEFORGE_TEST_EVIDENCE_DIR` (keeps every report as JSON).

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
| `litellm.keyed_providers` | `CODEFORGE_LITELLM_KEYED_PROVIDERS` | (empty) | Core and worker: providers whose API key LiteLLM holds, names only (comma-separated in the env var; `openai`, `anthropic`, `gemini`, `groq`, `mistral`, `openrouter`, `cerebras`, `chutes`, `aihubmix`, `deepseek`, `cohere`, `together_ai`, `fireworks_ai`, `github_copilot`). Their models are listed and can be the default model; other cloud routes show as one entry and are not routed to. A provider whose key variable is set in the process' environment counts too. An unknown name, or a YAML value that is not a list (worker), stops startup. `docker-compose.prod.yml` sets it from the key variables; set it (or export the keys) when the Core and the worker run outside compose. |
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
| `mcp.allowed_private_hosts` | `CODEFORGE_MCP_ALLOWED_PRIVATE_HOSTS` | `` (none) | Host names, IPs and CIDRs (comma-separated in the env) whose private addresses `sse` / `streamable_http` MCP servers may use (Core connection test and worker runs). Loopback opens only by an explicit entry (`localhost`, `127.0.0.1`, `::1`, a loopback CIDR); link-local and cloud metadata addresses never. Invalid entries (`host:port`, URLs, wildcards, bad CIDRs) stop startup. Servers from `mcp.servers_dir` need no entry in the worker. No CLI flag |
| `mcp.use_proxy` | `CODEFORGE_MCP_USE_PROXY` | `false` | Send `sse` / `streamable_http` MCP connections (Core test, worker runs) through the proxy of the environment (`HTTPS_PROXY`, `HTTP_PROXY`, `NO_PROXY`). The URL's host is still checked, but the address is not pinned (DNS rebinding is then the proxy's job); the Core logs a warning at startup |
| `auth.enabled` | `CODEFORGE_AUTH_ENABLED` | `true` | Enable JWT authentication |
| `auth.jwt_secret` | `CODEFORGE_AUTH_JWT_SECRET` | `` (random per start) | HMAC-SHA256 signing key; empty = auto-generated in memory at every start (sessions lost on restart). Must be >= 32 chars; well-known values are rejected unless `APP_ENV=development` |
| `auth.setup_token_file` | `CODEFORGE_AUTH_SETUP_TOKEN_FILE` | `data/setup_token` | One-time token for the first-admin setup page, written (0600) on a start without users and logged once (`SETUP TOKEN`); deleted after the setup. Empty: log only (KI-119) |
| `github.token` | `CODEFORGE_GITHUB_TOKEN` (or `_FILE`) | `` | Operator's GitHub token for the REST API of github.com: the github-issues PM provider of integrations without their own token, and PR delivery for github.com repositories of projects whose github-api provider has no token; serves only the default tenant (KI-117) |
| `auth.llm_key_encryption_secret` | `CODEFORGE_AUTH_LLM_KEY_ENCRYPTION_SECRET` | `` (falls back to the JWT secret) | Key material for encrypting stored LLM provider keys; set it to decouple them from the JWT secret (rotating it makes stored LLM keys unreadable). Every secret setting also accepts `<KEY>_FILE` |
| `auth.access_token_expiry` | `CODEFORGE_AUTH_ACCESS_EXPIRY` | `15m` | Access token lifetime |
| `auth.refresh_token_expiry` | `CODEFORGE_AUTH_REFRESH_EXPIRY` | `168h` | Refresh token lifetime (7d) |
| `auth.bcrypt_cost` | `CODEFORGE_AUTH_BCRYPT_COST` | `12` | Bcrypt work factor |
| `auth.default_admin_email` | `CODEFORGE_AUTH_ADMIN_EMAIL` | `admin@localhost` | Seed admin email |
| `auth.default_admin_pass` | `CODEFORGE_AUTH_ADMIN_PASS` | `` | Seed admin password |
| `auth.auto_generate_initial_password` | `CODEFORGE_AUTH_AUTO_GENERATE_PASSWORD` | `false` | Auto-generate admin password to `initial_password_file` |
| `auth.initial_password_file` | `CODEFORGE_AUTH_INITIAL_PASSWORD_FILE` | `data/initial_admin_password` | File path for generated password |
| `auth.setup_timeout_minutes` | `CODEFORGE_AUTH_SETUP_TIMEOUT_MINUTES` | `5` | Setup wizard timeout |
| `benchmark.datasets_dir` | (YAML only; `CODEFORGE_BENCHMARK_DATASETS_DIR` is read by the Python worker only) | `configs/benchmarks` | Directory with benchmark dataset YAML files. Datasets must be names or paths inside it (KI-107); the worker resolves a relative value against its working directory, the parent, then `CODEFORGE_WORKSPACE` |
| `benchmark.watchdog_timeout` | `CODEFORGE_BENCHMARK_WATCHDOG_TIMEOUT` | `2h` | Watchdog timeout for stuck benchmark runs |
| `knowledge.content_root` | `CODEFORGE_KNOWLEDGE_CONTENT_ROOT` (Core and worker) | `data/knowledge` | Directory knowledge-base content lives in, one area per tenant: `<content_root>/<tenant_id>/`. A knowledge base's `content_path` is relative to its tenant's area (`.` is the whole area; an absolute path is accepted only inside it) and no symlink leads out of the area; every other path is one and the same 400. Core and worker must see the same files: production mounts the read-only `knowledge` volume at `/data/knowledge` in both and the operator fills it (KI-105) |
| `github.client_id` | `GITHUB_CLIENT_ID` | `` | OAuth client ID: alone it enables the GitHub device-flow subscription provider; with `client_secret` and `callback_url` it also enables the web flow |
| `github.client_secret` | `GITHUB_CLIENT_SECRET` (or `GITHUB_CLIENT_SECRET_FILE`) | `` | OAuth client secret of the GitHub OAuth web flow (`POST /api/v1/auth/github` answers 501 until `client_id`, `client_secret` and `callback_url` are all set; `client_secret` or `callback_url` without the other two stops startup) |
| `github.callback_url` | `GITHUB_CALLBACK_URL` | `` | Redirect URI of the web flow, the only one sent to GitHub: `https` (`http` only on localhost or loopback), path `/api/v1/auth/github/callback`, no user info, query or fragment. Register the same URL in the GitHub OAuth app; it must be on the origin the web UI uses for the API |
| `postgres.max_conn_lifetime` | `CODEFORGE_PG_MAX_CONN_LIFETIME` | `30m` | Max connection lifetime |
| `postgres.max_conn_idle_time` | `CODEFORGE_PG_MAX_CONN_IDLE_TIME` | `5m` | Max connection idle time |
| `postgres.health_check` | `CODEFORGE_PG_HEALTH_CHECK` | `30s` | Health check interval |
| `logging.service` | `CODEFORGE_LOG_SERVICE` | `codeforge-core` | Service name in structured logs |
| `logging.async` | `CODEFORGE_LOG_ASYNC` | `true` | Enable async log buffering |
| `rate.auth_per_second` | `CODEFORGE_RATE_AUTH_RPS` | `0.167` | Auth endpoint rate limit (req/s) |
| `rate.auth_burst` | `CODEFORGE_RATE_AUTH_BURST` | `5` | Auth endpoint burst capacity |
| `policy.default_profile` | `CODEFORGE_POLICY_DEFAULT` | `headless-safe-sandbox` | Default policy preset |
| `policy.custom_dir` | `CODEFORGE_POLICY_DIR` | `data/policies` | Custom policy profiles, per tenant in `<custom_dir>/<tenant_id>/<name>.yaml` (loaded at start; API-created profiles and Allow-Always clones are written atomically to the owning tenant's directory). Flat files directly in `custom_dir` (layout before 2026-09-30) are still loaded read-only for the default tenant; saving one writes a copy to the default tenant's directory. `""` keeps profiles in memory only and disables Allow-Always (409) |
| `workspace.root` | `CODEFORGE_WORKSPACE_ROOT` | `data/workspaces` | Workspace root directory |
| `workspace.pipeline_dir` | `CODEFORGE_WORKSPACE_PIPELINE_DIR` | `` | Pipeline config directory |
| `workspace.adopt_roots` | `CODEFORGE_WORKSPACE_ADOPT_ROOTS` | `` | Comma-separated absolute directories (not `/`) whose subdirectories platform admins may adopt as a workspace (`local_path`); a project's stored local `repo_url` may be cloned from them too. Without it everyone adopts only inside their tenant's directory `<workspace.root>/<tenant_id>/`; an adopt root never opens another tenant's area of the workspace root |
| `workspace.tool_acls` | `CODEFORGE_WORKSPACE_TOOL_ACLS` | `off` (`required` in the Core image and `docker-compose.prod.yml`) | Per-tenant tool identities (KI-96, ADR-018): with `required` the Core allocates each tenant's tool UID lazily (`tenants.tool_uid`; 503 when the range 20000-29999 is exhausted), creates tenant directories with POSIX ACLs for it before any clone or init, sends `tool_uid` on every payload that starts tool processes, advances the UID sequence over the bindings on the workspaces volume at startup, and deletes project workspaces through the worker (409 while the project has active work). Linux only; it must match the worker's `CODEFORGE_TOOL_ISOLATION=required`. An unknown value counts as `required` (logged as an error); `off` with `APP_ENV=production` logs a warning |
| `runtime.stall_threshold` | `CODEFORGE_STALL_THRESHOLD` | `5` | Stall detection threshold (repeated actions) |
| `runtime.stall_max_retries` | `CODEFORGE_STALL_MAX_RETRIES` | `2` | New runs a plan step gets after runs that stalled (re-planning); `0` = none, negative is rejected |
| `runtime.quality_gate_timeout` | `CODEFORGE_QG_TIMEOUT` | `60s` | Timeout per gate command (sent to the worker, which kills the command's process group); must be at least 1s |
| `runtime.default_deliver_mode` | `CODEFORGE_DELIVER_MODE` | `` | Default delivery mode |
| `runtime.default_test_command` | `CODEFORGE_TEST_COMMAND` | `` | Last fallback for the gate test command: project config `test_command` first, then the default of the language whose test runner is set up in the workspace |
| `runtime.default_lint_command` | `CODEFORGE_LINT_COMMAND` | `` | Last fallback for the gate lint command (project config `lint_command` first, then the language default) |
| `runtime.delivery_commit_prefix` | `CODEFORGE_COMMIT_PREFIX` | `codeforge:` | Git commit prefix |
| `runtime.heartbeat_interval` | `CODEFORGE_HEARTBEAT_INTERVAL` | `30s` | How often workers report runs, conversation runs and backend tasks alive: sent as `heartbeat_seconds` on `runs.start`, `conversation.run.start` and `tasks.agent.*` (whole seconds); `0` = the default, otherwise at least `1s` (checked at load) |
| `runtime.heartbeat_timeout` | `CODEFORGE_HEARTBEAT_TIMEOUT` | `120s` | Heartbeat timeout; the stuck-work watchdog ends runs, conversation runs and backend tasks without a heartbeat for `heartbeat_timeout + 2 x heartbeat_interval`; `0` disables that check, otherwise it must be longer than the interval (checked at load) |
| `runtime.task_accept_timeout` | `CODEFORGE_TASK_ACCEPT_TIMEOUT` | `1h` | How long a dispatched backend task may wait for a worker to accept it before the watchdog fails it (check "tasks never accepted"); `0` turns the check off, negative is rejected |
| `runtime.approval_timeout_seconds` | `CODEFORGE_APPROVAL_TIMEOUT_SECONDS` | `60` | HITL approval timeout (seconds); also sent to the worker, which waits this long plus 15 s for a tool-call decision. Raise it when approvals come by email |
| `runtime.stale_check_interval` | (YAML only) | `60s` | How often the stuck-work watchdog runs (checks: lost tasks, tasks never accepted, quality gates, lost runs, lost conversation runs, ended teams, undecided review refactorings); must be > 0 |
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
| `orchestrator.default_context_budget` | `CODEFORGE_ORCH_CONTEXT_BUDGET` | `4096` | Token budget for orchestrator context |
| `orchestrator.prompt_reserve` | `CODEFORGE_ORCH_PROMPT_RESERVE` | `1024` | Prompt token reserve |
| `orchestrator.subagent_enabled` | `CODEFORGE_ORCH_SUBAGENT_ENABLED` | `true` | Enable sub-agent search |
| `orchestrator.subagent_timeout` | `CODEFORGE_ORCH_SUBAGENT_TIMEOUT` | `60s` | Sub-agent request timeout |
| `orchestrator.context_rerank_enabled` | `CODEFORGE_CONTEXT_RERANK_ENABLED` | `false` | Enable LLM context reranking |
| `orchestrator.context_rerank_model` | `CODEFORGE_CONTEXT_RERANK_MODEL` | `` | Model for reranking |
| `webhook.delivery_retention` | `CODEFORGE_WEBHOOK_DELIVERY_RETENTION` | `168h` | How long a webhook remembers a delivery (its body hash and delivery ID). Within it, a redelivery, or a replay of a signed delivery under any delivery ID, is handled once; must be positive. Webhooks are registered per project (`POST /api/v1/projects/{id}/webhooks`, KI-85, see [Inbound Webhooks](#inbound-webhooks-ki-85)) |
| `webhook.github_secret`, `webhook.gitlab_token`, `webhook.plane_secret` | `CODEFORGE_WEBHOOK_GITHUB_SECRET` etc. | `` | Removed (KI-85): ignored; the startup log names them |
| `notification.slack_webhook_url` | `CODEFORGE_NOTIFICATION_SLACK_WEBHOOK_URL` | `` | Slack webhook URL; approval requests of `approval_tenants` are posted there, with a link to the approval page, when `web_ui_url` is set too (no buttons, KI-84) |
| `notification.discord_webhook_url` | `CODEFORGE_NOTIFICATION_DISCORD_WEBHOOK_URL` | `` | Discord webhook URL |
| `notification.smtp_host` | `CODEFORGE_SMTP_HOST` | `` | SMTP server hostname |
| `notification.smtp_port` | `CODEFORGE_SMTP_PORT` | `587` | SMTP server port; startup rejects ports outside 1-65535 when `smtp_host` is set |
| `notification.smtp_from` | `CODEFORGE_SMTP_FROM` | `` | SMTP sender email |
| `notification.smtp_password` | `CODEFORGE_SMTP_PASSWORD` | `` | SMTP password |
| `notification.approval_recipients` | `CODEFORGE_NOTIFICATION_APPROVAL_RECIPIENTS` | `` | Comma-separated bare email addresses that get an email for each tool call awaiting approval of the tenants in `approval_tenants` (other tenants' requests are not mailed). Needs `smtp_host`, `smtp_from` and `web_ui_url` as well; the provider is registered only when all four are set and the startup log names what is missing |
| `notification.web_ui_url` | `CODEFORGE_NOTIFICATION_WEB_UI_URL` | `` | Base URL of the web UI (absolute http(s), no user info, query or fragment); approval emails and Slack messages link to `<web_ui_url>/approvals/<run>/<call>`, which asks for a login |
| `notification.approval_tenants` | `CODEFORGE_NOTIFICATION_APPROVAL_TENANTS` | default tenant | Comma-separated tenant IDs (UUIDs) whose approval requests reach the Slack channel and the approval emails; empty: none |
| `retention.interval` | `CODEFORGE_RETENTION_INTERVAL` | `24h` | How often the GDPR retention job runs (also once at startup); `0` disables it ([data-retention.md](data-retention.md)) |
| `retention.sessions` | `CODEFORGE_RETENTION_SESSIONS` | `720h` | Delete agent sessions idle longer than this (`0` keeps them) |
| `retention.conversations` | `CODEFORGE_RETENTION_CONVERSATIONS` | `8760h` | Delete conversations (with messages) idle longer than this |
| `retention.cost_records` | `CODEFORGE_RETENTION_COST_RECORDS` | `8760h` | Delete runs (LLM cost records) idle longer than this |
| `retention.audit_entries` | `CODEFORGE_RETENTION_AUDIT_ENTRIES` | `61320h` | Delete audit log entries older than this (7 years) |
| `retention.audit_ip_addresses` | `CODEFORGE_RETENTION_AUDIT_IP_ADDRESSES` | `4320h` | Remove IP addresses from audit entries older than this (180 days); periods under 24h are rejected |
| `retention.handoff_claims` | `CODEFORGE_RETENTION_HANDOFF_CLAIMS` | `720h` | Delete handoff claims whose stage was done longer ago than this (claims never done are kept); `0` keeps them, otherwise at least `720h`, the NATS stream's message max age (`messagequeue.StreamMaxAge`) ([data-retention.md](data-retention.md)) |
| `a2a.base_url` | `CODEFORGE_A2A_BASE_URL` | `http://localhost:<CODEFORGE_PORT>` | Public URL for AgentCard |
| `a2a.api_keys` | `CODEFORGE_A2A_API_KEYS` | `` | Comma-separated A2A API keys, each `<key>` (default tenant) or `<tenant-uuid>:<key>` (that tenant; UUID in any case). Parsed only when A2A is enabled; a malformed tenant prefix, an empty key or a repeated key stops startup; without keys every `/a2a` request gets 401 |
| `a2a.transport` | `CODEFORGE_A2A_TRANSPORT` | `jsonrpc` | Transport protocol (only `jsonrpc` is implemented; the value is informational) |
| `a2a.max_tasks` | `CODEFORGE_A2A_MAX_TASKS` | `100` | Max concurrent A2A tasks (not enforced yet) |
| `a2a.allow_open` | `CODEFORGE_A2A_ALLOW_OPEN` | `false` | Allow AgentCard discovery without A2A API key |
| `a2a.streaming` | `CODEFORGE_A2A_STREAMING` | `false` | Enable A2A streaming |
| `agent.default_model` | `CODEFORGE_AGENT_DEFAULT_MODEL` | `` | Default agent model (empty = auto-discover) |
| `agent.max_context_tokens` | `CODEFORGE_AGENT_MAX_CONTEXT_TOKENS` | `128000` | Max context window tokens |
| `agent.max_loop_iterations` | `CODEFORGE_AGENT_MAX_LOOP_ITERATIONS` | `50` | Max tool-use loop iterations |
| `agent.agentic_by_default` | `CODEFORGE_AGENT_AGENTIC_BY_DEFAULT` | `true` | Enable agentic mode by default |
| `agent.tool_output_max_chars` | `CODEFORGE_AGENT_TOOL_OUTPUT_MAX_CHARS` | `10000` | Max chars per tool output (head and tail kept) for tool results in the agent loop and conversation history and for quality gate outputs (sent with `runs.start`, `conversation.run.start`, `runs.qualitygate.request`). 0 = the worker's default; allowed 0 to 80000, other values stop startup (two gate outputs must stay below the NATS max payload of 1 MiB); the worker clamps to the same range |
| `agent.auto_agent_fix_attempts` | `CODEFORGE_AGENT_AUTO_AGENT_FIX_ATTEMPTS` | `2` | Runs the auto-agent gets to fix a feature whose verification (change check, test and lint command) failed before the feature is marked failed; 0 to 10, other values stop startup (KI-152) |
| `agent.conversation_rollout_count` | `CODEFORGE_AGENT_CONVERSATION_ROLLOUT_COUNT` | `1` | Conversation rollout count (1-8) |
| `agent.summarize_threshold` | `CODEFORGE_SUMMARIZE_THRESHOLD` | `0` | Message count to trigger summarization (0 = disabled) |
| `litellm.health_poll_interval` | `CODEFORGE_LITELLM_HEALTH_POLL_INTERVAL` | `60s` | LiteLLM health poll interval |
| `plane.api_token` | `CODEFORGE_PLANE_API_TOKEN` (or `_FILE`) | `` | Plane.so API token for PM sync and Plane webhooks; serves only the default tenant (webhook syncs, imports), other tenants need their own `api_token` |
| `plane.base_url` | `CODEFORGE_PLANE_BASE_URL` | `https://api.plane.so` | Plane API the token belongs to (absolute http(s) URL); the token is sent only there, a project whose `plane_base_url` names another host is not synced by webhooks |
| `pm.allowed_private_hosts` | `CODEFORGE_PM_ALLOWED_PRIVATE_HOSTS` | `` (none) | Host names, IPs and CIDRs (comma-separated in the env) whose private addresses the GitLab PM provider may reach. Its base URL is a project's `repo_url` host or a manual sync's `base_url`, chosen by tenants. Loopback opens only by an explicit entry; link-local and cloud metadata addresses never. Invalid entries stop startup. Separate from `mcp.allowed_private_hosts`. A self-hosted GitLab on a private network must be listed. GitLab PM requests use no proxy |
| `copilot.hosts_file_path` | `CODEFORGE_COPILOT_HOSTS_FILE` | `` (falls back to `~/.config/github-copilot/hosts.json`) | Copilot hosts file path |
| `experience.enabled` | `CODEFORGE_EXPERIENCE_ENABLED` | `false` | Experience pool (Go and worker): tenant-scoped cache used only for the first turn of a simple (non-agentic) chat |
| `experience.confidence_threshold` | `CODEFORGE_EXPERIENCE_CONFIDENCE_THRESHOLD` | `0.85` | Minimum similarity to use a cached answer (0 < value <= 1) |
| `experience.max_entries` | `CODEFORGE_EXPERIENCE_MAX_ENTRIES` | `1000` | Max experience pool size |
| `app_env` | `APP_ENV` | `` | Application environment (`development`/`production`) |
| `internal_key` | `CODEFORGE_INTERNAL_KEY` | `` | Shared secret for worker-to-core API auth |
| `env_file` | `CODEFORGE_ENV_FILE` | `` | Path to .env file for OAuth device flow |

#### Python Worker Config (`workers/codeforge/config.py`, routing vars in `llm.py`, backend paths in `backends/*.py`)

| ENV Variable | Default | Description |
|---|---|---|
| `NATS_URL` | `nats://localhost:4222` | NATS server URL, with `user:password@` when the server requires authentication (production). Read from `NATS_URL_FILE` when that is set |
| `LITELLM_BASE_URL` | `http://localhost:4000` | LiteLLM Proxy URL |
| `LITELLM_MASTER_KEY` | `sk-codeforge-dev` | LiteLLM API key (dev default, matches the compose LiteLLM default; a warning is logged). Read from `LITELLM_MASTER_KEY_FILE` when that is set |
| `DATABASE_URL_FILE`, `NATS_URL_FILE`, `LITELLM_MASTER_KEY_FILE`, `CODEFORGE_INTERNAL_KEY_FILE` | unset | Path of a secret file for the setting without the suffix (production: `/run/secrets/<name>`). Setting both forms is a startup error; an empty or missing file is an error; the files are read once, then the worker locks its secrets directory (KI-71) |
| `CODEFORGE_TOOL_ISOLATION` | `off` (`required` in the worker image and in `docker-compose.prod.yml`) | `required`: agent tool processes start only as their tenant's tool user (`tool_uid` of the payload, 20000-29999) under Landlock, and every tool call fails with `ToolIsolationError` and `/health/ready` answers 503 when that is not possible; payloads without `tool_uid` are refused; `off`: they run as the worker user (development, tests). An unknown value counts as `required` |
| `CODEFORGE_TOOL_HOME_BASE` | `/home/codeforge-tools` | Base of the tenants' HOMEs (`<base>/<uid>`; production: the `tool_homes` volume, which must support POSIX ACLs and must not be mounted `noexec`) |
| `CODEFORGE_TOOL_PATH` | `/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin` | `PATH` of tool processes (the HOME's `.local/bin`, `go/bin`, `.cargo/bin` and `.npm-global/bin` follow); never the worker's venv. In the worker image its interpreter has pytest and ruff from `workers/tool-requirements.txt`; add toolchain bin directories here |
| `CODEFORGE_TOOL_LANDLOCK` | unset (follows `CODEFORGE_TOOL_ISOLATION`) | `required` or `off`; an unknown value counts as `required`; `off` is refused with `APP_ENV=production` (the worker is not ready) and logs a warning elsewhere: without Landlock tool command lines are readable across tenants |
| `CODEFORGE_TOOL_LANDLOCK_MIN_ABI` | `2` | Lowest Landlock ABI the worker accepts (ABI 2: no truncate handling; scopes for signals and abstract sockets need ABI 6, otherwise a warning is logged) |
| `CODEFORGE_TOOL_READ_PATHS` | unset | Colon-separated operator directories tool processes may read and execute (for example `/opt`, or `/app/.venv` for backend CLIs installed there); each must be absolute, exist and contain no symlink; `/`, `/proc`, `/sys`, `/run`, `/tmp`, `/data`, `/home`, `/var/lib/codeforge`, their ancestors and anything below `/run`, `/proc`, `/data`, `/home/codeforge-tools` or `/var/lib/codeforge` are refused (the worker is then not ready) |
| `CODEFORGE_TOOL_CACHE_MAX_MB` | `4096` | When a tenant has no work left in the worker, its `<HOME>/.cache` is removed as the tenant if it is larger than this |
| `CODEFORGE_WORKSPACE_GID` | `10010` | Workspace group (`codeforge-ws`) of the worker and the Go Core; tool processes are not in it (KI-96) |
| `CODEFORGE_WORKSPACE_ROOT` | unset | The Go Core's workspace root (same variable, `/data/workspaces` in production); required with isolation. At startup the worker sets it to 2771, prepares its state directory `<root>/.codeforge` (UID bindings, migration stamps, locks) and detects a rollback; each tenant's tree from before KI-96 is migrated at its first work item |
| `CODEFORGE_KNOWLEDGE_CONTENT_ROOT` | `data/knowledge` | Knowledge content root (same setting as the Core's `knowledge.content_root`): the worker indexes knowledge bases only below `<root>/<tenant_id>/` and refuses anything else (KI-105) |
| `CODEFORGE_WORKER_LOG_LEVEL` | `info` | Worker log level (falls back to `logging.level` in codeforge.yaml) |
| `CODEFORGE_WORKER_LOG_SERVICE` | `codeforge-worker` | Worker service name |
| `CODEFORGE_WORKER_HEALTH_PORT` | `8081` | Worker HTTP health server: `GET /health` (liveness) and `GET /health/ready` (NATS connected, every consumer loop alive, notification consumers restored, not stopping; `503 {"status":"tool isolation not ready: <reason>"}` while tool isolation is required but not ready, `503 {"status":"starting"}` before the consumer starts, `503 {"status":"not ready"}` otherwise); `0` picks a free port; a port that cannot be bound makes the worker exit 1 before connecting to NATS. Two workers on one host need different ports |
| `CODEFORGE_AIDER_PATH` | `aider` | Path to Aider CLI binary |
| `CODEFORGE_GOOSE_PATH` | `goose` | Path to Goose CLI binary |
| `CODEFORGE_OPENCODE_PATH` | `opencode` | Path to OpenCode CLI binary |
| `CODEFORGE_PLANDEX_PATH` | `plandex` | Path to Plandex CLI binary |
| `CODEFORGE_OPENHANDS_URL` | `http://localhost:3000` | OpenHands service URL |
| `CODEFORGE_CLAUDECODE_ENABLED` | `false` | Enable Claude Code as routing target |
| `CODEFORGE_CLAUDECODE_PATH` | `claude` | Path to the Claude Code CLI binary; it must pass the capability check (flags `--tools`, `--setting-sources`, `--strict-mcp-config`, `--permission-mode dontAsk`, `--system-prompt-file`, ...; 2.1.x tested), otherwise claudecode is not routed to. Runs create a private `/tmp/cf-cc-*` directory. Managed Claude Code settings must not set `disableAllHooks` or `allowManagedHooksOnly` |
| `CODEFORGE_CLAUDECODE_MAX_TURNS` | `50` | Default max agentic turns per Claude Code run |
| `CODEFORGE_CLAUDECODE_TIMEOUT` | `300` | Run time limit per Claude Code turn in seconds (approval waits not counted) |
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
| `CODEFORGE_BENCHMARK_DATASETS_DIR` | `configs/benchmarks` | Benchmark datasets directory; every dataset is read below it (an absolute path only inside it). A relative value is looked up below the worker's working directory, then its parent (in dev the worker runs from `workers/`), then `CODEFORGE_WORKSPACE`; set an absolute path when none of them has `configs/benchmarks` (KI-107) |
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

The Go Core and Python Workers communicate via NATS JetStream subjects. The tables below are a subset; the authoritative lists are `internal/port/messagequeue/queue.go` (Go) and `workers/codeforge/nats_subjects.py` (Python). Not listed here: `runs.heartbeat`, `runs.qualitygate.*`, `runs.trajectory.event`, `benchmark.task.*`, `context.shared.updated`, `context.rerank.*`, `repomap.generate.*`, `conversation.run.*`, `conversation.compact.*`, `conversation.test.*` (workspace test run of the auto-agent, worker side), `evaluation.gemmas.*`, `a2a.task.*`, `memory.*`, `handoff.request` (worker -> Go Core) and `handoff.approved` (Go Core only), `backends.health.*`, `prompt.evolution.*`. Dead-letter copies (`<subject>.dlq`) of `runs.start`, `conversation.run.start`, `tasks.agent.*`, `handoff.request`, `handoff.approved` and `benchmark.run.request` end the work they carried as failed in the Go Core.

In production a new subject, worker durable or notification consumer also needs an entry in `configs/nats/nats-server.conf` (the `core` and `worker` users publish only their own subjects and the worker may use only consumers named there); `workers/tests/test_nats_permissions.py` fails without it. See [Tool Isolation and NATS Authentication](#tool-isolation-and-nats-authentication).

#### Legacy Task Protocol (fire-and-forget)

| Subject | Direction | Purpose |
|---------|-----------|---------|
| `tasks.agent.<name>` | Go -> Python | Dispatch task to agent backend (name = aider/goose/openhands/opencode/plandex) |
| `tasks.result` | Python -> Go | Task result from worker |
| `tasks.output` | Python -> Go | Streaming output line |
| `tasks.cancel` | Go -> Python | Cancel a task: the worker stops the backend's process group; a cancel for a task still queued is remembered so it is not started later |
| `tasks.heartbeat` | Python -> Go | Every `heartbeat_seconds` (from `runtime.heartbeat_interval`) while a worker executes a task; carries the dispatch ID of the task message |
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

#### Workspace Deletion (KI-96)

| Subject | Direction | Purpose |
|---------|-----------|---------|
| `workspace.delete.request` | Go -> Python | Remove a deleted project's workspace as the tenant's tool UID (`deletion_id`, `tenant_id`, `tool_uid`, `project_id`, `workspace_path`); at least once, durable `codeforge-py-workspace-delete-request`, a missing workspace counts as removed; the Core republishes pending deletions every 10 minutes and treats `workspace.delete.request.dlq` as a failed attempt |
| `workspace.delete.result` | Python -> Go | Outcome (`deletion_id`, `ok`, `error`); the Core marks the `workspace_deletions` row done or records the error |

Only with `workspace.tool_acls: required`; otherwise the Go Core removes the workspace itself.

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

# Python Worker (python:3.12-slim, poetry; starts as root, the entrypoint runs the worker as uid 10001, tool processes as the tenants' tool users 20000-29999)
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

Production layout (since 2026-09-30): PostgreSQL 18 with TLS (self-signed certificate from `generate-secrets.sh`, copied to a tmpfs by an entrypoint wrapper; clients use `sslmode=require`), the core with a read-only root filesystem plus volumes `core_data` (`/data`, holds `data/policies`, `data/initial_admin_password`) and `workspaces` (`/data/workspaces`, shared with the worker at the same path) plus the read-only `knowledge` volume (`/data/knowledge`, mounted in core and worker: knowledge-base content in `<tenant_id>/` subdirectories that the operator fills, KI-105), tmpfs `/tmp` for core and worker, the core running as UID/GID 10001 and the worker starting as root with only `SETUID`, `SETGID` and `KILL` and running as UID 10001 while agent tool processes run as their tenant's tool UID (20000-29999) with HOMEs on the `tool_homes` volume (`/home/codeforge-tools`) and the worker's `/tmp` at mode 1771 (see [Tool Isolation and NATS Authentication](#tool-isolation-and-nats-authentication)), NATS pinned to `nats:2.15-alpine` with authenticated users, LiteLLM `v1.103.1` on the `internal` and `egress` networks with `host.docker.internal` mapped to the host gateway (local model servers). All credentials come from Docker secret files, see [Secret Management](#secret-management). Zero-downtime deployments: see [Blue-green deployment](#blue-green-deployment).

#### Blue-green deployment

`docker-compose.blue-green.yml` is an overlay of `docker-compose.prod.yml` that runs two colors of the core and the
frontend behind Traefik (the only service that publishes ports, 80 and 443). The colors are Compose profiles
(`blue`, `green`), so a plain `up -d` starts neither.
During a switch both cores hand worker results and HITL decisions to the core that waits for them (core NATS
`core.relay.*`, KI-86): approvals, approval pages and auto-agent waits work on either color. WebSocket events and the
auto-agent registry stay per core (KI-139).

```bash
export ACME_EMAIL=ops@example.com          # Let's Encrypt account email (required)
export CODEFORGE_DOMAIN=codeforge.example.com   # public host name (required); or set both in .env

# 1. Shared services first: postgres, nats, litellm, worker and Traefik (no color yet)
docker compose -f docker-compose.prod.yml -f docker-compose.blue-green.yml up -d

# 2. Start a color; the script waits until it is healthy, then stops the other one
./scripts/deploy-blue-green.sh            # the color that is not running (blue first)
./scripts/deploy-blue-green.sh green      # or name the color
DRY_RUN=1 ./scripts/deploy-blue-green.sh  # print the plan; compose commands run with --dry-run
```

- Run step 1 once (and again whenever the shared services change). The script refuses to run unless postgres, nats
  and litellm are running and healthy, and it starts a color with `--no-deps`, so a deployment never recreates them.
- Traefik must be v3.6 or later (the pinned image is `traefik:v3.6`): older Docker providers speak an API version that
  Docker Engine 29 refuses. It is configured with command flags in the overlay (ACME HTTP challenge, Docker provider
  on the network `codeforge-public`, rate-limit and security-header middlewares from `traefik/dynamic/`); there is no
  `traefik/traefik.yaml`.
- Routing: `/api`, `/health`, `/ws`, `/.well-known` and `/a2a` go to the core of the running color, everything else to
  its frontend. Each frontend's nginx proxies to its own core through `CORE_UPSTREAM` (default `core:8080`, the
  overlay sets `core-blue:8080` / `core-green:8080`). Both cores carry the network alias `core`, which the worker
  uses (`CODEFORGE_CORE_URL`); during a switch it resolves to both.
- The overlay uses the Compose tags `!reset` and `!override`; pre-commit runs `check-yaml --unsafe` on this one file.
- A failed color is stopped again and the active one keeps serving. Backup and restore: [disaster-recovery.md](disaster-recovery.md).

#### CI/CD

CI (`.github/workflows/ci.yml`) runs on pushes to `main`/`staging` and on pull requests to `main` and `staging`: Go build, `go vet -tags=integration`, unit tests with `-race`, `integration`-tagged tests against PostgreSQL + NATS, golangci-lint v2.11.4; Python (`poetry run ruff check .`, `poetry run ruff format --check .` with the Poetry-pinned ruff 0.15.1, `poetry run pytest`); frontend lint, format check, type check (`npm run typecheck`), unit tests (`npm run test`, vitest) and build; Lighthouse CI, contract tests and security scanning (govulncheck, npm audit, pip-audit and gitleaks; `.gitleaks.toml` allowlists exact synthetic test-fixture values, never paths); smoke tests and feature verification run only on pushes to `staging`/`main`. The Python job also runs the tool isolation tests as root (`sudo -E`, `CODEFORGE_ISOLATION_TESTS=required`), and the job `tenant-isolation-docker` builds the worker and battery images, prints the kernel, LSMs and Landlock ABI, runs `scripts/check-tool-isolation.sh` and the Docker suite (1,000,000 migration entries) and uploads the evidence (KI-96).

GitHub Actions automatically builds and pushes Docker images to `ghcr.io` on push to `main`/`staging` and on version tags. See `.github/workflows/docker-build.yml`. Each build job records the pushed image by digest and the Grype scan job scans exactly those references.

#### Versioning and Release

The root `VERSION` file (a semver string, currently `0.8.0`) is the single source of the version. To change it, edit `VERSION` and run `./scripts/sync-version.sh`, which copies it into `pyproject.toml`, `frontend/package.json` and the root entries of `frontend/package-lock.json`; every layer then picks it up. Releases are merged to `main` only on an explicit request of the project owner.

| Layer | Mechanism | Key file |
|---|---|---|
| Go | `internal/version` reads `VERSION` at startup (working directory, then up to two parents; `dev` if none); a build overrides it with `-ldflags "-X .../internal/version.Version=... -X .../internal/version.GitSHA=..."` | `internal/version/version.go` |
| Python | `_read_version()` reads `VERSION` from the project root, then the working directory (`dev` if none) | `workers/codeforge/__init__.py` |
| Frontend | Vite reads `../VERSION` at build time and defines `__APP_VERSION__` | `frontend/vite.config.ts` |
| Docker | `docker-build.yml` reads `VERSION` and passes `APP_VERSION` and `GIT_SHA` as build args; the Go image sets them via ldflags, the worker and frontend images copy `VERSION`; all three carry the OCI labels `org.opencontainers.image.version` and `.revision` | `Dockerfile`, `Dockerfile.worker`, `Dockerfile.frontend`, `.github/workflows/docker-build.yml` |

Image tags (`ghcr.io/<owner>/codeforge-core`, `-worker`, `-frontend`): a push to `main` or `staging` tags the branch name and the short commit SHA. A `v*` tag on main that names `VERSION` builds `<version>`, `<major>.<minor>` and `latest` (not for prereleases); any other `v*` tag fails the workflow. `docker-compose.prod.yml` pins `:<VERSION>`, and `./scripts/sync-version.sh` rewrites those pins (KI-116). `docker-compose.egress.yml` is an opt-in override that gives the worker (and so every tenant's tool processes) a route to the internet, for agent backends and external MCP servers that cannot go through LiteLLM. The worker image sets `AIDER_ANALYTICS_DISABLE=true`, `AIDER_CHECK_UPDATE=false`, `OPENCODE_DISABLE_AUTOUPDATE=true` and `GOOSE_DISABLE_KEYRING=1`; Claude Code still needs `CODEFORGE_CLAUDECODE_ENABLED=true` plus `ANTHROPIC_API_KEY` or `CLAUDE_CODE_OAUTH_TOKEN` in the worker environment, and OpenHands needs `CODEFORGE_OPENHANDS_URL`.

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
| DOCS_MCP_API_BASE         | http://host.docker.internal:1234/v1      | Embedding API endpoint of the dev docs-mcp service |
| DOCS_MCP_API_KEY          | lm-studio                                | API key for embeddings (docs-mcp) |
| DOCS_MCP_EMBEDDING_MODEL  | text-embedding-nomic-embed-text-v1.5     | Embedding model name (docs-mcp) |
| OPENAI_API_KEY            | (optional)                               | OpenAI API Key (via LiteLLM)    |
| ANTHROPIC_API_KEY         | (optional)                               | Anthropic API Key (via LiteLLM) |
| GEMINI_API_KEY            | (optional)                               | Google Gemini API Key           |
| GROQ_API_KEY              | (optional)                               | Groq API Key (fast inference)   |
| MISTRAL_API_KEY           | (optional)                               | Mistral AI API Key              |
| OPENROUTER_API_KEY        | (optional)                               | OpenRouter API Key              |
| POSTGRES_PASSWORD         | (required)                               | PostgreSQL password              |
| OLLAMA_BASE_URL           | http://host.docker.internal:11434        | Ollama endpoint (local, without `/v1`); used by Go model discovery; the compose files pass it to LiteLLM as `OLLAMA_OPENAI_API_BASE=<url>/v1` (the `ollama/*` route, Ollama's OpenAI-compatible endpoint, which keeps tool calls structured) and as `OLLAMA_API_BASE=<url>` (LiteLLM reads model capabilities and context windows from Ollama) |
| CODEFORGE_OTEL_ENABLED    | false                                    | Enable OpenTelemetry tracing    |
| CODEFORGE_OTEL_ENDPOINT   | localhost:4317                              | OTLP gRPC endpoint              |
| CODEFORGE_OTEL_SERVICE_NAME | codeforge-core                          | OTEL service name               |
| CODEFORGE_OTEL_SAMPLE_RATE | 1.0                                     | Trace sampling rate (0.0-1.0)   |
| CODEFORGE_A2A_ENABLED     | false                                    | Enable A2A protocol endpoints   |
| CODEFORGE_AGUI_ENABLED    | false                                    | Reserved, currently has no effect (AG-UI run events are always emitted) |
| CODEFORGE_MCP_ENABLED     | false                                    | Enable MCP integration          |
| CODEFORGE_MCP_SERVERS_DIR |                                          | MCP server YAML definitions dir |
| CODEFORGE_MCP_SERVER_PORT | 3001                                     | Built-in MCP server port        |
| CODEFORGE_MCP_ALLOWED_PRIVATE_HOSTS |                                | Comma-separated hosts, IPs, CIDRs whose private addresses sse/streamable_http MCP servers may use; loopback only by an explicit entry; metadata never |
| CODEFORGE_MCP_USE_PROXY   | false                                    | Connect sse/streamable_http MCP servers through the environment proxy (address not pinned) |
| CODEFORGE_PM_ALLOWED_PRIVATE_HOSTS |                                 | Comma-separated hosts, IPs, CIDRs whose private addresses the GitLab PM provider may reach (a self-hosted GitLab); loopback only by an explicit entry; metadata never |
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
| CODEFORGE_QUARANTINE_EXPIRY_HOURS | 72                                | Hours until unreviewed messages expire (`quarantine.expiry_hours`, stored as `expires_at`; 1 to 2562047, anything else stops startup); the stuck-work watchdog sets overdue messages to `expired` and rejects a held A2A task with them, KI-91 |
| CODEFORGE_LSP_ENABLED       | false                                    | Enable LSP integration           |
| CODEFORGE_ORCH_REVIEW_ROUTER_ENABLED | false                          | Enable confidence-based review routing |
| CODEFORGE_ORCH_EMBEDDING_MODEL | text-embedding-3-small            | Embedding model for code retrieval (`orchestrator.default_embedding_model`); without a usable model (401, 403, 404, a 400 for an unknown model, or any answer but a 429 naming an authentication or API key problem) retrieval runs BM25-only and warns once; a cloud model without its key is still called (LiteLLM may route the name to a local server) and its authentication error makes the index BM25-only; the warning names the answer; a rate limit, server error or timeout fails the build and keeps the existing index; the index status reports `bm25_only`, and /search names BM25-only and failed indexes (KI-150) |
| CODEFORGE_LITELLM_KEYED_PROVIDERS | (empty; set by docker-compose.prod.yml) | Core and worker: providers whose key LiteLLM has, names only (`litellm.keyed_providers`); compose derives it with `${OPENAI_API_KEY:+openai,}...` (Compose v2 `${VAR:+replacement}` interpolation, checked with v5.1.1) |
| CODEFORGE_ORCH_REVIEW_CONFIDENCE_THRESHOLD | 0.7                      | Steps below this get routed to review |
| CODEFORGE_ORCH_REVIEW_ROUTER_MODEL |                                  | LLM model for review evaluation  |
| CODEFORGE_COPILOT_ENABLED   | false                                    | Enable GitHub Copilot token exchange |
| CODEFORGE_ROUTING_ENABLED   | true                                     | Enable hybrid intelligent routing |
| CODEFORGE_MODEL_CAPABILITIES | (empty) | Worker: tool capability per model, comma-separated `pattern=level` entries (`litellm.model_capabilities` in YAML); case-sensitive shell globs, first match wins; levels `full`, `api_with_tools`, `pure_completion`; overrides LiteLLM's `supports_function_calling` and the name heuristics; an invalid entry stops the worker at startup; a YAML `litellm.model_capabilities` that is not a mapping stops the worker at startup |
| CODEFORGE_TEXT_TOOL_GRAMMAR | true | Worker: `pure_completion` models call tools through the text tool protocol (ADR-021); true constrains the reply with a JSON-schema grammar where the server supports it (`litellm.text_tool_grammar` in YAML); false for thinking models or servers whose output gets worse under a grammar. Ollama: set `OLLAMA_CONTEXT_LENGTH`, the worker assumes 16k |
| CODEFORGE_EXPERIENCE_ENABLED | false                                   | Enable the experience pool (worker and Go); tenant-scoped, first turn of a simple chat only |
| CODEFORGE_TEST_DATABASE_URL | (unset)                                  | PostgreSQL URL for the worker's database tests (experience pool, skills); they are skipped when unset |
| CODEFORGE_A2A_BASE_URL     | `http://localhost:<CODEFORGE_PORT>`      | Public URL for AgentCard         |
| CODEFORGE_A2A_API_KEYS     |                                          | Comma-separated A2A API keys: `<key>` or `<tenant-uuid>:<key>` |
| CODEFORGE_A2A_TRANSPORT    | jsonrpc                                  | Transport protocol (only `jsonrpc` is implemented) |
| CODEFORGE_A2A_MAX_TASKS    | 100                                      | Max concurrent A2A tasks (not enforced yet) |
| CODEFORGE_A2A_ALLOW_OPEN   | false                                    | Allow AgentCard discovery without A2A API key |
| CODEFORGE_OTEL_INSECURE    | false                                    | Use plaintext gRPC (Go Core and worker); set `true` for a local collector such as Jaeger |
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
| DEEPEVAL_TELEMETRY_OPT_OUT  | YES (forced by the worker)               | deepeval telemetry off; see the note below |

**Note on deepeval telemetry:** the `deepeval` dependency (benchmark evaluation) would by
default look up the public IP, start Sentry/PostHog telemetry, and upload metric results and
traces to Confident AI when it finds a key. The worker forces these settings before any deepeval
import (`workers/codeforge/evaluation/_deepeval_env.py`; `Dockerfile.worker` sets them as `ENV`
too), so an operator setting cannot switch the traffic back on: `DEEPEVAL_TELEMETRY_OPT_OUT=YES`,
`DEEPEVAL_UPDATE_WARNING_OPT_IN=0`, `CONFIDENT_METRIC_LOGGING_ENABLED=NO`,
`CONFIDENT_TRACING_ENABLED=NO`, `DEEPEVAL_DISABLE_DOTENV=1`, `DEEPEVAL_DISABLE_LEGACY_KEYFILE=1`.
Do not set `ERROR_REPORTING` to a true value in the worker environment (deepeval then connects
to an external host on import).

### Secret Management

In development, secrets are loaded from environment variables (`.env` file).

In production, every secret comes from a Docker secret file. The Go Core reads `<KEY>_FILE` for its secret settings
(`CODEFORGE_INTERNAL_KEY`, `DATABASE_URL`, `NATS_URL`, `LITELLM_MASTER_KEY`, `CODEFORGE_AUTH_JWT_SECRET`,
`CODEFORGE_AUTH_LLM_KEY_ENCRYPTION_SECRET`, the admin password, GitHub client secret, SMTP password,
Plane token, A2A keys; `internal/secrets.LookupFileEnv`). Setting both `KEY` and `KEY_FILE` stops the core at startup;
a missing or empty file is an error; a trailing newline is trimmed; list settings (A2A keys) split on commas and
newlines. The worker reads `DATABASE_URL_FILE`, `NATS_URL_FILE`, `LITELLM_MASTER_KEY_FILE` and `CODEFORGE_INTERNAL_KEY_FILE`
the same way (no secret in its environment). PostgreSQL uses `POSTGRES_PASSWORD_FILE`, NATS reads the passwords of its users
from a generated `nats-passwords.conf`, and LiteLLM exports its values from the files in an entrypoint wrapper. Agent tool
subprocesses never inherit these values (scrubbed environment, `workers/codeforge/subprocess_env.py`) and cannot read the
files (tool user, secrets tmpfs). URL credentials are redacted in all logs.

```bash
./scripts/generate-secrets.sh        # writes ./secrets next to docker-compose.prod.yml (or SECRETS_DIR from env/.env)
./scripts/validate-env.sh            # checks the files the compose file mounts
docker compose -f docker-compose.prod.yml up -d
```

`generate-secrets.sh` creates missing secrets as hex values (`nats-core-pass` and `nats-worker-pass` among them), a PostgreSQL TLS pair
and the derived `database-url`, `nats-core-url`, `nats-worker-url` and `nats-passwords.conf` (written only when missing or when
their inputs were just generated, so edits such as `sslmode=verify-full` survive). The pre-KI-71 files `nats-user`,
`nats-pass`, `nats-url` and `nats-auth.conf` are unused; the script says so and you may delete them. It reads `POSTGRES_USER`/`POSTGRES_DB` like compose (environment, then `.env`).
Rotation: `codeforge-internal-key`, `nats-core-pass` and `nats-worker-pass` rotate by deleting the file and re-running (the NATS URLs and `nats-passwords.conf` follow; then recreate the services);
`postgres-password` must be changed in the database first (`ALTER USER`), then in `postgres-password` and
`database-url`; the JWT secret (logs everyone out, VCS tokens become unreadable), the LLM key encryption secret and the
LiteLLM master key are never regenerated for a directory in use (the script stops and explains). A directory counts as in use once any file only the script creates exists (derived files, JWT or LLM key secret, TLS pair); a directory with only operator pre-seeded files (e.g. your own `postgres-password`) is treated as new. For an existing
installation the LLM key encryption secret is created with the JWT secret's value, so stored LLM keys stay readable.
`validate-env.sh` checks presence, readability by the non-root containers, length, known dev defaults, that
`database-url` matches `POSTGRES_USER`/`POSTGRES_DB` and does not disable TLS, that `nats-core-url` and `nats-worker-url`
connect as the users `core` and `worker` with the passwords in `nats-passwords.conf`, and that the two passwords differ.

See `docs/SECURITY.md` for the full secret management policy.

### Tool Isolation and NATS Authentication

Design and rationale: [ADR-017](architecture/adr/017-tool-isolation-and-nats-authentication.md) and, for per-tenant tool
users and Landlock, [ADR-018](architecture/adr/018-per-tenant-tool-identities-and-landlock.md); the security model:
[SECURITY.md](SECURITY.md#agent-tool-isolation); the process and UID model:
[architecture.md](architecture.md#process-and-uid-model).

**Production.** The worker container starts as root with `cap_drop: ALL`, `cap_add: SETUID, SETGID, KILL` and
`no-new-privileges`; `scripts/worker-entrypoint.sh` runs the worker as uid 10001 (group `codeforge-ws` 10010) with those
capabilities as ambient capabilities. Agent tool processes run as uid/gid T, the tenant's tool UID (20000-29999), with
no supplementary group, no capabilities, umask 007 and Landlock, started only through `workers/codeforge/tool_process.py`.
The worker mounts `/run/secrets` as a tmpfs (`uid=10001,mode=0700`) with the secret files inside, the `tool_homes` volume at
`/home/codeforge-tools` (the tenants' HOMEs) and `/tmp` as a tmpfs with `uid=10001,gid=10010,mode=1771`; the workspaces
volume is `10001:10010` with the root at mode 2771; the core runs with `CODEFORGE_WORKSPACE_TOOL_ACLS=required`. The NATS server loads `configs/nats/nats-server.conf`
(Compose `configs:`) and the secret `nats-passwords.conf`; the core connects with `nats-core-url`, the worker with
`nats-worker-url` (both mounted as `/run/secrets/nats-url`, also in the blue-green overlay).

**Upgrading from before KI-71.**

```bash
git pull                                         # configs/nats/ must be in the deploy directory
./scripts/generate-secrets.sh                    # creates nats-core-pass, nats-worker-pass and the derived NATS files
./scripts/validate-env.sh                        # Compose fails until the new NATS files exist
docker compose -f docker-compose.prod.yml pull   # or build; the NATS image is nats:2.15-alpine
docker compose -f docker-compose.prod.yml up -d
```

The Go Core applies migrations 111 (`quarantine_messages.consumed_at`) and 112 (tenant of MCP links; it deletes links
between a project and a server of different tenants). The first worker start walks the existing workspaces once
(`/health/ready` answers `503 {"status":"starting"}` meanwhile) and logs "workspaces opened to the workspace group". A
workspace root the worker does not own is not walked: the log names the fix (`chown 10001:10010 <root> && chmod 2775
<root>`). `CLAUDE_CONFIG_DIR` and adopted workspaces outside `/data/workspaces` must be accessible to group 10010. Platforms
that forbid root containers cannot start tool processes: every tool call fails with `ToolIsolationError`.

**Python tools for tool processes.** Tool processes never use the worker's venv. `workers/tool-requirements.txt` lists
pytest (with its dependencies) and ruff, hash-pinned to poetry.lock; `Dockerfile.worker` installs it into the image's system
interpreter. When poetry.lock changes, `workers/tests/test_tool_requirements.py` fails and prints the file's new content.
Further tools for agents go on `CODEFORGE_TOOL_PATH` (and, outside `/usr`, `CODEFORGE_TOOL_READ_PATHS`).

**Checking it.** The worker logs "tool isolation required: every tenant's tool processes run as the tenant's tool UID"
(with the Landlock mode and ABI), or the error and its reason, once at startup. `./scripts/check-tool-isolation.sh` runs
the check of two tenants in the worker image with the production settings and prints each tool process's credentials (Uid
and Gid T, Groups empty, `CapEff`/`CapAmb` 0, `NoNewPrivs` 1, Umask 0007), what it can reach of the worker, the other
tenant and its own tenant's other project, and whether a command line carries its environment. The full suite:

```bash
CODEFORGE_TEST_WORKER_IMAGE=<image> poetry run pytest workers/tests/test_tenant_isolation_docker.py
```

**Development.** Isolation is `off` and the dev compose runs NATS without authentication; nothing changes for
`go run`, `poetry run` or the devcontainer. To try isolation locally run the worker image with the production settings
(`docker compose -f docker-compose.prod.yml`, or `scripts/check-tool-isolation.sh`).

| Config / secret | Purpose |
|---|---|
| `configs/nats/nats-server.conf` | NATS users `core` and `worker`, their permissions, per-service inbox prefixes; keep it in step with `internal/port/messagequeue/queue.go` and `workers/codeforge/nats_subjects.py` |
| `nats-core-pass`, `nats-worker-pass` (secrets) | Passwords of the two NATS users |
| `nats-core-url`, `nats-worker-url`, `nats-passwords.conf` (derived) | URLs with credentials for the core and the worker; the password file the NATS config includes |
| `scripts/worker-entrypoint.sh` | Image entrypoint of the worker |
| `scripts/check-tool-isolation.sh` | Container check of the isolation (two tenants) |
| `scripts/check-host.sh` | Host preflight before the KI-96 upgrade |
| `workers/tool-requirements.txt` | Python tools (pytest, ruff) of the tool PATH in the worker image |

#### Upgrading to per-tenant tool users (KI-96)

```bash
tar --acls --xattrs -C <workspaces volume path> -czf workspaces-backup.tgz .   # or a filesystem snapshot
WORKER_IMAGE=<new worker image> ./scripts/check-host.sh                        # must print "result: OK"
docker compose -f docker-compose.prod.yml stop worker                          # every replica; no old worker may run again
docker compose -f docker-compose.prod.yml up -d core                           # migrations 120, 121
docker compose -f docker-compose.prod.yml up -d worker
```

- **Back up first** with a tool that keeps POSIX ACLs and xattrs. A restore without ACLs fails closed: each tenant
  migrates again at its next work item.
- **Preflight.** `scripts/check-host.sh` runs `codeforge.host_check` in the new worker image with the production service
  definition against the real `workspaces` and `tool_homes` volumes (extra compose files go before `run`, for example
  `-f docker-compose.blue-green.yml`). It prints the host's kernel, LSM list and Docker version, the Landlock ABI (ENOSYS:
  no Landlock or a seccomp profile that blocks it; EOPNOTSUPP: not in the `lsm=` list), POSIX ACLs, file systems and mount
  options of both volumes, the mode of `/tmp` and the worker's isolation check, and exits non-zero on any failure. It
  prepares the volumes as the new worker's start does (root 2771, `.codeforge`); a running old worker keeps working.
- **Stop every worker** (all replicas of `--scale worker=N`) before the new Core starts: an old worker runs tools as 10002
  in group 10010, which can reach every tenant tree, and takes no tenant lock. `deploy-blue-green.sh` handles only the
  core and frontend colors, so the worker steps are manual there.
- **The Core** applies migrations 120 (`tenants.tool_uid`) and 121 (`workspace_deletions`), advances the UID sequence over
  the bindings on the volume, and logs every adopted workspace outside the root it cannot open itself, with the tenant, its
  tool UID and the command (`setfacl -R -m u:<T>:rwX -m d:u:<T>:rwX -m g:10010:rwX -m d:g:10010:rwX <dir>`); platform
  admins see `tool_uid` in `GET /api/v1/tenants`.
- **The new worker** answers `/health/ready` with 503 and the reason until isolation is ready. Each tenant's tree migrates
  at its first work item (in a thread; a log line gives counts and time, about 0.7 s per 100,000 entries with warm caches).
- **Host requirements** (otherwise the worker stays 503 and runs no tool):

| Requirement | Works | Does not work -> remedy |
|---|---|---|
| Landlock ABI >= 2, and `landlock` in the LSM list | Debian 12 (6.1, ABI 2, no truncate handling), Amazon Linux 2023 (6.1, ABI 2), Ubuntu 22.04 HWE and 24.04 (6.8, ABI 4), Debian 13 (6.12, ABI 6), GitHub ubuntu-24.04 runners (ABI 7) | Ubuntu 22.04 GA (5.15, ABI 1): HWE kernel. Kernels without Landlock in `lsm=`: enable it. There is no production mode without Landlock. |
| Docker's seccomp profile allows `landlock_*` | Docker 23.0 or later (20.10 with the backport) | Older engines return ENOSYS: upgrade Docker |
| POSIX ACLs on `workspaces` and `tool_homes` | ext4, xfs, btrfs; ZFS with `acltype=posixacl` | ZFS default (`acltype=off`), NFSv4, CIFS, ramfs: use a local file system or set `acltype=posixacl` |
| `tool_homes` not mounted `noexec` | a named local volume | a tmpfs (Docker's tmpfs is `noexec`) |
| All workers on one host | `flock` across containers on one volume | workers on several hosts sharing storage |

- **Visible changes.** Tenant creation (`POST /api/v1/tenants`) is for platform admins only. Deleting a project answers
  409 while it has active work, and the worker removes the workspace asynchronously. A tenant's first workspace or tool work answers 503
  when all 10,000 tool UIDs are taken. Tools cannot use `ps`, `pkill`, `df`, `ss`, `/proc/self` in child processes or
  `/tmp`; background processes end when the tenant's work in that worker ends. Files from before the upgrade stay owned by
  10002 until replaced. With isolation required an operator's `CLAUDE_CONFIG_DIR` no longer reaches Claude Code (use
  `CLAUDE_CODE_OAUTH_TOKEN` or `ANTHROPIC_API_KEY`).
- **Rollback.** Stop the new workers, then start the old images; old tools keep working through the `g:10010` entries.
  Do not run the down migrations of 120 and 121 unless the workspaces are reset too. A later re-upgrade detects the
  rollback and migrates every tenant again.

### Inbound Webhooks (KI-85)

VCS and PM webhooks are registered per project (admins; editors can list them without secrets), in the UI under the project's **Webhooks** panel (project settings > Manage webhooks; KI-109) or through the API below. The panel shows the full inbound URL built from the browser's origin (use the address the provider reaches CodeForge under if that differs, e.g. behind a reverse proxy) and a generated secret exactly once. Each has a random ID, its own URL and its own secret,
which name the tenant and the project; `X-Tenant-ID` is never read on `/api/v1/webhooks/`. Security model: [SECURITY.md](SECURITY.md#security-measures).

```bash
# kind: vcs (github, gitlab) or pm (github, gitlab, plane); api_token: PM only (GitLab PRIVATE-TOKEN, GitHub GH_TOKEN, Plane api_token)
curl -X POST "$API/api/v1/projects/$PROJECT_ID/webhooks" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"kind":"pm","provider":"gitlab","api_token":"<gitlab token>"}'
# -> {"id": "...", "url": "/api/v1/webhooks/pm/gitlab/<id>", "secret": "<shown once>", ...}
```

Enter `$API_ORIGIN` + `url` and the `secret` at the provider (GitHub: payload URL and secret, content type JSON; GitLab: URL and secret token). Plane
generates its own secret: create the webhook in Plane first, then register it with `"secret": "<Plane's secret>"` (16 to 1024 printable characters).
`POST .../webhooks/{webhookId}/rotate` issues a new secret (for Plane: send Plane's new secret as `{"secret": ...}`), `PUT .../webhooks/{webhookId}/api-token`
sets or removes (`""`) the PM token, `DELETE` removes the webhook. An event is acted on only when it names the project's repository exactly
(host and owner/name of `repo_url`, case-insensitive; Plane: `plane_project_id`); a PM webhook syncs with its own `api_token`.

#### Upgrading to per-project webhooks (KI-85)

1. The old global webhook URLs (`/api/v1/webhooks/{vcs,pm}/{github,gitlab,plane}`) answer 410. Remove the `webhook.*` secret settings (they are ignored; the startup log names them).
2. Register each integration per project and enter the returned `url` (prefixed with the API origin) and the `secret` at the provider. For Plane, create the webhook in Plane first, then register it with Plane's secret.
3. Check that each project's `repo_url` names the exact host and owner/name; other events are ignored.
4. Rotating `auth.jwt_secret` makes stored webhook secrets unreadable (deliveries get 401): rotate them afterwards.
5. Slack approvals need `notification.web_ui_url` and cover only `notification.approval_tenants` (default: the default tenant).
6. List a private self-hosted GitLab in `pm.allowed_private_hosts`. GitLab PM requests no longer use `HTTP(S)_PROXY`.

The Core applies migration 113 (`webhook_endpoints`, `webhook_deliveries`) at startup.

### Distributed Tracing (OpenTelemetry)

CodeForge supports distributed tracing across Go Core, Python Workers, and NATS messaging using OpenTelemetry. Go injects W3C `traceparent` headers into NATS messages, Python extracts them on incoming messages, and every worker publish carries the current trace context (`TracingJetStreamContext`), so Python -> Go hops continue the trace. With OTEL enabled the worker also exports metrics through an OTLP MeterProvider; a metric exporter that cannot be set up is logged at startup and does not stop the worker.

#### Quick Start

```bash
# 1. Start Jaeger (OTLP collector + UI)
docker compose --profile dev up -d jaeger

# 2. Enable OTEL on Go Core (plaintext collector)
CODEFORGE_OTEL_ENABLED=true CODEFORGE_OTEL_INSECURE=true go run ./cmd/codeforge/

# 3. Enable OTEL on Python Worker
cd workers && CODEFORGE_OTEL_ENABLED=true CODEFORGE_OTEL_INSECURE=true poetry run python -m codeforge.consumer

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
| `CODEFORGE_OTEL_INSECURE` | `false` | Use plaintext gRPC; set `true` for a local collector such as Jaeger (Go Core and worker) |
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

Benchmark datasets are YAML files in `configs/benchmarks/` (configurable via `benchmark.datasets_dir` in `codeforge.yaml`). A run's `dataset` is a name (`.yaml` is added; `.yml` names resolve too) or a path inside that directory, stored relative; the Core and the worker read datasets only below it, through symlink-safe helpers (KI-107). See `configs/benchmarks/README.md` for the YAML schema.

Available metrics/evaluators: `llm_judge`, `functional_test`, `sparc`, `trajectory_verifier`, `correctness`, `faithfulness`, `relevance`, `coherence`, `fluency`, `tool_correctness`, `answer_relevancy`, `contextual_precision` (other names are rejected with 400). `trajectory_verifier` currently always scores 0.0 because the worker lacks `litellm` ([KI-37](todo.md#known-issues)).

#### Configuration

| YAML Key | ENV Variable | Default | Description |
|---|---|---|---|
| `benchmark.datasets_dir` | `CODEFORGE_BENCHMARK_DATASETS_DIR` (Python worker only) | `configs/benchmarks` | Directory with benchmark dataset YAML files. The Go Core (dataset listing and path resolution) reads only the YAML key. Datasets must be names or paths inside it (KI-107): an absolute path outside it is 400, a symlink out of it is neither listed nor read; the worker resolves a relative value against its working directory, the parent, then `CODEFORGE_WORKSPACE` |
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

The A2A (Agent-to-Agent) protocol enables CodeForge to communicate with external AI agents. When enabled, CodeForge exposes an AgentCard at `/.well-known/agent-card.json` and can delegate tasks to remote A2A agents. `/a2a` and the AgentCard are outside the JWT middleware and authenticate with A2A API keys (`Authorization: Bearer <key>`, see `a2a.api_keys`); a key maps to a tenant, the AgentCard is open without a key only with `a2a.allow_open`. The management endpoints under `/api/v1/a2a/*` use the normal CodeForge login.

#### Configuration

| YAML Key | ENV Variable | Default | Description |
|---|---|---|---|
| `a2a.enabled` | `CODEFORGE_A2A_ENABLED` | `false` | Enable A2A endpoints |
| `a2a.base_url` | `CODEFORGE_A2A_BASE_URL` | `http://localhost:<CODEFORGE_PORT>` | Public URL for AgentCard |
| `a2a.api_keys` | `CODEFORGE_A2A_API_KEYS` | (empty) | Comma-separated API keys for inbound auth, each `<key>` (default tenant) or `<tenant-uuid>:<key>`; no keys = every `/a2a` request gets 401 |
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

Each entry reads its provider key or base URL from the environment (`api_key: "os.environ/OPENAI_API_KEY"`, `api_base: "os.environ/OLLAMA_API_BASE"`); keys are never written into the file. A model the router or a mode picks works only when its provider entry exists and its variable holds a valid key, so check both when adding or switching a model. LiteLLM itself authenticates callers with `LITELLM_MASTER_KEY` (always an environment variable or `LITELLM_MASTER_KEY_FILE`, never hardcoded; the development LiteLLM container and the worker default to `sk-codeforge-dev`, see [Environment Variables](#environment-variables)).

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
