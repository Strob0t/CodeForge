# CodeForge — TODO Tracker

> **LLM Agents: This is your primary task reference.**
> Always read this file before starting work to understand current priorities.

## How to Use This File

- **Before starting work:** Read this file to understand what needs to be done
- **After completing a task:** Mark it `[x]`, add completion date, move to "Recently Completed" if needed
- **When discovering new work:** Add items to the appropriate section with context
- **Format:** `- [ ]` for open/pending, `- [x]` for done (with date)
- **Cross-reference:** Link to feature docs, architecture.md sections, or issues where relevant

---

## Phases 0–8: Complete

> All phases from 0 through 8 are complete. See sections below for details.
> **Current priority: Phase 10 (Frontend Foundations)** — then Phase 9 (Advanced) and Phase 11+ (Future GUI).

### Phase 3 — Reliability, Performance & Agent Foundation

### 3A. Configuration Management

- [x] (2026-02-17) Implement hierarchical config system (defaults < YAML < ENV)
  - Go: `internal/config/config.go` (typed Config struct), `internal/config/loader.go` (Load with merge logic)
  - Python: `workers/codeforge/config.py` (WorkerSettings from ENV)
  - Validation layer for required fields and min values
  - `codeforge.yaml.example` with all fields documented
  - 6 test functions in `internal/config/loader_test.go`
- [x] (2026-02-19) SIGHUP config reload (ConfigHolder with hot-reload, expanded SIGHUP handler)
  - `internal/config/config.go`: `ConfigHolder` struct with `sync.RWMutex`, `Get()`, `Reload()`
  - `internal/config/loader.go`: `LoadFrom(yamlPath)` for explicit YAML path loading
  - `cmd/codeforge/main.go`: SIGHUP handler now reloads both config and secrets vault
  - Warns about non-hot-reloadable fields (port, DSN, NATS URL)
- [x] (2026-02-19) CLI override support
  - `internal/config/loader.go`: `CLIFlags` struct with `*string` pointer fields (nil = unset)
  - `ParseFlags(args)`: stdlib `flag.FlagSet` parser, supports `--config/-c`, `--port/-p`, `--log-level`, `--dsn`, `--nats-url`
  - `LoadWithCLI(flags)`: full hierarchy defaults < YAML < ENV < CLI, returns resolved YAML path
  - `applyCLI()`: applies non-nil flag overrides after ENV
  - `cmd/codeforge/main.go`: parses `os.Args[1:]`, passes resolved YAML path to `ConfigHolder`
  - 8 new tests in `internal/config/loader_test.go` (parse, shorthand, invalid, apply, nil, override, custom config)

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
- [x] (2026-02-19) PostgreSQL log_line_prefix configuration
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
- [x] (2026-02-18) Trajectory API: cursor-paginated LoadTrajectory, TrajectoryStats, 2 REST endpoints, TrajectoryPanel frontend
- [x] (2026-02-19) Replay / Audit Trail — ReplayService (ListCheckpoints, Replay, AuditTrail, RecordAudit), LoadEventsRange + ListCheckpoints on eventstore, audit_trail table (migration 021), REST endpoints (checkpoints, replay, audit)
- [x] (2026-02-19) Session Events (Resume/Fork/Rewind) — Session entity, SessionService (Resume, Fork, Rewind, CRUD), sessions table (migration 021), 6 session event types, REST endpoints (resume, fork, rewind, list sessions, get session)

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
- [x] (2026-02-19) Full WHERE tenant_id clauses in all queries + tenant CRUD
  - Migration 018: `tenants` table, `tenant_id UUID` on 11 remaining tables
  - `internal/domain/tenant/tenant.go`: Tenant struct, CreateRequest, UpdateRequest
  - `internal/service/tenant.go`: TenantService (CRUD + ValidateExists)
  - `internal/adapter/postgres/store_tenant.go`: tenant CRUD + `tenantFromCtx()` helper
  - `internal/adapter/postgres/store.go`: ALL ~60 methods updated with `AND tenant_id = $N`
  - `internal/adapter/postgres/eventstore.go`: all 6 methods updated with tenant_id filtering
  - REST API: `POST/GET /api/v1/tenants`, `GET/PUT /api/v1/tenants/{id}`
  - `cmd/codeforge/main.go`: TenantService wired into handlers

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
- [x] (2026-02-19) Policy scope levels (global → project → run), run overrides, "why matched" explanation
  - `internal/domain/policy/evaluation.go`: Scope type (global/project/run), EvaluationResult struct
  - `internal/service/policy.go`: `EvaluateWithReason()`, `ResolveProfile()`, `evaluateWithReason()`
  - `internal/service/runtime.go`: uses EvaluateWithReason, logs evaluation reason at Debug level
  - `internal/adapter/http/handlers.go`: evaluate endpoint returns full EvaluationResult
  - Migration 019: `policy_profile` column on projects table
  - `internal/domain/project/project.go`: PolicyProfile field on Project struct

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
- [x] (2026-02-19) Hybrid execution mode — workspace mounted read-write, commands execute inside Docker container
  - `internal/service/sandbox.go`: `CreateHybrid()` method (read-write mount, no `--read-only`, `sleep infinity`)
  - `internal/service/runtime.go`: hybrid mode handling in `StartRun` (switch on ExecMode), `sendToolCallResponse` includes `exec_mode` + `container_id`
  - `internal/config/config.go`: `HybridConfig{CommandImage, MountMode}` sub-struct on Runtime
  - `internal/config/loader.go`: env overrides `CODEFORGE_HYBRID_IMAGE`, `CODEFORGE_HYBRID_MOUNT_MODE`
  - `internal/port/messagequeue/schemas.go`: `exec_mode`, `container_id` fields on `ToolCallResponsePayload`
  - Refactored Start/Stop/Exec/Remove to use ContainerID instead of regenerating names
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

