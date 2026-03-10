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
| Core Service   | Go 1.25    | HTTP/WS Server, Scheduling, Repo Mgmt   |
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
- **Go text/template Prompt Templates:** Prompts in separate `.tmpl` files via `//go:embed`, not in code
- **BM25S Keyword Retrieval:** BM25-based retrieval for code search and tool recommendation
- **Real-time State via WebSocket:** Live updates for agent status, logs, costs
- **Frontend:** SolidJS + Tailwind CSS
- **Agentic Conversation Loop (Phase 17):**
  - Chat UI triggers multi-turn tool-use loop (LLM → tools → results → repeat until done)
  - Go Core dispatches to Python Worker via NATS (`conversation.run.start/complete`)
  - 7 built-in tools: Read, Write, Edit, Bash, Search, Glob, ListDir + MCP tool merge
  - `AgentLoopExecutor` (Python): streaming LLM calls, per-tool policy enforcement, cost tracking
  - `ConversationHistoryManager` (Python): head-and-tail token budget, tool result truncation
  - HITL approval: `DecisionAsk` → WS `permission_request` → HTTP approve/deny → channel resume
  - Config: `Agent.MaxLoopIterations` (50), `Agent.MaxContextTokens` (120K), `Agent.ContextEnabled` (true), `Agent.ContextBudget` (2048), `Agent.ContextPromptReserve` (512), `Runtime.ApprovalTimeoutSeconds` (60)
  - **Adaptive Context Budget:** Budget decays linearly from `ContextBudget` to 0 over 60 history messages (`AdaptiveContextBudget()` in `internal/service/context_budget.go`). Early turns get full context; later turns get less as the agent builds its own context through tool calls.
  - **Auto-Indexing:** Clone, Adopt, and Setup handlers auto-trigger RepoMap + Retrieval Index + GraphRAG build (`autoIndexProject()` in `internal/adapter/http/handlers.go`)
  - Key files: `workers/codeforge/agent_loop.py`, `workers/codeforge/tools/`, `internal/service/conversation.go`
- **Chat Enhancements:** — **implemented**
  - HITL Permission UI: `PermissionRequestCard` with approve/deny, countdown, `supervised-ask-all` preset, autonomy-to-preset mapping
  - Inline Diff Review: `DiffPreview` component for write/edit tool calls
  - Action Buttons: copy, retry, apply, view diff on agent messages
  - Per-Message Cost: `MessageBadge` + `CostBreakdown` from AG-UI `state_delta`
  - Smart References: `@/#//` autocomplete, `AutocompletePopover`, `useFrequencyTracker`
  - Slash Commands: `/compact`, `/rewind`, `/clear`, `/help`, `/mode`, `/model` with `CommandRegistry`
  - Conversation Search: PostgreSQL FTS (GIN index, `ts_rank`), `POST /search/conversations`
  - Notification Center: `notificationStore`, browser push, Web Audio sounds, tab badge, AG-UI wiring
  - Real-Time Channels: 3 tables, 9 endpoints, WebSocket events, `ChannelList`, `ChannelView`, `ThreadPanel`
  - Feature spec: `docs/features/05-chat-enhancements.md`
