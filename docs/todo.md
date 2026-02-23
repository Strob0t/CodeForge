# CodeForge — TODO Tracker

> LLM Agents: This is your **primary** task reference.
> Always read this file before starting work to understand current priorities.

### How to Use This File

- Before starting work: Read this file to understand what needs to be done
- After completing a task: Mark it `[x]`, add completion date, move to "Recently Completed" if needed
- When discovering new work: Add items to the appropriate section with context
- Format: `- [ ]` for open/pending, `- [x]` for done (with date)
- Cross-reference: Link to feature docs, architecture.md sections, or issues where relevant

---

### Phases 0-8: Complete

> All phases from 0 through 11 are complete, including P0-P2 security hardening.
> Current priority: **Phase 12+ (Architecture Evolution)** — new items from brainstorming and research.

#### Phase 3 — Reliability, Performance & Agent Foundation

#### 3A. Configuration Management

- [x] (2026-02-17) Implement hierarchical config system (defaults < YAML < ENV) (Go: `internal/config/config.go` typed Config struct, `internal/config/loader.go` Load with merge logic; Python: `workers/codeforge/config.py` WorkerSettings from ENV; validation layer for required fields and min values; `codeforge.yaml.example` with all fields documented; 6 test functions in `internal/config/loader_test.go`)
- [x] (2026-02-19) SIGHUP config reload (ConfigHolder with hot-reload, expanded SIGHUP handler) (`internal/config/config.go`: `ConfigHolder` struct with `sync.RWMutex`, `Get()`, `Reload()`; `internal/config/loader.go`: `LoadFrom(yamlPath)` for explicit YAML path loading; `cmd/codeforge/main.go`: SIGHUP handler now reloads both config and secrets vault; warns about non-hot-reloadable fields port, DSN, NATS URL)
- [x] (2026-02-19) CLI override support (`internal/config/loader.go`: `CLIFlags` struct with `*string` pointer fields nil = unset; `ParseFlags(args)`: stdlib `flag.FlagSet` parser, supports `--config/-c`, `--port/-p`, `--log-level`, `--dsn`, `--nats-url`; `LoadWithCLI(flags)`: full hierarchy defaults < YAML < ENV < CLI, returns resolved YAML path; `applyCLI()`: applies non-nil flag overrides after ENV; `cmd/codeforge/main.go`: parses `os.Args[1:]`, passes resolved YAML path to `ConfigHolder`; 8 new tests in `internal/config/loader_test.go` covering parse, shorthand, invalid, apply, nil, override, custom config)

#### 3B. Structured Logging & Observability

- [x] (2026-02-17) Structured JSON logging alignment (Go + Python) (Go: `internal/logger/logger.go` slog JSON handler with `service` field, level from config; Python: `workers/codeforge/logger.py` structlog JSON renderer with `service` field; common schema: `{time, level, service, msg, request_id, task_id}`; 3 test functions in `internal/logger/logger_test.go`)
- [x] (2026-02-17) Request ID propagation (Correlation ID) (Go: `internal/logger/context.go` WithRequestID/RequestID, `internal/middleware/requestid.go`; NATS: headers carry X-Request-ID, extracted in Subscribe, injected in Publish; Python: extract from NATS headers, bind to structlog context; 2 test functions in `internal/middleware/requestid_test.go`; 1 new Python test for request ID propagation)
- [x] (2026-02-19) PostgreSQL log_line_prefix configuration
- [x] (2026-02-17) Docker Compose logging configuration (`x-logging` anchor with `json-file` driver, `max-size: 10m`, `max-file: 3`; applied to all 5 services)
- [x] (2026-02-17) Async logging for Go Core (`internal/logger/async.go`: AsyncHandler wrapping slog.Handler with buffered channel 10,000 + 4 worker goroutines; non-blocking drop policy with atomic dropped counter; `Close()` flushes remaining records, `WithAttrs`/`WithGroup` share channel; `logger.go`: `New()` returns `(*slog.Logger, Closer)`, wraps with AsyncHandler when `cfg.Async == true`; 4 tests in `internal/logger/async_test.go`)
- [x] (2026-02-17) Python async logging with QueueHandler (`workers/codeforge/logger.py`: `QueueHandler` + `QueueListener` with 10,000-record buffer; `stop_logging()` for graceful shutdown with queue drain; 2 tests in `workers/tests/test_logger.py`)
- [x] (2026-02-17) Create `scripts/logs.sh` helper script (commands: `tail`, `errors`, `service <name>`, `request <id>`; uses `docker compose logs` for filtering)

#### 3C. Reliability Patterns

- [x] (2026-02-17) Circuit Breaker for external services (`internal/resilience/breaker.go`: zero-dep, states closed/open/half-open, 5 tests; wrapped: NATS Publish, LiteLLM doRequest via SetBreaker injection; configurable maxFailures and timeout from config.Breaker)
- [x] (2026-02-17) Graceful shutdown with drain phase (4-phase ordered shutdown: HTTP → cancel subscribers → NATS Drain → DB close; NATS: `Drain()` method added to Queue interface and NATS adapter; Python: 10s drain timeout to prevent hanging)
- [x] (2026-02-18) Agent execution timeout & heartbeat (timeout enforcement exists: `checkTermination()` in `runtime.go` checks elapsed time per tool call; cancellation via `POST /api/v1/runs/{id}/cancel` + NATS `runs.cancel` subject; heartbeat ticker 30s for progress tracking during long tool calls with Go heartbeat subscriber in StartSubscribers and timeout check in checkTermination and Python `start_heartbeat()`/`stop_heartbeat()` on RuntimeClient called from executor, config HeartbeatInterval 30s HeartbeatTimeout 120s with ENV overrides; context-level timeout wrapping entire run not just tool-call boundaries with goroutine-based timer per run tracked via `runTimeouts` sync.Map of context.CancelFunc, auto-cancels run when TerminationConfig.TimeoutSeconds expires)
- [x] (2026-02-17) Idempotency Keys for critical operations (`internal/middleware/idempotency.go`: HTTP middleware for POST/PUT/DELETE deduplication; NATS JetStream KV storage 24h TTL via `KeyValue()` method in `internal/adapter/nats/nats.go`; `Idempotency-Key` header: KV hit → replay cached response, miss → tee-capture + store; 5 tests in `internal/middleware/idempotency_test.go`)
- [x] (2026-02-17) Optimistic Locking for concurrent updates (Migration 003: `version INTEGER NOT NULL DEFAULT 1` on projects, agents, tasks + auto-increment trigger; domain: `ErrNotFound`, `ErrConflict` sentinel errors in `internal/domain/errors.go`; store: `WHERE version = $N` on UpdateProject, `pgx.ErrNoRows` → `ErrNotFound` on Get queries; HTTP: `writeDomainError()` maps ErrNotFound → 404, ErrConflict → 409; version field added to Project, Agent, Task domain structs)
- [x] (2026-02-17) Dead Letter Queue (DLQ) for failed messages (Go NATS: `moveToDLQ()` publishes to `{subject}.dlq` after 3 retries, acks original; `Retry-Count` header tracked, `NakWithDelay(2s)` for retries; Python consumer: `_move_to_dlq()` + `_retry_count()` with same MAX_RETRIES=3)
- [x] (2026-02-17) Schema validation for NATS messages (`internal/port/messagequeue/schemas.go`: typed payload structs per subject; `internal/port/messagequeue/validator.go`: `Validate(subject, data)` via JSON unmarshal; invalid messages sent directly to DLQ with no retries)

#### 3D. Event Sourcing for Agent Trajectory

- [x] (2026-02-17) Domain: `internal/domain/event/event.go` (AgentEvent struct, Type constants)
- [x] (2026-02-17) Port: `internal/port/eventstore/store.go` (Append, LoadByTask, LoadByAgent)
- [x] (2026-02-17) Storage: PostgreSQL table `agent_events` (append-only, migration 004, indexed)
- [x] (2026-02-17) Service: Event recording on dispatch/result/stop, LoadTaskEvents method
- [x] (2026-02-17) API: `GET /api/v1/tasks/{id}/events` handler + frontend client method
- [x] (2026-02-18) Event store enhancements: RunID association + LoadByRun query (Migration 013: `run_id UUID` column + index on `agent_events`; domain: `RunID` field on `AgentEvent`, set in `appendRunEvent`; port: `LoadByRun` method on `eventstore.Store` interface; Postgres adapter: `run_id` in INSERT/SELECT for all methods + `LoadByRun`; HTTP: `GET /api/v1/runs/{id}/events` handler + route; handlers: `Events eventstore.Store` field wired in main.go)
- [x] (2026-02-18) Trajectory API: cursor-paginated LoadTrajectory, TrajectoryStats, 2 REST endpoints, TrajectoryPanel frontend
- [x] (2026-02-19) Replay / Audit Trail — ReplayService (ListCheckpoints, Replay, AuditTrail, RecordAudit), LoadEventsRange + ListCheckpoints on eventstore, audit_trail table (migration 021), REST endpoints (checkpoints, replay, audit)
- [x] (2026-02-19) Session Events (Resume/Fork/Rewind) — Session entity, SessionService (Resume, Fork, Rewind, CRUD), sessions table (migration 021), 6 session event types, REST endpoints (resume, fork, rewind, list sessions, get session)

#### 3E. Performance Optimizations

- [x] (2026-02-17) Cache Layer (tiered L1 + L2) (Port: `internal/port/cache/cache.go` Get/Set/Delete interface; L1: `internal/adapter/ristretto/cache.go` in-process, configurable max cost; L2: `internal/adapter/natskv/cache.go` NATS JetStream KV; tiered: `internal/adapter/tiered/cache.go` L1 → L2 with backfill on L2 hit; config: `L1MaxSizeMB` 100, `L2Bucket` "CACHE", `L2TTL` 10m; 5 tests in `internal/adapter/tiered/cache_test.go`; dependency: `github.com/dgraph-io/ristretto/v2`)
- [x] (2026-02-17) Database connection pool tuning (`NewPool` accepts `config.Postgres` with MaxConns=15, MinConns=2, MaxConnLifetime=1h, MaxConnIdleTime=10m, HealthCheckPeriod=1m; all pool parameters configurable via YAML/ENV)
- [x] (2026-02-17) Rate limiting per IP (`internal/middleware/ratelimit.go`: token bucket, per-IP, configurable rate/burst; headers: `X-RateLimit-Remaining`, `X-RateLimit-Reset` GitHub-style; returns 429 with `Retry-After` header; 4 tests in `internal/middleware/ratelimit_test.go`)
- [x] (2026-02-18) Go Core: Worker pools for CPU-bound tasks (Git operations wrapped via `golang.org/x/sync/semaphore` with configurable limit default: 5; context propagation for cancellation via `semaphore.Acquire(ctx, 1)`; `internal/git/pool.go`: Pool struct with nil-safe `Run()` method; injected into gitlocal.Provider, CheckpointService, DeliverService; 4 tests in `internal/git/pool_test.go`)
- [x] (2026-02-18) Python Workers: Full asyncio adoption (NATS consumer: fully async with `await nats.connect()`, `await js.subscribe()`, `asyncio.gather()`; LiteLLM calls: async via `httpx.AsyncClient` for completion, health, embeddings; quality gate: `asyncio.create_subprocess_shell()` with timeout; DB queries: N/A — Python workers do not access PostgreSQL by design since Go is control plane)

#### 3F. Security & Isolation

