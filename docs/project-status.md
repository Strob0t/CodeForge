# CodeForge — Project Status

> Last update: 2026-02-19

### Phase 0: Project Setup (COMPLETED)

#### Completed

- [x] Market research conducted (docs/research/market-analysis.md) (20+ existing projects analyzed, market gap identified: no integrated solution for Project Dashboard + Roadmap + Multi-LLM + Agent Orchestration, SVN support confirmed as unique selling point)
- [x] Architecture decision: Go + TypeScript + Python (Three-Layer Hybrid)
- [x] Devcontainer configured (Go 1.23, Python 3.12, Node.js 22, Docker-in-Docker)
- [x] Linting/Formatting for all three languages (Ruff, golangci-lint, ESLint/Prettier)
- [x] Pre-commit Hooks (.pre-commit-config.yaml)
- [x] Python package management with Poetry (pyproject.toml)
- [x] Docker Compose for dev services (docs-mcp, playwright-mcp)
- [x] MCP Server Integration (.mcp.json)
- [x] .gitignore
- [x] CLAUDE.md (Project context for AI assistants)
- [x] Documentation (docs/)
- [x] Software architecture defined: Hexagonal Architecture + Provider Registry Pattern
- [x] LLM Capability Levels and Worker Modules defined (GraphRAG, Debate, Routing)
- [x] Agent Execution Modes defined (Sandbox, Mount, Hybrid)
- [x] Agent workflow defined (Plan → Approve → Execute → Review → Deliver)
- [x] Safety Layer designed (8 components: Budget, Command Safety, Branch Isolation, Test/Lint Gate, Max Steps, Rollback, Path Blocklist, Stall Detection)
- [x] Quality Layer extended: Action Sampling, RetryAgent + Reviewer, LLM Guardrail Agent, Debate (4 tiers)
- [x] YAML-based Tool Bundles, History Processors, Hook System, Trajectory Recording
- [x] Cost management designed (budget limits per task/project/user)
- [x] Competitive analysis deepened: BjornMelin/codeforge, Open SWE, SWE-agent, Devika
- [x] Jinja2 Prompt Templates, KeyBERT, Real-time WebSocket State designed
- [x] Frontend framework chosen: SolidJS + Tailwind CSS
- [x] Git workflow with commit checklist (pre-commit + documentation maintenance)
- [x] Orchestration frameworks analyzed: LangGraph, CrewAI, AutoGen, MetaGPT (detailed feature comparison and architecture mapping, adopted patterns identified and documented)
- [x] Framework insights integrated into architecture: Composite Memory Scoring, Context Window Strategies, Experience Pool, Tool Recommendation via BM25, Workbench, LLM Guardrail Agent, Structured Output / ActionNode, Event Bus for Observability, GraphFlow / DAG Orchestration, Composable Termination Conditions, Component System, Document Pipeline PRD→Design→Tasks→Code, MagenticOne Planning Loop, HandoffMessage Pattern, Human Feedback Provider Protocol
- [x] LLM Routing & Multi-Provider analyzed: LiteLLM (127+ Providers, Proxy Server, Router with 6 strategies, Budget Management, 42+ Observability), OpenRouter (300+ Models, Cloud-only, ~5.5% Fee as provider behind LiteLLM), Claude Code Router (Scenario-based Routing), OpenCode CLI (OpenAI-compatible Base URL Pattern, Copilot Token Exchange, Auto-Discovery)
- [x] Architecture decision: No custom LLM interface, LiteLLM Proxy as Docker sidecar (Go Core and Python Workers use OpenAI-compatible API against LiteLLM Port 4000, scenario-based routing via LiteLLM Tag-based Routing, custom development: Config Manager, User Key Mapping, Scenario Router, Cost Dashboard, Local Model Discovery, Copilot Token Exchange)
- [x] Roadmap/Spec/PM tools analyzed: OpenSpec, Spec Kit, Autospec, Plane.so, OpenProject, Ploi Roadmap (6+ SDD tools, 4+ PM tools, repo-based PM tools mapped, ADR/RFC tools and feature flag tools identified, Gitea/Forgejo identified as GitHub-compatible SCM alternative)
- [x] Auto-Detection architecture designed: Three-Tier Detection (Repo → Platform → File) (Spec-Driven Detectors: OpenSpec, Spec Kit, Autospec, ADR/RFC; Platform Detectors: GitHub, GitLab, Plane.so, OpenProject; File-Based Detectors: ROADMAP.md, TASKS.md, CHANGELOG.md)
- [x] Provider Registry extended: specprovider + pmprovider (same architecture as Git)
- [x] Architecture decision: No custom PM tool, bidirectional sync with existing tools (Adopted patterns: Cursor Pagination, HMAC-SHA256, Label Sync from Plane, Optimistic Locking, Schema Endpoints from OpenProject, Delta Spec Format from OpenSpec, `/ai` Endpoint from Ploi Roadmap; Explicitly NOT adopted: HAL+JSON/HATEOAS, GraphQL, custom PM tool)
- [x] Deep analysis of all AI Coding Agents (Section 1+2 in market-analysis.md): OpenHands (Event Sourcing, AgentHub, Microagents, Risk Management, V0→V1 SDK), SWE-agent (ACI, ReAct Loop, Tool Bundles, History Processors, SWE-ReX, Mini-SWE-Agent), Aider (tree-sitter Repo Map, 7+ Edit Formats, Architect/Editor Pattern in separate file: aider-deep-analysis.md), Cline (Three-Tier Runtime, Plan/Act Mode, Shadow Git, MCP, Ask/Say Approval), Devika (9 Sub-Agents, Jinja2 Templates, SentenceBERT, Agent State Visualization)
- [x] Extended competitive analysis: 12 new tools identified and analyzed (5 new competitors: Codel, AutoForge, bolt.diy, Dyad, CLI Agent Orchestrator; 7 new backend candidates: Goose, OpenCode, Plandex, AutoCodeRover, Roo Code, Codex CLI, SERA; backend integration priorities defined with Goose, OpenCode, Plandex as Priority 1; closest competitor identified: CLI Agent Orchestrator from AWS with same vision without Web GUI)
- [x] Architecture decision: YAML as unified configuration format (comment support)
- [x] Autonomy spectrum defined: 5 levels (supervised → headless) (Safety rules replace user at levels 4-5 with Budget, Tests, Blocklists, Branch Isolation; headless mode for CI/CD, cron jobs, API-driven pipelines; schedule support for automatic nightly reviews, dependency updates; API endpoint for external systems like GitHub Actions, GitLab CI, Jenkins)
- [x] Agent specialization elaborated: Modes System with YAML configs (Built-in Modes: architect, coder, reviewer, debugger, tester, lint-fixer, planner, researcher; Custom Modes: user-definable in `.codeforge/modes/`; Mode Pipelines and DAG composition for multi-agent workflows; each mode: own tools, LLM scenario, autonomy level, prompt template)
- [x] Coding agent insights integrated into architecture: Shadow Git Checkpoints, Event-Sourcing, Microagents from OpenHands; Diff-based File Review, ACI from SWE-agent, Stateless Agent Design from Devika; tree-sitter Repo Map, Architect/Editor Pattern, Edit Formats from Aider; Skills System, Risk Management from OpenHands, Plan/Act Mode from Cline
- [x] Architecture decision: PostgreSQL 17 as primary database (shared with LiteLLM) — [ADR-002](architecture/adr/002-postgresql-database.md) (pgx v5 Go driver, goose migrations, psycopg3 Python driver, shared instance with LiteLLM via schema separation, simplicity principle: no ORM, no code generator, no extra tooling)
- [x] Library decisions finalized (minimal-dependency principle): Go: chi v5 router with 0 deps, coder/websocket with 0 deps, git exec wrapper with 0 deps; Frontend: @solidjs/router, Tailwind CSS with no component lib, @solid-primitives/websocket at 728B, native fetch; Rejected: Echo, Fiber, gorilla/websocket, go-git with 28 deps, axios, styled-components, Kobalte
- [x] Protocol support analyzed and prioritized: MCP, LSP, OpenTelemetry, A2A, AG-UI (Tier 1 Phase 1-2: MCP, LSP, OpenTelemetry GenAI; Tier 2 Phase 2-3: A2A, AG-UI; Tier 3 future: ANP, LSAP)
- [x] Documentation consistency audit: all docs synchronized and translated to English
- [x] Documentation structure created: docs/README.md documentation index, docs/todo.md central TODO tracker for LLM agents, docs/features/ individual feature specs for all 4 pillars, docs/architecture/adr/ ADR template for future decisions, Documentation Policy added to CLAUDE.md

#### Open

> Phase 0 complete. All tasks done — proceed to Phase 1.
> For granular tasks, see [todo.md](todo.md).

- [x] Devcontainer verified (Go 1.23.12, Python 3.12.12, Node.js 22.22.0, Docker-in-Docker)
- [x] Go module initialized, project structure created, chi HTTP server with health endpoint
- [x] Python Workers scaffold (consumer, health, 3 tests passing)
- [x] SolidJS frontend initialized (Tailwind CSS v4, ESLint 9, Prettier, @solidjs/router)

### Phase 1: Foundation (COMPLETED)

- [x] (2026-02-14) WP1: Infrastructure — Docker Compose (NATS, LiteLLM), DB schema, migrations
- [x] (2026-02-14) WP2: Go Core — Domain entities, ports, registries, WebSocket, NATS adapter
- [x] (2026-02-14) WP3: Python Worker — NATS consumer, LiteLLM client, Pydantic models, 16 tests
- [x] (2026-02-14) WP4: Go Core — DB store, REST API (projects/tasks), services, handler tests
- [x] (2026-02-14) WP5: Frontend — API client, WebSocket, Dashboard page with CRUD
- [x] (2026-02-14) WP6: Protocol stubs (MCP, LSP, OTEL), GitHub Actions CI

#### Phase 1 Key Deliverables
- **Go:** 1.24, chi v5, pgx v5, goose, coder/websocket, nats.go — 0 lint issues, 11 tests (expanded to 27 in Phase 2)
- Python: nats-py, httpx, pydantic v2 — 16 tests, ruff clean
- Frontend: SolidJS, @solidjs/router, @solid-primitives/websocket — build + lint + format clean
- CI: 3-job GitHub Actions (Go, Python, Frontend), branch protection script for `main`
- API: 9 REST endpoints, WebSocket, health with service status

### Phase 2: MVP Features (COMPLETED)

- [x] (2026-02-14) WP1: Git Local Provider — Clone, Status, Pull, ListBranches, Checkout via git CLI
- [x] (2026-02-14) WP2: Agent Lifecycle — Aider backend, async NATS dispatch, agent CRUD API
- [x] (2026-02-14) WP3: WebSocket Events — Live agent output, task/agent status broadcasting
- [x] (2026-02-14) WP4: LLM Provider Management — LiteLLM admin API client, model CRUD endpoints
- [x] (2026-02-14) WP5: Frontend — Project detail page, git operations UI, task list
- [x] (2026-02-14) WP6: Frontend — Agent monitor panel, live terminal output, task create/expand
- [x] (2026-02-14) WP7: Frontend — LLM models page, add/delete models, health status
- [x] (2026-02-14) WP8: Integration test, documentation update, test fixes

#### Phase 2 Key Deliverables
- **Go:** 27 tests, gitlocal provider, aider backend, agent service, LiteLLM client, 19 REST endpoints
- Python: 16 tests, streaming output via NATS, LiteLLM health checks
- Frontend: 13 components, 4 routes (/, /projects, /projects/:id, /models), WebSocket live updates
- API: Git ops (clone/pull/branches/checkout/status), Agent CRUD + dispatch/stop, LLM CRUD + health

### Phase 3: Reliability, Performance & Agent Foundation (COMPLETED)

- [x] (2026-02-17) WP1: Configuration Management — hierarchical config (defaults < YAML < ENV < CLI), typed Config struct, validation, 6 tests
- [x] (2026-02-19) WP1 continued: CLI override support — `ParseFlags`, `LoadWithCLI`, `--config/-c`, `--port/-p`, `--log-level`, `--dsn`, `--nats-url`, 8 tests
- [x] (2026-02-17) WP2: Structured Logging & Request ID — slog factory, structlog, X-Request-ID propagation HTTP→NATS→Python, 6 new tests
- [x] (2026-02-17) WP3: Graceful Shutdown & Docker Logging — 4-phase ordered shutdown, NATS Drain, Docker log rotation, logs.sh helper
- [x] (2026-02-17) WP4: Optimistic Locking & DB Pool Tuning
- [x] (2026-02-17) WP5: Circuit Breaker
- [x] (2026-02-17) WP6: Dead Letter Queue & Schema Validation
- [x] (2026-02-17) WP7: Event Sourcing for Agent Trajectory
- [x] (2026-02-17) WP8: Health Granularity & Rate Limiting

