# CodeForge — Tech Stack

### Languages & Versions

| Language   | Version | Area of Use           |
|------------|---------|----------------------|
| Go         | 1.24    | Core Service         |
| Python     | 3.12    | AI Workers           |
| TypeScript | 5.x     | Frontend             |
| Node.js    | 22 LTS  | Frontend Build/Dev   |

### Linting & Formatting

#### Python

- Linter/Formatter: [Ruff](https://docs.astral.sh/ruff/) v0.15.1 (replaces flake8, isort, black, bandit)
- Configuration: `pyproject.toml` under `[tool.ruff]`
- Rules: pyflakes (F), pycodestyle (E/W), isort (I), pep8-naming (N), pyupgrade (UP), bugbear (B), simplify (SIM), bandit security (S), unnecessary comprehensions (C4), mccabe complexity (C90, threshold 12), performance (PERF), anti-patterns (PIE), return issues (RET), modernization (FURB), logging (LOG), print detection (T20), pytest style (PT)
- Line length: 120

#### Go

- Linter: [golangci-lint](https://golangci-lint.run/) v2 (aggregator)
- Configuration: `.golangci.yml` (v2 format)
- Active linters: errcheck, govet, staticcheck, unused, ineffassign, gocritic, misspell, unconvert, unparam, gosec (security), bodyclose (HTTP response body), noctx (context-less HTTP), errorlint (error wrapping), revive (18 curated rules), fatcontext (loop context leak), dupword (comment typos), durationcheck (duration bugs)
- Formatter: gofmt + goimports

#### TypeScript

- Linter: ESLint 9 (flat config) with typescript-eslint strict + stylistic configs
- Import sorting: eslint-plugin-simple-import-sort
- Formatter: Prettier
- Configuration: `frontend/eslint.config.js` (flat config format)

#### Pre-commit Hooks

- Configuration: `.pre-commit-config.yaml`
- Invocation: `pre-commit run --all-files`
- Runs automatically on every `git commit`

### Package Management

| Language   | Tool       | Lockfile         | Config           |
|------------|------------|------------------|------------------|
| Python     | Poetry     | poetry.lock      | pyproject.toml   |
| Go         | Go Modules | go.sum           | go.mod           |
| TypeScript | npm        | package-lock.json| package.json     |

### Infrastructure

#### Devcontainer

- Base image: `mcr.microsoft.com/devcontainers/base:bookworm` (Debian 12)
- Features: Go, Python, Node.js, Docker-in-Docker, Git
- Setup: `.devcontainer/setup.sh` (automatic via postCreateCommand)

#### Docker Compose (Dev Services)

- postgres (Port 5432) — PostgreSQL 17, shared instance (CodeForge + LiteLLM)
- nats (Port 4222/8222) — NATS JetStream message queue
- litellm (Port 4000) — LLM Routing and Multi-Provider Gateway
- docs-mcp-server (Port 6280) — Documentation indexing for LLM context
- playwright-mcp (Port 8001) — Browser automation and web scraping

#### Docker Production

- `Dockerfile` — Go Core multi-stage build (golang:1.24-alpine to alpine:3.21)
- `Dockerfile.worker` — Python Workers (python:3.12-slim, poetry, non-root)
- `Dockerfile.frontend` — Frontend (node:22-alpine build to nginx:alpine serve)
- `docker-compose.prod.yml` — 6 services (core, worker, frontend, postgres, nats, litellm)
- `.github/workflows/docker-build.yml` — CI with 3 parallel image builds to ghcr.io

#### MCP Server

- Configuration: `.mcp.json`
- Automatically loaded via `enableAllProjectMcpServers: true`

### VS Code Extensions (in Devcontainer)

| Extension | Purpose |
|---|---|
| anthropic.claude-code | Claude Code CLI Integration |
| golang.go | Go Language Support |
| ms-python.python | Python Language Support |
| ms-python.vscode-pylance | Python Type Checking |
| dbaeumer.vscode-eslint | ESLint Integration |
| esbenp.prettier-vscode | Prettier Integration |
| bradlc.vscode-tailwindcss | Tailwind CSS IntelliSense |

### Installed Dependencies

#### Go Core Service

- HTTP Router (`chi` v5) — zero deps, 100% `net/http` compatible, route groups + middleware chaining
- WebSocket (`coder/websocket` v1.8+) — zero deps, context-native, concurrent-write-safe
- PostgreSQL Driver (`pgx` v5 + `pgxpool`) — primary database
- Database Migrations (`goose`) — SQL-based schema migrations
- NATS Client (`nats.go` + `nats.go/jetstream`) — message queue to Python Workers
- Tiered Cache (`dgraph-io/ristretto` v2) — in-process L1 cache
- Worker Pool (`golang.org/x/sync/semaphore`) — bounded concurrency for git operations
- Git Operations (`os/exec` wrapper around `git` CLI) — zero deps, 100% feature coverage, native performance
- Spec Providers: OpenSpec (`adapter/openspec/`), Markdown (`adapter/markdownspec/`) — self-registering via `init()`
- PM Providers: GitHub Issues (`adapter/githubpm/`) — `gh` CLI integration, self-registering via `init()`

#### Python Workers

- LiteLLM — client library for OpenAI-compatible API against LiteLLM Proxy
- NATS Client (`nats-py`) — asyncio-native, message queue to Go Core
- Jinja2 — prompt templates
- tree-sitter ^0.24 — code parsing into ASTs for repo map generation and code chunking
- tree-sitter-language-pack ^0.13 — pre-built parsers for 16+ languages
- networkx ^3.4 — graph algorithms (PageRank for file ranking in repo maps)
- bm25s ^0.2 — fast BM25 keyword search (500x faster than rank_bm25, numpy+scipy only)
- numpy ^2.0 — numerical computing for embedding vectors and cosine similarity
- psycopg[binary] ^3.2 — PostgreSQL driver for graph storage (sync+async)
- httpx ^0.28 — async HTTP client for LiteLLM proxy calls

#### Planned Dependencies (not yet installed)

- SVN Operations (`os/exec` wrapper around `svn` CLI) — Phase 9+
- KeyBERT (Keyword Extraction for Retrieval) — Phase 9+
- LangGraph (Agent Orchestration) — Phase 9+
- OpenTelemetry SDKs (Go + Python) — Phase 9+

#### Protocols & Standards

- MCP (Model Context Protocol) — Agent-to-Tool communication, JSON-RPC 2.0, Anthropic (Go Core: MCP server + client registry; Python Workers: MCP client for agent tool access)
- LSP (Language Server Protocol) — code intelligence for agents, Microsoft (Go Core: LSP server lifecycle management per project language)
- OpenTelemetry GenAI — LLM/agent observability, traces + metrics, CNCF (LiteLLM: native OTEL export; Go: `go.opentelemetry.io/otel` v1.40 + SDK + OTLP gRPC exporters + `otelhttp` middleware; Python: `opentelemetry-api` + `opentelemetry-sdk`, planned)
- A2A (Agent-to-Agent Protocol) — stub with Agent Cards + task create/get endpoints
- AG-UI (Agent-User Interaction Protocol) — event types defined, dual-emit planned

#### Agent Backend Integration (Phase 9+)

- Goose (Rust, MCP-native, subprocess integration)
- OpenCode (Go, Client/Server, LSP-aware)
- Plandex (Go, Planning-First, Diff Sandbox)

#### Infrastructure Services

- NATS JetStream (Port 4222/8222) — message queue between Go Core and Python Workers (Image: `nats:2-alpine`, subject-based routing, JetStream persistence, built-in KV store; ADR: [001-nats-jetstream-message-queue.md](architecture/adr/001-nats-jetstream-message-queue.md))
- PostgreSQL 17 (Port 5432) — primary database for App + LiteLLM (Image: `postgres:17-alpine`, shared instance with CodeForge on `public` schema and LiteLLM on `litellm` schema; Go Driver: pgx v5, Migrations: goose, Python Driver: psycopg3; ADR: [002-postgresql-database.md](architecture/adr/002-postgresql-database.md))
- LiteLLM Proxy (Docker Sidecar, Port 4000) — central LLM gateway (Image: `docker.litellm.ai/berriai/litellm:main-stable`, 127+ providers, 6 routing strategies, budget management; Config: `litellm_config.yaml` generated by Go Core; Dependencies: PostgreSQL shared instance with `?schema=litellm`, Redis optional for multi-instance only)

#### TypeScript Frontend

- SolidJS — reactive UI framework
- `@solidjs/router` — official SolidJS router (nested routes, lazy loading)
- Tailwind CSS — direct utility classes, no component library
- `@solid-primitives/websocket` (728 bytes) — WebSocket with auto-reconnect + heartbeat
- Native `fetch` API — thin wrapper (~30-50 LOC), no axios/ky
- SolidJS built-in state management (signals, stores, context) — no external state library
- `lucide-solid` — icons (tree-shakeable, direct imports)
- `@axe-core/playwright` (devDependency) — automated WCAG accessibility auditing in E2E tests