### 6D. GraphRAG — Structural Code Graph Intelligence (COMPLETED)

- [x] (2026-02-18) PostgreSQL adjacency-list graph (no Neo4j — single-DB architecture)
  - Migration 016: `graph_nodes`, `graph_edges`, `graph_metadata` tables
  - Nodes: function/class/method/module definitions via tree-sitter
  - Edges: import edges (Python/Go/TS/JS), call edges (name-matching heuristic)
- [x] (2026-02-18) Python: CodeGraphBuilder + GraphSearcher (`workers/codeforge/graphrag.py`)
  - Build pipeline: walk files → tree-sitter parse → extract definitions + imports + calls → batch upsert to PostgreSQL
  - BFS search with hop-decay scoring (default decay 0.7): hop 0 = 1.0, hop 1 = 0.7, hop 2 = 0.49
  - Bidirectional traversal (outgoing + incoming edges)
  - Edge path tracking for explainability
  - 19 tests in `test_graphrag.py` — all passing
- [x] (2026-02-18) Python consumer: 2 new NATS handlers (`graph.build.request`, `graph.search.request`)
- [x] (2026-02-18) Go: GraphService (`internal/service/graph.go`)
  - Follows RetrievalService pattern: syncWaiter, health tracking, WS broadcasts
  - RequestBuild, HandleBuildResult, SearchSync, HandleSearchResult, GetStatus, StartSubscribers
- [x] (2026-02-18) Go: 4 NATS subjects + 5 payload types in schemas.go
- [x] (2026-02-18) Go: 4 config fields (GraphEnabled, GraphMaxHops, GraphTopK, GraphHopDecay)
- [x] (2026-02-18) Go: Context optimizer integration — `fetchGraphEntries()` uses retrieval hits as seed symbols
  - Priority: 70 - (distance * 10) → hop 0 = 70, hop 1 = 60, hop 2 = 50
  - Guarded by GraphEnabled + graph status "ready"
- [x] (2026-02-18) Go: 3 HTTP endpoints (`POST /graph/build`, `GET /graph/status`, `POST /graph/search`)
- [x] (2026-02-18) Go: EntryGraph kind in domain context, WS GraphStatusEvent
- [x] (2026-02-18) Frontend: GraphStatus/GraphSearchHit/GraphSearchResult types, API client, RetrievalPanel graph section
- [x] (2026-02-18) All linting passes: golangci-lint, ruff, ESLint, prettier, pre-commit

---

## Phase 8 — Roadmap Foundation, Event Trajectory, Docker Production

### 8A. Roadmap/Feature-Map Foundation (Pillar 2)

- [x] (2026-02-18) Domain models: `internal/domain/roadmap/` (Roadmap, Milestone, Feature, statuses, validation)
  - Roadmap/Milestone statuses: draft, active, complete, archived
  - Feature statuses: backlog, planned, in_progress, done, cancelled
  - Optimistic locking (Version field), tenant_id, labels ([]string), external_ids (map)
  - 18 domain tests in `roadmap_test.go`
- [x] (2026-02-18) Migration 017: `roadmaps`, `milestones`, `features` tables
  - Unique idx on project_id, sort_order indexes, TEXT[] labels, JSONB external_ids
  - Reuses existing `update_updated_at()` and `increment_version()` trigger functions
- [x] (2026-02-18) Port interfaces: `specprovider/` (SpecProvider + Registry), `pmprovider/` (PMProvider + Registry)
  - Follows gitprovider pattern: self-registering via `init()`, capability declarations
- [x] (2026-02-18) Store interface: 16 new methods on `database.Store` (Roadmap/Milestone/Feature CRUD)
- [x] (2026-02-18) Postgres adapter: 16 method implementations with optimistic locking
- [x] (2026-02-18) RoadmapService: CRUD delegation, AutoDetect (file markers), AIView (json/yaml/markdown)
  - Broadcasts `roadmap.status` WS event on mutations
- [x] (2026-02-18) REST API: 12 roadmap endpoints
  - GET/POST/PUT/DELETE /projects/{id}/roadmap
  - GET /projects/{id}/roadmap/ai, POST /projects/{id}/roadmap/detect
  - POST /projects/{id}/roadmap/milestones
  - GET/PUT/DELETE /milestones/{id}
  - POST /milestones/{id}/features, GET/PUT/DELETE /features/{id}
- [x] (2026-02-18) WS event: `roadmap.status` with RoadmapStatusEvent struct
- [x] (2026-02-18) Frontend: RoadmapPanel.tsx — milestone/feature tree, create/edit forms, auto-detect, AI view
- [x] (2026-02-18) main.go wiring: RoadmapService creation + Handlers struct field

### 8B. Event Trajectory API + Frontend

