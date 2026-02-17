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

- [ ] Design Policy domain model
  - `internal/domain/policy.go`: PolicyProfile, PermissionRule, ToolSpecifier
  - Permission modes: `default`, `acceptEdits`, `plan`, `delegate`
  - Rule evaluation: deny → ask → allow (first match wins)
- [ ] Implement Permission Gate middleware
  - Before every mutating ToolCall: evaluate policy rules
  - ToolSpecifier patterns: `Read`, `Edit`, `Bash(git status:*)`, `Bash(go test:*)`
  - Path allow/deny lists (glob patterns)
  - Command allow/deny lists for Bash
- [ ] Implement Checkpoint system
  - Before every `Edit/Write/Replace`: automatic checkpoint
  - On failed Quality Gates (tests/lint): automatic rewind or re-plan
  - Shadow Git approach for Mount mode, Blob snapshots for Sandbox
- [ ] Quality Gates (Definition of Done)
  - Configurable per project: `requireTestsPass`, `requireLintPass`
  - `rollbackOnGateFail`: automatic rewind on gate failure
  - `maxSteps`, `timeoutSeconds`, `maxCost`, `stallDetection` as termination conditions
- [ ] Policy Presets (4 built-in profiles)
  - `plan-readonly`: Debug/Preview, no side-effects, read-only tools only
  - `headless-safe-sandbox`: Default for autonomous server jobs, safety first
  - `headless-permissive-sandbox`: Batch/refactor, more freedom, still sandboxed
  - `trusted-mount-autonomous`: Power-user, direct mount, all local tools allowed
- [ ] Policy UI in Frontend
  - Policy Editor per project (YAML-style, fits CodeForge config standard)
  - "Effective Permission Preview": show which rule matches and why
  - Scope levels: global (user) → project → run/session (override)
  - Preset selection + "Customize" (load preset, edit, save as new profile)
  - Run overrides: temporarily override policy per run

### 4B. Runtime API (Austauschbare Execution Environments)

- [ ] Define Runtime Client protocol (Go ↔ Python)
  - ToolCall/ToolResult schema: exit code, stdout/stderr, diff, touched paths
  - Typed events: `ToolCallRequest`, `ToolCallResult`, `FileEdit`, `ShellExec`
  - NATS subjects for runtime communication
- [ ] Implement Execution Modes
  - Sandbox: Isolated Docker container with cgroups v2 limits
  - Mount: Direct file access to host workspace
  - Hybrid: Read from host, write in sandbox, merge on success
- [ ] Runtime Compliance Tests
  - Test suite that validates each Runtime implementation
  - Feature parity checks across Sandbox/Mount/Hybrid

### 4C. Headless Autonomy (Server-First Execution)

- [ ] Auto-Approval rules per tool category
  - Read/Grep always allowed
  - Bash/Edit only in whitelisted paths or in sandbox
  - Network access configurable (deny by default in headless)
- [ ] Termination Conditions (replace HITL)
  - MaxSteps, wall-time timeout, budget/token limits
  - Stall Detection: no progress for N steps → re-plan or abort
  - "Definition of Done": tests pass, lint pass, diff under limit
- [ ] Deliver modes for headless output
  - `patch`: Generate diff/patch file
  - `commit-local`: Git commit locally (no push)
  - `pr`: Create pull request via API
  - `branch`: Push to feature branch only
- [ ] API endpoint for external triggers
  - POST `/api/v1/runs` for GitHub Actions, GitLab CI, Jenkins, cron jobs
  - Accept policy profile override per run

---

## Phase 5 — Multi-Agent Orchestration

> Source: Analyse-Dokument Section "Multi-Agent Orchestration Architecture"

### 5A. Orchestrator Agent (Meta-Agent)

- [ ] Domain model: `internal/domain/orchestrator.go`
  - Orchestrator entity (ID, ProjectID, Mode, Strategy, MaxParallel, State)
  - OrchestratorMode: `manual`, `semi_auto`, `full_auto`
  - OrchestrationStrategy: TaskDecomposition, TeamFormation, ContextOptimizer
- [ ] Orchestrator Service
  - Reads Feature Map / TODO list
  - Decomposes features into subtasks (context-optimized)
  - Decides agent strategy (single, pair, team)
  - Monitors progress, reacts to failures (re-plan, retry)

### 5B. Task Decomposition (Execution Plans)

- [ ] Domain model: `internal/domain/execution_plan.go`
  - ExecutionPlan: tasks, DAG dependencies, estimated cost, state
  - PlannedTask: context estimate, files, acceptance criteria, strategy
  - ContextEstimate: token count, file sizes, split suggestions
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

- [ ] Unit tests for AsyncHandler (buffer overflow, concurrent writes, flush)
- [ ] Integration tests for Config Loader (precedence, validation, reload)
- [ ] Integration tests for Idempotency (duplicate requests, TTL expiry)
- [ ] Load tests for Rate Limiting (sustained vs burst, per-user limiters)
- [ ] Runtime Compliance Tests (Sandbox/Mount/Hybrid feature parity)
- [ ] Policy Gate tests (deny/ask/allow evaluation, path scoping)

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
