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
- [x] (2026-02-17) Async logging for Go Core
  - `internal/logger/async.go`: AsyncHandler wrapping slog.Handler with buffered channel (10,000) + 4 worker goroutines
  - Non-blocking drop policy with atomic dropped counter
  - `Close()` flushes remaining records, `WithAttrs`/`WithGroup` share channel
  - `logger.go`: `New()` returns `(*slog.Logger, Closer)`, wraps with AsyncHandler when `cfg.Async == true`
  - 4 tests in `internal/logger/async_test.go`
- [x] (2026-02-17) Python async logging with QueueHandler
  - `workers/codeforge/logger.py`: `QueueHandler` + `QueueListener` with 10,000-record buffer
  - `stop_logging()` for graceful shutdown with queue drain
  - 2 tests in `workers/tests/test_logger.py`
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
- [x] (2026-02-18) Agent execution timeout & heartbeat
  - [x] Timeout enforcement exists: `checkTermination()` in `runtime.go` checks elapsed time per tool call
  - [x] Cancellation via `POST /api/v1/runs/{id}/cancel` + NATS `runs.cancel` subject
  - [x] (2026-02-18) Heartbeat ticker (30s) for progress tracking during long tool calls
    - Go: `heartbeats` sync.Map, heartbeat subscriber in StartSubscribers, timeout check in checkTermination
    - Python: `start_heartbeat()`/`stop_heartbeat()` on RuntimeClient, called from executor
    - Config: `HeartbeatInterval` (30s), `HeartbeatTimeout` (120s) with ENV overrides
  - [x] (2026-02-18) Context-level timeout (wrapping entire run, not just tool-call boundaries)
    - Goroutine-based timer per run, tracked via `runTimeouts` sync.Map of context.CancelFunc
    - Auto-cancels run when TerminationConfig.TimeoutSeconds expires
- [x] (2026-02-17) Idempotency Keys for critical operations
  - `internal/middleware/idempotency.go`: HTTP middleware for POST/PUT/DELETE deduplication
  - NATS JetStream KV storage (24h TTL) via `KeyValue()` method in `internal/adapter/nats/nats.go`
  - `Idempotency-Key` header: KV hit → replay cached response; miss → tee-capture + store
  - 5 tests in `internal/middleware/idempotency_test.go`
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
- [x] (2026-02-18) Event store enhancements: RunID association + LoadByRun query
  - Migration 013: `run_id UUID` column + index on `agent_events`
  - Domain: `RunID` field on `AgentEvent`, set in `appendRunEvent`
  - Port: `LoadByRun` method on `eventstore.Store` interface
  - Postgres adapter: `run_id` in INSERT/SELECT for all methods + `LoadByRun`
  - HTTP: `GET /api/v1/runs/{id}/events` handler + route
  - Handlers: `Events eventstore.Store` field wired in main.go
- [ ] Features: Replay task, trajectory inspector, audit trail
- [ ] Session Events as Source of Truth (append-only log for Resume/Fork/Rewind)
  - Every user/model/tool action recorded as event
  - Stream events via WebSocket/AG-UI to frontend
  - Event-schema versioning from day one

### 3E. Performance Optimizations

- [x] (2026-02-17) Cache Layer (tiered L1 + L2)
  - Port: `internal/port/cache/cache.go` (Get/Set/Delete interface)
  - L1: `internal/adapter/ristretto/cache.go` (in-process, configurable max cost)
  - L2: `internal/adapter/natskv/cache.go` (NATS JetStream KV)
  - Tiered: `internal/adapter/tiered/cache.go` (L1 → L2 with backfill on L2 hit)
  - Config: `L1MaxSizeMB` (100), `L2Bucket` ("CACHE"), `L2TTL` (10m)
  - 5 tests in `internal/adapter/tiered/cache_test.go`
  - Dependency: `github.com/dgraph-io/ristretto/v2`
- [x] (2026-02-17) Database connection pool tuning
  - `NewPool` accepts `config.Postgres` with MaxConns=15, MinConns=2, MaxConnLifetime=1h, MaxConnIdleTime=10m, HealthCheckPeriod=1m
  - All pool parameters configurable via YAML/ENV
