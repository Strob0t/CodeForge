# CodeForge — Project Status

> Last update: 2026-02-18

## Phase 0: Project Setup (current)

### Completed

- [x] Market research conducted (docs/research/market-analysis.md)
  - 20+ existing projects analyzed
  - Market gap identified: No integrated solution for Project Dashboard + Roadmap + Multi-LLM + Agent Orchestration
  - SVN support confirmed as unique selling point
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
- [x] Orchestration frameworks analyzed: LangGraph, CrewAI, AutoGen, MetaGPT
  - Detailed feature comparison and architecture mapping
  - Adopted patterns identified and documented
- [x] Framework insights integrated into architecture:
  - Composite Memory Scoring (Semantic + Recency + Importance)
  - Context Window Strategies (Buffered, TokenLimited, HeadAndTail)
  - Experience Pool (@exp_cache) for caching successful runs
  - Tool Recommendation via BM25, Workbench (Tool Container)
  - LLM Guardrail Agent, Structured Output / ActionNode
  - Event Bus for Observability, GraphFlow / DAG Orchestration
  - Composable Termination Conditions, Component System (declarative)
  - Document Pipeline PRD→Design→Tasks→Code
  - MagenticOne Planning Loop (Stall Detection + Re-Planning)
  - HandoffMessage Pattern, Human Feedback Provider Protocol
- [x] LLM Routing & Multi-Provider analyzed: LiteLLM, OpenRouter, Claude Code Router, OpenCode CLI
  - LiteLLM: 127+ Providers, Proxy Server, Router (6 strategies), Budget Management, 42+ Observability
  - OpenRouter: 300+ Models, Cloud-only, ~5.5% Fee → as provider behind LiteLLM
  - Claude Code Router: Scenario-based Routing (default/background/think/longContext)
  - OpenCode CLI: OpenAI-compatible Base URL Pattern, Copilot Token Exchange, Auto-Discovery
- [x] Architecture decision: No custom LLM interface, LiteLLM Proxy as Docker sidecar
  - Go Core and Python Workers use OpenAI-compatible API against LiteLLM (Port 4000)
  - Scenario-based routing via LiteLLM Tag-based Routing
  - Custom development: Config Manager, User Key Mapping, Scenario Router, Cost Dashboard
  - Local Model Discovery (Ollama/LM Studio), Copilot Token Exchange
- [x] Roadmap/Spec/PM tools analyzed: OpenSpec, Spec Kit, Autospec, Plane.so, OpenProject, Ploi Roadmap
  - 6+ SDD tools analyzed (OpenSpec, GitHub Spec Kit, Autospec, BMAD-METHOD, Amazon Kiro, cc-sdd)
  - 4+ PM tools analyzed (Plane.so, OpenProject, Ploi Roadmap, Huly, Linear)
  - Repo-based PM tools mapped (Markdown Projects, Backlog.md, git-bug, Tasks.md)
  - ADR/RFC tools and feature flag tools identified as extensions
  - Gitea/Forgejo identified as GitHub-compatible SCM alternative
- [x] Auto-Detection architecture designed: Three-Tier Detection (Repo → Platform → File)
  - Spec-Driven Detectors: OpenSpec, Spec Kit, Autospec, ADR/RFC
  - Platform Detectors: GitHub, GitLab, Plane.so, OpenProject
  - File-Based Detectors: ROADMAP.md, TASKS.md, CHANGELOG.md
- [x] Provider Registry extended: specprovider + pmprovider (same architecture as Git)
- [x] Architecture decision: No custom PM tool, bidirectional sync with existing tools
  - Adopted patterns: Cursor Pagination, HMAC-SHA256, Label Sync (Plane), Optimistic Locking, Schema Endpoints (OpenProject), Delta Spec Format (OpenSpec), `/ai` Endpoint (Ploi Roadmap)
  - Explicitly NOT adopted: HAL+JSON/HATEOAS, GraphQL, custom PM tool
- [x] Deep analysis of all AI Coding Agents (Section 1+2 in market-analysis.md):
  - OpenHands: Event Sourcing, AgentHub, Microagents, Risk Management, V0→V1 SDK
  - SWE-agent: ACI, ReAct Loop, Tool Bundles, History Processors, SWE-ReX, Mini-SWE-Agent
  - Aider: tree-sitter Repo Map, 7+ Edit Formats, Architect/Editor Pattern (separate file: aider-deep-analysis.md)
  - Cline: Three-Tier Runtime, Plan/Act Mode, Shadow Git, MCP, Ask/Say Approval
  - Devika: 9 Sub-Agents, Jinja2 Templates, SentenceBERT, Agent State Visualization
- [x] Extended competitive analysis: 12 new tools identified and analyzed
  - 5 new competitors: Codel, AutoForge, bolt.diy, Dyad, CLI Agent Orchestrator (AWS)
  - 7 new backend candidates: Goose, OpenCode, Plandex, AutoCodeRover, Roo Code, Codex CLI, SERA
  - Backend integration priorities defined (Goose, OpenCode, Plandex as Priority 1)
  - Closest competitor identified: CLI Agent Orchestrator (AWS) — same vision without Web GUI
- [x] Architecture decision: YAML as unified configuration format (comment support)
- [x] Autonomy spectrum defined: 5 levels (supervised → headless)
  - Safety rules replace user at levels 4-5 (Budget, Tests, Blocklists, Branch Isolation)
  - Headless mode for CI/CD, cron jobs, API-driven pipelines
  - Schedule support for automatic nightly reviews, dependency updates
  - API endpoint for external systems (GitHub Actions, GitLab CI, Jenkins)
- [x] Agent specialization elaborated: Modes System (YAML configs)
  - Built-in Modes: architect, coder, reviewer, debugger, tester, lint-fixer, planner, researcher
  - Custom Modes: User-definable in `.codeforge/modes/`
  - Mode Pipelines and DAG composition for multi-agent workflows
  - Each mode: own tools, LLM scenario, autonomy level, prompt template
- [x] Coding agent insights integrated into architecture:
  - Shadow Git Checkpoints, Event-Sourcing, Microagents (OpenHands)
  - Diff-based File Review, ACI (SWE-agent), Stateless Agent Design (Devika)
  - tree-sitter Repo Map, Architect/Editor Pattern, Edit Formats (Aider)
  - Skills System, Risk Management (OpenHands), Plan/Act Mode (Cline)
- [x] Architecture decision: PostgreSQL 17 as primary database (shared with LiteLLM) — [ADR-002](architecture/adr/002-postgresql-database.md)
  - pgx v5 (Go driver), goose (migrations), psycopg3 (Python driver)
  - Shared instance with LiteLLM via schema separation
  - Simplicity principle: no ORM, no code generator, no extra tooling
