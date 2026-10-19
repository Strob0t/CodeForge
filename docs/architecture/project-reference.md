# Project Reference

Descriptive project context that used to live in the root agent instruction file (`CLAUDE.md`, now
[`AGENTS.md`](../../AGENTS.md)). `AGENTS.md` keeps the rules that apply to every change; this page keeps the
catalogues of implemented features, adopted patterns, protocols and competitors. Feature details:
[`docs/features/`](../features/); architecture: [`docs/architecture.md`](../architecture.md).

## Market Positioning

Unique combination: Project Dashboard + Roadmap + Multi-LLM + Agent Orchestration (no competitor has all four).
Closest: OpenHands (no Roadmap, no Multi-Project Dashboard, no SVN). Details: `docs/research/market-analysis.md`

## Chat Enhancements — **implemented**
HITL Permission UI (`PermissionRequestCard`: approve/deny/allow-always, countdown, preset mapping, `POST /policies/allow-always`), Inline Diff Review (`DiffPreview`), Action Buttons (copy/retry/apply/diff), Per-Message Cost (`MessageBadge`+`CostBreakdown` via AG-UI `state_delta`), Smart References (`@/#//` autocomplete, `AutocompletePopover`, `useFrequencyTracker`), Slash Commands (`/compact`/`/rewind`/`/clear`/`/diff`/`/cost`/`/help`/`/mode`/`/model` via `CommandService` + `GET /commands`, frontend `commandStore.ts`/`commandExecutor.ts`), Conversation Search (PostgreSQL FTS, GIN, `ts_rank`, `POST /search/conversations`), Notification Center (`notificationStore`, browser push, Web Audio, tab badge), Real-Time Channels (3 tables, 9 endpoints, WS events, `ChannelList`/`ChannelView`/`ThreadPanel`). Spec: `docs/features/05-chat-enhancements.md`

## Framework & Agent Insights (adopted patterns)

**From LangGraph, CrewAI, AutoGen, MetaGPT:**
Composite Memory Scoring (Semantic+Recency+Importance) — `workers/codeforge/memory/scorer.py`, `internal/service/memory.go` | Context Window Strategies (Buffered/TokenLimited/HeadAndTail) | Experience Pool (@exp_cache, opt-in via `experience.enabled`, tenant-scoped, only for the first turn of a simple chat; never replaces agent work) — `workers/codeforge/memory/experience.py`, `internal/service/experience_pool.go` | Tool Recommendation via BM25 | Workbench (tool container, shared state, MCP) | LLM Guardrail Agent | Structured Output/ActionNode (schema validation + review/revise) | Event Bus (Agent/Task/System -> WS) | GraphFlow/DAG (Conditional Edges, Parallel Nodes, Cycles) | Composable Termination (MaxSteps|Budget|Timeout) | Component System (JSON serializable, GUI editor) | Document Pipeline PRD->Design->Tasks->Code | MagenticOne Planning Loop (Stall Detection + Re-Planning) | HandoffMessage Pattern — `internal/domain/orchestration/handoff.go`, `internal/service/handoff.go`, `workers/codeforge/tools/handoff.py` | Human Feedback Provider (Web GUI, Slack, Email) — `internal/port/feedback/provider.go`, `internal/adapter/slack/feedback.go`, `internal/adapter/email/feedback.go`

**From Cline, Devika:**
Plan/Act Mode (**implemented**: `workers/codeforge/plan_act.py`, `internal/service/conversation_dispatch.go`) | Shadow Git Checkpoints | Ask/Say Approval Pattern | MCP extensibility | .clinerules-like YAML config | Auto-Compact (~80% window) | Diff-based File Review | Sub-Agent Architecture | Agent State Visualization | LLM-driven Web Crawler | Stateless Agent Design (state in core)

**From OpenHands, SWE-agent:**
Event-Sourcing (EventStream) | Workspace Abstraction (Local/Docker/Remote, self-healing) | AgentHub (CodeAct, Browsing, Delegator, Microagents) | Microagents (YAML+MD trigger-driven) — `internal/domain/microagent/`, `internal/service/microagent.go` | Skills System — `workers/codeforge/skills/`, `internal/service/skill.go` | Risk Management (LLMSecurityAnalyzer) | V0->V1 SDK Migration | RouterLLM via LiteLLM tags — `internal/service/conversation_dispatch.go`, `workers/codeforge/llm.py`, `workers/codeforge/consumer/_conversation_routing.py` | ACI (shell for LLMs) | Per-Mode Tool Lists | History Processors | SWE-ReX Sandbox | Mini-SWE-Agent (100 LOC, 74% SWE-bench) | ToolFilterConfig (blocklist + conditional blocking)

## Competitor Analysis

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

## Implemented Phases

**Security & Trust (Phase 23):** Trust Annotations (4 levels, auto-stamped on NATS) — `internal/domain/trust/` | Message Quarantine (risk scoring, admin review) — `internal/service/quarantine.go`, migration 049 | Persistent Agent Identity (stats, state, capabilities, inbox) — `internal/domain/agent/agent.go` | War Room — `frontend/src/features/project/WarRoom.tsx` | A2A API keys (per tenant), quarantine of inbound A2A prompts and handoffs, `HandoffService` — `internal/middleware/a2a_auth.go`, `internal/service/handoff.go`