- [x] (2026-02-17) Agent sandbox resource limits (Shared type: `internal/domain/resource/limits.go` Limits struct, Merge, Cap; agent domain: `ResourceLimits *resource.Limits` field on Agent struct; policy domain: `ResourceLimits *resource.Limits` field on PolicyProfile struct; store/handler chain updated: CreateAgent accepts `*resource.Limits`, stored as JSONB; migration: `012_add_agent_resource_limits.sql`; sandbox merge logic: global config → policy limits → agent limits → cap at ceiling; 5 tests in `internal/domain/resource/limits_test.go`)
- [x] (2026-02-18) Secrets vault with SIGHUP hot reload (`internal/secrets/vault.go`: Vault struct with RWMutex, Get/Reload methods; `internal/secrets/env_loader.go`: EnvLoader factory for env-var-based secrets; SIGHUP handler in `cmd/codeforge/main.go` triggers `vault.Reload()`; LiteLLM client: `SetVault()` injection, `masterKey()` reads from vault; 4 tests in `internal/secrets/vault_test.go`)

#### 3G. API & Health

- [x] (2026-02-17) Health check granularity (`/health` liveness always 200, `/health/ready` readiness pings DB, NATS, LiteLLM; returns 503 with per-service status + latency if any dependency down; NATS `IsConnected()` added to Queue interface)
- [x] (2026-02-18) API deprecation middleware (`internal/middleware/deprecation.go`: `Deprecation(sunset)` adds RFC 8594 headers; `Deprecation: true` + `Sunset: <HTTP-date>` headers on deprecated routes; 3 tests in `internal/middleware/deprecation_test.go`; ready for use when `/api/v2` is introduced with no sunset date set yet)
- [x] (2026-02-18) Database migrations: Rollback capability (All 15 migrations have `-- +goose Up` and `-- +goose Down` sections; `RollbackMigrations()` and `MigrationVersion()` exposed in `internal/adapter/postgres/postgres.go`; integration test `TestMigrationUpDown` in `tests/integration/migration_test.go`; `./scripts/test.sh migrations` sub-command for CI rollback verification)

#### 3H. Multi-Tenancy Preparation (Soft Launch)

- [x] (2026-02-18) Add tenant_id to all tables (Migration 014: `tenant_id UUID NOT NULL DEFAULT '00000000-...'` on 7 tables; indexes on projects, tasks, agents, runs; `internal/middleware/tenant.go`: TenantID middleware X-Tenant-ID header, default fallback; context helpers: `TenantIDFromContext()`, `WithTenantID()`; domain structs: `TenantID` field added to Project, Task, Agent, Run; middleware chain: `r.Use(middleware.TenantID)` after RequestID; 3 tests in `internal/middleware/tenant_test.go`)
- [x] (2026-02-19) Full WHERE tenant_id clauses in all queries + tenant CRUD (Migration 018: `tenants` table, `tenant_id UUID` on 11 remaining tables; `internal/domain/tenant/tenant.go`: Tenant struct, CreateRequest, UpdateRequest; `internal/service/tenant.go`: TenantService CRUD + ValidateExists; `internal/adapter/postgres/store_tenant.go`: tenant CRUD + `tenantFromCtx()` helper; `internal/adapter/postgres/store.go`: ALL ~60 methods updated with `AND tenant_id = $N`; `internal/adapter/postgres/eventstore.go`: all 6 methods updated with tenant_id filtering; REST API: `POST/GET /api/v1/tenants`, `GET/PUT /api/v1/tenants/{id}`; `cmd/codeforge/main.go`: TenantService wired into handlers)

---

### Phase 4 — Agent Execution Engine (Approach C: Go Control Plane + Python Runtime)

> Architectural decision: Go-Core as "Control Plane" (State/Policies/Sessions),
> Python-Worker as "Data/Execution Plane" (Models/Tools/Loop execution).
> Source: Analyse-Dokument Section 13, Approach C.

#### 4A. Policy Layer (Permission + Checkpoint Gate)

- [x] (2026-02-17) Design Policy domain model (`internal/domain/policy/policy.go`: PolicyProfile, PermissionRule, ToolSpecifier, ToolCall; permission modes: `default`, `acceptEdits`, `plan`, `delegate`; decisions: `allow`, `deny`, `ask`; quality gates: `RequireTestsPass`, `RequireLintPass`, `RollbackOnGateFail`; termination conditions: `MaxSteps`, `TimeoutSeconds`, `MaxCost`, `StallDetection`; validation: `internal/domain/policy/validate.go`)
- [x] (2026-02-17) Implement Policy Evaluator service (`internal/service/policy.go`: first-match-wins rule evaluation; ToolSpecifier matching: exact tool + optional sub-pattern glob; path allow/deny lists with glob patterns using `**` recursive matching; command allow/deny lists prefix-based matching; mode-based fallback: `plan`→deny, `default`→ask, `acceptEdits`→allow, `delegate`→allow)
- [x] (2026-02-17) Policy Presets (4 built-in profiles) (`plan-readonly`: read-only, deny Edit/Write/Bash, 30 steps, 300s, $1; `headless-safe-sandbox`: safe Bash git/test only, path deny for secrets, 50 steps, 600s, $5; `headless-permissive-sandbox`: broader Bash, deny network cmds, 100 steps, 1800s, $20; `trusted-mount-autonomous`: all tools, deny only secrets paths, 200 steps, 3600s, $50)
- [x] (2026-02-17) YAML-configurable custom policies (`internal/domain/policy/loader.go`: LoadFromFile, LoadFromDirectory; config: `CODEFORGE_POLICY_DEFAULT`, `CODEFORGE_POLICY_DIR` env vars)
- [x] (2026-02-17) Policy REST API (3 endpoints) (`GET /api/v1/policies` list all profile names, `GET /api/v1/policies/{name}` full profile definition, `POST /api/v1/policies/{name}/evaluate` evaluate a ToolCall)
- [x] (2026-02-17) Policy tests: 46 test functions across 4 files (`internal/domain/policy/policy_test.go` 5, `presets_test.go` 8, `loader_test.go` 7; `internal/service/policy_test.go` 25; config tests 3 in `loader_test.go`, handler tests 7 in `handlers_test.go`)
- [x] (2026-02-17) Implement Checkpoint system (`internal/service/checkpoint.go`: CheckpointService with shadow Git commits; CreateCheckpoint: `git add -A && git commit` for file-modifying tools Edit, Write, Bash; RewindToFirst: `git reset --hard {firstHash}^` restore pre-run state on quality gate failure; RewindToLast: `git reset --hard {lastHash}^` undo last change only; CleanupCheckpoints: `git reset --soft {firstHash}^` remove shadow commits, keep working state; runtime integration: checkpoint on tool call, rewind on rollback, cleanup on finalize/cancel; 5 tests in `internal/service/checkpoint_test.go`)
- [x] (2026-02-18) Policy UI in Frontend (Backend: CRUD endpoints POST /policies, DELETE /policies/{name} with YAML persistence; backend: SaveProfile, DeleteProfile methods on PolicyService with preset protection; backend: SaveToFile in policy loader, IsPreset helper in presets; frontend: full type definitions PolicyProfile, PermissionRule, QualityGate, etc.; frontend: extended API client get, create, delete, evaluate; frontend: PolicyPanel component with 3 views list, detail with evaluate tester, editor; frontend: integrated into ProjectDetailPage between agents and run management; tests: 6 new service tests, 6 new handler tests — all passing)
- [x] (2026-02-19) Policy scope levels (global → project → run), run overrides, "why matched" explanation (`internal/domain/policy/evaluation.go`: Scope type global/project/run, EvaluationResult struct; `internal/service/policy.go`: `EvaluateWithReason()`, `ResolveProfile()`, `evaluateWithReason()`; `internal/service/runtime.go`: uses EvaluateWithReason, logs evaluation reason at Debug level; `internal/adapter/http/handlers.go`: evaluate endpoint returns full EvaluationResult; migration 019: `policy_profile` column on projects table; `internal/domain/project/project.go`: PolicyProfile field on Project struct)

#### 4B. Runtime API (Step-by-Step Execution Protocol)

- [x] (2026-02-17) Define Runtime Client protocol (Go ↔ Python) (Run entity: `internal/domain/run/run.go` Run, StartRequest, Status, ExecMode; ToolCall types: `internal/domain/run/toolcall.go` ToolCallRequest/Response/Result; validation: `internal/domain/run/validate.go` Run.Validate, StartRequest.Validate; NATS subjects: `runs.start`, `runs.toolcall.{request,response,result}`, `runs.complete`, `runs.cancel`, `runs.output`; NATS payloads: RunStartPayload, ToolCallRequestPayload, ToolCallResponsePayload, etc.; event types: `run.started`, `run.completed`, `run.toolcall.{requested,approved,denied,result}`; WS events: `run.status`, `run.toolcall`)
- [x] (2026-02-17) Database: `runs` table (migration 005, FK to tasks/agents/projects, optimistic locking) (Store interface: CreateRun, GetRun, UpdateRunStatus, CompleteRun, ListRunsByTask)
- [x] (2026-02-17) RuntimeService: `internal/service/runtime.go` (StartRun, HandleToolCallRequest, HandleToolCallResult, HandleRunComplete, CancelRun; termination enforcement: MaxSteps, MaxCost, Timeout checked per tool call; policy evaluation per tool call reuses Phase 4A policy layer; quality gates logged with enforcement deferred to 4C; NATS subscribers for 4 subjects, cancel cleanup)
- [x] (2026-02-17) REST API: 4 new endpoints (`POST /api/v1/runs` start a run, `GET /api/v1/runs/{id}` get run details, `POST /api/v1/runs/{id}/cancel` cancel a run, `GET /api/v1/tasks/{id}/runs` list runs for a task)
- [x] (2026-02-17) Python RuntimeClient: `workers/codeforge/runtime.py` (request_tool_call, report_tool_result, complete_run, send_output; cancel listener, step/cost tracking; consumer extended with `runs.start` subscription; executor extended with `execute_with_runtime()` method)
- [x] (2026-02-17) Tests: 44 new test functions across Go + Python (`internal/domain/run/run_test.go` 15, `internal/service/runtime_test.go` 22; `internal/adapter/http/handlers_test.go` +5, `workers/tests/test_runtime.py` 9)
- [x] (2026-02-17) Implement Execution Modes — Docker Sandbox (`internal/service/sandbox.go`: SandboxService with Docker CLI os/exec; Create docker create with resource flags, Start, Exec, Stop, Remove, Get; config: `SandboxConfig` in `internal/config/config.go` memory, cpu, pids, storage, network, image; domain: `ExecModeHybrid` constant added to `internal/domain/run/run.go`; runtime integration: sandbox lifecycle in StartRun, finalizeRun, CancelRun; 5 tests in `internal/service/sandbox_test.go`; mount mode: implicit — Python worker operates directly on host filesystem with no additional Go code needed)
- [x] (2026-02-19) Hybrid execution mode — workspace mounted read-write, commands execute inside Docker container (`internal/service/sandbox.go`: `CreateHybrid()` method read-write mount, no `--read-only`, `sleep infinity`; `internal/service/runtime.go`: hybrid mode handling in `StartRun` switch on ExecMode, `sendToolCallResponse` includes `exec_mode` + `container_id`; `internal/config/config.go`: `HybridConfig{CommandImage, MountMode}` sub-struct on Runtime; `internal/config/loader.go`: env overrides `CODEFORGE_HYBRID_IMAGE`, `CODEFORGE_HYBRID_MOUNT_MODE`; `internal/port/messagequeue/schemas.go`: `exec_mode`, `container_id` fields on `ToolCallResponsePayload`; refactored Start/Stop/Exec/Remove to use ContainerID instead of regenerating names)
- [x] (2026-02-18) Runtime Compliance Tests (`internal/service/runtime_compliance_test.go`: 8 sub-tests x 2 modes Mount, Sandbox; sub-tests: StartRun, ToolCallFlow, PolicyEnforcement, Termination_MaxSteps, Termination_MaxCost, CancelRun, Completion, StallDetection; all 16 compliance test cases passing)