- [x] Library decisions finalized (minimal-dependency principle):
  - Go: chi v5 (router, 0 deps), coder/websocket (WS, 0 deps), git exec wrapper (0 deps)
  - Frontend: @solidjs/router, Tailwind CSS (no component lib), @solid-primitives/websocket (728B), native fetch
  - Rejected: Echo, Fiber, gorilla/websocket, go-git (28 deps), axios, styled-components, Kobalte
- [x] Protocol support analyzed and prioritized: MCP, LSP, OpenTelemetry, A2A, AG-UI
  - Tier 1 (Phase 1-2): MCP (agent ↔ tools), LSP (code intelligence), OpenTelemetry GenAI (observability)
  - Tier 2 (Phase 2-3): A2A (agent coordination, Linux Foundation), AG-UI (frontend streaming, CopilotKit)
  - Tier 3 (future): ANP (decentralized), LSAP (LSP for AI)
- [x] Documentation consistency audit: all docs synchronized and translated to English
- [x] Documentation structure created:
  - docs/README.md (documentation index)
  - docs/todo.md (central TODO tracker for LLM agents)
  - docs/features/ (individual feature specs for all 4 pillars)
  - docs/architecture/adr/ (ADR template for future decisions)
  - Documentation Policy added to CLAUDE.md

### Open

> **Phase 0 complete.** All tasks done — proceed to Phase 1.
> For granular tasks, see [todo.md](todo.md).

- [x] Devcontainer verified (Go 1.23.12, Python 3.12.12, Node.js 22.22.0, Docker-in-Docker)
- [x] Go module initialized, project structure created, chi HTTP server with health endpoint
- [x] Python Workers scaffold (consumer, health, 3 tests passing)
- [x] SolidJS frontend initialized (Tailwind CSS v4, ESLint 9, Prettier, @solidjs/router)

## Phase 1: Foundation (COMPLETED)

- [x] (2026-02-14) WP1: Infrastructure — Docker Compose (NATS, LiteLLM), DB schema, migrations
- [x] (2026-02-14) WP2: Go Core — Domain entities, ports, registries, WebSocket, NATS adapter
- [x] (2026-02-14) WP3: Python Worker — NATS consumer, LiteLLM client, Pydantic models, 16 tests
- [x] (2026-02-14) WP4: Go Core — DB store, REST API (projects/tasks), services, handler tests
- [x] (2026-02-14) WP5: Frontend — API client, WebSocket, Dashboard page with CRUD
- [x] (2026-02-14) WP6: Protocol stubs (MCP, LSP, OTEL), GitHub Actions CI

### Phase 1 Key Deliverables
- **Go:** 1.24, chi v5, pgx v5, goose, coder/websocket, nats.go — 0 lint issues, 11 tests (expanded to 27 in Phase 2)
- **Python:** nats-py, httpx, pydantic v2 — 16 tests, ruff clean
- **Frontend:** SolidJS, @solidjs/router, @solid-primitives/websocket — build + lint + format clean
- **CI:** 3-job GitHub Actions (Go, Python, Frontend)
- **API:** 9 REST endpoints, WebSocket, health with service status

## Phase 2: MVP Features (COMPLETED)

- [x] (2026-02-14) WP1: Git Local Provider — Clone, Status, Pull, ListBranches, Checkout via git CLI
- [x] (2026-02-14) WP2: Agent Lifecycle — Aider backend, async NATS dispatch, agent CRUD API
- [x] (2026-02-14) WP3: WebSocket Events — Live agent output, task/agent status broadcasting
- [x] (2026-02-14) WP4: LLM Provider Management — LiteLLM admin API client, model CRUD endpoints
- [x] (2026-02-14) WP5: Frontend — Project detail page, git operations UI, task list
- [x] (2026-02-14) WP6: Frontend — Agent monitor panel, live terminal output, task create/expand
- [x] (2026-02-14) WP7: Frontend — LLM models page, add/delete models, health status
- [x] (2026-02-14) WP8: Integration test, documentation update, test fixes

### Phase 2 Key Deliverables
- **Go:** 27 tests, gitlocal provider, aider backend, agent service, LiteLLM client, 19 REST endpoints
- **Python:** 16 tests, streaming output via NATS, LiteLLM health checks
- **Frontend:** 13 components, 4 routes (/, /projects, /projects/:id, /models), WebSocket live updates
- **API:** Git ops (clone/pull/branches/checkout/status), Agent CRUD + dispatch/stop, LLM CRUD + health

## Phase 3: Reliability, Performance & Agent Foundation (COMPLETED)

- [x] (2026-02-17) WP1: Configuration Management — hierarchical config (defaults < YAML < ENV), typed Config struct, validation, 6 tests
- [x] (2026-02-17) WP2: Structured Logging & Request ID — slog factory, structlog, X-Request-ID propagation HTTP→NATS→Python, 6 new tests
- [x] (2026-02-17) WP3: Graceful Shutdown & Docker Logging — 4-phase ordered shutdown, NATS Drain, Docker log rotation, logs.sh helper
- [x] (2026-02-17) WP4: Optimistic Locking & DB Pool Tuning
- [x] (2026-02-17) WP5: Circuit Breaker
- [x] (2026-02-17) WP6: Dead Letter Queue & Schema Validation
- [x] (2026-02-17) WP7: Event Sourcing for Agent Trajectory
- [x] (2026-02-17) WP8: Health Granularity & Rate Limiting

## Phase 4: Agent Execution Engine (COMPLETED)

### 4A. Policy Layer (COMPLETED)

- [x] (2026-02-17) Domain model: PolicyProfile, PermissionRule, ToolSpecifier, ToolCall, QualityGate, TerminationCondition
- [x] (2026-02-17) Validation: name required, valid mode/decision, non-negative limits
- [x] (2026-02-17) 4 built-in presets: plan-readonly, headless-safe-sandbox, headless-permissive-sandbox, trusted-mount-autonomous
- [x] (2026-02-17) YAML loader: LoadFromFile, LoadFromDirectory (custom policies)
- [x] (2026-02-17) Policy evaluator: first-match-wins rules, glob matching (incl. **), path/command constraints
- [x] (2026-02-17) Config integration: Policy.DefaultProfile, Policy.CustomDir, ENV overrides
- [x] (2026-02-17) REST API: GET /policies, GET /policies/{name}, POST /policies/{name}/evaluate
- [x] (2026-02-17) Composition root wiring in cmd/codeforge/main.go
- [x] (2026-02-17) 46 test functions: domain (20), service (25), config (3), handlers (7) — all passing

### Phase 4A Key Deliverables
- **New files:** 8 (policy domain types, validate, presets, loader, service, 3 test files)
- **Modified files:** 7 (config, loader, handlers, routes, main, config tests, handler tests)
- **API:** 3 new REST endpoints under /api/v1/policies
- **Tests:** 46 new test functions, all Go tests pass (153+ total)

