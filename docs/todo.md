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

## Current Priority: Phase 3 — Reliability, Performance & Agent Foundation

> Phase 0, Phase 1, and Phase 2 are complete. See "Recently Completed" below.
> Items below are extracted from architecture review + Perplexity analysis (2026-02-16/17).

### 3A. Configuration Management

- [x] (2026-02-17) Implement hierarchical config system (defaults < YAML < ENV)
  - Go: `internal/config/config.go` (typed Config struct), `internal/config/loader.go` (Load with merge logic)
  - Python: `workers/codeforge/config.py` (WorkerSettings from ENV)
  - Validation layer for required fields and min values
  - `codeforge.yaml.example` with all fields documented
  - 6 test functions in `internal/config/loader_test.go`
  - Deferred: SIGHUP reload, CLI override support

### 3B. Structured Logging & Observability

- [x] (2026-02-17) Structured JSON logging alignment (Go + Python)
  - Go: `internal/logger/logger.go` — slog JSON handler with `service` field, level from config
  - Python: `workers/codeforge/logger.py` — structlog JSON renderer with `service` field
  - Common schema: `{time, level, service, msg, request_id, task_id}`
  - 3 test functions in `internal/logger/logger_test.go`
- [x] (2026-02-17) Request ID propagation (Correlation ID)
  - Go: `internal/logger/context.go` (WithRequestID/RequestID), `internal/middleware/requestid.go`
  - NATS: Headers carry X-Request-ID, extracted in Subscribe, injected in Publish
  - Python: Extract from NATS headers, bind to structlog context
  - 2 test functions in `internal/middleware/requestid_test.go`
  - 1 new Python test for request ID propagation
  - Deferred: PostgreSQL log_line_prefix configuration
- [x] (2026-02-17) Docker Compose logging configuration
  - `x-logging` anchor with `json-file` driver, `max-size: 10m`, `max-file: 3`
  - Applied to all 5 services
- [ ] Async logging for Go Core
  - Create `internal/logger/async.go` with AsyncHandler (slog wrapper)
  - Buffer size: 10,000 records, 4 worker goroutines
  - Sample strategy for backpressure (keep 10% when queue > 80%)
  - Flush on shutdown, panic, and ERROR/FATAL logs (sync)
- [ ] Python async logging with QueueHandler
  - `workers/codeforge/logger.py` with `QueueHandler` + `QueueListener`
  - Buffer size: 10,000 records, graceful shutdown with queue drain
- [x] (2026-02-17) Create `scripts/logs.sh` helper script
  - Commands: `tail`, `errors`, `service <name>`, `request <id>`
  - Uses `docker compose logs` for filtering

### 3C. Reliability Patterns

- [x] (2026-02-17) Circuit Breaker for external services
  - `internal/resilience/breaker.go`: zero-dep, states closed/open/half-open, 5 tests
  - Wrapped: NATS Publish, LiteLLM doRequest (via SetBreaker injection)
  - Configurable maxFailures and timeout from config.Breaker
- [x] (2026-02-17) Graceful shutdown with drain phase
  - 4-phase ordered shutdown: HTTP → cancel subscribers → NATS Drain → DB close
  - NATS: `Drain()` method added to Queue interface and NATS adapter
  - Python: 10s drain timeout to prevent hanging
- [ ] Agent execution timeout & heartbeat
  - Context with timeout (10min default, configurable)
  - Heartbeat ticker (30s) for progress tracking
  - Cancellation channel for user abort
  - Implement in `internal/service/agent.go`
- [ ] Idempotency Keys for critical operations
  - Middleware: `internal/middleware/idempotency.go`
  - Storage: NATS JetStream KV (24h TTL)
  - Apply to POST/PUT/DELETE endpoints (task creation, agent start)
  - Add `Idempotency-Key` header to API spec