- [x] (2026-02-18) Event store extension: TrajectoryFilter, TrajectoryPage, TrajectorySummary types
  - Cursor-paginated LoadTrajectory with type/time filtering
  - TrajectoryStats with SQL aggregates (event counts, duration, tool calls, errors)
- [x] (2026-02-18) Postgres implementation: dynamic WHERE clause builder, cursor pagination, aggregate stats
- [x] (2026-02-18) REST API: 2 trajectory endpoints
  - GET /runs/{id}/trajectory (?types=...&after=...&before=...&cursor=...&limit=50)
  - GET /runs/{id}/trajectory/export (?format=json, Content-Disposition: attachment)
- [x] (2026-02-18) Frontend: TrajectoryPanel.tsx — vertical timeline, event type filters, stats summary, export

### 8C. Docker/CI for Production

- [x] (2026-02-18) Dockerfile (Go Core): multi-stage golang:1.24-alpine → alpine:3.21 with git+ca-certs
- [x] (2026-02-18) Dockerfile.worker (Python): python:3.12-slim, poetry install --only main, non-root user
- [x] (2026-02-18) Dockerfile.frontend: node:22-alpine build → nginx:alpine serve with SPA routing + API proxy
- [x] (2026-02-18) frontend/nginx.conf: try_files for SPA, proxy_pass to core:8080 for /api/ and /ws
- [x] (2026-02-18) .dockerignore: exclude .venv, node_modules, .git, data/, __pycache__
- [x] (2026-02-18) docker-compose.prod.yml: 6 services (core, worker, frontend, postgres, nats, litellm)
  - Named volumes, health checks, restart: unless-stopped, tuned PostgreSQL (256MB shared_buffers)
- [x] (2026-02-18) .github/workflows/docker-build.yml: 3 parallel jobs (core, worker, frontend)
  - ghcr.io push, branch/semver/sha tags, Docker layer cache

---

## Phase 9+ — Advanced Features & Vision

### Roadmap/Feature Map (Advanced)

- [x] (2026-02-18) OpenSpec adapter (`internal/adapter/openspec/`) — detect, list, read specs from `openspec/` directory
- [x] (2026-02-18) Markdown spec adapter (`internal/adapter/markdownspec/`) — detect, list, read `ROADMAP.md` / `roadmap.md`
- [x] (2026-02-18) GitHub Issues PM adapter (`internal/adapter/githubpm/`) — list/get issues via `gh` CLI
- [x] (2026-02-18) Enhanced AutoDetect — provider-based detection with hardcoded fallback for uncovered formats
- [x] (2026-02-18) Spec import (`ImportSpecs`) — discover specs via providers, create milestones/features
- [x] (2026-02-18) PM import (`ImportPMItems`) — import work items from PM providers into roadmap
- [x] (2026-02-18) 4 new REST endpoints: import specs, import PM items, list spec providers, list PM providers
- [x] (2026-02-18) Frontend: import UI in RoadmapPanel (Import Specs button, Import from PM form, result display)
- [x] (2026-02-19) Spec Kit adapter (`adapter/speckit/`) — `.specify/` directory
- [x] (2026-02-19) Autospec adapter (`adapter/autospec/`) — `specs/spec.yaml` file
- [x] (2026-02-19) Bidirectional PM sync — SyncService with pull/push/bidi directions, CreateItem/UpdateItem on pmprovider interface, REST endpoint `POST /projects/{id}/roadmap/sync`
- [x] (2026-02-19) Webhook-based real-time VCS sync — VCSWebhookService for GitHub/GitLab push + PR events, HMAC-SHA256 verification middleware, REST endpoints `POST /webhooks/vcs/github`, `POST /webhooks/vcs/gitlab`

### Version Control

- [x] (2026-02-19) SVN integration — svn adapter (`internal/adapter/svn/`) implementing gitprovider.Provider via svn CLI (checkout, update, status, info, ls branches)
- [x] (2026-02-19) Gitea/Forgejo support — gitea PM adapter (`internal/adapter/gitea/`) implementing full pmprovider.Provider via REST API (list, get, create, update issues)

### Protocols

- [x] (2026-02-19) A2A protocol stub (agent discovery via `/.well-known/agent.json`, task create/get via `/a2a/tasks`, AgentCard with 2 skills)
- [x] (2026-02-19) AG-UI protocol event types (8 event types: run_started, run_finished, text_message, tool_call, tool_result, state_delta, step_started, step_finished)

### Integrations

- [x] (2026-02-19) GitHub/GitLab VCS Webhook system — VCSWebhookService processes push/PR events, broadcasts via WebSocket, HMAC-SHA256 signature verification
- [x] (2026-02-19) Webhook notifications (Slack, Discord) — NotificationService fan-out, Slack Block Kit adapter, Discord embed adapter, self-registering via notifier registry
- [x] (2026-02-19) PM Webhook Sync — PMWebhookService for GitHub Issues, GitLab Issues, Plane.so webhook events, normalized to PMWebhookEvent

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
- [x] (2026-02-19) OpenTelemetry — real TracerProvider + MeterProvider with OTLP gRPC exporters, HTTP middleware, span helpers, metric instruments

### Operations

- [x] (2026-02-18) Backup & disaster recovery strategy
  - `scripts/backup-postgres.sh` — pg_dump with custom format, compression, retention cleanup
  - `scripts/restore-postgres.sh` — restore from file or latest, drop-and-recreate
  - Docker Compose WAL config (`wal_level=replica`, `archive_mode=on`) for future PITR
  - Documented in `docs/dev-setup.md` (backup, restore, cron, WAL archiving)