### 4A-UI. Policy UI in Frontend (COMPLETED)

- [x] (2026-02-18) Backend: CRUD endpoints — POST /policies (create), DELETE /policies/{name} (delete custom only)
- [x] (2026-02-18) Backend: SaveProfile, DeleteProfile on PolicyService; IsPreset helper; SaveToFile in loader
- [x] (2026-02-18) Frontend: 10 new type definitions (PolicyProfile, PermissionRule, QualityGate, etc.)
- [x] (2026-02-18) Frontend: Extended API client (get, create, delete, evaluate)
- [x] (2026-02-18) Frontend: PolicyPanel component with 3 views (list, detail + evaluate tester, editor)
- [x] (2026-02-18) Frontend: Integrated into ProjectDetailPage between agents and run management
- [x] (2026-02-18) Tests: 6 new service tests (SaveProfile, DeleteProfile), 6 new handler tests (create/delete endpoints)

### 4B. Runtime API — Step-by-Step Execution Protocol (COMPLETED)

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

### Phase 4B Key Deliverables
- **New files:** 6 Go (run domain 3, runtime service, runtime tests, migration), 2 Python (runtime.py, test_runtime.py)
- **Modified files:** 12 Go (queue.go, schemas.go, store.go, event.go, events.go, handlers.go, routes.go, main.go, handlers_test.go, project_test.go), 3 Python (models.py, consumer.py, executor.py), 1 CI (.github/workflows/ci.yml)
- **API:** 4 new REST endpoints under /api/v1/runs + /api/v1/tasks/{id}/runs
- **Protocol:** Conversational NATS protocol for per-tool-call policy enforcement
- **Tests:** 44 new test functions (Go: 42, Python: 9), all passing

### 4C. Headless Autonomy — Stall Detection, Quality Gates, Delivery (COMPLETED)

- [x] (2026-02-17) CI Fix: golangci-lint v2 config migration (.golangci.yml)
- [x] (2026-02-17) Config extension: `config.Runtime` struct (6 fields) + ENV overrides + YAML example
- [x] (2026-02-17) Stall Detection: StallTracker (FNV-64a hash ring buffer), per-run tracking in RuntimeService
- [x] (2026-02-17) Quality Gate Enforcement: NATS request/result protocol, Python QualityGateExecutor
- [x] (2026-02-17) Deliver Modes: 5 strategies (none, patch, commit-local, branch, pr) via DeliverService
- [x] (2026-02-17) Frontend: RunPanel component, Run types/API, WS event integration
- [x] (2026-02-17) Events: 7 new event types + 2 WS event types (QG + delivery)

### Phase 4C Key Deliverables
- **New files:** 5 Go (stall.go, stall_test.go, deliver.go, deliver_test.go, migration 006), 2 Python (qualitygate.py, test_qualitygate.py), 1 Frontend (RunPanel.tsx)
- **Modified files:** 12 Go (config.go, loader.go, policy.go, presets.go, run.go, validate.go, event.go, events.go, queue.go, schemas.go, runtime.go, runtime_test.go, store.go, handlers_test.go, main.go), 3 Python (runtime.py, models.py, consumer.py), 3 Frontend (types.ts, client.ts, ProjectDetailPage.tsx), 2 Config (.golangci.yml, codeforge.yaml.example)
- **Tests:** 22+ new test functions (Go: stall 10, deliver 5, runtime 6+; Python: QG 7), all passing
- **Protocol:** Quality gate NATS subjects (request/result), stall detection, delivery pipeline

## Phase 3 & 4 Completion: Six Missing Features (COMPLETED)

> Implemented after Phase 6C to fill gaps from Phases 3 and 4.

- [x] (2026-02-18) **3G: Deprecation Middleware** — RFC 8594 `Deprecation` + `Sunset` headers, 3 tests
- [x] (2026-02-18) **3F: Secrets Vault with SIGHUP Hot Reload** — RWMutex vault, EnvLoader, SIGHUP handler, LiteLLM integration, 4 tests
- [x] (2026-02-18) **3C: Heartbeat Ticker + Context-Level Timeout** — Go heartbeat subscriber/timeout tracking (sync.Map), Python heartbeat ticker, config fields + ENV overrides, goroutine-based run timeout
- [x] (2026-02-18) **3D: Event Store Enhancements** — RunID on events (migration 013), LoadByRun query, GET /runs/{id}/events endpoint
- [x] (2026-02-18) **3H: Multi-Tenancy Preparation** — tenant_id on 7 tables (migration 014), TenantID middleware, context helpers, domain struct fields, 3 tests
- [x] (2026-02-18) **4B: Runtime Compliance Tests** — 8 sub-tests × 2 exec modes (Mount, Sandbox), 16 test cases passing

### Key Deliverables
- **New files (11):** deprecation.go/test, vault.go, env_loader.go, vault_test.go, tenant.go/test, migrations 013/014, runtime_compliance_test.go
- **Modified files (~15):** config.go, loader.go, runtime.go, event.go, eventstore interface, postgres/eventstore.go, handlers.go, routes.go, main.go, domain structs (Project/Task/Agent/Run), runtime.py, test_runtime.py
- **Tests:** 30+ new test functions (Go + Python), all passing
- **Verification:** golangci-lint 0 issues, 79/79 Python tests, frontend clean, pre-commit all hooks pass

## Phase 5: Multi-Agent Orchestration (IN PROGRESS)

### 5A. Execution Plans — DAG Scheduling with 4 Protocols (COMPLETED)

- [x] (2026-02-17) Domain model: `internal/domain/plan/` (plan.go, validate.go, dag.go)
  - ExecutionPlan, Step, CreatePlanRequest with JSON tags
  - Protocol (sequential/parallel/ping_pong/consensus), Status, StepStatus with IsTerminal()
  - Validation: name required, valid protocol, protocol-specific step count rules
  - DAG cycle detection via Kahn's algorithm (topological sort)
  - DAG helpers: ReadySteps, RunningCount, AllTerminal, AnyFailed
- [x] (2026-02-17) Domain tests: 25 tests (16 validation + 8 DAG + 1 compile check)
- [x] (2026-02-17) Config extension: `config.Orchestrator` (MaxParallel=4, PingPongMaxRounds=3, ConsensusQuorum=0)
  - ENV overrides: CODEFORGE_ORCH_MAX_PARALLEL, CODEFORGE_ORCH_PINGPONG_MAX_ROUNDS, CODEFORGE_ORCH_CONSENSUS_QUORUM
- [x] (2026-02-17) Database: migration 007_create_execution_plans.sql (execution_plans + plan_steps tables)
  - UUID arrays for step dependencies, FK to projects/tasks/agents/runs