#### 4C. Headless Autonomy (Server-First Execution) (COMPLETED)

- [x] (2026-02-17) CI Fix: golangci-lint v2 config migration (local-prefixes array, removed v1 options)
- [x] (2026-02-17) Config extension: `config.Runtime` struct with 6 fields (StallThreshold, QualityGateTimeout, DefaultDeliverMode, DefaultTestCommand, DefaultLintCommand, DeliveryCommitPrefix) + ENV overrides + YAML example
- [x] (2026-02-17) Stall Detection: `internal/domain/run/stall.go` (StallTracker with FNV-64a hash ring buffer), 10 domain tests (Progress tools Edit, Write, Bash with success reset counter; non-progress tools increment; repetition detection via output hash ring buffer size 3; configurable threshold from policy `StallThreshold` or `config.Runtime.StallThreshold`; integrated into `HandleToolCallResult` — terminates run on stall)
- [x] (2026-02-17) Quality Gate Enforcement: NATS request/result protocol (Go → Python → Go) (New status `StatusQualityGate` for transient gate-check state; `HandleRunComplete` triggers gate request when policy has `RequireTestsPass`/`RequireLintPass`; `HandleQualityGateResult` processes outcomes: pass → deliver → finalize, fail + rollback → fail; Python `QualityGateExecutor` runs test/lint commands via `asyncio.create_subprocess_shell` with timeout; consumer extended with `runs.qualitygate.request` subscription; 7 Python tests for quality gate executor)
- [x] (2026-02-17) Deliver Modes: 5 strategies (none, patch, commit-local, branch, pr) (Domain types: `DeliverMode` in `run.go`, migration `006_add_deliver_mode.sql`; `DeliverService` in `internal/service/deliver.go` using git CLI + `gh` for PRs; graceful fallback: PR → branch-only if `gh` unavailable, push failure non-fatal; 5 delivery tests none, patch, commit-local, branch, no-workspace)
- [x] (2026-02-17) Frontend: RunPanel component, Run types/API client, WS event integration (`RunPanel.tsx`: start form task/agent/policy/deliver, active run display, run history; `ProjectDetailPage.tsx`: agents resource, RunPanel integration, WS cases for run/QG/delivery events; API: `runs.start/get/cancel/listByTask`, `policies.list`)
- [x] (2026-02-17) Events: 7 new event types (QG started/passed/failed, delivery started/completed/failed, stall detected)
- [x] (2026-02-17) WS: `run.qualitygate` and `run.delivery` events with typed structs
- [x] (2026-02-17) Checkpoint system — see 4A above

---

### Phase 5 — Multi-Agent Orchestration

> Source: Analyse-Dokument Section "Multi-Agent Orchestration Architecture"

#### 5A. Execution Plans — DAG Scheduling with 4 Protocols (COMPLETED)