- **Framework Insights (LangGraph, CrewAI, AutoGen, MetaGPT):**
  > Reference patterns from framework analysis.
  - Composite Memory Scoring (Semantic + Recency + Importance) — `workers/codeforge/memory/scorer.py`, `internal/service/memory.go`
  - Context Window Strategies (Buffered, TokenLimited, HeadAndTail)
  - Experience Pool (@exp_cache) for caching successful runs — `workers/codeforge/memory/experience.py`, `internal/service/experience_pool.go`
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
  - HandoffMessage Pattern (agent handoff between specialists) — `internal/domain/orchestration/handoff.go`, `internal/service/handoff.go`, `workers/codeforge/tools/handoff.py`
  - Human Feedback Provider Protocol (Web GUI, Slack, Email extensible) — `internal/port/feedback/provider.go`, `internal/adapter/slack/feedback.go`, `internal/adapter/email/feedback.go`
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
  - Microagents: YAML+Markdown-based trigger-driven special agents — `internal/domain/microagent/`, `internal/service/microagent.go`
  - Skills System (reusable Python snippets, automatically in prompt) — `workers/codeforge/skills/`, `internal/service/skill.go`
  - Risk Management with LLMSecurityAnalyzer (InvariantAnalyzer)
  - V0→V1 SDK Migration Pattern (AgentSkills as MCP server)
  - RouterLLM scenario wiring via LiteLLM tag-based routing — `internal/service/conversation.go`, `workers/codeforge/consumer.py`
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
  - AI Maestro (Next.js/Node.js, Peer Mesh, AMP Protocol, CozoDB) — multi-machine agent orchestration, War Room UX, persistent agent identity. Patterns extracted: trust annotations, quarantine, agent identity, War Room (Phase 23)
  - AMP (Agent Messaging Protocol v0.1.2-draft, Apache 2.0, 23blocks) — secure inter-agent messaging with Ed25519 signatures, trust annotations, federation. Too immature for adoption; patterns extracted into Phase 23
- **Security & Trust Infrastructure (Phase 23):** — **implemented**
  - Trust Annotations: 4 trust levels (untrusted, partial, verified, full), auto-stamped on NATS payloads — `internal/domain/trust/`
  - Message Quarantine: risk scoring, admin review hold, Evaluate/Approve/Reject — `internal/service/quarantine.go`, migration 049
  - Persistent Agent Identity: fingerprint, stats accumulation, inbox — `internal/domain/agent/agent.go`
  - War Room: live multi-agent collaboration view — `frontend/src/features/project/WarRoom.tsx`
- **Benchmark & Evaluation System (Phase 26 + 28):** — **implemented**
  - Phase 26: Provider interface pattern, evaluator plugins (LLMJudge, FunctionalTest, SPARC), 3 runner types, external providers (HumanEval, MBPP, SWE-bench)
  - Phase 28 (R2E-Gym/EntroPO): Hybrid verification pipeline, trajectory verifier, multi-rollout scaling, diversity-aware MAB (entropy-UCB1), DPO export, SWE-GEN synthetic tasks — `workers/codeforge/evaluation/`
- **Roadmap/Feature-Map Auto-Detection & Adaptive Integration:**
  - **No custom PM tool** — sync with existing tools (Plane, OpenProject, GitHub/GitLab Issues)
  - **Auto-Detection:** Three-tier detection (repo files → platform APIs → file markers)
  - **Multi-Format SDD Support:** OpenSpec (`openspec/`), Spec Kit (`.specify/`), Autospec (`specs/spec.yaml`)
  - **Provider Registry:** `specprovider` (repo specs) + `pmprovider` (PM platforms), same architecture as Git
  - **Bidirectional Sync:** CodeForge ↔ PM Tool ↔ Repo Specs, Webhook/Poll/Manual
  - **Adopted Patterns:** Plane (Cursor Pagination, HMAC-SHA256, Label Sync), OpenProject (Optimistic Locking, Schema Endpoints), OpenSpec (Delta Spec Format), Ploi Roadmap (`/ai` endpoint)
  - **Gitea/Forgejo:** GitHub adapter works with minimal changes (compatible API)
  - Detailed analysis: docs/research/market-analysis.md Section 5
- **Database: PostgreSQL 18** (shared instance with LiteLLM, schema separation)
  - Go: pgx v5 (driver) + goose (migrations)
  - Python: psycopg3 (sync+async)
  - NATS JetStream KV for ephemeral state (heartbeats, locks)
  - ADR: docs/architecture/adr/002-postgresql-database.md
- **Go Libraries (minimal-dep principle):**
  - HTTP Router: chi v5 (minimal deps, 100% net/http compatible, route groups + middleware)
  - WebSocket: coder/websocket v1.8+ (minimal deps, context-native, concurrent-write-safe)
  - Git: os/exec wrapper around git CLI (zero deps, 100% feature coverage, native speed)
  - NOT used: Echo/Fiber (framework coupling), gorilla/websocket (no context, panic on concurrent writes), go-git (28 deps, 4-9x slower)