- [x] (2026-02-19) Blue-Green deployment — Traefik config, docker-compose overlay, deploy script with health checks and rollback
- [x] (2026-02-19) Multi-tenancy (full, beyond soft-launch) — see 3H for details

### CI/CD & Tooling

- [x] (2026-02-18) Pre-commit & linting hardening — security analysis, anti-pattern detection, import sorting
  - Python: 10 ruff rule groups (S, C4, C90, PERF, PIE, RET, FURB, LOG, T20, PT), mccabe threshold 12
  - Go: 8 new golangci-lint linters (gosec, bodyclose, noctx, errorlint, revive, fatcontext, dupword, durationcheck)
  - TypeScript: strict + stylistic ESLint configs, eslint-plugin-simple-import-sort
  - Pre-commit: ruff-pre-commit bumped to v0.15.1, ESLint hook fixed for local binary
  - All violations fixed across 31 files
- [x] (2026-02-18) GitHub Actions workflow: build Docker images (3 parallel jobs, ghcr.io, layer cache)
- [x] (2026-02-19) Branch protection rules for `main` (GitHub)
  - `scripts/setup-branch-protection.sh`: Configurable via `gh` CLI
  - Requires PR with 1 approving review, dismiss stale reviews
  - Required status checks: Go, Python, Frontend (CI jobs)
  - Require up-to-date branches, linear history, conversation resolution
  - No force pushes or branch deletion
- [x] (2026-02-19) Branch protection rules (domain feature) — ProtectionRule model with glob pattern matching, EvaluatePush/EvaluateMerge/EvaluateDelete, CRUD REST API, migration 020, tenant-aware store methods, optimistic locking

---

## Phase 10 — Frontend Foundations (Cross-Cutting)

> These foundations must be built BEFORE adding more feature UIs.
> They affect every component and are cheaper to retrofit now than later.

### 10A. Theme System (Dark/Light Mode Toggle)

- [x] (2026-02-19) Define CSS custom properties for color tokens (bg-primary, bg-surface, text-primary, border, etc.)
  - `:root` and `.dark` blocks in `index.css` with 17 design tokens each
  - Surface, border, text, accent, and agent status tokens
- [x] (2026-02-19) Implement Tailwind `dark:` variants on all existing components (~18 files)
  - `@custom-variant dark (&:where(.dark, .dark *));` for Tailwind CSS v4 class-based dark mode
  - All components: App, Dashboard, ProjectCard, CostDashboard, ModelsPage, AgentPanel, TaskPanel, LiveOutput, RunPanel, PlanPanel, PolicyPanel, ProjectDetailPage, RepoMapPanel, RetrievalPanel, RoadmapPanel, TrajectoryPanel, Toast
- [x] (2026-02-19) Theme toggle component in Sidebar
  - `ThemeProvider.tsx`: ThemeProvider context + ThemeToggle button (sun/moon/gear icons)
  - Cycles through light -> dark -> system
- [x] (2026-02-19) localStorage persistence for user preference (key: `codeforge-theme`)
- [x] (2026-02-19) System preference detection via `prefers-color-scheme` media query
  - `window.matchMedia` listener for real-time system preference changes
- [x] (2026-02-19) Formalize 5-color agent status schema as design tokens
  - `--cf-status-running` (blue), `--cf-status-idle` (green), `--cf-status-waiting` (yellow), `--cf-status-error` (red), `--cf-status-planning` (blue)
- [x] (2026-02-19) Customization hooks for branding (CSS custom property overrides in `:root`)

### 10B. i18n (Internationalization)

- [x] (2026-02-19) Evaluate i18n approach: custom signal-based context provider (no external dependency)
  - `I18nProvider` with `createContext`/`useContext`, `createSignal` for locale + translations
  - Flat dot-separated keys (`TranslationKey = keyof typeof en`), type-safe at compile time
  - `{{variable}}` interpolation via `String.replaceAll()`
  - Lazy bundle loading via dynamic `import()` — Vite auto code-splits non-English bundles
  - `localStorage` persistence (key: `codeforge-locale`), browser language auto-detection
- [x] (2026-02-19) Extract all hardcoded UI strings from ~20 components into key-based bundles (~480 keys)
  - Shell components: App.tsx, ThemeProvider, OfflineBanner, CommandPalette, Toast
  - Dashboard: DashboardPage, ProjectCard
  - Costs: CostDashboardPage
  - Models: ModelsPage
  - Project panels: AgentPanel, TaskPanel, RunPanel, PlanPanel, LiveOutput, TrajectoryPanel
  - Project detail: ProjectDetailPage, PolicyPanel, RepoMapPanel, RetrievalPanel, RoadmapPanel
- [x] (2026-02-19) Language bundles: English (default) + German (code-split, 19 kB chunk)
  - `frontend/src/i18n/en.ts` — ~480 translation keys, source of truth
  - `frontend/src/i18n/locales/de.ts` — full German translation
