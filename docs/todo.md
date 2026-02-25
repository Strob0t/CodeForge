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
- [x] (2026-02-24) AG-UI protocol integration — events emitted in runtime.go (tool_call, tool_result, run_started, run_finished) + conversation.go (text_message); frontend websocket.ts AG-UI event handlers; ChatPanel streaming display

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
- [x] (2026-02-24) E2E live tests: Auth flows (login, JWT, role-based access, API keys, logout, session expiry) in `frontend/e2e/auth.spec.ts` + OWASP WSTG security tests (injection, broken auth, IDOR, CORS, headers, CSRF, path traversal) in `frontend/e2e/security.spec.ts`

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

### OWASP Audit Remediation (2026-02-23)

- [x] (2026-02-23) P0: Fix BOLA in `GetProjectByRepoName` — add `AND tenant_id = $2` filter (`internal/adapter/postgres/store.go`)
- [x] (2026-02-23) P1: Add HTTP security headers — X-Frame-Options, X-Content-Type-Options, Referrer-Policy, Permissions-Policy (`frontend/nginx.conf`, `internal/adapter/http/middleware.go`, `cmd/codeforge/main.go`)
- [x] (2026-02-23) P1: Fix rate limiter IP extraction — use X-Real-Ip/X-Forwarded-For from trusted proxy (`internal/middleware/ratelimit.go`)
- [x] (2026-02-23) P1: Fix JWT audience/issuer validation + fail-closed revocation (`internal/service/auth.go`)
- [x] (2026-02-23) P1: Fix webhook token timing attack — use `crypto/subtle.ConstantTimeCompare` (`internal/middleware/webhook.go`)
- [x] (2026-02-23) P1: Fix auth error message leakage — use generic messages, log details server-side (`internal/adapter/http/handlers_auth.go`)
- [x] (2026-02-23) P1: Fix policy path traversal — add `filepath.Clean()` normalization (`internal/service/policy.go`)
- [x] (2026-02-23) P2: Fix quality gate command injection — switch from `create_subprocess_shell` to `create_subprocess_exec` with `shlex.split` (`workers/codeforge/qualitygate.py`)
- [x] (2026-02-23) P2: Create SECURITY.md and CONTRIBUTING.md
- [x] (2026-02-23) P2: Fix CLAUDE.md documentation inaccuracies (Jinja2→text/template, KeyBERT→BM25S, lucide-solid→Unicode+SVG, zero-dep→minimal-dep, websocket size)
- [x] (2026-02-23) P3: Fix CI postgres version 16→17 + add security scanning job (govulncheck, pip-audit, npm audit)
- [x] (2026-02-23) A: DB adapter integration tests (`internal/adapter/postgres/store_test.go` — ProjectCRUD with tenant isolation, GetProjectByRepoName tenant isolation, UserCRUD with email uniqueness, TokenRevocation lifecycle)
- [x] (2026-02-23) B: Frontend Vitest unit tests (37 tests: `formatters.test.ts` 21 tests for i18n formatters, `cache.test.ts` 10 tests for offline cache, `StepProgress.test.tsx` 6 tests for component rendering)
- [x] (2026-02-23) C: NATS adapter integration tests (`internal/adapter/nats/nats_test.go` — PublishSubscribe, RequestIDPropagation, DLQ validation failure, DLQ retry exhaustion, KeyValue CRUD, IsConnected)
- [x] (2026-02-23) D: Error message leakage in non-auth handlers — `writeInternalError()` helper, ~40 StatusInternalServerError calls sanitized, StatusBadGateway/StatusGatewayTimeout sanitized (`internal/adapter/http/handlers.go`)
- [x] (2026-02-23) E: Docker socket hardening — `--security-opt=no-new-privileges`, `--cap-drop=ALL` on sandbox containers (`internal/service/sandbox.go`)
- [x] (2026-02-23) F: MCP/A2A/LSP/AG-UI documentation — marked as stub/planned in CLAUDE.md
- [x] (2026-02-23) G: SBOM generation — `anchore/sbom-action` for Go and Frontend in CI (`.github/workflows/ci.yml`)
- [x] (2026-02-23) H: Content-Security-Policy header — SPA-compatible CSP on nginx + Go middleware
- [x] (2026-02-23) I: HSTS header — commented-out in nginx.conf, ready for production HTTPS

### Comprehensive OWASP Top 10:2025 + WSTG v4.2 Remediation (2026-02-23)

#### P0 Blocker Fixes
- [x] (2026-02-23) Auth enabled validation: reject empty JWT secret when `auth.enabled=true` (`internal/config/loader.go`: `Validate()` method)
- [x] (2026-02-23) Hardcoded credentials removed: `codeforge.yaml` → `codeforge.example.yaml` with placeholders, added `codeforge.yaml` to `.gitignore`
- [x] (2026-02-23) Docker compose prod hardened: removed default password fallbacks (`${VAR:?required}`), sslmode=require, postgres port not exposed, NATS auth, read_only containers
- [x] (2026-02-23) Tenant isolation in store_review.go: added `AND tenant_id = $N` to all 11 read/update/delete queries
- [x] (2026-02-23) WebSocket origin validation: replaced `InsecureSkipVerify: true` with configurable `OriginPatterns` from CORS config
- [x] (2026-02-23) WebSocket tenant-scoped broadcast: `BroadcastToTenant()` method, connections track tenant ID
- [x] (2026-02-23) Webhook HMAC wired to routes: moved webhook routes outside auth group, per-route HMAC/Token middleware
- [x] (2026-02-23) Webhook HMAC empty secret bypass: now returns 503 instead of pass-through when secret not configured
- [x] (2026-02-23) DeleteAPIKey IDOR: added `user_id` filter (`DELETE FROM api_keys WHERE id=$1 AND user_id=$2`)
- [x] (2026-02-23) Path traversal in policy CRUD: `sanitizeName()` rejects path separators, `..`, dot-prefix, overlong names
- [x] (2026-02-23) Path traversal in AdoptProject/DetectStackByPath: `filepath.Clean()` + absolute path validation
- [x] (2026-02-23) Rate limiter hardened: removed spoofable X-Forwarded-For trust, added `maxBuckets` cap (100k) against memory exhaustion

#### P1 Critical Fixes
- [x] (2026-02-23) JWT error message sanitization: replaced `err.Error()` interpolation with generic `"invalid token"` message
- [x] (2026-02-23) MustChangePassword on API key auth: enforced on API key path same as Bearer token path
- [x] (2026-02-23) Refresh token rotation race condition: `SELECT ... FOR UPDATE` inside transaction, `RotateRefreshToken` takes hash instead of ID
- [x] (2026-02-23) Role escalation prevention: `Register()` validates role against `ValidRoles`, defaults to `viewer`
- [x] (2026-02-23) Account lockout: 5 failed attempts → 15min lock (`FailedAttempts`, `LockedUntil` fields on User, migration 031)
- [x] (2026-02-23) Sandbox hybrid mode hardened: added `--security-opt=no-new-privileges --cap-drop=ALL` to CreateHybrid
- [x] (2026-02-23) RBAC for tenant management: `/tenants` routes wrapped in `RequireRole(admin)` middleware
- [x] (2026-02-23) Request body size limits: generic `readJSON[T]` helper with 1MB `http.MaxBytesReader` on all JSON decode handlers (~35+ endpoints)

### Backend QA — Comprehensive Test Suite (2026-02-24)

> 266-test automated suite covering 44 modules across the entire Go backend API surface (177+ routes).

#### Phase 1: Discovery & Test Plan
- [x] (2026-02-24) Mapped all 177+ REST/WebSocket endpoints from `routes.go`
- [x] (2026-02-24) Identified 44 testable modules covering auth, CRUD, middleware, business logic, error handling

#### Phase 2: Test Execution
- [x] (2026-02-24) Executed 266 tests against live server with auth, NATS, PostgreSQL, LiteLLM

#### Phase 3: Bug Diagnosis & Fix — Commit `1e2afd6` (Round 1)
- [x] (2026-02-24) Fix context_optimizer.go: missing mutex unlock on early return path
- [x] (2026-02-24) Fix knowledgebase.go: CreateRequest validation missing required fields
- [x] (2026-02-24) Fix store.go: `depends_on` column scan error on plan step queries

#### Phase 3: Bug Diagnosis & Fix — Commit `192a579` (Round 2)
- [x] (2026-02-24) Fix webhook auth bypass: `/api/v1/webhooks/*` blocked by JWT middleware — added `publicPrefixes` in `auth.go`
- [x] (2026-02-24) Fix unique constraint 500: SQLSTATE 23505 unhandled in `writeDomainError` — added 409 Conflict mapping in `handlers.go`
- [x] (2026-02-24) Fix run validation 500: `StartRequest.Validate()` missing `domain.ErrValidation` wrapping + handler used `writeInternalError` — fixed in `validate.go` + `handlers.go`
- [x] (2026-02-24) Fix shared context author UUID: DB column `UUID NOT NULL` but no Go-side format validation — added regex check in `shared.go`
- [x] (2026-02-24) Fix KB index 500: "no content path" error missing `domain.ErrValidation` wrapping — fixed in `knowledgebase.go`

#### Phase 3: Bug Diagnosis & Fix — Round 3 (Domain Error Handling Audit)
- [x] (2026-02-24) Fix git handlers: `ProjectGitStatus`, `PullProject`, `ListProjectBranches`, `CheckoutBranch` returned HTTP 500 for nonexistent projects instead of 404 — changed `writeInternalError` to `writeDomainError` in `handlers.go`
- [x] (2026-02-24) Fix `CancelRun` handler: returned HTTP 500 for nonexistent runs instead of 404 — changed to `writeDomainError`
- [x] (2026-02-24) Fix `DispatchTask` and `StopAgentTask`: returned HTTP 500 for nonexistent agents/tasks — changed to `writeDomainError`
- [x] (2026-02-24) Systematic audit: changed 35 handlers from `writeInternalError` to `writeDomainError` where resource IDs are involved (tasks, agents, runs, plans, teams, costs, sessions, branch-rules, reviews, conversations, roadmap, milestones, trajectory, context packs, sync)
- [x] (2026-02-24) Kept `writeInternalError` only for 8 truly global operations (ListProjects, GlobalCostSummary, ListTenants, GlobalAuditTrail, GetSettings, UpdateSettings, ListVCSAccounts, RequestGraphBuild after project verified)
- [x] (2026-02-24) Verified: all single-resource 404 tests pass, invalid UUID returns 400, existing resources unaffected