- [x] (2026-02-17) Domain model: `internal/domain/plan/` (plan.go, validate.go, dag.go) (ExecutionPlan, Step, Protocol, Status, StepStatus, CreatePlanRequest; DAG cycle detection Kahn's algorithm, ReadySteps, RunningCount, AllTerminal, AnyFailed; 25 domain tests 16 validation + 8 DAG + 1 compile check)
- [x] (2026-02-17) Config: `config.Orchestrator` (MaxParallel, PingPongMaxRounds, ConsensusQuorum) + ENV overrides
- [x] (2026-02-17) Database: migration 007 (execution_plans + plan_steps tables with UUID arrays)
- [x] (2026-02-17) Store interface: 9 new methods + Postgres adapter (transactional CreatePlan)
- [x] (2026-02-17) Events: 5 plan event types + 2 WS event types
- [x] (2026-02-17) RuntimeService callback: SetOnRunComplete + invocation in finalizeRun
- [x] (2026-02-17) OrchestratorService: 4 protocol handlers (sequential, parallel, ping_pong, consensus) (CreatePlan, StartPlan, GetPlan, ListPlans, CancelPlan, HandleRunCompleted)
- [x] (2026-02-17) REST API: 5 endpoints for plan management
- [x] (2026-02-17) Frontend: PlanPanel.tsx, plan types/API client, WS integration in ProjectDetailPage
- [x] (2026-02-17) Tests: 12 orchestrator service tests, all passing

#### 5B. Orchestrator Agent (Meta-Agent) (COMPLETED)

- [x] (2026-02-17) Orchestrator Mode: `manual`, `semi_auto`, `full_auto` — config, env overrides, domain types
- [x] (2026-02-17) LLM-based feature decomposition via LiteLLM ChatCompletion (Go → LiteLLM Proxy)
- [x] (2026-02-17) MetaAgentService: prompt engineering, JSON parsing, task creation, agent selection
- [x] (2026-02-17) Agent strategy selection (single, pair, team) with hint-based agent matching
- [x] (2026-02-17) Auto-start: `full_auto` mode or `auto_start` override starts plan immediately
- [x] (2026-02-17) REST API: `POST /api/v1/projects/{id}/decompose`
- [x] (2026-02-17) Frontend: Decompose Feature form in PlanPanel with context/model/auto-start options
- [x] (2026-02-17) Tests: 4 litellm client tests, 5 domain tests, 9 meta-agent service tests — all passing

#### 5C. Agent Teams + Context-Optimized Planning (COMPLETED)

- [x] (2026-02-17) Domain model: `internal/domain/agent/team.go` (AgentTeam: members, protocol, status, version; TeamMember: AgentID, Role coder/reviewer/tester/documenter/planner; TeamProtocol: reuses plan.Protocol sequential, ping_pong, consensus, parallel; CreateTeamRequest validation: name, project_id, roles, no duplicates; 8 domain tests in `team_test.go`)
- [x] (2026-02-17) Database: migration 008 (agent_teams + team_members)
- [x] (2026-02-17) Store interface: 5 new methods + Postgres adapter
- [x] (2026-02-17) Config: `max_team_size` in Orchestrator (default 5)
- [x] (2026-02-17) PoolManagerService: `internal/service/pool_manager.go` (CreateTeam, AssembleTeamForStrategy, CleanupTeam, GetTeam, ListTeams, DeleteTeam; resource availability checks agent exists, idle, same project; 8 service tests)
- [x] (2026-02-17) TaskPlannerService: `internal/service/task_planner.go` (PlanFeature: context enrichment workspace file tree → LLM decompose → optional auto-team; complexity heuristic single/pair/team based on step count; 3 service tests)
- [x] (2026-02-17) REST API: 5 new endpoints (team CRUD + plan-feature)
- [x] (2026-02-17) Frontend: team types, API client (teams namespace + planFeature)
- [x] (2026-02-17) SharedContext: versioned team-level shared state (done in 5D)
- [x] (2026-02-17) NATS message bus for context updates (done in 5D)

#### 5D. Context Optimizer (COMPLETED)

- [x] (2026-02-17) Token budget management per task (ContextPack domain model with token budget + entries; EstimateTokens heuristic len/4, ScoreFileRelevance keyword matching; configurable budget default_context_budget and prompt reserve)
- [x] (2026-02-17) Context packing as structured artifacts (ContextOptimizerService: scan workspace → score → pack within budget → persist; SharedContextService: team-level shared state with NATS notifications; pre-packed context injected into RunStartPayload for Python workers; 4 new REST endpoints task context CRUD, shared context CRUD; 26+ new test functions Go domain + service + Python, all passing)

#### 5E. Integration Fixes, WS Events, Modes System (COMPLETED)

- [x] (2026-02-17) Fix TeamID propagation (Run, ExecutionPlan, StartRequest) (Migration 010: team_id on runs + execution_plans, output on runs; orchestrator → runtime → ContextOptimizer TeamID flow fixed)
- [x] (2026-02-17) Auto-init SharedContext on team creation (PoolManager)
- [x] (2026-02-17) Auto-populate SharedContext from run outputs (Orchestrator)
- [x] (2026-02-17) WS events: team.status, shared.updated (events.go + broadcasts)
- [x] (2026-02-17) Modes System: domain model, 8 presets, ModeService, 3 REST endpoints
- [x] (2026-02-17) Frontend: Mode/CreateModeRequest types, modes API namespace
- [x] (2026-02-17) Mock stores + test fixes (CompleteRun signature, nil-safe hub)

---

### Phase 6 — Code-RAG (Context Engine for Large Codebases)

> Source: Analyse-Dokument Section 14/5, "RAG am Anfang"
> Three-tier approach: RepoMap → Hybrid Retrieval → GraphRAG (later)

#### 6A. Repo Map (COMPLETED)

- [x] (2026-02-17) tree-sitter based Repo Map (Python Worker: RepoMapGenerator with tree-sitter + tree-sitter-language-pack; symbol extraction functions, classes, methods, types, interfaces for 16+ languages; file ranking via networkx PageRank import graph analysis; Go Backend: domain model, PostgreSQL store, RepoMapService, REST API, WS events; frontend: RepoMapPanel component with stats, language tags, collapsible map text; NATS integration: repomap.generate / repomap.result subjects; new dependencies: tree-sitter ^0.24, tree-sitter-language-pack ^0.13, networkx ^3.4)

#### 6B. Hybrid Retrieval — BM25 + Semantic Search (COMPLETED)

- [x] (2026-02-17) Python Worker: HybridRetriever with BM25S + LiteLLM embeddings (AST-aware code chunking via tree-sitter reuse from 6A; BM25S keyword indexing 500x faster than rank_bm25; semantic embeddings via LiteLLM proxy `/v1/embeddings`; Reciprocal Rank Fusion RRF with k=60 to combine rankings; in-memory per-project indexes no vector DB; shared constants extracted to `_tree_sitter_common.py`)
- [x] (2026-02-17) Go Backend: RetrievalService with synchronous search (NATS subjects: retrieval.index.request/result, retrieval.search.request/result; channel-based waiter pattern with correlation IDs for sync search 30s timeout; REST API: POST /projects/{id}/index, GET /projects/{id}/index, POST /projects/{id}/search; WS event: retrieval.status building/ready/error; context optimizer auto-injects hybrid results as EntryHybrid)
- [x] (2026-02-17) Frontend: RetrievalPanel component (Index status display file count, chunk count, embedding model; build Index / search UI with results display; integrated into ProjectDetailPage with WS event handler)
- [x] (2026-02-17) New dependencies: bm25s ^0.2, numpy ^2.0
- [x] (2026-02-17) Tests: 11 Python tests (chunking, RRF, cosine similarity, integration), 5 Go tests (service), 3 Go handler tests — all passing

#### 6C. Retrieval Sub-Agent (COMPLETED)

- [x] (2026-02-18) LLM-guided multi-query retrieval sub-agent (Python: `RetrievalSubAgent` class — expand queries via LLM, parallel hybrid search, dedup, LLM re-rank; Python: `SubAgentSearchRequest` / `SubAgentSearchResult` Pydantic models; Python: consumer handler for `retrieval.subagent.request` NATS subject; Go: `SubAgentSearchSync` / `HandleSubAgentSearchResult` on RetrievalService 60s timeout, correlation ID waiter; Go: NATS subjects `retrieval.subagent.request` / `retrieval.subagent.result` + payload types; Go: config fields `SubAgentModel`, `SubAgentMaxQueries`, `SubAgentRerank` with ENV overrides; Go: context optimizer `fetchRetrievalEntries()` sub-agent first, single-shot fallback; Go: HTTP handler `POST /projects/{id}/search/agent`; frontend: agent/standard search toggle in RetrievalPanel, expanded queries display; tests: 8 Python tests unit + integration, 3 new Go service tests — all passing)
- [x] (2026-02-18) Code review refinements (16 changes across architecture, quality, tests, performance) (Generic `syncWaiter[T]` replacing duplicate waiter patterns DRY; health tracking with 30s cooldown fast-fail on worker failures; percentile-based priority normalization RRF scores mapped to 60-85 range; shared retrieval deadline sub-agent + single-shot fallback under one timeout; parallel workspace scan + retrieval in BuildContextPack; check-before-build guard to prevent redundant context pack builds; defense-in-depth validation Pydantic validators + Go handler clamping; unified `SearchResult` dataclass into `RetrievalSearchHit` Pydantic model; DRY `_publish_error_result()` helper in consumer error handling; pre-built rank dict for O(1) BM25 lookup, per_query_k = top_k fix; 5 new tests 3 Python, 2 Go — all 77 Python + all Go tests passing)

#### 6D. GraphRAG — Structural Code Graph Intelligence (COMPLETED)

- [x] (2026-02-18) PostgreSQL adjacency-list graph (no Neo4j — single-DB architecture) (Migration 016: `graph_nodes`, `graph_edges`, `graph_metadata` tables; nodes: function/class/method/module definitions via tree-sitter; edges: import edges Python/Go/TS/JS, call edges name-matching heuristic)
- [x] (2026-02-18) Python: CodeGraphBuilder + GraphSearcher (`workers/codeforge/graphrag.py`) (Build pipeline: walk files → tree-sitter parse → extract definitions + imports + calls → batch upsert to PostgreSQL; BFS search with hop-decay scoring default decay 0.7: hop 0 = 1.0, hop 1 = 0.7, hop 2 = 0.49; bidirectional traversal outgoing + incoming edges; edge path tracking for explainability; 19 tests in `test_graphrag.py` — all passing)
- [x] (2026-02-18) Python consumer: 2 new NATS handlers (`graph.build.request`, `graph.search.request`)
- [x] (2026-02-18) Go: GraphService (`internal/service/graph.go`) (Follows RetrievalService pattern: syncWaiter, health tracking, WS broadcasts; RequestBuild, HandleBuildResult, SearchSync, HandleSearchResult, GetStatus, StartSubscribers)
- [x] (2026-02-18) Go: 4 NATS subjects + 5 payload types in schemas.go
- [x] (2026-02-18) Go: 4 config fields (GraphEnabled, GraphMaxHops, GraphTopK, GraphHopDecay)
- [x] (2026-02-18) Go: Context optimizer integration — `fetchGraphEntries()` uses retrieval hits as seed symbols (Priority: 70 - distance * 10 → hop 0 = 70, hop 1 = 60, hop 2 = 50; guarded by GraphEnabled + graph status "ready")
- [x] (2026-02-18) Go: 3 HTTP endpoints (`POST /graph/build`, `GET /graph/status`, `POST /graph/search`)
- [x] (2026-02-18) Go: EntryGraph kind in domain context, WS GraphStatusEvent
- [x] (2026-02-18) Frontend: GraphStatus/GraphSearchHit/GraphSearchResult types, API client, RetrievalPanel graph section
- [x] (2026-02-18) All linting passes: golangci-lint, ruff, ESLint, prettier, pre-commit

---

### Phase 8 — Roadmap Foundation, Event Trajectory, Docker Production

#### 8A. Roadmap/Feature-Map Foundation (Pillar 2)

- [x] (2026-02-18) Domain models: `internal/domain/roadmap/` (Roadmap, Milestone, Feature, statuses, validation) (Roadmap/Milestone statuses: draft, active, complete, archived; feature statuses: backlog, planned, in_progress, done, cancelled; optimistic locking Version field, tenant_id, labels []string, external_ids map; 18 domain tests in `roadmap_test.go`)
- [x] (2026-02-18) Migration 017: `roadmaps`, `milestones`, `features` tables (Unique idx on project_id, sort_order indexes, TEXT[] labels, JSONB external_ids; reuses existing `update_updated_at()` and `increment_version()` trigger functions)
- [x] (2026-02-18) Port interfaces: `specprovider/` (SpecProvider + Registry), `pmprovider/` (PMProvider + Registry) (Follows gitprovider pattern: self-registering via `init()`, capability declarations)
- [x] (2026-02-18) Store interface: 16 new methods on `database.Store` (Roadmap/Milestone/Feature CRUD)
- [x] (2026-02-18) Postgres adapter: 16 method implementations with optimistic locking
- [x] (2026-02-18) RoadmapService: CRUD delegation, AutoDetect (file markers), AIView (json/yaml/markdown) (Broadcasts `roadmap.status` WS event on mutations)
- [x] (2026-02-18) REST API: 12 roadmap endpoints (GET/POST/PUT/DELETE /projects/{id}/roadmap, GET /projects/{id}/roadmap/ai, POST /projects/{id}/roadmap/detect, POST /projects/{id}/roadmap/milestones, GET/PUT/DELETE /milestones/{id}, POST /milestones/{id}/features, GET/PUT/DELETE /features/{id})
- [x] (2026-02-18) WS event: `roadmap.status` with RoadmapStatusEvent struct
- [x] (2026-02-18) Frontend: RoadmapPanel.tsx — milestone/feature tree, create/edit forms, auto-detect, AI view
- [x] (2026-02-18) main.go wiring: RoadmapService creation + Handlers struct field

#### 8B. Event Trajectory API + Frontend

- [x] (2026-02-18) Event store extension: TrajectoryFilter, TrajectoryPage, TrajectorySummary types (Cursor-paginated LoadTrajectory with type/time filtering; TrajectoryStats with SQL aggregates event counts, duration, tool calls, errors)
- [x] (2026-02-18) Postgres implementation: dynamic WHERE clause builder, cursor pagination, aggregate stats
- [x] (2026-02-18) REST API: 2 trajectory endpoints (GET /runs/{id}/trajectory with ?types=...&after=...&before=...&cursor=...&limit=50; GET /runs/{id}/trajectory/export with ?format=json, Content-Disposition: attachment)
- [x] (2026-02-18) Frontend: TrajectoryPanel.tsx — vertical timeline, event type filters, stats summary, export

#### 8C. Docker/CI for Production

- [x] (2026-02-18) Dockerfile (Go Core): multi-stage golang:1.24-alpine → alpine:3.21 with git+ca-certs
- [x] (2026-02-18) Dockerfile.worker (Python): python:3.12-slim, poetry install --only main, non-root user
- [x] (2026-02-18) Dockerfile.frontend: node:22-alpine build → nginx:alpine serve with SPA routing + API proxy
- [x] (2026-02-18) frontend/nginx.conf: try_files for SPA, proxy_pass to core:8080 for /api/ and /ws
- [x] (2026-02-18) .dockerignore: exclude .venv, node_modules, .git, data/, __pycache__
- [x] (2026-02-18) docker-compose.prod.yml: 6 services (core, worker, frontend, postgres, nats, litellm) (Named volumes, health checks, restart: unless-stopped, tuned PostgreSQL 256MB shared_buffers)
- [x] (2026-02-18) .github/workflows/docker-build.yml: 3 parallel jobs (core, worker, frontend) (ghcr.io push, branch/semver/sha tags, Docker layer cache)

---

### Phase 9+ — Advanced Features & Vision

#### Roadmap/Feature Map (Advanced)

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

#### Version Control

- [x] (2026-02-19) SVN integration — svn adapter (`internal/adapter/svn/`) implementing gitprovider.Provider via svn CLI (checkout, update, status, info, ls branches)
- [x] (2026-02-19) Gitea/Forgejo support — gitea PM adapter (`internal/adapter/gitea/`) implementing full pmprovider.Provider via REST API (list, get, create, update issues)

#### Protocols

- [x] (2026-02-19) A2A protocol stub (agent discovery via `/.well-known/agent.json`, task create/get via `/a2a/tasks`, AgentCard with 2 skills)
- [x] (2026-02-19) AG-UI protocol event types (8 event types: run_started, run_finished, text_message, tool_call, tool_result, state_delta, step_started, step_finished)

#### Integrations

- [x] (2026-02-19) GitHub/GitLab VCS Webhook system — VCSWebhookService processes push/PR events, broadcasts via WebSocket, HMAC-SHA256 signature verification
- [x] (2026-02-19) Webhook notifications (Slack, Discord) — NotificationService fan-out, Slack Block Kit adapter, Discord embed adapter, self-registering via notifier registry
- [x] (2026-02-19) PM Webhook Sync — PMWebhookService for GitHub Issues, GitLab Issues, Plane.so webhook events, normalized to PMWebhookEvent

#### Cost & Monitoring

- [x] (2026-02-18) Phase 7: Cost & Token Transparency — Full implementation (Feature 1: real cost calculation in Python workers with LiteLLM `x-litellm-response-cost` header extraction in `workers/codeforge/llm.py`, fallback pricing table in `configs/model_pricing.yaml` and `workers/codeforge/pricing.py`, real cost passed to runtime in `workers/codeforge/executor.py`, 7 new Python tests; Feature 2: token persistence in database with migration 015 adding `tokens_in`, `tokens_out`, `model` columns on runs table, domain fields `TokensIn`, `TokensOut`, `Model` on `run.Run`, store/NATS payloads/Python models/RuntimeService all updated, Python runtime token accumulators `_total_tokens_in`, `_total_tokens_out`, `_model`; Feature 3: cost aggregation API with 5 endpoints for domain `internal/domain/cost/cost.go` Summary/ProjectSummary/ModelSummary/DailyCost, store with 5 SQL aggregation queries, service `internal/service/cost.go` CostService, HTTP GET /costs, GET /projects/{id}/costs, GET /projects/{id}/costs/by-model, GET /projects/{id}/costs/daily, GET /projects/{id}/costs/runs; Feature 4: WS budget alerts + token events with `RunStatusEvent` extended with `tokens_in`, `tokens_out`, `model`, `BudgetAlertEvent` type fires at 80% and 90% of MaxCost, dedup via `sync.Map` keyed by `"runID:threshold"`, 3 budget alert tests; Feature 5: frontend cost dashboard + enhancements with types CostSummary/ProjectCostSummary/ModelCostSummary/DailyCost/BudgetAlertEvent, API client `api.costs` namespace 5 methods, `CostDashboardPage` global totals + project breakdown table, `ProjectCostSection` per-project cost cards + model breakdown + daily cost bars + recent runs, route `/costs` with nav link, RunPanel tokens + model display, ProjectDetailPage budget alert banner + cost section)
- [x] (2026-02-19) OpenTelemetry — real TracerProvider + MeterProvider with OTLP gRPC exporters, HTTP middleware, span helpers, metric instruments

#### Operations

- [x] (2026-02-18) Backup & disaster recovery strategy (`scripts/backup-postgres.sh` pg_dump with custom format, compression, retention cleanup; `scripts/restore-postgres.sh` restore from file or latest, drop-and-recreate; Docker Compose WAL config `wal_level=replica`, `archive_mode=on` for future PITR; documented in `docs/dev-setup.md` backup, restore, cron, WAL archiving)
- [x] (2026-02-19) Blue-Green deployment — Traefik config, docker-compose overlay, deploy script with health checks and rollback
- [x] (2026-02-19) Multi-tenancy (full, beyond soft-launch) — see 3H for details

#### CI/CD & Tooling

- [x] (2026-02-18) Pre-commit & linting hardening — security analysis, anti-pattern detection, import sorting (Python: 10 ruff rule groups S, C4, C90, PERF, PIE, RET, FURB, LOG, T20, PT with mccabe threshold 12; Go: 8 new golangci-lint linters gosec, bodyclose, noctx, errorlint, revive, fatcontext, dupword, durationcheck; TypeScript: strict + stylistic ESLint configs, eslint-plugin-simple-import-sort; pre-commit: ruff-pre-commit bumped to v0.15.1, ESLint hook fixed for local binary; all violations fixed across 31 files)
- [x] (2026-02-18) GitHub Actions workflow: build Docker images (3 parallel jobs, ghcr.io, layer cache)
- [x] (2026-02-19) Branch protection rules for `main` (GitHub) (`scripts/setup-branch-protection.sh`: configurable via `gh` CLI; requires PR with 1 approving review, dismiss stale reviews; required status checks: Go, Python, Frontend CI jobs; require up-to-date branches, linear history, conversation resolution; no force pushes or branch deletion)
- [x] (2026-02-19) Branch protection rules (domain feature) — ProtectionRule model with glob pattern matching, EvaluatePush/EvaluateMerge/EvaluateDelete, CRUD REST API, migration 020, tenant-aware store methods, optimistic locking

---

### Phase 10 — Frontend Foundations (Cross-Cutting)

> These foundations must be built BEFORE adding more feature UIs.
> They affect every component and are cheaper to retrofit now than later.

#### 10A. Theme System (Dark/Light Mode Toggle)

- [x] (2026-02-19) Define CSS custom properties for color tokens (bg-primary, bg-surface, text-primary, border, etc.) (`:root` and `.dark` blocks in `index.css` with 17 design tokens each; surface, border, text, accent, and agent status tokens)
- [x] (2026-02-19) Implement Tailwind `dark:` variants on all existing components (~18 files) (`@custom-variant dark (&:where(.dark, .dark *));` for Tailwind CSS v4 class-based dark mode; all components: App, Dashboard, ProjectCard, CostDashboard, ModelsPage, AgentPanel, TaskPanel, LiveOutput, RunPanel, PlanPanel, PolicyPanel, ProjectDetailPage, RepoMapPanel, RetrievalPanel, RoadmapPanel, TrajectoryPanel, Toast)
- [x] (2026-02-19) Theme toggle component in Sidebar (`ThemeProvider.tsx`: ThemeProvider context + ThemeToggle button with sun/moon/gear icons; cycles through light -> dark -> system)
- [x] (2026-02-19) localStorage persistence for user preference (key: `codeforge-theme`)
- [x] (2026-02-19) System preference detection via `prefers-color-scheme` media query (`window.matchMedia` listener for real-time system preference changes)
- [x] (2026-02-19) Formalize 5-color agent status schema as design tokens (`--cf-status-running` blue, `--cf-status-idle` green, `--cf-status-waiting` yellow, `--cf-status-error` red, `--cf-status-planning` blue)
- [x] (2026-02-19) Customization hooks for branding (CSS custom property overrides in `:root`)

#### 10B. i18n (Internationalization)

- [x] (2026-02-19) Evaluate i18n approach: custom signal-based context provider (no external dependency) (`I18nProvider` with `createContext`/`useContext`, `createSignal` for locale + translations; flat dot-separated keys `TranslationKey = keyof typeof en`, type-safe at compile time; `{{variable}}` interpolation via `String.replaceAll()`; lazy bundle loading via dynamic `import()` with Vite auto code-splits non-English bundles; `localStorage` persistence key: `codeforge-locale`, browser language auto-detection)
- [x] (2026-02-19) Extract all hardcoded UI strings from ~20 components into key-based bundles (~480 keys) (Shell components: App.tsx, ThemeProvider, OfflineBanner, CommandPalette, Toast; dashboard: DashboardPage, ProjectCard; costs: CostDashboardPage; models: ModelsPage; project panels: AgentPanel, TaskPanel, RunPanel, PlanPanel, LiveOutput, TrajectoryPanel; project detail: ProjectDetailPage, PolicyPanel, RepoMapPanel, RetrievalPanel, RoadmapPanel)
- [x] (2026-02-19) Language bundles: English (default) + German (code-split, 19 kB chunk) (`frontend/src/i18n/en.ts` ~480 translation keys source of truth; `frontend/src/i18n/locales/de.ts` full German translation)
- [x] (2026-02-19) Language switcher in Sidebar (LocaleSwitcher component, cycles EN/DE)
- [x] (2026-02-19) Locale-aware date/number formatting (Intl.DateTimeFormat, Intl.NumberFormat) (`frontend/src/i18n/formatters.ts`: 9 formatter functions date, dateTime, time, number, compact, currency, duration, score, percent; `frontend/src/i18n/context.tsx`: `fmt` object on I18nContextValue, reactive locale binding; replaced inline formatters in 8 components: ProjectCard, CostDashboardPage, TaskPanel, RunPanel, RepoMapPanel, RetrievalPanel, TrajectoryPanel, ProjectDetailPage; removed 4 duplicate local formatters formatDate, formatNumber x2, formatCost x2, formatTokens)
- [x] (2026-02-19) Pluralization support via `tp()` function using `Intl.PluralRules` (`tp(key, count, params?)` resolves `key_one`, `key_other` etc. based on CLDR plural rules; `Intl.PluralRules` instances cached per locale for performance; count automatically injected as `{{count}}` parameter; 12 plural keys added to en.ts: costs.runs, costs.steps, run.steps, trajectory.events/toolCalls/errors _one/_other; 12 plural keys added to de.ts matching; updated 3 components: CostDashboardPage 2 locations, RunPanel 2 locations, TrajectoryPanel 3 locations)

#### 10C. Authentication & Authorization

- [x] (2026-02-19) Auth strategy: JWT (HS256 via stdlib crypto) + httpOnly refresh cookie (Access token in JS memory signal 15min, refresh token in httpOnly Secure SameSite=Strict cookie 7d; auth disabled by default `auth.enabled: false`, default admin context injected for backward compatibility)
- [x] (2026-02-19) Go Core: Auth middleware (after RequestID, before TenantID in chain) (`internal/middleware/auth.go`: JWT/API key validation, public path exemption /health, /ws, /auth/login, /auth/refresh; `internal/middleware/rbac.go`: `RequireRole(roles...)` middleware factory; modified `internal/middleware/tenant.go`: extracts tenant from user claims, fallback to X-Tenant-ID header)
- [x] (2026-02-19) User domain model (`internal/domain/user/`) (`user.go`: User struct, Role admin/editor/viewer, CreateRequest, LoginRequest, LoginResponse, TokenClaims, Validate(); `apikey.go`: APIKey struct, CreateAPIKeyRequest, CreateAPIKeyResponse, `cfk_` prefix)
- [x] (2026-02-19) Database migration: `022_create_users_api_keys.sql` (`users` table with bcrypt password hash, email+tenant_id unique; `refresh_tokens`; `api_keys` with SHA-256 key hash)
- [x] (2026-02-19) Role-Based Access Control: admin, editor, viewer (Admin-only routes: /api/v1/users/* CRUD, RequireRole middleware)
- [x] (2026-02-19) API Key support for headless/CI access (`X-API-Key` header) (SHA-256 hashed in DB, `cfk_` prefix, checked before Bearer token in middleware)
- [x] (2026-02-19) Auth service (`internal/service/auth.go`) (Register, Login, RefreshTokens rotation, Logout, ValidateAccessToken, ValidateAPIKey; CreateAPIKey, ListAPIKeys, DeleteAPIKey, User CRUD, SeedDefaultAdmin; JWT HS256 via stdlib `crypto/hmac` + `crypto/sha256` with no third-party JWT library)
- [x] (2026-02-19) Store interface + Postgres implementation (14 new methods in `internal/port/database/store.go` User 6, RefreshToken 4, APIKey 4; `store_user.go`, `store_refresh_token.go`, `store_api_key.go`)
- [x] (2026-02-19) HTTP handlers (`internal/adapter/http/handlers_auth.go`) (11 handler methods: Login, Refresh, Logout, GetCurrentUser, CRUD API keys, CRUD users admin; routes in `routes.go`, CORS headers updated for auth)
- [x] (2026-02-19) Config: `internal/config/config.go` Auth struct, 7 CODEFORGE_AUTH_* env overrides
- [x] (2026-02-19) Frontend: Auth Context provider, protected route guard (`AuthProvider.tsx`: SolidJS context with signals, login/logout, auto-refresh scheduling, session restore; `RouteGuard.tsx`: redirect to /login if not authenticated; `RoleGate.tsx`: conditional render by role)
- [x] (2026-02-19) Frontend: Login page, logout flow, session refresh (`frontend/src/features/auth/LoginPage.tsx`: email/password form, error handling, redirect; `frontend/src/api/client.ts`: setAccessTokenGetter, Authorization header, credentials: "include")
- [x] (2026-02-19) Frontend: Show current user in Sidebar, role-based UI element visibility (`App.tsx`: AuthProvider in provider chain, UserInfo component in sidebar)
- [x] (2026-02-19) i18n: ~27 auth keys in en.ts and de.ts
- [x] (2026-02-19) Tests: domain validation, auth service (7 tests), auth middleware (4 tests), RBAC middleware (4 tests)
- [x] (2026-02-19) Connects to existing Multi-Tenancy (Phase 3H tenant_id)

#### 10D. WCAG 2.2 Conformance (Level AA)

- [x] (2026-02-19) Audit all existing components against WCAG 2.2 AA criteria
- [x] (2026-02-19) `aria-label` / `aria-labelledby` on all interactive elements (buttons, inputs, selects)
- [x] (2026-02-19) Focus management: visible focus rings (`:focus-visible`), focus trap in modals/dialogs
- [x] (2026-02-19) Keyboard navigation: correct Tab order, Enter/Space for buttons, Escape for close
- [x] (2026-02-19) Color contrast: minimum 4.5:1 ratio for all text/background combinations (depends on 10A Theme)
- [x] (2026-02-19) Screen reader support: landmark regions (`<main>`, `<nav>`, `<aside>`), `aria-live` for real-time updates
- [x] (2026-02-19) Skip-to-content link as first focusable element
- [x] (2026-02-19) Form accessibility: explicit `<label>` associations, error announcements, required field indicators
- [x] (2026-02-19) Motion: `prefers-reduced-motion` media query for animations
- [x] (2026-02-19) Test with axe-core / Playwright accessibility audit in E2E tests (`@axe-core/playwright` integrated as devDependency; `frontend/e2e/a11y.spec.ts`: WCAG 2.2 AA checks on Dashboard, Costs, Models, Login pages; tags: `wcag2a`, `wcag2aa`, `wcag22aa`)

#### 10E. Keyboard Shortcuts (Command Palette)

- [x] (2026-02-19) Global hotkey handler: Ctrl+K / Cmd+K for Command Palette overlay
- [x] (2026-02-19) Navigation shortcuts: Ctrl+1 Dashboard, Ctrl+2 Costs, Ctrl+3 Models
- [x] (2026-02-19) Action shortcuts: Escape close, Enter select, Arrow keys navigate
- [x] (2026-02-19) Shortcut help overlay: Ctrl+/ opens palette with all shortcuts visible
- [x] (2026-02-19) Zero external dependencies — SolidJS `onMount`/`onCleanup` + native KeyboardEvent
- [x] (2026-02-19) Files: `App.tsx` (integration), new `frontend/src/components/CommandPalette.tsx`
- [x] (2026-02-19) WCAG: `role="dialog"`, `aria-modal`, `role="combobox"` + `role="listbox"` + `role="option"`, `aria-activedescendant`, `aria-selected` (Commands: Go to Dashboard, Go to Costs, Go to Models, Toggle Theme, Show Keyboard Shortcuts; fuzzy search filtering, section grouping Navigation, Actions, Theme; platform-aware modifier key display Cmd on Mac, Ctrl elsewhere)

#### 10F. Toast/Notification System

- [x] (2026-02-19) Global toast container (top-right, max 3 simultaneous, stacked)
- [x] (2026-02-19) Toast types: success (green), error (red), warning (yellow), info (blue)
- [x] (2026-02-19) Auto-dismiss (5s default), manually closable
- [x] (2026-02-19) Zero external dependencies — SolidJS `createSignal` + `Portal`
- [x] (2026-02-19) Integration points: API errors, WebSocket events (budget alerts, run complete, agent status)
- [x] (2026-02-19) WCAG: `aria-live="polite"` for info/success, `role="alert"` for errors
- [x] (2026-02-19) Files: new `frontend/src/components/Toast.tsx`, `App.tsx` (provider)
- [x] (2026-02-19) Wire toasts into all 7 API-calling panels (DashboardPage: project create/delete success/error; ProjectDetailPage: budget alerts warning, run/plan completion info, git operations success/error; RunPanel: run start/cancel success/error; AgentPanel: agent create/delete/dispatch/stop success/error; RoadmapPanel: roadmap create/delete, import specs/PM, milestone/feature create success/error; TaskPanel: task create success/error; ModelsPage: model add/delete success/error)

#### 10G. Error Boundary + Offline Detection

- [x] (2026-02-19) SolidJS `ErrorBoundary` around App root with fallback UI + retry button
- [x] (2026-02-19) Online/Offline detection: `navigator.onLine` + WebSocket connection status
- [x] (2026-02-19) Visible reconnect banner when connection lost (WebSocket already has auto-reconnect)
- [x] (2026-02-19) API client: retry logic with exponential backoff (max 3 retries, 1s/2s/4s)
- [x] (2026-02-19) Graceful degradation: show cached data when offline, queue actions for retry (`frontend/src/api/cache.ts`: in-memory GET response cache 5min TTL, offline mutation queue with auto-process on reconnect; `frontend/src/api/client.ts`: GET responses cached on success, served from cache on network failure; mutations queued when offline, auto-retried via `window.addEventListener("online")`)
- [x] (2026-02-19) Files: `App.tsx`, `api/client.ts`, new `frontend/src/components/OfflineBanner.tsx`

#### Dependencies

```text
10C Auth ──────────────────────┐
10A Theme ─── 10D WCAG ────────┤
10B i18n  ─────────────────────┤──> All further feature UIs
10F Toast ─── 10G Error ───────┤
10E Keyboard ──────────────────┘
```

---

### Phase 11+ — Future GUI Enhancements

> Items from the "Glass Cockpit" GUI design review.
> These are valuable but too early for the current project phase.
> Prerequisites: Phase 10 foundations + working multi-agent orchestration.

#### Visual Enhancements (requires graph rendering library)

- [x] (2026-02-19) Architecture graph / pulsating project blueprint visualization (`ArchitectureGraph` SVG component on Context tab, queries GraphRAG search API; force-directed layout with repulsion, attraction, center gravity, animated convergence; nodes colored by kind module/class/function/method, sized by importance; hover highlights connected edges, labels appear, non-connected nodes dim; seed symbol input, configurable hop depth, raw results collapsible; 18 new i18n keys EN + DE)
- [x] (2026-02-19) Agent network visualization (nodes + edges, message flow animation) (`AgentNetwork` SVG component on Agents tab, circle layout from team members; nodes colored by role coder/reviewer/tester/documenter/planner with status ring; WS event listener animates message flow between agents pulse + arrow; team selector, role legend, message flow log; 11 new i18n keys EN + DE)
- [x] (2026-02-19) Step-progress indicators replacing time estimates (`StepProgress` reusable component: progress bar + fraction label current/max; color coding: blue <70%, yellow 70-90%, red >90%, indeterminate when max unknown; ARIA progressbar role with aria-valuenow/max, locale-aware labels; RunPanel: shows progress bar during active runs max from policy termination.max_steps; PlanPanel: shows progress bar for running plans completed steps / total steps; 5 i18n keys EN + DE)

#### Advanced Layouts

- [x] (2026-02-19) Split-screen feature planning (prompt left + generated plan preview right) (PlanPanel decompose form stores result in signal instead of discarding; responsive split layout: single column on small screens, side-by-side on lg+; right panel shows plan name, description, protocol, step list with dependencies; accept/discard buttons to confirm or re-try the decomposition; 8 new i18n keys EN + DE for plan preview)
- [x] (2026-02-19) ProjectDetailPage tab navigation (Overview | Tasks & Roadmap | Agents & Runs | Context | Costs) (5 tabs replace vertically stacked layout; tab bar with `role="tablist"`, `aria-selected`, `aria-controls` for a11y; i18n keys: `detail.tab.overview`, `.tasks`, `.agents`, `.context`, `.costs` EN + DE; Overview: Git status, branches, clone/pull actions; Tasks & Roadmap: TaskPanel + RoadmapPanel; Agents & Runs: AgentPanel + PolicyPanel + RunPanel + PlanPanel + LiveOutput; Context: RepoMapPanel + RetrievalPanel requires workspace; Costs: ProjectCostSection)
- [x] (2026-02-19) Multi-terminal view with tiles per agent (`MultiTerminal` component with per-agent terminal tiles in responsive grid; auto-scroll, expand/collapse single tile to full width, max line truncation; ProjectDetailPage tracks output per agent_id, shows MultiTerminal when 2+ agents active; falls back to single LiveOutput when only one agent has output; 7 new i18n keys EN + DE for multi-terminal)
- [x] (2026-02-19) Global activity/notification stream (cross-project, not just per-project) (`ActivityPage` component at `/activity` with global WebSocket subscription; classifies 12+ WS event types run.status, run.toolcall, run.budget_alert, run.qualitygate, run.delivery, agent.status, task.status, plan.status, plan.step.status, repomap/retrieval/roadmap.status; severity-tagged entries info/success/warning/error with color-coded badges; type icons for visual differentiation, filter by event type, pause/resume/clear; max 200 entries newest first, project links, ARIA role="log" + aria-live="polite"; 12 i18n keys EN + DE, sidebar nav link)

#### Developer Tools

- [x] (2026-02-19) Vector search simulator / "What does the agent know?" debug tool (`SearchSimulator` component on Context tab, uses hybrid + agent + graph search APIs; adjustable BM25/semantic weight sliders, top-K, token budget; token budget progress bar with per-result token estimation and budget fit indicator; agent mode toggle query expansion, optional GraphRAG cross-reference; results colored green/red based on budget fit, BM25 + semantic rank columns; 28 new i18n keys EN + DE for simulator)
- [x] (2026-02-19) Diff-review / code preview for agent output (before/after comparison) (`DiffPreview` reusable component: parses unified diff into files/hunks/lines; color-coded: green additions, red removals, blue hunk headers, gray context; line numbers old/new, collapsible per-file sections, +/- counts per file; auto-detected in TrajectoryPanel EventDetail: checks .diff, .patch, .output for diff content; also renders in non-tool events e.g. delivery events with patch content; 2 i18n keys EN + DE)
- [x] (2026-02-19) Trajectory replay / inspector with step-by-step playback (Replay mode toggle in TrajectoryPanel: scrubber bar, play/pause, step prev/next; 4 playback speeds 0.5x, 1x, 2x, 4x with cycle button; mini timeline dots colored by event type, highlighting played events; enhanced EventDetail component: structured tool call/result display tool name, input, output, errors; browse mode existing enhanced with blue highlight on expanded events; 13 new i18n keys EN + DE, ARIA labels for all controls)

#### Missing UI for Existing Backend Features

- [x] (2026-02-19) Settings/Configuration page (Provider info cards Git, Agent, Spec, PM with loading/empty states; LLM health status indicator connected/unavailable/checking; API key management: create, list, delete, copy warning for new keys; user management table admin only: enable/disable, delete, role badges; ~38 i18n keys in EN + DE, route `/settings`, nav link in sidebar; reuses existing backend endpoints: providers.*, auth.*, users.*, llm.health)
- [x] (2026-02-19) Mode selection UI (architect, coder, reviewer, debugger, etc.) (ModesPage at `/modes` with card grid for all 8 built-in + custom modes; create custom mode form id, name, description, tools, scenario, autonomy, prompt; mode cards: tool badges, LLM scenario, autonomy level with color coding, expandable prompt; built-in modes protected from overwrite, sorted before custom; ~35 i18n keys EN + DE, sidebar nav link, route wired)
- [x] (2026-02-19) Team/Multi-Agent management UI — backend ready via `api.teams` (TeamsPage at `/teams` with project selector, team creation form, and team list; create team: name, protocol round-robin/pipeline/parallel/consensus/ping-pong, member assignment agent + role; team cards: status badge, protocol badge, member count, expandable detail with member list and shared context; shared context viewer: key/value items with author and token count; 5 team roles: coder, reviewer, tester, documenter, planner color-coded badges; ~30 i18n keys EN + DE, sidebar nav link, route wired)

---

### Documentation TODOs

- [x] (2026-02-18) Create ADR for Config Hierarchy (`docs/architecture/adr/003-config-hierarchy.md`)
- [x] (2026-02-18) Create ADR for Async Logging (`docs/architecture/adr/004-async-logging.md`)
- [x] (2026-02-18) Create ADR for Docker-Native Logging (`docs/architecture/adr/005-docker-native-logging.md`)
- [x] (2026-02-18) Create ADR for Agent Execution (Approach C) (`docs/architecture/adr/006-agent-execution-approach-c.md`)
- [x] (2026-02-18) Create ADR for Policy Layer (`docs/architecture/adr/007-policy-layer.md`)
- [x] (2026-02-18) Update `docs/architecture.md` with new patterns (Infrastructure Patterns section: Circuit Breaker, Cache Layer, Idempotency, Rate Limiting; Agent Execution section: Policy Layer, Runtime API, Checkpoint System, Docker Sandbox; Observability section: Event Sourcing, Structured Logging, Configuration)
- [x] (2026-02-18) Update `docs/dev-setup.md` with logging section (Docker Compose log commands, log level config, Request ID propagation, helper script, log rotation)
- [x] (2026-02-18) Update `CLAUDE.md` with new principles (ADR index, Infrastructure Principles section; config hierarchy, async-first concurrency, Docker-native logging, Policy Layer, Approach C, resilience patterns)

---

### Testing Requirements

- [x] (2026-02-18) E2E test infrastructure (Playwright) (Playwright config, 5 test files 17 tests, fixtures with API helper; tests: health checks, sidebar navigation, project CRUD, costs, models; Vite dev proxy fix `/health` not proxied, `scripts/test.sh e2e` command; ESLint/tsconfig updated to include `e2e/`, `.gitignore` updated for artifacts)
- [x] (2026-02-17) Test runner script (`scripts/test.sh`) — unified Go/Python/Frontend/Integration runner
- [x] (2026-02-17) Integration test infrastructure (`tests/integration/`) — real PostgreSQL, build-tagged (Health/liveness tests, Project CRUD lifecycle, Task CRUD lifecycle, validation tests; fixed goose migration `$$` blocks StatementBegin/StatementEnd annotations; updated `.claude/commands/test.md` to use test runner script)
- [x] (2026-02-17) Unit tests for AsyncHandler (buffer overflow, concurrent writes, flush) — 4 tests in `internal/logger/async_test.go`
- [x] (2026-02-19) Integration tests for Config Loader (precedence, validation, reload) (`internal/config/loader_integration_test.go`: 10 tests covering full hierarchy, partial override, invalid env, missing YAML, malformed YAML, validation, orchestrator overrides, reload, reload validation failure, reload env override)
- [x] (2026-02-17) Unit tests for Idempotency (no header, store, replay, GET ignored, different keys) — 5 tests in `internal/middleware/idempotency_test.go`
- [x] (2026-02-19) Load tests for Rate Limiting (sustained vs burst, per-user limiters) (`tests/load/ratelimit_test.go` build tag `//go:build load`; 6 tests: sustained load, burst absorption, per-IP isolation, concurrent bucket creation, headers, cleanup under load; run with: `go test -tags load -count=1 ./tests/load/`)
- [x] (2026-02-18) Runtime Compliance Tests (Sandbox/Mount feature parity) — 16 sub-tests passing
- [x] (2026-02-17) Policy Gate tests (deny/ask/allow evaluation, path scoping, command matching, preset integration)
- [ ] E2E live tests: Create test users via API and test auth flows end-to-end (login, JWT refresh, role-based access, API key creation/usage, forced password change for seeded admin, password complexity enforcement, logout, session expiry). **Must use Playwright MCP** (`browser_navigate`, `browser_snapshot`, `browser_fill_form`, `browser_click`, etc.) to test all UI/UX flows through the actual browser. If Playwright MCP is not reachable, inform the user and abort the test — do not fall back to API-only testing for UI validation. **Security testing must follow [OWASP Top 10 (2025)](https://owasp.org/Top10/2025/) and [OWASP Web Security Testing Guide (WSTG)](https://owasp.org/www-project-web-security-testing-guide/latest/):** test for injection (SQL, XSS, command), broken authentication (session fixation, credential stuffing, brute force), broken access control (IDOR, privilege escalation, path traversal), security misconfiguration (CORS, headers, error disclosure), SSRF, cryptographic failures, and CSRF. Document each finding with WSTG test ID reference.

---

### Recently Completed

> Move items here after completion for context. Periodically archive old items.

- [x] (2026-02-14) Phase 2 completed: MVP Features (WP1: Git Local Provider; WP2: Agent Lifecycle; WP3: WebSocket Events; WP4: LLM Provider Management; WP5-7: Frontend pages; WP8: Integration test + docs; Go: 27 tests, gitlocal provider, aider backend, agent service, LiteLLM client, 19 REST endpoints; Python: 16 tests, streaming output via NATS, LiteLLM health checks; Frontend: 13 components, 4 routes /, /projects, /projects/:id, /models, WebSocket live updates)
- [x] (2026-02-14) Phase 1 completed: Infrastructure, Go Core, Python Workers, Frontend, CI/CD (Docker Compose: PostgreSQL, NATS JetStream, LiteLLM Proxy; Go: Hexagonal architecture, REST API, WebSocket, NATS, PostgreSQL; Python: NATS consumer, LiteLLM client, Pydantic models, 16 tests; Frontend: SolidJS dashboard, API client, WebSocket, health indicators; CI: GitHub Actions Go + Python + Frontend)
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

### Post-Phase 11: Security Hardening (P0 Audit Fixes)

- [x] (2026-02-19) P0-1: Prompt injection defense in MetaAgent decomposition (`sanitizePromptInput()` strips control chars, role markers, enforces length limit; data-boundary instruction added to system prompt; 6 unit tests in `internal/service/sanitize_test.go`)
- [x] (2026-02-19) P0-2: Secret redaction utilities for safe logging (`Vault.Redacted()` masks secret values first 2 chars + "****"; `Vault.RedactString()` scrubs secrets from arbitrary strings; `Vault.Keys()` returns key names without values; 4 new tests in `internal/secrets/vault_test.go`)
- [x] (2026-02-19) P0-3: Wire audit trail into RuntimeService lifecycle (`appendAudit()` helper wired into 8 lifecycle points: start, complete, cancel, policy deny, quality gate pass/fail/error, delivery success/fail, budget exceeded)
- [x] (2026-02-19) P0-4: Quality gate fail-closed on NATS publish failure (NATS publish failure now finalizes run as failed instead of silently passing)
- [x] (2026-02-19) P0-5: Post-execution budget enforcement (Immediate budget check in `HandleToolCallResult` after cost accumulation; prevents budget overrun from single expensive tool calls)

### Post-Phase 11: Security Hardening (P1 + P2 Audit Fixes)

- [x] (2026-02-19) P1-1: GitLab PM adapter (`internal/adapter/gitlab/provider.go`: REST API v4 with PRIVATE-TOKEN auth; ListItems, GetItem, CreateItem, UpdateItem using `/api/v4/projects/:id/issues`; self-registering via `init()` in `register.go`, imported in `cmd/codeforge/providers.go`; 8 table-driven tests with `httptest.NewServer` mocks)
- [x] (2026-02-19) P1-2: Prompt injection hardening — text/template for meta-agent (`internal/service/templates/decompose_system.tmpl` + `decompose_user.tmpl`; `buildDecomposePrompt()` uses `//go:embed` + `text/template` instead of hardcoded strings; `sanitizePromptInput()` applied to user data before template rendering)
- [x] (2026-02-19) P1-3: BM25-inspired file relevance scoring (`ScoreFileRelevance()` in `context_optimizer.go` replaced with BM25 algorithm k1=1.5, b=0.75; same function signature and 0-100 output range for backward compatibility)
- [x] (2026-02-19) P1-4: Branch protection default-DENY (`EvaluatePush/Merge/Delete` now deny when enabled rules exist but none match; zero rules → allow backward compat; all rules disabled → allow)
- [x] (2026-02-19) P1-5: WebSocket authentication (`/ws` removed from public paths; JWT via `?token=` query parameter; auth-disabled mode still accepts all connections backward compat)
- [x] (2026-02-19) P1-6: JWT standard claims (jti, aud, iss) + revocation (`signJWT()` includes JTI UUID, audience "codeforge", issuer "codeforge-core"; revocation via PostgreSQL `revoked_tokens` table migration 023; fail-open on DB error, skip check for old tokens without JTI; `StartTokenCleanup()` goroutine purges expired entries)
- [x] (2026-02-19) P1-7: Tenant UUID format validation (`X-Tenant-ID` header validated against UUID regex in tenant middleware; invalid format → 400; empty → default tenant backward compat)
- [x] (2026-02-19) P1-8: Stall re-planning with retry mechanism (`StallTracker` extended with `retryCount`, `maxRetries`, `CanRetry()`, `RecordRetry()`; `OrchestratorService.ReplanStep()` resets stalled run for re-dispatch; configurable `StallMaxRetries` in runtime config default: 2)
- [x] (2026-02-19) P2-1: API key resource-based scopes (`Scopes []string` on APIKey with constants projects:read/write, runs:read/write, etc.; `RequireScope()` middleware; nil scopes = full access backward compat for old keys; migration 023 adds `scopes TEXT[]` column to `api_keys`)
- [x] (2026-02-19) P2-2: Forced password change for seeded admin (`MustChangePassword bool` field on User; seeded admin gets `true`; auth middleware returns 403 for non-exempt paths when flag is set; `/api/v1/auth/change-password` endpoint clears the flag)
- [x] (2026-02-19) P2-3: Atomic refresh token rotation (`RotateRefreshToken()` wraps delete+insert in a PostgreSQL transaction; prevents race conditions in concurrent refresh attempts)
- [x] (2026-02-19) P2-4: Password complexity enforcement (Min 10 chars, must contain uppercase + lowercase + digit; applied to registration and password change; existing users unaffected)
- [x] (2026-02-19) P2-5: Delivery push error propagation (`PushError` field on `DeliveryResult`; `deliverPR()` skips PR creation on push failure; error surfaced in audit log and WebSocket broadcast)

---

### Phase 12+ — Architecture Evolution

> Insights from theme analysis of brainstorming sessions, role engineering research,
> and Claude Code prompt architecture study. Organized by priority.
> Sources: brainstorming conversation, `data/docs/` role research, Piebald-AI claude-code-system-prompts.

#### 12A. Mode System Extensions (P1)

- [x] (2026-02-23) Add `RequiredArtifact`, `DeniedTools`, `DeniedActions` fields to Mode domain struct, update Validate() for overlap detection, wire mode end-to-end through NATS to Python executor (Go: `mode.go`, `presets.go`, `schemas.go`, `runtime.go`, `store.go`, `main.go`; Python: `models.py`, `executor.py`, `consumer.py`; Frontend: `types.ts`, `ModesPage.tsx`, `en.ts`, `de.ts`; DB: migration 024; Tests: 16 Go tests, 5 Python tests)
- [x] (2026-02-23) Decompose prompt templates into ROLE/TOOLS/ARTIFACT/ACTIONS/GUARDRAILS sections (5 Go `text/template` files with `//go:embed`, conditional assembly via `BuildModePrompt()` in `mode_prompt.go`, custom modes bypass templates)
- [x] (2026-02-23) Add prompt token counting per section (per-section `EstimateTokens()`, `WarnIfOverBudget()` with 1024-token soft limit, 10 tests)
- [x] (2026-02-23) Implement mode-specific prompt composition (built-in modes use template assembly, custom modes use raw PromptPrefix, `runtime.go` calls `BuildModePrompt()` before NATS publish)
- [ ] Reference: Claude Code conditional prompt assembly pattern (~40 modular sections, token-budget-aware)

#### 12B. LLM Routing Implementation (P1)

- [x] (2026-02-23) Wire scenario tags to LiteLLM model selection via native tag-based routing (`litellm/config.yaml`: moved tags from `model_info.tags` to `litellm_params.tags`, added `router_settings.enable_tag_filtering`; Python: `resolve_scenario()` + `tags` param on `completion()` in `llm.py`; `executor.py` reads `mode.llm_scenario` and passes tag + temperature)
- [x] (2026-02-23) Implement tag-to-model mapping in LiteLLM proxy config — 30+ models tagged with default/background/think/longContext/review/plan; Gemini models get `longContext` tag; `ScenarioConfig` maps scenarios to temperatures (think=0.3, review=0.1, default=0.2, background=0.1, plan=0.3)
- [x] (2026-02-23) Add routing decision logging for observability — structured log in `executor.py` (run_id, mode, scenario, temperature) + debug log in `llm.py` (model, temperature, tags, prompt_len); 5 new tests in `test_llm.py`
- [ ] Reference: Tags already defined in Mode domain (`internal/domain/mode/mode.go`), LiteLLM adapter has no routing logic yet (`internal/adapter/litellm/`)

#### 12C. Role Evaluation Framework (P1)

- [x] (2026-02-23) Define role responsibility matrix (9 roles: Orchestrator, Architect, Coder, Reviewer, Security, Tester, Debugger, Proponent, Moderator) with input/output/allowed/denied columns (`workers/tests/role_matrix.py`)
- [x] (2026-02-23) Implement FakeLLM test harness for deterministic agent testing (`workers/tests/fake_llm.py`): fixture-based, call tracking, `from_fixture()` class method
- [x] (2026-02-23) Create `tests/scenarios/` fixture directory structure: 5 roles x 3 files = 15 JSON fixtures (`{role}/{scenario}/input.json`, `expected_output.json`, `llm_responses.json`)
- [x] (2026-02-23) Define 7 MVP evaluation tests (`workers/tests/test_role_evaluation.py`): architect generates plan, coder produces diff, reviewer catches bug, tester reports pass/fail, security flags risk, debate convergence, orchestrator boundary
- [x] (2026-02-23) Integrate evaluation metrics schema (`workers/tests/evaluation.py`): EvaluationMetrics dataclass (passed, tokens, steps, cost, artifact_quality)
- [ ] Research reference: SPARC-Bench, DeepEval, AgentNeo, REALM-Bench, GEMMAS

#### 12D. RAG Shared Scope System (P1)

- [x] (2026-02-23) Add Scope domain model (`internal/domain/context/scope.go`): ScopeType enum (shared/global), RetrievalScope struct, CreateScopeRequest, UpdateScopeRequest with Validate methods
- [x] (2026-02-23) DB migration (`internal/adapter/postgres/migrations/025_retrieval_scopes.sql`): retrieval_scopes + retrieval_scope_projects join table with FK cascade
- [x] (2026-02-23) Store interface + PostgreSQL implementation (`store_scope.go`): 8 scope methods (CRUD + project management)
- [x] (2026-02-23) Service layer (`internal/service/scope.go`): ScopeService with CRUD, cross-project SearchScope fan-out, SearchScopeGraph fan-out
- [x] (2026-02-23) HTTP handlers (`internal/adapter/http/handlers_scope.go`): 9 endpoints (CRUD + add/remove project + search + graph search)
- [x] (2026-02-23) Route registration, Handlers struct wiring, main.go initialization
- [x] (2026-02-23) NATS payload updates: ProjectID field on RetrievalSearchHitPayload and GraphSearchHitPayload
- [x] (2026-02-23) Python model updates: project_id field on RetrievalSearchHit and GraphSearchHit
- [x] (2026-02-23) Security constraint: projects isolated by default, explicit opt-in required for shared scopes
- [ ] Extend HybridRetriever (`workers/codeforge/retrieval.py`) to accept scope_id parameter for cross-project search (deferred - fan-out in Go)
- [ ] Extend CodeGraphBuilder (`workers/codeforge/graphrag.py`) to accept scope_id parameter (deferred - fan-out in Go)
- [ ] Implement incremental indexing with hash-based delta detection (deferred - independent feature)
- [ ] Add scope management frontend UI (scope list, project assignment, index status per scope)

#### 12E. Artifact-Gated Pipelines (P2) — COMPLETED

- [x] (2026-02-23) Go artifact domain package (`internal/domain/artifact/artifact.go`): ArtifactType enum (6 types), ValidationResult struct, Validate() dispatcher, IsKnownType(), 6 per-type structural validators (PLAN.md, DIFF, REVIEW.md, TEST_REPORT, AUDIT_REPORT, DECISION.md)
- [x] (2026-02-23) Go artifact tests (`internal/domain/artifact/artifact_test.go`): 10 table-driven test functions covering all artifact types, empty/unknown edge cases
- [x] (2026-02-23) Run domain extended (`internal/domain/run/run.go`): ArtifactType, ArtifactValid (*bool), ArtifactErrors fields
- [x] (2026-02-23) DB migration (`026_artifact_fields.sql`): artifact_type, artifact_valid, artifact_errors columns on runs table
- [x] (2026-02-23) Store interface + implementation: UpdateRunArtifact method, scanRun updated for 3 new fields, 3 SELECT queries updated (GetRun, ListRunsByTask, RecentRunsWithCost)
- [x] (2026-02-23) Event types (`event.go`): TypeArtifactValidated, TypeArtifactFailed
- [x] (2026-02-23) WS events (`events.go`): EventArtifactValidation constant + ArtifactValidationEvent struct
- [x] (2026-02-23) Runtime integration (`runtime.go`): artifact validation gate in HandleRunComplete, before quality gates — validates output against mode's RequiredArtifact, persists result, broadcasts WS event, fails run on validation failure
- [x] (2026-02-23) Python artifact models (`workers/codeforge/artifacts.py`): Pydantic-based validators mirroring Go, validate_artifact() + is_known_type()
- [x] (2026-02-23) Python artifact tests (`workers/tests/test_artifacts.py`): 35 test cases mirroring Go coverage

#### 12F. Pipeline Templates (P2) — COMPLETED

- [x] (2026-02-23) ModeID on plan steps (`plan.go`): Added ModeID field to Step + CreateStepRequest, DB migration (`027_step_mode_id.sql`), store queries updated (CreatePlan INSERT, ListPlanSteps SELECT, GetPlanStepByRunID SELECT, scanPlanStep Scan)
- [x] (2026-02-23) Orchestrator ModeID passthrough (`orchestrator.go`): startStep() now passes step.ModeID to run.StartRequest
- [x] (2026-02-23) Pipeline domain package (`internal/domain/pipeline/`): Template, Step, StepBinding, InstantiateRequest structs; Validate() with DAG validation via Kahn's algorithm; Instantiate() produces CreatePlanRequest from template + bindings
- [x] (2026-02-23) Pipeline presets (`presets.go`): 3 built-in templates — `standard-dev` (architect→coder→reviewer→tester, sequential), `security-audit` (architect→coder→security, sequential), `review-only` (reviewer+security, parallel)
- [x] (2026-02-23) Pipeline loader (`loader.go`): LoadFromFile, LoadFromDirectory (YAML, same pattern as policy/loader.go, missing dirs return nil)
- [x] (2026-02-23) Pipeline tests (`pipeline_test.go`): 22 tests — validation (10), instantiation (4), presets (2), loader (6)
- [x] (2026-02-23) Pipeline service (`internal/service/pipeline.go`): PipelineService with List, Get, Register, Instantiate; mode reference validation via ModeService
- [x] (2026-02-23) HTTP endpoints: GET/POST `/api/v1/pipelines`, GET `/api/v1/pipelines/{id}`, POST `/api/v1/pipelines/{id}/instantiate`
- [x] (2026-02-23) Wired in main.go: PipelineService created with ModeService, passed to Handlers

#### 12G. Project Workspace Management (P2) — COMPLETED 2026-02-23

- [x] (2026-02-23) Make `WorkspaceRoot` configurable via config hierarchy — `Workspace` struct in Config with `Root` (default `data/workspaces`) + `PipelineDir`, env vars `CODEFORGE_WORKSPACE_ROOT` / `CODEFORGE_WORKSPACE_PIPELINE_DIR`
- [x] (2026-02-23) Implement workspace cleanup on project delete — `Delete()` fetches project, deletes DB record, removes workspace directory if under workspace root (best-effort, logged warning on failure)
- [x] (2026-02-23) Add "adopt existing" mode for local projects — `POST /api/v1/projects/{id}/adopt` validates directory exists, sets WorkspacePath without copying/moving files
- [x] (2026-02-23) Add workspace health checks — `GET /api/v1/projects/{id}/workspace` returns exists, path, disk usage (bytes), git repo (bool), last modified
- [x] (2026-02-23) Support configurable workspace root path per tenant — Clone uses `{root}/{tenantID}/{projectID}` path pattern with tenant ID from context

#### 12H. Per-Tool Token Tracking (P2) — COMPLETED 2026-02-23

- [x] (2026-02-23) Migration 028: Added `tool_name`, `model`, `tokens_in`, `tokens_out`, `cost_usd` columns to `agent_events` with partial indexes
- [x] (2026-02-23) Extended `AgentEvent` domain type with per-tool token fields
- [x] (2026-02-23) Added `cost.ToolSummary` struct and `CostByTool`/`CostByToolForRun` store methods
- [x] (2026-02-23) Wired `HandleToolCallResult` to populate per-tool token data on events
- [x] (2026-02-23) Added `CostService.ByTool`/`ByToolForRun` delegates + HTTP handlers + routes
- [x] (2026-02-23) Extended `TrajectorySummary` with `total_tokens_in`/`total_tokens_out`/`total_cost_usd`
- [x] (2026-02-23) Added "Cost by Tool" section to frontend Cost Dashboard
- [x] (2026-02-23) Extracted `scanEvent` helper in event store to reduce 6 duplicated scan sites

#### 12I. Periodic Reviews & Audits (P2) — COMPLETED 2026-02-23

- [x] (2026-02-23) ReviewPolicy domain model with 3 trigger types: commit_count, pre_merge, cron (`internal/domain/review/review.go`)
- [x] (2026-02-23) Minimal cron parser with ParseCronExpr, NextAfter, ValidateCronExpr (`internal/domain/review/cron.go`, table-driven tests)
- [x] (2026-02-23) Migration 029: `review_policies` + `reviews` tables with partial indexes
- [x] (2026-02-23) PostgreSQL store: 13 methods for CRUD, counter management, review lifecycle (`internal/adapter/postgres/store_review.go`)
- [x] (2026-02-23) ReviewService with CRUD, push/pre-merge/cron trigger logic, plan callback (`internal/service/review.go`)
- [x] (2026-02-23) Orchestrator `SetOnPlanComplete` callback wired to review status updates
- [x] (2026-02-23) VCS webhook push/PR integration: auto-triggers commit_count and pre_merge reviews
- [x] (2026-02-23) HTTP API: 8 endpoints for review policies and reviews
- [x] (2026-02-23) WebSocket `review.status` event + 3 domain event types
- [x] (2026-02-23) Frontend: TypeScript types, API client, i18n keys

#### 12J. Project Creation Wizard (P3) — COMPLETED 2026-02-23

- [x] (2026-02-23) Language/framework auto-detection on project creation (scan repo for package.json, go.mod, pyproject.toml, Cargo.toml, etc.) — `internal/domain/project/scan.go`: `ScanWorkspace()` reads top-level dir entries, matches against `manifestMap`, reads manifest content (capped 64KB) for framework detection
- [x] (2026-02-23) Propose linter/formatter standards based on detected stack — `internal/domain/project/stackmap.go`: static `toolRecommendations` map (go→golangci-lint/gofmt, ts→eslint/prettier, python→ruff/pytest, rust→clippy/rustfmt, etc.)
- [x] (2026-02-23) Suggest default mode and pipeline based on project type — `coreModeRecommendations()` (coder, reviewer, tester, security, architect) + `corePipelineRecommendations()` (standard-dev, review-only) for all languages
- [x] (2026-02-23) API endpoints: `GET /projects/{id}/detect-stack`, `POST /detect-stack` — `internal/adapter/http/handlers.go`, `routes.go`
- [x] (2026-02-23) Frontend: "Detect Stack" button on ProjectCard, detection results panel with language badges + recommendation chips — `DashboardPage.tsx`, `ProjectCard.tsx`
- [x] (2026-02-23) 16 tests: 9 scan tests (7 project types + nonexistent + not-a-dir) + 7 stackmap validation tests

#### 12K. Knowledge Bases (P3)

- [ ] Curated knowledge modules for common frameworks (React, Go stdlib, Python stdlib, etc.)
- [ ] Programming paradigm standards (SOLID, Clean Architecture, DDD patterns)
- [ ] Implement as pre-built retrieval indexes that can be attached to project scopes
- [ ] Ties into RAG Shared Scope system (12D) — knowledge bases are a type of shared scope with type "global"

#### Dependencies

```text
12A Mode Extensions ──┐
12B LLM Routing ──────┤──> 12F Pipeline Templates
12C Role Evaluation ──┤
12D RAG Scopes ───────┼──> 12K Knowledge Bases
12E Artifact Pipes ───┤──> 12I Periodic Reviews
12G Workspace Mgmt ───┘
12H Token Tracking (independent)
12J Project Wizard (independent, benefits from 12A + 12G)
```

---

### Notes

- Phases 0-11 complete. All phases implemented. P0-P2 security hardening complete.
- **Phase 12+ Dependencies:** Mode Extensions + LLM Routing + Role Evaluation → Pipeline Templates; RAG Scopes → Knowledge Bases; Artifact Pipes → Periodic Reviews
- **Completed Dependencies:** Structured Logging → Request ID → Docker Logging → Log Script
- Completed: Event Sourcing → Policy Layer → Runtime API → Headless Autonomy
- Completed: Repo Map → Hybrid Retrieval → Retrieval Sub-Agent → GraphRAG
- Completed: Roadmap Domain → Store → Service → Handlers → Frontend
- Completed: Auth → Multi-Tenancy full rollout; Theme → WCAG audit
- Testing: Each new pattern requires unit + integration tests before merge
- Documentation: ADRs must be written before implementation (capture decision context)
- Source: Analysis document `docs/Analyse des CodeForge-Projekts (staging-Branch).md`
- Source: Role engineering research `data/docs/` + Claude Code prompt architecture (Piebald-AI)
