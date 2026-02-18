# CodeForge — Tech Stack

## Languages & Versions

| Language   | Version | Area of Use           |
|------------|---------|----------------------|
| Go         | 1.24    | Core Service         |
| Python     | 3.12    | AI Workers           |
| TypeScript | 5.x     | Frontend             |
| Node.js    | 22 LTS  | Frontend Build/Dev   |

## Linting & Formatting

### Python
- **Linter/Formatter:** [Ruff](https://docs.astral.sh/ruff/) v0.15.1 (replaces flake8, isort, black, bandit)
- **Configuration:** `pyproject.toml` under `[tool.ruff]`
- **Rules:** pyflakes (F), pycodestyle (E/W), isort (I), pep8-naming (N), pyupgrade (UP), bugbear (B), simplify (SIM), bandit security (S), unnecessary comprehensions (C4), mccabe complexity (C90, threshold 12), performance (PERF), anti-patterns (PIE), return issues (RET), modernization (FURB), logging (LOG), print detection (T20), pytest style (PT)
- **Line Length:** 120

### Go
- **Linter:** [golangci-lint](https://golangci-lint.run/) v2 (Aggregator)
- **Configuration:** `.golangci.yml` (v2 format)
- **Active Linters:** errcheck, govet, staticcheck, unused, ineffassign, gocritic, misspell, unconvert, unparam, gosec (security), bodyclose (HTTP response body), noctx (context-less HTTP), errorlint (error wrapping), revive (18 curated rules), fatcontext (loop context leak), dupword (comment typos), durationcheck (duration bugs)
- **Formatter:** gofmt + goimports

### TypeScript
- **Linter:** ESLint 9 (flat config) with typescript-eslint strict + stylistic configs
- **Import Sorting:** eslint-plugin-simple-import-sort
- **Formatter:** Prettier
- **Configuration:** `frontend/eslint.config.js` (flat config format)

### Pre-commit Hooks
- **Configuration:** `.pre-commit-config.yaml`
- **Invocation:** `pre-commit run --all-files`
- Runs automatically on every `git commit`

## Package Management

| Language   | Tool       | Lockfile         | Config           |
|------------|------------|------------------|------------------|
| Python     | Poetry     | poetry.lock      | pyproject.toml   |
| Go         | Go Modules | go.sum           | go.mod           |
| TypeScript | npm        | package-lock.json| package.json     |

## Infrastructure

### Devcontainer
- **Base Image:** `mcr.microsoft.com/devcontainers/base:bookworm` (Debian 12)
- **Features:** Go, Python, Node.js, Docker-in-Docker, Git
- **Setup:** `.devcontainer/setup.sh` (automatic via postCreateCommand)

### Docker Compose (Dev Services)
- **docs-mcp-server** (Port 6280) — Documentation indexing for LLM context
- **playwright-mcp** (Port 8001) — Browser automation / web scraping
- **litellm-proxy** (Port 4000) — LLM Routing & Multi-Provider Gateway *(planned, not yet in docker-compose.yml)*

### MCP Server
- Configuration: `.mcp.json`
- Automatically loaded via `enableAllProjectMcpServers: true`

## VS Code Extensions (in Devcontainer)

| Extension | Purpose |
|---|---|
| anthropic.claude-code | Claude Code CLI Integration |
| golang.go | Go Language Support |
| ms-python.python | Python Language Support |
| ms-python.vscode-pylance | Python Type Checking |
| dbaeumer.vscode-eslint | ESLint Integration |
| esbenp.prettier-vscode | Prettier Integration |
| bradlc.vscode-tailwindcss | Tailwind CSS IntelliSense |

## Planned Dependencies (not yet installed)

### Go Core Service
- HTTP Router (`chi` v5) — zero deps, 100% `net/http` compatible, route groups + middleware chaining
- WebSocket (`coder/websocket` v1.8+) — zero deps, context-native, concurrent-write-safe
- PostgreSQL Driver (`pgx` v5 + `pgxpool`) — primary database
- Database Migrations (`goose`) — SQL-based schema migrations
- NATS Client (`nats.go` + `nats.go/jetstream`) — message queue to Python Workers
- Git Operations (`os/exec` wrapper around `git` CLI) — zero deps, 100% feature coverage, native performance
- SVN Operations (`os/exec` wrapper around `svn` CLI)

### Python Workers
- LiteLLM (client library for OpenAI-compatible API against LiteLLM Proxy)
- LangGraph (Agent Orchestration)
- PostgreSQL Driver (`psycopg3`) — read access to task metadata, sync+async
- NATS Client (`nats-py`) — asyncio-native, message queue to Go Core
- Jinja2 (Prompt Templates)
- KeyBERT (Keyword Extraction for Retrieval)
- tree-sitter ^0.24 — Code parsing into ASTs for repo map generation and code chunking
- tree-sitter-language-pack ^0.13 — Pre-built parsers for 16+ languages
- networkx ^3.4 — Graph algorithms (PageRank for file ranking in repo maps)
- bm25s ^0.2 — Fast BM25 keyword search (500x faster than rank_bm25, numpy+scipy only)
- numpy ^2.0 — Numerical computing for embedding vectors and cosine similarity

### Protocols & Standards
- MCP (Model Context Protocol) — Agent ↔ Tool communication, JSON-RPC 2.0 (Anthropic)
  - Go Core: MCP server + client registry
  - Python Workers: MCP client for agent tool access
- LSP (Language Server Protocol) — Code intelligence for agents (Microsoft)
  - Go Core: LSP server lifecycle management per project language
- OpenTelemetry GenAI — LLM/agent observability, traces + metrics (CNCF)
  - LiteLLM: native OTEL export
  - Go: `go.opentelemetry.io/otel` SDK
  - Python: `opentelemetry-api` + `opentelemetry-sdk`
- A2A (Agent-to-Agent Protocol, Phase 2-3) — Peer-to-peer agent coordination (Linux Foundation)
- AG-UI (Agent-User Interaction Protocol, Phase 2-3) — Agent ↔ frontend streaming (CopilotKit)

### Agent Backend Integration (Priority 1)
- Goose (Rust, MCP-native, subprocess integration)
- OpenCode (Go, Client/Server, LSP-aware)
- Plandex (Go, Planning-First, Diff Sandbox)

### Infrastructure Services
- NATS JetStream (Port 4222/8222) — Message Queue between Go Core and Python Workers
  - Image: `nats:2-alpine`
  - Subject-based routing, JetStream persistence, built-in KV store
  - ADR: [docs/architecture/adr/001-nats-jetstream-message-queue.md](architecture/adr/001-nats-jetstream-message-queue.md)
- PostgreSQL 17 (Port 5432) — Primary Database for App + LiteLLM
  - Image: `postgres:17-alpine`
  - Shared instance: CodeForge (`public` schema) + LiteLLM (`litellm` schema)
  - Go Driver: pgx v5, Migrations: goose, Python Driver: psycopg3
  - ADR: [docs/architecture/adr/002-postgresql-database.md](architecture/adr/002-postgresql-database.md)
- LiteLLM Proxy (Docker Sidecar, Port 4000) — Central LLM Gateway
  - Image: `docker.litellm.ai/berriai/litellm:main-stable`
  - 127+ Providers, 6 Routing Strategies, Budget Management
  - Config: `litellm_config.yaml` (generated by Go Core)
  - Dependencies: PostgreSQL (shared instance, `?schema=litellm`), Redis optional (only for multi-instance)

### TypeScript Frontend
- SolidJS (reactive UI framework)
- `@solidjs/router` — official SolidJS router (nested routes, lazy loading)
- Tailwind CSS (direct utility classes, no component library)
- `@solid-primitives/websocket` (728 bytes) — WebSocket with auto-reconnect + heartbeat
- Native `fetch` API — thin wrapper (~30-50 LOC), no axios/ky
- SolidJS built-in state management (signals, stores, context) — no external state library
- `lucide-solid` — icons (optional, tree-shakeable, direct imports)
- WebSocket Client
- solid-router (SPA routing)