#### Final Results
- [x] (2026-02-24) **262/266 PASS (98.5%)** — 42/44 modules at 100%, 4 remaining failures are infrastructure timeouts (Python retrieval workers not running)
- [x] (2026-02-24) **Round 3**: 44/44 regression tests PASS (26 domain-error + 18 sanity tests) — all resource-scoped handlers now return proper HTTP status codes
- [ ] BLOCKED: 4 search/graph endpoint tests require running Python retrieval workers (search project, agent search, graph search, SQLi in search)

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
- [x] (2026-02-23) Reference: Claude Code conditional prompt assembly pattern (~40 modular sections, token-budget-aware) — implemented via `BuildModePrompt()` in `mode_prompt.go`

#### 12B. LLM Routing Implementation (P1)

- [x] (2026-02-23) Wire scenario tags to LiteLLM model selection via native tag-based routing (`litellm/config.yaml`: moved tags from `model_info.tags` to `litellm_params.tags`, added `router_settings.enable_tag_filtering`; Python: `resolve_scenario()` + `tags` param on `completion()` in `llm.py`; `executor.py` reads `mode.llm_scenario` and passes tag + temperature)
- [x] (2026-02-23) Implement tag-to-model mapping in LiteLLM proxy config — 30+ models tagged with default/background/think/longContext/review/plan; Gemini models get `longContext` tag; `ScenarioConfig` maps scenarios to temperatures (think=0.3, review=0.1, default=0.2, background=0.1, plan=0.3)
- [x] (2026-02-23) Add routing decision logging for observability — structured log in `executor.py` (run_id, mode, scenario, temperature) + debug log in `llm.py` (model, temperature, tags, prompt_len); 5 new tests in `test_llm.py`
- [x] (2026-02-23) Reference: Tags already defined in Mode domain (`internal/domain/mode/mode.go`), LiteLLM adapter has no routing logic yet (`internal/adapter/litellm/`) — tag-based routing implemented in Phase 12B

#### 12C. Role Evaluation Framework (P1)

- [x] (2026-02-23) Define role responsibility matrix (9 roles: Orchestrator, Architect, Coder, Reviewer, Security, Tester, Debugger, Proponent, Moderator) with input/output/allowed/denied columns (`workers/tests/role_matrix.py`)
- [x] (2026-02-23) Implement FakeLLM test harness for deterministic agent testing (`workers/tests/fake_llm.py`): fixture-based, call tracking, `from_fixture()` class method
- [x] (2026-02-23) Create `tests/scenarios/` fixture directory structure: 5 roles x 3 files = 15 JSON fixtures (`{role}/{scenario}/input.json`, `expected_output.json`, `llm_responses.json`)
- [x] (2026-02-23) Define 7 MVP evaluation tests (`workers/tests/test_role_evaluation.py`): architect generates plan, coder produces diff, reviewer catches bug, tester reports pass/fail, security flags risk, debate convergence, orchestrator boundary
- [x] (2026-02-23) Integrate evaluation metrics schema (`workers/tests/evaluation.py`): EvaluationMetrics dataclass (passed, tokens, steps, cost, artifact_quality)
- [x] (2026-02-23) Research reference: SPARC-Bench, DeepEval, AgentNeo, REALM-Bench, GEMMAS — evaluation framework implemented with FakeLLM + fixtures in Phase 12C

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
- [x] (2026-02-23) Extend HybridRetriever with scope_id observability pass-through — added `scope_id` to 4 NATS payloads (Go schemas + Python models), variadic `scopeID` param on Go `SearchSync`/`SearchSync`, `logger.bind(scope_id=...)` in Python consumer handlers
- [x] (2026-02-23) Extend CodeGraphBuilder with scope_id observability pass-through — same changes cover graph payloads (`GraphBuildRequest`, `GraphSearchRequest`)
- [x] (2026-02-23) Implement incremental indexing with hash-based delta detection — `FileHashRecord` dataclass, `_file_sha256()` helper, `chunk_workspace_by_file()`, `_build_incremental()` with SHA-256 delta detection, chunk/embedding reuse for unchanged files; 5 new tests (no-change, add, change, delete, model-change)
- [x] (2026-02-23) Add scope management frontend UI — `ScopesPage.tsx` with CRUD, project multi-select, KB attachment, hybrid search; API client `scopes` namespace (9 methods); ~30 i18n keys (en+de); route + sidebar nav

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

#### 12K. Knowledge Bases (P3) — COMPLETED 2026-02-23

- [x] (2026-02-23) Domain types: `KnowledgeBase`, `CreateRequest`, `UpdateRequest`, `Category`, `Status` in `internal/domain/knowledgebase/knowledgebase.go`
- [x] (2026-02-23) Built-in catalog: 8 entries (go-stdlib, react-patterns, python-stdlib, solid-principles, clean-architecture, ddd-patterns, security-owasp, rest-api-design) in `catalog.go`
- [x] (2026-02-23) Database migration `030_knowledge_bases.sql`: `knowledge_bases` table + `scope_knowledge_bases` join table with cascade deletes
- [x] (2026-02-23) Store interface: 9 new methods on `database.Store` for KB CRUD + scope attachment + status updates
- [x] (2026-02-23) Postgres store implementation: `store_knowledgebase.go` — tenant-scoped queries, partial update, ErrNotFound mapping
- [x] (2026-02-23) Service: `KnowledgeBaseService` — CRUD, scope attach/detach, `RequestIndex` (reuses retrieval pipeline with `kb:` prefix), `SeedBuiltins` (idempotent startup seeding)
- [x] (2026-02-23) Scope integration: `SearchScope` now includes indexed KBs attached to scope in fan-out search alongside project indexes
- [x] (2026-02-23) HTTP API: 9 handlers — CRUD + index + scope attach/detach/list endpoints
- [x] (2026-02-23) Frontend: TypeScript types, API client (8 methods), KnowledgeBasesPage with card grid, create form, index/delete actions, sidebar nav link
- [x] (2026-02-23) i18n: 26 keys for KB UI labels, statuses, categories, toasts
- [x] (2026-02-23) Tests: 3 domain tests (validation, categories, catalog) + 6 service tests (CRUD, delete-builtin, scope attach/detach, seed idempotency, validation errors)

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

### Backend E2E Vision Test (2026-02-23)

- [x] (2026-02-23) Create E2E test plan document (`docs/e2e-test-plan.md`) — 6 phases, 60+ test cases across all 4 pillars
- [x] (2026-02-23) Create executable E2E test script (`/tmp/e2e-vision-test.sh`) — automated PASS/FAIL/SKIP reporting
- [x] (2026-02-23) Fix bug: ReviewService.CreatePolicy missing TenantID — added `tenantID` param to service method, handler extracts from context (`internal/service/review.go`, `internal/adapter/http/handlers.go`)
- [x] (2026-02-23) E2E test results: **88 PASS, 0 FAIL, 3 SKIP** (97% pass rate)
  - Phase 0: 6 PASS — infrastructure (Go Core, PostgreSQL, NATS, LiteLLM, Groq, Mistral)
  - Phase 1: 13 PASS — Project Dashboard (create, clone, workspace, stack detect, git ops, providers)
  - Phase 2: 15 PASS — Roadmap (CRUD, AI views JSON/YAML/MD, spec detect, GitHub PM import, sync)
  - Phase 3: 8 PASS — Multi-LLM (models, health, direct Groq/Mistral calls, costs, backends)
  - Phase 4: 24 PASS, 1 SKIP — Agent Orchestration (agent/task CRUD, policy eval, modes, run creation; run execution skipped — Python worker not running)
  - Phase 5: 21 PASS, 2 SKIP — Cross-pillar (scope, review policy, plans, costs, audit; repomap/search skipped — Python worker needed)
  - Phase 6: 1 PASS — WebSocket connection established
- [ ] Start Python worker and re-run E2E tests to validate agent execution pipeline (Phase 4.15, 5.2, 5.5)

---

### Phase 13 — UI/UX Improvements & Orchestrator Chat

> Frontend UX overhaul: validation, dropdowns, CRUD completeness, settings, spec detection,
> orchestrator chat interface, automatic orchestration, dev tooling, AG-UI protocol integration.
> Sources: 16 improvement requests + project-status.md + 04-agent-orchestration.md

#### 13.1 Foundation Fixes (Phase 1)

##### 13.1A Backend Input Validation
- [x] (2026-02-24) Create `internal/domain/project/validation.go` with `ValidateCreateRequest()` and `ValidateUpdateRequest()`
  - Name: non-empty, max 255 chars, no control chars
  - Provider: must be in `gitprovider.Available()` or empty
  - RepoURL: if non-empty, must be valid git URL (`https://...` or `git@...:...`)
  - Description: max 2000 chars
  - Return `domain.ErrValidation` wrapping specific field errors
- [x] (2026-02-24) Create `internal/domain/project/validation_test.go` with table-driven tests
- [x] (2026-02-24) Wire validation into `CreateProject` handler and `ProjectService.Create()`

##### 13.1B Mode Scenario Validation
- [x] (2026-02-24) Add `ValidScenarios` list to `internal/domain/mode/mode.go`, validate `LLMScenario` in `Validate()`
  - Valid scenarios: `default`, `background`, `think`, `longContext`, `review`, `plan`
- [x] (2026-02-24) Add `ListScenarios` handler in `internal/adapter/http/handlers.go`
- [x] (2026-02-24) Add route `r.Get("/modes/scenarios", h.ListScenarios)` BEFORE `r.Get("/modes/{id}", ...)` in routes.go

##### 13.1C Toast Positioning Fix
- [x] (2026-02-24) Fix `frontend/src/components/Toast.tsx`: change `fixed right-4 top-4 z-50` to `fixed right-4 top-16 z-[60]`

##### 13.1D Dropdowns for Known Options
- [x] (2026-02-24) Dashboard: replace provider `<input>` with `<select>` populated from `api.providers.git()` data
- [x] (2026-02-24) Modes: replace scenario `<input>` with `<select>` from `/modes/scenarios`
- [x] (2026-02-24) Add `modes.scenarios()` method to `frontend/src/api/client.ts`
- [x] (2026-02-24) Add dropdown labels to `frontend/src/i18n/en.ts`

#### 13.2 CRUD Completeness (Phase 2)

##### 13.2A Make Projects Editable
- [x] (2026-02-24) Add `UpdateRequest` struct (pointer fields for partial updates) to `internal/domain/project/project.go`
- [x] (2026-02-24) Add `ValidateUpdateRequest()` to `internal/domain/project/validation.go`
- [x] (2026-02-24) Add `Update()` method to `internal/service/project.go` (fetch, merge, validate, save)
- [x] (2026-02-24) Add `UpdateProject` handler to `internal/adapter/http/handlers.go`
- [x] (2026-02-24) Add route `r.Put("/projects/{id}", h.UpdateProject)` in routes.go
- [x] (2026-02-24) Frontend: add edit mode signal, "Edit" button on project cards, reuse form for create/edit in `DashboardPage.tsx`
- [x] (2026-02-24) Add `projects.update()` to `frontend/src/api/client.ts`
- [x] (2026-02-24) Add `UpdateProjectRequest` to `frontend/src/api/types.ts`