- [x] (2026-02-17) Optimistic Locking for concurrent updates
  - Migration 003: `version INTEGER NOT NULL DEFAULT 1` on projects, agents, tasks + auto-increment trigger
  - Domain: `ErrNotFound`, `ErrConflict` sentinel errors in `internal/domain/errors.go`
  - Store: `WHERE version = $N` on UpdateProject, `pgx.ErrNoRows` → `ErrNotFound` on Get queries
  - HTTP: `writeDomainError()` maps ErrNotFound → 404, ErrConflict → 409
  - Version field added to Project, Agent, Task domain structs
- [x] (2026-02-17) Dead Letter Queue (DLQ) for failed messages
  - Go NATS: `moveToDLQ()` publishes to `{subject}.dlq` after 3 retries, acks original
  - `Retry-Count` header tracked, `NakWithDelay(2s)` for retries
  - Python consumer: `_move_to_dlq()` + `_retry_count()` with same MAX_RETRIES=3
- [x] (2026-02-17) Schema validation for NATS messages
  - `internal/port/messagequeue/schemas.go`: typed payload structs per subject
  - `internal/port/messagequeue/validator.go`: `Validate(subject, data)` via JSON unmarshal
  - Invalid messages sent directly to DLQ (no retries)

### 3D. Event Sourcing for Agent Trajectory

- [x] (2026-02-17) Domain: `internal/domain/event/event.go` (AgentEvent struct, Type constants)
- [x] (2026-02-17) Port: `internal/port/eventstore/store.go` (Append, LoadByTask, LoadByAgent)
- [x] (2026-02-17) Storage: PostgreSQL table `agent_events` (append-only, migration 004, indexed)
- [x] (2026-02-17) Service: Event recording on dispatch/result/stop, LoadTaskEvents method
- [x] (2026-02-17) API: `GET /api/v1/tasks/{id}/events` handler + frontend client method
- [ ] Features: Replay task, trajectory inspector, audit trail
- [ ] Session Events as Source of Truth (append-only log for Resume/Fork/Rewind)
  - Every user/model/tool action recorded as event
  - Stream events via WebSocket/AG-UI to frontend
  - Event-schema versioning from day one

### 3E. Performance Optimizations

- [ ] Cache Layer with invalidation
  - Multi-tier: L1 (ristretto in-memory 100MB), L2 (NATS KV)
  - Create `internal/cache/cache.go` with Get/Set/Invalidate
  - Cache keys: `{namespace}:{entityID}:{operation}`
  - TTL per namespace: git (5m), tasks (1m), agents (10m)
- [x] (2026-02-17) Database connection pool tuning
  - `NewPool` accepts `config.Postgres` with MaxConns=15, MinConns=2, MaxConnLifetime=1h, MaxConnIdleTime=10m, HealthCheckPeriod=1m
  - All pool parameters configurable via YAML/ENV
- [x] (2026-02-17) Rate limiting per IP
  - `internal/middleware/ratelimit.go`: token bucket, per-IP, configurable rate/burst
  - Headers: `X-RateLimit-Remaining`, `X-RateLimit-Reset` (GitHub-style)
  - Returns 429 with `Retry-After` header
  - 4 tests in `internal/middleware/ratelimit_test.go`
- [ ] Go Core: Worker pools for CPU-bound tasks
  - Git operations (clone, diff parsing) via `errgroup.Group` with `SetLimit(5)`
  - Context propagation for cancellation
- [ ] Python Workers: Full asyncio adoption
  - NATS consumer async, LiteLLM calls async (httpx already async)
  - DB queries async (psycopg3 async mode)

### 3F. Security & Isolation

- [ ] Agent sandbox resource limits (Docker cgroups v2)
  - Create `internal/adapter/sandbox/docker.go`
  - Memory: 512MB, CPUs: 1, PidsLimit: 100
  - Storage quota: `--storage-opt size=10G`
  - Network: `none` mode initially, enable on-demand
  - Time limit: Context timeout (10min default)