- [x] (2026-02-17) Rate limiting per IP
  - `internal/middleware/ratelimit.go`: token bucket, per-IP, configurable rate/burst
  - Headers: `X-RateLimit-Remaining`, `X-RateLimit-Reset` (GitHub-style)
  - Returns 429 with `Retry-After` header
  - 4 tests in `internal/middleware/ratelimit_test.go`
- [x] (2026-02-18) Go Core: Worker pools for CPU-bound tasks
  - Git operations wrapped via `golang.org/x/sync/semaphore` with configurable limit (default: 5)
  - Context propagation for cancellation via `semaphore.Acquire(ctx, 1)`
  - `internal/git/pool.go`: Pool struct with nil-safe `Run()` method
  - Injected into gitlocal.Provider, CheckpointService, DeliverService
  - 4 tests in `internal/git/pool_test.go`
- [x] (2026-02-18) Python Workers: Full asyncio adoption
  - NATS consumer: fully async (`await nats.connect()`, `await js.subscribe()`, `asyncio.gather()`)
  - LiteLLM calls: async via `httpx.AsyncClient` (completion, health, embeddings)
  - Quality gate: `asyncio.create_subprocess_shell()` with timeout
  - DB queries: N/A — Python workers do not access PostgreSQL (by design: Go is control plane)

### 3F. Security & Isolation

- [x] (2026-02-17) Agent sandbox resource limits
  - Shared type: `internal/domain/resource/limits.go` (Limits struct, Merge, Cap)
  - Agent domain: `ResourceLimits *resource.Limits` field on Agent struct
  - Policy domain: `ResourceLimits *resource.Limits` field on PolicyProfile struct
  - Store/handler chain updated: CreateAgent accepts `*resource.Limits`, stored as JSONB
  - Migration: `012_add_agent_resource_limits.sql`
  - Sandbox merge logic: global config → policy limits → agent limits → cap at ceiling
  - 5 tests in `internal/domain/resource/limits_test.go`
- [x] (2026-02-18) Secrets vault with SIGHUP hot reload
  - `internal/secrets/vault.go`: Vault struct with RWMutex, Get/Reload methods
  - `internal/secrets/env_loader.go`: EnvLoader factory for env-var-based secrets
  - SIGHUP handler in `cmd/codeforge/main.go` triggers `vault.Reload()`
  - LiteLLM client: `SetVault()` injection, `masterKey()` reads from vault
  - 4 tests in `internal/secrets/vault_test.go`

### 3G. API & Health

- [x] (2026-02-17) Health check granularity
  - `/health` liveness (always 200), `/health/ready` readiness (pings DB, NATS, LiteLLM)
  - Returns 503 with per-service status + latency if any dependency down
  - NATS `IsConnected()` added to Queue interface
- [x] (2026-02-18) API deprecation middleware
  - `internal/middleware/deprecation.go`: `Deprecation(sunset)` adds RFC 8594 headers
  - `Deprecation: true` + `Sunset: <HTTP-date>` headers on deprecated routes
  - 3 tests in `internal/middleware/deprecation_test.go`
  - Ready for use when `/api/v2` is introduced (no sunset date set yet)
- [x] (2026-02-18) Database migrations: Rollback capability
  - All 15 migrations have `-- +goose Up` and `-- +goose Down` sections
  - `RollbackMigrations()` and `MigrationVersion()` exposed in `internal/adapter/postgres/postgres.go`
  - Integration test `TestMigrationUpDown` in `tests/integration/migration_test.go`
  - `./scripts/test.sh migrations` sub-command for CI rollback verification

### 3H. Multi-Tenancy Preparation (Soft Launch)