##### 13.2B Make Modes Editable
- [x] (2026-02-24) Add `Update()` method to `internal/service/mode.go` (reject builtin modes)
- [x] (2026-02-24) Add `UpdateMode` handler to `internal/adapter/http/handlers.go`
- [x] (2026-02-24) Add route `r.Put("/modes/{id}", h.UpdateMode)` in routes.go
- [x] (2026-02-24) Frontend: "Edit" button on custom mode cards, populate form, switch submit target in `ModesPage.tsx`
- [x] (2026-02-24) Add `modes.update()` to `frontend/src/api/client.ts`

##### 13.2C Repo URL Auto-Detection
- [x] (2026-02-24) Create `internal/domain/project/urlparse.go` with `ParseRepoURL(rawURL) -> (owner, repo, provider, error)`
- [x] (2026-02-24) Create `internal/domain/project/urlparse_test.go` with GitHub/GitLab/Gitea/Bitbucket URL tests
- [x] (2026-02-24) Add `ParseRepoURL` handler + route `r.Post("/parse-repo-url", h.ParseRepoURL)` in handlers/routes
- [x] (2026-02-24) Update `ValidateCreateRequest()`: allow empty name if `RepoURL` is set (auto-generate from URL)
- [x] (2026-02-24) Frontend: debounced `onInput` on repo_url field; auto-fill name+provider; remove required marker on name when URL present

##### 13.2D Local Project Hint/Button
- [x] (2026-02-24) Frontend: add tab toggle `[Remote] [Local]` at top of project form in `DashboardPage.tsx`
  - Local mode: path input + optional name; calls `POST /projects` then `POST /projects/{id}/adopt`
  - Auto-detect name from directory basename

#### 13.3 Settings & Account Management (Phase 3)

##### 13.3A General Settings Page
- [x] (2026-02-24) Create `internal/domain/settings/settings.go` with `Settings` struct (default provider, default autonomy, auto-clone, etc.)
- [x] (2026-02-24) Create migration `032_create_settings.sql` — `settings` table (key TEXT PK, value JSONB)
- [x] (2026-02-24) Add `GetSettings`, `UpdateSettings` handlers + routes
- [x] (2026-02-24) Frontend: add "General" section to `SettingsPage.tsx` with editable form + save button
- [x] (2026-02-24) Add `settings.get()`, `settings.update()` to API client

##### 13.3B VCS Account Management
- [x] (2026-02-24) Create `internal/domain/vcsaccount/account.go` — `VCSAccount` entity (id, tenant, provider, label, server_url, auth_method, encrypted_token)
- [x] (2026-02-24) Create `internal/domain/vcsaccount/crypto.go` — AES-256-GCM encrypt/decrypt (SHA-256 hash of JWT secret -> 32-byte AES key)
- [x] (2026-02-24) Create migration `033_create_vcs_accounts.sql` — `vcs_accounts` table with `encrypted_token BYTEA`
- [x] (2026-02-24) Store + handler CRUD for VCS accounts + `TestVCSAccount` handler (tries listing repos with stored credentials)
- [x] (2026-02-24) Frontend: "VCS Accounts" section in `SettingsPage.tsx` — list, add form (provider dropdown, label, token, optional server URL), test + delete buttons

#### 13.4 Spec/Roadmap Detection Fix (Phase 4)

- [x] (2026-02-24) Expand `fileMarkers` in `internal/service/roadmap.go`: add `TODO.md`, `todo.md`, `docs/TODO.md`, `docs/roadmap.md`, `CHANGELOG.md`
- [x] (2026-02-24) After provider + marker checks: shallow-scan root and `docs/` for `.md` files containing "roadmap", "todo", "spec", "feature", "milestone" (case-insensitive)
- [x] (2026-02-24) Return ALL matches in `DetectionResult.FileMarkers` (not just first)
- [x] (2026-02-24) Frontend: make "Detect Specs" button more prominent in `ProjectDetailPage.tsx`, show all detected markers in a list

#### 13.5 Chat Interface & Orchestrator Conversation (Phase 5)

##### 13.5A Backend: Conversation API
- [x] (2026-02-24) Create `internal/domain/conversation/conversation.go` — `Conversation` + `Message` entities
- [x] (2026-02-24) Create migration `034_create_conversations.sql` — `conversations` + `conversation_messages` tables
- [x] (2026-02-24) Create `internal/service/conversation.go` — `ConversationService` with `Create`, `List`, `Get`, `SendMessage`
- [x] (2026-02-24) Create `internal/service/templates/conversation_system.tmpl` — dynamic system prompt (project context, agents, roadmap, task history)
- [x] (2026-02-24) Add conversation handlers + routes: `POST /projects/{id}/conversations`, `GET /conversations/{id}`, `POST /conversations/{id}/messages`
- [x] (2026-02-24) Add `EventConversationMessage` to `internal/adapter/ws/events.go`
- [x] (2026-02-24) Add conversation CRUD to PostgreSQL store
- [x] (2026-02-24) `SendMessage` flow: store user msg → build context → LiteLLM call → WS broadcast AG-UI `agui.text_message` → store full response (streaming deferred to Phase 8)

##### 13.5B Frontend: Chat Panel
- [x] (2026-02-24) Create `frontend/src/features/project/ChatPanel.tsx`
  - Message list (scrollable, auto-scroll), input textarea (Enter=send, Shift+Enter=newline)
  - Message bubbles: user (right/blue), assistant (left/gray)
  - Loading indicator ("Thinking..." animation)
  - WebSocket subscription for `agui.text_message`, `agui.tool_call` (deferred to Phase 8)
- [x] (2026-02-24) Add "Chat" tab to `ProjectDetailPage.tsx`
- [x] (2026-02-24) Add `Conversation`, `Message`, `SendMessageRequest` to `frontend/src/api/types.ts`
- [x] (2026-02-24) Add `conversations` namespace to `frontend/src/api/client.ts`

#### 13.6 Automatic Orchestration (Phase 6)

- [x] (2026-02-24) Add `SetupProject(ctx, id)` to `internal/service/project.go` — chain: clone → detect stack → detect specs → import specs (each step idempotent)
- [x] (2026-02-24) Add `SetupProject` handler for `POST /api/v1/projects/{id}/setup`
- [x] (2026-02-24) Frontend: after project creation, fire-and-forget auto-setup with toast notification
- [x] (2026-02-24) Advanced settings toggle for fine-grained workflow configuration (mode selection, team composition, autonomy level)

#### 13.7 Dev Tooling & CI (Phase 7)

##### 13.7A Lighthouse CI
- [x] (2026-02-24) Add `lighthouse` job to `.github/workflows/ci.yml` using `treosh/lighthouse-ci-action@v12`
- [x] (2026-02-24) Create `frontend/lighthouserc.yaml` — thresholds: performance warn >0.8, accessibility error >0.9

##### 13.7B Prompt Benchmark (Dev Mode)
- [x] (2026-02-24) Add `BenchmarkPrompt` handler (behind `DEV_MODE` env check) + dev route `POST /dev/benchmark`
- [x] (2026-02-24) Frontend: "Developer Tools" section in `SettingsPage.tsx` with prompt benchmark form (model, prompt, temperature, max tokens, latency display)

##### 13.7C Refactorer Mode Improvement
- [x] (2026-02-24) Add `Bash` to refactorer tools in `internal/domain/mode/presets.go`
- [x] (2026-02-24) Improve refactorer `PromptPrefix` with specific refactoring strategies

#### 13.8 AG-UI Protocol Integration (Phase 8)

- [x] (2026-02-24) Wire AG-UI events into Hub broadcasts via existing `BroadcastEvent()` method
- [x] (2026-02-24) Emit `agui.text_message` during LLM response in `internal/service/conversation.go`
- [x] (2026-02-24) Emit `agui.tool_call` / `agui.tool_result` / `agui.run_started` / `agui.run_finished` in `internal/service/runtime.go`
- [x] (2026-02-24) Add AG-UI event types + `onAGUIEvent()` handler to `frontend/src/api/websocket.ts`
- [x] (2026-02-24) Consume AG-UI events in `ChatPanel.tsx` — streaming content display, thinking indicator, auto-refetch on run_finished
- [x] (2026-02-24) Emit `agui.step_started` in orchestrator `startStep()` and `agui.step_finished` in `broadcastStepStatus()` for terminal statuses
- [x] (2026-02-24) Subscribe to step events in `ChatPanel.tsx` — plan step status badges (running/completed/failed)
- `agui.state_delta` intentionally deferred — no natural emission point in current architecture (CodeForge uses DB-backed state + native WS events, not CopilotKit shared mutable state)

#### 13.9 Outstanding Items (Phase 9)

##### 13.9A E2E Auth + OWASP Security Tests
- [x] (2026-02-24) Create `frontend/e2e/auth.spec.ts` — Playwright tests: login, JWT refresh, role-based access, API key CRUD, forced password change, logout, session expiry
- [x] (2026-02-24) Create `frontend/e2e/security.spec.ts` — OWASP WSTG tests: injection, broken auth, IDOR, CORS, headers, CSRF, path traversal

##### 13.9B Policy "Effective Permission Preview"
- [x] (2026-02-24) Add "Preview" mode to `PolicyPanel.tsx` showing matched rule + reason for a given tool call
  - Full EvaluationResult display (decision, scope, matched_rule, reason) + standalone preview view

##### 13.9C Retrieval Sub-Agent Enhancements
- [x] (2026-02-24) Add configurable `expansion_prompt` field to project config (variadic parameter in `SubAgentSearchSync`, read from `project.Config["expansion_prompt"]`)
- [x] (2026-02-24) Wire sub-agent LLM call costs into existing cost aggregation (event store recording in `HandleSubAgentSearchResult`, Python `CostAccumulator`)

##### 13.9D Additional Agent Backends
- [x] (2026-02-24) Goose adapter implementing `agentbackend.Backend` with `Register(queue)` pattern
- [x] (2026-02-24) OpenCode adapter implementing `agentbackend.Backend`
- [x] (2026-02-24) Plandex adapter implementing `agentbackend.Backend`
- [x] (2026-02-24) OpenHands adapter implementing `agentbackend.Backend`

#### Phase 13 Dependencies

```text
13.1 (Foundation) → 13.2 (CRUD) → 13.5 (Chat) → 13.6 (Auto)
                  ↘ 13.3 (Settings) ↗                 ↘ 13.8 (AG-UI)
                  ↘ 13.4 (Spec Fix) ↗
13.7 (Dev Tooling) — independent
13.9A (E2E Auth) — independent, after 13.1
13.9B-D — independent
```

#### Phase 13 DB Migrations