### Phase 4: Agent Execution Engine (COMPLETED)

#### 4A. Policy Layer (COMPLETED)

- [x] (2026-02-17) Domain model: PolicyProfile, PermissionRule, ToolSpecifier, ToolCall, QualityGate, TerminationCondition
- [x] (2026-02-17) Validation: name required, valid mode/decision, non-negative limits
- [x] (2026-02-17) 4 built-in presets: plan-readonly, headless-safe-sandbox, headless-permissive-sandbox, trusted-mount-autonomous
- [x] (2026-02-17) YAML loader: LoadFromFile, LoadFromDirectory (custom policies)
- [x] (2026-02-17) Policy evaluator: first-match-wins rules, glob matching (incl. **), path/command constraints
- [x] (2026-02-17) Config integration: Policy.DefaultProfile, Policy.CustomDir, ENV overrides
- [x] (2026-02-17) REST API: GET /policies, GET /policies/{name}, POST /policies/{name}/evaluate
- [x] (2026-02-17) Composition root wiring in cmd/codeforge/main.go
- [x] (2026-02-17) 46 test functions: domain (20), service (25), config (3), handlers (7) — all passing

#### Phase 4A Key Deliverables
- **New files:** 8 (policy domain types, validate, presets, loader, service, 3 test files)
- Modified files: 7 (config, loader, handlers, routes, main, config tests, handler tests)
- API: 3 new REST endpoints under /api/v1/policies
- Tests: 46 new test functions, all Go tests pass (153+ total)

#### 4A-UI. Policy UI in Frontend (COMPLETED)

- [x] (2026-02-18) Backend: CRUD endpoints — POST /policies (create), DELETE /policies/{name} (delete custom only)
- [x] (2026-02-18) Backend: SaveProfile, DeleteProfile on PolicyService; IsPreset helper; SaveToFile in loader
- [x] (2026-02-18) Frontend: 10 new type definitions (PolicyProfile, PermissionRule, QualityGate, etc.)
- [x] (2026-02-18) Frontend: Extended API client (get, create, delete, evaluate)
- [x] (2026-02-18) Frontend: PolicyPanel component with 3 views (list, detail + evaluate tester, editor)
- [x] (2026-02-18) Frontend: Integrated into ProjectDetailPage between agents and run management
- [x] (2026-02-18) Tests: 6 new service tests (SaveProfile, DeleteProfile), 6 new handler tests (create/delete endpoints)

#### 4B. Runtime API — Step-by-Step Execution Protocol (COMPLETED)

- [x] (2026-02-17) CI Fix: golangci-lint-action v6→v7, Python working-directory removed
- [x] (2026-02-17) Domain: Run entity (run.go, validate.go, toolcall.go) + 15 domain tests
- [x] (2026-02-17) NATS: 7 new subjects (runs.*) + 8 payload types in schemas.go
- [x] (2026-02-17) Database: migration 005_create_runs.sql, Store interface extended (5 methods)
- [x] (2026-02-17) Events: 6 new event types + 2 WS event types with structs
- [x] (2026-02-17) RuntimeService: StartRun, HandleToolCallRequest (with termination + policy eval), HandleToolCallResult, HandleRunComplete, CancelRun, StartSubscribers
- [x] (2026-02-17) REST API: POST /runs, GET /runs/{id}, POST /runs/{id}/cancel, GET /tasks/{id}/runs
- [x] (2026-02-17) Composition root: RuntimeService wired in main.go with subscribers + shutdown
- [x] (2026-02-17) Python: RuntimeClient (runtime.py), RunStartMessage/TerminationConfig/ToolCallDecision models, consumer extended with runs.start, executor with execute_with_runtime()
- [x] (2026-02-17) 44 new test functions: Go domain (15), Go service (22), Go handlers (+5), Python runtime (9)

#### Phase 4B Key Deliverables
- **New files:** 6 Go (run domain 3, runtime service, runtime tests, migration), 2 Python (runtime.py, test_runtime.py)
- Modified files: 12 Go (queue.go, schemas.go, store.go, event.go, events.go, handlers.go, routes.go, main.go, handlers_test.go, project_test.go), 3 Python (models.py, consumer.py, executor.py), 1 CI (.github/workflows/ci.yml)
- API: 4 new REST endpoints under /api/v1/runs + /api/v1/tasks/{id}/runs
- Protocol: Conversational NATS protocol for per-tool-call policy enforcement
- Tests: 44 new test functions (Go: 42, Python: 9), all passing

#### 4C. Headless Autonomy — Stall Detection, Quality Gates, Delivery (COMPLETED)

- [x] (2026-02-17) CI Fix: golangci-lint v2 config migration (.golangci.yml)
- [x] (2026-02-17) Config extension: `config.Runtime` struct (6 fields) + ENV overrides + YAML example
- [x] (2026-02-17) Stall Detection: StallTracker (FNV-64a hash ring buffer), per-run tracking in RuntimeService
- [x] (2026-02-17) Quality Gate Enforcement: NATS request/result protocol, Python QualityGateExecutor
- [x] (2026-02-17) Deliver Modes: 5 strategies (none, patch, commit-local, branch, pr) via DeliverService
- [x] (2026-02-17) Frontend: RunPanel component, Run types/API, WS event integration
- [x] (2026-02-17) Events: 7 new event types + 2 WS event types (QG + delivery)

#### Phase 4C Key Deliverables
- **New files:** 5 Go (stall.go, stall_test.go, deliver.go, deliver_test.go, migration 006), 2 Python (qualitygate.py, test_qualitygate.py), 1 Frontend (RunPanel.tsx)
- Modified files: 12 Go (config.go, loader.go, policy.go, presets.go, run.go, validate.go, event.go, events.go, queue.go, schemas.go, runtime.go, runtime_test.go, store.go, handlers_test.go, main.go), 3 Python (runtime.py, models.py, consumer.py), 3 Frontend (types.ts, client.ts, ProjectDetailPage.tsx), 2 Config (.golangci.yml, codeforge.yaml.example)
- Tests: 22+ new test functions (Go: stall 10, deliver 5, runtime 6+; Python: QG 7), all passing
- Protocol: Quality gate NATS subjects (request/result), stall detection, delivery pipeline

### Phase 3 & 4 Completion: Six Missing Features (COMPLETED)

> Implemented after Phase 6C to fill gaps from Phases 3 and 4.

- [x] (2026-02-18) 3G: Deprecation Middleware — RFC 8594 `Deprecation` + `Sunset` headers, 3 tests
- [x] (2026-02-18) 3F: Secrets Vault with SIGHUP Hot Reload — RWMutex vault, EnvLoader, SIGHUP handler, LiteLLM integration, 4 tests
- [x] (2026-02-18) 3C: Heartbeat Ticker + Context-Level Timeout — Go heartbeat subscriber/timeout tracking (sync.Map), Python heartbeat ticker, config fields + ENV overrides, goroutine-based run timeout
- [x] (2026-02-18) 3D: Event Store Enhancements — RunID on events (migration 013), LoadByRun query, GET /runs/{id}/events endpoint
- [x] (2026-02-18) 3H: Multi-Tenancy Preparation — tenant_id on 7 tables (migration 014), TenantID middleware, context helpers, domain struct fields, 3 tests
- [x] (2026-02-19) 3H: Full Multi-Tenancy — tenants table (migration 018), tenant_id on all 18 tables, WHERE clauses in ~60 store methods + 6 eventstore methods, TenantService CRUD, REST API endpoints
- [x] (2026-02-19) 4A: Policy Scope Levels + "Why Matched" — EvaluationResult struct with Scope type (global/project/run), EvaluateWithReason() service method, ResolveProfile() scope resolution, evaluate endpoint returns full result, migration 019 (policy_profile on projects), Debug-level evaluation logging in runtime
- [x] (2026-02-19) 4A: Branch Protection Rules — ProtectionRule domain with glob pattern matching (filepath.Match), EvaluatePush/EvaluateMerge/EvaluateDelete functions, BranchProtectionService with CRUD + CheckBranch/CheckMerge, migration 020, tenant-aware store methods with optimistic locking, 5 REST endpoints, 6 domain test functions
- [x] (2026-02-19) 3D: Replay / Audit Trail — ReplayService (ListCheckpoints, Replay, AuditTrail, RecordAudit), LoadEventsRange + ListCheckpoints on eventstore, audit_trail table (migration 021), 4 REST endpoints (checkpoints, replay, global audit, project audit)
- [x] (2026-02-19) 3D: Session Events (Resume/Fork/Rewind) — Session entity, SessionService (Resume, Fork, Rewind), sessions table (migration 021), 6 session event types, 5 REST endpoints (resume, fork, rewind, list sessions, get session)
- [x] (2026-02-19) 4B: Hybrid Execution Mode — CreateHybrid() on SandboxService (read-write mount, no --read-only), hybrid mode in StartRun, ToolCallResponsePayload with exec_mode + container_id, HybridConfig (CommandImage, MountMode) with env overrides, refactored Start/Stop/Exec/Remove to use ContainerID
- [x] (2026-02-18) 4B: Runtime Compliance Tests — 8 sub-tests x 2 exec modes (Mount, Sandbox), 16 test cases passing

#### Key Deliverables
- **New files (11):** deprecation.go/test, vault.go, env_loader.go, vault_test.go, tenant.go/test, migrations 013/014, runtime_compliance_test.go
- Modified files (~15): config.go, loader.go, runtime.go, event.go, eventstore interface, postgres/eventstore.go, handlers.go, routes.go, main.go, domain structs (Project/Task/Agent/Run), runtime.py, test_runtime.py
- Tests: 30+ new test functions (Go + Python), all passing
- Verification: golangci-lint 0 issues, 79/79 Python tests, frontend clean, pre-commit all hooks pass

### Phase 5: Multi-Agent Orchestration (COMPLETED)

#### 5A. Execution Plans — DAG Scheduling with 4 Protocols (COMPLETED)

