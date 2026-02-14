# CodeForge — Project Context

## Language Policy

- **All project documentation, code comments, commit messages, and configs are written in English only.**
- **Project-specific memories and decisions are stored in this file (CLAUDE.md).**

## What is CodeForge?

Containerized service for orchestrating AI coding agents with a web GUI.

### Four Core Pillars:
1. **Project Dashboard** — Management of multiple repos (Git, GitHub, GitLab, SVN, local)
2. **Roadmap/Feature-Map** — Visual management, compatible with OpenSpec, bidirectional sync to repo specs
3. **Multi-LLM-Provider** — OpenAI, Claude, local models (Ollama/LM Studio), routing via LiteLLM
4. **Agent Orchestration** — Coordination of various coding agents (Aider, OpenHands, SWE-agent, etc.)

## Architecture

Three-layer hybrid stack:

```
TypeScript Frontend (SolidJS)
        |
        v  REST / WebSocket
Go Core Service (HTTP, WebSocket, Agent Lifecycle, Repo Management, Scheduling)
        |
        v  Message Queue (NATS JetStream)
Python AI Workers (LLM Calls, Agent Execution, LiteLLM, LangGraph)
```

## Tech Stack

| Layer          | Language   | Purpose                                  |
|----------------|------------|------------------------------------------|
| Frontend       | TypeScript | Web GUI                                  |
| Core Service   | Go 1.23    | HTTP/WS Server, Scheduling, Repo Mgmt   |
| AI Workers     | Python 3.12| LLM Integration, Agent Execution         |
| Infrastructure | Docker     | Containerization, Docker-in-Docker       |

## Configuration Format

- **YAML for all configuration files** — no exceptions
- Reason: YAML supports comments (JSON does not)
- Applies to: Modes, Tool Bundles, Project Settings, Safety Rules, Autonomy, Schedules
- JSON only for: API responses, event serialization, internal data exchange

## Tooling

- **Python:** Poetry, Ruff (Linting + Formatting), Pytest
- **Go:** golangci-lint, gofmt, goimports
- **TypeScript:** ESLint, Prettier
- **All:** pre-commit hooks (.pre-commit-config.yaml), Docker Compose

## Market Positioning

The specific combination of Project Dashboard + Roadmap + Multi-LLM + Agent Orchestration does not exist.
Closest competitor: OpenHands (no Roadmap, no Multi-Project Dashboard, no SVN).
Detailed analysis: docs/research/market-analysis.md

## Software Architecture

- **Hexagonal Architecture (Ports & Adapters)** for the Go Core
- **Provider Registry Pattern** for open-source extensibility (self-registering via `init()`)
- **Capabilities** instead of mandatory implementation — each provider declares what it can do
- **Compliance Tests** per interface — new adapters automatically inherit the test suite
- **LLM Capability Levels** — Workers supplement missing capabilities depending on the LLM:
  - Full-featured agents (Claude Code, Aider, OpenHands): orchestration only
  - API with tools (OpenAI, Claude API, Gemini): + Context Layer (GraphRAG) + Routing + Tool Definitions
  - Pure completion (Ollama, LM Studio): + everything (Context, Tools, Prompt Engineering, Quality Layer)
- **Worker Modules:** Context (GraphRAG), Quality (Debate/Reviewer/Sampler/Guardrail),
  Routing, Safety, Execution, Memory, History, Events, Orchestration, Hooks, Trajectory, HITL
- **Agent Execution Modes:** Sandbox (isolated container), Mount (direct file access), Hybrid
- **Safety Layer (8 components):** Budget Limiter, Command Safety Evaluator, Branch Isolation,
  Test/Lint Gate, Max Steps, Rollback, Path Blocklist, Stall Detection
- **Agent Workflow:** Plan → Approve → Execute → Review → Deliver (configurable)
- **Autonomy Spectrum (5 Levels):**
  - Level 1 `supervised`: User approves everything
  - Level 2 `semi-auto`: User approves only destructive actions (delete, terminal, deploy)
  - Level 3 `auto-edit`: User approves only terminal/deploy
  - Level 4 `full-auto`: Safety rules replace user (budget, tests, blocklists)
  - Level 5 `headless`: Fully autonomous, no UI needed (CI/CD, cron jobs, API)
- **Modes System (Agent Specialization):**
  - YAML-configurable agent roles (architect, coder, reviewer, debugger, etc.)
  - Each mode: own tools, LLM scenario, autonomy level, prompt template
  - Built-in modes + custom modes (user-defined in `.codeforge/modes/`)
  - Mode pipelines and DAG composition for multi-agent workflows
  - Schedule support for autonomous cron jobs (headless)
