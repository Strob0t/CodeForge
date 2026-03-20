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
Python AI Workers (LLM Calls, Agent Execution, LiteLLM, LangGraph)
```

| Layer | Language | Purpose |
|---|---|---|
| Frontend | TypeScript | Web GUI (SolidJS + Tailwind CSS) |
| Core Service | Go 1.25 | HTTP/WS Server, Scheduling, Repo Mgmt |
| AI Workers | Python 3.12 | LLM Integration, Agent Execution |
| Infrastructure | Docker | Containerization, Docker-in-Docker |

## Configuration & Tooling

- **YAML for all config files** (comments support) — Modes, Tool Bundles, Settings, Safety, Autonomy, Schedules
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

### Agent System
- **Execution Modes:** Sandbox (isolated container), Mount (direct file access), Hybrid
- **Safety Layer (8):** Budget Limiter, Command Safety Evaluator, Branch Isolation, Test/Lint Gate, Max Steps, Rollback, Path Blocklist, Stall Detection
- **Workflow:** Plan -> Approve -> Execute -> Review -> Deliver (configurable)
- **Autonomy Levels:** 1=supervised (approve all), 2=semi-auto (approve destructive), 3=auto-edit (approve terminal/deploy), 4=full-auto (safety rules replace user), 5=headless (CI/CD, cron, API)
- **Modes System:** YAML-configurable roles (architect, coder, reviewer, debugger), per-mode tools/LLM/autonomy/prompt, built-in + custom (`.codeforge/modes/`), DAG pipelines, schedule support
- **Tool Bundles:** YAML-based declarative tool definitions
- **History Processors:** Context window optimization pipeline
- **Hook System:** Observer pattern for agent/environment lifecycle
- **Trajectory:** Recording, replay, inspector, audit trail
- **Cost Management:** Budget limits per task/project/user, auto-tracking
- **Prompt Templates:** Go `text/template` in `.tmpl` files via `//go:embed`
- **BM25S Retrieval:** Code search and tool recommendation
- **SimHash Dedup:** 64-bit fingerprints, hamming distance threshold — `internal/service/dedup.go`
- **Real-time State:** WebSocket live updates for agent status, logs, costs

### Agentic Conversation Loop (Phase 17) — **implemented**
- Multi-turn tool-use loop: LLM -> tools -> results -> repeat. Go dispatches via NATS (`conversation.run.start/complete`)
- 7 built-in tools: Read, Write, Edit, Bash, Search, Glob, ListDir + MCP merge
- `AgentLoopExecutor` (Python): streaming LLM, per-tool policy, cost tracking
- `ConversationHistoryManager`: head-and-tail token budget, tool result truncation
- HITL: `DecisionAsk` -> WS `permission_request` -> HTTP approve/deny -> channel resume
- Config: `MaxLoopIterations` (50), `MaxContextTokens` (120K), `ContextEnabled` (true), `ContextBudget` (2048), `ContextPromptReserve` (512), `ApprovalTimeoutSeconds` (60)
- **Adaptive Context Budget:** Linear decay from `ContextBudget` to 0 over 60 messages — `internal/service/context_budget.go`
- **Auto-Indexing:** Clone/Adopt/Setup trigger RepoMap + Retrieval Index + GraphRAG — `internal/adapter/http/handlers.go`
- Key files: `workers/codeforge/agent_loop.py`, `workers/codeforge/tools/`, `internal/service/conversation.go`

### Chat Enhancements — **implemented**
HITL Permission UI (`PermissionRequestCard`: approve/deny/allow-always, countdown, preset mapping, `POST /policies/allow-always`), Inline Diff Review (`DiffPreview`), Action Buttons (copy/retry/apply/diff), Per-Message Cost (`MessageBadge`+`CostBreakdown` via AG-UI `state_delta`), Smart References (`@/#//` autocomplete, `AutocompletePopover`, `useFrequencyTracker`), Slash Commands (`/compact`/`/rewind`/`/clear`/`/help`/`/mode`/`/model` via `CommandRegistry`), Conversation Search (PostgreSQL FTS, GIN, `ts_rank`, `POST /search/conversations`), Notification Center (`notificationStore`, browser push, Web Audio, tab badge), Real-Time Channels (3 tables, 9 endpoints, WS events, `ChannelList`/`ChannelView`/`ThreadPanel`). Spec: `docs/features/05-chat-enhancements.md`