- [x] (2026-02-17) Store interface: 9 new methods (CreatePlan, GetPlan, ListPlansByProject, UpdatePlanStatus, CreatePlanStep, ListPlanSteps, UpdatePlanStepStatus, GetPlanStepByRunID, UpdatePlanStepRound)
- [x] (2026-02-17) Postgres adapter: transactional CreatePlan, auto-loading steps in GetPlan
- [x] (2026-02-17) Events: 5 plan event types + 2 WS event types (plan.status, plan.step.status)
- [x] (2026-02-17) RuntimeService callback: SetOnRunComplete + invocation in finalizeRun
- [x] (2026-02-17) OrchestratorService: CreatePlan, StartPlan, GetPlan, ListPlans, CancelPlan, HandleRunCompleted
  - Sequential: one step at a time, failure stops plan
  - Parallel: all ready steps up to MaxParallel
  - PingPong: 2 agents alternate for N rounds
  - Consensus: same task to multiple agents, majority quorum vote
  - Core scheduling: mutex-protected advancePlan with protocol dispatch
- [x] (2026-02-17) REST API: 5 new endpoints (POST/GET /projects/{id}/plans, GET /plans/{id}, POST /plans/{id}/start, POST /plans/{id}/cancel)
- [x] (2026-02-17) Composition root: OrchestratorService wired with onRunComplete callback
- [x] (2026-02-17) Frontend: PlanPanel component, plan types + API client, WS event integration
- [x] (2026-02-17) Tests: 12 orchestrator service tests + 25 domain tests, all passing

### Phase 5A Key Deliverables
- **New files:** 8 Go (plan domain 3, plan tests 2, orchestrator service, orchestrator tests, migration), 1 Frontend (PlanPanel.tsx)
- **Modified files:** 13 Go (config.go, loader.go, store.go, postgres/store.go, event.go, events.go, runtime.go, handlers.go, routes.go, main.go, handlers_test.go, runtime_test.go, project_test.go), 3 Frontend (types.ts, client.ts, ProjectDetailPage.tsx), 1 Config (codeforge.yaml.example)
- **API:** 5 new REST endpoints for execution plan management
- **Tests:** 37 new test functions (25 domain + 12 service), all Go tests pass

### 5B. Orchestrator Agent — Meta-Agent (COMPLETED)

- [x] (2026-02-17) LiteLLM ChatCompletion client: types + method in `internal/adapter/litellm/client.go`
- [x] (2026-02-17) Decomposition domain types: `internal/domain/plan/decompose.go`
  - OrchestratorMode (manual/semi_auto/full_auto), AgentStrategy (single/pair/team)
  - DecomposeRequest, DecomposeResult, SubtaskDefinition
  - Validation: request + result validation, dependency index checks, self-reference detection
  - StrategyToProtocol mapping helper
- [x] (2026-02-17) Config extension: Mode, DecomposeModel, DecomposeMaxTokens in `config.Orchestrator`
  - ENV overrides: CODEFORGE_ORCH_MODE, CODEFORGE_ORCH_DECOMPOSE_MODEL, CODEFORGE_ORCH_DECOMPOSE_MAX_TOKENS
  - Defaults: semi_auto, openai/gpt-4o-mini, 4096
- [x] (2026-02-17) MetaAgentService: `internal/service/meta_agent.go`
  - DecomposeFeature: validate → load project/agents/tasks → build prompt → call LLM → parse JSON → create tasks → select agents → create plan → optionally auto-start
  - buildDecomposePrompt: system + user prompt with feature, context, agent list, existing tasks
  - selectAgent: hint-based matching (backend exact → name substring → idle fallback)
  - extractJSON: strips markdown code fences, finds JSON boundaries
  - Auto-start: full_auto mode or req.AutoStart triggers StartPlan
- [x] (2026-02-17) REST API: `POST /api/v1/projects/{id}/decompose` handler + route
- [x] (2026-02-17) Composition root: MetaAgentService wired in `cmd/codeforge/main.go`
- [x] (2026-02-17) Frontend: DecomposeRequest type, api.plans.decompose method, PlanPanel decompose UI
- [x] (2026-02-17) Tests: 4 litellm client tests, 5 domain tests (19 cases), 9 meta-agent service tests

### Phase 5B Key Deliverables
- **New files:** 3 Go (decompose.go, decompose_test.go, meta_agent.go, meta_agent_test.go), extended 1 test file (client_test.go)
- **Modified files:** 6 Go (client.go, config.go, loader.go, handlers.go, routes.go, main.go), 3 Frontend (types.ts, client.ts, PlanPanel.tsx), 1 Config (codeforge.yaml.example)
- **API:** 1 new REST endpoint (POST /projects/{id}/decompose)
- **Tests:** 18 new test functions (4 litellm + 5 domain + 9 service), all Go tests pass

### 5C. Agent Teams + Context-Optimized Planning (COMPLETED)

- [x] (2026-02-17) Agent Team domain model: `internal/domain/agent/team.go`
  - TeamRole (coder/reviewer/tester/documenter/planner), TeamStatus (initializing/active/completed/failed)
  - Team, TeamMember, CreateTeamRequest, CreateMemberRequest structs
  - Validation: name, project_id, at least 1 member, valid roles, no duplicate agents
  - 8 domain tests in `internal/domain/agent/team_test.go`
- [x] (2026-02-17) Database: migration 008_create_agent_teams.sql (agent_teams + team_members tables)
  - FK to projects and agents, unique constraint on (team_id, agent_id)
- [x] (2026-02-17) Store interface: 5 new methods (CreateTeam, GetTeam, ListTeamsByProject, UpdateTeamStatus, DeleteTeam)
- [x] (2026-02-17) Postgres adapter: transactional CreateTeam, member batch-loading in GetTeam/ListTeamsByProject
- [x] (2026-02-17) Config extension: `max_team_size` in Orchestrator (default: 5, ENV: CODEFORGE_ORCH_MAX_TEAM_SIZE)
- [x] (2026-02-17) PlanFeatureRequest domain type: extends DecomposeRequest with auto_team option
- [x] (2026-02-17) PoolManagerService: `internal/service/pool_manager.go`
  - CreateTeam (validates agents exist, idle, belong to project), AssembleTeamForStrategy (auto role assignment)
  - CleanupTeam (mark status, release agents to idle), GetTeam, ListTeams, DeleteTeam
  - 8 service tests in `internal/service/pool_manager_test.go`
- [x] (2026-02-17) TaskPlannerService: `internal/service/task_planner.go`
  - PlanFeature (enriches context with file listing, delegates to MetaAgentService, optionally assembles team)
  - gatherProjectContext (workspace file tree, skip hidden, cap at 100 entries)
  - estimateComplexity heuristic (1=single, 2=pair, 3+=team)
  - 3 service tests in `internal/service/task_planner_test.go`
