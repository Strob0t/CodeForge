# AGENTS.md

Instructions for developers and AI assistants working on CodeForge. This file holds the **rules** that apply to every change; how things work lives in `docs/` (linked below). It is the single instruction file for every coding agent (the AGENTS.md convention); there is no `CLAUDE.md`. Claude Code reads `AGENTS.md` when no `CLAUDE.md` exists (v2.1.277 or newer). Project-specific memories and decisions are stored here.

All project documentation, code comments, commit messages and configs are written in **English only**.

**Keeping this file small:** one rule per line, mechanics and status go to `docs/` and are linked. Add a line only when breaking it would cause a defect that review and CI would not reliably catch. Target: under 3,000 words.

---

## 1. Workflow (IMPORTANT!)

### Branches
- `staging`: development branch; all work lands here (directly or through a pull request into `staging`). `main`: stable releases.
- Never commit to `main`. Never merge into `main` without an explicit user request.

### Before every commit

```bash
pre-commit run --all-files                                  # gofmt, goimports, go vet, golangci-lint, ruff, eslint, prettier, ...
go test -race ./...                                         # Go (CI: Go 1.25.14, also -tags=integration with PostgreSQL + NATS)
cd workers && poetry run pytest                             # Python
cd frontend && npm run lint && npm run format:check && npm run typecheck && npm test   # Frontend
```

- Fix all errors before committing (warnings can be acceptable depending on the check; do not add new ones).
- Small, atomic commits; one isolated subtask per commit, never batch unrelated changes.
- Conventional Commits: `<type>(<scope>): <subject>` (`feat`, `fix`, `docs`, `refactor`, `test`, `chore`, ...), English.
- **Docs ship with the code: every change is documented in the same commit** (table below). Read `docs/todo.md` before work; mark items `[x]` with the date on completion; add new tasks and Known Issues when discovered. Feature TODOs in `docs/features/*.md` are cross-referenced in `docs/todo.md`.
- Push after each successful change.
- Larger refactors: write a brief plan (problem, design, affected files, tests) in `docs/plans/` first.

| Change type | Update |
|---|---|
| Feature work | `docs/features/*.md`, `docs/todo.md` |
| Architecture decision | `docs/architecture.md`, `docs/architecture/adr/` (`_template.md`: Context -> Decision -> Consequences -> Alternatives), `AGENTS.md` |
| New dependency/tool | `docs/tech-stack.md`, `docs/dev-setup.md` |
| Milestone complete | `docs/project-status.md`, `docs/todo.md` |
| New dir/port/env var/config key | `docs/dev-setup.md` |
| Core pillar change | `AGENTS.md`, `docs/features/*.md` (one per pillar, sub-features as sections) |
| Design spec/plan | `docs/specs/` or `docs/plans/`, `docs/todo.md` |
| Test results / audit findings | `docs/testing/` / `docs/audits/`, `docs/todo.md` |
| Any code change | `docs/todo.md` |

### Versioning and release (merge to `main` only on explicit user request)
- Source of truth: the `VERSION` file (semver, currently **0.8.0**). Change it, then run `./scripts/sync-version.sh` (pyproject.toml, package.json, package-lock.json). Go reads it via `internal/version` (ldflags `version.Version`, `version.GitSHA`), Python via `workers/codeforge/__init__.py`, the frontend via Vite `__APP_VERSION__`, Docker via `ARG APP_VERSION` and OCI labels (`.github/workflows/docker-build.yml`).

---

## 2. Project overview

CodeForge is a containerized service for orchestrating AI coding agents with a web GUI. Four core pillars:
1. **Project Dashboard**: multiple repos (Git, GitHub, GitLab, SVN, local)
2. **Roadmap/Feature-Map**: visual management, OpenSpec-compatible, bidirectional sync to repo specs
3. **Multi-LLM-Provider**: OpenAI, Claude, local models (Ollama/LM Studio), routing via LiteLLM
4. **Agent Orchestration**: coordination of coding agents (Aider, OpenHands, SWE-agent, Claude Code, ...)

| Layer | Language | Purpose |
|---|---|---|
| Frontend | TypeScript (SolidJS + Tailwind CSS) | Web GUI, REST + WebSocket |
| Core Service | Go 1.25 | HTTP/WS, scheduling, repo management, state, policies (NATS JetStream to the workers) |
| AI Workers | Python 3.12 | LLM calls via the LiteLLM proxy, agent loop, tools |
| Infrastructure | Docker | Containers, Docker-in-Docker |