### Framework & Agent Insights (adopted patterns)

**From LangGraph, CrewAI, AutoGen, MetaGPT:**
Composite Memory Scoring (Semantic+Recency+Importance) — `workers/codeforge/memory/scorer.py`, `internal/service/memory.go` | Context Window Strategies (Buffered/TokenLimited/HeadAndTail) | Experience Pool (@exp_cache) — `workers/codeforge/memory/experience.py`, `internal/service/experience_pool.go` | Tool Recommendation via BM25 | Workbench (tool container, shared state, MCP) | LLM Guardrail Agent | Structured Output/ActionNode (schema validation + review/revise) | Event Bus (Agent/Task/System -> WS) | GraphFlow/DAG (Conditional Edges, Parallel Nodes, Cycles) | Composable Termination (MaxSteps|Budget|Timeout) | Component System (JSON serializable, GUI editor) | Document Pipeline PRD->Design->Tasks->Code | MagenticOne Planning Loop (Stall Detection + Re-Planning) | HandoffMessage Pattern — `internal/domain/orchestration/handoff.go`, `internal/service/handoff.go`, `workers/codeforge/tools/handoff.py` | Human Feedback Provider (Web GUI, Slack, Email) — `internal/port/feedback/provider.go`, `internal/adapter/slack/feedback.go`, `internal/adapter/email/feedback.go`

**From Cline, Devika:**
Plan/Act Mode (**implemented**: `workers/codeforge/plan_act.py`, `internal/service/conversation_agent.go`) | Shadow Git Checkpoints | Ask/Say Approval Pattern | MCP extensibility | .clinerules-like YAML config | Auto-Compact (~80% window) | Diff-based File Review | Sub-Agent Architecture | Agent State Visualization | LLM-driven Web Crawler | Stateless Agent Design (state in core)

**From OpenHands, SWE-agent:**
Event-Sourcing (EventStream) | Workspace Abstraction (Local/Docker/Remote, self-healing) | AgentHub (CodeAct, Browsing, Delegator, Microagents) | Microagents (YAML+MD trigger-driven) — `internal/domain/microagent/`, `internal/service/microagent.go` | Skills System — `workers/codeforge/skills/`, `internal/service/skill.go` | Risk Management (LLMSecurityAnalyzer) | V0->V1 SDK Migration | RouterLLM via LiteLLM tags — `internal/service/conversation.go`, `workers/codeforge/consumer.py` | ACI (shell for LLMs) | Tool Bundles (YAML) | History Processors | SWE-ReX Sandbox | Mini-SWE-Agent (100 LOC, 74% SWE-bench) | ToolFilterConfig (blocklist + conditional blocking)

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

**Security & Trust (Phase 23):** Trust Annotations (4 levels, auto-stamped on NATS) — `internal/domain/trust/` | Message Quarantine (risk scoring, admin review) — `internal/service/quarantine.go`, migration 049 | Persistent Agent Identity (fingerprint, stats, inbox) — `internal/domain/agent/agent.go` | War Room — `frontend/src/features/project/WarRoom.tsx`

**Benchmark & Evaluation (Phase 26+28+5):** Phase 26: Provider interface, evaluator plugins (LLMJudge, FunctionalTest, SPARC, FilesystemState), 3 runners, external providers (HumanEval, MBPP, SWE-bench, DPAI Arena, Terminal-Bench). Phase 28 (R2E-Gym/EntroPO): Hybrid verification, trajectory verifier, multi-rollout, entropy-UCB1 MAB, DPO export, SWE-GEN — `workers/codeforge/evaluation/`. Phase 5: DPAI Arena, Terminal-Bench+FilesystemStateEvaluator, RLVR export (`GET /benchmarks/runs/{id}/export/rlvr`).