- [x] (2026-02-18) Add tenant_id to all tables
  - Migration 014: `tenant_id UUID NOT NULL DEFAULT '00000000-...'` on 7 tables
  - Indexes on projects, tasks, agents, runs
  - `internal/middleware/tenant.go`: TenantID middleware (X-Tenant-ID header, default fallback)
  - Context helpers: `TenantIDFromContext()`, `WithTenantID()`
  - Domain structs: `TenantID` field added to Project, Task, Agent, Run
  - Middleware chain: `r.Use(middleware.TenantID)` after RequestID
  - 3 tests in `internal/middleware/tenant_test.go`
  - Deferred: full WHERE tenant_id clauses in queries (single-tenant for now)

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
- [x] (2026-02-17) Implement Checkpoint system
  - `internal/service/checkpoint.go`: CheckpointService with shadow Git commits
  - CreateCheckpoint: `git add -A && git commit` for file-modifying tools (Edit, Write, Bash)
  - RewindToFirst: `git reset --hard {firstHash}^` (restore pre-run state on quality gate failure)
  - RewindToLast: `git reset --hard {lastHash}^` (undo last change only)
  - CleanupCheckpoints: `git reset --soft {firstHash}^` (remove shadow commits, keep working state)
  - Runtime integration: checkpoint on tool call, rewind on rollback, cleanup on finalize/cancel
  - 5 tests in `internal/service/checkpoint_test.go`
- [x] (2026-02-18) Policy UI in Frontend
  - Backend: CRUD endpoints (POST /policies, DELETE /policies/{name}) with YAML persistence
  - Backend: SaveProfile, DeleteProfile methods on PolicyService (preset protection)
  - Backend: SaveToFile in policy loader, IsPreset helper in presets
  - Frontend: Full type definitions (PolicyProfile, PermissionRule, QualityGate, etc.)
  - Frontend: Extended API client (get, create, delete, evaluate)
  - Frontend: PolicyPanel component with 3 views (list, detail with evaluate tester, editor)
  - Frontend: Integrated into ProjectDetailPage (between agents and run management)
  - Tests: 6 new service tests, 6 new handler tests — all passing
  - Deferred: Scope levels (global → project → run), run overrides, "why matched" explanation

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
- [x] (2026-02-17) Implement Execution Modes — Docker Sandbox
  - `internal/service/sandbox.go`: SandboxService with Docker CLI (os/exec)
  - Create (docker create with resource flags), Start, Exec, Stop, Remove, Get
  - Config: `SandboxConfig` in `internal/config/config.go` (memory, cpu, pids, storage, network, image)
  - Domain: `ExecModeHybrid` constant added to `internal/domain/run/run.go`
  - Runtime integration: sandbox lifecycle in StartRun, finalizeRun, CancelRun
  - 5 tests in `internal/service/sandbox_test.go`
  - Mount mode: implicit — Python worker operates directly on host filesystem (no additional Go code needed)
  - Deferred: Hybrid mode (read from host, write in sandbox, merge on success)
- [x] (2026-02-18) Runtime Compliance Tests
  - `internal/service/runtime_compliance_test.go`: 8 sub-tests × 2 modes (Mount, Sandbox)
  - Sub-tests: StartRun, ToolCallFlow, PolicyEnforcement, Termination_MaxSteps, Termination_MaxCost, CancelRun, Completion, StallDetection
  - All 16 compliance test cases passing

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
- [x] (2026-02-17) Checkpoint system — see 4A above

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

### 5C. Agent Teams + Context-Optimized Planning (COMPLETED)

- [x] (2026-02-17) Domain model: `internal/domain/agent/team.go`
  - AgentTeam: members, protocol, status, version
  - TeamMember: AgentID, Role (coder/reviewer/tester/documenter/planner)
  - TeamProtocol: reuses plan.Protocol (sequential, ping_pong, consensus, parallel)
  - CreateTeamRequest validation: name, project_id, roles, no duplicates
  - 8 domain tests in `team_test.go`
- [x] (2026-02-17) Database: migration 008 (agent_teams + team_members)
- [x] (2026-02-17) Store interface: 5 new methods + Postgres adapter
- [x] (2026-02-17) Config: `max_team_size` in Orchestrator (default 5)
- [x] (2026-02-17) PoolManagerService: `internal/service/pool_manager.go`
  - CreateTeam, AssembleTeamForStrategy, CleanupTeam, GetTeam, ListTeams, DeleteTeam
  - Resource availability checks (agent exists, idle, same project)
  - 8 service tests
- [x] (2026-02-17) TaskPlannerService: `internal/service/task_planner.go`
  - PlanFeature: context enrichment (workspace file tree) → LLM decompose → optional auto-team
  - Complexity heuristic (single/pair/team based on step count)
  - 3 service tests