- [x] (2026-02-19) Language switcher in Sidebar (LocaleSwitcher component, cycles EN/DE)
- [x] (2026-02-19) Locale-aware date/number formatting (Intl.DateTimeFormat, Intl.NumberFormat)
  - `frontend/src/i18n/formatters.ts`: 9 formatter functions (date, dateTime, time, number, compact, currency, duration, score, percent)
  - `frontend/src/i18n/context.tsx`: `fmt` object on I18nContextValue, reactive locale binding
  - Replaced inline formatters in 8 components: ProjectCard, CostDashboardPage, TaskPanel, RunPanel, RepoMapPanel, RetrievalPanel, TrajectoryPanel, ProjectDetailPage
  - Removed 4 duplicate local formatters (formatDate, formatNumber x2, formatCost x2, formatTokens)
- [x] (2026-02-19) Pluralization support via `tp()` function using `Intl.PluralRules`
  - `tp(key, count, params?)` resolves `key_one`, `key_other` etc. based on CLDR plural rules
  - `Intl.PluralRules` instances cached per locale for performance
  - Count automatically injected as `{{count}}` parameter
  - 12 plural keys added to en.ts: costs.runs, costs.steps, run.steps, trajectory.events/toolCalls/errors (_one/_other)
  - 12 plural keys added to de.ts (matching)
  - Updated 3 components: CostDashboardPage (2 locations), RunPanel (2 locations), TrajectoryPanel (3 locations)

### 10C. Authentication & Authorization

- [x] (2026-02-19) Auth strategy: JWT (HS256 via stdlib crypto) + httpOnly refresh cookie
  - Access token in JS memory signal (15min), refresh token in httpOnly Secure SameSite=Strict cookie (7d)
  - Auth disabled by default (`auth.enabled: false`), default admin context injected for backward compatibility
- [x] (2026-02-19) Go Core: Auth middleware (after RequestID, before TenantID in chain)
  - `internal/middleware/auth.go`: JWT/API key validation, public path exemption (/health, /ws, /auth/login, /auth/refresh)
  - `internal/middleware/rbac.go`: `RequireRole(roles...)` middleware factory
  - Modified `internal/middleware/tenant.go`: extracts tenant from user claims, fallback to X-Tenant-ID header
- [x] (2026-02-19) User domain model (`internal/domain/user/`)
  - `user.go`: User struct, Role (admin/editor/viewer), CreateRequest, LoginRequest, LoginResponse, TokenClaims, Validate()
  - `apikey.go`: APIKey struct, CreateAPIKeyRequest, CreateAPIKeyResponse, `cfk_` prefix
- [x] (2026-02-19) Database migration: `022_create_users_api_keys.sql`
  - `users` table (bcrypt password hash, email+tenant_id unique), `refresh_tokens`, `api_keys` (SHA-256 key hash)