- [x] (2026-02-17) REST API: 5 new endpoints
  - `POST /api/v1/projects/{id}/teams` — create team
  - `GET /api/v1/projects/{id}/teams` — list teams
  - `GET /api/v1/teams/{id}` — get team
  - `DELETE /api/v1/teams/{id}` — delete team
  - `POST /api/v1/projects/{id}/plan-feature` — context-optimized planning
- [x] (2026-02-17) Composition root: PoolManagerService + TaskPlannerService wired in main.go
- [x] (2026-02-17) Frontend: team types (AgentTeam, TeamMember, CreateTeamRequest), PlanFeatureRequest
  - API client: teams namespace (list/get/create/delete) + plans.planFeature method

### Phase 5C Key Deliverables
- **New files:** 7 Go (team.go, team_test.go, 008 migration, pool_manager.go, pool_manager_test.go, task_planner.go, task_planner_test.go)
- **Modified files:** 9 Go (store.go, postgres/store.go, decompose.go, config.go, loader.go, handlers.go, routes.go, main.go, 4 test files for mock updates), 2 Frontend (types.ts, client.ts), 1 Config (codeforge.yaml.example)
- **API:** 5 new REST endpoints (team CRUD + plan-feature)
- **Tests:** 19 new test functions (8 domain + 8 pool manager + 3 task planner), all Go tests pass

### 5D. Context Optimizer — ContextPack, SharedContext, Token Budget (COMPLETED)

- [x] (2026-02-17) ContextPack domain model: `internal/domain/context/pack.go`
  - ContextPack, ContextEntry, EntryKind (file/snippet/summary/shared)
  - Validation: TaskID + ProjectID required, budget > 0, entries non-empty, valid kind
  - EstimateTokens heuristic: `len(s) / 4` (1 token ≈ 4 chars)
  - 8 domain tests in `internal/domain/context/pack_test.go`
- [x] (2026-02-17) SharedContext domain model: `internal/domain/context/shared.go`
  - SharedContext, SharedContextItem, AddSharedItemRequest
  - Versioned with optimistic locking, per-team unique shared context
  - 6 domain tests in `internal/domain/context/shared_test.go`
- [x] (2026-02-17) Database: migration 009_create_context_packs.sql
  - 4 tables: context_packs, context_entries, shared_contexts, shared_context_items
  - Indexes, unique constraints, CASCADE deletes, updated_at trigger
- [x] (2026-02-17) Config extension: `default_context_budget` (4096), `prompt_reserve` (1024)
  - ENV overrides: CODEFORGE_ORCH_CONTEXT_BUDGET, CODEFORGE_ORCH_PROMPT_RESERVE
- [x] (2026-02-17) Store interface: 9 new methods (ContextPack CRUD + SharedContext CRUD)
- [x] (2026-02-17) Postgres adapter: transactional CreateContextPack, upsert AddSharedContextItem with version bump
- [x] (2026-02-17) ContextOptimizerService: `internal/service/context_optimizer.go`
  - BuildContextPack: scan workspace → keyword scoring → shared context injection → budget packing → persist
  - ScoreFileRelevance: keyword-matching scorer (0-100)
  - GetPackByTask: retrieve existing pack for HTTP API
  - 6 service tests in `internal/service/context_optimizer_test.go`
- [x] (2026-02-17) SharedContextService: `internal/service/shared_context.go`
  - InitForTeam, AddItem (with NATS notification), Get
  - 4 service tests in `internal/service/shared_context_test.go`
- [x] (2026-02-17) NATS: 2 new subjects (context.packed, context.shared.updated)
  - 3 new payload types: ContextEntryPayload, ContextPackedPayload, SharedContextUpdatedPayload
  - RunStartPayload extended with Context field for pre-packed context delivery
- [x] (2026-02-17) RuntimeService integration: context pack auto-built before run start
  - toContextEntryPayloads helper, non-fatal error handling (run proceeds without context on failure)
- [x] (2026-02-17) Python worker: ContextEntry model, RunStartMessage.context field
  - Consumer enriches prompt with context section (--- Relevant Context ---)
  - 2 new Python tests (with context + without context)
- [x] (2026-02-17) REST API: 4 new endpoints
  - `GET /api/v1/tasks/{id}/context` — get context pack for task
  - `POST /api/v1/tasks/{id}/context` — build context pack
  - `GET /api/v1/teams/{id}/shared-context` — get team shared context
  - `POST /api/v1/teams/{id}/shared-context` — add shared context item
- [x] (2026-02-17) Composition root: ContextOptimizerService + SharedContextService wired in main.go
  - RuntimeService.SetContextOptimizer injected for automatic context packing
- [x] (2026-02-17) Frontend: context types (ContextPack, ContextEntry, SharedContext, SharedContextItem)
  - API client: tasks.context/buildContext + teams.sharedContext/addSharedItem

### Phase 5D Key Deliverables
- **New files:** 9 Go (pack.go, pack_test.go, shared.go, shared_test.go, 009 migration, context_optimizer.go, context_optimizer_test.go, shared_context.go, shared_context_test.go)
- **Modified files:** 11 Go (store.go, postgres/store.go, config.go, loader.go, queue.go, schemas.go, runtime.go, handlers.go, routes.go, main.go, 3 test files), 2 Python (models.py, consumer.py), 1 Python test (test_consumer.py), 2 Frontend (types.ts, client.ts)
- **API:** 4 new REST endpoints (task context + shared context)
- **Protocol:** Context-enriched RunStartPayload, NATS context subjects
- **Tests:** 26+ new test functions (14 domain + 10 service + 2 Python), all passing

### 5E. Integration Fixes, WS Events, Modes System (COMPLETED)

- [x] (2026-02-17) Fix TeamID propagation: added `TeamID` field to `ExecutionPlan`, `Run`, `StartRequest`
  - Migration 010: `team_id` column on `execution_plans` and `runs` tables + `output` column on `runs`
  - Orchestrator passes `p.TeamID` through to `StartRequest` in `startStep()`
  - Runtime passes `req.TeamID` to `BuildContextPack` (was hardcoded `""`)
  - Run.Output field for capturing textual output of completed runs (separate from Error)
- [x] (2026-02-17) Auto-initialize SharedContext on team creation
  - PoolManagerService.SetSharedContext injection, calls InitForTeam after store.CreateTeam
- [x] (2026-02-17) Auto-populate SharedContext from run outputs
  - OrchestratorService.SetSharedContext injection
  - HandleRunCompleted stores run output as shared context item (`step_output:{stepID}`)
- [x] (2026-02-17) WS events for teams and shared context
  - 2 new event types: `team.status`, `shared.updated`
  - TeamStatusEvent + SharedContextUpdateEvent structs in ws/events.go
  - PoolManagerService broadcasts team.status on CreateTeam
  - SharedContextService broadcasts shared.updated on AddItem (nil-safe)