- [x] (2026-02-17) REST API: 5 new endpoints (team CRUD + plan-feature)
- [x] (2026-02-17) Frontend: team types, API client (teams namespace + planFeature)
- [x] (2026-02-17) SharedContext: versioned team-level shared state (done in 5D)
- [x] (2026-02-17) NATS message bus for context updates (done in 5D)

### 5D. Context Optimizer (COMPLETED)

- [x] (2026-02-17) Token budget management per task
  - ContextPack domain model with token budget + entries
  - EstimateTokens heuristic (len/4), ScoreFileRelevance keyword matching
  - Configurable budget (default_context_budget) and prompt reserve
- [x] (2026-02-17) Context packing as structured artifacts
  - ContextOptimizerService: scan workspace → score → pack within budget → persist
  - SharedContextService: team-level shared state with NATS notifications
  - Pre-packed context injected into RunStartPayload for Python workers
  - 4 new REST endpoints (task context CRUD, shared context CRUD)
  - 26+ new test functions (Go domain + service + Python), all passing

### 5E. Integration Fixes, WS Events, Modes System (COMPLETED)

- [x] (2026-02-17) Fix TeamID propagation (Run, ExecutionPlan, StartRequest)
  - Migration 010: team_id on runs + execution_plans, output on runs
  - Orchestrator → Runtime → ContextOptimizer TeamID flow fixed
- [x] (2026-02-17) Auto-init SharedContext on team creation (PoolManager)
- [x] (2026-02-17) Auto-populate SharedContext from run outputs (Orchestrator)
- [x] (2026-02-17) WS events: team.status, shared.updated (events.go + broadcasts)
- [x] (2026-02-17) Modes System: domain model, 8 presets, ModeService, 3 REST endpoints
- [x] (2026-02-17) Frontend: Mode/CreateModeRequest types, modes API namespace
- [x] (2026-02-17) Mock stores + test fixes (CompleteRun signature, nil-safe hub)

---

## Phase 6 — Code-RAG (Context Engine for Large Codebases)

> Source: Analyse-Dokument Section 14/5, "RAG am Anfang"
> Three-tier approach: RepoMap → Hybrid Retrieval → GraphRAG (later)

### 6A. Repo Map (COMPLETED)

- [x] (2026-02-17) tree-sitter based Repo Map
  - Python Worker: RepoMapGenerator with tree-sitter + tree-sitter-language-pack
  - Symbol extraction (functions, classes, methods, types, interfaces) for 16+ languages
  - File ranking via networkx PageRank (import graph analysis)
  - Go Backend: domain model, PostgreSQL store, RepoMapService, REST API, WS events
  - Frontend: RepoMapPanel component with stats, language tags, collapsible map text
  - NATS integration: repomap.generate / repomap.result subjects
  - New dependencies: tree-sitter ^0.24, tree-sitter-language-pack ^0.13, networkx ^3.4

### 6B. Hybrid Retrieval — BM25 + Semantic Search (COMPLETED)

- [x] (2026-02-17) Python Worker: HybridRetriever with BM25S + LiteLLM embeddings
  - AST-aware code chunking via tree-sitter (reuse from 6A)
  - BM25S keyword indexing (500x faster than rank_bm25)
  - Semantic embeddings via LiteLLM proxy `/v1/embeddings`
  - Reciprocal Rank Fusion (RRF) with k=60 to combine rankings
  - In-memory per-project indexes (no vector DB)
  - Shared constants extracted to `_tree_sitter_common.py`
- [x] (2026-02-17) Go Backend: RetrievalService with synchronous search
  - NATS subjects: retrieval.index.{request,result}, retrieval.search.{request,result}
  - Channel-based waiter pattern with correlation IDs for sync search (30s timeout)
  - REST API: POST /projects/{id}/index, GET /projects/{id}/index, POST /projects/{id}/search
  - WS event: retrieval.status (building/ready/error)
  - Context optimizer auto-injects hybrid results as EntryHybrid
- [x] (2026-02-17) Frontend: RetrievalPanel component
  - Index status display (file count, chunk count, embedding model)
  - Build Index / Search UI with results display
  - Integrated into ProjectDetailPage with WS event handler
- [x] (2026-02-17) New dependencies: bm25s ^0.2, numpy ^2.0
- [x] (2026-02-17) Tests: 11 Python tests (chunking, RRF, cosine similarity, integration), 5 Go tests (service), 3 Go handler tests — all passing