**Contract-First Review/Refactor (Phase 31):** Boundary Detection (LLM-based, API/data/inter-service/cross-language) — `internal/domain/boundary/` | Review-Refactor Pipeline (4-step: boundary-analyzer->contract-reviewer->reviewer->refactorer) — `internal/domain/pipeline/presets.go` | 2 Modes: `boundary-analyzer` (read-only, plan), `contract-reviewer` (read-only, review) — `internal/domain/mode/presets.go` | ReviewTriggerService (cascade triggers, SHA dedup) — `internal/service/review_trigger.go` | DiffImpactScorer (3-tier HITL) — `internal/service/diff_impact.go` | Phase-aware Context Budget (100%/60%/50%/70%) — `internal/service/context_budget.go` | waiting_approval status — `internal/service/orchestrator.go` | BoundaryService — `internal/service/boundary.go` | Frontend: RefactorApproval, BoundariesPanel — `frontend/src/features/project/` | NATS: `review.>` wildcard — `internal/port/messagequeue/subjects.go`

**Visual Design Canvas (Phase 32):** SVG canvas with 7 tools (select, rect, ellipse, freehand, text, annotate, image) — `frontend/src/features/canvas/` | Triple export: PNG (offscreen), ASCII (char grid), JSON | Smart output: vision->PNG+JSON, text-only->ASCII+JSON, basic->JSON | Multimodal pipeline: `MessageImage` Frontend->Go JSONB->NATS->Python content-array->LiteLLM | Migration `075_add_message_images.sql` | `buildCanvasPrompt()` uses `supports_vision` | Spec: `docs/features/06-visual-design-canvas.md`

### Roadmap Auto-Detection & Integration
No custom PM tool — sync with Plane, OpenProject, GitHub/GitLab Issues. Auto-Detection: 3-tier (repo files->platform APIs->file markers). Multi-Format SDD: OpenSpec (`openspec/`), Spec Kit (`.specify/`), Autospec (`specs/spec.yaml`). Provider Registry: `specprovider` + `pmprovider`, same architecture as Git. Bidirectional Sync: CodeForge <-> PM Tool <-> Repo Specs (Webhook/Poll/Manual). Adopted patterns: Plane (cursor pagination, HMAC-SHA256, label sync), OpenProject (optimistic locking, schema endpoints), OpenSpec (delta spec), Ploi Roadmap (`/ai`). Gitea/Forgejo: GitHub adapter works (compatible API). Details: `docs/research/market-analysis.md` Section 5.

### Database & Libraries

**PostgreSQL 18** (shared with LiteLLM, schema separation): Go pgx v5 + goose migrations, Python psycopg3 (sync+async), NATS JetStream KV for ephemeral state. ADR: `docs/architecture/adr/002-postgresql-database.md`

**Go Libraries (minimal-dep):** chi v5 (router), coder/websocket v1.8+ (WS), os/exec git CLI wrapper. NOT used: Echo/Fiber, gorilla/websocket, go-git.

**Frontend Libraries (minimal-stack):** @solidjs/router, Tailwind CSS (no component lib), @solid-primitives/websocket, native fetch + thin wrapper, SolidJS signals/stores/context, Unicode+inline SVG icons, Outfit+Source Sans 3 fonts (self-hosted woff2 in `frontend/public/fonts/`), design system at `/design-system` (dev-only, docs in `frontend/src/ui/DESIGN-SYSTEM.md`), onboarding wizard (`codeforge-onboarding-completed` key) — `frontend/src/features/onboarding/OnboardingWizard.tsx`. NOT used: axios, styled-components, Kobalte, shadcn-solid, Socket.IO, Redux/Zustand.

### Protocol Support