- [ ] Secrets rotation support
  - Create `internal/secrets/vault.go` with hot reload
  - SIGHUP handler to trigger reload from ENV or external vault
  - RWMutex for safe concurrent access

### 3G. API & Health

- [x] (2026-02-17) Health check granularity
  - `/health` liveness (always 200), `/health/ready` readiness (pings DB, NATS, LiteLLM)
  - Returns 503 with per-service status + latency if any dependency down
  - NATS `IsConnected()` added to Queue interface
- [ ] API versioning with deprecation
  - Router: `/api/v1` (deprecated), `/api/v2` (current)
  - Middleware: `DeprecationWarning()` adds `Deprecation: true`, `Sunset: <date>` headers
- [ ] Database migrations: Rollback capability
  - All migrations must have `-- +goose Up` and `-- +goose Down`
  - Transactional migrations (BEGIN/COMMIT)
  - Test rollback in CI before merge

### 3H. Multi-Tenancy Preparation (Soft Launch)

- [ ] Add tenant_id to all tables
  - Migration: `004_add_tenant_id.sql`
  - Add `tenant_id UUID NOT NULL DEFAULT '00000000-...'`
  - Indexes: `idx_projects_tenant`, `idx_tasks_tenant`
  - Service layer: Extract tenant_id from context (single-tenant for now)

---

## Phase 4 — Agent Execution Engine (Approach C: Go Control Plane + Python Runtime)

> Architectural decision: Go-Core as "Control Plane" (State/Policies/Sessions),
> Python-Worker as "Data/Execution Plane" (Models/Tools/Loop execution).
> Source: Analyse-Dokument Section 13, Approach C.

### 4A. Policy Layer (Permission + Checkpoint Gate)

- [x] (2026-02-17) Design Policy domain model
  - `internal/domain/policy/policy.go`: PolicyProfile, PermissionRule, ToolSpecifier, ToolCall
  - Permission modes: `default`, `acceptEdits`, `plan`, `delegate`
  - Decisions: `allow`, `deny`, `ask`
  - Quality gates: `RequireTestsPass`, `RequireLintPass`, `RollbackOnGateFail`
  - Termination conditions: `MaxSteps`, `TimeoutSeconds`, `MaxCost`, `StallDetection`
  - Validation: `internal/domain/policy/validate.go`
- [x] (2026-02-17) Implement Policy Evaluator service
  - `internal/service/policy.go`: First-match-wins rule evaluation
  - ToolSpecifier matching: exact tool + optional sub-pattern (glob)
  - Path allow/deny lists (glob patterns with `**` recursive matching)
  - Command allow/deny lists (prefix-based matching)
  - Mode-based fallback: `plan`→deny, `default`→ask, `acceptEdits`→allow, `delegate`→allow
- [x] (2026-02-17) Policy Presets (4 built-in profiles)
  - `plan-readonly`: Read-only, deny Edit/Write/Bash, 30 steps, 300s, $1
  - `headless-safe-sandbox`: Safe Bash (git/test only), path deny for secrets, 50 steps, 600s, $5
  - `headless-permissive-sandbox`: Broader Bash, deny network cmds, 100 steps, 1800s, $20
  - `trusted-mount-autonomous`: All tools, deny only secrets paths, 200 steps, 3600s, $50
- [x] (2026-02-17) YAML-configurable custom policies
  - `internal/domain/policy/loader.go`: LoadFromFile, LoadFromDirectory
  - Config: `CODEFORGE_POLICY_DEFAULT`, `CODEFORGE_POLICY_DIR` env vars
- [x] (2026-02-17) Policy REST API (3 endpoints)
  - `GET /api/v1/policies` — list all profile names
  - `GET /api/v1/policies/{name}` — full profile definition
  - `POST /api/v1/policies/{name}/evaluate` — evaluate a ToolCall
