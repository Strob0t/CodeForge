# CodeForge — Project Status

> Last update: 2026-02-14

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
- **Go:** 1.24, chi v5, pgx v5, goose, coder/websocket, nats.go — 0 lint issues, 11 tests
- **Python:** nats-py, httpx, pydantic v2 — 16 tests, ruff clean
- **Frontend:** SolidJS, @solidjs/router, @solid-primitives/websocket — build + lint + format clean
- **CI:** 3-job GitHub Actions (Go, Python, Frontend)
- **API:** 9 REST endpoints, WebSocket, health with service status

## Phase 2: MVP Features

- [ ] Project management (add/remove repos, display status)
- [ ] Git integration (Clone, Pull, Branch, Diff)
- [ ] LLM provider management (API keys, model selection)
- [ ] Simple agent execution (single task to single agent)
- [ ] Basic Web GUI for all features above

## Phase 3: Advanced Features

- [ ] Roadmap/Feature Map Editor (Auto-Detection, Multi-Format SDD, bidirectional PM sync)
- [ ] OpenSpec/Spec Kit/Autospec integration
- [ ] SVN integration
- [ ] Multi-agent orchestration
- [ ] GitHub/GitLab Webhook integration
- [ ] Cost tracking for LLM usage
- [ ] Multi-tenancy / user management