| # | Phase | Table |
|---|-------|-------|
| 032 | 13.3A | `settings` |
| 033 | 13.3B | `vcs_accounts` |
| 034 | 13.5A | `conversations`, `conversation_messages` |

---

### Frontend E2E QA Results (2026-02-24)

Comprehensive browser-based QA testing via Playwright MCP.

#### Bugs Fixed

- [x] (2026-02-24) WebSocket auth: `buildWSURL()` in `websocket.ts` did not append JWT `?token=` query param required by Go backend (`middleware/auth.go:71`). Fixed by exporting `getAccessToken()` from `client.ts` and appending token to WS URL.
- [x] (2026-02-24) Cascade deletes: `runs` and `plan_steps` FK constraints lacked `ON DELETE CASCADE`, causing 500 errors when deleting projects/tasks/agents. Fixed via migration `035_fix_cascade_deletes.sql`.
- [x] (2026-02-24) Provider dropdown duplicate: i18n placeholder `dashboard.form.providerPlaceholder` was set to `"github"` instead of a descriptive placeholder, causing a visual duplicate in the `<select>`. Fixed in `en.ts` and `de.ts`.
- [x] (2026-02-24) Detect Stack crash: `detectFrameworks()` in `internal/domain/project/scan.go` returned `nil` for languages without framework rules. Go JSON marshals nil slices as `null`, causing `lang.frameworks.length` to crash in `DashboardPage.tsx`. Fixed: Go returns `[]string{}` instead of `nil`; frontend adds `?? []` null coalescing as defense in depth.
- [x] (2026-02-24) Settings popover dismiss: `CompactSettingsPopover.tsx` had click-outside handler via `createEffect` that never fired due to SolidJS reactivity timing. No Escape key handler existed. Fixed: register `mousedown` + `keydown` listeners on mount, check `props.open` in handler; click-outside checks parent container to avoid race with gear button toggle.

#### Known Issues (All Resolved)

- [x] (2026-02-24) Sign-out does not redirect to `/login`: Fixed in Phase 14 Bug Fixes. `AuthProvider.tsx` `logout()` now calls `navigate("/login", { replace: true })` after clearing state.
- [x] (2026-02-24) Non-existent project causes white screen crash: Fixed. `ProjectDetailPage.tsx` now checks `project.error` and shows "Project not found" with back-to-dashboard link instead of crashing.
- [x] (2026-02-24) Unknown routes show blank main area: Fixed in Phase 14 Bug Fixes. Catch-all `<Route path="*404" component={NotFoundPage} />` added in `index.tsx`.
- [x] (2026-02-24) Chat 500 error: `ChatCompletionStream()` fails because no LLM API keys configured. Not a code bug — requires LiteLLM proxy configuration with valid API keys. User messages are saved to DB correctly.

#### Test Results Summary

| # | Module | Status | Notes |
|---|--------|--------|-------|
| 1 | Navigation & Routing | PASS | All 13 routes, sidebar active states, keyboard shortcuts |
| 2 | Project Dashboard CRUD | PASS | Create, edit, delete, URL auto-detect, form validation |
| 3 | Project Detail Page | PASS | Roadmap CRUD, AI View, Import, Sync-to-file, Pull, Settings, Chat |
| 4 | Costs Dashboard | PASS | Summary cards, cost-by-project section |
| 5 | LLM Models Page | PASS | 30+ models displayed, Add Model form |
| 6 | Agent Modes Page | PASS | 8 built-in modes, prompt toggle, Add Mode form |
| 7 | Activity Log Page | PASS | Live badge, filters, Pause/Resume toggle |
| 8 | Knowledge Bases Page | PASS | 8 built-in + custom KBs, Create/Index/Delete |
| 9 | Scopes Page | PASS | Create shared scope, add projects/KBs, delete |
| 10 | Teams Page | PASS | Create team with protocol/members, delete |
| 11 | Settings Page | PASS | General, VCS, Providers, LLM, API Keys, Dev Tools |
| 12 | Theme & Language Switching | PASS | System/Light/Dark cycle, EN/DE full translation |
| 13 | Error Handling & Edge Cases | PASS | 404, ErrorBoundary, Detect Stack, form validation, keyboard shortcuts, Command Palette, a11y |

---

### Phase 14: UX Simplification — Claude Code-Inspired Workflow

Replace the 7-tab project detail page with a side-by-side layout (Roadmap left, Chat right). Simplify project creation. Add structured markdown parsing, drag-to-reorder, bidirectional sync, and production-quality chat.

#### Phase 14A: New Project View Layout (Side-by-Side)

- [x] Refactor `ProjectDetailPage.tsx`: remove Tab type, TABS array, tab navigation bar (2026-02-24)
- [x] Replace tab content with permanent two-column layout (Roadmap left w-1/2, Chat right w-1/2) (2026-02-24)
- [x] Keep header bar with: project name, git status badge, clone/pull buttons, gear icon (2026-02-24)
- [x] Create `CompactSettingsPopover.tsx` (NEW): gear icon popover with mode selection, autonomy level, agent backends, cost summary (2026-02-24)
- [x] Modify `ChatPanel.tsx`: change `h-[600px]` to `flex flex-col h-full`, remove conversation sidebar, auto-create conversation on mount (2026-02-24)
- [x] Modify `RoadmapPanel.tsx`: change to full-height layout (`h-full overflow-y-auto`) (2026-02-24)
- [x] Add i18n keys (en + de) for gear icon tooltip, compact settings section headers (2026-02-24)

#### Phase 14B: Simplified Project Creation with Branch Selection

- [x] Add `ListRemoteBranches` handler: `git ls-remote --heads <url>`, parse refs, return branch list (2026-02-24)
- [x] Add route `GET /projects/remote-branches` (before `/{id}` wildcard) (2026-02-24)
- [x] Modify `gitlocal.Provider.Clone()`: accept optional branch param, use `--branch <branch> --single-branch` if set (2026-02-24)
- [x] Add `Branch string` field to `project.CreateRequest` (2026-02-24)
- [x] Pass branch to `Clone()` in `ProjectService` (2026-02-24)
- [x] Frontend: add branch `<select>` in DashboardPage, fetch branches on URL input (debounced) (2026-02-24)
- [x] Frontend: add `projects.remoteBranches()` to API client (2026-02-24)
- [x] Add i18n keys (en + de) for branch label, placeholder, loading state (2026-02-24)

#### Phase 14C: Roadmap Structured Parsing + Drag-to-Reorder

- [x] Create `internal/adapter/markdownspec/parser.go`: `ParseMarkdown(content) -> []SpecItem` (headings, checkboxes, plain lists) (2026-02-24)
- [x] Create `internal/adapter/markdownspec/parser_test.go`: test nested checkboxes, mixed headings, edge cases (2026-02-24)
- [x] Modify `markdownspec/provider.go`: call `ParseMarkdown` in Detect/Import, map SpecItem to Milestone + Feature (2026-02-24)
- [x] Create `frontend/src/features/project/DragList.tsx`: generic drag-to-reorder using native HTML Drag and Drop API (2026-02-24)
- [x] Modify `RoadmapPanel.tsx`: wrap milestone/feature lists with DragList, add inline status toggle (2026-02-24)
- [x] Add API client methods: `roadmap.updateMilestoneOrder()`, `roadmap.updateFeatureStatus()` (2026-02-24)
- [x] Add backend handlers for milestone reorder + feature status update (if not existing) (2026-02-24)

#### Phase 14D: Bidirectional Sync (UI Changes -> Repo Files)

- [x] Create `internal/adapter/markdownspec/writer.go`: `RenderMarkdown(items) -> []byte` (inverse of parser) (2026-02-24)
- [x] Create `internal/adapter/markdownspec/writer_test.go`: round-trip tests (parse -> modify -> render) (2026-02-24)
- [x] Modify `markdownspec/provider.go`: set `Write: true` in Capabilities, implement Write method (2026-02-24)
- [x] Modify `roadmap.go`: add `SyncToSpecFile()` — load DB items, render markdown, write to detected file (2026-02-24)
- [x] Add `SyncToSpecFile` handler + route at `POST /projects/{id}/roadmap/sync-to-file` (2026-02-24)
- [x] Frontend: add "Sync to file" button in RoadmapPanel (2026-02-24)

#### Phase 14E: Chat Enhancements (Streaming + Markdown + Tool Calls)

- [x] Convert `conversation.go` SendMessage from non-streaming to `ChatCompletionStream` (LiteLLM streaming) (2026-02-24)
- [x] Broadcast `agui.text_message` chunks via WebSocket during streaming (2026-02-24)
- [x] Enrich system prompt with: detected roadmap items, project metadata, recent conversation context (2026-02-24)
- [x] Create `frontend/src/features/project/Markdown.tsx`: lightweight markdown renderer (headers, bold/italic, code blocks, links, lists) (2026-02-24)
- [x] Create `frontend/src/features/project/ToolCallCard.tsx`: collapsible tool call card with name, args, result (2026-02-24)
- [x] Modify `ChatPanel.tsx`: use Markdown component for assistant messages, integrate ToolCallCard inline (2026-02-24)

#### Phase 14 Frontend Bug Fixes (from E2E QA)

- [x] Fix sign-out redirect: ensure logout navigates to `/login` and clears cached data (2026-02-24)
- [x] Fix non-existent project crash: add error handling in ProjectDetailPage, show "Project not found" inside I18nProvider (2026-02-24)
- [x] Fix 404 page: add catch-all route with "Page not found" message and link back to dashboard (2026-02-24)

#### LLM Streaming E2E Bug Fixes

- [x] (2026-02-24) Fix wrong model name: `ConversationService` sent `model="default"` to LiteLLM causing 400 Bad Request. Added `ConversationModel` config field (`CODEFORGE_CONVERSATION_MODEL` env var), default fallback changed to `groq/llama-3.1-8b`. (`internal/config/config.go`, `internal/config/loader.go`, `cmd/codeforge/main.go`, `internal/service/conversation.go`)
- [x] (2026-02-24) Fix streaming accumulation bug: `ChatPanel.tsx` `setStreamingContent(content)` replaced signal with each chunk delta instead of appending. Changed to `setStreamingContent((prev) => prev + content)`. (`frontend/src/features/project/ChatPanel.tsx`)

#### Phase 14 Dependencies

```text
14A (Layout)  -----> 14C (Drag Reorder)  -----> 14D (Bidirectional Sync)
     |
     +-------------> 14E (Chat Enhancements)
     |
14B (Creation) --- independent, can parallel with 14A
Bug Fixes --- independent, anytime
```

---

### Phase 15: Protocol Integrations (MCP, LSP)

#### Phase 15A: MCP Client in Python Workers