- [x] (2026-02-19) Role-Based Access Control: admin, editor, viewer
  - Admin-only routes: /api/v1/users/* (CRUD), RequireRole middleware
- [x] (2026-02-19) API Key support for headless/CI access (`X-API-Key` header)
  - SHA-256 hashed in DB, `cfk_` prefix, checked before Bearer token in middleware
- [x] (2026-02-19) Auth service (`internal/service/auth.go`)
  - Register, Login, RefreshTokens (rotation), Logout, ValidateAccessToken, ValidateAPIKey
  - CreateAPIKey, ListAPIKeys, DeleteAPIKey, User CRUD, SeedDefaultAdmin
  - JWT HS256 via stdlib `crypto/hmac` + `crypto/sha256` (no third-party JWT library)
- [x] (2026-02-19) Store interface + Postgres implementation
  - 14 new methods in `internal/port/database/store.go` (User 6, RefreshToken 4, APIKey 4)
  - `store_user.go`, `store_refresh_token.go`, `store_api_key.go`
- [x] (2026-02-19) HTTP handlers (`internal/adapter/http/handlers_auth.go`)
  - 11 handler methods: Login, Refresh, Logout, GetCurrentUser, CRUD API keys, CRUD users (admin)
  - Routes in `routes.go`, CORS headers updated for auth
- [x] (2026-02-19) Config: `internal/config/config.go` Auth struct, 7 CODEFORGE_AUTH_* env overrides
- [x] (2026-02-19) Frontend: Auth Context provider, protected route guard
  - `AuthProvider.tsx`: SolidJS context with signals, login/logout, auto-refresh scheduling, session restore
  - `RouteGuard.tsx`: redirect to /login if not authenticated
  - `RoleGate.tsx`: conditional render by role
- [x] (2026-02-19) Frontend: Login page, logout flow, session refresh
  - `frontend/src/features/auth/LoginPage.tsx`: email/password form, error handling, redirect
  - `frontend/src/api/client.ts`: setAccessTokenGetter, Authorization header, credentials: "include"
- [x] (2026-02-19) Frontend: Show current user in Sidebar, role-based UI element visibility
  - `App.tsx`: AuthProvider in provider chain, UserInfo component in sidebar
- [x] (2026-02-19) i18n: ~27 auth keys in en.ts and de.ts
- [x] (2026-02-19) Tests: domain validation, auth service (7 tests), auth middleware (4 tests), RBAC middleware (4 tests)
- [x] (2026-02-19) Connects to existing Multi-Tenancy (Phase 3H tenant_id)

### 10D. WCAG 2.2 Conformance (Level AA)

- [x] (2026-02-19) Audit all existing components against WCAG 2.2 AA criteria
- [x] (2026-02-19) `aria-label` / `aria-labelledby` on all interactive elements (buttons, inputs, selects)
- [x] (2026-02-19) Focus management: visible focus rings (`:focus-visible`), focus trap in modals/dialogs
- [x] (2026-02-19) Keyboard navigation: correct Tab order, Enter/Space for buttons, Escape for close
- [x] (2026-02-19) Color contrast: minimum 4.5:1 ratio for all text/background combinations (depends on 10A Theme)
- [x] (2026-02-19) Screen reader support: landmark regions (`<main>`, `<nav>`, `<aside>`), `aria-live` for real-time updates
- [x] (2026-02-19) Skip-to-content link as first focusable element
- [x] (2026-02-19) Form accessibility: explicit `<label>` associations, error announcements, required field indicators
- [x] (2026-02-19) Motion: `prefers-reduced-motion` media query for animations
- [x] (2026-02-19) Test with axe-core / Playwright accessibility audit in E2E tests
  - `@axe-core/playwright` integrated as devDependency
  - `frontend/e2e/a11y.spec.ts`: WCAG 2.2 AA checks on Dashboard, Costs, Models, Login pages
  - Tags: `wcag2a`, `wcag2aa`, `wcag22aa`

### 10E. Keyboard Shortcuts (Command Palette)

- [x] (2026-02-19) Global hotkey handler: Ctrl+K / Cmd+K for Command Palette overlay
- [x] (2026-02-19) Navigation shortcuts: Ctrl+1 Dashboard, Ctrl+2 Costs, Ctrl+3 Models
- [x] (2026-02-19) Action shortcuts: Escape close, Enter select, Arrow keys navigate
- [x] (2026-02-19) Shortcut help overlay: Ctrl+/ opens palette with all shortcuts visible
- [x] (2026-02-19) Zero external dependencies — SolidJS `onMount`/`onCleanup` + native KeyboardEvent
- [x] (2026-02-19) Files: `App.tsx` (integration), new `frontend/src/components/CommandPalette.tsx`
- [x] (2026-02-19) WCAG: `role="dialog"`, `aria-modal`, `role="combobox"` + `role="listbox"` + `role="option"`, `aria-activedescendant`, `aria-selected`
  - Commands: Go to Dashboard, Go to Costs, Go to Models, Toggle Theme, Show Keyboard Shortcuts
  - Fuzzy search filtering, section grouping (Navigation, Actions, Theme)
  - Platform-aware modifier key display (Cmd on Mac, Ctrl elsewhere)

### 10F. Toast/Notification System

- [x] (2026-02-19) Global toast container (top-right, max 3 simultaneous, stacked)
- [x] (2026-02-19) Toast types: success (green), error (red), warning (yellow), info (blue)
- [x] (2026-02-19) Auto-dismiss (5s default), manually closable
- [x] (2026-02-19) Zero external dependencies — SolidJS `createSignal` + `Portal`
- [x] (2026-02-19) Integration points: API errors, WebSocket events (budget alerts, run complete, agent status)
- [x] (2026-02-19) WCAG: `aria-live="polite"` for info/success, `role="alert"` for errors
- [x] (2026-02-19) Files: new `frontend/src/components/Toast.tsx`, `App.tsx` (provider)
- [x] (2026-02-19) Wire toasts into all 7 API-calling panels
  - DashboardPage: project create/delete success/error
  - ProjectDetailPage: budget alerts (warning), run/plan completion (info), git operations (success/error)
  - RunPanel: run start/cancel success/error
  - AgentPanel: agent create/delete/dispatch/stop success/error
  - RoadmapPanel: roadmap create/delete, import specs/PM, milestone/feature create success/error
  - TaskPanel: task create success/error
  - ModelsPage: model add/delete success/error

### 10G. Error Boundary + Offline Detection

- [x] (2026-02-19) SolidJS `ErrorBoundary` around App root with fallback UI + retry button
- [x] (2026-02-19) Online/Offline detection: `navigator.onLine` + WebSocket connection status
- [x] (2026-02-19) Visible reconnect banner when connection lost (WebSocket already has auto-reconnect)
- [x] (2026-02-19) API client: retry logic with exponential backoff (max 3 retries, 1s/2s/4s)
- [x] (2026-02-19) Graceful degradation: show cached data when offline, queue actions for retry
  - `frontend/src/api/cache.ts`: in-memory GET response cache (5min TTL), offline mutation queue with auto-process on reconnect
  - `frontend/src/api/client.ts`: GET responses cached on success, served from cache on network failure; mutations queued when offline, auto-retried via `window.addEventListener("online")`
- [x] (2026-02-19) Files: `App.tsx`, `api/client.ts`, new `frontend/src/components/OfflineBanner.tsx`

### Dependencies

```
10C Auth ──────────────────────┐
10A Theme ─── 10D WCAG ────────┤
10B i18n  ─────────────────────┤──> All further feature UIs
10F Toast ─── 10G Error ───────┤
10E Keyboard ──────────────────┘
```

---

## Phase 11+ — Future GUI Enhancements

> Items from the "Glass Cockpit" GUI design review.
> These are valuable but too early for the current project phase.
> Prerequisites: Phase 10 foundations + working multi-agent orchestration.

### Visual Enhancements (requires graph rendering library)

- [x] (2026-02-19) Architecture graph / pulsating project blueprint visualization
  - `ArchitectureGraph` SVG component on Context tab, queries GraphRAG search API
  - Force-directed layout with repulsion, attraction, center gravity, animated convergence
  - Nodes colored by kind (module/class/function/method), sized by importance
  - Hover highlights connected edges, labels appear, non-connected nodes dim
  - Seed symbol input, configurable hop depth, raw results collapsible
  - 18 new i18n keys (EN + DE)
- [x] (2026-02-19) Agent network visualization (nodes + edges, message flow animation)
  - `AgentNetwork` SVG component on Agents tab, circle layout from team members
  - Nodes colored by role (coder/reviewer/tester/documenter/planner) with status ring
  - WS event listener animates message flow between agents (pulse + arrow)
  - Team selector, role legend, message flow log
  - 11 new i18n keys (EN + DE)
- [x] (2026-02-19) Step-progress indicators replacing time estimates
  - `StepProgress` reusable component: progress bar + fraction label (current/max)
  - Color coding: blue (<70%), yellow (70-90%), red (>90%), indeterminate when max unknown
  - ARIA progressbar role with aria-valuenow/max, locale-aware labels
  - RunPanel: shows progress bar during active runs (max from policy termination.max_steps)
  - PlanPanel: shows progress bar for running plans (completed steps / total steps)
  - 5 i18n keys (EN + DE)

### Advanced Layouts

- [x] (2026-02-19) Split-screen feature planning (prompt left + generated plan preview right)
  - PlanPanel decompose form stores result in signal instead of discarding
  - Responsive split layout: single column on small screens, side-by-side on lg+
  - Right panel shows plan name, description, protocol, step list with dependencies
  - Accept/discard buttons to confirm or re-try the decomposition
  - 8 new i18n keys (EN + DE) for plan preview
- [x] (2026-02-19) ProjectDetailPage tab navigation (Overview | Tasks & Roadmap | Agents & Runs | Context | Costs)
  - 5 tabs replace vertically stacked layout
  - Tab bar with `role="tablist"`, `aria-selected`, `aria-controls` for a11y
  - i18n keys: `detail.tab.overview`, `.tasks`, `.agents`, `.context`, `.costs` (EN + DE)
  - Overview: Git status, branches, clone/pull actions
  - Tasks & Roadmap: TaskPanel + RoadmapPanel
  - Agents & Runs: AgentPanel + PolicyPanel + RunPanel + PlanPanel + LiveOutput
  - Context: RepoMapPanel + RetrievalPanel (requires workspace)
  - Costs: ProjectCostSection
- [x] (2026-02-19) Multi-terminal view with tiles per agent
  - `MultiTerminal` component with per-agent terminal tiles in responsive grid
  - Auto-scroll, expand/collapse single tile to full width, max line truncation
  - ProjectDetailPage tracks output per agent_id, shows MultiTerminal when 2+ agents active
  - Falls back to single LiveOutput when only one agent has output
  - 7 new i18n keys (EN + DE) for multi-terminal
- [x] (2026-02-19) Global activity/notification stream (cross-project, not just per-project)
  - `ActivityPage` component at `/activity` with global WebSocket subscription
  - Classifies 12+ WS event types (run.status, run.toolcall, run.budget_alert, run.qualitygate, run.delivery, agent.status, task.status, plan.status, plan.step.status, repomap/retrieval/roadmap.status)
  - Severity-tagged entries (info/success/warning/error) with color-coded badges
  - Type icons for visual differentiation, filter by event type, pause/resume/clear
  - Max 200 entries (newest first), project links, ARIA role="log" + aria-live="polite"
  - 12 i18n keys (EN + DE), sidebar nav link

### Developer Tools

- [x] (2026-02-19) Vector search simulator / "What does the agent know?" debug tool
  - `SearchSimulator` component on Context tab, uses hybrid + agent + graph search APIs
  - Adjustable BM25/semantic weight sliders, top-K, token budget
  - Token budget progress bar with per-result token estimation and budget fit indicator
  - Agent mode toggle (query expansion), optional GraphRAG cross-reference
  - Results colored green/red based on budget fit, BM25 + semantic rank columns
  - 28 new i18n keys (EN + DE) for simulator
- [x] (2026-02-19) Diff-review / code preview for agent output (before/after comparison)
  - `DiffPreview` reusable component: parses unified diff into files/hunks/lines
  - Color-coded: green (additions), red (removals), blue (hunk headers), gray (context)
  - Line numbers (old/new), collapsible per-file sections, +/- counts per file
  - Auto-detected in TrajectoryPanel EventDetail: checks .diff, .patch, .output for diff content
  - Also renders in non-tool events (e.g. delivery events with patch content)
  - 2 i18n keys (EN + DE)
- [x] (2026-02-19) Trajectory replay / inspector with step-by-step playback
  - Replay mode toggle in TrajectoryPanel: scrubber bar, play/pause, step prev/next
  - 4 playback speeds (0.5x, 1x, 2x, 4x) with cycle button
  - Mini timeline dots colored by event type, highlighting played events
  - Enhanced EventDetail component: structured tool call/result display (tool name, input, output, errors)
  - Browse mode (existing) enhanced with blue highlight on expanded events
  - 13 new i18n keys (EN + DE), ARIA labels for all controls

### Missing UI for Existing Backend Features

- [x] (2026-02-19) Settings/Configuration page
  - Provider info cards (Git, Agent, Spec, PM) with loading/empty states
  - LLM health status indicator (connected/unavailable/checking)
  - API key management: create, list, delete, copy warning for new keys
  - User management table (admin only): enable/disable, delete, role badges
  - ~38 i18n keys in EN + DE, route `/settings`, nav link in sidebar
  - Reuses existing backend endpoints: providers.*, auth.*, users.*, llm.health
- [x] (2026-02-19) Mode selection UI (architect, coder, reviewer, debugger, etc.)
  - ModesPage at `/modes` with card grid for all 8 built-in + custom modes
  - Create custom mode form (id, name, description, tools, scenario, autonomy, prompt)
  - Mode cards: tool badges, LLM scenario, autonomy level with color coding, expandable prompt
  - Built-in modes protected from overwrite, sorted before custom
  - ~35 i18n keys (EN + DE), sidebar nav link, route wired
- [x] (2026-02-19) Team/Multi-Agent management UI — backend ready via `api.teams`
  - TeamsPage at `/teams` with project selector, team creation form, and team list
  - Create team: name, protocol (round-robin/pipeline/parallel/consensus/ping-pong), member assignment (agent + role)
  - Team cards: status badge, protocol badge, member count, expandable detail with member list and shared context
  - Shared context viewer: key/value items with author and token count
  - 5 team roles: coder, reviewer, tester, documenter, planner (color-coded badges)
  - ~30 i18n keys (EN + DE), sidebar nav link, route wired

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

- [x] (2026-02-18) E2E test infrastructure (Playwright)
  - Playwright config, 5 test files (17 tests), fixtures with API helper
  - Tests: health checks, sidebar navigation, project CRUD, costs, models
  - Vite dev proxy fix (`/health` not proxied), `scripts/test.sh e2e` command
  - ESLint/tsconfig updated to include `e2e/`, `.gitignore` updated for artifacts
- [x] (2026-02-17) Test runner script (`scripts/test.sh`) — unified Go/Python/Frontend/Integration runner
- [x] (2026-02-17) Integration test infrastructure (`tests/integration/`) — real PostgreSQL, build-tagged
  - Health/liveness tests, Project CRUD lifecycle, Task CRUD lifecycle, validation tests
  - Fixed goose migration `$$` blocks (StatementBegin/StatementEnd annotations)
  - Updated `.claude/commands/test.md` to use test runner script
- [x] (2026-02-17) Unit tests for AsyncHandler (buffer overflow, concurrent writes, flush) — 4 tests in `internal/logger/async_test.go`
- [x] (2026-02-19) Integration tests for Config Loader (precedence, validation, reload)
  - `internal/config/loader_integration_test.go`: 10 tests covering full hierarchy, partial override, invalid env, missing YAML, malformed YAML, validation, orchestrator overrides, reload, reload validation failure, reload env override
- [x] (2026-02-17) Unit tests for Idempotency (no header, store, replay, GET ignored, different keys) — 5 tests in `internal/middleware/idempotency_test.go`
- [x] (2026-02-19) Load tests for Rate Limiting (sustained vs burst, per-user limiters)
  - `tests/load/ratelimit_test.go` (build tag `//go:build load`)
  - 6 tests: sustained load, burst absorption, per-IP isolation, concurrent bucket creation, headers, cleanup under load
  - Run with: `go test -tags load -count=1 ./tests/load/`
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

## Post-Phase 11: Security Hardening (P0 Audit Fixes)

- [x] (2026-02-19) P0-1: Prompt injection defense in MetaAgent decomposition
  - `sanitizePromptInput()` strips control chars, role markers, enforces length limit
  - Data-boundary instruction added to system prompt
  - 6 unit tests in `internal/service/sanitize_test.go`
- [x] (2026-02-19) P0-2: Secret redaction utilities for safe logging
  - `Vault.Redacted()` masks secret values (first 2 chars + "****")
  - `Vault.RedactString()` scrubs secrets from arbitrary strings
  - `Vault.Keys()` returns key names without values
  - 4 new tests in `internal/secrets/vault_test.go`
- [x] (2026-02-19) P0-3: Wire audit trail into RuntimeService lifecycle
  - `appendAudit()` helper wired into 8 lifecycle points (start, complete, cancel, policy deny, quality gate pass/fail/error, delivery success/fail, budget exceeded)
- [x] (2026-02-19) P0-4: Quality gate fail-closed on NATS publish failure
  - NATS publish failure now finalizes run as failed instead of silently passing
- [x] (2026-02-19) P0-5: Post-execution budget enforcement
  - Immediate budget check in `HandleToolCallResult` after cost accumulation
  - Prevents budget overrun from single expensive tool calls

---

## Notes

- **Priority order**: Phases 0-11 complete. All phases implemented.
- **Dependencies**: Structured Logging → Request ID → Docker Logging → Log Script
- **Dependencies**: Event Sourcing → Policy Layer → Runtime API → Headless Autonomy
- **Dependencies**: Repo Map → Hybrid Retrieval → Retrieval Sub-Agent → GraphRAG
- **Dependencies**: Roadmap Domain → Store → Service → Handlers → Frontend
- **Dependencies**: Auth → Multi-Tenancy full rollout; Theme → WCAG audit
- **Testing**: Each new pattern requires unit + integration tests before merge
- **Documentation**: ADRs must be written before implementation (capture decision context)
- **Source**: Analysis document `docs/Analyse des CodeForge-Projekts (staging-Branch).md`