- [x] (2026-02-17) Policy tests: 46 test functions across 4 files
  - `internal/domain/policy/policy_test.go` (5), `presets_test.go` (8), `loader_test.go` (7)
  - `internal/service/policy_test.go` (25)
  - Config tests (3 in `loader_test.go`), handler tests (7 in `handlers_test.go`)
- [ ] Implement Checkpoint system
  - Before every `Edit/Write/Replace`: automatic checkpoint
  - On failed Quality Gates (tests/lint): automatic rewind or re-plan
  - Shadow Git approach for Mount mode, Blob snapshots for Sandbox
- [ ] Policy UI in Frontend
  - Policy Editor per project (YAML-style, fits CodeForge config standard)
  - "Effective Permission Preview": show which rule matches and why
  - Scope levels: global (user) → project → run/session (override)
  - Preset selection + "Customize" (load preset, edit, save as new profile)
  - Run overrides: temporarily override policy per run

### 4B. Runtime API (Step-by-Step Execution Protocol)

- [x] (2026-02-17) Define Runtime Client protocol (Go ↔ Python)
  - Run entity: `internal/domain/run/run.go` (Run, StartRequest, Status, ExecMode)
  - ToolCall types: `internal/domain/run/toolcall.go` (ToolCallRequest/Response/Result)
  - Validation: `internal/domain/run/validate.go` (Run.Validate, StartRequest.Validate)
  - NATS subjects: `runs.start`, `runs.toolcall.{request,response,result}`, `runs.complete`, `runs.cancel`, `runs.output`
  - NATS payloads: RunStartPayload, ToolCallRequestPayload, ToolCallResponsePayload, etc.
  - Event types: `run.started`, `run.completed`, `run.toolcall.{requested,approved,denied,result}`
  - WS events: `run.status`, `run.toolcall`
- [x] (2026-02-17) Database: `runs` table (migration 005, FK to tasks/agents/projects, optimistic locking)
  - Store interface: CreateRun, GetRun, UpdateRunStatus, CompleteRun, ListRunsByTask
- [x] (2026-02-17) RuntimeService: `internal/service/runtime.go`
  - StartRun, HandleToolCallRequest, HandleToolCallResult, HandleRunComplete, CancelRun
  - Termination enforcement: MaxSteps, MaxCost, Timeout checked per tool call
  - Policy evaluation per tool call (reuses Phase 4A policy layer)
  - Quality gates logged (enforcement deferred to 4C)
  - NATS subscribers for 4 subjects, cancel cleanup
- [x] (2026-02-17) REST API: 4 new endpoints
  - `POST /api/v1/runs` — start a run
  - `GET /api/v1/runs/{id}` — get run details
  - `POST /api/v1/runs/{id}/cancel` — cancel a run
  - `GET /api/v1/tasks/{id}/runs` — list runs for a task
- [x] (2026-02-17) Python RuntimeClient: `workers/codeforge/runtime.py`
  - request_tool_call, report_tool_result, complete_run, send_output
  - Cancel listener, step/cost tracking
  - Consumer extended with `runs.start` subscription
  - Executor extended with `execute_with_runtime()` method
- [x] (2026-02-17) Tests: 44 new test functions across Go + Python
  - `internal/domain/run/run_test.go` (15), `internal/service/runtime_test.go` (22)
  - `internal/adapter/http/handlers_test.go` (+5), `workers/tests/test_runtime.py` (9)
- [ ] Implement Execution Modes (actual Docker sandbox, mount, hybrid)
  - Sandbox: Isolated Docker container with cgroups v2 limits
  - Mount: Direct file access to host workspace
  - Hybrid: Read from host, write in sandbox, merge on success
- [ ] Runtime Compliance Tests
  - Test suite that validates each Runtime implementation
  - Feature parity checks across Sandbox/Mount/Hybrid

### 4C. Headless Autonomy (Server-First Execution) (COMPLETED)