- [x] (2026-02-17) Modes System — domain model, presets, service, HTTP endpoints
  - Domain: Mode struct with Validate(), 8 built-in presets (architect, coder, reviewer, debugger, tester, documenter, refactorer, security)
  - ModeService: List, Get, Register (custom modes, built-in protection)
  - REST API: GET /modes, GET /modes/{id}, POST /modes
  - Composition root: ModeService wired in main.go
- [x] (2026-02-17) Mock store updates + test fixes
  - CompleteRun signature updated across all mock stores (added `output` param)
  - SharedContextService nil-safe hub broadcasting
  - handlers_test.go newTestRouter includes ModeService
- [x] (2026-02-17) Frontend types + API client
  - Run type: added team_id, output fields
  - ExecutionPlan type: added team_id field
  - New types: Mode, CreateModeRequest, TeamStatusEvent, SharedContextUpdateEvent
  - API client: modes namespace (list/get/create)

### Phase 5E Key Deliverables
- **New files:** 3 Go (mode.go domain, presets.go, mode.go service), 2 Go tests (mode_test.go domain, mode_test.go service), 1 migration (010_add_team_id_fields.sql)
- **Modified files:** 12 Go (plan.go, run.go, store interface, postgres/store.go, orchestrator.go, runtime.go, pool_manager.go, shared_context.go, events.go, handlers.go, routes.go, main.go), 4 Go test files (project_test.go, runtime_test.go, handlers_test.go, mode_test.go), 2 Frontend (types.ts, client.ts)
- **API:** 3 new REST endpoints (modes CRUD)
- **Events:** 2 new WS event types (team.status, shared.updated)
- **Tests:** 16+ new test functions (8 mode domain + 8 mode service), all Go tests pass
- **Lint:** golangci-lint 0 issues, frontend lint + build clean

## Infrastructure Features (3B, 3C, 3E, 3F, 4A/4C, 4B) (COMPLETED)

> Implemented between Phase 6B and 6C as foundational capabilities for the agent execution pipeline.

- [x] (2026-02-17) **3B: Async Logging** — Go `AsyncHandler` (slog wrapper, 10k buffer, 4 workers, drop policy) + Python `QueueHandler`/`QueueListener`
- [x] (2026-02-17) **3C: Idempotency Keys** — HTTP middleware for POST/PUT/DELETE dedup via NATS JetStream KV (24h TTL)
- [x] (2026-02-17) **3E: Cache Layer** — Tiered L1 (ristretto in-process) + L2 (NATS KV) with backfill, port/adapter pattern
- [x] (2026-02-17) **4A/4C: Checkpoint System** — Shadow Git commits per file-modifying tool call, rewind on quality gate failure, cleanup on finalize
- [x] (2026-02-17) **4B: Docker Sandbox** — SandboxService with Docker CLI (create/start/exec/stop/remove), resource flags, runtime lifecycle integration
- [x] (2026-02-17) **3F: Resource Limits** — Shared `resource.Limits` type with `Merge`/`Cap`, agent + policy fields, JSONB storage, migration 012

### Key Deliverables
- **New files (14):** async.go, async_test.go, idempotency.go, idempotency_test.go, cache port + 3 adapters, cache_test.go, checkpoint.go, checkpoint_test.go, sandbox.go, sandbox_test.go, limits.go, limits_test.go, migration 012
- **Modified files (14):** logger.go, config.go, loader.go, nats.go, runtime.go, run.go, validate.go, agent.go, policy.go, store.go (port + postgres), handlers.go, agent service, main.go, logger.py, consumer.py
- **New dependency:** `github.com/dgraph-io/ristretto/v2`
- **Tests:** 36+ new test functions (Go + Python), all passing

## Phase 6: Code-RAG (Context Engine for Large Codebases) (COMPLETED)

### 6A. Repo Map — tree-sitter Based Code Intelligence (COMPLETED)

- [x] (2026-02-17) Python Worker: RepoMapGenerator with tree-sitter parsing
  - tree-sitter + tree-sitter-language-pack for 16+ language support
  - Symbol extraction: functions, classes, methods, types, interfaces
  - File ranking via networkx PageRank (import graph analysis)
  - Compact map output: files + key symbols with token budget
  - NATS integration: repomap.generate / repomap.result subjects
  - Configurable token budget, max files, tag format
- [x] (2026-02-17) Go Backend: RepoMap domain, store, service, HTTP, WS
  - Domain model: RepoMap entity with validation
  - PostgreSQL storage: migration 011_create_repo_maps.sql
  - RepoMapService: Generate (via NATS to Python), Get, HandleResult
  - REST API: GET/POST /projects/{id}/repomap
  - WS event: repomap.status (generating/ready/failed)
- [x] (2026-02-17) Frontend: RepoMapPanel component
  - Stats display: file count, symbol count, token count
  - Language tags, version info, collapsible map text
  - Generate/regenerate button with loading state
  - Integrated into ProjectDetailPage (between Git and Agents sections)
  - WS event handler for repomap.status

### Phase 6A Key Deliverables
- **Python:** RepoMapGenerator, tree-sitter parsing, NATS consumer extension
- **Go:** Domain model, PostgreSQL store, service, 2 REST endpoints, WS events
- **Frontend:** RepoMapPanel.tsx, types (RepoMap, RepoMapStatusEvent), API client (repomap namespace)
- **Dependencies:** tree-sitter ^0.24, tree-sitter-language-pack ^0.13, networkx ^3.4

### 6B. Hybrid Retrieval — BM25 + Semantic Search (COMPLETED)

- [x] (2026-02-17) Python Worker: HybridRetriever with BM25S + LiteLLM embeddings
  - CodeChunker: AST-aware code splitting via tree-sitter at definition boundaries
  - HybridRetriever: BM25S keyword indexing + semantic embeddings via LiteLLM
  - Reciprocal Rank Fusion (RRF, k=60) combining BM25 and semantic rankings
  - In-memory per-project indexes (ProjectIndex dataclass)
  - Shared constants extracted to `_tree_sitter_common.py` (reuse from 6A)
  - Consumer: 4 NATS subjects, 4 handler methods for index + search
- [x] (2026-02-17) Go Backend: RetrievalService with synchronous search waiter
  - RetrievalService: RequestIndex, HandleIndexResult, SearchSync, HandleSearchResult
  - Channel-based waiter pattern with crypto/rand correlation IDs (30s timeout)
  - NATS: 4 subjects (retrieval.index.{request,result}, retrieval.search.{request,result})
  - REST API: POST /projects/{id}/index, GET /projects/{id}/index, POST /projects/{id}/search
  - WS event: retrieval.status (building/ready/error)
  - Context optimizer: auto-injects hybrid results as EntryHybrid with priority scoring
  - Config: 4 new fields (DefaultEmbeddingModel, RetrievalTopK, BM25Weight, SemanticWeight)
