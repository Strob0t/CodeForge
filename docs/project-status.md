# CodeForge — Project Status

> Last update: 2026-02-17

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