**Benchmark & Evaluation (Phase 26+28+5):** Phase 26: Provider interface, evaluator plugins (LLMJudge, FunctionalTest, SPARC, FilesystemState), 3 runners, external providers (HumanEval, MBPP, SWE-bench, DPAI Arena, Terminal-Bench). Phase 28 (R2E-Gym/EntroPO): Hybrid verification, trajectory verifier, multi-rollout, entropy-UCB1 MAB, DPO export, SWE-GEN — `workers/codeforge/evaluation/`. Phase 5: DPAI Arena, Terminal-Bench+FilesystemStateEvaluator, RLVR export (`GET /benchmarks/runs/{id}/export/rlvr`).

**Contract-First Review/Refactor (Phase 31):** Boundary Detection (LLM-based, API/data/inter-service/cross-language) — `internal/domain/boundary/` | Review-Refactor Pipeline (4-step: boundary_analyzer->contract_reviewer->reviewer->refactorer) — `internal/domain/pipeline/presets.go` | 2 Modes: `boundary_analyzer` (read-only, plan), `contract_reviewer` (read-only, review) — `internal/domain/mode/presets.go` | ReviewTriggerService (manual trigger) — `internal/service/review_trigger.go` | ReviewPipelineService (review plan, baseline record `review_pipelines`, keep/undo decision) — `internal/service/review_pipeline.go` | DiffImpactScorer (3-tier HITL) — `internal/service/diff_impact.go` | Phase-aware Context Budget (100%/60%/50%/70%) — `internal/service/context_budget.go` | waiting_approval status — `internal/service/orchestrator.go` | BoundaryService — `internal/service/boundary.go` | Frontend: RefactorApproval, BoundariesPanel — `frontend/src/features/project/` | Approval prompt: the `review.approval_required` WebSocket event (no NATS subject)

**Visual Design Canvas (Phase 32):** SVG canvas with 9 tools (select, rect, ellipse, freehand, text, annotate, image, polygon, node) — `frontend/src/features/canvas/` | Triple export: PNG (offscreen), ASCII (char grid), JSON | Smart output: vision->PNG+JSON, text-only->ASCII+JSON, basic->JSON | Multimodal pipeline: `MessageImage` Frontend->Go JSONB->NATS->Python content-array->LiteLLM | Migration `075_add_message_images.sql` | `buildCanvasPrompt()` uses `supports_vision` | Spec: `docs/features/06-visual-design-canvas.md`

## Roadmap Auto-Detection & Integration
No custom PM tool — sync with Plane, OpenProject, GitHub/GitLab Issues. Auto-Detection: 3-tier (repo files->platform APIs->file markers). Multi-Format SDD: OpenSpec (`openspec/`), Spec Kit (`.specify/`), Autospec (`specs/spec.yaml`). Provider Registry: `specprovider` + `pmprovider`, same architecture as Git. Bidirectional Sync: CodeForge <-> PM Tool <-> Repo Specs (Webhook/Poll/Manual). Adopted patterns: Plane (cursor pagination, HMAC-SHA256, label sync), OpenProject (optimistic locking, schema endpoints), OpenSpec (delta spec), Ploi Roadmap (`/ai`). Gitea/Forgejo: GitHub adapter works (compatible API). Details: `docs/research/market-analysis.md` Section 5.

## Protocol Support

| Protocol | Status | Purpose |
|---|---|---|
| **MCP** | Phase 15 | Agent<->Tool (JSON-RPC). Go: mcp-go SDK (tools: list_projects, get_project, get_run_status, get_cost_summary; resources: codeforge://projects, codeforge://costs/summary; registry with PG persistence, project assignment, HTTP CRUD). Python: McpWorkbench (multi-server, BM25 recommend, discovery, bridging). Frontend: MCPServersPage. Config: `mcp.enabled/servers_dir/server_port` (3001). Policy: `mcp:server:tool` glob matching. |
| **LSP** | Phase 15D | Code intelligence (go-to-def, refs, diagnostics). Go Core manages lifecycle per project language. |
| **A2A** | Phase 27 | Agent-to-Agent v0.3.0 (LF, Apache 2.0). Protobuf `lf.a2a.v1`, JSON-RPC 2.0/HTTPS, SSE, push notifications. 11 RPCs, 8 task states. Types: AgentCard, Task, Message, Part, Artifact. Security: API Key, Bearer/JWT, OAuth 2.0, OIDC, mTLS, JWS. Multi-tenant `/{tenant}/`. SDKs: Go/Python/JS/Java/.NET. Integration: agents as A2A servers, Go Core as client via `a2a-go`, NATS internal + A2A external. |
| **AG-UI** | Phase 17+ | Agent<->Frontend streaming (CopilotKit). 12 events (all prefixed `agui.`): run_started/finished, text_message, tool_call/result, state_delta, step_started/finished + CodeForge extensions permission_request, goal_proposal, action_suggestion, roadmap_proposal (`internal/domain/event/agui.go`). WS format across Go+TS (20+ files). |
| **OTEL GenAI** | implemented (opt-in, `otel.enabled`) | LLM/agent observability. Go: OTLP traces + metrics (`internal/adapter/otel/`), HTTP middleware, run/toolcall/delivery spans (`internal/telemetry/`). Python: `workers/codeforge/tracing/` with `gen_ai.*` LLM span attributes, OTLP metrics, `traceparent` on every worker publish. `otel.insecure` (default false) selects plaintext gRPC on both sides. Jaeger in `docker-compose.yml` (dev profile). LiteLLM OTEL callback not configured yet. |
| **Watch** | future | ANP (decentralized networking), LSAP (LSP for AI agents). |

MCP + A2A complementary: "MCP for tools, A2A for agents"