- [x] (2026-02-17) Frontend: RetrievalPanel component
  - Index status display with stats (file count, chunk count, embedding model, status badge)
  - Build Index button (disabled while building)
  - Search bar with results list (filepath:lines, symbol name, language badge, score)
  - Integrated into ProjectDetailPage with retrieval.status WS handler

### Phase 6B Key Deliverables
- **New files:** 5 (retrieval.py, _tree_sitter_common.py, test_retrieval.py, retrieval.go, retrieval_test.go, RetrievalPanel.tsx)
- **Modified files:** 19 (consumer.py, models.py, repomap.py, pyproject.toml, queue.go, schemas.go, validator.go, nats.go, pack.go, config.go, context_optimizer.go, events.go, handlers.go, routes.go, main.go, handlers_test.go, types.ts, client.ts, ProjectDetailPage.tsx)
- **API:** 3 new REST endpoints (index CRUD + search)
- **Dependencies:** bm25s ^0.2, numpy ^2.0
- **Tests:** 11 Python tests + 5 Go service tests + 3 Go handler tests, all passing

### 6C. Retrieval Sub-Agent — LLM-Guided Multi-Query Search (COMPLETED)

- [x] (2026-02-18) Python Worker: RetrievalSubAgent with LLM query expansion + parallel search + reranking
  - `RetrievalSubAgent` class: composes `HybridRetriever` + `LiteLLMClient`
  - Query expansion via LLM (task prompt → N focused search queries)
  - Parallel hybrid search across all expanded queries (`asyncio.gather`)
  - Deduplication by (filepath, start_line), keeps highest score
  - LLM re-ranking with score-based fallback on failure
  - `SubAgentSearchRequest` / `SubAgentSearchResult` Pydantic models
  - Consumer extended with `retrieval.subagent.request` NATS subscription + handler
- [x] (2026-02-18) Go Backend: RetrievalService sub-agent extensions
  - `SubAgentSearchSync()`: correlation-ID-based sync request/response (60s timeout)
  - `HandleSubAgentSearchResult()`: delivers result to waiting channel
  - NATS subjects: `retrieval.subagent.request`, `retrieval.subagent.result`
  - Payload types: `SubAgentSearchRequestPayload`, `SubAgentSearchResultPayload`
  - Config: `SubAgentModel`, `SubAgentMaxQueries`, `SubAgentRerank` + ENV overrides
  - Context optimizer: `fetchRetrievalEntries()` — sub-agent first, single-shot fallback
  - HTTP handler: `POST /projects/{id}/search/agent`
- [x] (2026-02-18) Frontend: Agent search mode in RetrievalPanel
  - Standard/Agent toggle button next to search input
  - Agent mode calls `api.retrieval.agentSearch()` endpoint
  - Expanded queries displayed as purple tags
  - Total candidates count badge
  - Types: `SubAgentSearchRequest`, `SubAgentSearchResult`

### Phase 6C Key Deliverables
- **New files:** 1 (test_retrieval_subagent.py)
- **Modified files:** 13 (models.py, retrieval.py, consumer.py, queue.go, schemas.go, validator.go, config.go, loader.go, retrieval.go, context_optimizer.go, handlers.go, routes.go, retrieval_test.go, types.ts, client.ts, RetrievalPanel.tsx)
- **API:** 1 new REST endpoint (POST /projects/{id}/search/agent)
- **Config:** 3 new env vars (CODEFORGE_ORCH_SUBAGENT_MODEL, CODEFORGE_ORCH_SUBAGENT_MAX_QUERIES, CODEFORGE_ORCH_SUBAGENT_RERANK)
- **Tests:** 8 Python tests + 3 Go service tests, all passing

### 6C Code Review Refinements (COMPLETED)

- [x] (2026-02-18) 16 code review improvements across architecture, quality, tests, performance
  - **Architecture:** Generic `syncWaiter[T]` (DRY), health tracking with 30s cooldown fast-fail, shared retrieval deadline, check-before-build guard, parallel workspace scan + retrieval
  - **Code Quality:** Unified `SearchResult` → `RetrievalSearchHit`, DRY `_publish_error_result()` consumer helper, defense-in-depth validation (Pydantic + Go handler bounds)
  - **Performance:** Pre-built rank dict for O(1) BM25 lookup, `per_query_k = top_k` fix, percentile-based priority normalization (60-85 range)
  - **Tests:** 5 new tests (error-in-payload Go, parallel-all-fail Python, Pydantic validator bounds, consumer error publish)
  - All 77 Python tests + all Go tests passing, golangci-lint 0 issues

### 6D. GraphRAG — Structural Code Graph Intelligence (COMPLETED)

- [x] (2026-02-18) PostgreSQL adjacency-list graph (no Neo4j — single-DB architecture)
  - Migration 016: `graph_nodes`, `graph_edges`, `graph_metadata` tables
  - Nodes: function/class/method/module definitions via tree-sitter
  - Edges: import edges (Python/Go/TS/JS), call edges (name-matching heuristic)
- [x] (2026-02-18) Python: CodeGraphBuilder + GraphSearcher (`workers/codeforge/graphrag.py`)
  - Build pipeline: walk files → tree-sitter parse → extract definitions + imports + calls → batch upsert to PostgreSQL
  - BFS search with hop-decay scoring (default decay 0.7): hop 0 = 1.0, hop 1 = 0.7, hop 2 = 0.49
  - Bidirectional traversal (outgoing + incoming edges), edge path tracking for explainability
- [x] (2026-02-18) Python consumer: 2 new NATS handlers (`graph.build.request`, `graph.search.request`)
- [x] (2026-02-18) Go: GraphService following RetrievalService pattern (syncWaiter, health tracking, WS broadcasts)
- [x] (2026-02-18) Go: 4 NATS subjects + 5 payload types, 4 config fields (GraphEnabled, GraphMaxHops, GraphTopK, GraphHopDecay)
- [x] (2026-02-18) Go: Context optimizer `fetchGraphEntries()` uses retrieval hits as seed symbols
  - Priority: 70 - (distance * 10) → hop 0 = 70, hop 1 = 60, hop 2 = 50
- [x] (2026-02-18) Go: 3 HTTP endpoints, EntryGraph domain kind, WS GraphStatusEvent
- [x] (2026-02-18) Frontend: Graph types, API client (graph namespace), RetrievalPanel graph section