- **YAML-based Tool Bundles:** Declarative tool definitions, no code needed
- **History Processors:** Context window optimization as pipeline
- **Hook System:** Observer pattern for agent/environment lifecycle
- **Trajectory Recording:** Recording, replay, inspector, audit trail
- **Cost Management:** Budget limits per task/project/user, auto-tracking
- **Jinja2 Prompt Templates:** Prompts in separate files, not in code
- **KeyBERT Keyword Extraction:** Semantic keywords for better retrieval
- **Real-time State via WebSocket:** Live updates for agent status, logs, costs
- **Frontend:** SolidJS + Tailwind CSS
- **Framework Insights (LangGraph, CrewAI, AutoGen, MetaGPT):**
  - Composite Memory Scoring (Semantic + Recency + Importance)
  - Context Window Strategies (Buffered, TokenLimited, HeadAndTail)
  - Experience Pool (@exp_cache) for caching successful runs
  - Tool Recommendation via BM25 (automatic tool selection)
  - Workbench (tool container with shared state, MCP integration)
  - LLM Guardrail Agent (agent validates agent output)
  - Structured Output / ActionNode (schema validation + review/revise)
  - Event Bus for observability (Agent/Task/System Events → WebSocket)
  - GraphFlow / DAG Orchestration (Conditional Edges, Parallel Nodes, Cycles)
  - Composable Termination Conditions (MaxSteps | Budget | Timeout)
  - Component System (Agents/Tools/Workflows as JSON serializable, GUI editor)
  - Document Pipeline PRD→Design→Tasks→Code (reduces hallucination)
  - MagenticOne Planning Loop (Stall Detection + Re-Planning)
  - HandoffMessage Pattern (agent handoff between specialists)
  - Human Feedback Provider Protocol (Web GUI, Slack, Email extensible)
- **Coding Agent Insights (Cline, Devika):**
  - Plan/Act Mode Pattern (separate LLM configs per phase, user toggle)
  - Shadow Git Checkpoints (isolated git repo for rollback)
  - Ask/Say Approval Pattern (granular permissions per tool category)
  - MCP as standard extensibility protocol for tools
  - .clinerules-like project configuration (YAML-based)
  - Auto-Compact Context Management (summarization at ~80% window)
  - Diff-based File Review (side-by-side before approval)
  - Sub-Agent Architecture (planner/researcher/coder separation)
  - Agent State Visualization (internal monologue, steps, browser, terminal)
  - LLM-driven Web Crawler (page content → LLM → action loop)
  - Stateless Agent Design (state in core, not in agents)
- **Coding Agent Insights (OpenHands, SWE-agent):**
  - Event-Sourcing Architecture (EventStream as central abstraction)
  - Workspace Abstraction (Local/Docker/Remote, self-healing containers)
  - AgentHub with specialized agents (CodeAct, Browsing, Delegator, Microagents)
  - Microagents: YAML+Markdown-based trigger-driven special agents
  - Skills System (reusable Python snippets, automatically in prompt)
  - Risk Management with LLMSecurityAnalyzer (InvariantAnalyzer)
  - V0→V1 SDK Migration Pattern (AgentSkills as MCP server)
  - RouterLLM for local routing decisions (OpenRouter as fallback)
  - ACI (Agent-Computer Interface): Shell commands optimized for LLMs
  - Tool Bundles (YAML): Declarative, swappable tool definitions
  - History Processors: Pipeline for context window optimization
  - SWE-ReX Sandbox: Docker-based remote execution
  - Mini-SWE-Agent Pattern: 100 lines of Python, 74% SWE-bench
  - ToolFilterConfig: Blocklist + conditional blocking for command safety
- **Extended Competitor Analysis (12 new tools):**
  - Codel (Go+React, Docker Sandbox, AGPL-3.0) — architecture reference
  - CLI Agent Orchestrator (AWS, Supervisor/Worker, tmux/MCP) — closest competitor
  - Goose (Rust, MCP-native, 30k+ stars, Apache 2.0) — backend candidate
  - OpenCode (Go, Client/Server, LSP, MIT) — backend candidate
  - Plandex (Go, Planning-First, Diff Sandbox, MIT) — backend candidate
  - Roo Code (Modes System, Cloud Agents, Apache 2.0) — pattern reference
  - Codex CLI (OpenAI, Multimodal, GitHub Action, Apache 2.0) — backend candidate
  - SERA (Ai2, Open Model Weights, $400 Training, Apache 2.0) — self-hosted model
  - bolt.diy (19k stars, 19+ providers, MIT) — multi-LLM reference
  - AutoForge (Two-Agent, Test-First, Multi-Session) — workflow pattern
  - Dyad (Local-First, Apache 2.0) — UX reference
  - AutoCodeRover (AST-aware, GPL-3.0, $0.70/task) — niche agent
