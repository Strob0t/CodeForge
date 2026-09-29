# CodeForge — Tech Stack

> `KI-n` references point to the [Known Issues](todo.md#known-issues) in todo.md.

### Languages & Versions

| Language   | Version | Area of Use           |
|------------|---------|----------------------|
| Go         | 1.25    | Core Service         |
| Python     | 3.12    | AI Workers           |
| TypeScript | 5.x     | Frontend             |
| Node.js    | 22 LTS  | Frontend Build/Dev   |

### Linting & Formatting

#### Python

- Linter/Formatter: [Ruff](https://docs.astral.sh/ruff/) v0.15.1 (replaces flake8, isort, black, bandit); pinned as a Poetry dev dependency, kept in sync with the `ruff-pre-commit` rev
- Configuration: `pyproject.toml` under `[tool.ruff]`
- Rules: pyflakes (F), pycodestyle (E/W), isort (I), pep8-naming (N), pyupgrade (UP), bugbear (B), builtins (A), simplify (SIM), type-checking (TCH), ruff-specific (RUF), bandit security (S), unnecessary comprehensions (C4), mccabe complexity (C90, threshold 12), performance (PERF), anti-patterns (PIE), return issues (RET), modernization (FURB), logging (LOG), print detection (T20), pytest style (PT)
- Line length: 120

#### Go

- Linter: [golangci-lint](https://golangci-lint.run/) v2 (aggregator; CI pins v2.5.0)
- Configuration: `.golangci.yml` (v2 format)
- Active linters: errcheck, govet, staticcheck, unused, ineffassign, gocritic, misspell, unconvert, unparam, gosec (security), bodyclose (HTTP response body), noctx (context-less HTTP), errorlint (error wrapping), revive (18 curated rules), fatcontext (loop context leak), dupword (comment typos), durationcheck (duration bugs)
- Formatter: gofmt + goimports

#### TypeScript

- Linter: ESLint 9 (flat config) with typescript-eslint strict + stylistic configs, `eslint-plugin-solid` and `eslint-plugin-jsx-a11y`
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
- Features: Go, Python, Node.js, Docker-outside-of-Docker, Git
- Setup: `.devcontainer/setup.sh` (automatic via postCreateCommand)

#### Docker Compose (Dev Services)

- postgres (Port 5432) — PostgreSQL 18, shared instance (CodeForge + LiteLLM); the volume mount path does not fit the PG 18 image (KI-43)
- nats (Port 4222/8222) — NATS JetStream message queue
- litellm (Port 4000) — LLM Routing and Multi-Provider Gateway
- docs-mcp (Port 6280) — Documentation indexing for LLM context
- playwright-mcp (Port 8001) — Browser automation and web scraping (profile-gated: `profiles: [dev]`)
- jaeger (Port 16686) — Distributed tracing UI (profile-gated: `profiles: [dev]`)

#### Docker Production

- `Dockerfile` — Go Core multi-stage build (golang:1.25-alpine to alpine:3.21)
- `Dockerfile.worker` — Python Workers (python:3.12-slim, poetry, non-root)
- `Dockerfile.frontend` — Frontend (node:22-alpine build to nginxinc/nginx-unprivileged:1.27-alpine serve)
- `docker-compose.prod.yml` — 6 services (core, worker, frontend, postgres, nats, litellm); does not start as shipped, see KI-43, KI-44, KI-45, KI-46
- `.github/workflows/docker-build.yml` — CI with 3 parallel image builds to ghcr.io (the image scan job pulls a tag that is never pushed, KI-48)

#### MCP Server

- Configuration: local `.mcp.json` (gitignored, not checked in; each developer creates it)
- Enable the project servers via `enableAllProjectMcpServers: true` in your local Claude Code settings

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

- HTTP Router (`chi` v5.3.0) — zero deps, 100% `net/http` compatible, route groups + middleware chaining
- WebSocket (`coder/websocket` v1.8+) — zero deps, context-native, concurrent-write-safe
- PostgreSQL Driver (`pgx` v5.9.2 + `pgxpool`) — primary database
- Database Migrations (`goose`) — SQL-based schema migrations
- NATS Client (`nats.go` + `nats.go/jetstream`) — message queue to Python Workers
- Tiered Cache (`dgraph-io/ristretto` v2) — in-process L1 cache (L1 + NATS KV L2 are constructed at startup but not yet injected into any service)
- Worker Pool (`golang.org/x/sync/semaphore`) — bounded concurrency for git operations
- Git Operations (`os/exec` wrapper around `git` CLI) — zero deps, 100% feature coverage, native performance
- YAML (`gopkg.in/yaml.v3`) — config and policy loaders
- UUIDs (`github.com/google/uuid`)
- Password hashing (`golang.org/x/crypto/bcrypt`) and admin CLI TTY input (`golang.org/x/term`)
- gRPC (`google.golang.org/grpc` v1.83.1) — transport for the OTLP exporters
- Indirect modules with security fixes: `golang.org/x/net` v0.55.0, `golang.org/x/text` v0.39.0
- Spec Providers: OpenSpec (`adapter/openspec/`), Markdown (`adapter/markdownspec/`), Spec Kit (`adapter/speckit/`), Autospec (`adapter/autospec/`) — self-registering via `init()`
- PM Providers: GitHub Issues (`adapter/githubpm/`, `gh` CLI), GitLab (`adapter/gitlab/`), Plane (`adapter/plane/`), Gitea/Forgejo/Codeberg (`adapter/gitea/`) — self-registering via `init()`

#### Python Workers

- httpx ^0.28 — async HTTP client for LiteLLM Proxy calls (litellm is a runtime dependency of the LiteLLM Docker sidecar, not installed in the worker venv; the `trajectory_verifier` / `logprob_verifier` evaluators still import it, KI-37)
- NATS Client (`nats-py`) — asyncio-native, message queue to Go Core
- pydantic ^2.10 — NATS message schemas
- structlog ^25.0 — JSON logging
- pyyaml ^6.0 — YAML config, skills and benchmark datasets
- psutil ^6.0 — process memory (RSS) monitoring in the agent loop
- tree-sitter ^0.25 — code parsing into ASTs for repo map generation and code chunking
- tree-sitter-language-pack ^0.13 — pre-built parsers for 16+ languages
- bm25s ^0.3 — fast BM25 keyword search (500x faster than rank_bm25, numpy+scipy only)
- numpy ^2.0 — numerical computing for embedding vectors and cosine similarity
- psycopg ^3.2 — PostgreSQL driver for graph storage (sync+async)
- psycopg-binary ^3.3.3 — compiled C extension for psycopg (faster I/O)
- deepeval ^3.0 — LLM-as-judge evaluation framework (GEval, faithfulness, relevancy metrics; telemetry not opted out, KI-54)
- scikit-learn ^1.6 — TF-IDF vectorization and cosine similarity for collaboration metrics
- opentelemetry-api + opentelemetry-sdk + opentelemetry-exporter-otlp-proto-grpc — always installed; tracing is enabled at runtime via `otel.enabled` / `CODEFORGE_OTEL_ENABLED` (agent execution tracing, tool selection and goal decomposition metrics; metrics are not exported yet, KI-36)
- Optional extras: `claude-code-sdk` (extra `claudecode`, Claude Code executor), `datasets` (extra `hf`, HuggingFace benchmark datasets)
- Dev group: pytest 9.1, pytest-asyncio ^1, pre-commit ^4, ruff 0.15.1
- Transitive versions with security fixes (`poetry.lock`): cryptography 50, pyjwt 2.15, starlette 1.7, python-multipart 0.0.32, requests 2.34, urllib3 2.8, aiohttp 3.14

#### MCP & A2A SDKs

- mcp-go v0.45 (Go MCP SDK) -- MCP server implementation in Go Core
- a2a-go v0.3.8 (Go A2A SDK) -- Agent-to-Agent protocol client/server
- mcp 1.30 (Python MCP SDK) -- MCP client in Python Workers

#### Protocols & Standards

- MCP (Model Context Protocol) — Agent-to-Tool communication, JSON-RPC 2.0, Anthropic (Go Core: MCP server + client registry; Python Workers: MCP client for agent tool access)
- LSP (Language Server Protocol) — code intelligence for agents, Microsoft (Go Core: LSP server lifecycle management per project language)
- OpenTelemetry GenAI — LLM/agent observability, traces + metrics, CNCF (LiteLLM: native OTEL export; Go: `go.opentelemetry.io/otel` v1.44.0 + SDK + OTLP gRPC exporters + `otelhttp` middleware; Python: `opentelemetry-api` + `opentelemetry-sdk` + OTLP gRPC exporter; export gaps: KI-36)
- A2A (Agent-to-Agent Protocol v0.3.0) — full implementation with AgentCard, JSON-RPC task lifecycle, inbound/outbound federation, trust annotations, push notifications (Phase 27); inbound auth and quarantine gaps: KI-15
- AG-UI (Agent-User Interaction Protocol) — 12 event types (8 core + 4 CodeForge extensions: `permission_request`, `goal_proposal`, `action_suggestion`, `roadmap_proposal`) emitted via WebSocket, Go + Python + Frontend integration (Phase 17+)

#### Agent Backend Integration (Phase 9+)

- Goose (Rust, MCP-native, subprocess integration)
- OpenCode (Go, Client/Server, LSP-aware)
- Plandex (Go, Planning-First, Diff Sandbox)

#### Infrastructure Services

- NATS JetStream (Port 4222/8222) — message queue between Go Core and Python Workers (Image: `nats:2-alpine`, subject-based routing, JetStream persistence, built-in KV store; ADR: [001-nats-jetstream-message-queue.md](architecture/adr/001-nats-jetstream-message-queue.md))
- PostgreSQL 18 (Port 5432) — primary database for App + LiteLLM (Image: `postgres:18-alpine`, shared instance, both CodeForge and LiteLLM use `public` schema (LiteLLM tables prefixed with `LiteLLM_`); Go Driver: pgx v5, Migrations: goose, Python Driver: psycopg3; ADR: [002-postgresql-database.md](architecture/adr/002-postgresql-database.md))
- LiteLLM Proxy (Docker Sidecar, Port 4000) — central LLM gateway (Dev image: `docker.litellm.ai/berriai/litellm:main-stable`; Prod image: `ghcr.io/berriai/litellm:v1.63.2` (pinned); 127+ providers, 6 routing strategies, budget management; Config: hand-maintained `litellm/config.yaml` with provider-level wildcard entries, mounted into the container; Go Core adds/removes models at runtime via the LiteLLM admin API; Dependencies: PostgreSQL shared instance, Redis optional for multi-instance only)

#### TypeScript Frontend

- SolidJS — reactive UI framework
- `@solidjs/router` — official SolidJS router (nested routes, lazy loading)
- Tailwind CSS v4 (via `@tailwindcss/vite`) — direct utility classes, no component library
- Custom WebSocket wrapper (~280 LOC, `frontend/src/api/websocket.ts`) — auto-reconnect with a fresh token after a fixed 1 s delay, no heartbeat (no external WebSocket library)
- Native `fetch` API — thin wrapper (~200 LOC), no axios/ky
- SolidJS built-in state management (signals, stores, context) — no external state library
- `vscode-icons-js` — file/folder icons (VS Code icon set, tree-shakeable)
- `@unovis/ts` + `@unovis/solid` ^1.7.1 — dashboard charts
- `solid-monaco` — code editor
- `@tanstack/solid-virtual` — virtualized lists (benchmark live feed)
- Build (devDependencies): Vite 7 + `vite-plugin-solid`
- Tests (devDependencies): Vitest 4 + jsdom + `@solidjs/testing-library` + `@testing-library/jest-dom` (unit/component); Playwright (`@playwright/test`) for E2E
- `@axe-core/playwright` (devDependency) — automated WCAG accessibility auditing in E2E tests

#### Typography

- **Outfit** (display headings) — self-hosted woff2 in `frontend/public/fonts/`, 3 weights (400/500/700)
- **Source Sans 3** (body text) — self-hosted variable woff2 in `frontend/public/fonts/`, weights 200-900 (latin + latin-ext)
- No npm font packages or external CDN — zero runtime dependency for fonts