- [x] (2026-02-17) Domain model: `internal/domain/plan/` (plan.go, validate.go, dag.go) (ExecutionPlan, Step, CreatePlanRequest with JSON tags; Protocol sequential/parallel/ping_pong/consensus, Status, StepStatus with IsTerminal(); Validation: name required, valid protocol, protocol-specific step count rules; DAG cycle detection via Kahn's algorithm topological sort; DAG helpers: ReadySteps, RunningCount, AllTerminal, AnyFailed)
- [x] (2026-02-17) Domain tests: 25 tests (16 validation + 8 DAG + 1 compile check)
- [x] (2026-02-17) Config extension: `config.Orchestrator` (MaxParallel=4, PingPongMaxRounds=3, ConsensusQuorum=0) (ENV overrides: CODEFORGE_ORCH_MAX_PARALLEL, CODEFORGE_ORCH_PINGPONG_MAX_ROUNDS, CODEFORGE_ORCH_CONSENSUS_QUORUM)
- [x] (2026-02-17) Database: migration 007_create_execution_plans.sql (execution_plans + plan_steps tables, UUID arrays for step dependencies, FK to projects/tasks/agents/runs)
- [x] (2026-02-17) Store interface: 9 new methods (CreatePlan, GetPlan, ListPlansByProject, UpdatePlanStatus, CreatePlanStep, ListPlanSteps, UpdatePlanStepStatus, GetPlanStepByRunID, UpdatePlanStepRound)
- [x] (2026-02-17) Postgres adapter: transactional CreatePlan, auto-loading steps in GetPlan
- [x] (2026-02-17) Events: 5 plan event types + 2 WS event types (plan.status, plan.step.status)
- [x] (2026-02-17) RuntimeService callback: SetOnRunComplete + invocation in finalizeRun
- [x] (2026-02-17) OrchestratorService: CreatePlan, StartPlan, GetPlan, ListPlans, CancelPlan, HandleRunCompleted (Sequential: one step at a time, failure stops plan; Parallel: all ready steps up to MaxParallel; PingPong: 2 agents alternate for N rounds; Consensus: same task to multiple agents, majority quorum vote; Core scheduling: mutex-protected advancePlan with protocol dispatch)
- [x] (2026-02-17) REST API: 5 new endpoints (POST/GET /projects/{id}/plans, GET /plans/{id}, POST /plans/{id}/start, POST /plans/{id}/cancel)
- [x] (2026-02-17) Composition root: OrchestratorService wired with onRunComplete callback
- [x] (2026-02-17) Frontend: PlanPanel component, plan types + API client, WS event integration
- [x] (2026-02-17) Tests: 12 orchestrator service tests + 25 domain tests, all passing

#### Phase 5A Key Deliverables
- **New files:** 8 Go (plan domain 3, plan tests 2, orchestrator service, orchestrator tests, migration), 1 Frontend (PlanPanel.tsx)
- Modified files: 13 Go (config.go, loader.go, store.go, postgres/store.go, event.go, events.go, runtime.go, handlers.go, routes.go, main.go, handlers_test.go, runtime_test.go, project_test.go), 3 Frontend (types.ts, client.ts, ProjectDetailPage.tsx), 1 Config (codeforge.yaml.example)
- API: 5 new REST endpoints for execution plan management
- Tests: 37 new test functions (25 domain + 12 service), all Go tests pass

#### 5B. Orchestrator Agent — Meta-Agent (COMPLETED)

- [x] (2026-02-17) LiteLLM ChatCompletion client: types + method in `internal/adapter/litellm/client.go`
- [x] (2026-02-17) Decomposition domain types: `internal/domain/plan/decompose.go` (OrchestratorMode manual/semi_auto/full_auto, AgentStrategy single/pair/team; DecomposeRequest, DecomposeResult, SubtaskDefinition; Validation: request + result validation, dependency index checks, self-reference detection; StrategyToProtocol mapping helper)
- [x] (2026-02-17) Config extension: Mode, DecomposeModel, DecomposeMaxTokens in `config.Orchestrator` (ENV overrides: CODEFORGE_ORCH_MODE, CODEFORGE_ORCH_DECOMPOSE_MODEL, CODEFORGE_ORCH_DECOMPOSE_MAX_TOKENS; Defaults: semi_auto, openai/gpt-4o-mini, 4096)
- [x] (2026-02-17) MetaAgentService: `internal/service/meta_agent.go` (DecomposeFeature: validate → load project/agents/tasks → build prompt → call LLM → parse JSON → create tasks → select agents → create plan → optionally auto-start; buildDecomposePrompt: system + user prompt with feature, context, agent list, existing tasks; selectAgent: hint-based matching backend exact → name substring → idle fallback; extractJSON: strips markdown code fences, finds JSON boundaries; Auto-start: full_auto mode or req.AutoStart triggers StartPlan)
- [x] (2026-02-17) REST API: `POST /api/v1/projects/{id}/decompose` handler + route
- [x] (2026-02-17) Composition root: MetaAgentService wired in `cmd/codeforge/main.go`
- [x] (2026-02-17) Frontend: DecomposeRequest type, api.plans.decompose method, PlanPanel decompose UI
- [x] (2026-02-17) Tests: 4 litellm client tests, 5 domain tests (19 cases), 9 meta-agent service tests

#### Phase 5B Key Deliverables
- **New files:** 3 Go (decompose.go, decompose_test.go, meta_agent.go, meta_agent_test.go), extended 1 test file (client_test.go)
- Modified files: 6 Go (client.go, config.go, loader.go, handlers.go, routes.go, main.go), 3 Frontend (types.ts, client.ts, PlanPanel.tsx), 1 Config (codeforge.yaml.example)
- API: 1 new REST endpoint (POST /projects/{id}/decompose)
- Tests: 18 new test functions (4 litellm + 5 domain + 9 service), all Go tests pass

#### 5C. Agent Teams + Context-Optimized Planning (COMPLETED)

- [x] (2026-02-17) Agent Team domain model: `internal/domain/agent/team.go` (TeamRole coder/reviewer/tester/documenter/planner, TeamStatus initializing/active/completed/failed; Team, TeamMember, CreateTeamRequest, CreateMemberRequest structs; Validation: name, project_id, at least 1 member, valid roles, no duplicate agents; 8 domain tests in `internal/domain/agent/team_test.go`)
- [x] (2026-02-17) Database: migration 008_create_agent_teams.sql (agent_teams + team_members tables, FK to projects and agents, unique constraint on team_id + agent_id)
- [x] (2026-02-17) Store interface: 5 new methods (CreateTeam, GetTeam, ListTeamsByProject, UpdateTeamStatus, DeleteTeam)
- [x] (2026-02-17) Postgres adapter: transactional CreateTeam, member batch-loading in GetTeam/ListTeamsByProject
- [x] (2026-02-17) Config extension: `max_team_size` in Orchestrator (default: 5, ENV: CODEFORGE_ORCH_MAX_TEAM_SIZE)
- [x] (2026-02-17) PlanFeatureRequest domain type: extends DecomposeRequest with auto_team option
- [x] (2026-02-17) PoolManagerService: `internal/service/pool_manager.go` (CreateTeam validates agents exist, idle, belong to project; AssembleTeamForStrategy auto role assignment; CleanupTeam mark status, release agents to idle; GetTeam, ListTeams, DeleteTeam; 8 service tests in `internal/service/pool_manager_test.go`)
- [x] (2026-02-17) TaskPlannerService: `internal/service/task_planner.go` (PlanFeature enriches context with file listing, delegates to MetaAgentService, optionally assembles team; gatherProjectContext workspace file tree, skip hidden, cap at 100 entries; estimateComplexity heuristic 1=single, 2=pair, 3+=team; 3 service tests in `internal/service/task_planner_test.go`)
- [x] (2026-02-17) REST API: 5 new endpoints (POST /projects/{id}/teams create team, GET /projects/{id}/teams list teams, GET /teams/{id} get team, DELETE /teams/{id} delete team, POST /projects/{id}/plan-feature context-optimized planning)
- [x] (2026-02-17) Composition root: PoolManagerService + TaskPlannerService wired in main.go
- [x] (2026-02-17) Frontend: team types (AgentTeam, TeamMember, CreateTeamRequest), PlanFeatureRequest (API client: teams namespace list/get/create/delete + plans.planFeature method)

#### Phase 5C Key Deliverables
- **New files:** 7 Go (team.go, team_test.go, 008 migration, pool_manager.go, pool_manager_test.go, task_planner.go, task_planner_test.go)
- Modified files: 9 Go (store.go, postgres/store.go, decompose.go, config.go, loader.go, handlers.go, routes.go, main.go, 4 test files for mock updates), 2 Frontend (types.ts, client.ts), 1 Config (codeforge.yaml.example)
- API: 5 new REST endpoints (team CRUD + plan-feature)
- Tests: 19 new test functions (8 domain + 8 pool manager + 3 task planner), all Go tests pass

#### 5D. Context Optimizer — ContextPack, SharedContext, Token Budget (COMPLETED)

- [x] (2026-02-17) ContextPack domain model: `internal/domain/context/pack.go` (ContextPack, ContextEntry, EntryKind file/snippet/summary/shared; Validation: TaskID + ProjectID required, budget > 0, entries non-empty, valid kind; EstimateTokens heuristic: `len(s) / 4` at 1 token per 4 chars; 8 domain tests in `internal/domain/context/pack_test.go`)
- [x] (2026-02-17) SharedContext domain model: `internal/domain/context/shared.go` (SharedContext, SharedContextItem, AddSharedItemRequest; Versioned with optimistic locking, per-team unique shared context; 6 domain tests in `internal/domain/context/shared_test.go`)
- [x] (2026-02-17) Database: migration 009_create_context_packs.sql (4 tables: context_packs, context_entries, shared_contexts, shared_context_items; Indexes, unique constraints, CASCADE deletes, updated_at trigger)
- [x] (2026-02-17) Config extension: `default_context_budget` (4096), `prompt_reserve` (1024) (ENV overrides: CODEFORGE_ORCH_CONTEXT_BUDGET, CODEFORGE_ORCH_PROMPT_RESERVE)
- [x] (2026-02-17) Store interface: 9 new methods (ContextPack CRUD + SharedContext CRUD)
- [x] (2026-02-17) Postgres adapter: transactional CreateContextPack, upsert AddSharedContextItem with version bump
- [x] (2026-02-17) ContextOptimizerService: `internal/service/context_optimizer.go` (BuildContextPack: scan workspace → keyword scoring → shared context injection → budget packing → persist; ScoreFileRelevance: keyword-matching scorer 0-100; GetPackByTask: retrieve existing pack for HTTP API; 6 service tests in `internal/service/context_optimizer_test.go`)
- [x] (2026-02-17) SharedContextService: `internal/service/shared_context.go` (InitForTeam, AddItem with NATS notification, Get; 4 service tests in `internal/service/shared_context_test.go`)
- [x] (2026-02-17) NATS: 2 new subjects (context.packed, context.shared.updated) (3 new payload types: ContextEntryPayload, ContextPackedPayload, SharedContextUpdatedPayload; RunStartPayload extended with Context field for pre-packed context delivery)
- [x] (2026-02-17) RuntimeService integration: context pack auto-built before run start (toContextEntryPayloads helper, non-fatal error handling where run proceeds without context on failure)
- [x] (2026-02-17) Python worker: ContextEntry model, RunStartMessage.context field (Consumer enriches prompt with context section --- Relevant Context ---; 2 new Python tests with context + without context)
- [x] (2026-02-17) REST API: 4 new endpoints (GET /tasks/{id}/context get context pack, POST /tasks/{id}/context build context pack, GET /teams/{id}/shared-context get team shared context, POST /teams/{id}/shared-context add shared context item)
- [x] (2026-02-17) Composition root: ContextOptimizerService + SharedContextService wired in main.go (RuntimeService.SetContextOptimizer injected for automatic context packing)
- [x] (2026-02-17) Frontend: context types (ContextPack, ContextEntry, SharedContext, SharedContextItem) (API client: tasks.context/buildContext + teams.sharedContext/addSharedItem)

#### Phase 5D Key Deliverables
- **New files:** 9 Go (pack.go, pack_test.go, shared.go, shared_test.go, 009 migration, context_optimizer.go, context_optimizer_test.go, shared_context.go, shared_context_test.go)
- Modified files: 11 Go (store.go, postgres/store.go, config.go, loader.go, queue.go, schemas.go, runtime.go, handlers.go, routes.go, main.go, 3 test files), 2 Python (models.py, consumer.py), 1 Python test (test_consumer.py), 2 Frontend (types.ts, client.ts)
- API: 4 new REST endpoints (task context + shared context)
- Protocol: Context-enriched RunStartPayload, NATS context subjects
- Tests: 26+ new test functions (14 domain + 10 service + 2 Python), all passing

#### 5E. Integration Fixes, WS Events, Modes System (COMPLETED)

- [x] (2026-02-17) Fix TeamID propagation: added `TeamID` field to `ExecutionPlan`, `Run`, `StartRequest` (Migration 010: `team_id` column on `execution_plans` and `runs` tables + `output` column on `runs`; Orchestrator passes `p.TeamID` through to `StartRequest` in `startStep()`; Runtime passes `req.TeamID` to `BuildContextPack`; Run.Output field for capturing textual output of completed runs separate from Error)
- [x] (2026-02-17) Auto-initialize SharedContext on team creation (PoolManagerService.SetSharedContext injection, calls InitForTeam after store.CreateTeam)
- [x] (2026-02-17) Auto-populate SharedContext from run outputs (OrchestratorService.SetSharedContext injection; HandleRunCompleted stores run output as shared context item `step_output:{stepID}`)
- [x] (2026-02-17) WS events for teams and shared context (2 new event types: `team.status`, `shared.updated`; TeamStatusEvent + SharedContextUpdateEvent structs in ws/events.go; PoolManagerService broadcasts team.status on CreateTeam; SharedContextService broadcasts shared.updated on AddItem nil-safe)
- [x] (2026-02-17) Modes System — domain model, presets, service, HTTP endpoints (Domain: Mode struct with Validate(), 8 built-in presets architect, coder, reviewer, debugger, tester, documenter, refactorer, security; ModeService: List, Get, Register with custom modes and built-in protection; REST API: GET /modes, GET /modes/{id}, POST /modes; Composition root: ModeService wired in main.go)
- [x] (2026-02-17) Mock store updates + test fixes (CompleteRun signature updated across all mock stores with added `output` param; SharedContextService nil-safe hub broadcasting; handlers_test.go newTestRouter includes ModeService)
- [x] (2026-02-17) Frontend types + API client (Run type: added team_id, output fields; ExecutionPlan type: added team_id field; New types: Mode, CreateModeRequest, TeamStatusEvent, SharedContextUpdateEvent; API client: modes namespace list/get/create)

#### Phase 5E Key Deliverables
- **New files:** 3 Go (mode.go domain, presets.go, mode.go service), 2 Go tests (mode_test.go domain, mode_test.go service), 1 migration (010_add_team_id_fields.sql)
- Modified files: 12 Go (plan.go, run.go, store interface, postgres/store.go, orchestrator.go, runtime.go, pool_manager.go, shared_context.go, events.go, handlers.go, routes.go, main.go), 4 Go test files (project_test.go, runtime_test.go, handlers_test.go, mode_test.go), 2 Frontend (types.ts, client.ts)
- API: 3 new REST endpoints (modes CRUD)
- Events: 2 new WS event types (team.status, shared.updated)
- Tests: 16+ new test functions (8 mode domain + 8 mode service), all Go tests pass
- Lint: golangci-lint 0 issues, frontend lint + build clean

### Infrastructure Features (3B, 3C, 3E, 3F, 4A/4C, 4B) (COMPLETED)

> Implemented between Phase 6B and 6C as foundational capabilities for the agent execution pipeline.

- [x] (2026-02-17) 3B: Async Logging — Go `AsyncHandler` (slog wrapper, 10k buffer, 4 workers, drop policy) + Python `QueueHandler`/`QueueListener`
- [x] (2026-02-17) 3C: Idempotency Keys — HTTP middleware for POST/PUT/DELETE dedup via NATS JetStream KV (24h TTL)
- [x] (2026-02-17) 3E: Cache Layer — Tiered L1 (ristretto in-process) + L2 (NATS KV) with backfill, port/adapter pattern
- [x] (2026-02-17) 4A/4C: Checkpoint System — Shadow Git commits per file-modifying tool call, rewind on quality gate failure, cleanup on finalize
- [x] (2026-02-17) 4B: Docker Sandbox — SandboxService with Docker CLI (create/start/exec/stop/remove), resource flags, runtime lifecycle integration
- [x] (2026-02-17) 3F: Resource Limits — Shared `resource.Limits` type with `Merge`/`Cap`, agent + policy fields, JSONB storage, migration 012

#### Key Deliverables
- **New files (14):** async.go, async_test.go, idempotency.go, idempotency_test.go, cache port + 3 adapters, cache_test.go, checkpoint.go, checkpoint_test.go, sandbox.go, sandbox_test.go, limits.go, limits_test.go, migration 012
- Modified files (14): logger.go, config.go, loader.go, nats.go, runtime.go, run.go, validate.go, agent.go, policy.go, store.go (port + postgres), handlers.go, agent service, main.go, logger.py, consumer.py
- New dependency: `github.com/dgraph-io/ristretto/v2`
- Tests: 36+ new test functions (Go + Python), all passing

### Phase 6: Code-RAG (Context Engine for Large Codebases) (COMPLETED)

#### 6A. Repo Map — tree-sitter Based Code Intelligence (COMPLETED)

- [x] (2026-02-17) Python Worker: RepoMapGenerator with tree-sitter parsing (tree-sitter + tree-sitter-language-pack for 16+ language support; symbol extraction: functions, classes, methods, types, interfaces; file ranking via networkx PageRank import graph analysis; compact map output: files + key symbols with token budget; NATS integration: repomap.generate / repomap.result subjects; configurable token budget, max files, tag format)
- [x] (2026-02-17) Go Backend: RepoMap domain, store, service, HTTP, WS (Domain model: RepoMap entity with validation; PostgreSQL storage: migration 011_create_repo_maps.sql; RepoMapService: Generate via NATS to Python, Get, HandleResult; REST API: GET/POST /projects/{id}/repomap; WS event: repomap.status generating/ready/failed)
- [x] (2026-02-17) Frontend: RepoMapPanel component (Stats display: file count, symbol count, token count; language tags, version info, collapsible map text; generate/regenerate button with loading state; integrated into ProjectDetailPage between Git and Agents sections; WS event handler for repomap.status)

#### Phase 6A Key Deliverables
- **Python:** RepoMapGenerator, tree-sitter parsing, NATS consumer extension
- Go: Domain model, PostgreSQL store, service, 2 REST endpoints, WS events
- Frontend: RepoMapPanel.tsx, types (RepoMap, RepoMapStatusEvent), API client (repomap namespace)
- Dependencies: tree-sitter ^0.24, tree-sitter-language-pack ^0.13, networkx ^3.4

#### 6B. Hybrid Retrieval — BM25 + Semantic Search (COMPLETED)

- [x] (2026-02-17) Python Worker: HybridRetriever with BM25S + LiteLLM embeddings (CodeChunker: AST-aware code splitting via tree-sitter at definition boundaries; HybridRetriever: BM25S keyword indexing + semantic embeddings via LiteLLM; Reciprocal Rank Fusion RRF k=60 combining BM25 and semantic rankings; in-memory per-project indexes ProjectIndex dataclass; shared constants extracted to `_tree_sitter_common.py` for reuse from 6A; consumer: 4 NATS subjects, 4 handler methods for index + search)
- [x] (2026-02-17) Go Backend: RetrievalService with synchronous search waiter (RetrievalService: RequestIndex, HandleIndexResult, SearchSync, HandleSearchResult; channel-based waiter pattern with crypto/rand correlation IDs at 30s timeout; NATS: 4 subjects retrieval.index.request/result, retrieval.search.request/result; REST API: POST /projects/{id}/index, GET /projects/{id}/index, POST /projects/{id}/search; WS event: retrieval.status building/ready/error; context optimizer: auto-injects hybrid results as EntryHybrid with priority scoring; config: 4 new fields DefaultEmbeddingModel, RetrievalTopK, BM25Weight, SemanticWeight)
- [x] (2026-02-17) Frontend: RetrievalPanel component (Index status display with stats: file count, chunk count, embedding model, status badge; build Index button disabled while building; search bar with results list showing filepath:lines, symbol name, language badge, score; integrated into ProjectDetailPage with retrieval.status WS handler)

#### Phase 6B Key Deliverables
- **New files:** 5 (retrieval.py, _tree_sitter_common.py, test_retrieval.py, retrieval.go, retrieval_test.go, RetrievalPanel.tsx)
- Modified files: 19 (consumer.py, models.py, repomap.py, pyproject.toml, queue.go, schemas.go, validator.go, nats.go, pack.go, config.go, context_optimizer.go, events.go, handlers.go, routes.go, main.go, handlers_test.go, types.ts, client.ts, ProjectDetailPage.tsx)
- API: 3 new REST endpoints (index CRUD + search)
- Dependencies: bm25s ^0.2, numpy ^2.0
- Tests: 11 Python tests + 5 Go service tests + 3 Go handler tests, all passing

#### 6C. Retrieval Sub-Agent — LLM-Guided Multi-Query Search (COMPLETED)

- [x] (2026-02-18) Python Worker: RetrievalSubAgent with LLM query expansion + parallel search + reranking (`RetrievalSubAgent` class: composes `HybridRetriever` + `LiteLLMClient`; query expansion via LLM task prompt → N focused search queries; parallel hybrid search across all expanded queries `asyncio.gather`; deduplication by filepath+start_line, keeps highest score; LLM re-ranking with score-based fallback on failure; `SubAgentSearchRequest` / `SubAgentSearchResult` Pydantic models; consumer extended with `retrieval.subagent.request` NATS subscription + handler)
- [x] (2026-02-18) Go Backend: RetrievalService sub-agent extensions (`SubAgentSearchSync()`: correlation-ID-based sync request/response at 60s timeout; `HandleSubAgentSearchResult()`: delivers result to waiting channel; NATS subjects: `retrieval.subagent.request`, `retrieval.subagent.result`; payload types: `SubAgentSearchRequestPayload`, `SubAgentSearchResultPayload`; config: `SubAgentModel`, `SubAgentMaxQueries`, `SubAgentRerank` + ENV overrides; context optimizer: `fetchRetrievalEntries()` sub-agent first, single-shot fallback; HTTP handler: `POST /projects/{id}/search/agent`)
- [x] (2026-02-18) Frontend: Agent search mode in RetrievalPanel (Standard/Agent toggle button next to search input; agent mode calls `api.retrieval.agentSearch()` endpoint; expanded queries displayed as purple tags; total candidates count badge; types: `SubAgentSearchRequest`, `SubAgentSearchResult`)

#### Phase 6C Key Deliverables
- **New files:** 1 (test_retrieval_subagent.py)
- Modified files: 13 (models.py, retrieval.py, consumer.py, queue.go, schemas.go, validator.go, config.go, loader.go, retrieval.go, context_optimizer.go, handlers.go, routes.go, retrieval_test.go, types.ts, client.ts, RetrievalPanel.tsx)
- API: 1 new REST endpoint (POST /projects/{id}/search/agent)
- Config: 3 new env vars (CODEFORGE_ORCH_SUBAGENT_MODEL, CODEFORGE_ORCH_SUBAGENT_MAX_QUERIES, CODEFORGE_ORCH_SUBAGENT_RERANK)
- Tests: 8 Python tests + 3 Go service tests, all passing

#### 6C Code Review Refinements (COMPLETED)

- [x] (2026-02-18) 16 code review improvements across architecture, quality, tests, performance
- Architecture: Generic `syncWaiter[T]` (DRY), health tracking with 30s cooldown fast-fail, shared retrieval deadline, check-before-build guard, parallel workspace scan + retrieval
- Code Quality: Unified `SearchResult` → `RetrievalSearchHit`, DRY `_publish_error_result()` consumer helper, defense-in-depth validation (Pydantic + Go handler bounds)
- Performance: Pre-built rank dict for O(1) BM25 lookup, `per_query_k = top_k` fix, percentile-based priority normalization (60-85 range)
- Tests: 5 new tests (error-in-payload Go, parallel-all-fail Python, Pydantic validator bounds, consumer error publish)
- All 77 Python tests + all Go tests passing, golangci-lint 0 issues

#### 6D. GraphRAG — Structural Code Graph Intelligence (COMPLETED)

- [x] (2026-02-18) PostgreSQL adjacency-list graph with no Neo4j, using single-DB architecture (Migration 016: `graph_nodes`, `graph_edges`, `graph_metadata` tables; nodes: function/class/method/module definitions via tree-sitter; edges: import edges for Python/Go/TS/JS, call edges via name-matching heuristic)
- [x] (2026-02-18) Python: CodeGraphBuilder + GraphSearcher (`workers/codeforge/graphrag.py`) (Build pipeline: walk files → tree-sitter parse → extract definitions + imports + calls → batch upsert to PostgreSQL; BFS search with hop-decay scoring at default decay 0.7: hop 0 = 1.0, hop 1 = 0.7, hop 2 = 0.49; bidirectional traversal outgoing + incoming edges, edge path tracking for explainability)
- [x] (2026-02-18) Python consumer: 2 new NATS handlers (`graph.build.request`, `graph.search.request`)
- [x] (2026-02-18) Go: GraphService following RetrievalService pattern (syncWaiter, health tracking, WS broadcasts)
- [x] (2026-02-18) Go: 4 NATS subjects + 5 payload types, 4 config fields (GraphEnabled, GraphMaxHops, GraphTopK, GraphHopDecay)
- [x] (2026-02-18) Go: Context optimizer `fetchGraphEntries()` uses retrieval hits as seed symbols (Priority: 70 - distance * 10 → hop 0 = 70, hop 1 = 60, hop 2 = 50)
- [x] (2026-02-18) Go: 3 HTTP endpoints, EntryGraph domain kind, WS GraphStatusEvent
- [x] (2026-02-18) Frontend: Graph types, API client (graph namespace), RetrievalPanel graph section

#### Phase 6D Key Deliverables
- **New files:** 4 (016_graph_nodes_edges.sql, graphrag.py, test_graphrag.py, graph.go)
- Modified files: 14 (models.py, consumer.py, queue.go, schemas.go, config.go, loader.go, pack.go, context_optimizer.go, handlers.go, routes.go, events.go, main.go, types.ts, client.ts)
- API: 3 new REST endpoints (POST /graph/build, GET /graph/status, POST /graph/search)
- Config: 4 new env vars (CODEFORGE_ORCH_GRAPH_ENABLED, _MAX_HOPS, _TOP_K, _HOP_DECAY)
- Tests: 19 Python tests, all passing; Go compilation + all existing tests passing
- Linting: golangci-lint 0 issues, ruff 0 issues, ESLint 0 issues, pre-commit all hooks pass

### Phase 7: Cost & Token Transparency (COMPLETED)

> Priority: "It must always be very obvious what costs/tokens CodeForge causes with the LLMs."

- [x] (2026-02-18) Feature 1: Real Cost Calculation — Python workers extract cost from LiteLLM response header + fallback pricing table for local models
- [x] (2026-02-18) Feature 2: Token Persistence — `tokens_in`, `tokens_out`, `model` columns on runs table, Python token accumulators, Go/Python NATS payload updates
- [x] (2026-02-18) Feature 3: Cost Aggregation API — 5 new REST endpoints for cost breakdown by project, model, and time
- [x] (2026-02-18) Feature 4: WS Budget Alerts — `BudgetAlertEvent` at 80%/90% of MaxCost with dedup, token fields on `RunStatusEvent`
- [x] (2026-02-18) Feature 5: Frontend Dashboard — `/costs` page with global totals + project breakdown; per-project cost section with model breakdown, daily bars, recent runs; token/model display in RunPanel; budget alert banner

#### Phase 7 Key Deliverables

- **New files:** 4 (cost domain, cost service, pricing table YAML, pricing module, cost dashboard)
- Modified files: ~20 (across Go, Python, TypeScript)
- New REST endpoints: 5 cost aggregation endpoints
- New WS event: `run.budget_alert`
- Tests: 7 Python pricing tests, 3 Go budget alert tests, all existing tests passing
- Migration: 015_add_run_tokens.sql

### Production Readiness: Worker Pools, Migration Rollback, Rate Limiter Hardening, Backup/DR (COMPLETED)

- [x] (2026-02-18) Git Worker Pool — `internal/git/pool.go` using `golang.org/x/sync/semaphore`, nil-safe Run(), injected into gitlocal.Provider, CheckpointService, DeliverService; config `git.max_concurrent` (default 5); 4 tests
- [x] (2026-02-18) Migration Rollback Verification — `RollbackMigrations()` + `MigrationVersion()` in postgres.go; integration test `TestMigrationUpDown` (up 15, down 15, re-up 15); `./scripts/test.sh migrations` sub-command
- [x] (2026-02-18) Rate Limiter Hardening — `StartCleanup()` background goroutine for stale bucket eviction; `Len()` for metrics; config `rate.cleanup_interval` (5m), `rate.max_idle_time` (10m); cleanup + len + timing tests; 2 benchmarks
- [x] (2026-02-18) Backup & Disaster Recovery — `scripts/backup-postgres.sh` (pg_dump custom format, retention cleanup), `scripts/restore-postgres.sh` (drop-recreate, latest mode); Docker Compose WAL config (wal_level=replica, archive_mode=on); documented in dev-setup.md

#### Key Deliverables
- **New files (6):** pool.go, pool_test.go, migration_test.go, backup-postgres.sh, restore-postgres.sh
- Modified files (~14): config.go, loader.go, provider.go, checkpoint.go, deliver.go, main.go, ratelimit.go, ratelimit_test.go, postgres.go, test.sh, docker-compose.yml, .gitignore, codeforge.yaml.example, dev-setup.md
- Tests: 4 pool tests + 3 ratelimit tests + 2 benchmarks + 1 integration test

### Pre-Commit & Linting Hardening (COMPLETED)

- [x] (2026-02-18) Security analysis, anti-pattern detection, import sorting across all three languages
- Python (pyproject.toml): Added 10 ruff rule groups — S (bandit security), C4 (unnecessary comprehensions), C90 (mccabe complexity, threshold 12), PERF (performance anti-patterns), PIE (misc anti-patterns), RET (return issues), FURB (modernization), LOG (logging best practices), T20 (print detection), PT (pytest style). Added RET504 to ignore, per-file-ignores for tests (S101, PT011, S108). Fixed C901 noqa, PERF401 list.extend, FURB110 ternary.
- Go (.golangci.yml): Added 8 linters — gosec (security), bodyclose (HTTP response body), noctx (context-less HTTP), errorlint (error wrapping), revive (18 curated rules), fatcontext (loop context leak), dupword (comment typos), durationcheck (duration multiplication). Settings: gosec excludes G404/G115, errorlint errorf-only, file-specific exclusions for loader.go (G304) and deliver.go (G306). Test exclusions for noctx, gosec G306/G204/G301, bodyclose. Fixed permissions (0o644→0o600, 0o755→0o750), added nolint annotations.
- TypeScript (frontend/eslint.config.js): Replaced `recommended` with `strict` + `stylistic` configs from typescript-eslint. Added `eslint-plugin-simple-import-sort` for import/export ordering. Fixed void→undefined in generic types, replaced non-null assertions with optional chaining and nullish coalescing.
- Pre-commit (.pre-commit-config.yaml): Bumped ruff-pre-commit v0.11.12→v0.15.1. Fixed ESLint hook to use local binary (`frontend/node_modules/.bin/eslint`) with explicit config path and `^frontend/src/` file filter.
- 31 files changed, all 15 pre-commit hooks pass, 89 Python tests pass, all Go tests pass

### Documentation Debt (COMPLETED)

- [x] (2026-02-18) ADR-003: Config Hierarchy — three-tier (defaults < YAML < env vars), typed Config struct, validation
- [x] (2026-02-18) ADR-004: Async Logging — buffered channel (10K) + worker pool (4), non-blocking drops, graceful drain
- [x] (2026-02-18) ADR-005: Docker-Native Logging — json-file driver with rotation, no external monitoring stack, structured JSON + jq
- [x] (2026-02-18) ADR-006: Agent Execution Approach C — Go control plane (state/policies/sessions) + Python runtime (LLM/tools/loop)
- [x] (2026-02-18) ADR-007: Policy Layer — first-match-wins permission rules, 4 presets, quality gates, termination conditions
- [x] (2026-02-18) architecture.md — added Infrastructure Patterns section (reliability, performance, agent execution, observability)
- [x] (2026-02-18) dev-setup.md — added Logging section (log access, helper script, log levels, request ID, log rotation)
- [x] (2026-02-18) CLAUDE.md — added ADR index, Infrastructure Principles section (config, async, logging, policy, approach C, resilience)

### Phase 8: Roadmap Foundation, Event Trajectory, Docker Production (COMPLETED)

#### 8A. Roadmap/Feature-Map Foundation — Pillar 2 (COMPLETED)

- [x] (2026-02-18) Domain models: Roadmap, Milestone, Feature with statuses, validation, optimistic locking (18 domain tests in `internal/domain/roadmap/roadmap_test.go`)
- [x] (2026-02-18) Migration 017: 3 tables (roadmaps, milestones, features) with indexes, triggers, up/down
- [x] (2026-02-18) Port interfaces: `specprovider/` (SpecProvider + Registry), `pmprovider/` (PMProvider + Registry)
- [x] (2026-02-18) Store interface: 16 new methods + Postgres adapter implementation
- [x] (2026-02-18) RoadmapService: CRUD, AutoDetect (file markers), AIView (json/yaml/markdown), WS broadcast
- [x] (2026-02-18) REST API: 12 roadmap endpoints (CRUD + AI view + detect + milestones + features)
- [x] (2026-02-18) WS event: `roadmap.status` with RoadmapStatusEvent
- [x] (2026-02-18) Frontend: RoadmapPanel.tsx (milestone/feature tree, forms, auto-detect, AI view)
- [x] (2026-02-18) main.go wiring: RoadmapService creation + Handlers integration

#### 8B. Event Trajectory API + Frontend (COMPLETED)

- [x] (2026-02-18) Event store extension: TrajectoryFilter, TrajectoryPage, TrajectorySummary (Cursor-paginated LoadTrajectory with type/time filtering; TrajectoryStats via SQL aggregates)
- [x] (2026-02-18) Postgres implementation: dynamic WHERE builder, cursor pagination, aggregate stats
- [x] (2026-02-18) REST API: 2 endpoints (GET /runs/{id}/trajectory, GET /runs/{id}/trajectory/export)
- [x] (2026-02-18) Frontend: TrajectoryPanel.tsx (timeline, filters, stats, export)

#### 8C. Docker/CI for Production (COMPLETED)

- [x] (2026-02-18) Dockerfile (Go Core): multi-stage golang:1.24-alpine → alpine:3.21
- [x] (2026-02-18) Dockerfile.worker (Python): python:3.12-slim, poetry, non-root user
- [x] (2026-02-18) Dockerfile.frontend: node:22-alpine → nginx:alpine with SPA routing
- [x] (2026-02-18) frontend/nginx.conf: try_files, proxy_pass for /api/ and /ws
- [x] (2026-02-18) docker-compose.prod.yml: 6 services with health checks, named volumes, tuned PostgreSQL
- [x] (2026-02-18) .github/workflows/docker-build.yml: 3 parallel jobs → ghcr.io
- [x] (2026-02-18) .dockerignore: excludes .venv, node_modules, .git, data/, __pycache__

#### Phase 8 Key Deliverables
- **New files (19):** roadmap domain (3), migration 017, specprovider (2), pmprovider (2), roadmap service + tests, RoadmapPanel.tsx, TrajectoryPanel.tsx, Dockerfile (3), nginx.conf, docker-compose.prod.yml, docker-build.yml, .dockerignore
- Modified files (16): store.go (port + postgres), eventstore (port + postgres), handlers.go, routes.go, events.go, main.go, types.ts, client.ts, ProjectDetailPage.tsx, RunPanel.tsx, 4 test files (mock updates)
- New REST endpoints: 14 (12 roadmap + 2 trajectory)
- New WS event: `roadmap.status`
- Store methods: 16 roadmap + 2 trajectory
- Verification: go build, golangci-lint 0 issues, go test -race all pass, ESLint clean, npm run build clean, pre-commit 15/15 hooks pass

### Phase 9A: Spec Provider Adapters + Enhanced AutoDetect + Spec Import (COMPLETED)

> Wires real spec/PM adapters into the existing provider framework so AutoDetect and spec import work through the registry system.

- [x] (2026-02-18) OpenSpec Adapter (`internal/adapter/openspec/`) — `specprovider.Provider` for `openspec/` directory (Detect, ListSpecs with YAML title extraction, ReadSpec with path traversal protection; self-registration via `init()`, 8 unit tests)
- [x] (2026-02-18) Markdown Spec Adapter (`internal/adapter/markdownspec/`) — `specprovider.Provider` for ROADMAP.md (Detect `ROADMAP.md`/`roadmap.md`, ListSpecs, ReadSpec; self-registration via `init()`, 7 unit tests)
- [x] (2026-02-18) GitHub Issues PM Adapter (`internal/adapter/githubpm/`) — `pmprovider.Provider` via `gh` CLI (ListItems, GetItem with `owner/repo` validation; swappable `execCommand` for testing, 9 unit tests)
- [x] (2026-02-18) Enhanced AutoDetect — two-phase: spec providers first, hardcoded fileMarkers fallback (format alias mapping to prevent duplication between providers and fileMarkers)
- [x] (2026-02-18) ImportSpecs — discover specs via providers, auto-create roadmap, milestone per format, features per spec
- [x] (2026-02-18) ImportPMItems — find PM provider by name, list items, create milestone + features
- [x] (2026-02-18) 4 new REST endpoints: import specs, import PM items, list spec providers, list PM providers
- [x] (2026-02-18) Provider wiring — blank imports in `providers.go`, registry instantiation in `main.go`
- [x] (2026-02-18) Frontend — Import Specs button, Import from PM form (provider dropdown + project ref), import result display
- [x] (2026-02-18) Frontend types + API client — ImportResult, PMImportRequest, ProviderInfo types; importSpecs, importPMItems, providers.spec/pm methods

#### Phase 9A Key Deliverables
- **New files (9):** openspec/ (provider.go, register.go, provider_test.go), markdownspec/ (provider.go, register.go, provider_test.go), githubpm/ (provider.go, register.go, provider_test.go)
- Modified files (10): roadmap.go (domain), roadmap.go (service), handlers.go, routes.go, providers.go, main.go, types.ts, client.ts, RoadmapPanel.tsx
- New REST endpoints: 4 (import specs, import PM, list spec providers, list PM providers)
- Tests: 24 new adapter tests (8 + 7 + 9), all Go tests pass
- Verification: go build, golangci-lint 0 issues, go test -race all pass, ESLint clean, npm run build clean, pre-commit 15/15 hooks pass

### E2E Test Infrastructure — Playwright (COMPLETED)

- [x] (2026-02-18) Playwright-based E2E browser tests for the frontend
- `frontend/playwright.config.ts` — single Chromium project, workers: 1, baseURL localhost:3000
- `frontend/e2e/fixtures.ts` — shared API helper (createProject, deleteProject, deleteAllProjects) with auto-cleanup
- 5 test files, 17 tests total: `health.spec.ts` (3) frontend loads, API ok status, backend health endpoint; `navigation.spec.ts` (4) sidebar links Dashboard, Costs, Models, back; `projects.spec.ts` (5) empty state, create via form, detail page, delete, validation; `costs.spec.ts` (2) heading, empty state message; `models.spec.ts` (3) heading, Add Model form toggle, LiteLLM status
- Vite proxy fix: `/health` route added to `vite.config.ts`
- `scripts/test.sh e2e` command with port checks (8080 + 3000)
- `@playwright/test` devDependency, tsconfig/eslint updated for `e2e/` directory

### Phase 9B: SVN + Gitea + VCS Webhooks + Bidirectional PM Sync (COMPLETED)

> Adds version control and PM integration adapters, VCS webhook processing, and bidirectional roadmap sync.

- [x] (2026-02-19) SVN Integration (`internal/adapter/svn/`) — gitprovider.Provider for SVN repos (Clone via svn checkout, Pull via svn update, Status via svn status + svn info, ListBranches via svn ls; swappable `execCommand` for testing, self-registration via `init()`, 5 unit tests)
- [x] (2026-02-19) Gitea/Forgejo PM Adapter (`internal/adapter/gitea/`) — pmprovider.Provider via REST API (Full CRUD: ListItems, GetItem, CreateItem, UpdateItem via `{baseURL}/api/v1/repos/{owner}/{repo}/issues`; token-based auth, self-registration via `init()` factory, 6 unit tests with httptest)
- [x] (2026-02-19) VCS Webhooks — GitHub + GitLab push/PR event processing (`internal/middleware/webhook.go`: HMAC-SHA256 signature verification + static token verification; `internal/domain/webhook/event.go`: VCSEvent, VCSPushEvent, VCSPullRequestEvent, VCSCommit types; `internal/service/vcs_webhook.go`: HandleGitHubPush, HandleGitLabPush, HandleGitHubPullRequest; WebSocket broadcast of VCS events EventVCSPush, EventVCSPullRequest; REST endpoints: `POST /webhooks/vcs/github`, `POST /webhooks/vcs/gitlab`; config: `Webhook.GitHubSecret`, `Webhook.GitLabToken` with env overrides; 4 service tests)
- [x] (2026-02-19) Bidirectional PM Sync — pull/push/bidi sync between CodeForge roadmap and PM providers (`internal/domain/roadmap/sync.go`: SyncDirection pull/push/bidi, SyncConfig, SyncResult; `internal/service/sync.go`: SyncService with pullFromPM import items → features and pushToPM export features → items; `internal/port/pmprovider/provider.go`: Added CreateItem/UpdateItem to Provider interface + ErrNotSupported; `internal/adapter/githubpm/provider.go`: Stub CreateItem/UpdateItem returning ErrNotSupported; REST endpoint: `POST /projects/{id}/roadmap/sync`)
- [x] (2026-02-19) Provider wiring — blank imports for svn + gitea in `providers.go`, services wired in `main.go`

#### Phase 9B Key Deliverables
- **New files (12):** svn/ (provider.go, register.go, provider_test.go), gitea/ (provider.go, register.go, provider_test.go), webhook middleware, webhook event types, roadmap sync types, vcs_webhook service + test, sync service
- Modified files (10): pmprovider/provider.go, githubpm/provider.go, ws/events.go, config.go, loader.go, handlers.go, routes.go, providers.go, main.go, gitea/provider_test.go
- New REST endpoints: 3 (2 VCS webhooks + 1 roadmap sync)
- Interface change: pmprovider.Provider gains CreateItem + UpdateItem
- Tests: 15+ new test functions (5 SVN + 6 Gitea + 4 VCS webhook), all Go tests pass
- Verification: go build, golangci-lint 0 issues, go test all pass, pre-commit 15/15 hooks pass

### Phase 9C: PM Webhook Sync + Slack/Discord Notifications (COMPLETED)

> Adds PM webhook event processing and notification adapter infrastructure.

- [x] (2026-02-19) PM Webhook Service (`internal/service/pm_webhook.go`) (HandleGitHubIssueWebhook, HandleGitLabIssueWebhook, HandlePlaneWebhook; JSON payload parsing, normalized PMWebhookEvent output; WebSocket broadcast + SyncService integration for auto-sync on webhook)
- [x] (2026-02-19) PM Webhook Domain (`internal/domain/webhook/webhook.go`) (PMWebhookEvent with Provider, Action, ItemID, ProjectRef; PMWebhookConfig with Provider, Secret, Enabled)
- [x] (2026-02-19) Notifier Port (`internal/port/notifier/`) (Notifier interface: Name(), Capabilities(), Send(); standard registry pattern: Factory, Register, New, Available)
- [x] (2026-02-19) Slack Adapter (`internal/adapter/slack/`) (HTTP POST with Block Kit JSON header + section + context blocks; level-based prefix tags, self-registration via init(), 5 tests)
- [x] (2026-02-19) Discord Adapter (`internal/adapter/discord/`) (HTTP POST with embed JSON, color-coded levels green/red/orange/blue; 204 success handling, self-registration via init(), 5 tests)
- [x] (2026-02-19) NotificationService (`internal/service/notification.go`) (Fan-out to all registered notifiers, event filtering, error logging; 4 tests for fan-out, filtering, error continues, count)
- [x] (2026-02-19) REST endpoints: 3 PM webhook routes (`POST /api/v1/webhooks/pm/github` GitHub Issues webhook, `POST /api/v1/webhooks/pm/gitlab` GitLab Issues webhook, `POST /api/v1/webhooks/pm/plane` Plane.so webhook)
- [x] (2026-02-19) Config: Notification struct (SlackWebhookURL, DiscordWebhookURL, EnabledEvents), PlaneSecret
- [x] (2026-02-19) Provider wiring: blank imports for slack + discord in providers.go

#### Phase 9C Key Deliverables
- **New files (11):** notifier port (2), slack adapter (3), discord adapter (3), notification service + test, pm_webhook service, webhook domain
- Modified files (6): config.go, loader.go, providers.go, handlers.go, routes.go, main.go
- New REST endpoints: 3 (PM webhooks for GitHub, GitLab, Plane)
- Tests: 14 new test functions (5 Slack + 5 Discord + 4 notification service), all Go tests pass
- Verification: go build, golangci-lint 0 issues, go test all pass, pre-commit 15/15 hooks pass

### Phase 9D: OpenTelemetry + A2A + AG-UI + Blue-Green (COMPLETED)

> Adds observability, protocol stubs, and deployment infrastructure.

- [x] (2026-02-19) OpenTelemetry — Real TracerProvider + MeterProvider replacing no-op stub (`internal/adapter/otel/setup.go`: OTLP gRPC trace + metric exporters, TraceIDRatioBased sampling; `internal/adapter/otel/middleware.go`: chi HTTP middleware via `otelhttp.NewHandler`; `internal/adapter/otel/spans.go`: StartRunSpan, StartToolCallSpan, StartDeliverySpan helpers; `internal/adapter/otel/metrics.go`: 6 metric instruments runs started/completed/failed, tool calls, duration, cost; config: `OTEL{Enabled, Endpoint, ServiceName, Insecure, SampleRate}` with env overrides)
- [x] (2026-02-19) A2A Protocol Stub — Google Agent-to-Agent protocol (`internal/port/a2a/types.go`: AgentCard, Skill, TaskRequest, TaskResponse types; `internal/port/a2a/agentcard.go`: BuildAgentCard with 2 skills code-task, decompose; `internal/port/a2a/handler.go`: HTTP handlers for `/.well-known/agent.json`, `/a2a/tasks`; 5 handler tests, all passing; config: `A2A.Enabled bool` default false)
- [x] (2026-02-19) AG-UI Protocol Events — CopilotKit Agent-User Interaction (`internal/adapter/ws/agui_events.go`: 8 event types + 8 structs; `frontend/src/api/types.ts`: 8 TypeScript interfaces for AG-UI events; config: `AGUI.Enabled bool` default false)
- [x] (2026-02-19) Blue-Green Deployment — Zero-downtime deployment infrastructure (`traefik/traefik.yaml`: static config with entrypoints, Docker provider, file provider; `docker-compose.blue-green.yml`: overlay with blue/green service pairs + Traefik routing; `scripts/deploy-blue-green.sh`: deployment script detect active, deploy inactive, health-check, switch)
- [x] (2026-02-19) Config + Wiring — OTEL, A2A, AGUI config structs with env overrides in loader.go; OTEL middleware + A2A route mounting in main.go

#### Phase 9D Key Deliverables
- **New files (10):** otel (3: middleware.go, spans.go, metrics.go), a2a (4: types.go, agentcard.go, handler.go, handler_test.go), ws/agui_events.go, traefik/traefik.yaml, docker-compose.blue-green.yml, scripts/deploy-blue-green.sh
- Modified files (6): otel/setup.go (rewritten), config.go, loader.go, main.go, frontend/src/api/types.ts, go.mod + go.sum
- New Go deps: go.opentelemetry.io/otel (v1.40), go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
- Tests: 5 A2A handler tests, all Go tests pass
- Verification: go build, golangci-lint 0 issues, go test all pass, pre-commit 15/15 hooks pass

### Phase 10 — Frontend Foundations (IN PROGRESS)

#### 10C. Authentication & Authorization (COMPLETED)

- [x] (2026-02-19) JWT auth with HS256 (stdlib `crypto/hmac` + `crypto/sha256`, no third-party JWT library) (Access token in JS memory signal 15min, refresh token in httpOnly Secure SameSite=Strict cookie 7d; auth disabled by default `auth.enabled: false`, default admin context injected for backward compatibility)
- [x] (2026-02-19) User domain model: `internal/domain/user/user.go`, `apikey.go` (User struct, Role type admin/editor/viewer, CreateRequest, LoginRequest, LoginResponse, TokenClaims; APIKey struct with `cfk_` prefix, SHA-256 hashed in DB)
- [x] (2026-02-19) Database migration: `022_create_users_api_keys.sql` (`users` with email+tenant_id unique, `refresh_tokens` with token_hash unique, `api_keys` with key_hash unique)
- [x] (2026-02-19) Auth service: `internal/service/auth.go` (~320 lines) (Register with bcrypt, Login, RefreshTokens with rotation, Logout, ValidateAccessToken, ValidateAPIKey; CreateAPIKey, ListAPIKeys, DeleteAPIKey, User CRUD, SeedDefaultAdmin)
- [x] (2026-02-19) Auth + RBAC middleware (`internal/middleware/auth.go`: JWT/API key validation, public path exemption, auth-disabled default admin; `internal/middleware/rbac.go`: `RequireRole(roles...)` middleware factory; `internal/middleware/tenant.go`: now extracts tenant from user claims first, falls back to header; middleware chain: CORS > OTEL > RequestID > Auth > TenantID > Logger > ...)
- [x] (2026-02-19) Store interface + Postgres: 14 new methods (User 6, RefreshToken 4, APIKey 4) (`store_user.go`, `store_refresh_token.go`, `store_api_key.go`)
- [x] (2026-02-19) HTTP handlers: `internal/adapter/http/handlers_auth.go` (11 handlers) (Public: Login, Refresh; Authenticated: Logout, GetCurrentUser, API key CRUD; Admin: User CRUD; CORS updated with X-API-Key, X-Tenant-ID, X-Idempotency-Key, Allow-Credentials)
- [x] (2026-02-19) Config: `Auth` struct with 7 fields, 7 `CODEFORGE_AUTH_*` env overrides
- [x] (2026-02-19) Frontend: AuthProvider, RouteGuard, RoleGate, LoginPage (`AuthProvider.tsx`: SolidJS context, login/logout, auto-refresh scheduling, session restore; `RouteGuard.tsx`: redirect to /login if not authenticated; `RoleGate.tsx`: conditional render by role; `LoginPage.tsx`: email/password form with i18n; `client.ts`: setAccessTokenGetter, Authorization header, credentials: "include", auth/users API groups; `App.tsx`: AuthProvider in provider chain, UserInfo in sidebar, /login route)
- [x] (2026-02-19) i18n: ~27 auth keys in en.ts and de.ts
- [x] (2026-02-19) Tests: domain validation (3), auth service (7), auth middleware (4), RBAC middleware (4)

##### Phase 10C Key Deliverables
- **New files (16):** user.go, apikey.go, 022 migration, store_user.go, store_refresh_token.go, store_api_key.go, auth.go (service), auth.go (middleware), rbac.go, handlers_auth.go, AuthProvider.tsx, RouteGuard.tsx, RoleGate.tsx, LoginPage.tsx, user_test.go, auth_test.go (service), auth_test.go (middleware), rbac_test.go
- Modified files (14): config.go, loader.go, store.go, tenant.go, middleware.go, handlers.go, routes.go, main.go, types.ts, client.ts, App.tsx, index.tsx, en.ts, de.ts
- Verification: `go build` clean, `go test` all pass, `golangci-lint` 0 issues, `vite build` clean (58 modules), `pre-commit` all hooks pass

#### 10B. i18n — Internationalization (COMPLETED)

- [x] (2026-02-19) Custom signal-based i18n context provider (zero external dependencies) (`I18nProvider` with `createContext`/`useContext`, `createSignal` for locale + translations; flat dot-separated keys `TranslationKey = keyof typeof en`, type-safe at compile time; `{{variable}}` interpolation via `String.replaceAll()`; lazy bundle loading via dynamic `import()` with Vite auto code-splits for non-English bundles at German = 19 kB chunk; `localStorage` persistence key: `codeforge-locale`, browser language auto-detection from `navigator.language`; provider nesting: ErrorBoundary > I18nProvider > ThemeProvider > ToastProvider > AppShell)
- [x] (2026-02-19) ~480 translation keys across ~20 component files (English default: `frontend/src/i18n/en.ts` source of truth, all keys; German: `frontend/src/i18n/locales/de.ts` full translation, lazy-loaded; keys organized by namespace: `app.*`, `dashboard.*`, `project.*`, `cost.*`, `model.*`, `agent.*`, `task.*`, `run.*`, `plan.*`, `policy.*`, `repomap.*`, `retrieval.*`, `roadmap.*`, `trajectory.*`, `output.*`, `detail.*`, `common.*`, `theme.*`, `offline.*`, `toast.*`, `cmd.*`)
- [x] (2026-02-19) LocaleSwitcher component in sidebar (cycles EN/DE)
- [x] (2026-02-19) All ~20 component files updated with `t()` calls replacing hardcoded strings (Variable shadowing fix: loop variables renamed from `t` to `task`/`evType` where needed; constant-to-function pattern: translatable arrays DELIVER_MODES, PROTOCOL_OPTIONS, MODES, commands moved inside components as reactive functions)
- [x] (2026-02-19) Pluralization support via `tp()` function using `Intl.PluralRules` (`tp(key, count, params?)` resolves `key_one`, `key_other` etc. based on CLDR plural rules; `Intl.PluralRules` instances cached per locale for performance; 12 plural keys added to en.ts + de.ts for costs.runs, costs.steps, run.steps, trajectory.events/toolCalls/errors; updated CostDashboardPage, RunPanel, TrajectoryPanel to use `tp()` instead of `{count} {t(key)}`)

##### Phase 10B Key Deliverables
- **New files (6):** en.ts (translations), locales/de.ts (German), context.tsx (provider), index.ts (re-exports), LocaleSwitcher.tsx, formatters.ts
- Modified files (20+): App.tsx + 4 shell components + 15 feature components + 3 updated for tp()
- Translation keys: ~492 across 21 namespaces (incl. 12 plural keys)
- Bundle size: German chunk = 20.64 kB (code-split by Vite)
- Verification: Vite build clean (58 modules, 3.42s), no new TypeScript errors introduced

#### 10A. Theme System — Dark/Light Mode Toggle (COMPLETED)

- [x] (2026-02-19) CSS Design Tokens in `frontend/src/index.css` (`:root` and `.dark` blocks with 17 custom properties each; surface colors bg-primary, bg-surface, bg-surface-alt, bg-inset; border colors border, border-subtle, border-input; text colors text-primary, text-secondary, text-tertiary, text-muted; accent colors accent, accent-hover; agent status tokens running, idle, waiting, error, planning; `@custom-variant dark (&:where(.dark, .dark *));` for Tailwind CSS v4 class-based dark mode)
- [x] (2026-02-19) `frontend/src/components/ThemeProvider.tsx` — ThemeProvider + ThemeToggle (`ThemeContext` with `theme()`, `resolved()`, `setTheme()`, `toggle()`; 3-state toggle: light -> dark -> system with sun/moon/gear icons; `localStorage` persistence key: `codeforge-theme`; `prefers-color-scheme` media query with real-time change listener; applies `.dark` class on `<html>` element)
- [x] (2026-02-19) Dark mode variants on all 18 component files (App shell, sidebar, navigation in App.tsx; Dashboard: DashboardPage, ProjectCard; Costs: CostDashboardPage; Models: ModelsPage; Project panels: AgentPanel, TaskPanel, LiveOutput, RunPanel, PlanPanel, PolicyPanel; Project detail: ProjectDetailPage; Context panels: RepoMapPanel, RetrievalPanel; Roadmap: RoadmapPanel; Trajectory: TrajectoryPanel; Utility: Toast)
- Color mapping: bg-white→dark:bg-gray-800, bg-gray-50→dark:bg-gray-900, border-gray-200→dark:border-gray-700, text-gray-900→dark:text-gray-100, status badges with dark:bg-{color}-900/30

#### 10E. Keyboard Shortcuts — Command Palette (COMPLETED)

- [x] (2026-02-19) `frontend/src/components/CommandPalette.tsx` — searchable command palette (Opens with Ctrl+K / Cmd+K toggle, Ctrl+/ shortcut help; navigation commands: Go to Dashboard Ctrl+1, Costs Ctrl+2, Models Ctrl+3; action commands: Toggle Theme, Show Keyboard Shortcuts; fuzzy search filtering with section grouping Navigation, Actions, Theme; arrow key navigation, Enter to select, Escape to close; platform-aware modifier display Cmd on Mac, Ctrl elsewhere; WCAG: `role="dialog"` + `aria-modal`, combobox pattern with listbox, `aria-activedescendant`; zero external dependencies using SolidJS signals + native KeyboardEvent)
- [x] (2026-02-19) `App.tsx` updated with CommandPalette integration inside ThemeProvider

#### 10F. Toast/Notification System (COMPLETED)

- [x] (2026-02-19) `frontend/src/components/Toast.tsx` — ToastProvider + ToastItem (Context-based API: `useToast()` returns `{ show, dismiss }`; 4 toast levels: success, error, warning, info color-coded; auto-dismiss 5s default, max 3 simultaneous, oldest eviction; zero dependencies: SolidJS `createSignal` + `Portal`; WCAG: `aria-live="polite"`, `role="alert"` for errors, `role="status"` for others)
- [x] (2026-02-19) Toasts wired into all 7 API-calling panels (DashboardPage: project create/delete; ProjectDetailPage: budget alerts, run/plan completion, git operations; RunPanel, AgentPanel, RoadmapPanel, TaskPanel, ModelsPage: all CRUD operations)

#### 10D. WCAG 2.2 AA Conformance (COMPLETED)

- [x] (2026-02-19) Comprehensive WCAG audit across all ~20 frontend component files
- [x] (2026-02-19) `index.css`: `:focus-visible` outline style (2px solid, 2px offset), `.skip-link` class (sr-only until focused), `@media (prefers-reduced-motion: reduce)` disables transitions/animations
- [x] (2026-02-19) `App.tsx`: Skip-to-content link (`#main-content`), landmark `aria-label` on `<aside>` and `<nav>`, `aria-live="polite"` on status indicators
- [x] (2026-02-19) All panels: `aria-label` on buttons/inputs/selects, `for`/`id` associations on form labels, `role="status"` + `aria-label` on status badges, `aria-hidden="true"` on decorative elements
- [x] (2026-02-19) Interactive elements: keyboard handlers (Enter/Space) on expandable rows (TrajectoryPanel, PlanPanel), `tabIndex={0}` on clickable non-button elements, `aria-expanded` on toggleable sections
- [x] (2026-02-19) Tables: `scope="col"` on `<th>` elements (CostDashboardPage, ModelsPage)
- [x] (2026-02-19) CommandPalette: focus trap (Tab key prevented from escaping), `role="dialog"` + `aria-modal`, combobox/listbox/option pattern
- [x] (2026-02-19) Color contrast: Tailwind dark mode classes ensure 4.5:1+ ratio for all text/background pairs
- Files changed: index.css, App.tsx, CommandPalette.tsx, OfflineBanner.tsx, DashboardPage.tsx, ProjectCard.tsx, CostDashboardPage.tsx, ModelsPage.tsx, AgentPanel.tsx, LiveOutput.tsx, PlanPanel.tsx, PolicyPanel.tsx, ProjectDetailPage.tsx, RepoMapPanel.tsx, RetrievalPanel.tsx, RoadmapPanel.tsx, RunPanel.tsx, TaskPanel.tsx, TrajectoryPanel.tsx
- [x] (2026-02-19) axe-core Playwright accessibility audit in E2E tests (`@axe-core/playwright` devDependency; `frontend/e2e/a11y.spec.ts`: automated WCAG 2.2 AA checks on Dashboard, Costs, Models, Login pages; tags: `wcag2a`, `wcag2aa`, `wcag22aa`)

#### 10G. Error Boundary + Offline Detection (COMPLETED)

- [x] (2026-02-19) SolidJS `ErrorBoundary` in `App.tsx` with fallback UI + retry button
- [x] (2026-02-19) `frontend/src/components/OfflineBanner.tsx` — online/offline + WS status banner (`navigator.onLine` + window online/offline events + WebSocket connection status; yellow banner with pulsing indicator when disconnected)
- [x] (2026-02-19) API retry logic in `frontend/src/api/client.ts` (Exponential backoff 1s/2s/4s, max 3 retries; only idempotent methods GET/PUT/DELETE on 502/503/504; network failure TypeError retry for all methods)
- [x] (2026-02-19) Graceful degradation: in-memory GET cache + offline mutation queue (`frontend/src/api/cache.ts`: response cache 5min TTL, mutation queue with auto-drain; `frontend/src/api/client.ts`: cache on GET success, serve stale on network fail; queue mutations when offline, auto-retry on `window.online` event; `.gitignore` updated for Playwright artifacts e2e-results/, e2e-report/, test-results/)

### Phase 11 — Future GUI Enhancements (COMPLETED)

#### 11A. ProjectDetailPage Tab Navigation (COMPLETED)

- [x] (2026-02-19) Tab-based layout replacing vertical panel stack (5 tabs: Overview, Tasks & Roadmap, Agents & Runs, Context, Costs; tab bar with `role="tablist"`, `aria-selected`, `aria-controls` for WCAG compliance; i18n keys: `detail.tab.*` EN + DE; Overview: Git status, branches, clone/pull actions; Tasks & Roadmap: TaskPanel + RoadmapPanel; Agents & Runs: AgentPanel + PolicyPanel + RunPanel + PlanPanel + LiveOutput; Context: RepoMapPanel + RetrievalPanel requires workspace; Costs: ProjectCostSection)

#### 11B. Settings/Configuration Page (COMPLETED)

- [x] (2026-02-19) Full settings page aggregating existing backend APIs (Provider info cards Git, Agent, Spec, PM with loading/empty states; LLM health status indicator connected/unavailable/checking; API key management: create, list, delete with copy-once warning; user management table admin only: enable/disable toggle, delete, role badges; ~38 i18n keys EN + DE, route `/settings`, sidebar nav link; ProviderCard helper component for uniform display)

#### 11C. Mode Selection UI (COMPLETED)

- [x] (2026-02-19) Agent modes page with card grid and create form (ModesPage at `/modes`: displays all 8 built-in + custom modes as cards; mode cards: name, description, tool badges, LLM scenario, autonomy level color-coded, expandable prompt; create custom mode form: id, name, description, tools, scenario, autonomy, prompt prefix; built-in modes sorted before custom, protected from overwrite; ~35 i18n keys EN + DE, sidebar nav link)

#### 11D. Step-Progress Indicators (COMPLETED)

- [x] (2026-02-19) Visual progress bars for runs and plans (`StepProgress` reusable component: horizontal bar + fraction label; color coding: blue normal, yellow 70-90%, red >90%, indeterminate when max unknown; ARIA progressbar role, locale-aware screen reader labels; RunPanel: progress bar during active runs max from policy termination.max_steps; PlanPanel: progress bar for running plans completed steps / total steps + plan detail view; 5 i18n keys EN + DE)

#### 11E. Global Activity Stream (COMPLETED)

- [x] (2026-02-19) Cross-project real-time activity stream (`ActivityPage` component with global WebSocket subscription via `createCodeForgeWS()`; `classifyMessage()` maps 12+ WS event types to severity-tagged `ActivityEntry` objects; event types: run.status, run.toolcall, run.budget_alert, run.qualitygate, run.delivery, agent.status, task.status, plan.status, plan.step.status, repomap/retrieval/roadmap.status; severity classification: info default, success completed, warning budget/denied, error failed/timeout; TYPE_ICONS and SEVERITY_COLORS for visual differentiation; filter by event type, pause/resume streaming, clear all, max 200 entries newest first; project links via projectId, ARIA role="log" + aria-live="polite"; route `/activity`, sidebar nav link between Modes and Settings; 12 i18n keys EN + DE)

#### 11F. Team/Multi-Agent Management UI (COMPLETED)

- [x] (2026-02-19) Team management page for multi-agent coordination (`TeamsPage` at `/teams` with project selector dropdown; create team form: name, protocol round-robin/pipeline/parallel/consensus/ping-pong, member assignment; member assignment: select agent + role coder/reviewer/tester/documenter/planner with add/remove; team cards: status badge initializing/active/completed/failed, protocol, member count, creation date; expandable detail: member list with role-colored badges, agent name resolution; shared context viewer: key/value items with author, token count, version number; TEAM_STATUS_COLORS and ROLE_COLORS for visual differentiation; route `/teams`, sidebar nav link between Activity and Settings; ~30 i18n keys EN + DE)

#### 11G. Trajectory Replay Inspector (COMPLETED)

- [x] (2026-02-19) Step-by-step event replay with playback controls (Replay mode toggle in existing TrajectoryPanel with browse mode preserved; timeline scrubber bar range input + mini timeline dots colored by event type; play/pause button with auto-advance, 4 playback speeds 0.5x/1x/2x/4x; step prev/next buttons for manual navigation; enhanced `EventDetail` component: structured display for tool calls with tool name badge, input, output, error sections; non-tool events still show raw JSON payload; browse mode enhanced with blue border highlight on expanded events; ARIA labels on all replay controls, aria-live on current event; 13 new i18n keys EN + DE)

#### 11H. Diff-Review Code Preview (COMPLETED)

- [x] (2026-02-19) Unified diff viewer for agent code changes (`DiffPreview` reusable component: parses unified diff text into files, hunks, and lines; color-coded display: green additions, red removals, blue hunk headers, gray context; old/new line numbers, collapsible per-file sections, +/- counts per file header; auto-detection in TrajectoryPanel: checks payload.diff, payload.patch, payload.output for unified diff patterns; renders in tool_result events and non-tool events e.g. delivery with patch content; falls back to regular output display when no diff detected)

#### 11I. Split-Screen Feature Planning (COMPLETED)

- [x] (2026-02-19) Split-screen decompose view in PlanPanel (Prompt form on the left, generated plan preview on the right; responsive grid: single column by default, side-by-side on lg+ breakpoint; decompose result ExecutionPlan stored in signal instead of discarded; plan preview shows: name, description, protocol badge, step count; step list with numbered badges, task/agent names, dependency references; accept button confirms and closes form, discard button clears preview for re-try; 8 new i18n keys EN + DE in `plan.preview.*` namespace)

#### 11J. Multi-Terminal Agent View (COMPLETED)

- [x] (2026-02-19) Tiled terminal output for concurrent agent teams (`MultiTerminal` component: responsive grid of per-agent terminal tiles; `TerminalTile` sub-component: auto-scroll, line count, stdout/stderr coloring; expand/collapse: click to expand single tile to full width, hiding others; ProjectDetailPage tracks output per `agent_id` from `task.output` WS events; shows MultiTerminal when 2+ agents have output, falls back to single LiveOutput; responsive grid: 1 col default, 2 cols on lg at <=4 agents, 3 cols on xl at >4 agents; 7 new i18n keys EN + DE in `multiTerminal.*` namespace)

#### 11K. Vector Search Simulator (COMPLETED)

- [x] (2026-02-19) "What does the agent know?" debug tool (`SearchSimulator` component on Context tab alongside RepoMapPanel and RetrievalPanel; uses hybrid search, agent search query expansion, and GraphRAG APIs; adjustable parameters: BM25/semantic weight sliders linked, top-K, token budget; token budget progress bar with per-result token estimation ~4 chars/token; results color-coded green/red based on budget fit, BM25 rank + semantic rank columns; agent mode toggle enables query expansion, GraphRAG toggle adds graph results; 28 new i18n keys EN + DE in `simulator.*` namespace)

#### 11L. Architecture Graph Visualization (COMPLETED)

- [x] (2026-02-19) SVG-based code architecture graph explorer (`ArchitectureGraph` component on Context tab, queries GraphRAG search API; force-directed layout: repulsion between all nodes, attraction along edges, center gravity; animated convergence 120 frames, nodes colored by kind module/class/function/method; node size varies by kind module=12, class=10, function=6, method=5; hover interaction: highlight connected edges, show labels, dim unconnected nodes; seed symbol input comma-separated, configurable max hops, graph status display; raw search results in collapsible `<details>` section; 18 new i18n keys EN + DE in `archGraph.*` namespace)

#### 11M. Agent Network Visualization (COMPLETED)

- [x] (2026-02-19) Real-time agent team communication graph (`AgentNetwork` component on Agents tab, visualizes team members as SVG network; circle layout for team members, nodes colored by role with status ring idle/active/error; WS event listener for `team.message` and `shared_context.update` — animates message flow; active edges pulse with arrowhead markers, reset after 1.5s animation; agent status tracking from `agent.status` WS events; team selector buttons with status badges, role legend, message flow log last 20; 11 new i18n keys EN + DE in `agentNetwork.*` namespace)

### Post-Phase 11: Security Hardening (COMPLETED)

> All 18 findings (5 P0, 8 P1, 5 P2) from security audit, fixed 2026-02-19.

- [x] (2026-02-19) P0-1: Prompt Injection Defense — `sanitizePromptInput()` in `meta_agent.go`: strips control chars, neutralizes role markers (system:/assistant:/[system]/etc.), truncates at 10k chars, data-boundary instruction in system prompt. 6 tests in `sanitize_test.go`.
- [x] (2026-02-19) P0-2: Secret Redaction Utilities — `Vault.Redacted()` (masked value for logs), `Vault.RedactString()` (scrub secrets from arbitrary strings), `Vault.Keys()` (key names only), `maskValue()` helper. 4 new tests in `vault_test.go`.
- [x] (2026-02-19) P0-3: Audit Trail in RuntimeService — `appendAudit()` helper wired into 8 lifecycle points: run.started, run.completed, run.cancelled, policy.denied, qualitygate.passed, qualitygate.failed, qualitygate.error, delivery.completed, delivery.failed, budget.exceeded.
- [x] (2026-02-19) P0-4: Quality Gate Fail-Closed — NATS publish failure for quality gate request now fails the run instead of silently passing. Audit entry recorded.
- [x] (2026-02-19) P0-5: Post-Execution Budget Enforcement — Immediate budget check in `HandleToolCallResult` after cost accumulation. Single expensive tool call that exceeds budget triggers immediate run termination with StatusTimeout.
- [x] (2026-02-19) P1-1: GitLab PM Adapter — `internal/adapter/gitlab/` with REST API v4, PRIVATE-TOKEN auth, ListItems/GetItem/CreateItem/UpdateItem, self-registering provider, 8 tests.
- [x] (2026-02-19) P1-2: Prompt Templates — `text/template` + `//go:embed` for meta-agent decomposition prompts (`decompose_system.tmpl`, `decompose_user.tmpl`). Replaces hardcoded strings.
- [x] (2026-02-19) P1-3: BM25-Inspired Scoring — `ScoreFileRelevance()` replaced with BM25 algorithm (k1=1.5, b=0.75). Same interface, normalized 0-100 output.
- [x] (2026-02-19) P1-4: Branch Protection Default-DENY — `EvaluatePush/Merge/Delete` deny when enabled rules exist but none match. Zero rules → allow (backward compat).
- [x] (2026-02-19) P1-5: WebSocket Authentication — `/ws` requires JWT via `?token=` query param when auth enabled. Auth-disabled mode unchanged.
- [x] (2026-02-19) P1-6: JWT Standard Claims + Revocation — JTI (UUID), audience, issuer in tokens. PostgreSQL `revoked_tokens` blacklist table (migration 023). Fail-open on DB error, backward-compat for old tokens. Token cleanup goroutine.
- [x] (2026-02-19) P1-7: Tenant UUID Validation — `X-Tenant-ID` header validated against UUID regex. Invalid → 400, empty → default tenant.
- [x] (2026-02-19) P1-8: Stall Re-Planning — `StallTracker` retry mechanism (maxRetries default 2). `ReplanStep()` resets stalled runs for re-dispatch. Configurable via `StallMaxRetries`.
- [x] (2026-02-19) P2-1: API Key Scopes — Resource-based scopes (`projects:read`, `runs:write`, etc.) on API keys. `RequireScope()` middleware. Nil scopes = full access (backward compat). Migration 023.
- [x] (2026-02-19) P2-2: Forced Password Change — `MustChangePassword` flag on User. Seeded admin gets `true`. 403 on non-exempt paths. `/api/v1/auth/change-password` endpoint.
- [x] (2026-02-19) P2-3: Atomic Refresh Token Rotation — `RotateRefreshToken()` wraps delete+insert in PostgreSQL transaction. Prevents race conditions.
- [x] (2026-02-19) P2-4: Password Complexity — Min 10 chars, uppercase + lowercase + digit required. Applies to registration and password change only.
- [x] (2026-02-19) P2-5: Delivery Error Propagation — `PushError` field on `DeliveryResult`. `deliverPR()` skips PR creation on push failure. Error surfaced in audit/WS.

### Phase 12A — Mode System Extensions (IN PROGRESS)

- [x] (2026-02-23) Mode domain extended with `DeniedTools`, `DeniedActions`, `RequiredArtifact` fields + overlap validation in `Validate()`
- [x] (2026-02-23) All 8 built-in presets updated with meaningful defaults for new fields
- [x] (2026-02-23) Agent struct extended with `ModeID` field; Run struct and StartRequest extended with `ModeID`
- [x] (2026-02-23) NATS `ModePayload` struct added to `RunStartPayload` for mode metadata transmission to workers
- [x] (2026-02-23) Python `ModeConfig` Pydantic model added; `RunStartMessage` extended with `mode` field
- [x] (2026-02-23) RuntimeService wired with ModeService: mode resolution chain (explicit > agent > "coder" fallback), mode in NATS payload, event metadata, audit trail
- [x] (2026-02-23) Python executor uses `mode.prompt_prefix` as system prompt (backward-compatible fallback)
- [x] (2026-02-23) Frontend: new form fields (Denied Tools, Denied Actions, Required Artifact) + card display + i18n (en + de)
- [x] (2026-02-23) DB migration 024: `mode_id` column on agents and runs tables; store queries updated
- [x] (2026-02-23) Tests: 16 Go mode tests (overlap detection, presets validation, unique IDs), 5 Python ModeConfig tests