- [x] (2026-02-24) Domain types: `internal/domain/mcp/mcp.go` -- TransportType (stdio/sse), ServerStatus (registered/connected/disconnected/error), ServerDef struct, ServerTool struct, Validate()
- [x] (2026-02-24) Domain tests: `internal/domain/mcp/mcp_test.go` -- 8 table-driven validation cases
- [x] (2026-02-24) Config: `internal/config/config.go` -- MCP struct (Enabled, ServersDir, ServerPort), ENV overrides in loader.go
- [x] (2026-02-24) NATS subjects: `internal/port/messagequeue/queue.go` -- SubjectMCPServerStatus, SubjectMCPToolDiscovery
- [x] (2026-02-24) NATS payloads: `internal/port/messagequeue/schemas.go` -- MCPServerDefPayload, MCPServerStatusPayload, MCPToolDiscoveryPayload, MCPServers field on RunStartPayload
- [x] (2026-02-24) MCPService: `internal/service/mcp.go` -- in-memory registry with sync.RWMutex, Register/Remove/Get/List/ResolveForRun, YAML directory loader
- [x] (2026-02-24) MCPService DB: `internal/service/mcp_db.go` -- DB-backed CRUD (CreateDB, GetDB, ListDB, UpdateDB, DeleteDB, AssignToProject, UnassignFromProject, ListByProject, ListTools, UpsertTools)
- [x] (2026-02-24) MCPService tests: `internal/service/mcp_test.go` -- 10 test functions
- [x] (2026-02-24) Runtime integration: `internal/service/runtime.go` -- mcpSvc field, SetMCPService setter, MCP server resolution in run start payload
- [x] (2026-02-24) Python models: `workers/codeforge/mcp_models.py` -- Pydantic MCPServerDef, MCPTool, MCPToolCallResult
- [x] (2026-02-24) Python workbench: `workers/codeforge/mcp_workbench.py` -- McpServerConnection, McpWorkbench (multi-server container), McpToolRecommender (BM25)
- [x] (2026-02-24) Executor integration: `workers/codeforge/executor.py` -- mcp_servers param, McpWorkbench connect/discover/disconnect
- [x] (2026-02-24) Worker models: `workers/codeforge/models.py` -- mcp_servers field on RunStartMessage

#### Phase 15B: MCP Server in Go Core (IDE Integration)