- **Frontend Libraries (minimal-stack principle):**
  - Routing: @solidjs/router (only viable SolidJS router)
  - Styling: Tailwind CSS directly (no component library, no CSS-in-JS)
  - WebSocket: @solid-primitives/websocket (auto-reconnect)
  - HTTP: native fetch API + thin wrapper (~30-50 LOC)
  - State: SolidJS built-in signals/stores/context (no external state library)
  - Icons: Unicode symbols + inline SVG (no icon library dependency)
  - NOT used: axios, styled-components, Kobalte, shadcn-solid, Socket.IO, Redux/Zustand
- **Protocol Support (MCP, LSP, A2A, AG-UI, OpenTelemetry):**
  - **MCP** (Model Context Protocol): Agent ↔ Tool communication (JSON-RPC, Anthropic standard) — **implemented (Phase 15)**
    - Go Core: MCP server via mcp-go SDK (expose tools: list_projects, get_project, get_run_status, get_cost_summary; resources: codeforge://projects, codeforge://costs/summary; auth middleware)
    - Go Core: MCP server registry with PostgreSQL persistence, project-level assignment, HTTP CRUD API
    - Python Workers: McpWorkbench (multi-server container, BM25 tool recommendation, tool discovery, tool call bridging)
    - Frontend: MCPServersPage (server list, add/edit, test connection, tools discovery)
    - Config: `mcp.enabled`, `mcp.servers_dir`, `mcp.server_port` (default 3001)
    - Policy: `mcp:server:tool` namespaced tool calls with glob matching
  - **LSP** (Language Server Protocol): Code intelligence for agents (go-to-definition, refs, diagnostics) — **implemented (Phase 15D)**
    - Go Core manages LSP server lifecycle per project language
  - **OpenTelemetry GenAI**: Standardized LLM/agent observability (traces, metrics, events)
    - LiteLLM exports OTEL traces natively, Go Core adds agent lifecycle spans
  - **A2A** (Agent-to-Agent Protocol v0.3.0, Apache 2.0, Linux Foundation, 50+ partners) — **implemented (Phase 27)**
    - Spec: Protobuf-defined (`lf.a2a.v1`), JSON-RPC 2.0 over HTTPS, SSE streaming, push notifications (webhooks)
    - 11 RPCs: SendMessage, SendStreamingMessage, GetTask, ListTasks, CancelTask, SubscribeToTask, GetExtendedAgentCard, CRUD PushNotificationConfig
    - Task lifecycle (8 states): SUBMITTED → WORKING → COMPLETED/FAILED/CANCELED/REJECTED, interrupted: INPUT_REQUIRED/AUTH_REQUIRED
    - Core types: AgentCard (self-describing manifest with skills, capabilities, security), Task, Message (USER/AGENT roles), Part (text/binary/URL/JSON + MIME), Artifact
    - Security: OpenAPI 3.2 model — API Key, HTTP Auth (Bearer/JWT), OAuth 2.0 (AuthCode+PKCE, ClientCredentials, DeviceCode), OpenID Connect, mTLS, JWS-signed AgentCards
    - Multi-tenant: built-in `/{tenant}/` path prefix
    - Official SDKs: Go (`a2a-go`), Python, JS (`@a2a-js/sdk`), Java, .NET
    - Complementary to MCP: "MCP for tools, A2A for agents"
    - CodeForge integration: Agent backends as A2A servers (AgentCard → Provider Registry), Go Core as A2A client via `a2a-go`, NATS for internal + A2A for external federation
    - Partners: Atlassian, Salesforce, PayPal, Cohere, and others
  - **AG-UI** (Agent-User Interaction Protocol): Bi-directional agent ↔ frontend streaming (CopilotKit) — **implemented (Phase 17+)**
    - 8 event types: run_started, run_finished, text_message, tool_call, tool_result, state_delta, step_started, step_finished
    - Frontend WebSocket follows AG-UI event format across Go and TypeScript (20+ files)
  - **Future/Watch:** ANP (decentralized agent networking), LSAP (LSP for AI agents)
- **LLM Integration (LiteLLM, OpenRouter, Claude Code Router, OpenCode CLI):**
  - **No custom LLM provider interface** — LiteLLM Proxy as Docker sidecar (port 4000)
  - Go Core + Python Workers communicate via OpenAI-compatible API against LiteLLM
  - **Hybrid Intelligent Routing (Phase 29):** Three-layer cascade — ComplexityAnalyzer (rule-based, <1ms) -> MABModelSelector (UCB1 learning) -> LLMMetaRouter (cold-start fallback). Enable with `CODEFORGE_ROUTING_ENABLED=true`. Package: `workers/codeforge/routing/`
  - LiteLLM config uses provider-level wildcards (`openai/*`, `anthropic/*`, etc.) — HybridRouter selects exact model name
  - Scenario-based tag routing (default/background/think/longContext/review/plan) as fallback when routing disabled
  - OpenRouter as optional provider behind LiteLLM
  - GitHub Copilot Token Exchange as provider (Go Core) — `internal/adapter/copilot/client.go`, `POST /api/v1/copilot/exchange`
  - Local Model Auto-Discovery (Ollama/LM Studio `/v1/models`)
  - LiteLLM Config Manager, User Key Mapping, Cost Dashboard as custom development
- Detailed description: docs/architecture.md
- Framework comparison: docs/research/market-analysis.md

## Strategic Principles

- Leverage existing building blocks (LiteLLM, OpenSpec, Aider/OpenHands as backends)
- Do not reinvent the wheel for individual components
- Differentiation through integration of all four pillars
- Performance focus: Go for core, Python only for AI-specific work

## Architectural Decisions (ADRs)

- **ADR-001:** NATS JetStream as message queue (docs/architecture/adr/001-nats-jetstream-message-queue.md)
- **ADR-002:** PostgreSQL 18 as primary database (docs/architecture/adr/002-postgresql-database.md)
- **ADR-003:** Hierarchical config: defaults < YAML < env vars < CLI flags (docs/architecture/adr/003-config-hierarchy.md)
- **ADR-004:** Async logging with buffered channel + worker pool (docs/architecture/adr/004-async-logging.md)
- **ADR-005:** Docker-native logging, no external monitoring stack (docs/architecture/adr/005-docker-native-logging.md)
- **ADR-006:** Agent execution Approach C: Go control plane + Python runtime (docs/architecture/adr/006-agent-execution-approach-c.md)
- **ADR-007:** Policy layer: first-match-wins permission rules (docs/architecture/adr/007-policy-layer.md)
- **ADR-008:** Benchmark evaluation framework: DeepEval + AgentNeo + GEMMAS (docs/architecture/adr/008-benchmark-evaluation-framework.md)

## Infrastructure Principles

- **Config hierarchy:** defaults < YAML < env vars < CLI flags. The system must run with zero configuration. CLI flags have the highest precedence.
- **Async-first concurrency:** Logging, NATS publishing, and LLM calls must never block the hot path. Use buffered channels + worker pools (Go) or QueueHandler + QueueListener (Python).
- **Docker-native logging:** No external monitoring stack (no ELK, no Loki, no Grafana). All services write structured JSON to stdout. Docker's json-file driver handles rotation. Use `docker compose logs` + `jq` for debugging.
- **Policy Layer:** Agent tool calls are governed by declarative YAML policies with first-match-wins evaluation. 4 built-in presets cover common security profiles. Custom policies extend without code changes.
- **Approach C (Go Control Plane + Python Runtime):** Go Core owns state, policies, and sessions. Python Workers own LLM calls, tool execution, and the agent loop. Communication via NATS with per-tool-call policy enforcement.
- **Resilience patterns:** Circuit breakers on external calls (NATS, LiteLLM). Idempotency keys for mutating HTTP operations. Dead letter queues for failed NATS messages. Graceful 4-phase shutdown.

## Character Encoding

- **Config files (.env, .yaml, .toml, .json, .sh, .gitignore) must use ASCII only** — no box-drawing characters (─, ═, │, etc.)
- Use regular dashes `-` and equals `=` for section separators in config files
- Use Mermaid diagram blocks (` ```mermaid `) for architecture diagrams in documentation

## Coding Principles

- **Strict type safety**: Never use `any` (TypeScript), `interface{}` / `any` (Go), or `Any` (Python) unless there is absolutely no alternative. All function signatures, return types, and variables must be explicitly typed. Use generics, union types, or specific interfaces instead.
- **Minimal dependencies**: Use only as many libraries as strictly necessary. Most libraries bring too much overhead. Prefer the standard library when it covers 80%+ of the need.
- **Readable code is documentation**: Write clean, self-explanatory code. Well-written code documents itself. Add comments only where the "why" is not obvious from the code.
- **DRY / Reusable**: Extract repeating patterns into reusable functions, types, or packages. Avoid copy-paste code. But: don't abstract prematurely — three similar occurrences justify extraction.
- **Simple over clever**: Prefer straightforward solutions over clever tricks. The next developer (or LLM agent) must understand the code immediately.
- **Minimal surface area**: Keep APIs, interfaces, and exported types as small as possible. Start private, export only when needed.

### The Zen of Python (PEP 20) — applies to ALL languages, not just Python

These principles guide all code in this project across Go, Python, and TypeScript:

- Beautiful is better than ugly.
- Explicit is better than implicit.
- Simple is better than complex.
- Complex is better than complicated.
- Flat is better than nested.
- Sparse is better than dense.
- Readability counts.
- Special cases aren't special enough to break the rules.
- Although practicality beats purity.
- Errors should never pass silently.
- Unless explicitly silenced.
- In the face of ambiguity, refuse the temptation to guess.
- There should be one-- and preferably only one --obvious way to do it.
- Now is better than never.
- Although never is often better than *right* now.
- If the implementation is hard to explain, it's a bad idea.
- If the implementation is easy to explain, it may be a good idea.
- Namespaces are one honking great idea -- let's do more of those!

## Cross-Language Integration Checklist (Go / NATS / Python)

When modifying code that crosses the Go/Python boundary via NATS, verify ALL of the following:

### NATS Subjects & Streams
- Subject strings must match EXACTLY between Go constants (`internal/port/messagequeue/subjects.go`) and Python constants (`workers/codeforge/consumer/_subjects.py`)
- The NATS JetStream stream config (`internal/port/messagequeue/jetstream.go`) must include wildcard patterns for any new subject prefixes (e.g. `benchmark.>`)
- New subjects need both a publisher (one side) and a subscriber (other side)

### JSON Payload Contracts
- Go struct JSON tags (`json:"field_name"`) must match Python Pydantic field names exactly
- When adding/renaming a field in Go (`internal/port/messagequeue/schemas.go` or domain models), update the corresponding Pydantic model in `workers/codeforge/models.py`
- When adding/renaming a field in Python, update the corresponding Go struct
- Test round-trip: Go marshal -> Python unmarshal AND Python dump -> Go unmarshal
- Watch for type mismatches: Go `int64` vs Python `int`, Go `float64` vs Python `float`, Go `time.Time` vs Python `datetime`

### API Keys & Secrets
- NEVER hardcode API keys, tokens, or secrets -- always read from environment variables
- LiteLLM proxy auth: use `LITELLM_MASTER_KEY` env var (default fallback: `sk-codeforge-dev`)
- When a component calls LiteLLM, verify the model name has a valid API key configured in `litellm-config.yaml`

### Method Signatures & Interfaces
- Python `LiteLLMClient` method is `chat_completion()` (NOT `chat()`) -- check all callers and test fakes
- When changing a Go interface method signature (e.g. value->pointer for large structs), grep ALL implementations including test mocks
- `golangci-lint hugeParam`: structs >80 bytes should be passed by pointer

### Path Resolution
- Frontend sends dataset names (e.g. `"basic-coding"`), Go resolves to absolute file paths before publishing to NATS
- Python worker receives absolute paths -- it does NOT resolve relative names
- Verify path resolution in `internal/service/benchmark.go` `resolveDatasetPath()`

### Idempotency
- NATS JetStream redelivers unacked messages -- handlers must be idempotent
- Check for duplicate-processing guards (e.g. skip if run already `"completed"`)
- Always `msg.ack()` even on error to prevent infinite redelivery loops

### Error Handling
- Always capture exceptions: `except Exception as exc:` (NOT bare `except Exception:`)
- Log the actual exception: `error=str(exc)` (NOT `error=str(log)` or other objects)
- Publish error results back to NATS so the Go side knows about failures

### Tenant Isolation in SQL Queries
- ALL queries on tenant-scoped tables MUST include `AND tenant_id = $N` with `tenantFromCtx(ctx)`
- Exceptions: user management, token revocation, tenant management itself
- Never rely on filter parameters for tenant isolation — always use `tenantFromCtx(ctx)`
- LIMIT values must use parameterized placeholders (`$N`), not integer interpolation (`%d`)
- NATS payloads that cross the Go/Python boundary MUST carry `tenant_id` so background jobs can inject tenant context via `tenantctx.WithTenant(ctx, payload.TenantID)`
- Reference: `store.go:GetProject` (correct pattern), `store_a2a.go` (fixed pattern)

## Development Methodology: TDD (Test-Driven Development)

**All new features MUST follow TDD.** No exceptions.

### TDD Workflow

1. **Feature Analysis (RED planning)** — Before writing ANY code, thoroughly analyze:
   - Feature goals and acceptance criteria
   - Happy path scenarios
   - Error paths and failure modes
   - **Edge cases** (empty inputs, nil values, boundary conditions, concurrent access, overflow, unicode, max lengths, duplicate keys, etc.)
   - Integration points and side effects

2. **Write Tests First (RED)** — Write comprehensive tests that FAIL:
   - Cover all scenarios identified in step 1
   - Include table-driven tests for related cases
   - Test error messages and error types, not just "it errors"
   - Test boundary values (0, 1, max, max+1)
   - Test nil/empty/missing inputs explicitly

3. **Implement Code (GREEN)** — Write the MINIMUM code to make all tests pass:
   - Don't add features not covered by tests
   - Don't optimize prematurely
   - Each test should go from RED to GREEN

4. **Refactor (REFACTOR)** — Clean up while keeping tests green:
   - Extract common patterns
   - Improve naming
   - Remove duplication

### Edge Case Checklist (apply to every feature)

- Nil/null pointer inputs
- Empty strings, empty slices, empty maps
- Duplicate entries (idempotency)
- Concurrent access (race conditions)
- Maximum length / overflow values
- Invalid UTF-8 / special characters
- Missing required fields
- Already-exists vs not-found scenarios
- Permission / authorization edge cases
- Timeout and cancellation behavior

## E2E Test Setup

Running E2E Playwright tests requires the full stack in **development mode**:

```bash
# 1. Start Docker services
docker compose up -d postgres nats litellm

# 2. Start Go backend with APP_ENV=development (enables benchmark endpoints, dev-mode features)
APP_ENV=development go run ./cmd/codeforge/

# 3. Start frontend dev server
cd frontend && npm run dev

# 4. Run E2E tests
cd frontend && npx playwright test
```

- **`APP_ENV=development` is required** — without it, dev-mode-only endpoints (benchmarks, agent features) return 403
- The `/health` endpoint exposes `dev_mode: true/false` so the frontend can conditionally show features
- Backend port: 8080, Frontend port: 3000
- E2E tests use `admin@localhost` / `Changeme123` (seeded admin)
- Playwright config: chromium only, workers:1, retries:1

### LLM E2E Tests (API-Level, no browser)

```bash
# Only backend + infrastructure needed (no frontend dev server)
cd frontend && npx playwright test --config=playwright.llm.config.ts
```

- 95 tests across 12 spec files in `frontend/e2e/llm/`
- Covers: prerequisites, model management, conversations (simple + agentic), streaming AG-UI, multi-provider, routing, cost tracking, MCP tools, benchmarks
- Helper module: `frontend/e2e/llm/llm-helpers.ts`
- Dedicated config: `frontend/playwright.llm.config.ts` (no browser, sequential execution)

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