### 6C. Retrieval Sub-Agent (COMPLETED)

- [x] (2026-02-18) LLM-guided multi-query retrieval sub-agent
  - Python: `RetrievalSubAgent` class — expand queries via LLM, parallel hybrid search, dedup, LLM re-rank
  - Python: `SubAgentSearchRequest` / `SubAgentSearchResult` Pydantic models
  - Python: Consumer handler for `retrieval.subagent.request` NATS subject
  - Go: `SubAgentSearchSync` / `HandleSubAgentSearchResult` on RetrievalService (60s timeout, correlation ID waiter)
  - Go: NATS subjects `retrieval.subagent.request` / `retrieval.subagent.result` + payload types
  - Go: Config fields `SubAgentModel`, `SubAgentMaxQueries`, `SubAgentRerank` with ENV overrides
  - Go: Context optimizer `fetchRetrievalEntries()` — sub-agent first, single-shot fallback
  - Go: HTTP handler `POST /projects/{id}/search/agent`
  - Frontend: Agent/Standard search toggle in RetrievalPanel, expanded queries display
  - Tests: 8 Python tests (unit + integration), 3 new Go service tests — all passing
- [x] (2026-02-18) Code review refinements (16 changes across architecture, quality, tests, performance)
  - Generic `syncWaiter[T]` replacing duplicate waiter patterns (DRY)
  - Health tracking with 30s cooldown fast-fail on worker failures
  - Percentile-based priority normalization (RRF scores mapped to 60-85 range)
  - Shared retrieval deadline (sub-agent + single-shot fallback under one timeout)
  - Parallel workspace scan + retrieval in BuildContextPack
  - Check-before-build guard to prevent redundant context pack builds
  - Defense-in-depth validation (Pydantic validators + Go handler clamping)
  - Unified `SearchResult` dataclass into `RetrievalSearchHit` Pydantic model
  - DRY `_publish_error_result()` helper in consumer error handling
  - Pre-built rank dict for O(1) BM25 lookup, per_query_k = top_k fix
  - 5 new tests (3 Python, 2 Go) — all 77 Python + all Go tests passing

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

- [x] (2026-02-18) Phase 7: Cost & Token Transparency — Full implementation
  - [x] Feature 1: Real cost calculation in Python workers
    - LiteLLM `x-litellm-response-cost` header extraction (`workers/codeforge/llm.py`)
    - Fallback pricing table (`configs/model_pricing.yaml`, `workers/codeforge/pricing.py`)
    - Real cost passed to runtime (`workers/codeforge/executor.py`)
    - 7 new Python tests (pricing + LLM cost extraction)
  - [x] Feature 2: Token persistence in database
    - Migration 015: `tokens_in`, `tokens_out`, `model` columns on runs table
    - Domain fields: `TokensIn`, `TokensOut`, `Model` on `run.Run`
    - Store, NATS payloads, Python models, RuntimeService all updated
    - Python runtime: token accumulators (`_total_tokens_in`, `_total_tokens_out`, `_model`)
  - [x] Feature 3: Cost aggregation API (5 endpoints)
    - Domain: `internal/domain/cost/cost.go` (Summary, ProjectSummary, ModelSummary, DailyCost)
    - Store: 5 SQL aggregation queries (global, per-project, per-model, time-series, recent runs)
    - Service: `internal/service/cost.go` (CostService)
    - HTTP: `GET /costs`, `GET /projects/{id}/costs`, `GET /projects/{id}/costs/by-model`, `GET /projects/{id}/costs/daily`, `GET /projects/{id}/costs/runs`
  - [x] Feature 4: WS budget alerts + token events
    - `RunStatusEvent` extended with `tokens_in`, `tokens_out`, `model`
    - `BudgetAlertEvent` type: fires at 80% and 90% of MaxCost
    - Dedup via `sync.Map` keyed by `"runID:threshold"`
    - 3 budget alert tests (80%, 90%, no-duplicate)
  - [x] Feature 5: Frontend cost dashboard + enhancements
    - Types: `CostSummary`, `ProjectCostSummary`, `ModelCostSummary`, `DailyCost`, `BudgetAlertEvent`
    - API client: `api.costs` namespace (5 methods)
    - `CostDashboardPage`: global totals, project breakdown table
    - `ProjectCostSection`: per-project cost cards, model breakdown, daily cost bars, recent runs
    - Route: `/costs` with nav link
    - RunPanel: tokens + model display in active run and history
    - ProjectDetailPage: budget alert banner + cost section