- [x] (2026-02-24) Dependency: `go.mod` -- added github.com/mark3labs/mcp-go
- [x] (2026-02-24) Server rewrite: `internal/adapter/mcp/server.go` -- real mcp-go implementation, ServerConfig, ServerDeps (narrow interfaces: ProjectLister, RunReader, CostReader), Streamable HTTP transport
- [x] (2026-02-24) Tools: `internal/adapter/mcp/tools.go` -- 4 registered tools (list_projects, get_project, get_run_status, get_cost_summary)
- [x] (2026-02-24) Resources: `internal/adapter/mcp/resources.go` -- 2 resources (codeforge://projects, codeforge://costs/summary)
- [x] (2026-02-24) Auth middleware: `internal/adapter/mcp/auth.go` -- Bearer token/API key validation
- [x] (2026-02-24) Server tests: `internal/adapter/mcp/server_test.go` -- 8 tests (lifecycle, tool registration, handlers, nil deps, missing args)
- [x] (2026-02-24) Wiring: `cmd/codeforge/main.go` -- MCPService creation, store setup, SetMCPService on runtime, MCP Server start/stop when enabled

#### Phase 15C: MCP Server Registry + Frontend UI

- [x] (2026-02-24) Migration: `internal/adapter/postgres/migrations/036_create_mcp_servers.sql` -- mcp_servers, project_mcp_servers, mcp_server_tools tables
- [x] (2026-02-24) Store interface: `internal/port/database/store.go` -- 11 MCP methods added
- [x] (2026-02-24) Postgres store: `internal/adapter/postgres/store_mcp.go` -- full implementation with pgx
- [x] (2026-02-24) HTTP handlers: `internal/adapter/http/handlers_mcp.go` -- 10 handlers (CRUD, test, tools, project assignment)
- [x] (2026-02-24) Routes: `internal/adapter/http/routes.go` -- 10 MCP routes
- [x] (2026-02-24) Frontend types: `frontend/src/api/types.ts` -- MCPServer, MCPServerTool, CreateMCPServerRequest
- [x] (2026-02-24) Frontend API: `frontend/src/api/client.ts` -- mcp namespace with 10 methods
- [x] (2026-02-24) Frontend page: `frontend/src/features/mcp/MCPServersPage.tsx` -- server list, add/edit modal, test connection, delete, tools discovery
- [x] (2026-02-24) Frontend routing: `frontend/src/index.tsx` + `App.tsx` -- /mcp route + nav link
- [x] (2026-02-24) i18n: `frontend/src/i18n/en.ts` + `de.ts` -- ~55 MCP keys each

#### Phase 15D: Tool Routing Integration

- [x] (2026-02-24) Policy tests: `internal/domain/policy/policy_test.go` -- test cases for `mcp:` prefixed tool matching

#### Phase 15D: LSP (Language Server Protocol) — Code Intelligence for Agents

- [x] (2026-02-24) Domain types: `internal/domain/lsp/types.go` — Position, Range, Location, Diagnostic, DocumentSymbol, HoverResult, ServerStatus, ServerInfo
- [x] (2026-02-24) Language config: `internal/domain/lsp/language.go` — DefaultServers map (go, python, typescript, javascript)
- [x] (2026-02-24) Config: `internal/config/config.go` — LSP struct (Enabled, StartTimeout, ShutdownTimeout, DiagnosticDelay, MaxDiagnostics, AutoStart)
- [x] (2026-02-24) JSON-RPC transport: `internal/adapter/lsp/jsonrpc.go` — Content-Length framing over stdio
- [x] (2026-02-24) LSP client: `internal/adapter/lsp/client.go` — Full client replacing 30-line stub (Start, Stop, Definition, References, DocumentSymbols, Hover, Diagnostics, OpenFile, readLoop)
- [x] (2026-02-24) LSP service: `internal/service/lsp.go` — Per-project language server management, diagnostic caching, debounced WS broadcast, context entry generation
- [x] (2026-02-24) WebSocket events: `internal/adapter/ws/events.go` — EventLSPStatus, EventLSPDiagnostic + typed structs
- [x] (2026-02-24) HTTP handlers: `internal/adapter/http/handlers.go` — 8 handlers (StartLSP, StopLSP, LSPStatus, LSPDiagnostics, LSPDefinition, LSPReferences, LSPDocumentSymbols, LSPHover)
- [x] (2026-02-24) Routes: `internal/adapter/http/routes.go` — 8 LSP endpoints under `/projects/{id}/lsp/`
- [x] (2026-02-24) Context enrichment: `internal/service/context_optimizer.go` — SetLSP + diagnostic injection at priority 95
- [x] (2026-02-24) Wiring: `cmd/codeforge/main.go` — LSPService init (config-gated), injected into contextOptSvc and Handlers
- [x] (2026-02-24) Frontend types: `frontend/src/api/types.ts` — LSPServerInfo, LSPDiagnostic, LSPLocation, LSPDocumentSymbol, LSPHoverResult
- [x] (2026-02-24) Frontend API: `frontend/src/api/client.ts` — 8 LSP API methods
- [x] (2026-02-24) Frontend panel: `frontend/src/features/project/LSPPanel.tsx` — Language server status, start/stop, diagnostic badges
- [x] (2026-02-24) Context pack: `internal/domain/context/pack.go` — Added EntryDiagnostic kind

---

### Phase 16: Frontend Design System Rework

> Custom design system with theme engine, 23 reusable components, full page migration.

#### 16A. Foundation — Theme Engine + Design Tokens

- [x] (2026-02-24) Expand CSS design tokens: ~25 new tokens (semantic colors, shadows, radii, interactive states) in `:root` and `.dark`
- [x] (2026-02-24) Register tokens as Tailwind v4 `@theme` values (enables `bg-cf-accent`, `text-cf-danger`, etc.)
- [x] (2026-02-24) Custom theme engine: `ThemeDefinition` type, `applyCustomTheme()`, `registerTheme()`, localStorage persistence
- [x] (2026-02-24) Built-in themes: Nord and Solarized Dark with full token overrides
- [x] (2026-02-24) Directory structure: `src/ui/{tokens,primitives,composites,layout}/` with barrel exports

#### 16B. Primitives — 11 Atomic Components (`src/ui/primitives/`)

- [x] (2026-02-24) Button (primary/secondary/danger/ghost, sm/md/lg, loading, fullWidth)
- [x] (2026-02-24) Input, Select, Textarea (error state, mono font, token-based styling)
- [x] (2026-02-24) Checkbox (checked/onChange/disabled)
- [x] (2026-02-24) Label (required indicator with sr-only text)
- [x] (2026-02-24) Badge (6 variants: default/primary/success/warning/danger/info, pill mode)
- [x] (2026-02-24) Alert (4 variants with icon, dismissible)
- [x] (2026-02-24) Spinner (CSS animation, respects reduced-motion, 3 sizes)
- [x] (2026-02-24) StatusDot (color prop, pulse animation, accessibility label)
- [x] (2026-02-24) FormField (label + input + help + error wrapper with aria-describedby)

#### 16C. Composites — 8 Compound Components (`src/ui/composites/`)

- [x] (2026-02-24) Card (compound: Header/Body/Footer sub-components)
- [x] (2026-02-24) Modal (Portal, focus trap, Escape close, aria-modal, body scroll lock)
- [x] (2026-02-24) Table (generic typed columns, loading/empty states)
- [x] (2026-02-24) Tabs (underline/pills variants, keyboard accessible)
- [x] (2026-02-24) EmptyState, LoadingState, ConfirmDialog, SectionHeader

#### 16D. Layout — 4 App Shell Components (`src/ui/layout/`)

- [x] (2026-02-24) Sidebar (compound: Header/Nav/Footer)
- [x] (2026-02-24) NavLink (active state styling with @solidjs/router)
- [x] (2026-02-24) PageLayout (title + description + action + content)
- [x] (2026-02-24) Section (SectionHeader + Card wrapper)

#### 16E. Page Migration — All 40+ Files

- [x] (2026-02-24) Batch 1: App.tsx (Sidebar, NavLink, StatusDot), NotFoundPage, LoginPage
- [x] (2026-02-24) Batch 2: DashboardPage, ProjectCard, ModesPage, MCPServersPage, CostDashboardPage
- [x] (2026-02-24) Batch 3: SettingsPage, ScopesPage, TeamsPage, KnowledgeBasesPage, ActivityPage, ModelsPage
- [x] (2026-02-24) Batch 4: ProjectDetailPage + 20 sub-panels (PolicyPanel, PlanPanel, RoadmapPanel, etc.)
- [x] (2026-02-24) Batch 5: Toast, CommandPalette, OfflineBanner, StepProgress, DiffPreview

#### 16F. Polish — WCAG 2.2 AA + Cleanup

- [x] (2026-02-24) Fix `--cf-text-muted` contrast: gray-400 -> gray-500 (4.6:1 on white, WCAG AA compliant)
- [x] (2026-02-24) Eliminate all hardcoded color classes: 0 remaining `bg-gray-*`/`text-gray-*` in .tsx files
- [x] (2026-02-24) ESLint: 0 errors (11 pre-existing warnings only)
- [x] (2026-02-24) Production build: CSS reduced from 62KB to 40KB (-35%)
- [x] (2026-02-24) All 37 unit tests pass

---

### Notes

- Phases 0-16 complete. All phases implemented. P0-P2 security hardening complete. Backend E2E vision test passed (88/91). Frontend E2E QA: 13/13 test areas pass (5 bugs found and fixed: detect stack nil-slice, settings popover dismiss, sync-to-file stale binary, cascade deletes, WS auth). Phase 15: MCP + LSP protocol integrations complete (mcp-go server, Python workbench, DB registry, frontend, policy integration).
### Phase 17: Interactive Agent Loop (Claude Code-like Core)

> Core agentic loop: user sends message → LLM selects tools → executes → iterates until done.

#### 17A: LiteLLM Client Tool-Calling Support

- [x] (2026-02-25) Python LiteLLM client: `ToolCallPart`, `ChatCompletionResponse`, `chat_completion()`, `chat_completion_stream()` with SSE tool_call delta accumulation
- [x] (2026-02-25) Go LiteLLM client: `ToolFunction`, `ToolDefinition`, `ToolCall` structs, streaming tool_call assembly
- [x] (2026-02-25) Tests: `test_llm_tools.py` (14 tests), `client_test.go` (3 new tests)

#### 17B: Built-in Tool Registry (Python)

- [x] (2026-02-25) Tool framework: `ToolDefinition`, `ToolResult`, `ToolExecutor` Protocol, `ToolRegistry` with register/execute/merge_mcp_tools
- [x] (2026-02-25) 7 built-in tools: ReadFile, WriteFile, EditFile, Bash, SearchFiles, GlobFiles, ListDirectory
- [x] (2026-02-25) `build_default_registry()` factory, MCP tool merge via `_McpToolProxy`
- [x] (2026-02-25) Tests: `test_tools.py` (31 tests)

#### 17C: NATS Protocol Extension

- [x] (2026-02-25) New NATS subjects: `conversation.run.start`, `conversation.run.complete`
- [x] (2026-02-25) New payloads: `ConversationMessagePayload`, `ConversationRunStartPayload`, `ConversationRunCompletePayload`
- [x] (2026-02-25) Python models: `ConversationRunStartMessage`, `ConversationRunCompleteMessage`, `AgentLoopResult`

#### 17D: Conversation Data Model Extension

- [x] (2026-02-25) Domain: Extended `Message` with `ToolCalls`, `ToolCallID`, `ToolName`
- [x] (2026-02-25) Migration 037: `tool_calls JSONB`, `tool_call_id TEXT`, `tool_name TEXT`, nullable content, 'tool' role
- [x] (2026-02-25) Store: Updated `CreateMessage`, `ListMessages`, added `CreateToolMessages` batch insert

#### 17E: Agentic Loop (Python Worker)

- [x] (2026-02-25) `AgentLoopExecutor`: multi-turn tool-use loop with policy enforcement, cost tracking, cancellation
- [x] (2026-02-25) `ConversationHistoryManager`: head-and-tail strategy, token budget, tool result truncation
- [x] (2026-02-25) Consumer integration: `conversation.run.start` subscription, `_handle_conversation_run` handler
- [x] (2026-02-25) Tests: `test_agent_loop.py` (9 tests), `test_history.py` (9 tests)

#### 17F: Go Conversation Service Agentic Mode

- [x] (2026-02-25) `Agent` config struct with defaults and env overrides
- [x] (2026-02-25) `ConversationService` extended: `SetQueue`, `SetAgentConfig`, `SetMCPService`, `SetPolicyService`
- [x] (2026-02-25) `SendMessageAgentic()`: stores user message, builds context, publishes to NATS, returns immediately
- [x] (2026-02-25) `HandleConversationRunComplete()`: stores assistant + tool messages, broadcasts run_finished
- [x] (2026-02-25) `StartCompletionSubscriber()`: NATS subscription for `conversation.run.complete`
- [x] (2026-02-25) System prompt template: added "Available Tools" section with usage guidelines
- [x] (2026-02-25) HTTP handler: agentic mode detection + routing, returns 202 Accepted for async dispatch
- [x] (2026-02-25) `cmd/codeforge/main.go`: wiring for queue, agent config, MCP, policy, completion subscriber

#### 17G: Frontend Enhancement

- [x] (2026-02-25) `ToolCallCard.tsx`: tool-type icons, permission denied badge, collapsible sections
- [x] (2026-02-25) `ChatPanel.tsx`: step counter, running cost, agentic mode indicator, grouped tool calls
- [x] (2026-02-25) `types.ts`: extended `ConversationMessage` with `tool_calls`, `tool_call_id`, `tool_name`

#### 17H: HITL (Human-in-the-Loop) Approval

- [x] (2026-02-25) `AGUIPermissionRequest` event type and struct for WebSocket broadcast
- [x] (2026-02-25) `RuntimeService.waitForApproval()`: blocks on channel, broadcasts permission request, configurable timeout
- [x] (2026-02-25) `RuntimeService.ResolveApproval()`: resolves pending approval from HTTP handler
- [x] (2026-02-25) `POST /api/v1/runs/{id}/approve/{callId}` HTTP endpoint
- [x] (2026-02-25) `ApprovalTimeoutSeconds` config with 60s default

#### 17J: Documentation & Configuration

- [x] (2026-02-25) `docs/todo.md`: Phase 17 section with all completed items
- [x] (2026-02-25) `docs/project-status.md`: Phase 17 status entry
- [x] (2026-02-25) `docs/architecture.md`: Agentic loop data flow section with diagram
- [x] (2026-02-25) `docs/features/04-agent-orchestration.md`: Agentic conversation mode documentation
- [x] (2026-02-25) `CLAUDE.md`: Agentic loop architecture summary

---

### Phase 18: Live E2E Functional Testing & Blockers

#### 18A: NATS Stream Subjects Bug Fix (CRITICAL)

- [x] (2026-02-25) Fix `internal/adapter/nats/nats.go:50`: Add `"conversation.>"` to JetStream stream subjects (was silently rejecting all agentic conversation messages)

#### 18B: System Prompt Self-Correction Enhancement

- [x] (2026-02-25) Update `internal/service/templates/conversation_system.tmpl`: Replace single "report errors" line with 5 retry/self-correction instructions (retry on failure, re-read on edit mismatch, diagnose bash errors, iterate until done, explain what went wrong)

#### 18C: Model Auto-Discovery

- [x] (2026-02-25) `internal/adapter/litellm/client.go`: `DiscoverModels()` — queries LiteLLM `/model/info` + `/v1/models`, returns `DiscoveredModel` with status, tags, cost, provider
- [x] (2026-02-25) `internal/adapter/litellm/client.go`: `DiscoverOllamaModels()` — queries Ollama `/api/tags` when `OLLAMA_BASE_URL` is set
- [x] (2026-02-25) `internal/adapter/http/handlers.go`: `DiscoverLLMModels` handler for `GET /api/v1/llm/discover`
- [x] (2026-02-25) `internal/adapter/http/routes.go`: Register `/llm/discover` route
- [x] (2026-02-25) `frontend/src/api/types.ts`: `DiscoveredModel` and `DiscoverModelsResponse` types
- [x] (2026-02-25) `frontend/src/api/client.ts`: `api.llm.discover()` method
- [x] (2026-02-25) `frontend/src/features/llm/ModelsPage.tsx`: "Discover Models" button, discovered models section with status badges and cost info
- [x] (2026-02-25) `frontend/src/i18n/en.ts` + `frontend/src/i18n/locales/de.ts`: i18n keys for discover UI

#### 18D: Runtime Fix — Conversation Tool Call Policy

- [x] (2026-02-25) `internal/service/runtime.go`: Fix `HandleToolCallRequest` to support conversation-based runs (run_id = conversation_id, no run record in DB). Added `handleConversationToolCall()` fallback: resolves project policy from conversation, evaluates policy, supports HITL approval.
- [x] (2026-02-25) `internal/service/runtime.go`: Fix `HandleToolCallResult` to gracefully skip cost/token tracking for conversation runs (no run record to update).
- [x] (2026-02-25) `codeforge.yaml`: Add `agent:` section with `default_model: groq/llama-3.1-8b`, `max_context_tokens`, `max_loop_iterations`, `agentic_by_default`

#### 18E: Live Testing (Manual)

- [x] (2026-02-25) Boot full stack: Go Core (8080), Python Worker (NATS), LiteLLM (4000), Frontend (3001) — all healthy
- [x] (2026-02-25) Scenario 1: Simple file read — agent reads `codeforge.yaml` via `read_file` tool, returns comprehensive 14-section summary. 1 step, no errors. Model: groq/llama-3.1-8b.
- [x] (2026-02-25) Scenario 2: Multi-step code change — agent attempts adding `/api/v1/ping` endpoint. 6 steps, 49 messages, multiple `write_file` and `bash` tool calls. Self-correction demonstrated (retried on compilation failures). Model: groq/llama-3.1-8b.
- [x] (2026-02-25) Scenario 3: Bug analysis — agent reads `nats.go`, searches for patterns, reports configuration status. 3 steps, no errors. Model: groq/llama-3.1-8b.
- [ ] Scenario 4: Complex multi-file feature (skipped — requires more capable model for multi-file edits)
- [x] (2026-02-25) Frontend validation: Chat UI shows messages, tool calls, tool results, model badges. WebSocket connected. Discover Models shows 37 models with status badges and pricing.
- [x] (2026-02-25) Message persistence: Messages persist across page refresh (stored in PostgreSQL).
- [ ] Cost tracking: Groq reports $0.00 cost (free tier) — needs paid model for cost validation.
- [ ] HITL approval: Not triggered with default "headless-safe-sandbox" policy — needs "ask" policy rule.

### Phase 19: Frontend UX Improvements

> Project detail page layout enhancements: resizable panels, collapsible roadmap.

#### 19A: Resizable Roadmap/Chat Split

- [ ] Make the Roadmap panel and Chat panel widths adjustable via a draggable splitter/divider
- [ ] Persist the user's chosen split ratio (e.g. in localStorage)
- [ ] Ensure responsive behavior — collapse to stacked layout on narrow viewports

#### 19B: Collapsible Roadmap Panel

- [ ] Add a toggle button to show/hide the Roadmap panel entirely
- [ ] When hidden, the Chat panel should expand to fill the available space
- [ ] Persist collapse state (e.g. in localStorage)

#### 19C: Chat Auto-Scroll

- [ ] Automatically scroll to the bottom when new messages arrive
- [ ] Disable auto-scroll when the user scrolls up (reading history)
- [ ] Re-enable auto-scroll when the user scrolls back to the bottom

#### 19D: Remove Mode Selection from Project Settings

- [ ] Remove "Default Mode" dropdown from CompactSettingsPopover and DashboardPage advanced settings
- [ ] The orchestrator decides which mode/agent to use per task — not the user per project
- [ ] Remove `default_mode` from project config keys

#### 19E: Remove Agent Backends from Project Settings

- [ ] Remove "Agent Backends" checkbox list from CompactSettingsPopover and DashboardPage advanced settings
- [ ] Remove `agent_backends` from project config keys
- [ ] Remove hardcoded backend list (aider, goose, opencode, openhands, plandex) from frontend
- [ ] LLM connection runs exclusively via LiteLLM — local agent backends (Aider, Goose, etc.) are out of scope for now

#### 19F: Overhaul Built-in Mode Prompt Prefixes

> Current prompt prefixes are 2-3 generic lines ("You are a software developer. Write clean code.").
> Real coding agents (Claude Code, Aider, etc.) use detailed system prompts with concrete behavioral
> rules, methodology, constraints, and output expectations. The built-in modes need the same depth.
> Reference: https://github.com/Piebald-AI/claude-code-system-prompts

- [ ] **Coder mode:** Expand prompt prefix with: read-before-modify rule, avoid over-engineering, no unnecessary additions/abstractions/error-handling, security awareness (OWASP top 10), minimal changes, follow project conventions, explicit output as diff
- [ ] **Architect mode:** Expand with: thorough exploration methodology (find patterns, trace code paths, understand current architecture), step-by-step plan output format, critical files list, trade-off analysis, dependency sequencing
- [ ] **Reviewer mode:** Expand with: focus on correctness/security/performance, severity scoring, confidence filtering (only flag >80% confident issues), concrete exploit scenarios for security issues, actionable recommendations, structured output format
- [ ] **Debugger mode:** Expand with: systematic diagnosis methodology (reproduce, isolate, trace), read error messages carefully, minimal targeted fixes, verify fix doesn't introduce regressions, explain root cause
- [ ] **Tester mode:** Expand with: coverage-driven approach, test edge cases and error paths, clear test names describing expected behavior, arrange-act-assert pattern, mock external dependencies, failure messages that explain what went wrong
- [ ] **Documenter mode:** Expand with: audience-aware writing (developer docs vs. user docs), keep docs close to code, update not rewrite, explain "why" not just "what", examples over abstractions
- [ ] **Refactorer mode:** Expand with: verify tests pass before and after, one refactoring at a time, preserve external behavior, specific strategies (extract method, rename for clarity, reduce nesting, DRY, type safety)
- [ ] **Security Auditor mode:** Expand with: structured methodology (context research → comparative analysis → vulnerability assessment), severity/confidence scoring, false positive filtering rules, category-specific checks (injection, auth, crypto, data exposure), output format with exploit scenario + fix recommendation
- [ ] **All modes:** Add common rules — read files before modifying, no unnecessary file creation, avoid over-engineering, respect project conventions from CLAUDE.md
- [ ] **Template files:** Update `.tmpl` files in `internal/service/templates/` if prompt structure changes
- [ ] **Prompt length:** Verify assembled prompts stay within reasonable token budget (warn if >2048 tokens)

#### 19G: MCP Streamable HTTP Transport

- [ ] **Go Domain:** Add `TransportStreamableHTTP` (`streamable_http`) to `TransportType` in `internal/domain/mcp/mcp.go`
- [ ] **Go Domain:** Update `validTransports` map and `Validate()` — `streamable_http` requires `url` field (like SSE)
- [ ] **DB Migration:** New migration `0XX_mcp_add_streamable_http.sql` — alter CHECK constraint to `('stdio', 'sse', 'streamable_http')`
- [ ] **Frontend:** Add `streamable_http` option to transport dropdown in `MCPServersPage.tsx`
- [ ] **Frontend:** Show URL field for `streamable_http` (same as SSE), add Headers support for both SSE and streamable_http
- [ ] **Frontend:** Update TypeScript types (`MCPServer.transport` union type)
- [ ] **Python Worker:** Add `streamable_http` case in `McpServerConnection.connect()` using `mcp.client.streamable_http.streamablehttp_client()`

#### 19H: MCP Server Pre-Save Validation (Real Connection Test)

> The test endpoint must perform a real MCP handshake to verify the server is a valid
> MCP resource an LLM agent can interact with. The test runs automatically before saving.
> If it fails, the frontend shows a confirmation dialog asking whether to save anyway.

**Backend — real test endpoint (`POST /api/v1/mcp/servers/test`):**

- [ ] New endpoint that accepts a `ServerDef` body (no ID needed — tests before creation)
- [ ] For `sse` / `streamable_http`: Use `mcp-go` client SDK to connect to URL, perform MCP `initialize` handshake, call `tools/list`
- [ ] For `stdio`: Spawn process via `mcp-go` stdio client, perform `initialize` + `tools/list`, kill process
- [ ] Return result: `{ success: bool, server_name: string, server_version: string, tools: [{name, description}], error: string }`
- [ ] Timeout: 10s max — if server doesn't respond, return `success: false` with timeout error
- [ ] Update existing `POST /api/v1/mcp/servers/{id}/test` to also do a real connection test (re-reads config from DB, runs same logic)

**Frontend — pre-save flow:**

- [ ] On form submit: first call `POST /api/v1/mcp/servers/test` with the form data
- [ ] Show a loading/spinner state ("Testing connection...")
- [ ] If test succeeds: save directly (create or update), show discovered tools count in success toast
- [ ] If test fails: show `ConfirmDialog` — "Connection test failed: {error}. Save anyway?"
- [ ] On confirm: save with `status: "error"`, on cancel: stay in form
- [ ] Keep separate "Test" button in server actions table for re-testing existing servers

### Phase 20: Benchmark Mode (Dev-Mode Agent Evaluation)

> Evaluation framework for measuring agent quality, tool usage, and multi-agent collaboration.
> Only available in dev mode. Uses DeepEval as primary evaluation framework, AgentNeo for
> observability/tracing, and GEMMAS-inspired metrics for multi-agent collaboration quality.
>
> Research references:
> - DeepEval: github.com/confident-ai/deepeval (Apache 2.0, 13.8k stars, 60+ metrics)
> - AgentNeo: github.com/raga-ai-hub/RagaAI-Catalyst (Apache 2.0, ~6k stars)
> - GEMMAS: arxiv.org/abs/2507.13190 (EMNLP 2025, IDS + UPR metrics, no code released)
> - REALM-Bench: arxiv.org/abs/2502.18836 (Stanford, multi-agent coordination benchmark)
> - SPARC-Bench: github.com/agenticsorg/sparc-bench (MIT, SWE-bench based, Roo Code ecosystem)

#### 20A: DeepEval Integration — Agent Evaluation Metrics

**Dependencies:** `pip install deepeval` in Python worker Poetry environment

**Python Worker — evaluation harness (`workers/codeforge/evaluation/`):**

- [ ] Add `deepeval` to `pyproject.toml` dependencies
- [ ] Create `workers/codeforge/evaluation/__init__.py` — package init
- [ ] Create `workers/codeforge/evaluation/runner.py` — `BenchmarkRunner` class:
  - Accepts a list of `BenchmarkTask` (input prompt, expected output, expected tool calls)
  - Runs the task through the existing agent executor (`execute_with_runtime` or conversation loop)
  - Captures actual output, tool calls, step count, cost, tokens
  - Returns `BenchmarkResult` with all captured data
- [ ] Create `workers/codeforge/evaluation/metrics.py` — metric wrappers:
  - `evaluate_task_completion(input, actual_output, expected_output)` — uses DeepEval `TaskCompletionMetric`
  - `evaluate_tool_correctness(expected_tools, actual_tools)` — uses DeepEval `ToolCorrectnessMetric` with strict mode (name + args + ordering)
  - `evaluate_step_efficiency(trace)` — uses DeepEval `StepEfficiencyMetric`
  - `evaluate_faithfulness(output, context)` — uses DeepEval `FaithfulnessMetric` (for RAG-backed agent responses)
  - `evaluate_answer_relevancy(input, output)` — uses DeepEval `AnswerRelevancyMetric`
  - All metrics use LiteLLM as judge LLM (configure via `DeepEvalBaseLLM` wrapper pointing to `LITELLM_URL`)
- [ ] Create `workers/codeforge/evaluation/litellm_judge.py` — custom `DeepEvalBaseLLM` subclass:
  - Wraps LiteLLM proxy (`http://codeforge-litellm:4000/v1/chat/completions`) as the evaluation judge
  - Configurable model for judging (e.g., `openai/gpt-4o` or `anthropic/claude-sonnet`)
  - Handles auth headers, retries, timeout
- [ ] Create `workers/codeforge/evaluation/datasets.py` — benchmark dataset management:
  - `load_dataset(path: str) -> list[BenchmarkTask]` — loads YAML benchmark files
  - `save_results(results: list[BenchmarkResult], path: str)` — saves results as JSON
  - Built-in sample dataset: 10-20 basic coding tasks (explain code, find bug, write function, refactor)

**Benchmark task YAML format (`data/benchmarks/`):**

- [ ] Create `data/benchmarks/` directory
- [ ] Create `data/benchmarks/basic-coding.yaml` — sample dataset with 10-20 tasks:
  - Each task: `id`, `name`, `input` (user prompt), `expected_output` (reference answer), `expected_tools` (list of tool names + args), `context` (optional retrieval context), `difficulty` (easy/medium/hard)
  - Tasks cover: code explanation, bug detection, function writing, refactoring, test writing
- [ ] Create `data/benchmarks/README.md` — format documentation, how to add custom benchmarks

**NATS subject for benchmark runs:**

- [ ] Add `benchmark.run.request` subject — Go Core publishes, Python Worker subscribes
- [ ] Add `benchmark.run.result` subject — Python Worker publishes results back
- [ ] Message format: `BenchmarkRunRequest { run_id, dataset_path, model, metrics: list[str] }`
- [ ] Result format: `BenchmarkRunResult { run_id, tasks: list[{task_id, scores: dict[str, float], output, cost, tokens, duration}], summary: {avg_scores, total_cost, total_time} }`
- [ ] Subscribe in `consumer.py` — `_handle_benchmark_run()` method, only when dev mode is active

**Tests:**

- [ ] `workers/tests/test_evaluation_runner.py` — unit tests for BenchmarkRunner with mocked executor
- [ ] `workers/tests/test_evaluation_metrics.py` — unit tests for metric wrappers with mocked DeepEval
- [ ] `workers/tests/test_litellm_judge.py` — integration test for LiteLLM judge wrapper

#### 20B: AgentNeo Integration — Observability & Tracing

**Dependencies:** `pip install agentneo` in Python worker Poetry environment

**Python Worker — tracing instrumentation:**

- [ ] Add `agentneo` to `pyproject.toml` dependencies
- [ ] Create `workers/codeforge/tracing/__init__.py` — package init
- [ ] Create `workers/codeforge/tracing/setup.py` — `TracingManager` class:
  - `init(project_name, enabled: bool)` — creates AgentNeo `Tracer` instance, only when dev mode active
  - `get_tracer()` — returns the active tracer (or a no-op stub when disabled)
  - Auto-instrumentation: call `tracer.instrument_litellm()` to monkey-patch all LiteLLM calls
  - Session management: `start_session(run_id)`, `end_session(run_id)`
- [ ] Instrument `workers/codeforge/executor.py` — add `@tracer.trace_agent("executor")` decorator to `execute_with_runtime()`
- [ ] Instrument `workers/codeforge/mcp_workbench.py` — add `@tracer.trace_tool("mcp_tool")` decorator to `call_tool()`
- [ ] Instrument `workers/codeforge/conversation.py` (or equivalent conversation loop) — trace each LLM call and tool execution step
- [ ] Create `workers/codeforge/tracing/metrics.py` — AgentNeo metric evaluation:
  - `evaluate_tool_selection_accuracy(session)` — how well agent selects appropriate tools
  - `evaluate_goal_decomposition(session)` — how well agent breaks down complex tasks
  - `evaluate_plan_adaptability(session)` — how well agent adapts when tools fail or return unexpected results
  - All use AgentNeo's built-in `execute()` method with LLM judge

**Dashboard integration:**

- [ ] Create `workers/codeforge/tracing/dashboard.py` — optional dashboard launcher:
  - `launch(port: int = 3100)` — starts AgentNeo React dashboard (only in dev mode)
  - Exposes trace history, execution graphs, token/cost breakdowns per run
- [ ] Add `benchmark.dashboard_port` config key to `codeforge.yaml` (default: 3100, 0 = disabled)
- [ ] Document dashboard access in `docs/dev-setup.md`

**Tests:**

- [ ] `workers/tests/test_tracing_setup.py` — test TracingManager init, no-op when disabled
- [ ] `workers/tests/test_tracing_instrumentation.py` — verify decorators don't break executor flow

#### 20C: GEMMAS-Inspired Metrics — Multi-Agent Collaboration Quality

> Custom implementation based on GEMMAS paper (arxiv.org/abs/2507.13190).
> No code was released, so we implement the two core metrics ourselves.
> Only relevant once multi-agent workflows (DAG orchestration) are active.

**Python Worker — collaboration metrics (`workers/codeforge/evaluation/collaboration.py`):**

- [ ] Create `workers/codeforge/evaluation/collaboration.py` with two metric classes:

- [ ] **`InformationDiversityScore` (IDS):**
  - Input: list of agent messages from a multi-agent run (each: `agent_id`, `content`, `round`, `parent_agent_id`)
  - Build DAG: nodes = agents, edges = message flow between agents
  - Build spatial adjacency matrix S (direct communication links) and temporal matrix T (causal dependencies across rounds)
  - Compute pairwise similarity using TF-IDF (syntactic) + sentence-transformers BERT embeddings (semantic)
  - Combine with equal weights: `SS_total = 0.5 * tfidf_sim + 0.5 * bert_sim`
  - Compute IDS: weighted average of `(1 - SS_total)` across all connected agent pairs
  - Return score 0.0-1.0 (higher = more diverse contributions, less redundancy)

- [ ] **`UnnecessaryPathRatio` (UPR):**
  - Input: same DAG as IDS + correctness scores per reasoning path (from DeepEval or manual annotation)
  - Enumerate all paths in DAG from root agents to final output
  - Classify paths: necessary (correctness >= 0.5) vs unnecessary (correctness < 0.5)
  - Compute UPR: `1 - |necessary_paths| / |all_paths|`
  - Return score 0.0-1.0 (lower = more efficient, fewer wasted reasoning paths)

- [ ] Add `sentence-transformers` to `pyproject.toml` dependencies (for BERT embeddings in IDS)
- [ ] Add `scikit-learn` dependency (for TF-IDF computation in IDS)

**Integration with event stream:**

- [ ] Create `workers/codeforge/evaluation/dag_builder.py` — `build_collaboration_dag(messages: list[AgentMessage]) -> CollaborationDAG`:
  - Parses agent-to-agent messages from NATS event stream or trajectory recording
  - Builds adjacency matrices (spatial + temporal)
  - Returns structured DAG object compatible with IDS and UPR computation
- [ ] Hook into trajectory recording system — when a multi-agent run completes, automatically compute IDS + UPR if benchmark mode is active

**Tests:**

- [ ] `workers/tests/test_collaboration_ids.py` — test IDS with known diverse vs redundant message sets
- [ ] `workers/tests/test_collaboration_upr.py` — test UPR with known necessary vs unnecessary paths
- [ ] `workers/tests/test_dag_builder.py` — test DAG construction from sample agent messages

#### 20D: Go Core — Benchmark API & Dev-Mode Gate

**Benchmark API endpoints (only accessible in dev mode):**

- [ ] Add dev-mode check middleware: `func devModeOnly(cfg *config.Config) func(http.Handler) http.Handler` — returns 403 if `dev_mode: false`
- [ ] `POST /api/v1/benchmark/runs` — start a benchmark run:
  - Request body: `{ dataset: string, model: string, metrics: []string }`
  - Publishes `benchmark.run.request` to NATS
  - Returns `{ run_id: string, status: "started" }`
  - Stores benchmark run in DB (new table `benchmark_runs`)
- [ ] `GET /api/v1/benchmark/runs` — list all benchmark runs with summary scores
- [ ] `GET /api/v1/benchmark/runs/{id}` — get detailed results for a single run (per-task scores, outputs, costs)
- [ ] `DELETE /api/v1/benchmark/runs/{id}` — delete a benchmark run and its results
- [ ] `GET /api/v1/benchmark/datasets` — list available benchmark datasets (reads `data/benchmarks/*.yaml`)
- [ ] `GET /api/v1/benchmark/compare?runs=id1,id2` — compare two or more runs side-by-side (score deltas, regression detection)

**Database migration:**

- [ ] New migration `0XX_create_benchmark_runs.sql`:
  - `benchmark_runs` table: `id`, `dataset`, `model`, `status` (running/completed/failed), `summary_scores` (JSONB), `total_cost`, `total_tokens`, `total_duration_ms`, `created_at`, `completed_at`
  - `benchmark_results` table: `id`, `run_id` (FK), `task_id`, `task_name`, `scores` (JSONB: metric_name -> score), `actual_output`, `expected_output`, `tool_calls` (JSONB), `cost_usd`, `tokens_in`, `tokens_out`, `duration_ms`

**Config:**

- [ ] Add `benchmark` section to `codeforge.yaml`:
  ```yaml
  benchmark:
    enabled: false          # only true in dev mode
    datasets_dir: "data/benchmarks"
    judge_model: "openai/gpt-4o"  # LLM used by DeepEval as judge
    dashboard_port: 3100    # AgentNeo dashboard, 0 = disabled
  ```

#### 20E: Frontend — Benchmark Dashboard Page

**New page: `/benchmark` (only visible in dev mode):**

- [ ] Create `frontend/src/features/benchmark/BenchmarkPage.tsx` — main benchmark page:
  - Header: "Agent Benchmark" title + "New Run" button
  - Run list table: ID, dataset, model, status, avg scores (task completion, tool correctness, step efficiency), total cost, duration, date
  - Status badges: running (blue pulse), completed (green), failed (red)

- [ ] Create `frontend/src/features/benchmark/BenchmarkRunDetail.tsx` — single run detail view:
  - Summary card: overall scores (gauges/progress bars for each metric 0-100%), total cost, total tokens, duration
  - Per-task results table: task name, difficulty, individual metric scores, pass/fail, expand to see full output
  - Expandable diff view: expected vs actual output side-by-side
  - Tool call timeline: which tools were called, in what order, with what arguments
  - Cost breakdown: per-task token usage and cost

- [ ] Create `frontend/src/features/benchmark/BenchmarkCompare.tsx` — run comparison view:
  - Select 2-3 runs to compare
  - Side-by-side score comparison (bar charts or table with delta indicators)
  - Regression detection: highlight metrics that got worse (red) or better (green)
  - Cost-performance scatter: X = cost, Y = score, plot all compared runs

- [ ] Create `frontend/src/features/benchmark/NewBenchmarkRunDialog.tsx` — start new run dialog:
  - Dataset selector dropdown (from `GET /api/v1/benchmark/datasets`)
  - Model selector (from available LiteLLM models)
  - Metrics multi-select checkboxes: task_completion, tool_correctness, step_efficiency, faithfulness, answer_relevancy
  - "Start Run" button → `POST /api/v1/benchmark/runs`

- [ ] Add API client methods in `frontend/src/api/client.ts`:
  - `api.benchmark.listRuns()`, `api.benchmark.getRun(id)`, `api.benchmark.createRun(req)`, `api.benchmark.deleteRun(id)`, `api.benchmark.comparRuns(ids)`, `api.benchmark.listDatasets()`

- [ ] Add TypeScript types in `frontend/src/api/types.ts`:
  - `BenchmarkRun`, `BenchmarkResult`, `BenchmarkDataset`, `BenchmarkCompareResponse`

- [ ] Add route `/benchmark` in router (conditionally rendered only when dev mode is active)
- [ ] Add "Benchmark" nav item in sidebar (conditionally rendered only when dev mode is active)

- [ ] Add i18n keys for all benchmark UI strings in locale files

#### 20F: Documentation & ADR

- [ ] Create `docs/architecture/adr/008-benchmark-evaluation-framework.md`:
  - Context: Need to measure agent quality beyond "does it work"
  - Decision: DeepEval (primary metrics) + AgentNeo (tracing) + GEMMAS-inspired (collaboration)
  - Alternatives considered: SPARC-Bench (too Roo-coupled), REALM-Bench (no license, logistics-focused), RAGAS (RAG-only)
  - Consequences: Python-only evaluation, LLM-as-judge cost overhead, dev-mode only
- [ ] Update `docs/features/04-agent-orchestration.md` — add benchmark mode section
- [ ] Update `docs/dev-setup.md` — add benchmark configuration, AgentNeo dashboard access, benchmark dataset format
- [ ] Update `docs/tech-stack.md` — add DeepEval, AgentNeo, sentence-transformers to Python dependencies
- [ ] Update `CLAUDE.md` — add benchmark references to architecture section

---

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