- **Roadmap/Feature-Map Auto-Detection & Adaptive Integration:**
  - **No custom PM tool** — sync with existing tools (Plane, OpenProject, GitHub/GitLab Issues)
  - **Auto-Detection:** Three-tier detection (repo files → platform APIs → file markers)
  - **Multi-Format SDD Support:** OpenSpec (`openspec/`), Spec Kit (`.specify/`), Autospec (`specs/spec.yaml`)
  - **Provider Registry:** `specprovider` (repo specs) + `pmprovider` (PM platforms), same architecture as Git
  - **Bidirectional Sync:** CodeForge ↔ PM Tool ↔ Repo Specs, Webhook/Poll/Manual
  - **Adopted Patterns:** Plane (Cursor Pagination, HMAC-SHA256, Label Sync), OpenProject (Optimistic Locking, Schema Endpoints), OpenSpec (Delta Spec Format), Ploi Roadmap (`/ai` endpoint)
  - **Gitea/Forgejo:** GitHub adapter works with minimal changes (compatible API)
  - Detailed analysis: docs/research/market-analysis.md Section 5
- **Database: PostgreSQL 17** (shared instance with LiteLLM, schema separation)
  - Go: pgx v5 (driver) + goose (migrations)
  - Python: psycopg3 (sync+async)
  - NATS JetStream KV for ephemeral state (heartbeats, locks)
  - ADR: docs/architecture/adr/002-postgresql-database.md
- **Go Libraries (zero-dep principle):**
  - HTTP Router: chi v5 (zero deps, 100% net/http compatible, route groups + middleware)
  - WebSocket: coder/websocket v1.8+ (zero deps, context-native, concurrent-write-safe)
  - Git: os/exec wrapper around git CLI (zero deps, 100% feature coverage, native speed)
  - NOT used: Echo/Fiber (framework coupling), gorilla/websocket (no context, panic on concurrent writes), go-git (28 deps, 4-9x slower)
- **Frontend Libraries (minimal-stack principle):**
  - Routing: @solidjs/router (only viable SolidJS router)
  - Styling: Tailwind CSS directly (no component library, no CSS-in-JS)
  - WebSocket: @solid-primitives/websocket (728 bytes, auto-reconnect)
  - HTTP: native fetch API + thin wrapper (~30-50 LOC)
  - State: SolidJS built-in signals/stores/context (no external state library)
  - Icons: lucide-solid (tree-shakeable, direct imports)
  - NOT used: axios, styled-components, Kobalte, shadcn-solid, Socket.IO, Redux/Zustand
- **Protocol Support (MCP, LSP, A2A, AG-UI, OpenTelemetry):**
  - **MCP** (Model Context Protocol): Agent ↔ Tool communication (JSON-RPC, Anthropic standard)
    - Go Core: MCP server (expose tools) + MCP client registry (connect external tools)
    - Python Workers: MCP for agent tool access
  - **LSP** (Language Server Protocol): Code intelligence for agents (go-to-definition, refs, diagnostics)
    - Go Core manages LSP server lifecycle per project language
  - **OpenTelemetry GenAI**: Standardized LLM/agent observability (traces, metrics, events)
    - LiteLLM exports OTEL traces natively, Go Core adds agent lifecycle spans
  - **A2A** (Agent-to-Agent Protocol, Phase 2-3): Peer-to-peer agent coordination (Google → Linux Foundation)
    - Agent backends register as A2A agents with capability cards
  - **AG-UI** (Agent-User Interaction Protocol, Phase 2-3): Bi-directional agent ↔ frontend streaming (CopilotKit)
    - Frontend WebSocket follows AG-UI event format (TEXT_MESSAGE, TOOL_CALL, STATE_DELTA)
  - **Future/Watch:** ANP (decentralized agent networking), LSAP (LSP for AI agents)
- **LLM Integration (LiteLLM, OpenRouter, Claude Code Router, OpenCode CLI):**
  - **No custom LLM provider interface** — LiteLLM Proxy as Docker sidecar (port 4000)
  - Go Core + Python Workers communicate via OpenAI-compatible API against LiteLLM
  - Scenario-based routing via LiteLLM Tags (default/background/think/longContext/review/plan)
  - OpenRouter as optional provider behind LiteLLM
  - GitHub Copilot Token Exchange as provider (Go Core)
  - Local Model Auto-Discovery (Ollama/LM Studio `/v1/models`)
  - LiteLLM Config Manager, User Key Mapping, Cost Dashboard as custom development
- Detailed description: docs/architecture.md
- Framework comparison: docs/research/market-analysis.md

## Strategic Principles

