# AGENTS.md

Instructions for developers and AI assistants working on CodeForge. Details live in `docs/`; this file holds the rules that apply to every change.

This is the single instruction file for every coding agent (the AGENTS.md convention); there is no `CLAUDE.md`. Claude Code reads `AGENTS.md` automatically when no `CLAUDE.md` exists (v2.1.277 or newer). Project-specific memories and decisions are stored here.

All project documentation, code comments, commit messages and configs are written in **English only**.

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

Rules:
- Fix all errors before committing (warnings can be acceptable depending on the check; do not add new ones).
- Small, atomic commits; one isolated subtask per commit, never batch unrelated changes.
- Commit messages follow Conventional Commits: `<type>(<scope>): <subject>` (`feat`, `fix`, `docs`, `refactor`, `test`, `chore`, ...), English.
- **Docs ship with the code: every change must be documented** in the same commit (table below). Read `docs/todo.md` before work; mark items `[x]` with the date on completion; add new tasks and Known Issues when discovered. Feature TODOs in `docs/features/*.md` are cross-referenced in `docs/todo.md`.
- Push after each successful change.
- Larger refactors: write a brief Markdown plan (problem, design, affected files, tests) in `docs/plans/` first.

| Change type | Update |
|---|---|
| Feature work | `docs/features/*.md`, `docs/todo.md` |
| Architecture decision | `docs/architecture.md`, `docs/architecture/adr/`, `AGENTS.md` |
| New dependency/tool | `docs/tech-stack.md`, `docs/dev-setup.md` |
| Milestone complete | `docs/project-status.md`, `docs/todo.md` |
| New dir/port/env var | `docs/dev-setup.md` |
| Core pillar change | `AGENTS.md`, `docs/features/*.md` |
| Design spec/plan | `docs/specs/` or `docs/plans/`, `docs/todo.md` |
| Test results | `docs/testing/`, `docs/todo.md` |
| Audit findings | `docs/audits/`, `docs/todo.md` |
| Any code change | `docs/todo.md` |

Feature docs: one per pillar in `docs/features/`, sub-features as sections, with overview/design/API/TODOs. ADRs: Context -> Decision -> Consequences -> Alternatives (`docs/architecture/adr/_template.md`).

### Versioning and release (merge to `main` only on explicit user request)

**Source of truth:** `VERSION` file (root, semver string e.g. `0.8.0`). Current: **v0.8.0**

| Layer | Mechanism | Key file |
|---|---|---|
| Go | `internal/version` reads `VERSION`, overridable via `-ldflags` | `internal/version/version.go` |
| Python | `_read_version()` traverses paths | `workers/codeforge/__init__.py` |
| Frontend | Vite `define: { __APP_VERSION__ }` at build | `frontend/vite.config.ts` |
| Docker | `ARG APP_VERSION` + OCI labels | `.github/workflows/docker-build.yml` |

**Change:** Edit `VERSION` -> `./scripts/sync-version.sh` (propagates to pyproject.toml, package.json, package-lock.json) -> all layers auto-pick-up.

**Build (Docker/CI):** Go: ldflags (`version.Version`, `version.GitSHA`). Python/Frontend: `VERSION` file COPY'd. OCI labels: `org.opencontainers.image.version/.revision`. Tags: `ghcr.io/.../codeforge-core:0.8.0`

---

## 2. Project overview

CodeForge is a containerized service for orchestrating AI coding agents with a web GUI. Four core pillars:
1. **Project Dashboard**: management of multiple repos (Git, GitHub, GitLab, SVN, local)
2. **Roadmap/Feature-Map**: visual management, compatible with OpenSpec, bidirectional sync to repo specs
3. **Multi-LLM-Provider**: OpenAI, Claude, local models (Ollama/LM Studio), routing via LiteLLM
4. **Agent Orchestration**: coordination of coding agents (Aider, OpenHands, SWE-agent, Claude Code, ...)

```
TypeScript Frontend (SolidJS)  -->  REST / WebSocket
Go Core Service (HTTP, WS, Agent Lifecycle, Repo Mgmt, Scheduling)  -->  NATS JetStream
Python AI Workers (LLM Calls via LiteLLM Proxy, Agent Execution with custom agent loop; LangGraph planned)
```