| Protocol | Status | Purpose |
|---|---|---|
| **MCP** | Phase 15 | Agent<->Tool (JSON-RPC). Go: mcp-go SDK (tools: list_projects, get_project, get_run_status, get_cost_summary; resources: codeforge://projects, codeforge://costs/summary; registry with PG persistence, project assignment, HTTP CRUD). Python: McpWorkbench (multi-server, BM25 recommend, discovery, bridging). Frontend: MCPServersPage. Config: `mcp.enabled/servers_dir/server_port` (3001). Policy: `mcp:server:tool` glob matching. |
| **LSP** | Phase 15D | Code intelligence (go-to-def, refs, diagnostics). Go Core manages lifecycle per project language. |
| **A2A** | Phase 27 | Agent-to-Agent v0.3.0 (LF, Apache 2.0). Protobuf `lf.a2a.v1`, JSON-RPC 2.0/HTTPS, SSE, push notifications. 11 RPCs, 8 task states. Types: AgentCard, Task, Message, Part, Artifact. Security: API Key, Bearer/JWT, OAuth 2.0, OIDC, mTLS, JWS. Multi-tenant `/{tenant}/`. SDKs: Go/Python/JS/Java/.NET. Integration: agents as A2A servers, Go Core as client via `a2a-go`, NATS internal + A2A external. |
| **AG-UI** | Phase 17+ | Agent<->Frontend streaming (CopilotKit). 8 events: run_started/finished, text_message, tool_call/result, state_delta, step_started/finished. WS format across Go+TS (20+ files). |
| **OTEL GenAI** | planned | LLM/agent observability. LiteLLM exports OTEL natively, Go adds agent spans. |
| **Watch** | future | ANP (decentralized networking), LSAP (LSP for AI agents). |

MCP + A2A complementary: "MCP for tools, A2A for agents"

### LLM Integration
- **LiteLLM Proxy** as Docker sidecar (port 4000) — no custom LLM provider interface
- Go + Python communicate via OpenAI-compatible API against LiteLLM
- **Hybrid Routing (Phase 29):** 3-layer cascade: ComplexityAnalyzer (rule-based, <1ms) -> MABModelSelector (UCB1) -> LLMMetaRouter (cold-start). Enable: `CODEFORGE_ROUTING_ENABLED=true`. Package: `workers/codeforge/routing/`
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
| 008 | Benchmark: DeepEval + AgentNeo + GEMMAS | `docs/architecture/adr/008-benchmark-evaluation-framework.md` |

**Infrastructure Principles:**
- **Zero-config startup** — system runs with defaults; CLI flags have highest precedence
- **Async-first:** Logging, NATS, LLM calls never block hot path. Buffered channels + workers (Go), QueueHandler + QueueListener (Python)
- **Docker-native logging:** Structured JSON to stdout, `docker compose logs` + `jq` for debugging
- **Policy Layer:** Declarative YAML, first-match-wins, 4 built-in presets, extensible without code
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
- Subjects must match EXACTLY: Go (`internal/port/messagequeue/subjects.go`) <-> Python (`workers/codeforge/consumer/_subjects.py`)
- JetStream config (`internal/port/messagequeue/jetstream.go`) must include wildcards for new prefixes (e.g. `benchmark.>`)
- New subjects need publisher (one side) + subscriber (other side)

### JSON Payload Contracts
- Go JSON tags must match Python Pydantic field names exactly
- Sync changes: Go (`internal/port/messagequeue/schemas.go` / domain) <-> Python (`workers/codeforge/models.py`)
- Test round-trip both directions
- Type mapping: Go `int64`/`float64`/`time.Time` <-> Python `int`/`float`/`datetime`

### API Keys & Secrets
- NEVER hardcode — always env vars. LiteLLM auth: `LITELLM_MASTER_KEY` (default: `sk-codeforge-dev`)
- Verify model has valid key in `litellm-config.yaml`

### Method Signatures & Interfaces
- Python `LiteLLMClient.chat_completion()` (NOT `chat()`) — check callers + fakes
- Go interface changes: grep ALL implementations including test mocks
- `golangci-lint hugeParam`: structs >80 bytes -> pointer

### Path Resolution
- Frontend sends dataset names -> Go resolves to absolute paths -> NATS -> Python receives absolute paths
- Verify: `internal/service/benchmark.go` `resolveDatasetPath()`

### Idempotency
- JetStream redelivers unacked messages — handlers must be idempotent
- Duplicate guards (skip if already `"completed"`), always `msg.ack()` even on error

### Error Handling
- `except Exception as exc:` (NOT bare), log `error=str(exc)`, publish errors back to NATS

### Tenant Isolation
- ALL tenant-scoped queries: `AND tenant_id = $N` with `tenantFromCtx(ctx)` (exceptions: user/token/tenant mgmt)
- LIMIT via `$N` placeholders, not `%d` interpolation
- NATS payloads MUST carry `tenant_id` for background jobs -> `tenantctx.WithTenant(ctx, payload.TenantID)`
- Reference: `store.go:GetProject` (correct), `store_a2a.go` (fixed)

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
# 2. Go backend (APP_ENV=development required — dev endpoints return 403 without it)
APP_ENV=development go run ./cmd/codeforge/
# 3. Frontend
cd frontend && npm run dev
# 4. Tests
cd frontend && npx playwright test
```

- `/health` exposes `dev_mode: true/false` | Backend: 8080, Frontend: 3000
- Credentials: `admin@localhost` / `Changeme123` | Playwright: chromium, workers:1, retries:1

### LLM E2E Tests (API-Level, no browser)

```bash
cd frontend && npx playwright test --config=playwright.llm.config.ts
```

95 tests, 12 specs in `frontend/e2e/llm/`. Helper: `frontend/e2e/llm/llm-helpers.ts`. Config: `frontend/playwright.llm.config.ts`

### Autonomous Goal-to-Program Test (Playwright-MCP)

Testplan: `docs/testing/2026-03-19-autonomous-goal-to-program-testplan.md` | Tool complexity: `docs/plans/tool-call-complexity-plan.md`

**Critical Startup Sequence (in order):**
1. `docker compose up -d postgres nats litellm`
2. Resolve container IPs (`localhost` ports BROKEN in WSL2):
   ```bash
   NATS_IP=$(docker inspect codeforge-nats | grep -m1 '"IPAddress"' | grep -oP '[\d.]+')
   LITELLM_IP=$(docker inspect codeforge-litellm | grep -m1 '"IPAddress"' | grep -oP '[\d.]+')
   POSTGRES_IP=$(docker inspect codeforge-postgres | grep -m1 '"IPAddress"' | grep -oP '[\d.]+')
   ```
3. Purge NATS JetStream (`await js.purge_stream('CODEFORGE')` + delete stale consumers)
4. Start Go backend: `APP_ENV=development go run ./cmd/codeforge/`
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

**Key env vars:** `LITELLM_BASE_URL` (NOT `LITELLM_URL`), `CODEFORGE_ROUTING_ENABLED=false` (router picks unhealthy models), auth field: `access_token` (NOT `token`)

**Project setup:**
- Create project: `POST /projects` with `config: {"autonomy_level": "4", "policy_preset": "trusted-mount-autonomous", "execution_mode": "mount"}`
- **MUST adopt workspace separately:** `POST /projects/{id}/adopt` with `{"path": "/abs/path"}` — CreateProject ignores `local_path` in body
- TestRepo clone fails often — use local workspace creation instead
- Auto-onboarding disabled (ChatPanel.tsx)
- Model: `"openai/container"` (or any healthy model)

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
├── todo.md                 # Central TODO (single source of truth)
├── architecture.md         # System architecture
├── dev-setup.md            # Setup guide
├── project-status.md       # Phase tracking
├── tech-stack.md           # Dependencies
├── features/               # Per-pillar specs (01-06)
├── specs/                  # Design specs (*-design.md)
├── plans/                  # Implementation plans (*-plan.md)
├── testing/                # Test plans + reports
├── audits/                 # Schema, UX, code audits
├── architecture/adr/       # ADRs (use _template.md)
├── research/               # Market research
└── prompts/                # Prompt templates
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