### Phase 6D Key Deliverables
- **New files:** 4 (016_graph_nodes_edges.sql, graphrag.py, test_graphrag.py, graph.go)
- **Modified files:** 14 (models.py, consumer.py, queue.go, schemas.go, config.go, loader.go, pack.go, context_optimizer.go, handlers.go, routes.go, events.go, main.go, types.ts, client.ts)
- **API:** 3 new REST endpoints (POST /graph/build, GET /graph/status, POST /graph/search)
- **Config:** 4 new env vars (CODEFORGE_ORCH_GRAPH_ENABLED, _MAX_HOPS, _TOP_K, _HOP_DECAY)
- **Tests:** 19 Python tests, all passing; Go compilation + all existing tests passing
- **Linting:** golangci-lint 0 issues, ruff 0 issues, ESLint 0 issues, pre-commit all hooks pass

## Phase 7: Cost & Token Transparency (COMPLETED)

> Priority: "It must always be very obvious what costs/tokens CodeForge causes with the LLMs."

- [x] (2026-02-18) **Feature 1: Real Cost Calculation** — Python workers extract cost from LiteLLM response header + fallback pricing table for local models
- [x] (2026-02-18) **Feature 2: Token Persistence** — `tokens_in`, `tokens_out`, `model` columns on runs table, Python token accumulators, Go/Python NATS payload updates
- [x] (2026-02-18) **Feature 3: Cost Aggregation API** — 5 new REST endpoints for cost breakdown by project, model, and time
- [x] (2026-02-18) **Feature 4: WS Budget Alerts** — `BudgetAlertEvent` at 80%/90% of MaxCost with dedup, token fields on `RunStatusEvent`
- [x] (2026-02-18) **Feature 5: Frontend Dashboard** — `/costs` page with global totals + project breakdown; per-project cost section with model breakdown, daily bars, recent runs; token/model display in RunPanel; budget alert banner

### Phase 7 Key Deliverables

- **New files:** 4 (cost domain, cost service, pricing table YAML, pricing module, cost dashboard)
- **Modified files:** ~20 (across Go, Python, TypeScript)
- **New REST endpoints:** 5 cost aggregation endpoints
- **New WS event:** `run.budget_alert`
- **Tests:** 7 Python pricing tests, 3 Go budget alert tests, all existing tests passing
- **Migration:** 015_add_run_tokens.sql

## Production Readiness: Worker Pools, Migration Rollback, Rate Limiter Hardening, Backup/DR (COMPLETED)

- [x] (2026-02-18) **Git Worker Pool** — `internal/git/pool.go` using `golang.org/x/sync/semaphore`, nil-safe Run(), injected into gitlocal.Provider, CheckpointService, DeliverService; config `git.max_concurrent` (default 5); 4 tests
- [x] (2026-02-18) **Migration Rollback Verification** — `RollbackMigrations()` + `MigrationVersion()` in postgres.go; integration test `TestMigrationUpDown` (up 15, down 15, re-up 15); `./scripts/test.sh migrations` sub-command
- [x] (2026-02-18) **Rate Limiter Hardening** — `StartCleanup()` background goroutine for stale bucket eviction; `Len()` for metrics; config `rate.cleanup_interval` (5m), `rate.max_idle_time` (10m); cleanup + len + timing tests; 2 benchmarks
- [x] (2026-02-18) **Backup & Disaster Recovery** — `scripts/backup-postgres.sh` (pg_dump custom format, retention cleanup), `scripts/restore-postgres.sh` (drop-recreate, latest mode); Docker Compose WAL config (wal_level=replica, archive_mode=on); documented in dev-setup.md

### Key Deliverables
- **New files (6):** pool.go, pool_test.go, migration_test.go, backup-postgres.sh, restore-postgres.sh
- **Modified files (~14):** config.go, loader.go, provider.go, checkpoint.go, deliver.go, main.go, ratelimit.go, ratelimit_test.go, postgres.go, test.sh, docker-compose.yml, .gitignore, codeforge.yaml.example, dev-setup.md
- **Tests:** 4 pool tests + 3 ratelimit tests + 2 benchmarks + 1 integration test

## Pre-Commit & Linting Hardening (COMPLETED)

- [x] (2026-02-18) Security analysis, anti-pattern detection, import sorting across all three languages
  - **Python (pyproject.toml):** Added 10 ruff rule groups — S (bandit security), C4 (unnecessary comprehensions), C90 (mccabe complexity, threshold 12), PERF (performance anti-patterns), PIE (misc anti-patterns), RET (return issues), FURB (modernization), LOG (logging best practices), T20 (print detection), PT (pytest style). Added RET504 to ignore, per-file-ignores for tests (S101, PT011, S108). Fixed C901 noqa, PERF401 list.extend, FURB110 ternary.
  - **Go (.golangci.yml):** Added 8 linters — gosec (security), bodyclose (HTTP response body), noctx (context-less HTTP), errorlint (error wrapping), revive (18 curated rules), fatcontext (loop context leak), dupword (comment typos), durationcheck (duration multiplication). Settings: gosec excludes G404/G115, errorlint errorf-only, file-specific exclusions for loader.go (G304) and deliver.go (G306). Test exclusions for noctx, gosec G306/G204/G301, bodyclose. Fixed permissions (0o644→0o600, 0o755→0o750), added nolint annotations.
  - **TypeScript (frontend/eslint.config.js):** Replaced `recommended` with `strict` + `stylistic` configs from typescript-eslint. Added `eslint-plugin-simple-import-sort` for import/export ordering. Fixed void→undefined in generic types, replaced non-null assertions with optional chaining and nullish coalescing.
  - **Pre-commit (.pre-commit-config.yaml):** Bumped ruff-pre-commit v0.11.12→v0.15.1. Fixed ESLint hook to use local binary (`frontend/node_modules/.bin/eslint`) with explicit config path and `^frontend/src/` file filter.
  - **31 files changed**, all 15 pre-commit hooks pass, 89 Python tests pass, all Go tests pass

## Documentation Debt (COMPLETED)

- [x] (2026-02-18) **ADR-003:** Config Hierarchy — three-tier (defaults < YAML < env vars), typed Config struct, validation
- [x] (2026-02-18) **ADR-004:** Async Logging — buffered channel (10K) + worker pool (4), non-blocking drops, graceful drain
- [x] (2026-02-18) **ADR-005:** Docker-Native Logging — json-file driver with rotation, no external monitoring stack, structured JSON + jq
- [x] (2026-02-18) **ADR-006:** Agent Execution Approach C — Go control plane (state/policies/sessions) + Python runtime (LLM/tools/loop)
- [x] (2026-02-18) **ADR-007:** Policy Layer — first-match-wins permission rules, 4 presets, quality gates, termination conditions
- [x] (2026-02-18) **architecture.md** — added Infrastructure Patterns section (reliability, performance, agent execution, observability)
- [x] (2026-02-18) **dev-setup.md** — added Logging section (log access, helper script, log levels, request ID, log rotation)
- [x] (2026-02-18) **CLAUDE.md** — added ADR index, Infrastructure Principles section (config, async, logging, policy, approach C, resilience)