- [x] (2026-02-17) CI Fix: golangci-lint v2 config migration (local-prefixes array, removed v1 options)
- [x] (2026-02-17) Config extension: `config.Runtime` struct with 6 fields (StallThreshold, QualityGateTimeout, DefaultDeliverMode, DefaultTestCommand, DefaultLintCommand, DeliveryCommitPrefix) + ENV overrides + YAML example
- [x] (2026-02-17) Stall Detection: `internal/domain/run/stall.go` (StallTracker with FNV-64a hash ring buffer), 10 domain tests
  - Progress tools (Edit, Write, Bash with success) reset counter; non-progress tools increment
  - Repetition detection via output hash ring buffer (size 3)
  - Configurable threshold from policy `StallThreshold` or `config.Runtime.StallThreshold`
  - Integrated into `HandleToolCallResult` — terminates run on stall
- [x] (2026-02-17) Quality Gate Enforcement: NATS request/result protocol (Go → Python → Go)
  - New status `StatusQualityGate` for transient gate-check state
  - `HandleRunComplete` triggers gate request when policy has `RequireTestsPass`/`RequireLintPass`
  - `HandleQualityGateResult` processes outcomes: pass → deliver → finalize; fail + rollback → fail
  - Python `QualityGateExecutor` runs test/lint commands via `asyncio.create_subprocess_shell` with timeout
  - Consumer extended with `runs.qualitygate.request` subscription
  - 7 Python tests for quality gate executor
- [x] (2026-02-17) Deliver Modes: 5 strategies (none, patch, commit-local, branch, pr)
  - Domain types: `DeliverMode` in `run.go`, migration `006_add_deliver_mode.sql`
  - `DeliverService` in `internal/service/deliver.go` using git CLI + `gh` for PRs
  - Graceful fallback: PR → branch-only if `gh` unavailable; push failure non-fatal
  - 5 delivery tests (none, patch, commit-local, branch, no-workspace)
- [x] (2026-02-17) Frontend: RunPanel component, Run types/API client, WS event integration
  - `RunPanel.tsx`: start form (task/agent/policy/deliver), active run display, run history
  - `ProjectDetailPage.tsx`: agents resource, RunPanel integration, WS cases for run/QG/delivery events
  - API: `runs.start/get/cancel/listByTask`, `policies.list`
- [x] (2026-02-17) Events: 7 new event types (QG started/passed/failed, delivery started/completed/failed, stall detected)
- [x] (2026-02-17) WS: `run.qualitygate` and `run.delivery` events with typed structs
- [ ] Implement Checkpoint system
  - Before every `Edit/Write/Replace`: automatic checkpoint
  - On failed Quality Gates (tests/lint): automatic rewind or re-plan
  - Shadow Git approach for Mount mode, Blob snapshots for Sandbox

---

## Phase 5 — Multi-Agent Orchestration

> Source: Analyse-Dokument Section "Multi-Agent Orchestration Architecture"

### 5A. Execution Plans — DAG Scheduling with 4 Protocols (COMPLETED)