| Layer | Language | Purpose |
|---|---|---|
| Frontend | TypeScript | Web GUI (SolidJS + Tailwind CSS) |
| Core Service | Go 1.25 | HTTP/WS Server, Scheduling, Repo Mgmt |
| AI Workers | Python 3.12 | LLM Integration, Agent Execution |
| Infrastructure | Docker | Containerization, Docker-in-Docker |

Strategic principles:
- Leverage existing building blocks (LiteLLM, OpenSpec, Aider/OpenHands as backends)
- Do not reinvent the wheel — differentiate through integration of all four pillars
- Performance focus: Go for core, Python only for AI-specific work

Market positioning, adopted framework patterns, competitors, implemented phases and protocols: [`docs/architecture/project-reference.md`](docs/architecture/project-reference.md). Architecture: [`docs/architecture.md`](docs/architecture.md). Known Issues and the fix plan: [`docs/todo.md`](docs/todo.md#known-issues), [`docs/known-issues-fix-plan.md`](docs/known-issues-fix-plan.md).

---

## 3. Architecture (Hexagonal, Approach C)

- **Hexagonal Architecture (Ports & Adapters)** for Go Core
- **Provider Registry Pattern** — self-registering via `init()`, open-source extensibility
- **Capabilities** instead of mandatory implementation — each provider declares what it can do
- **Compliance Tests** per interface — new adapters inherit the test suite automatically
- **LLM Capability Levels:**
  - Full-featured agents (Claude Code, Aider, OpenHands): orchestration only
  - API with tools (OpenAI, Claude API, Gemini): + Context (GraphRAG) + Routing + Tools
  - Pure completion (Ollama, LM Studio): + everything (Context, Tools, Prompts, Quality)
- **Worker Modules:** Context (GraphRAG), Quality (Debate/Reviewer/Sampler/Guardrail), Routing, Safety, Execution, Memory, History, Events, Orchestration, Hooks, Trajectory, HITL
- **Event types** live in `internal/domain/event/` (broadcast.go, broadcast_payloads.go, agui.go) -- not in adapter layer
- **OTEL span helpers** live in `internal/telemetry/` (API-only, no SDK dependency) -- services use `port/metrics.Recorder` interface
- **Port interfaces for decoupling:** `port/codeintel/` (LSP provider), `port/tokenexchange/` (Copilot token exchange), `port/llm/` (LLM provider)
- **logBestEffort pattern** (`internal/service/log_best_effort.go`) for non-fatal store errors -- logs with structured context instead of silencing

Infrastructure principles:
- **Zero-config startup** — system runs with defaults; CLI flags have highest precedence
- **Async-first:** Logging, NATS, LLM calls never block hot path. Buffered channels + workers (Go), QueueHandler + QueueListener (Python)
- **Docker-native logging:** Structured JSON to stdout, `docker compose logs` + `jq` for debugging
- **Policy Layer:** Declarative YAML, 5 built-in presets, extensible without code (`policy.custom_dir`, default `data/policies`). ADR-015: canonical tool names (`internal/domain/policy/toolnames.go`), deny lists win regardless of rule order, then first-match-wins; workspace-relative paths; shell commands parsed per simple command (`command.go`, opaque constructs fail closed); unknown profile/mode denies. Workers send `tool`, `command`, `path`, `mode_id` and a display-only `arguments_preview` on `runs.toolcall.request`; `agui.permission_request` carries the deciding `profile`. Claude Code runs (`claudecode/*`) reach the same check through a PreToolUse hook and a per-run socket (`workers/codeforge/claude_code_executor.py`; repo settings and MCP servers are not loaded; every Bash call starts in the workspace via `CLAUDE_BASH_MAINTAIN_PROJECT_WORKING_DIR=1`). Custom profiles are tenant-scoped (`<custom_dir>/<tenant_id>/`, presets global and read-only); runs and conversations resolve the profile request > project (`policy_profile` / config `policy_preset`) > default; Bash redirection targets are checked against `path_deny` (unknown targets fail closed); the worker offers the LLM only the tools its mode allows
- **Approach C:** Go owns state/policies/sessions; Python owns LLM/tools/agent loop; NATS with per-tool-call policy
- **Workspaces are untrusted:** agents write to them, so the Go Core never executes workspace code (tests, hooks, build scripts run in the worker, the auto-agent's test run included: `conversation.test.request`) and every Go git call in a workspace goes through `internal/git` (sanitised environment, config allowlist, no hooks/filters/fsmonitor; KI-77). Workspaces with submodules or a nested `.git` are refused for all Go git operations (KI-88). Never call `exec.Command("git", ...)` on a workspace directly
- **Resilience:** Circuit breakers (NATS, LiteLLM), idempotency keys, dead letter queues, 4-phase graceful shutdown

Configuration: YAML for all config files (comments allowed: modes, settings, safety, autonomy, schedules); JSON only for API responses, event serialization and internal data exchange. Precedence: defaults < YAML < env < CLI flags (ADR-003).

### LLM integration
- **LiteLLM Proxy** as Docker sidecar (port 4000) — no custom LLM provider interface
- Go + Python communicate via OpenAI-compatible API against LiteLLM
- **Hybrid Routing (Phase 29):** Enabled by default (`CODEFORGE_ROUTING_ENABLED=true`). Cascade order: (1) ComplexityAnalyzer (rule-based, <1ms, always runs) -> (2) MABModelSelector (UCB1, primary) -> (3) LLMMetaRouter (cold-start fallback) -> (4) Complexity defaults (final fallback). Package: `workers/codeforge/routing/`
- LiteLLM uses provider wildcards (`openai/*`, `anthropic/*`) — HybridRouter picks exact model
- Scenario tags (default/background/think/longContext/review/plan) as fallback when routing disabled
- OpenRouter as optional provider; GitHub Copilot Token Exchange — `internal/adapter/copilot/client.go`, `POST /api/v1/copilot/exchange` (platform admins only, returns status/expiry, never the token). Platform admin = admin of the default tenant (`user.User.IsPlatformAdmin()`, `middleware.RequirePlatformAdmin`, `is_platform_admin` on the user JSON): only platform admins change shared LLM models and subscription providers; `GET /llm/models` strips credential parameters
- Local Model Auto-Discovery (Ollama/LM Studio `/v1/models`)
- Custom: LiteLLM Config Manager, User Key Mapping, Cost Dashboard

Details: `docs/architecture.md` | Framework comparison: `docs/research/market-analysis.md`

### Architectural decisions
| ADR | Decision | Details |
|---|---|---|
| 001 | NATS JetStream as MQ | `docs/architecture/adr/001-nats-jetstream-message-queue.md` |
| 002 | PostgreSQL 18 as DB | `docs/architecture/adr/002-postgresql-database.md` |
| 003 | Config: defaults < YAML < env < CLI flags | `docs/architecture/adr/003-config-hierarchy.md` |
| 004 | Async logging (buffered channel + workers) | `docs/architecture/adr/004-async-logging.md` |
| 005 | Docker-native logging (no ELK/Loki/Grafana) | `docs/architecture/adr/005-docker-native-logging.md` |
| 006 | Approach C: Go control plane + Python runtime | `docs/architecture/adr/006-agent-execution-approach-c.md` |
| 007 | Policy: first-match-wins permission rules | `docs/architecture/adr/007-policy-layer.md` |
| 008 | Benchmark: DeepEval + GEMMAS-inspired metrics (AgentNeo removed 2026-03-05) | `docs/architecture/adr/008-benchmark-evaluation-framework.md` |
| 009 | GDPR compliance architecture | `docs/architecture/adr/009-gdpr-compliance-architecture.md` |
| 010 | A2A protocol adoption for agent federation | `docs/architecture/adr/010-a2a-protocol-adoption.md` |
| 011 | Trust annotations and message quarantine | `docs/architecture/adr/011-trust-quarantine-system.md` |
| 012 | Hybrid routing cascade for model selection | `docs/architecture/adr/012-hybrid-routing-cascade.md` |
| 013 | Service layer config sub-struct imports | `docs/architecture/adr/013-config-import-in-services.md` |
| 014 | Store interface segregation plan | `docs/architecture/adr/014-store-interface-segregation.md` |
| 015 | Policy deny lists are blocklists; canonical tool names (amends 007) | `docs/architecture/adr/015-policy-deny-lists-and-tool-names.md` |
| 016 | NATS delivery: shared durables without replay, ack-on-accept runs, bounded retries + DLQ (refines 001) | `docs/architecture/adr/016-nats-delivery-semantics.md` |

---

## 4. Dependencies

- Minimal dependencies: prefer the stdlib when it covers 80%+ of the need, then established libraries, then custom code. New dependencies need an explicit justification and an entry in `docs/tech-stack.md`.
- Tooling: **Python** Poetry, Ruff, Pytest | **Go** golangci-lint v2.11.4, gofmt, goimports | **TS** ESLint, Prettier | **All** pre-commit hooks (`.pre-commit-config.yaml`), Docker Compose.

**PostgreSQL 18** (shared with LiteLLM: same `codeforge` database and `public` schema, LiteLLM tables prefixed `LiteLLM_`; no schema separation): Go pgx v5 + goose migrations, Python psycopg3 (sync+async), NATS JetStream KV for ephemeral state. ADR: `docs/architecture/adr/002-postgresql-database.md`

**Go Libraries (minimal-dep):** chi v5 (router), coder/websocket v1.8+ (WS), git CLI via the hardened `internal/git` package (KI-77). NOT used: Echo/Fiber, gorilla/websocket, go-git.

**Frontend Libraries (minimal-stack):** @solidjs/router, Tailwind CSS (no component lib), native WebSocket wrapper (`frontend/src/api/websocket.ts`), native fetch + thin wrapper, SolidJS signals/stores/context, Unicode+inline SVG icons plus vscode-icons-js (file icons), @unovis/solid + @unovis/ts (charts), solid-monaco (code editor), @tanstack/solid-virtual (virtual lists), Outfit+Source Sans 3 fonts (self-hosted woff2 in `frontend/public/fonts/`), design system at `/design-system` (dev-only, docs in `frontend/src/ui/DESIGN-SYSTEM.md`), onboarding wizard (`codeforge-onboarding-completed` key) — `frontend/src/features/onboarding/OnboardingWizard.tsx`. NOT used: axios, styled-components, Kobalte, shadcn-solid, Socket.IO, Redux/Zustand.

---

## 5. Language rules (MUST READ!)

- **Strict type safety:** No `any`/`interface{}`/`Any` — use generics, unions, specific interfaces
- **Minimal dependencies:** Prefer stdlib when it covers 80%+ of need
- **Readable code = documentation:** Comments only for non-obvious "why"
- **DRY:** Extract at 3+ occurrences, no premature abstraction
- **Simple over clever:** Next developer/agent must understand immediately
- **Minimal surface area:** Start private, export only when needed

The Zen of Python (PEP 20) applies to ALL languages:
- Beautiful is better than ugly.
- Explicit is better than implicit.
- Simple is better than complex.
- Complex is better than complicated.
- Flat is better than nested.
- Sparse is better than dense.
- Readability counts.
- Special cases aren't special enough to break the rules.
- Although practicality beats purity.
- Errors should never pass silently.
- Unless explicitly silenced.
- In the face of ambiguity, refuse the temptation to guess.
- There should be one-- and preferably only one --obvious way to do it.
- Now is better than never.
- Although never is often better than *right* now.
- If the implementation is hard to explain, it's a bad idea.
- If the implementation is easy to explain, it may be a good idea.
- Namespaces are one honking great idea -- let's do more of those!

Language specifics:
- **Go:** `gofmt` + `goimports`; golangci-lint (`hugeParam`: structs > 80 bytes by pointer); interface changes: grep ALL implementations including test mocks; non-fatal store errors via `logBestEffort`.
- **Python:** `except Exception as exc:` (never bare), log `error=str(exc)`; `LiteLLMClient.chat_completion()` (not `chat()`), check callers and fakes.
- **TypeScript:** no `any`; API calls through the API client (`frontend/src/api/`), never raw `fetch()`.

Character encoding:
- **Config files (.env, .yaml, .toml, .json, .sh, .gitignore): ASCII only** — no box-drawing chars
- Regular dashes `-` and `=` for separators; Mermaid blocks for diagrams in docs

---

## 6. Testing (TDD mandatory)

**All new features MUST follow TDD.** No exceptions.

1. **RED planning** — Analyze: goals, acceptance criteria, happy path, error paths, edge cases, integration points
2. **RED** — Write failing tests: table-driven, error types/messages, boundary values (0, 1, max, max+1), nil/empty
3. **GREEN** — Minimum code to pass all tests
4. **REFACTOR** — Clean up, extract, rename, deduplicate (tests stay green)

**Edge case checklist (every feature):**
Nil/null pointers | empty strings/slices/maps | duplicates (idempotency) | concurrent access | max length/overflow | invalid UTF-8/special chars | missing required fields | exists vs not-found | permission edge cases | timeout/cancellation

E2E (full stack in development mode; details, LLM E2E tests and the autonomous goal-to-program run: [`docs/testing/e2e-setup.md`](docs/testing/e2e-setup.md)):

```bash
docker compose up -d postgres nats litellm
APP_ENV=development CODEFORGE_AUTH_ADMIN_PASS=Changeme123 go run ./cmd/codeforge/   # dev endpoints return 403 without APP_ENV=development
cd frontend && npm run dev
cd frontend && npx playwright test
```

Start the Python worker only after the Go Core (it waits for the Core to create the NATS stream). Backend 8080, frontend 3000; credentials `admin@localhost` / `Changeme123` (seeded only when `CODEFORGE_AUTH_ADMIN_PASS` is set; the admin must change the password, which `frontend/e2e/global-setup.ts` handles).

---

## 7. Agent system and cross-language rules

### Agent system
- **Execution Modes:** Sandbox (isolated container), Mount (direct file access), Hybrid — sandbox/hybrid are rejected at start (HTTP 400) until tools run inside the container (KI-13, fail closed); the project config key `execution_mode` (only `mount` accepted) sets the default
- **Safety Layer (8):** Budget Limiter, Command Safety Evaluator, Branch Isolation, Test/Lint Gate, Max Steps, Rollback, Path Blocklist, Stall Detection
- **Workflow:** Plan -> Approve -> Execute -> Review -> Deliver (configurable)
- **Autonomy Levels:** 1=supervised (approve all), 2=semi-auto (approve destructive), 3=auto-edit (approve terminal/deploy), 4=full-auto (safety rules replace user), 5=headless (CI/CD, cron, API)
- **Modes System:** YAML-configurable roles (architect, coder, reviewer, debugger), per-mode tools/LLM/autonomy/prompt, built-in + custom (`.codeforge/modes/`), DAG pipelines, schedule support
- **Per-Mode Tool Lists:** Tools are defined inline in each Mode struct (`Mode.Tools` / `Mode.DeniedTools`, canonical names `Read`/`Write`/`Edit`/`Bash`/`Grep`/`Glob`/`ListDir`), not as separate YAML bundle files — enforced by the Go policy evaluation on run and conversation paths (`DeniedTools` denies; a non-empty `Tools` list denies unlisted built-in tools)
- **History Processors:** Context window optimization pipeline
- **Hook System:** Observer pattern for agent/environment lifecycle
- **Trajectory:** Recording, replay, inspector, audit trail
- **Cost Management:** Budget limits per task/project/user, auto-tracking
- **Prompt Templates:** YAML prompt library (`internal/service/prompts/`, embedded via `//go:embed`, rendered with Go `text/template` by `PromptAssembler`) plus `.tmpl` files in `internal/service/templates/`
- **BM25S Retrieval:** Code search and tool recommendation
- **SimHash Dedup:** 64-bit fingerprints, hamming distance threshold — `internal/service/dedup.go`
- **Real-time State:** WebSocket live updates for agent status, logs, costs — tenant-scoped (`BroadcastEvent` uses the tenant in ctx, events without one are dropped; `BroadcastGlobal` only for tenant-free data), per-client send queues, single-use tickets (`POST /api/v1/ws/ticket`, then `GET /ws?ticket=`; no JWT in the URL). NATS payloads carry `tenant_id` and the worker echoes it

### Agentic conversation loop and runs
- Multi-turn tool-use loop: LLM -> tools -> results -> repeat. Go dispatches via NATS (`conversation.run.start/complete`)
- 10 built-in tools (`read_file`, `write_file`, `edit_file`, `bash`, `search_files`, `glob_files`, `list_directory`, `search_conversations`, `search_skills`, `create_skill`) plus context-dependent `handoff_to` (offered whenever registered), `propose_goal`, `propose_roadmap` + MCP merge (`spawn_subagent` is not offered until it starts sub-agents: see [Known Issues](docs/todo.md#known-issues) KI-25)
- `AgentLoopExecutor` (Python): streaming LLM, per-tool policy, cost tracking
- `ConversationHistoryManager`: head-and-tail token budget, tool result truncation
- HITL: `DecisionAsk` -> WS `agui.permission_request` -> HTTP approve/deny -> channel resume
- One run per conversation: a second message while a run is active gets 409; each dispatch has a `turn_id` (on `conversation.run.start`, `runs.toolcall.request`, `conversation.run.complete`) so calls of a stopped run are denied
- Config: `MaxLoopIterations` (50), `MaxContextTokens` (128K), `ContextEnabled` (true), `ContextBudget` (2048), `ContextPromptReserve` (512), `ApprovalTimeoutSeconds` (60), `ToolOutputMaxChars` (10000; sent on `runs.start` and `conversation.run.start`)
- **Adaptive Context Budget:** Linear decay from `ContextBudget` to 0 over 60 messages — `internal/service/context_budget.go`
- **Auto-Indexing:** Clone/Adopt/Setup trigger RepoMap + Retrieval Index + GraphRAG — `internal/service/project.go` (`AutoIndex`), called from `internal/adapter/http/handlers_project.go`
- Runs (`runs.start`) use the same loop in the run's project workspace (shared setup: `workers/codeforge/loop_config.py` `build_loop_config`, `resolve_model_and_fallbacks`; no skill tools, no Claude Code models), with heartbeats and a Go decision per LLM and tool call
- Key files: `workers/codeforge/agent_loop.py`, `workers/codeforge/tools/`, `internal/service/conversation.go`, `internal/service/runtime_execution.go`, `internal/service/runtime_lifecycle.go`

### Cross-language integration checklist (Go / NATS / Python)

When modifying code that crosses the Go/Python boundary via NATS, verify ALL:

#### NATS Subjects & Streams
- Subjects must match EXACTLY: Go (`internal/port/messagequeue/queue.go`, `Subject*` constants) <-> Python (`workers/codeforge/consumer/_subjects.py`)
- JetStream stream config (`internal/adapter/nats/nats.go`, `CreateOrUpdateStream` subjects list) must include wildcards for new prefixes (e.g. `benchmark.>`)
- New subjects need publisher (one side) + subscriber (other side)

#### JSON Payload Contracts
- Go JSON tags must match Python Pydantic field names exactly
- Sync changes: Go (`internal/port/messagequeue/schemas_*.go` / domain) <-> Python (`workers/codeforge/models.py`)
- Test round-trip both directions
- Type mapping: Go `int64`/`float64`/`time.Time` <-> Python `int`/`float`/`datetime`

#### API Keys & Secrets
- NEVER hardcode — always env vars. LiteLLM auth: `LITELLM_MASTER_KEY` (default: `sk-codeforge-dev`)
- Verify model has valid key in `litellm/config.yaml`

#### Method Signatures & Interfaces
- Python `LiteLLMClient.chat_completion()` (NOT `chat()`) — check callers + fakes
- Go interface changes: grep ALL implementations including test mocks
- `golangci-lint hugeParam`: structs >80 bytes -> pointer

#### Path Resolution
- Frontend sends dataset names -> Go resolves to absolute paths -> NATS -> Python receives absolute paths
- Verify: `internal/service/benchmark_run.go` `(*BenchmarkRunManager).resolveDatasetPath()`

#### Delivery & Idempotency (ADR-016)
- One shared durable pull consumer per subject and side (`codeforge-go-*` / `codeforge-py-*`), created with deliver policy `new`, no inactivity threshold, `MaxDeliver` 4 — provisioning: `internal/adapter/nats/nats.go`, `workers/codeforge/consumer/_delivery.py`
- Workspace-changing work (`runs.start`, `conversation.run.start`, `tasks.agent.*`, `benchmark.run.request`) is at-most-once: accepted with a confirmed double ack (`ack_sync`, retried 3 times; unconfirmed -> NAK, work not started), registered in `self._in_flight`, and its failure is reported as a failed completion; completions go through `_publish_result` / `publish_with_retry` (retries, one `Nats-Msg-Id`). Every other subject is at-least-once and its handler must be idempotent
- Settle every message exactly once: success or a published error result -> ack; failure -> `_retry_or_dead_letter` (NAK with delay, DLQ + ack on the last attempt, from `num_delivered`); invalid payload -> `_reject_invalid` (DLQ + `term`). Never NAK an invalid payload, never ack without a DLQ copy (DLQ copies drop `Nats-*` headers)
- Duplicate guards (skip if already `"completed"`); at-least-once requests are deduplicated per message (key plus stream sequence), at-most-once work by run ID; a failed request is removed from the dedup cache
- Notification subscriptions (per-run and per-task cancel listeners, one `runtime.listen_for_cancel` helper, which replays from the start message's stream sequence so no cancel is lost; tool-call responses) use deliver policy `new` and ack policy `none`; a start skipped because it was cancelled publishes a `cancelled` completion (`tasks.cancel` does not skip run starts); Go keeps handlers in progress up to `Queue.SetMaxHandlerDuration` (covers the HITL approval timeout). The worker waits for a tool-call decision up to the approval timeout (`approval_timeout_seconds` on `runs.start` / `conversation.run.start`, default 60 s) plus 15 s
- Quality gates: `runs.qualitygate.request` carries `timeout_seconds` and `heartbeat_seconds`; while a gate runs the worker sends `runs.heartbeat` (with `tenant_id`, phase `quality_gate`) and keeps the request in progress; a check that could not run is reported as a null verdict plus `error` (only a check that ran and failed rolls back)
- `tasks.agent.*` carries `TaskAgentPayload` (`task_id`, `project_id`, `tenant_id`, `agent_id`, `backend`, `workspace_path`, `dispatch_id`, `heartbeat_seconds`); `tasks.heartbeat` and `tasks.result` echo `dispatch_id`: a heartbeat counts only for its dispatch, only the current dispatch's result ends the task and a dispatch's cost counts once (`task_result_costs`); an ended dispatch clears `dispatch_id`. Backend CLIs run in their own process group, which `tasks.cancel` stops (result status `cancelled`)
- A worker whose consumer loop gives up fails its unfinished accepted work (30 s grace) and exits 1; on SIGTERM it gives accepted work 5 s, then fails it before draining NATS (prod `stop_grace_period: 45s`). The worker waits for the Go Core to create the stream and must start after it
- Heartbeats: runs (`runs.heartbeat`, conversation runs with `turn_id`), backend tasks (`tasks.heartbeat`) and quality gates (phase `quality_gate`) carry `tenant_id`; Go stores them and the stuck-work watchdog ends work whose heartbeats stop (`heartbeat_timeout + 2 x heartbeat_interval`); `heartbeat_seconds` (`runtime.heartbeat_interval`) is sent on `runs.start`, `conversation.run.start` and `tasks.agent.*`; Go subscribes to `runs.start.dlq`, `conversation.run.start.dlq`, `tasks.agent.*.dlq`, `handoff.request.dlq`, `handoff.approved.dlq` and `benchmark.run.request.dlq` to end dead-lettered work; a `tasks.cancel` for a queued task is remembered so the task is not started later
- A handler may ask for a delayed redelivery with `messagequeue.RetryAfter` (it counts as a delivery). Handoffs: `handoff.request` (worker -> Go, `handoff_id`, string metadata) and `handoff.approved` (Go only) are at-least-once but start a run, so Go claims each stage in `handoff_claims` first (done state, 11-minute lease takeover, task reuse on retry, permanent `StartRun` errors refuse at once). A turn's worker completion is claimed once (`conversation_turn_completions`; Go's own failed completions do not claim); a non-active turn's completion stores messages and cost only when no newer turn started, otherwise cost only; tool calls of non-active turns are denied; a failed stop replays every deferred message kind in order

#### Error Handling
- `except Exception as exc:` (NOT bare), log `error=str(exc)`, publish errors back to NATS, then settle the message as above

#### Tenant Isolation
- ALL tenant-scoped queries: `AND tenant_id = $N` with `tenantFromCtx(ctx)` (exceptions: user/token/tenant mgmt, and system jobs that must span tenants - the GDPR retention sweep and the stuck-work watchdog's `ListStaleRuns` - whose queries are commented `INTENTIONALLY CROSS-TENANT` with the reason and handle each row in its own tenant's context)
- LIMIT via `$N` placeholders, not `%d` interpolation
- NATS payloads MUST carry `tenant_id` for background jobs -> `tenantctx.WithTenant(ctx, payload.TenantID)`; every Go publish also carries the tenant as the `X-Tenant-ID` header (used only when neither request nor payload sets it: request > payload > header), and the worker echoes it on everything it publishes while handling a message. Outgoing payloads take their tenant from the context (`outgoingTenant`; a missing tenant is logged as an error)
- Reference: `store_project.go:GetProject` (correct), `store_a2a.go` (fixed)

---

## 8. Subagents

- Use them for well-specified, independent work (one Known Issue or one coherent group per agent, its own git worktree); give the scope, what not to touch, the conventions of this file and the verification commands in the prompt.
- Review every agent result before it lands: cherry-pick onto the working branch, run the full verification (Go race + integration, Python, frontend, golangci-lint, pre-commit), a security review and a code review, then a fix round; docs are written by the lead, not the agent.
- Agents never push, never edit `AGENTS.md`/`docs/` unless asked, and use a private test database when they add migrations.

---

## 9. Dev container and cloud sessions

- **Dev container** (`.devcontainer/devcontainer.json`, `setup.sh`): Go 1.25, Python 3.12, Node 22, Poetry, golangci-lint v2.11.4, goimports, pre-commit hooks; services via `docker compose up -d postgres nats litellm` (docker-outside-of-docker, container names on the `codeforge` network). Published compose ports bind to `127.0.0.1` only.
- **Claude Code on the web**: `.claude/hooks/session-start.sh` (registered in `.claude/settings.json`) installs the toolchains like CI and starts PostgreSQL 18 (`codeforge-test-postgres`) and NATS JetStream (`codeforge-test-nats`) on `127.0.0.1:5432` / `:4222`, exporting `DATABASE_URL` / `NATS_URL`.
- **Startup order** (manual runs): Docker services -> Go Core -> Python worker -> frontend; on WSL2 use container IPs (`source scripts/resolve-docker-ips.sh`). Details: [`docs/dev-setup.md`](docs/dev-setup.md).
- **JetStream error 10047** (insufficient storage) in tests means the disk is full: free space (e.g. `go clean -cache`) and restart the NATS container.

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
| Frontend | `frontend/src/` (`api/`, `features/`, `ui/DESIGN-SYSTEM.md`) |
| Migrations | `internal/adapter/postgres/migrations/` (goose) |
| Config | `codeforge.example.yaml`, `internal/config/` |
| Compose / deployment | `docker-compose.yml`, `docker-compose.prod.yml`, `docs/disaster-recovery.md` |
| Docs index / TODO / Known Issues | `docs/README.md`, `docs/todo.md` |
| Project reference (patterns, competitors, phases, protocols) | `docs/architecture/project-reference.md` |

```
docs/
├── README.md               # Index
├── todo.md                 # Central TODO (single source of truth, incl. Known Issues)
├── known-issues-fix-plan.md # Milestone plan (S0-S6) for fixing the Known Issues
├── architecture.md         # System architecture
├── dev-setup.md            # Setup guide
├── project-status.md       # Phase tracking
├── tech-stack.md           # Dependencies
├── SECURITY.md             # Security policy, secret management
├── data-retention.md       # GDPR data retention policy
├── privacy-policy.md       # Privacy & LLM data processing notice
├── disaster-recovery.md    # Backup/restore runbook
├── features/               # Feature specs (01-07)
├── api/                    # OpenAPI spec (openapi.yaml)
├── security/               # Breach notification procedure, data classification
├── specs/                  # Design specs (*-design.md)
├── plans/                  # Implementation plans (*-plan.md)
├── testing/                # Test plans + reports, E2E setup (e2e-setup.md)
├── audits/                 # Schema, UX, code audits
├── architecture/           # ADRs 001-016 (adr/, use _template.md), project-reference.md
├── research/               # Market research
└── prompts/                # Claude Code audit/discovery prompts
```
