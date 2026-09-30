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

```
TypeScript Frontend (SolidJS)  -->  REST / WebSocket
Go Core Service (HTTP, WS, Agent Lifecycle, Repo Mgmt, Scheduling)  -->  NATS JetStream
Python AI Workers (LLM Calls via LiteLLM Proxy, Agent Execution with custom agent loop; LangGraph planned)
```

| Layer | Language | Purpose |
|---|---|---|
| Frontend | TypeScript | Web GUI (SolidJS + Tailwind CSS) |
| Core Service | Go 1.25 | HTTP/WS Server, Scheduling, Repo Mgmt |
| AI Workers | Python 3.12 | LLM Integration, Agent Execution |
| Infrastructure | Docker | Containerization, Docker-in-Docker |

## Configuration & Tooling

- **YAML for all config files** (comments support) — Modes, Settings, Safety, Autonomy, Schedules
- **JSON only for:** API responses, event serialization, internal data exchange
- **Python:** Poetry, Ruff, Pytest | **Go:** golangci-lint, gofmt, goimports | **TS:** ESLint, Prettier
- **All:** pre-commit hooks (.pre-commit-config.yaml), Docker Compose

## Market Positioning

Unique combination: Project Dashboard + Roadmap + Multi-LLM + Agent Orchestration (no competitor has all four).
Closest: OpenHands (no Roadmap, no Multi-Project Dashboard, no SVN). Details: `docs/research/market-analysis.md`

## Software Architecture

### Core Patterns
- **Hexagonal Architecture (Ports & Adapters)** for Go Core
- **Provider Registry Pattern** — self-registering via `init()`, open-source extensibility
- **Capabilities** instead of mandatory implementation — each provider declares what it can do
- **Compliance Tests** per interface — new adapters inherit the test suite automatically
- **LLM Capability Levels:**
  - Full-featured agents (Claude Code, Aider, OpenHands): orchestration only
  - API with tools (OpenAI, Claude API, Gemini): + Context (GraphRAG) + Routing + Tools
  - Pure completion (Ollama, LM Studio): + everything (Context, Tools, Prompts, Quality)
- **Worker Modules:** Context (GraphRAG), Quality (Debate/Reviewer/Sampler/Guardrail), Routing, Safety, Execution, Memory, History, Events, Orchestration, Hooks, Trajectory, HITL
- **Event types** live in `internal/domain/event/` (broadcast.go, broadcast_payloads.go, agui.go) -- not in adapter layer
- **OTEL span helpers** live in `internal/telemetry/` (API-only, no SDK dependency) -- services use `port/metrics.Recorder` interface
- **Port interfaces for decoupling:** `port/codeintel/` (LSP provider), `port/tokenexchange/` (Copilot token exchange), `port/llm/` (LLM provider)
- **logBestEffort pattern** (`internal/service/log_best_effort.go`) for non-fatal store errors -- logs with structured context instead of silencing

### Agent System
- **Execution Modes:** Sandbox (isolated container), Mount (direct file access), Hybrid — sandbox/hybrid are rejected at start (HTTP 400) until tools run inside the container (KI-13, fail closed); the project config key `execution_mode` (only `mount` accepted) sets the default
- **Safety Layer (8):** Budget Limiter, Command Safety Evaluator, Branch Isolation, Test/Lint Gate, Max Steps, Rollback, Path Blocklist, Stall Detection
- **Workflow:** Plan -> Approve -> Execute -> Review -> Deliver (configurable)
- **Autonomy Levels:** 1=supervised (approve all), 2=semi-auto (approve destructive), 3=auto-edit (approve terminal/deploy), 4=full-auto (safety rules replace user), 5=headless (CI/CD, cron, API)
- **Modes System:** YAML-configurable roles (architect, coder, reviewer, debugger), per-mode tools/LLM/autonomy/prompt, built-in + custom (`.codeforge/modes/`), DAG pipelines, schedule support
- **Per-Mode Tool Lists:** Tools are defined inline in each Mode struct (`Mode.Tools` / `Mode.DeniedTools`, canonical names `Read`/`Write`/`Edit`/`Bash`/`Grep`/`Glob`/`ListDir`), not as separate YAML bundle files — enforced by the Go policy evaluation on run and conversation paths (`DeniedTools` denies; a non-empty `Tools` list denies unlisted built-in tools)
- **History Processors:** Context window optimization pipeline
- **Hook System:** Observer pattern for agent/environment lifecycle
- **Trajectory:** Recording, replay, inspector, audit trail
- **Cost Management:** Budget limits per task/project/user, auto-tracking
- **Prompt Templates:** YAML prompt library (`internal/service/prompts/`, embedded via `//go:embed`, rendered with Go `text/template` by `PromptAssembler`) plus `.tmpl` files in `internal/service/templates/`
- **BM25S Retrieval:** Code search and tool recommendation
- **SimHash Dedup:** 64-bit fingerprints, hamming distance threshold — `internal/service/dedup.go`
- **Real-time State:** WebSocket live updates for agent status, logs, costs — tenant-scoped (`BroadcastEvent` uses the tenant in ctx, events without one are dropped; `BroadcastGlobal` only for tenant-free data), per-client send queues, single-use tickets (`POST /api/v1/ws/ticket`, then `GET /ws?ticket=`; no JWT in the URL). NATS payloads carry `tenant_id` and the worker echoes it