- [x] (2026-02-17) Domain model: `internal/domain/plan/` (plan.go, validate.go, dag.go)
  - ExecutionPlan, Step, Protocol, Status, StepStatus, CreatePlanRequest
  - DAG cycle detection (Kahn's algorithm), ReadySteps, RunningCount, AllTerminal, AnyFailed
  - 25 domain tests (16 validation + 8 DAG + 1 compile check)
- [x] (2026-02-17) Config: `config.Orchestrator` (MaxParallel, PingPongMaxRounds, ConsensusQuorum) + ENV overrides
- [x] (2026-02-17) Database: migration 007 (execution_plans + plan_steps tables with UUID arrays)
- [x] (2026-02-17) Store interface: 9 new methods + Postgres adapter (transactional CreatePlan)
- [x] (2026-02-17) Events: 5 plan event types + 2 WS event types
- [x] (2026-02-17) RuntimeService callback: SetOnRunComplete + invocation in finalizeRun
- [x] (2026-02-17) OrchestratorService: 4 protocol handlers (sequential, parallel, ping_pong, consensus)
  - CreatePlan, StartPlan, GetPlan, ListPlans, CancelPlan, HandleRunCompleted
- [x] (2026-02-17) REST API: 5 endpoints for plan management
- [x] (2026-02-17) Frontend: PlanPanel.tsx, plan types/API client, WS integration in ProjectDetailPage
- [x] (2026-02-17) Tests: 12 orchestrator service tests, all passing

### 5B. Orchestrator Agent (Meta-Agent) (COMPLETED)

- [x] (2026-02-17) Orchestrator Mode: `manual`, `semi_auto`, `full_auto` — config, env overrides, domain types
- [x] (2026-02-17) LLM-based feature decomposition via LiteLLM ChatCompletion (Go → LiteLLM Proxy)
- [x] (2026-02-17) MetaAgentService: prompt engineering, JSON parsing, task creation, agent selection
- [x] (2026-02-17) Agent strategy selection (single, pair, team) with hint-based agent matching
- [x] (2026-02-17) Auto-start: `full_auto` mode or `auto_start` override starts plan immediately
- [x] (2026-02-17) REST API: `POST /api/v1/projects/{id}/decompose`
- [x] (2026-02-17) Frontend: Decompose Feature form in PlanPanel with context/model/auto-start options
- [x] (2026-02-17) Tests: 4 litellm client tests, 5 domain tests, 9 meta-agent service tests — all passing

### 5C. Task Decomposition (Context-Optimized Planning)

- [ ] Task Planner Service: `internal/service/task_planner.go`
  - Feature → context-optimized subtasks
  - Analyze complexity, identify relevant files
  - Estimate context size, split if > threshold
  - Build dependency graph (DAG)
  - Assign strategies: single/pair/team based on heuristics
  - LLM-based intelligent splitting (with fallback to file-based)

### 5C. Agent Teams (Collaboration Units)

- [ ] Domain model: `internal/domain/agent_team.go`
  - AgentTeam: members, protocol, shared context, NATS message bus
  - TeamMember: AgentID, Role (coder/reviewer/tester/documenter/planner)
  - TeamProtocol: `sequential`, `ping_pong`, `consensus`, `parallel`
  - SharedContext: files (versioned), artifacts, decisions, conversation
- [ ] Agent Pool Manager: `internal/service/agent_pool_manager.go`
  - Check resource availability before spawning
  - Spawn teams for tasks with correct roles
  - Lifecycle management (initialize → running → completed/failed)
  - Cleanup: terminate team on failure or completion

### 5D. Context Optimizer

- [ ] Token budget management per task
  - Estimate tokens needed for each file set
  - Split tasks that exceed context window
  - Prioritize most relevant files (by dependency graph)
- [ ] Context packing as structured artifacts
  - Store Context Packs in session events (not ad-hoc text)
  - Reproducible retrieval via schema/artifact

---

## Phase 6 — Code-RAG (Context Engine for Large Codebases)

> Source: Analyse-Dokument Section 14/5, "RAG am Anfang"
> Three-tier approach: RepoMap → Hybrid Retrieval → GraphRAG (later)

### 6A. Repo Map (High ROI, Phase 3-4)

- [ ] tree-sitter based Repo Map
  - Parse all project files, extract symbols/signatures
  - Compact overview: files + key symbols (functions, classes, types)
  - Python Worker: use tree-sitter bindings
  - Output as structured data (JSON) for agent context
  - Inspired by Aider's `repomap.py`

### 6B. Hybrid Retrieval (Phase 4)

- [ ] Keyword/regex search tools (grep, BM25)
  - Fast directory search with concise result listing
  - Strict token budget for search results (avoid context pollution)
- [ ] Embedding Search for semantic queries
  - Use docs-mcp or dedicated embedding service
  - Top-K results as "Context Pack" with token budget
- [ ] Combine: Hybrid Retrieval = keyword + semantic, ranked

### 6C. Retrieval Sub-Agent (Phase 5)

- [ ] Dedicated search agent for context retrieval
  - Orchestrator delegates "find context" to Retrieval Agent
  - Parallel tool calls, many searches, compact result
  - Returns structured Context Pack to requesting agent

### 6D. GraphRAG (Phase 6+)

- [ ] Vector entry points + Graph traversal
  - Call graph, import graph, ownership relationships
  - Use where architecture relationships matter
  - Careful with context explosion (graph expansion)
- [ ] Graph DB integration (Neo4j or similar)

---

## Phase 7+ — Advanced Features & Vision

### Roadmap/Feature Map

- [ ] Roadmap/Feature Map Editor (Auto-Detection, Multi-Format SDD)
- [ ] OpenSpec/Spec Kit/Autospec integration
- [ ] Bidirectional PM sync (Plane.so, OpenProject, GitHub/GitLab Issues)

### Version Control

- [ ] SVN integration (provider registry pattern)
- [ ] Gitea/Forgejo support (GitHub adapter works with minimal changes)

### Protocols

- [ ] A2A protocol integration (agent discovery, task delegation, Agent Cards)
- [ ] AG-UI protocol integration (agent ↔ frontend streaming, replace custom WS events)

### Integrations

- [ ] GitHub/GitLab Webhook system for external integrations
- [ ] Webhook notifications (Slack, Discord)

### Cost & Monitoring

- [ ] Cost tracking dashboard for LLM usage
- [ ] Real-time budget alerts via WebSocket
- [ ] Distributed tracing (OpenTelemetry full implementation)

### Operations

- [ ] Backup & disaster recovery strategy
  - `scripts/backup-postgres.sh` (pg_dump daily at 3 AM UTC)
  - Retention: 7 daily, 4 weekly, 12 monthly
- [ ] Blue-Green deployment support (Traefik labels)
- [ ] Multi-tenancy / user management (full, beyond soft-launch)

### CI/CD

- [ ] GitHub Actions workflow: build Docker images
- [ ] Branch protection rules for `main`

---

## Documentation TODOs

- [ ] Create ADR for Config Hierarchy (`docs/architecture/adr/003-config-hierarchy.md`)
- [ ] Create ADR for Async Logging (`docs/architecture/adr/004-async-logging.md`)
- [ ] Create ADR for Docker-Native Logging (`docs/architecture/adr/005-docker-native-logging.md`)
- [ ] Create ADR for Agent Execution (Approach C) (`docs/architecture/adr/006-agent-execution-approach-c.md`)
- [ ] Create ADR for Policy Layer (`docs/architecture/adr/007-policy-layer.md`)
- [ ] Update `docs/architecture.md` with new patterns
  - Event Sourcing, Circuit Breaker, Cache Layer, Idempotency, Rate Limiting
  - Policy Layer, Runtime API, Execution Modes
- [ ] Update `docs/dev-setup.md` with logging section
  - Docker Compose log commands, log level config, Request ID, helper script
- [ ] Update `CLAUDE.md` with new principles
  - Config hierarchy rule, async-first concurrency
  - Docker-native logging, no external monitoring tools
  - Policy Layer, Approach C decision

---

## Testing Requirements

- [x] (2026-02-17) Test runner script (`scripts/test.sh`) — unified Go/Python/Frontend/Integration runner
- [x] (2026-02-17) Integration test infrastructure (`tests/integration/`) — real PostgreSQL, build-tagged
  - Health/liveness tests, Project CRUD lifecycle, Task CRUD lifecycle, validation tests
  - Fixed goose migration `$$` blocks (StatementBegin/StatementEnd annotations)
  - Updated `.claude/commands/test.md` to use test runner script
- [ ] Unit tests for AsyncHandler (buffer overflow, concurrent writes, flush)
- [ ] Integration tests for Config Loader (precedence, validation, reload)
- [ ] Integration tests for Idempotency (duplicate requests, TTL expiry)
- [ ] Load tests for Rate Limiting (sustained vs burst, per-user limiters)
- [ ] Runtime Compliance Tests (Sandbox/Mount/Hybrid feature parity)
- [x] (2026-02-17) Policy Gate tests (deny/ask/allow evaluation, path scoping, command matching, preset integration)

---

## Recently Completed

> Move items here after completion for context. Periodically archive old items.

- [x] (2026-02-14) Phase 2 completed: MVP Features
  - WP1: Git Local Provider — Clone, Status, Pull, ListBranches, Checkout via git CLI
  - WP2: Agent Lifecycle — Aider backend, async NATS dispatch, agent CRUD API
  - WP3: WebSocket Events — Live agent output, task/agent status broadcasting
  - WP4: LLM Provider Management — LiteLLM admin API client, model CRUD endpoints
  - WP5: Frontend — Project detail page, git operations UI, task list
  - WP6: Frontend — Agent monitor panel, live terminal output, task create/expand
  - WP7: Frontend — LLM models page, add/delete models, health status
  - WP8: Integration test, documentation update, test fixes
  - **Go:** 27 tests, gitlocal provider, aider backend, agent service, LiteLLM client, 19 REST endpoints
  - **Python:** 16 tests, streaming output via NATS, LiteLLM health checks
  - **Frontend:** 13 components, 4 routes (/, /projects, /projects/:id, /models), WebSocket live updates
- [x] (2026-02-14) Phase 1 completed: Infrastructure, Go Core, Python Workers, Frontend, CI/CD
  - Docker Compose: PostgreSQL, NATS JetStream, LiteLLM Proxy
  - Go: Hexagonal architecture, REST API, WebSocket, NATS, PostgreSQL
  - Python: NATS consumer, LiteLLM client, Pydantic models, 16 tests
  - Frontend: SolidJS dashboard, API client, WebSocket, health indicators
  - CI: GitHub Actions (Go + Python + Frontend)
- [x] (2026-02-14) Phase 0 completed: devcontainer, Go scaffold, Python Workers, SolidJS frontend
- [x] (2026-02-14) Protocol support decided: MCP, LSP, OpenTelemetry (Tier 1), A2A, AG-UI (Tier 2)
- [x] (2026-02-14) Library decisions: chi (router), coder/websocket (WS), git exec wrapper, SolidJS minimal stack
- [x] (2026-02-14) PostgreSQL 17 chosen as database — [ADR-002](architecture/adr/002-postgresql-database.md)
- [x] (2026-02-14) NATS JetStream chosen as message queue — [ADR-001](architecture/adr/001-nats-jetstream-message-queue.md)
- [x] (2026-02-14) Documentation structure created (docs/README.md, docs/todo.md, feature specs)
- [x] (2026-02-14) Architecture harmony audit: all docs synchronized
- [x] (2026-02-14) All documentation translated from German to English
- [x] (2026-02-14) Coding agent insights integrated into architecture.md

For full completion history, see [project-status.md](project-status.md).

---

## Notes

- **Priority order**: Phase 3 (Reliability) → Phase 4 (Agent Engine) → Phase 5 (Multi-Agent) → Phase 6 (RAG)
- **Dependencies**: Structured Logging → Request ID → Docker Logging → Log Script
- **Dependencies**: Event Sourcing → Policy Layer → Runtime API → Headless Autonomy
- **Dependencies**: Repo Map → Hybrid Retrieval → Retrieval Sub-Agent → GraphRAG
- **Testing**: Each new pattern requires unit + integration tests before merge
- **Documentation**: ADRs must be written before implementation (capture decision context)
- **Source**: Analysis document `docs/Analyse des CodeForge-Projekts (staging-Branch).md`