Strategic principles: leverage existing building blocks (LiteLLM, OpenSpec, Aider/OpenHands as backends); do not reinvent the wheel, differentiate by integrating all four pillars; Go for the core, Python only for AI-specific work.

More: [`docs/architecture.md`](docs/architecture.md), [`docs/architecture/project-reference.md`](docs/architecture/project-reference.md) (patterns, competitors, phases, protocols), Known Issues in [`docs/todo.md`](docs/todo.md#known-issues) and [`docs/known-issues-fix-plan.md`](docs/known-issues-fix-plan.md).

---

## 3. Architecture rules

### Structure
- **Hexagonal (ports and adapters)** Go Core; providers self-register via `init()` (registry pattern); providers declare **capabilities** instead of implementing everything; every port interface has a **compliance test** suite that new adapters inherit.
- **Approach C (ADR-006):** Go owns state, policies and sessions; Python owns LLM calls, tools and the agent loop; every tool call gets a Go policy decision over NATS.
- **LLM capability levels:** full-featured agents (Claude Code, Aider, OpenHands) get orchestration only; APIs with tools (OpenAI, Claude, Gemini) add context (GraphRAG), routing and tools; pure completion (Ollama, LM Studio) gets everything (context, tools, prompts, quality). Worker modules: [`docs/architecture.md`](docs/architecture.md#worker-modules-in-detail).
- Where things live: event types in `internal/domain/event/` (not the adapter layer); OTEL span helpers in `internal/telemetry/` (API only; services use the `port/metrics.Recorder` interface); decoupling ports `port/codeintel/`, `port/tokenexchange/`, `port/llm/`; non-fatal store errors via `logBestEffort` (`internal/service/log_best_effort.go`), never silenced.
- Prompt templates: YAML library in `internal/service/prompts/` (`//go:embed`, `text/template` via `PromptAssembler`) plus `.tmpl` files in `internal/service/templates/`.

### Infrastructure
- **Zero-config startup:** everything runs with defaults. Config precedence: defaults < YAML < env < CLI flags (ADR-003). YAML for all config files; JSON only for API responses, events and internal data exchange.
- **Async-first:** logging, NATS and LLM calls never block the hot path (buffered channels + workers in Go, QueueHandler + QueueListener in Python). Logs are structured JSON on stdout (ADR-004, ADR-005).
- **Resilience:** circuit breakers (NATS, LiteLLM), idempotency keys, dead letter queues, 4-phase graceful shutdown.
- **Policy layer** ([ADR-007](docs/architecture/adr/007-policy-layer.md), [ADR-015](docs/architecture/adr/015-policy-deny-lists-and-tool-names.md)): YAML profiles (5 presets, custom profiles tenant-scoped in `<policy.custom_dir>/<tenant_id>/`); canonical tool names (`internal/domain/policy/toolnames.go`); deny lists win, then first-match-wins; unknown profile or mode denies; shell commands are checked per simple command and opaque constructs fail closed; profile resolution request > project > default. The worker offers the LLM only the tools its mode allows; Claude Code runs reach the same check through their PreToolUse hook.

### Workspaces are untrusted (ADR-017, ADR-018, [SECURITY.md](docs/SECURITY.md#agent-tool-isolation))
- The Go Core never executes workspace code: tests, hooks and build scripts run in the worker (including the auto-agent's `conversation.test.request`).
- Every Go git call on a workspace goes through `internal/git` (KI-77); never `exec.Command("git", ...)` on a workspace. Workspaces with submodules or a nested `.git` are refused for Go git operations (KI-88).
- Agent tool processes (Bash, grep, git, quality gates, workspace tests, backend CLIs, Claude Code, MCP stdio servers) start only through `workers/codeforge/tool_process.py`; no other worker module starts a process, and the Go Core never starts stdio MCP servers. They run as their tenant's tool UID (20000-29999, `tool_uid` from the Go Core) under Landlock, with the environment on a memfd, never in argv. With `CODEFORGE_TOOL_ISOLATION=required` a missing requirement fails every tool call.
- The worker never acts by path inside a tree a tool can write: descriptor-based walks only; its commands that run as a tool UID are bounded.
- In-process workspace file access goes through `os.Root` (Go, `internal/workspacefs`) and `codeforge.workspace_fs` (Python), never plain paths (KI-95). Knowledge-base content is read only below `knowledge.content_root/<tenant>`, benchmark datasets only below `benchmark.datasets_dir`.
- Project workspaces are deleted through the worker as the tenant (`workspace.delete.request`), not by the Go Core.

### LLM integration
- LiteLLM proxy (sidecar, port 4000) for all LLM calls; no custom LLM provider interface; Go and Python use its OpenAI-compatible API. Hybrid routing picks the exact model (ADR-012, `workers/codeforge/routing/`, on by default via `CODEFORGE_ROUTING_ENABLED`).
- Only platform admins (admins of the default tenant: `IsPlatformAdmin()`, `middleware.RequirePlatformAdmin`) change shared LLM models and subscription providers; `GET /llm/models` strips credential parameters; the Copilot token exchange returns status and expiry, never the token.

### Architectural decisions (`docs/architecture/adr/NNN-*.md`)
001 NATS JetStream | 002 PostgreSQL 18 | 003 config precedence | 004 async logging | 005 Docker-native logging | 006 Approach C | 007 policy layer | 008 benchmark evaluation | 009 GDPR | 010 A2A | 011 trust and quarantine | 012 hybrid routing | 013 config sub-structs in services | 014 store interface segregation | 015 deny lists and canonical tool names | 016 NATS delivery semantics | 017 tool isolation and NATS authentication | 018 per-tenant tool identities and Landlock

---

## 4. Dependencies

- Minimal dependencies: stdlib when it covers 80%+ of the need, then established libraries, then custom code. A new dependency needs an explicit justification and an entry in `docs/tech-stack.md`.
- Tooling: Python Poetry, Ruff, Pytest | Go golangci-lint v2.11.4, gofmt, goimports | TS ESLint, Prettier | pre-commit hooks (`.pre-commit-config.yaml`), Docker Compose.
- PostgreSQL 18, shared with LiteLLM (same database and `public` schema, LiteLLM tables prefixed `LiteLLM_`): Go pgx v5 + goose migrations, Python psycopg3; NATS JetStream KV for ephemeral state.
- Go: chi v5, coder/websocket, git CLI via `internal/git`. Not used: Echo/Fiber, gorilla/websocket, go-git.
- Frontend: @solidjs/router, Tailwind (no component library), native WebSocket and fetch wrappers in `frontend/src/api/`, SolidJS signals/stores/context, @unovis (charts), solid-monaco, @tanstack/solid-virtual, self-hosted fonts; design system docs in `frontend/src/ui/DESIGN-SYSTEM.md`. Not used: axios, styled-components, Kobalte, shadcn-solid, Socket.IO, Redux/Zustand. Full list: [`docs/tech-stack.md`](docs/tech-stack.md).

---

## 5. Language rules (MUST READ!)

- **Strict type safety:** no `any` / `interface{}` / `Any`; use generics, unions, specific interfaces.
- **Readable code = documentation:** comments only for the non-obvious "why".
- **DRY** at 3+ occurrences, no premature abstraction. **Simple over clever.** **Minimal surface area:** start private, export only when needed.
- **The Zen of Python (PEP 20) applies to all languages**, above all: explicit is better than implicit; simple is better than complex; flat is better than nested; readability counts; errors should never pass silently unless explicitly silenced; in the face of ambiguity, refuse the temptation to guess; if the implementation is hard to explain, it's a bad idea.
- **Go:** gofmt + goimports; golangci-lint (`hugeParam`: structs > 80 bytes by pointer); on interface changes grep ALL implementations including test mocks.
- **Python:** `except Exception as exc:` (never bare), log `error=str(exc)`; `LiteLLMClient.chat_completion()` (not `chat()`), check callers and fakes.
- **TypeScript:** no `any`; API calls only through the API client (`frontend/src/api/`), never raw `fetch()`.
- **Encoding:** config files (.env, .yaml, .toml, .json, .sh, .gitignore) are ASCII only; Mermaid for diagrams in docs.

---

## 6. Testing (TDD mandatory)

All new features follow TDD, no exceptions: **RED planning** (goals, acceptance criteria, happy/error paths, edge cases, integration points) -> **RED** (failing tests: table-driven, error types/messages, boundary values 0/1/max/max+1, nil/empty) -> **GREEN** (minimum code) -> **REFACTOR** (tests stay green).

Edge case checklist: nil/null | empty strings/slices/maps | duplicates (idempotency) | concurrent access | max length/overflow | invalid UTF-8/special chars | missing required fields | exists vs not-found | permission edge cases | timeout/cancellation.

E2E (details: [`docs/testing/e2e-setup.md`](docs/testing/e2e-setup.md)): `docker compose up -d postgres nats litellm`, then `APP_ENV=development CODEFORGE_AUTH_ADMIN_PASS=Changeme123 go run ./cmd/codeforge/`, then the Python worker (only after the Go Core), then `cd frontend && npm run dev` and `npx playwright test`. Backend 8080, frontend 3000, `admin@localhost` / `Changeme123`.

---

## 7. Agent system and cross-language rules

### Agent system ([`docs/features/04-agent-orchestration.md`](docs/features/04-agent-orchestration.md))
- Execution modes: only `mount` runs; `sandbox` and `hybrid` are rejected at start (HTTP 400, KI-13).
- Per-mode tool lists live inline in each Mode (`Mode.Tools` / `Mode.DeniedTools`, canonical names), not in separate YAML bundles, and are enforced by the Go policy on run and conversation paths.
- MCP servers are tenant-scoped and managed by the tenant's admins; credentials (env and header values, URL userinfo/query, credential args) are redacted to `***` and kept only when sent back unchanged for the same server; sse/streamable_http URLs follow `netutil.OutboundPolicy` in Core and worker (never link-local/metadata; private ranges only via `mcp.allowed_private_hosts`; the GitLab PM provider uses `pm.allowed_private_hosts`).
- Real-time state: `BroadcastEvent` is tenant-scoped (events without a tenant are dropped; `BroadcastGlobal` only for tenant-free data); WebSocket auth by single-use tickets (`POST /api/v1/ws/ticket`, then `GET /ws?ticket=`), never a JWT in the URL.
- Conversations: one active run per conversation (a second message gets 409); every dispatch has a `turn_id`, and calls of a stopped turn are denied. Runs (`runs.start`) use the same agent loop (`workers/codeforge/loop_config.py`). Key files: `workers/codeforge/agent_loop.py`, `workers/codeforge/tools/`, `internal/service/conversation.go`, `internal/service/runtime_*.go`.

### Cross-language checklist (Go / NATS / Python)

**Subjects and streams**
- Subjects match exactly: Go `internal/port/messagequeue/queue.go` (`Subject*`) <-> Python `workers/codeforge/consumer/_subjects.py`; new prefixes need a wildcard in the stream config (`internal/adapter/nats/nats.go`); every subject has a publisher on one side and a subscriber on the other.
- New subjects, worker durables and notification consumers go into `configs/nats/nats-server.conf` (per-user permissions, ADR-017; `workers/tests/test_nats_permissions.py` checks it with `NATS_SERVER_BIN`); inbox prefixes stay `_INBOX_core` / `_INBOX_worker`.

**Payloads**
- Go JSON tags equal the Pydantic field names (`internal/port/messagequeue/schemas_*.go` <-> `workers/codeforge/models.py`); test the round trip both ways; `int64`/`float64`/`time.Time` map to `int`/`float`/`datetime`.
- Secrets never hardcoded, always env vars (LiteLLM: `LITELLM_MASTER_KEY`); paths are resolved to absolute paths in Go before they go on NATS.

**Delivery and idempotency ([ADR-016](docs/architecture/adr/016-nats-delivery-semantics.md))**
- One shared durable pull consumer per subject and side (`codeforge-go-*` / `codeforge-py-*`, deliver policy `new`, `MaxDeliver` 4).
- Workspace-changing work (`runs.start`, `conversation.run.start`, `tasks.agent.*`, `benchmark.run.request`) is at-most-once: accepted with a confirmed ack, then its failure is reported as a failed completion, never re-executed. Every other handler is at-least-once and must be idempotent.
- Settle every message exactly once: ack on success or a published error result; `_retry_or_dead_letter` on failure; `_reject_invalid` (DLQ + term) for invalid payloads. Never NAK an invalid payload, never ack without a DLQ copy.
- Cancels and tool-call decisions reach runs and tasks through the worker's `NotificationHub` (`workers/codeforge/notifications.py`), never through per-run consumers.
- Work that can hang sends heartbeats with `tenant_id`; Go's stuck-work watchdog ends work whose heartbeats stop, and Go subscribes to the DLQs of at-most-once subjects to end dead-lettered work.
- The worker starts after the Go Core (the Core creates the stream).

**Errors**
- `except Exception as exc:`, log `error=str(exc)`, publish the error back to NATS, then settle the message as above.

**Tenant isolation**
- All tenant-scoped queries: `AND tenant_id = $N` with `tenantFromCtx(ctx)`. Exceptions only for user/token/tenant management and system jobs that must span tenants; those queries are commented `INTENTIONALLY CROSS-TENANT` with the reason and handle each row in its own tenant's context.
- LIMIT via `$N` placeholders, never `%d` interpolation.
- NATS payloads carry `tenant_id`; handlers use `tenantctx.WithTenant(ctx, payload.TenantID)`; every Go publish also sets the `X-Tenant-ID` header (precedence request > payload > header), and the worker echoes the tenant on everything it publishes while handling a message.
- Inbound webhooks are per project (`/api/v1/webhooks/{vcs|pm}/{provider}/{id}`, own secret, tenant from the ID); `X-Tenant-ID` is never read there (KI-85). Slack and approval emails go only to `notification.approval_tenants`.
- Reference implementation: `store_project.go:GetProject`.

---

## 8. Subagents

- Use them for well-specified, independent work (one Known Issue or one coherent group per agent, in its own git worktree); give the scope, what not to touch, the conventions of this file and the verification commands in the prompt.
- Review every agent result before it lands: cherry-pick onto the working branch, run the full verification (Go race + integration, Python, frontend, golangci-lint, pre-commit), a security review and a code review, then a fix round. Docs are written by the lead, not the agent.
- Agents never push, never edit `AGENTS.md` or `docs/` unless asked, and use a private test database when they add migrations.

---

## 9. Dev container and cloud sessions

- Dev container (`.devcontainer/`): Go 1.25, Python 3.12, Node 22, Poetry, golangci-lint v2.11.4, pre-commit; services via `docker compose up -d postgres nats litellm`. Published compose ports bind to `127.0.0.1` only.
- Claude Code on the web: `.claude/hooks/session-start.sh` installs the toolchains like CI and starts PostgreSQL 18 (`codeforge-test-postgres`) and NATS (`codeforge-test-nats`) on `127.0.0.1:5432` / `:4222`, exporting `DATABASE_URL` / `NATS_URL`.
- Startup order for manual runs: Docker services -> Go Core -> Python worker -> frontend (WSL2: `source scripts/resolve-docker-ips.sh`). Details: [`docs/dev-setup.md`](docs/dev-setup.md).
- JetStream error 10047 (insufficient storage) in tests means the disk is full: free space (e.g. `go clean -cache`) and restart the NATS container.
- Root tool-isolation tests start processes as the tool UIDs: never run two such test runs on one host at the same time.

---

## 10. Navigation

| Area | Path |
|---|---|
| Go entry point, wiring | `cmd/codeforge/main.go` |
| Domain / ports / adapters / services | `internal/domain/`, `internal/port/`, `internal/adapter/`, `internal/service/` |
| HTTP routes and handlers | `internal/adapter/http/` (`routes.go`) |
| NATS subjects and payloads | `internal/port/messagequeue/` <-> `workers/codeforge/consumer/_subjects.py`, `workers/codeforge/models.py` |
| Policy layer | `internal/domain/policy/`, `internal/service/policy.go` |
| Worker (agent loop, tools, consumers) | `workers/codeforge/` (`agent_loop.py`, `tools/`, `consumer/`) |
| Tool isolation, notifications, NATS permissions | `workers/codeforge/tool_process.py`, `workers/codeforge/notifications.py`, `configs/nats/nats-server.conf` |
| Frontend | `frontend/src/` (`api/`, `features/`, `ui/DESIGN-SYSTEM.md`) |
| Migrations | `internal/adapter/postgres/migrations/` (goose) |
| Config | `codeforge.example.yaml`, `internal/config/` |
| Compose / deployment | `docker-compose.yml`, `docker-compose.prod.yml`, `docs/disaster-recovery.md` |
| Docs index / TODO and Known Issues | `docs/README.md`, `docs/todo.md` |