### Agentic Conversation Loop (Phase 17) — **implemented**
- Multi-turn tool-use loop: LLM -> tools -> results -> repeat. Go dispatches via NATS (`conversation.run.start/complete`)
- 10 built-in tools (`read_file`, `write_file`, `edit_file`, `bash`, `search_files`, `glob_files`, `list_directory`, `search_conversations`, `search_skills`, `create_skill`) plus context-dependent `handoff_to`, `propose_goal`, `propose_roadmap`, `spawn_subagent` + MCP merge (tool defects: see [Known Issues](docs/todo.md#known-issues) KI-25, KI-38, KI-58)
- `AgentLoopExecutor` (Python): streaming LLM, per-tool policy, cost tracking
- `ConversationHistoryManager`: head-and-tail token budget, tool result truncation
- HITL: `DecisionAsk` -> WS `agui.permission_request` -> HTTP approve/deny -> channel resume
- One run per conversation: a second message while a run is active gets 409; each dispatch has a `turn_id` (on `conversation.run.start`, `runs.toolcall.request`, `conversation.run.complete`) so calls of a stopped run are denied
- Config: `MaxLoopIterations` (50), `MaxContextTokens` (128K), `ContextEnabled` (true), `ContextBudget` (2048), `ContextPromptReserve` (512), `ApprovalTimeoutSeconds` (60)
- **Adaptive Context Budget:** Linear decay from `ContextBudget` to 0 over 60 messages — `internal/service/context_budget.go`
- **Auto-Indexing:** Clone/Adopt/Setup trigger RepoMap + Retrieval Index + GraphRAG — `internal/service/project.go` (`AutoIndex`), called from `internal/adapter/http/handlers_project.go`
- Runs (`runs.start`) use the same loop in the run's project workspace (shared setup: `workers/codeforge/loop_config.py` `build_loop_config`, `resolve_model_and_fallbacks`; no skill tools, no Claude Code models), with heartbeats and a Go decision per LLM and tool call
- Key files: `workers/codeforge/agent_loop.py`, `workers/codeforge/tools/`, `internal/service/conversation.go`, `internal/service/runtime_execution.go`, `internal/service/runtime_lifecycle.go`

### Chat Enhancements — **implemented**
HITL Permission UI (`PermissionRequestCard`: approve/deny/allow-always, countdown, preset mapping, `POST /policies/allow-always`), Inline Diff Review (`DiffPreview`), Action Buttons (copy/retry/apply/diff), Per-Message Cost (`MessageBadge`+`CostBreakdown` via AG-UI `state_delta`), Smart References (`@/#//` autocomplete, `AutocompletePopover`, `useFrequencyTracker`), Slash Commands (`/compact`/`/rewind`/`/clear`/`/diff`/`/cost`/`/help`/`/mode`/`/model` via `CommandService` + `GET /commands`, frontend `commandStore.ts`/`commandExecutor.ts`), Conversation Search (PostgreSQL FTS, GIN, `ts_rank`, `POST /search/conversations`), Notification Center (`notificationStore`, browser push, Web Audio, tab badge), Real-Time Channels (3 tables, 9 endpoints, WS events, `ChannelList`/`ChannelView`/`ThreadPanel`). Spec: `docs/features/05-chat-enhancements.md`

### Framework & Agent Insights (adopted patterns)

**From LangGraph, CrewAI, AutoGen, MetaGPT:**
Composite Memory Scoring (Semantic+Recency+Importance) — `workers/codeforge/memory/scorer.py`, `internal/service/memory.go` | Context Window Strategies (Buffered/TokenLimited/HeadAndTail) | Experience Pool (@exp_cache) — `workers/codeforge/memory/experience.py`, `internal/service/experience_pool.go` | Tool Recommendation via BM25 | Workbench (tool container, shared state, MCP) | LLM Guardrail Agent | Structured Output/ActionNode (schema validation + review/revise) | Event Bus (Agent/Task/System -> WS) | GraphFlow/DAG (Conditional Edges, Parallel Nodes, Cycles) | Composable Termination (MaxSteps|Budget|Timeout) | Component System (JSON serializable, GUI editor) | Document Pipeline PRD->Design->Tasks->Code | MagenticOne Planning Loop (Stall Detection + Re-Planning) | HandoffMessage Pattern — `internal/domain/orchestration/handoff.go`, `internal/service/handoff.go`, `workers/codeforge/tools/handoff.py` | Human Feedback Provider (Web GUI, Slack, Email) — `internal/port/feedback/provider.go`, `internal/adapter/slack/feedback.go`, `internal/adapter/email/feedback.go`

**From Cline, Devika:**
Plan/Act Mode (**implemented**: `workers/codeforge/plan_act.py`, `internal/service/conversation_dispatch.go`) | Shadow Git Checkpoints | Ask/Say Approval Pattern | MCP extensibility | .clinerules-like YAML config | Auto-Compact (~80% window) | Diff-based File Review | Sub-Agent Architecture | Agent State Visualization | LLM-driven Web Crawler | Stateless Agent Design (state in core)

**From OpenHands, SWE-agent:**
Event-Sourcing (EventStream) | Workspace Abstraction (Local/Docker/Remote, self-healing) | AgentHub (CodeAct, Browsing, Delegator, Microagents) | Microagents (YAML+MD trigger-driven) — `internal/domain/microagent/`, `internal/service/microagent.go` | Skills System — `workers/codeforge/skills/`, `internal/service/skill.go` | Risk Management (LLMSecurityAnalyzer) | V0->V1 SDK Migration | RouterLLM via LiteLLM tags — `internal/service/conversation_dispatch.go`, `workers/codeforge/llm.py`, `workers/codeforge/consumer/_conversation_routing.py` | ACI (shell for LLMs) | Per-Mode Tool Lists | History Processors | SWE-ReX Sandbox | Mini-SWE-Agent (100 LOC, 74% SWE-bench) | ToolFilterConfig (blocklist + conditional blocking)

### Competitor Analysis

| Tool | Stack/License | Role |
|---|---|---|
| Codel | Go+React, Docker Sandbox, AGPL-3.0 | architecture reference |
| CLI Agent Orchestrator | AWS, Supervisor/Worker, tmux/MCP | closest competitor |
| Goose | Rust, MCP-native, 30k+ stars, Apache 2.0 | backend candidate |
| OpenCode | Go, Client/Server, LSP, MIT | backend candidate |
| Plandex | Go, Planning-First, Diff Sandbox, MIT | backend candidate |
| Roo Code | Modes System, Cloud Agents, Apache 2.0 | pattern reference |
| Codex CLI | OpenAI, Multimodal, GitHub Action, Apache 2.0 | backend candidate |
| SERA | Ai2, Open Model Weights, $400 Training, Apache 2.0 | self-hosted model |
| bolt.diy | 19k stars, 19+ providers, MIT | multi-LLM reference |
| AutoForge | Two-Agent, Test-First, Multi-Session | workflow pattern |
| Dyad | Local-First, Apache 2.0 | UX reference |
| AutoCodeRover | AST-aware, GPL-3.0, $0.70/task | niche agent |
| AI Maestro | Next.js/Node.js, Peer Mesh, AMP, CozoDB | multi-machine orchestration, War Room UX, agent identity (Phase 23) |
| AMP | Agent Messaging Protocol v0.1.2-draft, Apache 2.0, 23blocks | Ed25519 signatures, trust, federation. Patterns -> Phase 23 |

### Implemented Phases

**Security & Trust (Phase 23):** Trust Annotations (4 levels, auto-stamped on NATS) — `internal/domain/trust/` | Message Quarantine (risk scoring, admin review) — `internal/service/quarantine.go`, migration 049 | Persistent Agent Identity (stats, state, capabilities, inbox) — `internal/domain/agent/agent.go` | War Room — `frontend/src/features/project/WarRoom.tsx` | A2A/handoff trust gates bypassed: see [Known Issues](docs/todo.md#known-issues) KI-15

**Benchmark & Evaluation (Phase 26+28+5):** Phase 26: Provider interface, evaluator plugins (LLMJudge, FunctionalTest, SPARC, FilesystemState), 3 runners, external providers (HumanEval, MBPP, SWE-bench, DPAI Arena, Terminal-Bench). Phase 28 (R2E-Gym/EntroPO): Hybrid verification, trajectory verifier, multi-rollout, entropy-UCB1 MAB, DPO export, SWE-GEN — `workers/codeforge/evaluation/`. Phase 5: DPAI Arena, Terminal-Bench+FilesystemStateEvaluator, RLVR export (`GET /benchmarks/runs/{id}/export/rlvr`).

**Contract-First Review/Refactor (Phase 31):** Boundary Detection (LLM-based, API/data/inter-service/cross-language) — `internal/domain/boundary/` | Review-Refactor Pipeline (4-step: boundary_analyzer->contract_reviewer->reviewer->refactorer) — `internal/domain/pipeline/presets.go` | 2 Modes: `boundary_analyzer` (read-only, plan), `contract_reviewer` (read-only, review) — `internal/domain/mode/presets.go` | ReviewTriggerService (cascade triggers, SHA dedup) — `internal/service/review_trigger.go` | DiffImpactScorer (3-tier HITL) — `internal/service/diff_impact.go` | Phase-aware Context Budget (100%/60%/50%/70%) — `internal/service/context_budget.go` | waiting_approval status — `internal/service/orchestrator.go` | BoundaryService — `internal/service/boundary.go` | Frontend: RefactorApproval, BoundariesPanel — `frontend/src/features/project/` | NATS: `review.>` wildcard — `internal/adapter/nats/nats.go` (stream subjects), subjects in `internal/port/messagequeue/queue.go` | Not wired end-to-end yet: see [Known Issues](docs/todo.md#known-issues) KI-17

**Visual Design Canvas (Phase 32):** SVG canvas with 9 tools (select, rect, ellipse, freehand, text, annotate, image, polygon, node) — `frontend/src/features/canvas/` | Triple export: PNG (offscreen), ASCII (char grid), JSON | Smart output: vision->PNG+JSON, text-only->ASCII+JSON, basic->JSON | Multimodal pipeline: `MessageImage` Frontend->Go JSONB->NATS->Python content-array->LiteLLM | Migration `075_add_message_images.sql` | `buildCanvasPrompt()` uses `supports_vision` | Spec: `docs/features/06-visual-design-canvas.md`

### Roadmap Auto-Detection & Integration
No custom PM tool — sync with Plane, OpenProject, GitHub/GitLab Issues. Auto-Detection: 3-tier (repo files->platform APIs->file markers). Multi-Format SDD: OpenSpec (`openspec/`), Spec Kit (`.specify/`), Autospec (`specs/spec.yaml`). Provider Registry: `specprovider` + `pmprovider`, same architecture as Git. Bidirectional Sync: CodeForge <-> PM Tool <-> Repo Specs (Webhook/Poll/Manual). Adopted patterns: Plane (cursor pagination, HMAC-SHA256, label sync), OpenProject (optimistic locking, schema endpoints), OpenSpec (delta spec), Ploi Roadmap (`/ai`). Gitea/Forgejo: GitHub adapter works (compatible API). Details: `docs/research/market-analysis.md` Section 5.

### Database & Libraries

**PostgreSQL 18** (shared with LiteLLM: same `codeforge` database and `public` schema, LiteLLM tables prefixed `LiteLLM_`; no schema separation): Go pgx v5 + goose migrations, Python psycopg3 (sync+async), NATS JetStream KV for ephemeral state. ADR: `docs/architecture/adr/002-postgresql-database.md`

**Go Libraries (minimal-dep):** chi v5 (router), coder/websocket v1.8+ (WS), os/exec git CLI wrapper. NOT used: Echo/Fiber, gorilla/websocket, go-git.

**Frontend Libraries (minimal-stack):** @solidjs/router, Tailwind CSS (no component lib), native WebSocket wrapper (`frontend/src/api/websocket.ts`), native fetch + thin wrapper, SolidJS signals/stores/context, Unicode+inline SVG icons plus vscode-icons-js (file icons), @unovis/solid + @unovis/ts (charts), solid-monaco (code editor), @tanstack/solid-virtual (virtual lists), Outfit+Source Sans 3 fonts (self-hosted woff2 in `frontend/public/fonts/`), design system at `/design-system` (dev-only, docs in `frontend/src/ui/DESIGN-SYSTEM.md`), onboarding wizard (`codeforge-onboarding-completed` key) — `frontend/src/features/onboarding/OnboardingWizard.tsx`. NOT used: axios, styled-components, Kobalte, shadcn-solid, Socket.IO, Redux/Zustand.

### Protocol Support

| Protocol | Status | Purpose |
|---|---|---|
| **MCP** | Phase 15 | Agent<->Tool (JSON-RPC). Go: mcp-go SDK (tools: list_projects, get_project, get_run_status, get_cost_summary; resources: codeforge://projects, codeforge://costs/summary; registry with PG persistence, project assignment, HTTP CRUD). Python: McpWorkbench (multi-server, BM25 recommend, discovery, bridging). Frontend: MCPServersPage. Config: `mcp.enabled/servers_dir/server_port` (3001). Policy: `mcp:server:tool` glob matching. |
| **LSP** | Phase 15D | Code intelligence (go-to-def, refs, diagnostics). Go Core manages lifecycle per project language. |
| **A2A** | Phase 27 | Agent-to-Agent v0.3.0 (LF, Apache 2.0). Protobuf `lf.a2a.v1`, JSON-RPC 2.0/HTTPS, SSE, push notifications. 11 RPCs, 8 task states. Types: AgentCard, Task, Message, Part, Artifact. Security: API Key, Bearer/JWT, OAuth 2.0, OIDC, mTLS, JWS. Multi-tenant `/{tenant}/`. SDKs: Go/Python/JS/Java/.NET. Integration: agents as A2A servers, Go Core as client via `a2a-go`, NATS internal + A2A external. |
| **AG-UI** | Phase 17+ | Agent<->Frontend streaming (CopilotKit). 12 events (all prefixed `agui.`): run_started/finished, text_message, tool_call/result, state_delta, step_started/finished + CodeForge extensions permission_request, goal_proposal, action_suggestion, roadmap_proposal (`internal/domain/event/agui.go`). WS format across Go+TS (20+ files). |
| **OTEL GenAI** | implemented (opt-in, `otel.enabled`) | LLM/agent observability. Go: OTLP traces + metrics (`internal/adapter/otel/`), HTTP middleware, run/toolcall/delivery spans (`internal/telemetry/`). Python: `workers/codeforge/tracing/` with `gen_ai.*` LLM span attributes, OTLP metrics, `traceparent` on every worker publish. `otel.insecure` (default false) selects plaintext gRPC on both sides. Jaeger in `docker-compose.yml` (dev profile). LiteLLM OTEL callback not configured yet. |
| **Watch** | future | ANP (decentralized networking), LSAP (LSP for AI agents). |

MCP + A2A complementary: "MCP for tools, A2A for agents"

### LLM Integration
- **LiteLLM Proxy** as Docker sidecar (port 4000) — no custom LLM provider interface
- Go + Python communicate via OpenAI-compatible API against LiteLLM
- **Hybrid Routing (Phase 29):** Enabled by default (`CODEFORGE_ROUTING_ENABLED=true`). Cascade order: (1) ComplexityAnalyzer (rule-based, <1ms, always runs) -> (2) MABModelSelector (UCB1, primary) -> (3) LLMMetaRouter (cold-start fallback) -> (4) Complexity defaults (final fallback). Package: `workers/codeforge/routing/`
- LiteLLM uses provider wildcards (`openai/*`, `anthropic/*`) — HybridRouter picks exact model
- Scenario tags (default/background/think/longContext/review/plan) as fallback when routing disabled
- OpenRouter as optional provider; GitHub Copilot Token Exchange — `internal/adapter/copilot/client.go`, `POST /api/v1/copilot/exchange`
- Local Model Auto-Discovery (Ollama/LM Studio `/v1/models`)
- Custom: LiteLLM Config Manager, User Key Mapping, Cost Dashboard

Details: `docs/architecture.md` | Framework comparison: `docs/research/market-analysis.md`

## Strategic Principles

- Leverage existing building blocks (LiteLLM, OpenSpec, Aider/OpenHands as backends)
- Do not reinvent the wheel — differentiate through integration of all four pillars
- Performance focus: Go for core, Python only for AI-specific work

## Architectural Decisions & Infrastructure

| ADR | Decision | Details |
|---|---|---|
| 001 | NATS JetStream as MQ | `docs/architecture/adr/001-nats-jetstream-message-queue.md` |
| 002 | PostgreSQL 18 as DB | `docs/architecture/adr/002-postgresql-database.md` |
| 003 | Config: defaults < YAML < env < CLI flags | `docs/architecture/adr/003-config-hierarchy.md` |
| 004 | Async logging (buffered channel + workers) | `docs/architecture/adr/004-async-logging.md` |
| 005 | Docker-native logging (no ELK/Loki/Grafana) | `docs/architecture/adr/005-docker-native-logging.md` |
| 006 | Approach C: Go control plane + Python runtime | `docs/architecture/adr/006-agent-execution-approach-c.md` |
| 007 | Policy: first-match-wins permission rules | `docs/architecture/adr/007-policy-layer.md` |
| 008 | Benchmark: DeepEval + GEMMAS-inspired metrics (AgentNeo removed 2026-03-05) | `docs/architecture/adr/008-benchmark-evaluation-framework.md` |
| 009 | GDPR compliance architecture | `docs/architecture/adr/009-gdpr-compliance-architecture.md` |
| 010 | A2A protocol adoption for agent federation | `docs/architecture/adr/010-a2a-protocol-adoption.md` |
| 011 | Trust annotations and message quarantine | `docs/architecture/adr/011-trust-quarantine-system.md` |
| 012 | Hybrid routing cascade for model selection | `docs/architecture/adr/012-hybrid-routing-cascade.md` |
| 013 | Service layer config sub-struct imports | `docs/architecture/adr/013-config-import-in-services.md` |
| 014 | Store interface segregation plan | `docs/architecture/adr/014-store-interface-segregation.md` |
| 015 | Policy deny lists are blocklists; canonical tool names (amends 007) | `docs/architecture/adr/015-policy-deny-lists-and-tool-names.md` |
| 016 | NATS delivery: shared durables without replay, ack-on-accept runs, bounded retries + DLQ (refines 001) | `docs/architecture/adr/016-nats-delivery-semantics.md` |

**Infrastructure Principles:**
- **Zero-config startup** — system runs with defaults; CLI flags have highest precedence
- **Async-first:** Logging, NATS, LLM calls never block hot path. Buffered channels + workers (Go), QueueHandler + QueueListener (Python)
- **Docker-native logging:** Structured JSON to stdout, `docker compose logs` + `jq` for debugging
- **Policy Layer:** Declarative YAML, 5 built-in presets, extensible without code (`policy.custom_dir`, default `data/policies`). ADR-015: canonical tool names (`internal/domain/policy/toolnames.go`), deny lists win regardless of rule order, then first-match-wins; workspace-relative paths; shell commands parsed per simple command (`command.go`, opaque constructs fail closed); unknown profile/mode denies. Workers send `tool`, `command`, `path`, `mode_id` and a display-only `arguments_preview` on `runs.toolcall.request`; `agui.permission_request` carries the deciding `profile`. Claude Code runs (`claudecode/*`) reach the same check through a PreToolUse hook and a per-run socket (`workers/codeforge/claude_code_executor.py`; repo settings and MCP servers are not loaded). Open: global profile namespace (KI-68), follow-ups (KI-69)
- **Approach C:** Go owns state/policies/sessions; Python owns LLM/tools/agent loop; NATS with per-tool-call policy
- **Resilience:** Circuit breakers (NATS, LiteLLM), idempotency keys, dead letter queues, 4-phase graceful shutdown

## Character Encoding

- **Config files (.env, .yaml, .toml, .json, .sh, .gitignore): ASCII only** — no box-drawing chars
- Regular dashes `-` and `=` for separators; Mermaid blocks for diagrams in docs

## Coding Principles

- **Strict type safety:** No `any`/`interface{}`/`Any` — use generics, unions, specific interfaces
- **Minimal dependencies:** Prefer stdlib when it covers 80%+ of need
- **Readable code = documentation:** Comments only for non-obvious "why"
- **DRY:** Extract at 3+ occurrences, no premature abstraction
- **Simple over clever:** Next developer/agent must understand immediately
- **Minimal surface area:** Start private, export only when needed

### The Zen of Python (PEP 20) — applies to ALL languages

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

When modifying code that crosses the Go/Python boundary via NATS, verify ALL:

### NATS Subjects & Streams
- Subjects must match EXACTLY: Go (`internal/port/messagequeue/queue.go`, `Subject*` constants) <-> Python (`workers/codeforge/consumer/_subjects.py`)
- JetStream stream config (`internal/adapter/nats/nats.go`, `CreateOrUpdateStream` subjects list) must include wildcards for new prefixes (e.g. `benchmark.>`)
- New subjects need publisher (one side) + subscriber (other side)

### JSON Payload Contracts
- Go JSON tags must match Python Pydantic field names exactly
- Sync changes: Go (`internal/port/messagequeue/schemas_*.go` / domain) <-> Python (`workers/codeforge/models.py`)
- Test round-trip both directions
- Type mapping: Go `int64`/`float64`/`time.Time` <-> Python `int`/`float`/`datetime`

### API Keys & Secrets
- NEVER hardcode — always env vars. LiteLLM auth: `LITELLM_MASTER_KEY` (default: `sk-codeforge-dev`)
- Verify model has valid key in `litellm/config.yaml`

### Method Signatures & Interfaces
- Python `LiteLLMClient.chat_completion()` (NOT `chat()`) — check callers + fakes
- Go interface changes: grep ALL implementations including test mocks
- `golangci-lint hugeParam`: structs >80 bytes -> pointer

### Path Resolution
- Frontend sends dataset names -> Go resolves to absolute paths -> NATS -> Python receives absolute paths
- Verify: `internal/service/benchmark_run.go` `(*BenchmarkRunManager).resolveDatasetPath()`

### Delivery & Idempotency (ADR-016)
- One shared durable pull consumer per subject and side (`codeforge-go-*` / `codeforge-py-*`), created with deliver policy `new`, no inactivity threshold, `MaxDeliver` 4 — provisioning: `internal/adapter/nats/nats.go`, `workers/codeforge/consumer/_delivery.py`
- Workspace-changing work (`runs.start`, `conversation.run.start`, `tasks.agent.*`, `benchmark.run.request`) is at-most-once: accepted with a confirmed double ack (`ack_sync`, retried 3 times; unconfirmed -> NAK, work not started), registered in `self._in_flight`, and its failure is reported as a failed completion; completions go through `_publish_result` / `publish_with_retry` (retries, one `Nats-Msg-Id`). Every other subject is at-least-once and its handler must be idempotent
- Settle every message exactly once: success or a published error result -> ack; failure -> `_retry_or_dead_letter` (NAK with delay, DLQ + ack on the last attempt, from `num_delivered`); invalid payload -> `_reject_invalid` (DLQ + `term`). Never NAK an invalid payload, never ack without a DLQ copy (DLQ copies drop `Nats-*` headers)
- Duplicate guards (skip if already `"completed"`); a failed request is removed from the dedup cache
- Notification subscriptions (per-run and per-task cancel listeners, one `runtime.listen_for_cancel` helper; tool-call responses) use deliver policy `new` and ack policy `none`; Go keeps handlers in progress up to `Queue.SetMaxHandlerDuration` (covers the HITL approval timeout). The worker waits for a tool-call decision up to the approval timeout (`approval_timeout_seconds` on `runs.start` / `conversation.run.start`, default 60 s) plus 15 s
- Quality gates: `runs.qualitygate.request` carries `timeout_seconds` and `heartbeat_seconds`; while a gate runs the worker sends `runs.heartbeat` (with `tenant_id`, phase `quality_gate`) and keeps the request in progress; a check that could not run is reported as a null verdict plus `error` (only a check that ran and failed rolls back)
- `tasks.agent.*` carries `TaskAgentPayload` (`task_id`, `project_id`, `tenant_id`, `agent_id`, `backend`, `workspace_path`); backend CLIs run in their own process group, which `tasks.cancel` stops (result status `cancelled`)
- A worker whose consumer loop gives up fails its unfinished accepted work (30 s grace) and exits 1

### Error Handling
- `except Exception as exc:` (NOT bare), log `error=str(exc)`, publish errors back to NATS, then settle the message as above

### Tenant Isolation
- ALL tenant-scoped queries: `AND tenant_id = $N` with `tenantFromCtx(ctx)` (exceptions: user/token/tenant mgmt, and system jobs that must span tenants - the GDPR retention sweep and the stuck-work watchdog's `ListStaleRuns` - whose queries are commented `INTENTIONALLY CROSS-TENANT` with the reason and handle each row in its own tenant's context)
- LIMIT via `$N` placeholders, not `%d` interpolation
- NATS payloads MUST carry `tenant_id` for background jobs -> `tenantctx.WithTenant(ctx, payload.TenantID)`
- Reference: `store_project.go:GetProject` (correct), `store_a2a.go` (fixed)

## TDD (Test-Driven Development)

**All new features MUST follow TDD.** No exceptions.

1. **RED planning** — Analyze: goals, acceptance criteria, happy path, error paths, edge cases, integration points
2. **RED** — Write failing tests: table-driven, error types/messages, boundary values (0, 1, max, max+1), nil/empty
3. **GREEN** — Minimum code to pass all tests
4. **REFACTOR** — Clean up, extract, rename, deduplicate (tests stay green)

### Edge Case Checklist (every feature)
Nil/null pointers | empty strings/slices/maps | duplicates (idempotency) | concurrent access | max length/overflow | invalid UTF-8/special chars | missing required fields | exists vs not-found | permission edge cases | timeout/cancellation

## E2E Test Setup

Full stack in **development mode** required:

```bash
# 1. Docker services
docker compose up -d postgres nats litellm
# 2. Go backend (APP_ENV=development required — dev endpoints return 403 without it;
#    CODEFORGE_AUTH_ADMIN_PASS seeds the admin user that Playwright logs in with)
APP_ENV=development CODEFORGE_AUTH_ADMIN_PASS=Changeme123 go run ./cmd/codeforge/
# 3. Frontend
cd frontend && npm run dev
# 4. Tests
cd frontend && npx playwright test
```

- `/health` exposes `dev_mode: true/false` | Backend: 8080, Frontend: 3000
- Credentials: `admin@localhost` / `Changeme123` (seeded only when `CODEFORGE_AUTH_ADMIN_PASS` is set, otherwise the backend waits for the setup wizard; the admin is created with `must_change_password`, which `frontend/e2e/global-setup.ts` handles) | Playwright: chromium, workers:1, retries:1

### LLM E2E Tests (API-Level, no browser)

```bash
cd frontend && npx playwright test --config=playwright.llm.config.ts
```

88 tests, 11 specs in `frontend/e2e/llm/`. Helper: `frontend/e2e/llm/llm-helpers.ts`. Config: `frontend/playwright.llm.config.ts`

### Autonomous Goal-to-Program Test (Playwright-MCP)

Testplan: `docs/testing/autonomous-goal-to-program-testplan.md` | Tool complexity: `docs/plans/tool-call-complexity-plan.md`

**Critical Startup Sequence (in order):**
1. `docker compose up -d postgres nats litellm`
2. Resolve container IPs (`localhost` ports BROKEN in WSL2):
   ```bash
   NATS_IP=$(docker inspect codeforge-nats | grep -m1 '"IPAddress"' | grep -oP '[\d.]+')
   LITELLM_IP=$(docker inspect codeforge-litellm | grep -m1 '"IPAddress"' | grep -oP '[\d.]+')
   POSTGRES_IP=$(docker inspect codeforge-postgres | grep -m1 '"IPAddress"' | grep -oP '[\d.]+')
   ```
3. Purge NATS JetStream (`await js.purge_stream('CODEFORGE')` + delete stale consumers)
4. Start Go backend: `APP_ENV=development CODEFORGE_AUTH_ADMIN_PASS=Changeme123 go run ./cmd/codeforge/`
5. **VERIFY** toolcall consumer: `curl http://${NATS_IP}:8222/jsz?consumers=1`
6. Start Python worker:
   ```bash
   PYTHONPATH=/workspaces/CodeForge/workers \
     NATS_URL="nats://${NATS_IP}:4222" \
     LITELLM_BASE_URL="http://${LITELLM_IP}:4000" \
     LITELLM_MASTER_KEY="sk-codeforge-dev" \
     DATABASE_URL="postgresql://codeforge:codeforge_dev@${POSTGRES_IP}:5432/codeforge" \
     CODEFORGE_ROUTING_ENABLED=false \
     APP_ENV=development \
     .venv/bin/python -m codeforge.consumer
   ```
7. Frontend: `cd frontend && npm run dev`
8. Playwright-MCP browser: `http://host.docker.internal:3000` (not localhost)

**Key env vars:** `LITELLM_BASE_URL` (NOT `LITELLM_URL`), `CODEFORGE_ROUTING_ENABLED=false` (override default=true; avoids router picking unhealthy models in test), auth field: `access_token` (NOT `token`)

**Project setup:**
- Create project: `POST /projects` with `config: {"policy_preset": "trusted-mount-autonomous"}` (the backend also reads `execution_mode`, which only accepts `mount`, and the gate commands `test_command` / `lint_command`, validated on write; autonomy comes from the selected mode; `PUT /projects/{id}` merges config keys, `null` deletes one) and optional `"local_path": "/abs/path"` (auto-adopts workspace)
- Alternatively: `POST /projects/{id}/adopt` with `{"path": "/abs/path"}` as separate call
- TestRepo clone fails often — use local workspace creation instead
- Auto-onboarding disabled (ChatPanel.tsx)
- Model: `"openai/container"` or `"lm_studio/qwen/qwen3-30b-a3b"` (any healthy model)

**Local model timeouts:** 3-10x slower. S1: up to 60min, S4: up to 180min. DO NOT abort early. Monitor via API, not browser. "Stuck" = no new tool calls for 10min.

**HITL:** Poll logs for `"HITL approval requested"`. Approve: `POST /api/v1/runs/{convId}/approve/{callId}` `{"decision":"allow"}`. Bypass: `POST /conversations/{id}/bypass-approvals`

## Versioning

**Source of truth:** `VERSION` file (root, semver string e.g. `0.8.0`). Current: **v0.8.0**

| Layer | Mechanism | Key file |
|---|---|---|
| Go | `internal/version` reads `VERSION`, overridable via `-ldflags` | `internal/version/version.go` |
| Python | `_read_version()` traverses paths | `workers/codeforge/__init__.py` |
| Frontend | Vite `define: { __APP_VERSION__ }` at build | `frontend/vite.config.ts` |
| Docker | `ARG APP_VERSION` + OCI labels | `.github/workflows/docker-build.yml` |

**Change:** Edit `VERSION` -> `./scripts/sync-version.sh` (propagates to pyproject.toml, package.json, package-lock.json) -> all layers auto-pick-up.

**Build (Docker/CI):** Go: ldflags (`version.Version`, `version.GitSHA`). Python/Frontend: `VERSION` file COPY'd. OCI labels: `org.opencontainers.image.version/.revision`. Tags: `ghcr.io/.../codeforge-core:0.8.0`

## Git Workflow

- **Commits on `staging` only** — never `main` unless explicitly instructed
- **English only** for commits, docs, comments, config descriptions
- **Always push after committing**
- **Pre-commit checklist:**
  1. `pre-commit run --all-files` and fix errors
  2. Update docs: `docs/todo.md` (mark `[x]`/add new), `docs/architecture.md` (structural), `docs/features/*.md` (feature), `docs/dev-setup.md` (dirs/ports/env), `docs/tech-stack.md` (deps), `docs/project-status.md` (milestones), `CLAUDE.md` (pillars/arch/workflow)

## Documentation Policy

**Every change must be documented.**

```
docs/
├── README.md               # Index
├── todo.md                 # Central TODO (single source of truth, incl. Known Issues)
├── known-issues-fix-plan.md # Milestone plan (S0-S6) for fixing the Known Issues
├── architecture.md         # System architecture
├── dev-setup.md            # Setup guide
├── project-status.md       # Phase tracking
├── tech-stack.md           # Dependencies
├── SECURITY.md             # Security policy, secret management
├── data-retention.md       # GDPR data retention policy
├── privacy-policy.md       # Privacy & LLM data processing notice
├── disaster-recovery.md    # Backup/restore runbook
├── features/               # Feature specs (01-07)
├── api/                    # OpenAPI spec (openapi.yaml)
├── security/               # Breach notification procedure, data classification
├── specs/                  # Design specs (*-design.md)
├── plans/                  # Implementation plans (*-plan.md)
├── testing/                # Test plans + reports
├── audits/                 # Schema, UX, code audits
├── architecture/adr/       # ADRs 001-016 (use _template.md)
├── research/               # Market research
└── prompts/                # Claude Code audit/discovery prompts
```

**TODO rules:** Read `docs/todo.md` before work. Mark `[x]` with date on completion. Add new tasks when discovered. Feature TODOs in `docs/features/*.md` cross-referenced in `docs/todo.md`.

| Change Type | Update |
|---|---|
| Feature work | `docs/features/*.md`, `docs/todo.md` |
| Architecture decision | `docs/architecture.md`, `docs/architecture/adr/`, `CLAUDE.md` |
| New dependency/tool | `docs/tech-stack.md`, `docs/dev-setup.md` |
| Milestone complete | `docs/project-status.md`, `docs/todo.md` |
| New dir/port/env var | `docs/dev-setup.md` |
| Core pillar change | `CLAUDE.md`, `docs/features/*.md` |
| Design spec/plan | `docs/specs/` or `docs/plans/`, `docs/todo.md` |
| Test results | `docs/testing/`, `docs/todo.md` |
| Audit findings | `docs/audits/`, `docs/todo.md` |
| Any code change | `docs/todo.md` |

Feature docs: one per pillar in `docs/features/`, sub-features as sections, contain overview/design/API/TODOs. ADRs: Context -> Decision -> Consequences -> Alternatives.
