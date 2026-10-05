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

- Linter: [golangci-lint](https://golangci-lint.run/) v2 (aggregator; CI and devcontainer pin v2.11.4, the first line that knows every gosec rule excluded in `.golangci.yml`)
- Configuration: `.golangci.yml` (v2 format)
- Active linters: errcheck, govet, staticcheck, unused, ineffassign, gocritic, misspell, unconvert, unparam, gosec (security), bodyclose (HTTP response body), noctx (context-less HTTP), errorlint (error wrapping), revive (18 curated rules), fatcontext (loop context leak), dupword (comment typos), durationcheck (duration bugs)
- Formatter: gofmt + goimports

#### TypeScript

- Linter: ESLint 9 (flat config) with typescript-eslint strict + stylistic configs, `eslint-plugin-solid` and `eslint-plugin-jsx-a11y`
- Import sorting: eslint-plugin-simple-import-sort
- Formatter: Prettier
- Configuration: `frontend/eslint.config.js` (flat config format)

#### Benchmark grader

- The autonomous goal benchmark's grader (`testdata/autonomous-goal/mdlinkcheck/grade.py`) uses pytest, pytest-cov, ruff, mypy, radon and bandit from a separate virtual environment; they are not project dependencies ([autonomous-goal-benchmark.md](testing/autonomous-goal-benchmark.md#running-the-grader)).

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

#### Production images (S9-B, 2026-10-05)

- Core image: git and `subversion` (Alpine), the SVN adapter only.
- Worker image: Aider 0.86.2 (Apache-2.0, own venv, hash-locked in `workers/aider-requirements.txt`), Claude Code 2.1.289 (Anthropic, proprietary license), OpenCode 1.18.34 (MIT), Goose 1.29.0 (Apache-2.0, the last versioned image), `libgomp1`; pinned and checksummed per architecture; started only through `tool_process` as the tenant's tool UID. `uv` is used only to regenerate the aider lock.
- GitHub: the REST API through `internal/adapter/githubapi` (stdlib); no `gh` CLI.

#### Devcontainer

- Base image: `mcr.microsoft.com/devcontainers/base:bookworm` (Debian 12)
- Features: Go, Python, Node.js, Docker-outside-of-Docker, Git
- Setup: `.devcontainer/setup.sh` (automatic via postCreateCommand)

#### Docker Compose (Dev Services)

- postgres (Port 5432) — PostgreSQL 18, shared instance (CodeForge + LiteLLM); data volume at `/var/lib/postgresql` (PG 18 layout)
- nats (Port 4222/8222) — NATS JetStream message queue
- litellm (Port 4000) — LLM Routing and Multi-Provider Gateway
- docs-mcp (Port 6280) — Documentation indexing for LLM context
- playwright-mcp (Port 8001) — Browser automation and web scraping (profile-gated: `profiles: [dev]`)
- jaeger (Port 16686) — Distributed tracing UI (profile-gated: `profiles: [dev]`)

#### Docker Production

- `Dockerfile` — Go Core multi-stage build (golang:1.25-alpine to alpine:3.21)
- `Dockerfile.worker` — Python Workers (python:3.12-slim, poetry; the container starts as root with only `SETUID`/`SETGID`/`KILL` and its entrypoint runs the worker as uid 10001; agent tool processes run as their tenant's tool UID (20000-29999) through `setpriv` from util-linux, already part of the base image, and the launch helper `codeforge/tool_exec.py` (stdlib only, Landlock through `ctypes`); ADR-017, ADR-018. KI-96 adds the Debian `acl` package (`setfacl`/`getfacl` for operators and `scripts/check-host.sh`; the worker itself sets ACLs through xattrs), precompiles the stdlib (`compileall`, about 24 ms per tool launch instead of 80 ms on the read-only root), and installs pytest and ruff into the system interpreter for tool processes (`workers/tool-requirements.txt`))
- `Dockerfile.frontend` — Frontend (node:22-alpine build to nginxinc/nginx-unprivileged:1.27-alpine serve)
- `docker-compose.prod.yml` — 6 services (core, worker, frontend, postgres, nats, litellm); Docker secret files, PostgreSQL TLS, read-only core with `core_data`/`workspaces` volumes
- `.github/workflows/docker-build.yml` — CI with 3 parallel image builds to ghcr.io; the Grype scan scans the pushed images by digest

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
- Worker Pool (`golang.org/x/sync/semaphore`) — bounded concurrency for git operations
- Git Operations (`os/exec` wrapper around `git` CLI) — zero deps, 100% feature coverage, native performance; every call in an agent-writable workspace goes through the hardened `internal/git` package (sanitised environment, config allowlist, nested repositories refused)
- YAML (`gopkg.in/yaml.v3`) — config and policy loaders
- Linux syscalls (`golang.org/x/sys/unix`, already in the module graph, a direct dependency since KI-96) — POSIX ACL xattrs of the tenant directories (`internal/workspaceacl`, Linux only)
- UUIDs (`github.com/google/uuid`)
- Password hashing (`golang.org/x/crypto/bcrypt`) and admin CLI TTY input (`golang.org/x/term`)
- gRPC (`google.golang.org/grpc` v1.83.1) — transport for the OTLP exporters
- Indirect modules with security fixes: `golang.org/x/net` v0.55.0, `golang.org/x/text` v0.39.0
- Spec Providers: OpenSpec (`adapter/openspec/`), Markdown (`adapter/markdownspec/`), Spec Kit (`adapter/speckit/`), Autospec (`adapter/autospec/`) — self-registering via `init()`
- PM Providers: GitHub Issues (`adapter/githubpm/`, `gh` CLI), GitLab (`adapter/gitlab/`), Plane (`adapter/plane/`), Gitea/Forgejo/Codeberg (`adapter/gitea/`) — self-registering via `init()`

#### Python Workers

- httpx ^0.28 — async HTTP client for LiteLLM Proxy calls (litellm is a runtime dependency of the LiteLLM Docker sidecar, not installed in the worker venv; the `trajectory_verifier` / `logprob_verifier` evaluators call the proxy through the worker's HTTP client, KI-37)
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
- deepeval ^3.0 — LLM-as-judge evaluation framework (GEval, faithfulness, relevancy metrics; telemetry and Confident AI uploads forced off by the worker)
- scikit-learn ^1.6 — TF-IDF vectorization and cosine similarity for collaboration metrics
- opentelemetry-api + opentelemetry-sdk + opentelemetry-exporter-otlp-proto-grpc — always installed; tracing is enabled at runtime via `otel.enabled` / `CODEFORGE_OTEL_ENABLED` (agent execution tracing, tool selection and goal decomposition metrics; metrics are not exported yet, KI-36)
- Optional extras: `claude-code-sdk` (extra `claudecode`, Claude Code executor), `datasets` (extra `hf`, HuggingFace benchmark datasets)
- Dev group: pytest 9.1, pytest-asyncio ^1, pre-commit ^4, ruff 0.15.1
- Worker image: pytest and ruff (the poetry.lock versions, 9.1.1 and 0.15.1) are also installed into the system interpreter for agent tool processes (`workers/tool-requirements.txt`, `pip --require-hashes`): the default gate commands of Python projects and the auto-agent's workspace test. No new dependency.
- Transitive versions with security fixes (`poetry.lock`): cryptography 50, pyjwt 2.15, starlette 1.7, python-multipart 0.0.32, requests 2.34, urllib3 2.8, aiohttp 3.14

#### MCP & A2A SDKs

- mcp-go v0.45 (Go MCP SDK) -- MCP server implementation in Go Core
- a2a-go v0.3.8 (Go A2A SDK) -- Agent-to-Agent protocol client/server
- mcp 1.30 (Python MCP SDK) -- MCP client in Python Workers

#### Protocols & Standards

- MCP (Model Context Protocol) — Agent-to-Tool communication, JSON-RPC 2.0, Anthropic (Go Core: MCP server + client registry; Python Workers: MCP client for agent tool access)
- LSP (Language Server Protocol) — code intelligence for agents, Microsoft (Go Core: LSP server lifecycle management per project language)
- OpenTelemetry GenAI — LLM/agent observability, traces + metrics, CNCF (LiteLLM: native OTEL export; Go: `go.opentelemetry.io/otel` v1.45.0 + SDK + OTLP gRPC exporters + `otelhttp` middleware; Python: `opentelemetry-api` + `opentelemetry-sdk` + OTLP gRPC exporter; export gaps: KI-36)
- A2A (Agent-to-Agent Protocol v0.3.0) — full implementation with AgentCard, JSON-RPC task lifecycle, inbound/outbound federation, trust annotations, push notifications (Phase 27); inbound calls authenticate with per-tenant A2A API keys and their prompts pass the quarantine (KI-15)
- AG-UI (Agent-User Interaction Protocol) — 12 event types (8 core + 4 CodeForge extensions: `permission_request`, `goal_proposal`, `action_suggestion`, `roadmap_proposal`) emitted via WebSocket, Go + Python + Frontend integration (Phase 17+)

#### Agent Backend Integration (Phase 9+)

- Goose (Rust, MCP-native, subprocess integration)
- OpenCode (Go, Client/Server, LSP-aware)
- Plandex (Go, Planning-First, Diff Sandbox)

#### Infrastructure Services

- NATS JetStream (Port 4222/8222) — message queue between Go Core and Python Workers (Image: `nats:2-alpine` in development without authentication, `nats:2.15-alpine` pinned in production with one authenticated user per service from `configs/nats/nats-server.conf`, [ADR-017](architecture/adr/017-tool-isolation-and-nats-authentication.md); subject-based routing, JetStream persistence, built-in KV store; ADR: [001-nats-jetstream-message-queue.md](architecture/adr/001-nats-jetstream-message-queue.md))
- PostgreSQL 18 (Port 5432) — primary database for App + LiteLLM (Image: `postgres:18-alpine`, shared instance, both CodeForge and LiteLLM use `public` schema (LiteLLM tables prefixed with `LiteLLM_`); Go Driver: pgx v5, Migrations: goose, Python Driver: psycopg3; ADR: [002-postgresql-database.md](architecture/adr/002-postgresql-database.md))
- LiteLLM Proxy (Docker Sidecar, Port 4000) — central LLM gateway (Dev image: `docker.litellm.ai/berriai/litellm:main-stable`; Prod image: `ghcr.io/berriai/litellm:v1.103.1` (pinned; v1.63.2 no longer exists); 127+ providers, 6 routing strategies, budget management; Config: hand-maintained `litellm/config.yaml` with provider-level wildcard entries, mounted into the container; Go Core adds/removes models at runtime via the LiteLLM admin API; Dependencies: PostgreSQL shared instance, Redis optional for multi-instance only)
- Traefik v3.6 (ports 80/443, only with the blue-green overlay `docker-compose.blue-green.yml`) — TLS with Let's Encrypt (HTTP challenge) and routing to the running color of core and frontend; configured by command flags, v3.6 or later because older Docker providers are refused by Docker Engine 29

#### TypeScript Frontend

- SolidJS — reactive UI framework
- `@solidjs/router` — official SolidJS router (nested routes, lazy loading)
- Tailwind CSS v4 (via `@tailwindcss/vite`) — direct utility classes, no component library
- Custom WebSocket wrapper (~280 LOC, `frontend/src/api/websocket.ts`) — auto-reconnect with a fresh token after a fixed 1 s delay, no heartbeat (no external WebSocket library)
- Native `fetch` API — thin wrapper (~200 LOC), no axios/ky
- SolidJS built-in state management (signals, stores, context) — no external state library
- Icons: Unicode characters and inline SVG components (`frontend/src/ui/icons/`), no icon library; `vscode-icons-js` only for file/folder icons (VS Code icon set, tree-shakeable)
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