- [ ] Distributed tracing (OpenTelemetry full implementation)

### Operations

- [x] (2026-02-18) Backup & disaster recovery strategy
  - `scripts/backup-postgres.sh` — pg_dump with custom format, compression, retention cleanup
  - `scripts/restore-postgres.sh` — restore from file or latest, drop-and-recreate
  - Docker Compose WAL config (`wal_level=replica`, `archive_mode=on`) for future PITR
  - Documented in `docs/dev-setup.md` (backup, restore, cron, WAL archiving)
- [ ] Blue-Green deployment support (Traefik labels)
- [ ] Multi-tenancy / user management (full, beyond soft-launch)

### CI/CD & Tooling

- [x] (2026-02-18) Pre-commit & linting hardening — security analysis, anti-pattern detection, import sorting
  - Python: 10 ruff rule groups (S, C4, C90, PERF, PIE, RET, FURB, LOG, T20, PT), mccabe threshold 12
  - Go: 8 new golangci-lint linters (gosec, bodyclose, noctx, errorlint, revive, fatcontext, dupword, durationcheck)
  - TypeScript: strict + stylistic ESLint configs, eslint-plugin-simple-import-sort
  - Pre-commit: ruff-pre-commit bumped to v0.15.1, ESLint hook fixed for local binary
  - All violations fixed across 31 files
- [ ] GitHub Actions workflow: build Docker images
- [ ] Branch protection rules for `main`

---

## Documentation TODOs

- [x] (2026-02-18) Create ADR for Config Hierarchy (`docs/architecture/adr/003-config-hierarchy.md`)
- [x] (2026-02-18) Create ADR for Async Logging (`docs/architecture/adr/004-async-logging.md`)
- [x] (2026-02-18) Create ADR for Docker-Native Logging (`docs/architecture/adr/005-docker-native-logging.md`)
- [x] (2026-02-18) Create ADR for Agent Execution (Approach C) (`docs/architecture/adr/006-agent-execution-approach-c.md`)
- [x] (2026-02-18) Create ADR for Policy Layer (`docs/architecture/adr/007-policy-layer.md`)
- [x] (2026-02-18) Update `docs/architecture.md` with new patterns
  - Infrastructure Patterns section: Circuit Breaker, Cache Layer, Idempotency, Rate Limiting
  - Agent Execution section: Policy Layer, Runtime API, Checkpoint System, Docker Sandbox
  - Observability section: Event Sourcing, Structured Logging, Configuration
- [x] (2026-02-18) Update `docs/dev-setup.md` with logging section
  - Docker Compose log commands, log level config, Request ID propagation, helper script, log rotation
- [x] (2026-02-18) Update `CLAUDE.md` with new principles
  - ADR index, Infrastructure Principles section
  - Config hierarchy, async-first concurrency, Docker-native logging, Policy Layer, Approach C, resilience patterns

---

## Testing Requirements

- [x] (2026-02-17) Test runner script (`scripts/test.sh`) — unified Go/Python/Frontend/Integration runner
- [x] (2026-02-17) Integration test infrastructure (`tests/integration/`) — real PostgreSQL, build-tagged
  - Health/liveness tests, Project CRUD lifecycle, Task CRUD lifecycle, validation tests
  - Fixed goose migration `$$` blocks (StatementBegin/StatementEnd annotations)
  - Updated `.claude/commands/test.md` to use test runner script
- [x] (2026-02-17) Unit tests for AsyncHandler (buffer overflow, concurrent writes, flush) — 4 tests in `internal/logger/async_test.go`
- [ ] Integration tests for Config Loader (precedence, validation, reload)
- [x] (2026-02-17) Unit tests for Idempotency (no header, store, replay, GET ignored, different keys) — 5 tests in `internal/middleware/idempotency_test.go`
- [ ] Load tests for Rate Limiting (sustained vs burst, per-user limiters)
- [x] (2026-02-18) Runtime Compliance Tests (Sandbox/Mount feature parity) — 16 sub-tests passing
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