- Leverage existing building blocks (LiteLLM, OpenSpec, Aider/OpenHands as backends)
- Do not reinvent the wheel for individual components
- Differentiation through integration of all four pillars
- Performance focus: Go for core, Python only for AI-specific work

## Coding Principles

- **Minimal dependencies**: Use only as many libraries as strictly necessary. Most libraries bring too much overhead. Prefer the standard library when it covers 80%+ of the need.
- **Readable code is documentation**: Write clean, self-explanatory code. Well-written code documents itself. Add comments only where the "why" is not obvious from the code.
- **DRY / Reusable**: Extract repeating patterns into reusable functions, types, or packages. Avoid copy-paste code. But: don't abstract prematurely — three similar occurrences justify extraction.
- **Simple over clever**: Prefer straightforward solutions over clever tricks. The next developer (or LLM agent) must understand the code immediately.
- **Minimal surface area**: Keep APIs, interfaces, and exported types as small as possible. Start private, export only when needed.

## Git Workflow

- **Commits only on `staging`** — never directly on `main`, unless the user explicitly instructs otherwise
- **Branch strategy:** Development on `staging`, merge to `main` only on instruction
- **All commit messages, documentation, code comments, and configuration descriptions must be in English**
- **Always push to remote after committing** — run `git push` after every successful commit so the remote stays in sync
- **Before each commit — checklist:**
  1. Run `pre-commit run --all-files` and fix errors
  2. Update affected documentation (see Documentation Policy below):
     - `docs/todo.md` — mark completed tasks `[x]`, add new tasks discovered during work
     - `docs/architecture.md` — for architecture or structural changes
     - `docs/features/*.md` — for feature-specific changes (scope, design, API, TODOs)
     - `docs/dev-setup.md` — for new directories, ports, tooling, environment variables
     - `docs/tech-stack.md` — for new dependencies, language/tool versions
     - `docs/project-status.md` — check off completed items, add new items
     - `CLAUDE.md` — for changes to core pillars, architecture, workflow rules

## Documentation Policy

**Every change must be documented. Documentation is as important as code.**

### Documentation Structure

```
docs/
├── README.md                        # Documentation index (start here)
├── todo.md                          # Central TODO tracker for LLM agents
├── architecture.md                  # System architecture overview
├── dev-setup.md                     # Development setup guide
├── project-status.md                # Phase tracking & milestones
├── tech-stack.md                    # Languages, tools, dependencies
├── features/                        # Feature specifications (one per pillar)
│   ├── 01-project-dashboard.md      # Pillar 1: Multi-repo management
│   ├── 02-roadmap-feature-map.md    # Pillar 2: Visual roadmap, specs, PM sync
│   ├── 03-multi-llm-provider.md     # Pillar 3: LiteLLM, routing, cost tracking
│   └── 04-agent-orchestration.md    # Pillar 4: Agent modes, execution, safety
├── architecture/                    # Detailed architecture documents
│   └── adr/                         # Architecture Decision Records
└── research/                        # Market research & analysis
```

### TODO Tracking Rules

- **`docs/todo.md` is the single source of truth** for what needs to be done
- LLM agents must read `docs/todo.md` before starting any work
- After completing a task: mark it `[x]` in `docs/todo.md` with the date
- After discovering new work: add it to `docs/todo.md` in the appropriate section
- Feature-specific TODOs also live in `docs/features/*.md` but must be cross-referenced in `docs/todo.md`
- `docs/project-status.md` tracks high-level phase completion; `docs/todo.md` tracks granular tasks

### When to Update Which Document

| Change Type | Update These Files |
|---|---|
| New feature work | `docs/features/*.md` (scope, design, TODOs), `docs/todo.md` |
| Architecture decision | `docs/architecture.md`, `docs/architecture/adr/`, `CLAUDE.md` |
| New dependency/tool | `docs/tech-stack.md`, `docs/dev-setup.md` |
| Completed milestone | `docs/project-status.md`, `docs/todo.md` |
| New directory/port/env var | `docs/dev-setup.md` |
| Core pillar changes | `CLAUDE.md`, relevant `docs/features/*.md` |
| Any code change | `docs/todo.md` (mark task done or add new tasks) |

### Feature Documentation Rules

- Each of the four pillars has its own feature spec in `docs/features/`
- New sub-features are added as sections within the relevant pillar doc
- Feature docs contain: overview, design decisions, API endpoints, TODOs
- Feature-specific TODOs are cross-referenced in `docs/todo.md`

### Architecture Decision Records (ADRs)

- Major architectural decisions get an ADR in `docs/architecture/adr/`
- Use `docs/architecture/adr/_template.md` as the starting point
- ADR format: Context → Decision → Consequences → Alternatives
