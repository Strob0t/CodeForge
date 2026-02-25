# CodeForge -- Architecture

### Purpose

CodeForge is a containerized service for orchestrating AI coding agents. The architecture follows a three-layer model with strict language separation by responsibility.

### System Architecture

```text
┌─────────────────────────────────────────────────────┐
│                  TypeScript Frontend                 │
│                     (SolidJS)                        │
│                                                     │
│  ┌─────────┐  ┌──────────┐  ┌────────┐  ┌────────┐ │
│  │ Project  │  │ Roadmap/ │  │  LLM   │  │ Agent  │ │
│  │Dashboard │  │FeatureMap│  │Provider│  │Monitor │ │
│  └─────────┘  └──────────┘  └────────┘  └────────┘ │
└────────────────────┬────────────────────────────────┘
                     │ REST / WebSocket
┌────────────────────▼────────────────────────────────┐
│                  Go Core Service                     │
│                                                     │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐          │
│  │ HTTP/WS  │  │  Agent   │  │   Repo   │          │
│  │ Server   │  │Lifecycle │  │ Manager  │          │
│  └──────────┘  └──────────┘  └──────────┘          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐          │
│  │Scheduling│  │  Auth /  │  │  Queue   │          │
│  │ Engine   │  │ Sessions │  │ Producer │          │
│  └──────────┘  └──────────┘  └──────────┘          │
│  ┌──────────┐  ┌──────────┐                        │
│  │Auto-Detect│  │ PM Sync  │                        │
│  │ Engine   │  │ Service  │                        │
│  └──────────┘  └──────────┘                        │
└────────────┬────────────────────────┬───────────────┘
             │  Message Queue         │
             │  (NATS JetStream)      │
┌────────────▼──────┐  ┌─────────────▼───────────────┐
│  Python Worker 1  │  │  Python Worker N            │
│                   │  │                             │
│  ┌─────────────┐  │  │  ┌─────────────┐           │
│  │  LangGraph  │  │  │  │  LangGraph  │           │
│  │  (Agents)   │  │  │  │  (Agents)   │           │
│  └─────────────┘  │  │  └─────────────┘           │
│  ┌─────────────┐  │  │  ┌─────────────┐           │
│  │ Agent Exec  │  │  │  │ Agent Exec  │           │
│  │(Aider, etc.)│  │  │  │(OpenHands)  │           │
│  └─────────────┘  │  │  └─────────────┘           │
└────────┬──────────┘  └──────────┬──────────────────┘
         │  OpenAI-compatible API │
┌────────▼────────────────────────▼──────────────────┐
│            LiteLLM Proxy (Sidecar)                  │
│  127+ Provider │ Routing │ Budgets │ Cost-Tracking  │
└────────────────────────┬───────────────────────────┘
                         │ Provider APIs
┌────────────────────────▼───────────────────────────┐
│  OpenAI │ Anthropic │ Ollama │ Bedrock │ OpenRouter │
└────────────────────────────────────────────────────┘
```

### Layers in Detail

#### Frontend (TypeScript)

The frontend serves as the web GUI for all user interactions. It communicates with the Go Core via REST API for CRUD and WebSocket for real-time updates (agent logs, status).

**Core modules** include Project Dashboard (manage repos, status overview), Roadmap/Feature-Map Editor (visual, OpenSpec-compatible), LLM Provider Management (configuration, cost tracking), and Agent Monitor (live logs, task status, results).

#### Core Service (Go)

The core service provides a performant backend for HTTP, WebSocket, scheduling, and coordination. Go was chosen for native concurrency (goroutines), minimal RAM (~10-20MB), fast startup times, and excellent performance for thousands of simultaneous connections.

**Core modules** include HTTP/WebSocket Server, Agent Lifecycle Management (Start, Stop, Status, Restart), Repo Manager (Git, GitHub, GitLab, SVN Integration), Scheduling Engine (task queue, prioritization), Auth / Sessions / Multi-Tenancy, and Queue Producer (dispatch jobs to Python Workers).

#### AI Workers (Python)

The AI workers handle LLM interaction and agent execution. Python was chosen for native access to the AI ecosystem (LiteLLM, LangGraph, all LLM SDKs). Workers scale horizontally via Message Queue, supporting any number of worker instances.

**Core modules** include LiteLLM Integration (multi-provider routing: OpenAI, Claude, Ollama, etc.), Agent Execution (Aider, OpenHands, SWE-agent, Goose, OpenCode, Plandex as swappable backends), and LangGraph Orchestration (for complex multi-agent workflows).

### Communication Between Layers

| From -> To | Protocol | Purpose |
|---|---|---|
| Frontend -> Go | REST (HTTP/2) | CRUD Operations |
| Frontend -> Go | WebSocket | Real-time updates, logs |
| Go -> Python Workers | NATS JetStream | Job dispatch (subject-based routing) |
| Python Workers -> Go | NATS JetStream | Results, status updates |
| Go -> LiteLLM Proxy | HTTP (OpenAI format) | Config management, health checks |
| Python Workers -> LiteLLM Proxy | HTTP (OpenAI format) | LLM calls (`litellm.completion()`) |
| LiteLLM Proxy -> LLM APIs | HTTPS | Provider-specific API calls |
| Go -> SCM (Git/SVN) | CLI / REST API | Repo operations |
| Go -> Ollama/LM Studio | HTTP | Local Model Auto-Discovery |
| Go -> PM Platforms | REST API / Webhooks | Bidirectional PM sync (Plane, OpenProject, etc.) |
| Go -> Repo Specs | Filesystem | Spec detection and sync (OpenSpec, Spec Kit, Autospec) |
| Go <-> Tools/Agents | MCP (JSON-RPC) | Tool integration (server: expose tools, client: connect external) |
| Go -> LSP Servers | LSP (JSON-RPC) | Code intelligence per project language |
| Go -> OTEL Collector | OTLP (gRPC/HTTP) | Agent lifecycle traces, metrics |
| Python -> OTEL Collector | OTLP (gRPC/HTTP) | LLM call traces, token metrics |
| Frontend <- Go | AG-UI events (Phase 2-3) | Standardized agent output streaming |
| External Agents <-> Go | A2A (Phase 2-3) | Agent discovery via Agent Cards, task delegation |

### Protocol Support

CodeForge integrates with standardized protocols for tool integration, agent coordination, frontend streaming, code intelligence, and observability.

#### Tier 1: Essential (Phase 1-2)

| Protocol | Purpose | Standard | Integration Point |
|---|---|---|---|
| MCP (Model Context Protocol) | Agent <-> Tool communication | JSON-RPC 2.0 over stdio/SSE/HTTP (Anthropic) | **Implemented (Phase 15).** Go Core: MCP server via mcp-go SDK (4 tools, 2 resources, auth middleware, Streamable HTTP transport on port 3001). MCP server registry with PostgreSQL persistence, project-level assignment, 10 HTTP CRUD endpoints. Python Workers: McpWorkbench (multi-server container, BM25 tool recommendation). Frontend: MCPServersPage. Policy: `mcp:server:tool` glob matching |
| LSP (Language Server Protocol) | Code intelligence for agents | JSON-RPC over stdio/TCP (Microsoft) | **Implemented (Phase 15D).** Go Core: LSP client with JSON-RPC transport over stdio. Per-project language server lifecycle management. 8 HTTP endpoints under `/projects/{id}/lsp/`. Context enrichment with diagnostics. Frontend: LSPPanel |
| OpenTelemetry GenAI | Standardized LLM/agent observability | OTEL Semantic Conventions (CNCF) | LiteLLM exports OTEL traces natively. Go Core adds spans for agent lifecycle. Feeds Cost Dashboard + audit trails |

#### Tier 2: Important (Phase 2-3)

| Protocol | Purpose | Standard | Integration Point |
|---|---|---|---|
| A2A (Agent-to-Agent Protocol) | Peer-to-peer agent coordination | Agent Cards + Tasks over HTTP/SSE (Google -> Linux Foundation AAIF) | Agent backends register as A2A agents with capability cards. External agents discover and delegate to CodeForge |
| AG-UI (Agent-User Interaction Protocol) | Bi-directional agent <-> frontend streaming | JSON events over HTTP (CopilotKit) | Frontend WebSocket protocol follows AG-UI event format. Lifecycle events: TEXT_MESSAGE, TOOL_CALL, STATE_DELTA. Human-in-the-loop built in |

#### Tier 3: Future / Watch

| Protocol | Purpose | Notes |
|---|---|---|
| ANP (Agent Network Protocol) | Decentralized agent communication over internet | Early stage, W3C DIDs. Relevant when agents talk to external agent networks |
| LSAP (Language Server Agent Protocol) | LSP extension for AI agents | Emerging proposal, extends LSP with AI-specific capabilities |

#### Protocol Architecture

```text
┌─────────────────────────────────────────────────────┐
│              TypeScript Frontend                     │
│                                                     │
│   AG-UI Events <-> Agent output streaming            │
│   (TEXT_MESSAGE, TOOL_CALL, STATE_DELTA, APPROVAL)  │
└────────────────────┬────────────────────────────────┘
                     │ WebSocket (AG-UI event format)
┌────────────────────▼────────────────────────────────┐
│              Go Core Service                         │
│                                                     │
│   MCP Server <-> Expose CodeForge tools              │
│   MCP Client <-> Connect to external MCP servers     │
│   LSP Client <-> Code intelligence per language      │
│   A2A Server <-> Agent Cards, task delegation        │
│   OTEL SDK   <-> Traces, metrics -> collector         │
└────────────────────┬────────────────────────────────┘
                     │ NATS JetStream
┌────────────────────▼────────────────────────────────┐
│              Python Workers                          │
│                                                     │
│   MCP Client <-> Tool access for agents              │
│   OTEL SDK   <-> LLM call traces, token metrics      │
└─────────────────────────────────────────────────────┘
```

### Design Decisions

#### Why not everything in Python?

Go handles a fraction of the resources under the same load. A Go HTTP server scales effortlessly to tens of thousands of simultaneous connections. In Python you need significantly more tuning and instances for that.

#### Why not everything in Go?

The entire AI/agent ecosystem (LiteLLM, LangGraph, Aider, OpenHands, SWE-agent, all LLM SDKs) is Python. Connecting everything via bridges would be more overhead than dedicated Python workers.

#### Why Message Queue instead of direct calls?

Decoupling means the Go service does not have to wait for slow LLM calls. Workers are horizontally scalable. Jobs are not lost when a worker crashes (resilience). The queue buffers during load spikes (backpressure).

#### Why YAML as the uniform configuration format?

All configuration files in CodeForge use YAML with no exceptions. This applies to agent modes and specializations, tool bundles and tool definitions, project settings and safety rules, autonomy configuration, LiteLLM config (natively YAML), and prompt metadata (Jinja2 templates themselves remain `.jinja2`).

**YAML supports comments**, which is critical for documentation directly in the config (`# Why this budget limit?`), temporarily disabling settings (`# tools: [terminal]`), onboarding (contributors understand configs without external documentation), and versioning (comments explain changes in the Git diff).

JSON is not used for configuration files. JSON remains for API responses, event serialization, and internal data exchange.

### Software Architecture: Hexagonal + Provider Registry

#### Core Principle: Hexagonal Architecture (Ports and Adapters)

The core logic (domain + services) is completely isolated from external systems. All dependencies point inward, never outward.

```text
┌──────────────────────────────────────────────────────────┐
│                    ADAPTERS (outer)                        │
│  HTTP Handlers, GitHub, Postgres, NATS, Ollama, Aider     │
│                                                          │
│    ┌──────────────────────────────────────────────┐       │
│    │              PORTS (boundary)               │       │
│    │    Go Interfaces — define WHAT the           │       │
│    │    core logic needs, not HOW                 │       │
│    │                                              │       │
│    │    ┌──────────────────────────────┐          │       │
│    │    │        DOMAIN (core)        │          │       │
│    │    │   Business logic, entities  │          │       │
│    │    │   Rules, validation         │          │       │
│    │    │   Zero external imports     │          │       │
│    │    └──────────────────────────────┘          │       │
│    └──────────────────────────────────────────────┘       │
└──────────────────────────────────────────────────────────┘
```

#### Provider Registry Pattern

For open-source extensibility, CodeForge uses a self-registering provider pattern. New implementations (e.g., a Gitea adapter) require a Go package that satisfies the corresponding interface, a blank import in `cmd/codeforge/providers.go`, and no changes to the core logic.

```text
1. Port defines interface + registry
   (Register, New, Available)

2. Adapter implements interface
   and registers itself via init()

3. Blank import in providers.go
   activates the adapter

4. Core logic only uses the interface —
   does not know which adapter is behind it
```

This pattern follows the Go standard pattern (`database/sql` + `_ "github.com/lib/pq"`).

#### Provider Types

| Port | Interface | Example Adapters |
|---|---|---|
| `gitprovider` | `Provider` | github, gitlab, gitlocal, svn, gitea |
| `agentbackend` | `Backend` | aider, openhands, sweagent, goose, opencode, plandex |
| `specprovider` | `SpecProvider` | openspec, speckit, autospec |
| `pmprovider` | `PMProvider` | plane, openproject, github_pm, gitlab_pm |
| `database` | `Store` | postgres |
| `messagequeue` | `Queue` | nats |

#### Capabilities

Not every provider supports all operations. Instead of empty implementations, each provider declares its capabilities.

```go
type Capability string
const (
    CapClone    Capability = "clone"
    CapWebhooks Capability = "webhooks"
    CapPRs      Capability = "pull_requests"
    // ...
)
```

The core logic and the frontend check capabilities and adapt their behavior accordingly. SVN does not support webhooks, for example. That is not an error but declared behavior.

#### Compliance Tests

Each provider type ships a reusable test suite (`RunComplianceTests`). A new adapter calls this function and automatically receives all interface tests. Contributors write minimal test code and get maximum coverage.

#### Go Core Directory Structure

```text
cmd/
  codeforge/
    main.go              # Entry point, dependency injection
    providers.go         # Blank imports of all active adapters
internal/
  domain/                # Core: Entities, business rules (zero external imports)
    project/
    agent/
    roadmap/
  port/                  # Interfaces + registries
    gitprovider/
      provider.go        # Interface + capability definitions
      registry.go        # Register(), New(), Available()
      compliance_test.go # Reusable test suite
    agentbackend/
    specprovider/
      provider.go        # SpecProvider Interface (Detect, ReadSpecs, WriteChange, Watch)
      registry.go        # Register(), New(), Available()
    pmprovider/
      provider.go        # PMProvider Interface (Detect, SyncItems, CreateItem, Webhooks)
      registry.go        # Register(), New(), Available()
    database/
    messagequeue/
  adapter/               # Concrete implementations
    github/
    gitlab/
    gitlocal/
    svn/
    litellm/             # LiteLLM config management adapter
    aider/
    openhands/
    sweagent/
    goose/               # Goose agent backend (Priority 1)
    opencode/            # OpenCode agent backend (Priority 1)
    plandex/             # Plandex agent backend (Priority 1)
    openspec/            # OpenSpec spec adapter
    speckit/             # GitHub Spec Kit adapter
    autospec/            # Autospec adapter
    plane/               # Plane.so PM adapter
    openproject/         # OpenProject PM adapter
    github_pm/           # GitHub Issues/Projects PM adapter
    gitlab_pm/           # GitLab Issues/Boards PM adapter
    postgres/
    nats/
  service/               # Use cases (connects domain with ports)
```

### Infrastructure Patterns (Implemented)

#### Reliability

**Circuit Breaker** (`internal/resilience/breaker.go`) provides a zero-dependency circuit breaker for external service calls (NATS Publish, LiteLLM API).

```text
Closed ──(failure count >= maxFailures)──> Open
Open   ──(timeout elapsed)──────────────> Half-Open
Half-Open ──(success)───────────────────> Closed
Half-Open ──(failure)───────────────────> Open
```

The breaker has configurable `maxFailures` and `timeout` from `config.Breaker`. It is injected via `SetBreaker()` on NATS and LiteLLM adapters. This prevents cascading failures when downstream services are unavailable.

**Idempotency Middleware** (`internal/middleware/idempotency.go`) deduplicates mutating HTTP requests (POST/PUT/DELETE). The client sends an `Idempotency-Key` header. The first request executes, captures the response (status + headers + body), and stores it in NATS JetStream KV (24h TTL). Subsequent requests with the same key replay the cached response without re-executing. Response body is capped at 1 MB with best-effort storage (failures don't error the response). GET/HEAD/OPTIONS bypass the middleware entirely.

**Dead Letter Queue** handles failed NATS messages by retrying 3 times with `NakWithDelay(2s)`, then moving to `{subject}.dlq`. In Go, `moveToDLQ()` publishes to the DLQ subject and acks the original message. In Python, `_move_to_dlq()` uses the same retry counting via `Retry-Count` header. Invalid schema messages go directly to the DLQ with no retries.

**Graceful Shutdown** follows a 4-phase ordered sequence: HTTP server, cancel NATS subscribers, NATS Drain, PostgreSQL pool close.

#### Performance

**Tiered Cache** (L1 + L2) provides a two-level caching strategy.

```text
L1: Ristretto (in-process)     L2: NATS JetStream KV (distributed)
    100 MB, ~1ns reads              10-minute TTL, shared across instances

Get: L1 hit? -> return
     L2 hit? -> backfill L1 -> return
     Miss?   -> return not-found

Set: Write L1 + L2 (sequential)
Delete: Remove L1 + L2 (sequential)
```

The port lives at `internal/port/cache/cache.go` (Get/Set/Delete interface). Adapters live at `internal/adapter/ristretto/`, `internal/adapter/natskv/`, and `internal/adapter/tiered/`. L1 backfill uses shorter TTL (5 min) to prevent stale data.

**Rate Limiting** (`internal/middleware/ratelimit.go`) implements a token bucket rate limiter per IP address. It has configurable `requests_per_second` and `burst` from config. Response headers follow GitHub-style conventions: `X-RateLimit-Remaining`, `X-RateLimit-Reset`. It returns 429 with `Retry-After` header when the limit is exceeded.

#### Agent Execution

**Policy Layer** (`internal/service/policy.go`) evaluates first-match-wins permission rules per tool call. It ships with 4 built-in presets: `plan-readonly`, `headless-safe-sandbox`, `headless-permissive-sandbox`, and `trusted-mount-autonomous`. Custom YAML policies load from a configurable directory. Evaluation follows this order: tool specifier matching, path constraints (glob), command constraints (prefix), mode fallback. Quality gates require tests/lint pass with rollback on failure. Termination conditions include max steps, timeout, max cost, and stall detection. ADR: [007-policy-layer](architecture/adr/007-policy-layer.md).

**Runtime API** (`internal/service/runtime.go`) provides a step-by-step execution protocol between Go Control Plane and Python Workers. NATS subjects include `runs.start`, `runs.toolcall.{request,response,result}`, `runs.complete`, `runs.cancel`, and `runs.output`. The protocol enforces per-tool-call policy: Python requests permission, Go evaluates policy, Go responds allow/deny/ask. Termination enforcement checks max steps, max cost, timeout, and stall detection per tool call. Quality gate orchestration works as follows: Go triggers gate request, Python runs test/lint, Go processes result. Five deliver modes are supported: none, patch, commit-local, branch, and PR. ADR: [006-agent-execution-approach-c](architecture/adr/006-agent-execution-approach-c.md).

**Checkpoint System** (`internal/service/checkpoint.go`) creates shadow Git commits for safe rollback during agent execution. `CreateCheckpoint` runs `git add -A && git commit` after each file-modifying tool call. `RewindToFirst` runs `git reset --hard {first}^` to restore pre-run state on quality gate failure. `RewindToLast` runs `git reset --hard {last}^` to undo only the last change. `CleanupCheckpoints` runs `git reset --soft {first}^` to remove shadow commits while keeping the working tree.

**Docker Sandbox** (`internal/service/sandbox.go`) manages container lifecycle for isolated agent execution. It supports a Create, Start, Exec, Stop, Remove lifecycle via Docker CLI (`os/exec`). Resource limits include memory, CPU quota, PID limit, and network mode (default: `none`). A three-layer limit hierarchy applies: config defaults, then policy limits, then agent limits, capped at ceiling. The root filesystem is read-only with tmpfs `/tmp`.

#### Agentic Conversation Loop (Phase 17)

The agentic loop makes CodeForge an autonomous coding agent. When a user sends a message in the Chat UI, the system dispatches to a Python worker that runs a multi-turn tool-use loop: LLM decides which tools to call, tools execute, results feed back, and the loop continues until the task is done.

```text
┌──────────────┐                  ┌─────────────────┐
│   Frontend   │  WebSocket       │   Go Core        │
│  (SolidJS)   │◄─────────────── │  ConversationSvc │
│              │  AG-UI events    │                  │
│  ChatPanel   │                  │  1. Store user   │
│  ToolCards   │  POST /messages  │     message      │
│  Approval UI │ ────────────────►│  2. Build context│
└──────────────┘                  │  3. Publish NATS │
                                  └────────┬────────┘
                                           │ conversation.run.start
                                  ┌────────▼────────┐
                                  │  Python Worker   │
                                  │  AgentLoopExec   │
                                  │                  │
                                  │  LOOP:           │
                                  │  a. LLM call     │─── stream text ──► WS
                                  │  b. tool_calls?  │
                                  │     ├─ policy    │─── runs.toolcall ─► Go
                                  │     ├─ execute   │    (allow/deny/ask)
                                  │     └─ result    │
                                  │  c. no tools?    │
                                  │     └─ BREAK     │
                                  └────────┬────────┘
                                           │ conversation.run.complete
                                  ┌────────▼────────┐
                                  │   Go Core        │
                                  │  1. Store tool   │
                                  │     messages     │
                                  │  2. Store reply  │
                                  │  3. Broadcast    │
                                  │     run_finished │
                                  └─────────────────┘
```

**Conversation Service** (`internal/service/conversation.go`) provides two paths: simple (single LLM call for projects without workspaces) and agentic (multi-turn tool loop). `IsAgentic()` determines the mode from the request override, project config, and workspace presence. `SendMessageAgentic()` stores the user message, loads conversation history, builds a context pack (system prompt, tool definitions, MCP servers, policy profile), and publishes a `ConversationRunStartPayload` to NATS. It returns immediately (HTTP 202). `HandleConversationRunComplete()` receives the result via NATS, batch-inserts tool messages, stores the final assistant message, and broadcasts `agui.run_finished` via WebSocket.

**Agent Loop Executor** (`workers/codeforge/agent_loop.py`) implements the core loop. It merges built-in tools (Read, Write, Edit, Bash, Search, Glob, ListDir) with MCP-discovered tools into a single tools array. Each iteration calls `chat_completion_stream()`, streams text chunks to the frontend via AG-UI events, and checks for tool_calls. For each tool call, it requests permission from Go via the Runtime API, executes the tool if allowed, and appends the result to the message history. The loop terminates on `finish_reason="stop"`, max steps, max cost, or cancellation.

**Conversation History Manager** (`workers/codeforge/history.py`) assembles the message array within a token budget. It uses a head-and-tail strategy: always include the system prompt and the last N messages, compress older tool results to stay within `MaxContextTokens`. Long tool outputs are truncated to a configurable maximum (default 10,000 chars) with head+tail preservation.

**HITL Approval** (`internal/service/runtime.go`) intercepts `DecisionAsk` from the policy layer. When a tool call requires user approval, the runtime broadcasts an `agui.permission_request` event via WebSocket and blocks on a buffered channel with a configurable timeout (default 60s). The frontend shows an inline approval card with Allow/Deny buttons. The user's decision is sent via `POST /runs/{id}/approve/{callId}`, which resolves the channel and resumes execution.

**Configuration** (`internal/config/config.go`) includes an `Agent` section: `BuiltinTools` (tool allowlist), `DefaultModel`, `MaxContextTokens` (default 120000), `MaxLoopIterations` (default 50), `AgenticByDefault` (bool), `ToolOutputMaxChars` (default 10000). All fields have env overrides with `CODEFORGE_AGENT_*` prefix. The `Runtime` section adds `ApprovalTimeoutSeconds` (default 60) with `CODEFORGE_APPROVAL_TIMEOUT_SECONDS`.

#### Observability

**Event Sourcing** (`internal/domain/event/`) provides an append-only event stream for agent trajectory recording. Events are stored in a PostgreSQL table `agent_events` (indexed by task_id, agent_id, run_id, timestamp). There are 22+ event types covering tool call requested/approved/denied/result, run started/completed, stall detected, quality gate pass/fail, and delivery status. API endpoints include `GET /api/v1/tasks/{id}/events`, `GET /api/v1/runs/{id}/events`, `GET /api/v1/runs/{id}/trajectory` (cursor-paginated, type/time filtering), and `GET /api/v1/runs/{id}/trajectory/export` (JSON download). Trajectory Stats use SQL aggregates for total events, duration, tool calls, and errors. The frontend provides a TrajectoryPanel with timeline visualization, event filters, stats summary, and export. This enables replay, audit trail, and trajectory inspection (deferred: full replay UI).

**Structured Logging** uses async JSON logging across all services (ADR: [004-async-logging](architecture/adr/004-async-logging.md)). Go uses `slog.JSONHandler` wrapped in `AsyncHandler` (10K buffer, 4 workers, non-blocking drops). Python uses `structlog.JSONRenderer` via `QueueHandler` (10K buffer, background thread). The common schema is `{time, level, service, msg, request_id}`. Docker-native log management is described in ADR: [005-docker-native-logging](architecture/adr/005-docker-native-logging.md).

**Configuration** follows a hierarchical config system (ADR: [003-config-hierarchy](architecture/adr/003-config-hierarchy.md)). Three tiers apply: defaults < YAML (`codeforge.yaml`) < environment variables. A typed `Config` struct validates on startup. Go Core uses `CODEFORGE_*` prefix and Python uses `CODEFORGE_WORKER_*`.

### LLM Capability Levels

Not every LLM brings the same capabilities. CodeForge must fill the gaps so that even simple models can be used productively.

#### The Problem

```text
Claude Code / Aider       ->  own tool usage, codebase search, agent loop
OpenAI API (direct)       ->  function calling, but no codebase context
Ollama (local)            ->  pure text completion, no tools, no context
```

A local Ollama model knows nothing about the repo, cannot read files, and has no memory. CodeForge must provide these capabilities.

#### Capability Stacking by Python Workers

The workers supplement missing capabilities depending on the LLM level.

```text
┌──────────────────────────────────────────────────────┐
│                    CodeForge Worker                    │
│                                                      │
│  ┌────────────────────────────────────────────────┐  │
│  │  Context Layer (for all LLMs)                  │  │
│  │  GraphRAG: Vector search + Graph DB +          │  │
│  │  Web fallback -> find relevant code/docs        │  │
│  └────────────────────────────────────────────────┘  │
│                                                      │
│  ┌────────────────────────────────────────────────┐  │
│  │  Quality Layer (optional, configurable)        │  │
│  │  Multi-Agent Debate: Pro/Con/Moderator ->       │  │
│  │  Reduce hallucinations, verify solutions       │  │
│  └────────────────────────────────────────────────┘  │
│                                                      │
│  ┌────────────────────────────────────────────────┐  │
│  │  Routing Layer                                 │  │
│  │  Task-based model routing via LiteLLM ->        │  │
│  │  Right task to the right model                 │  │
│  └────────────────────────────────────────────────┘  │
│                                                      │
│  ┌────────────────────────────────────────────────┐  │
│  │  Execution Layer                               │  │
│  │  Agent backends: Aider, OpenHands, SWE-agent,  │  │
│  │  Goose, OpenCode, Plandex, or direct LLM API   │  │
│  └────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────┘
```

#### Three LLM Integration Levels

| Level | Example | What CodeForge Provides |
|---|---|---|
| Full-featured Agents | Claude Code, Aider, OpenHands | Orchestration only -- agent brings its own tools |
| API with Tool Support | OpenAI, Claude API, Gemini | Context Layer (GraphRAG) + Routing + Tool Definitions |
| Pure Completion | Ollama, LM Studio (local models) | Everything: Context, Tools, Prompt Engineering, Quality Layer |

The less an LLM can do, the more the CodeForge Worker takes over.

#### Worker Modules in Detail

**Context Layer -- GraphRAG (4-Tier Retrieval)**

Tier 1 (RepoMap) builds a file-level dependency graph via PageRank (tree-sitter). Tier 2 (Hybrid Retrieval) runs BM25 keyword search + semantic embeddings with RRF fusion. Tier 3 (Sub-Agent) uses LLM-guided multi-query expansion + reranking. Tier 4 (GraphRAG) uses PostgreSQL adjacency lists for import/call graph traversal (BFS with hop-decay scoring). All tiers use PostgreSQL (no Neo4j/Qdrant -- single-DB architecture). The result is relevant context prepended to the LLM prompt.

**Quality Layer -- Multi-Stage Quality Assurance**

Four strategies are available, graduated by effort and criticality.

Action Sampling (lightweight) generates multiple independent LLM responses. AskColleagues produces N proposals where an LLM synthesizes the best solution. BinaryComparison runs pairwise comparison and selects the winner. This strategy suits everyday tasks with moderate quality requirements.

RetryAgent + Reviewer (medium) has the agent solve a task multiple times with environment reset between attempts. An LLM-based reviewer evaluates each solution in score mode (numerical evaluation, average across samples) or chooser mode (direct comparison of all solutions). The best solution is selected. This strategy suits important changes with measurable quality.

LLM Guardrail Agent (medium) uses a separate LLM to evaluate the output of the working agent. It validates format compliance, safety, and correctness. It can reject and trigger retry before delivery. This strategy suits automated pipelines where human review is unavailable.

Multi-Agent Debate (heavyweight) has a pro agent argue for a solution, a con agent search for weaknesses, and a moderator synthesize the result. This strategy suits critical architecture decisions and security-relevant changes.

All four strategies are optional and configurable per project/task.

**Routing Layer -- Intelligent Model Routing**

The routing layer handles task classification (architecture, code generation, review, docs, tests), cost optimization (simple tasks to cheap models, complex to powerful ones), latency routing (fast responses for interactive usage), and fallback chains (if a provider fails, automatically use the next one). Routing rules are configurable per project and per user.

Cost management includes budget limits per task/project/user, automatic cost tracking via LiteLLM, warning/stop when budget is exceeded, and API call limits per agent run.

### Agent Execution: Modes, Safety, Workflow

#### Three Execution Modes

Not every use case needs a sandbox. CodeForge supports three modes.

```text
┌─────────────────────────────────────────────────────────────────┐
│                      Execution Modes                             │
│                                                                 │
│  ┌──────────────┐  ┌──────────────────┐  ┌──────────────────┐  │
│  │   Sandbox    │  │     Mount        │  │     Hybrid       │  │
│  │              │  │                  │  │                  │  │
│  │  Isolated    │  │  Agent works     │  │  Sandbox with    │  │
│  │  container,  │  │  directly on     │  │  mounted         │  │
│  │  repo copy   │  │  mounted path    │  │  volumes         │  │
│  │  in container│  │  of the host     │  │  (read/write     │  │
│  │              │  │                  │  │   configurable)  │  │
│  └──────────────┘  └──────────────────┘  └──────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

| Mode | When | Security | Speed |
|---|---|---|---|
| Sandbox | Untrusted agents, foreign models, batch jobs | High -- no access to host | Medium -- container overhead, repo copy |
| Mount | Trusted agents (Claude Code, Aider), local development | Low -- direct file access | High -- no overhead |
| Hybrid | Review workflows, CI-like execution | Medium -- controlled access | Medium |

Mount Mode gives the agent a path to the mounted repo (e.g., `/workspace/my-project`). Changes land directly in the host's filesystem. This mode is ideal for interactive use because the user sees changes immediately in their IDE. No container is needed since the agent runs in the worker process or native tool.

Sandbox Mode runs a Docker container per task (Docker-in-Docker). The repo is copied into the container or mounted as a read-only volume. The agent gets all necessary tools provisioned in the container. The result is extracted as a patch/diff and applied to the original repo.

**Hybrid Mode** runs a container with a mounted volume. Mount permissions are configurable: read-only source + write workspace copy. The agent can read, but changes go into a copy. The user reviews and merges manually.

#### Tool Provisioning for Sandbox Agents

Agents in sandbox containers need the right tools. CodeForge provides these automatically depending on the agent type and execution mode.

```text
┌─────────────────────────────────────────────────┐
│            Sandbox Container                     │
│                                                 │
│  ┌───────────────────────────────────────────┐  │
│  │  Base Image (Python/Node/Go)              │  │
│  └───────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────┐  │
│  │  CodeForge Tool Layer                     │  │
│  │  - Shell (with Safety Evaluator)          │  │
│  │  - File Read/Write/Patch                  │  │
│  │  - Grep/Search                            │  │
│  │  - Git Operations                         │  │
│  │  - Dependency Installation                │  │
│  │  - Test Runner                            │  │
│  └───────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────┐  │
│  │  Repo (copied or mounted)                 │  │
│  └───────────────────────────────────────────┘  │
└─────────────────────────────────────────────┘
```

Tools are defined as Pydantic schemas and passed to the LLM as function calls or tool definitions. Full-featured agents (Aider, OpenHands) bring their own tools and only need the repo.

#### Command Safety Evaluator

Every shell command from an agent goes through a safety check. The evaluator detects destructive operations (`rm -rf`, `git push --force`, etc.), prompt injection in commands, and assesses risk level (low / medium / high). Tool blocklists cover interactive programs (`vim`, `nano`), standalone interpreters (`python` without script), and dangerous commands, all configurable per project as YAML.

When uncertain, the evaluator blocks the command and asks the user (human-in-the-loop). This is optional for trusted agents in mount mode but mandatory for local models in the sandbox.

#### Agent Workflow: Plan -> Execute -> Review

A standardized workflow applies to all agents with configurable autonomy level.

```text
1. PLAN      Agent analyzes task + codebase, creates structured plan
                ↓
2. APPROVE   Plan is submitted for approval (depending on autonomy level)
                ↓  (User, safety rules, or auto-approve)
3. EXECUTE   Agent works through plan point by point
                ↓
4. REVIEW    Self-review, second agent, or guardrail agent
                ↓
5. DELIVER   Result as diff/patch, PR, or direct file change
```

Each step is individually configurable (skip, auto-approve, etc.). The autonomy level determines who may approve (user vs. safety rules). At level 4-5, safety rules replace the human approver.

#### Autonomy Spectrum

CodeForge supports five autonomy levels, from fully supervised operation to completely autonomous execution without user interaction.

```text
Level 1   Level 2     Level 3     Level 4      Level 5
supervised  semi-auto   auto-edit   full-auto    headless
  │           │           │           │            │
  ▼           ▼           ▼           ▼            ▼
 User       User        User       Safety       Safety
 approves   approves    approves   Rules        Rules
 EVERYTHING destructive Terminal/  replace      replace
            actions     Deploy     User         User
                                                + no UI
```

| Level | Name | Who Approves | Use Case |
|---|---|---|---|
| 1 | `supervised` | User at every step | Learning, critical codebases, onboarding |
| 2 | `semi-auto` | User for destructive actions (delete, terminal, deploy) | Everyday development with safety net |
| 3 | `auto-edit` | User only for terminal/deploy, file changes auto-approved | Experienced users, trusted agents |
| 4 | `full-auto` | Safety rules (budget, blocklists, tests) | Batch jobs, trusted agents, delegated tasks |
| 5 | `headless` | Safety rules, no UI needed | CI/CD, cron jobs, API-driven pipelines |

#### Configuration (YAML)

```yaml
# Project level: codeforge-project.yaml
autonomy:
  default_level: semi-auto       # Default for new tasks

  # Safety rules -- replace the user as guardrail at level 4-5
  safety:
    budget_hard_limit: 50.00     # USD -- agent stops when exceeded
    max_steps: 100               # Max actions per task
    max_file_changes: 50         # Max changed files per task
    blocked_paths:               # Files that may never be changed
      - ".env"
      - "secrets/"
      - "**/credentials.*"
      - "production.yml"
    blocked_commands:             # Shell commands that may never be executed
      - "rm -rf /"
      - "DROP TABLE"
      - "git push --force"
      - "chmod 777"
    require_tests_pass: true     # Agent must have green tests before deliver
    require_lint_pass: true      # Linting must pass before deliver
    rollback_on_failure: true    # Auto-rollback on test/lint failure
    branch_isolation: true       # Autonomous agents never work on main/master
    max_cost_per_step: 2.00      # USD -- single LLM call may cost max X
    stall_detection: true        # Detect and abort/re-plan on agent loops
```

#### Security for Fully Autonomous Execution

At level 4 (`full-auto`) and level 5 (`headless`), the following mechanisms replace the human approver.

```text
┌─────────────────────────────────────────────────────────────┐
│                  Safety Layer (replaces user)                  │
│                                                             │
│  ┌─────────────────┐  ┌─────────────────┐                  │
│  │  Budget Limiter  │  │ Command Safety  │                  │
│  │  Hard stop when  │  │ Evaluator       │                  │
│  │  exceeded        │  │ Blocklist +     │                  │
│  │                  │  │ Regex matching  │                  │
│  └─────────────────┘  └─────────────────┘                  │
│  ┌─────────────────┐  ┌─────────────────┐                  │
│  │  Branch Isolation│  │ Test/Lint Gate  │                  │
│  │  Never on main,  │  │ Deliver only    │                  │
│  │  always feature  │  │ when tests +    │                  │
│  │  branch          │  │ lint pass       │                  │
│  └─────────────────┘  └─────────────────┘                  │
│  ┌─────────────────┐  ┌─────────────────┐                  │
│  │  Max Steps       │  │ Rollback        │                  │
│  │  Infinite loop   │  │ Automatic on    │                  │
│  │  detection       │  │ failure         │                  │
│  └─────────────────┘  └─────────────────┘                  │
│  ┌─────────────────┐  ┌─────────────────┐                  │
│  │  Path Blocklist  │  │ Stall Detection │                  │
│  │  Sensitive files  │  │ Re-planning     │                  │
│  │  protected       │  │ or abort        │                  │
│  └─────────────────┘  └─────────────────┘                  │
└─────────────────────────────────────────────────────────────┘
```

#### Headless Mode (Level 5) -- Use Cases

```yaml
# Nightly code review (cron job)
# codeforge-schedules.yaml
schedules:
  - name: nightly-review
    cron: "0 2 * * *"                  # Every night at 2:00
    mode: reviewer
    autonomy: headless
    targets:
      - repo: "myorg/backend"
        branch: "develop"
    deliver: github-pr-comment         # Result as PR comment

  # Weekly dependency update
  - name: weekly-deps
    cron: "0 8 * * 1"                  # Mondays at 8:00
    mode: dependency-updater
    autonomy: headless
    targets:
      - repo: "myorg/backend"
      - repo: "myorg/frontend"
    deliver: pull-request               # Result as new PR
    safety:
      require_tests_pass: true
      max_file_changes: 5

  # Webhook-triggered: Lint fix on new PR
  - name: auto-lint-fix
    trigger: github-webhook             # On new PR
    event: pull_request.opened
    mode: lint-fixer
    autonomy: full-auto
    deliver: push-to-branch             # Push directly to the PR branch
    safety:
      max_file_changes: 20
      require_lint_pass: true
```

#### API-Driven Autonomous Execution

For CI/CD and external systems.

```json
{
  "repo": "myorg/backend",
  "task": "Fix all lint errors in src/",
  "mode": "lint-fixer",
  "autonomy": "full-auto",
  "deliver": "pull-request",
  "safety": {
    "budget_hard_limit": 10.00,
    "require_lint_pass": true
  },
  "callback_url": "https://ci.example.com/webhook"
}
```

The endpoint is `POST /api/v1/tasks`. No UI interaction is needed. The result is retrieved via callback or polling. This approach is ideal for GitHub Actions, GitLab CI, Jenkins, etc.

#### Jinja2 Prompt Templates

All prompts for LLM calls are stored as Jinja2 templates in separate files, not in Python code.

```text
workers/codeforge/templates/
  planner.jinja2          # Planning prompt
  coder.jinja2            # Code generation prompt
  reviewer.jinja2         # Review prompt
  researcher.jinja2       # Research prompt
  safety_evaluator.jinja2 # Safety check prompt
```

Prompts are adjustable without code changes. Contributors can improve prompts without knowing Python. Different prompt sets for different LLMs are possible. Templates are versionable and comparable (Git diff on prompt changes).

#### Keyword Extraction (KeyBERT)

For the Context Layer, semantic keyword extraction from tasks and code uses SentenceTransformers/BERT. It extracts relevant keywords from user requests and codebase. Maximal Marginal Relevance (MMR) provides diverse, non-redundant keywords. Keywords improve retrieval quality in the GraphRAG layer. The process is lightweight and runs locally without an external API.

#### Real-time State via WebSocket

Every state mutation of an agent is immediately emitted to the frontend via WebSocket. This includes agent status (active, waiting, finished), internal monologue (what the agent is "thinking"), current step in the workflow, token usage and costs in real time, and terminal/browser session data. The frontend can display live updates without polling.

#### Agent Specialization: Modes System

Inspired by Roo Code's Modes and Cline's `.clinerules`, CodeForge defines specialized agent modes as YAML configurations instead of using a general-purpose agent. Each mode has its own tools, LLM settings, and autonomy level.

```text
┌──────────────────────────────────────────────────────────┐
│                    Mode Registry                          │
│                                                          │
│  Built-in Modes        Custom Modes (user-defined)       │
│  ┌──────────────┐      ┌──────────────────────────┐     │
│  │ architect    │      │ my-react-reviewer        │     │
│  │ coder        │      │ security-auditor         │     │
│  │ reviewer     │      │ docs-writer              │     │
│  │ researcher   │      │ dependency-updater       │     │
│  │ tester       │      │ ...                      │     │
│  │ lint-fixer   │      │                          │     │
│  │ planner      │      │ (YAML in project or      │     │
│  │ debugger     │      │  global config)          │     │
│  └──────────────┘      └──────────────────────────┘     │
└──────────────────────────────────────────────────────────┘
```

#### Built-in Mode Definitions

```yaml
# modes/architect.yaml
name: architect
description: "Analyzes codebase structure, plans changes, creates design documents"
llm_scenario: think            # LiteLLM Tag -> strong reasoning model
autonomy: supervised           # Architecture decisions always with user
tools:
  - read_file
  - search_file
  - search_dir
  - list_files
  - plan                       # Create structured plan
  - web_search                 # Research documentation
# No write_file, no terminal -- Architect may only read and plan
prompt_template: architect.jinja2
max_steps: 30
```

```yaml
# modes/coder.yaml
name: coder
description: "Implements features, fixes bugs, writes code"
llm_scenario: default          # LiteLLM Tag -> standard coding model
autonomy: auto-edit            # File changes auto, terminal needs approval
tools:
  - read_file
  - write_file
  - search_file
  - search_dir
  - list_files
  - terminal                   # Shell commands (with Safety Evaluator)
  - git_diff
  - git_commit
  - lint
  - test
prompt_template: coder.jinja2
max_steps: 50
```

```yaml
# modes/reviewer.yaml
name: reviewer
description: "Reviews code changes for quality, bugs, security"
llm_scenario: review           # LiteLLM Tag -> review-optimized model
autonomy: headless             # Can run completely autonomously (readonly)
tools:
  - read_file
  - search_file
  - search_dir
  - list_files
  - git_diff
  - lint
  - test
# No write_file -- Reviewer may not edit, only evaluate
prompt_template: reviewer.jinja2
max_steps: 30
deliver: comment               # Result as comment (PR, issue, web GUI)
```

```yaml
# modes/debugger.yaml
name: debugger
description: "Analyzes errors, reproduces bugs, finds root causes"
llm_scenario: think            # Complex reasoning for debugging
autonomy: semi-auto            # Terminal execution with approval
tools:
  - read_file
  - search_file
  - search_dir
  - list_files
  - terminal                   # For reproduction and tests
  - git_log
  - git_diff
  - test
  - lint
prompt_template: debugger.jinja2
max_steps: 40
```

```yaml
# modes/nightly-reviewer.yaml
name: nightly-reviewer
description: "Automatic nightly code review"
llm_scenario: review
autonomy: headless             # Completely autonomous, no UI
tools:
  - read_file
  - search_file
  - search_dir
  - list_files
  - git_diff
  - lint
  - test
prompt_template: reviewer.jinja2
schedule: "0 2 * * *"         # Every night at 2:00
deliver: github-pr-comment
safety:
  budget_hard_limit: 5.00
  max_steps: 30
```

#### Custom Modes (User-Defined)

Users can create their own modes as YAML files.

```yaml
# .codeforge/modes/security-auditor.yaml
name: security-auditor
description: "Reviews code for OWASP Top 10, injection, XSS, etc."
llm_scenario: think
autonomy: headless
tools:
  - read_file
  - search_file
  - search_dir
  - list_files
  - lint
  - terminal                   # For security scanners (npm audit, bandit, etc.)
prompt_template: security-auditor.jinja2
safety:
  blocked_commands:
    - "curl"                   # No network access
    - "wget"
  max_steps: 50
deliver: security-report       # Structured security report
```

#### Mode Selection and Composition

Modes can be used individually or as a pipeline.

```yaml
# Single mode
task:
  mode: coder
  prompt: "Implement feature X"

# Pipeline: Architect plans, Coder implements, Reviewer reviews
task:
  pipeline:
    - mode: architect
      prompt: "Analyze the codebase and create a plan for feature X"
    - mode: coder
      prompt: "Implement the plan from the previous step"
    - mode: reviewer
      prompt: "Review the coder's changes"
    - mode: tester
      prompt: "Write tests for the new changes"

# DAG: Parallel execution + dependencies
task:
  dag:
    plan:
      mode: architect
    implement:
      mode: coder
      depends_on: [plan]
    test:
      mode: tester
      depends_on: [implement]
    review:
      mode: reviewer
      depends_on: [implement]     # Parallel to test
    deliver:
      mode: coder
      depends_on: [test, review]  # Only when both are done
```

#### Directory Structure

```text
# Global (shipped with CodeForge)
modes/
  architect.yaml
  coder.yaml
  reviewer.yaml
  researcher.yaml
  tester.yaml
  lint-fixer.yaml
  planner.yaml
  debugger.yaml
  dependency-updater.yaml

# Project-specific (user-defined)
.codeforge/
  modes/
    security-auditor.yaml
    my-react-reviewer.yaml
  project.yaml                 # Project settings (autonomy, safety, etc.)
  schedules.yaml               # Cron jobs for autonomous tasks
```

#### YAML-Based Tool Definitions

Tools for agents are defined declaratively in YAML, not hardcoded in code. Contributors can add new tools without writing Python code.

```yaml
# tools/bundles/file_ops/config.yaml
tools:
  read_file:
    docstring: "Read contents of a file"
    arguments:
      - name: path
        type: string
        required: true
        description: "Absolute path to the file"
  write_file:
    docstring: "Write contents to a file"
    arguments:
      - name: path
        type: string
        required: true
      - name: content
        type: string
        required: true
```

Tool bundles are directories with `config.yaml` + optional install script. Automatic conversion to OpenAI function calling format works with any LLM that supports function calling. For LLMs without function calling, backtick/JSON-based parsing serves as a fallback.

#### History Processors (Context Window Management)

Long agent sessions exceed the context window. History Processors optimize context as a configurable pipeline.

| Processor | Function |
|---|---|
| LastNObservations | Replace old tool outputs with summaries |
| ClosedWindowProcessor | Remove outdated file views, keep only the most recent |
| CacheControlProcessor | Set cache markers for prompt caching (Anthropic, etc.) |
| RemoveRegex | Remove specific patterns from the history |

Processors are applied as a pipeline one after another. They are configurable per agent type and LLM (small local models need more aggressive trimming).

#### Hook System (Observer Pattern)

Extension points at agent and environment lifecycle, without core modification.

```text
Agent Hooks:
  on_run_start       -> Start monitoring, logging
  on_step_done       -> Record step, update metrics
  on_model_query     -> Track costs, rate limiting
  on_run_end         -> Summary, cleanup

Environment Hooks:
  on_init            -> Prepare container
  on_copy_repo       -> Start repo indexing
  on_startup         -> Install tools
  on_close           -> Clean up container
```

Hooks enable monitoring, custom logging, metrics collection, and integration with external systems, all without modifying the core logic.

#### Trajectory Recording and Replay

Every agent run is recorded as a trajectory. Each step includes Thought, Action, Observation, Timestamp, and Cost. Trajectories are stored as JSON for analysis and reproducibility. Replay mode deterministically repeats a trajectory for debugging. The Inspector provides a web-based viewer integrated in the GUI. Batch statistics track success rates, costs, and steps across many runs.

Trajectories enable debugging of failed agent runs, comparison of different LLMs/configs on the same tasks, and audit trail for code changes by agents.

#### Python Workers Directory Structure

```text
workers/
  codeforge/
    consumer/            # Queue consumer (ingress)
    context/             # Context Layer
      graphrag.py        # Vector + Graph + Web retrieval
      indexer.py         # Codebase indexing
      keywords.py        # KeyBERT keyword extraction
    quality/             # Quality Layer
      debate.py          # Multi-Agent Debate (Pro/Con/Moderator)
      reviewer.py        # Score/Chooser-based solution reviewer
      sampler.py         # Action Sampling (AskColleagues, BinaryComparison)
      guardrail.py       # LLM Guardrail Agent (from CrewAI)
      action_node.py     # Structured Output / Schema validation (from MetaGPT)
    routing/             # Routing Layer
      router.py          # Task-based model routing
      cost.py            # Cost tracking and budgets
    safety/              # Safety Layer
      evaluator.py       # Command Safety Evaluator
      policies.py        # Project-specific security rules
      blocklists.py      # Tool blocklists (configurable)
    execution/           # Execution Layer
      sandbox.py         # Docker container management
      mount.py           # Mount mode logic
      tools.py           # Tool provisioning (Shell, File, Git, etc.)
      workbench.py       # Tool container with shared state (from AutoGen)
    memory/              # Memory Layer
      composite.py       # Composite Scoring (Semantic+Recency+Importance)
      context_window.py  # Context window strategies (Buffered/TokenLimited/HeadAndTail)
      experience.py      # Experience Pool (@exp_cache, from MetaGPT)
    history/             # History Management
      processors.py      # Context window optimization (pipeline)
    hooks/               # Hook System (Observer Pattern)
      agent_hooks.py     # Agent lifecycle hooks
      env_hooks.py       # Environment lifecycle hooks
    events/              # Event Bus (from CrewAI)
      bus.py             # Event emitter + subscriber
      types.py           # Agent/Task/System event definitions
    orchestration/       # Workflow Orchestration
      graph_flow.py      # DAG orchestration (from AutoGen)
      termination.py     # Composable Termination Conditions
      handoff.py         # HandoffMessage Pattern
      planning.py        # MagenticOne Planning Loop + Stall Detection
      pipeline.py        # Document pipeline PRD->Design->Tasks->Code (from MetaGPT)
    trajectory/          # Trajectory Recording
      recorder.py        # Step-by-step recording
      replay.py          # Deterministic replay
    agents/              # Agent backends (Aider, OpenHands, SWE-agent, Goose, OpenCode, Plandex)
    llm/                 # LLM client via LiteLLM
    models/              # Data models
      components.py      # Component System (JSON-serializable configs)
    tools/               # YAML-based tool bundles
      bundles/           # Tool bundle directories
      recommender.py     # BM25 Tool Recommendation (from MetaGPT)
    templates/           # Jinja2 prompt templates
    hitl/                # Human-in-the-Loop
      providers.py       # Human Feedback Provider Protocol (from CrewAI)
```

### Framework Insights: Adopted Patterns

From the analysis of LangGraph, CrewAI, AutoGen, and MetaGPT, the following patterns were adopted for CodeForge. Detailed comparison: [docs/research/market-analysis.md](research/market-analysis.md).

#### Composite Memory Scoring (from CrewAI)

Simple semantic similarity is not enough for memory recall. CodeForge uses weighted scoring from three factors.

```text
Score = (semantic_weight * cosine_similarity)
      + (recency_weight  * recency_decay)
      + (importance_weight * importance_score)
```

| Factor | Default Weight | Calculation |
|---|---|---|
| Semantic | 0.5 | Cosine similarity of embeddings |
| Recency | 0.3 | Exponential decay (half-life configurable) |
| Importance | 0.2 | LLM-based evaluation at storage time |

Two recall modes exist. **Shallow** recall uses direct vector search with composite scoring. Deep recall has an LLM distill sub-queries, search in parallel, and apply confidence-based routing.

#### Context Window Strategies (from AutoGen)

In addition to the History Processors, different strategies for chat completion context management are supported.

| Strategy | Behavior |
|---|---|
| Unbounded | Keep all messages (only for short sessions) |
| Buffered | Keep last N messages |
| TokenLimited | Trim to token budget |
| HeadAndTail | Keep first N + last M messages (system prompt + current context) |

Strategies are configurable per agent type and LLM. Small local models get more aggressive trimming, large API models keep more context.

#### Experience Pool (from MetaGPT)

Successful agent runs are cached and reused for similar tasks.

```python
@exp_cache(context_builder=build_task_context)
async def solve_task(task: Task) -> Result:
    # If similar task was already solved successfully:
    # -> Return cached result
    # Otherwise: Execute normally and cache result
```

The cache key is based on task description + codebase context. Retrieval is similarity-based (not exact match). A configurable confidence threshold controls when cached results are used. This saves LLM costs and improves consistency.

#### Tool Recommendation via BM25 (from MetaGPT)

Instead of passing all available tools to the LLM (token waste), relevant tools are automatically selected. BM25-based ranking scores tools against the current task context. Top-K tools are offered to the LLM as function calls. This reduces token usage and improves tool selection quality. All tools are used as a fallback when confidence score is low.

#### Workbench -- Tool Container (from AutoGen)

Related tools share state and lifecycle.

```python
class GitWorkbench(Workbench):
    """Git tools with shared repository state."""

    def __init__(self, repo_path: str):
        self.repo = git.Repo(repo_path)

    def get_tools(self) -> list[Tool]:
        return [
            Tool("git_status", self._status),
            Tool("git_diff", self._diff),
            Tool("git_commit", self._commit),
            # Tools share self.repo
        ]
```

Workbenches provide shared state between related tools, lifecycle management (start/stop/restart), dynamic tool discovery (tools can change), and are ideal for MCP integration (McpWorkbench).

#### LLM Guardrail Agent (from CrewAI)

A dedicated agent validates the output of another agent.

```text
Agent A (Coder) -> Output -> Guardrail Agent -> Validates -> Accept / Reject + Feedback
                                                              ↓ (on reject)
                                                         Agent A retries with feedback
```

This is integrated into the Quality Layer as a fourth strategy alongside Action Sampling, RetryAgent+Reviewer, and Multi-Agent Debate.

| Level | Effort | Mechanism |
|---|---|---|
| 1. Action Sampling | Light | N responses, select the best |
| 2. RetryAgent + Reviewer | Medium | Retry + score/chooser evaluation |
| 3. LLM Guardrail Agent | Medium | Dedicated agent checks output |
| 4. Multi-Agent Debate | Heavy | Pro/Con/Moderator |

#### Structured Output / ActionNode (from MetaGPT)

LLM outputs are validated against a schema and automatically corrected if needed.

```python
class CodeReviewOutput(ActionNode):
    issues: list[Issue]       # Found problems
    severity: str             # critical / warning / info
    suggestion: str           # Improvement suggestion
    approved: bool            # Review passed?
```

Schema definition uses Pydantic models. The LLM fills fields via constrained generation. An automatic review/revise cycle triggers on schema violation. Retry with error feedback is sent to the LLM.

#### Event Bus for Observability (from CrewAI)

All relevant events in the system are emitted via an event bus.

```text
Agent Events:          Task Events:           System Events:
  agent_started          task_assigned          budget_warning
  agent_step_done        task_completed         budget_exceeded
  agent_tool_called      task_failed            provider_error
  agent_tool_result      task_retrying          provider_fallback
  agent_thinking         task_guardrail_fail    queue_backpressure
  agent_finished         task_human_input       worker_started
  agent_error            task_delegated         worker_stopped
```

Events are streamed to the frontend via WebSocket. The dashboard can filter, aggregate, and visualize events. Monitoring/alerting based on events (e.g., budget_exceeded triggers notification) is supported. All events are persisted as an audit trail for traceability.

#### GraphFlow / DAG Orchestration (from AutoGen)

For complex multi-agent workflows with conditional paths.

```text
                    ┌─── success ──-> [Test Agent]
[Plan Agent] ──-> [Code Agent] ──┤
                    └─── failure ──-> [Debug Agent] ──-> [Code Agent]
                                                          (Cycle)
```

This supports conditional edges based on agent output, parallel nodes (activation="any" for race, activation="all" for join), cycle support with exit conditions (max_iterations, success_condition), a DiGraphBuilder API for fluent graph construction, and visualization in the frontend as an interactive DAG editor.

#### Termination Conditions (from AutoGen)

Flexible, composable stop conditions for agent workflows.

```python
# Composable with & (AND) and | (OR)
stop = (MaxSteps(50)
        | BudgetExceeded(max_cost=5.0)
        | TextMention("TASK_COMPLETE")
        | Timeout(minutes=30))
        & NotCondition(StallDetected())
```

Available conditions include MaxSteps, MaxMessages, MaxTokens, BudgetExceeded (cost limit), TextMention (specific text in output), Timeout (wall-clock-based), StallDetected (no progress), FunctionCallResult (specific tool result), and Custom (arbitrary predicate function).

#### Component System / Declarative Configuration (from AutoGen)

Agents, tools, and workflows are JSON/YAML serializable and reconstructable without code changes.

```json
{
  "provider": "codeforge.agents.CodeReviewAgent",
  "version": 1,
  "config": {
    "llm": "claude-sonnet-4-20250514",
    "tools": ["git_diff", "file_read", "lint"],
    "guardrail": "code_quality",
    "max_iterations": 10,
    "budget_limit": 2.0
  }
}
```

This is essential for the GUI workflow editor. Agents/workflows can be saved, shared, and versioned. Schema versioning with migration support enables import/export of agent configurations.

#### Document Pipeline PRD->Design->Tasks->Code (from MetaGPT)

For complex features, structured intermediate artifacts replace direct code generation.

```text
1. Requirement -> Structured PRD (JSON)
     User stories, acceptance criteria, scope
2. PRD -> System Design (JSON + Mermaid)
     Data structures, API specification, class diagram
3. Design -> Task List (JSON)
     Ordered list of files to create with dependencies
4. Tasks -> Code (per file)
     Context: Design + other already created files
5. Code -> Review + Tests
     Automatic validation against design specification
```

Each intermediate document is schema-validated (ActionNode). Structured constraints reduce hallucination. Incremental development takes existing code into account. Intermediate documents are visible and editable in the GUI.

#### MagenticOne Planning Loop (from AutoGen)

For complex, long-lived tasks, adaptive planning with stall detection applies.

```text
1. PLAN    -> Orchestrator creates initial plan
2. EXECUTE -> Agent works through next step
3. CHECK   -> Evaluate progress:
               - Progress? -> Continue with 2
               - Stall?    -> Re-planning (back to 1)
               - Done?     -> Deliver result
               - Failed?   -> Fact gathering, then re-planning
```

Stall detection recognizes when agents are going in circles. Re-planning adjusts the plan based on previous results. Fact gathering collects missing information before a new plan. Progress tracking uses a ledger (progress protocol).

#### HandoffMessage Pattern (from AutoGen)

Agents explicitly hand off tasks to specialists.

```text
[Planner Agent]
    -> HandoffMessage(target="coder", context="Implement feature X per plan")
        -> [Code Agent]
            -> HandoffMessage(target="reviewer", context="Review changes in src/")
                -> [Review Agent]
                    -> HandoffMessage(target="tester", context="Run test suite")
                        -> [Test Agent]
```

Handoff is explicit with context (not blind forwarding). The agent decides itself who to hand off to. This fits CodeForge's agent specialization (Planner, Coder, Reviewer, etc.) and works with different agent backends (Aider->OpenHands->SWE-agent).

#### Human Feedback Provider Protocol (from CrewAI)

Extensible HITL channels via a provider interface.

```python
class HumanFeedbackProvider(Protocol):
    async def request_feedback(
        self, context: dict, options: list[str]
    ) -> FeedbackResult:
        ...
```

**Implementations** include WebGuiProvider (feedback via the SolidJS web GUI, default), SlackProvider (approval requests as Slack messages), EmailProvider (approval via email link), and CliProvider (terminal input for development/debugging).

### Coding Agent Insights: Adopted Patterns

From the deep analysis of Cline, Devika, OpenHands, SWE-agent, and Aider, the following patterns were adopted for CodeForge. Detailed analysis: [docs/research/market-analysis.md](research/market-analysis.md) and [docs/research/aider-deep-analysis.md](research/aider-deep-analysis.md).

#### Shadow Git Checkpoints (from Cline)

An isolated git repository provides safe rollback during agent execution. Before each agent action, a checkpoint is created in a shadow git repo. On failure or user rejection, instant rollback to last good state occurs. This is separate from the project's actual git history (no polluting commits). It complements the Sandbox mode's container isolation and is integrated into the Safety Layer as the Rollback component.

#### Event-Sourcing Architecture (from OpenHands)

All agent activities are recorded as an append-only event stream.

```text
EventStream: Agent actions, observations, thoughts, tool results
     │
     ├── Replay: Reconstruct any point in time
     ├── Audit: Complete traceability of all actions
     ├── Debug: Step through failed runs
     └── Persist: Events stored for trajectory recording
```

The EventStream serves as the central abstraction for agent execution. All components communicate through events (not direct calls). This enables the Trajectory Recording system. The frontend receives events via WebSocket for live visualization.

#### Microagents (from OpenHands)

Small, trigger-driven agents defined in YAML+Markdown.

```yaml
# .codeforge/microagents/fix-imports.yaml
name: fix-imports
trigger: "import error"          # Triggered when pattern appears in output
type: knowledge                  # knowledge | repo | task
prompt: |
  When you see import errors in Python, check:
  1. Is the package in pyproject.toml?
  2. Is the import path correct?
  3. Run: poetry install
```

Three types exist: knowledge (factual), repo (project-specific), and task (action). They are auto-injected into agent context when the trigger matches. Microagents provide a lightweight alternative to full agent modes for simple patterns. Users can define them in `.codeforge/microagents/`.

#### Diff-based File Review (from Cline)

Before applying changes, the user sees a side-by-side diff. The agent proposes changes as a unified diff. The frontend renders before/after with syntax highlighting. The user can accept, reject, or edit individual hunks. This is integrated into the Plan -> Approve -> Execute workflow. At autonomy level 3+, diffs are auto-approved for file edits.

#### Stateless Agent Design (from Devika)

Agent processes are stateless with all state living in the Go Core. The agent receives full context (task, repo info, history) per invocation. No persistent agent processes exist between tasks. State transitions are tracked in the core service via database. This enables horizontal scaling since any worker can pick up any task. Agent State Visualization in the frontend reads from core, not from agents.

#### ACI -- Agent-Computer Interface (from SWE-agent)

Shell commands are optimized for LLM agents (not for humans). `open <file> [line]` replaces complex `vim`/`cat` invocations. `edit <start>:<end> <content>` replaces sed/awk. `search_dir <pattern> [dir]` replaces `grep -r`. `find_file <name> [dir]` replaces `find`. This reduces error rate by providing LLM-friendly abstractions. The commands are implemented as YAML tool bundles (see Tool Definitions section).

#### tree-sitter Repo Map (from Aider)

A semantic code map is generated via tree-sitter parsing. It extracts class/function/method definitions from all files. Results are ranked by relevance to the current task (PageRank on call graph). This provides a codebase overview without sending all file contents. It reduces token usage while maintaining context quality. The repo map is part of the Context Layer (GraphRAG) and complements vector search.

#### Architect/Editor Pattern (from Aider)

Separate LLM roles handle planning and implementation. The architect model (strong reasoning, e.g., Claude Opus) analyzes the codebase and creates a plan. The editor model (fast coding, e.g., Claude Sonnet) implements the plan. This maps directly to CodeForge's Modes System: `architect` -> `coder` pipeline. Cost optimization uses the expensive model only for planning and the cheaper model for execution. The pattern is configurable via mode pipelines in task YAML.

#### Edit Formats (from Aider)

Multiple output formats support different LLM capabilities.

| Format | When | How |
|---|---|---|
| whole-file | Small files, local models | LLM outputs complete file |
| diff | Standard edits, capable models | Unified diff format |
| search/replace | Precise edits | Search block -> Replace block |
| udiff | Complex multi-file edits | Universal diff with context |

Format selection is based on LLM capability level and file size. Automatic retry with simpler format occurs on parse failure. This is integrated into the Execution Layer's tool provisioning.

#### Skills System (from OpenHands)

Reusable Python snippets are automatically injected into agent context. Pre-built skills cover common operations (file manipulation, git, testing). Skills are Python functions available in the agent's execution environment. They are automatically included in the prompt based on task context. Users can extend them with custom skills in `.codeforge/skills/`. Skills complement YAML Tool Bundles (skills are code, bundles are declarations).

#### Risk Management (from OpenHands)

LLM-based security analysis evaluates agent actions. The **InvariantAnalyzer** validates agent actions against security policies. It checks for path traversal, command injection, and credential exposure. It runs as a pre-execution filter in the Safety Layer and complements the Command Safety Evaluator with LLM-based reasoning. It can be disabled for trusted agents to reduce latency.

### Roadmap/Feature-Map: Auto-Detection and Adaptive Integration

#### Core Principle

CodeForge automatically detects which spec-driven development tools, PM platforms, and roadmap artifacts are used in a project, and offers appropriate integration. No proprietary PM tool is built. Instead, bidirectional sync with existing tools is provided.

#### Provider Registry for Specs and PM

The same architecture as `gitprovider` applies. New adapters only require a new package and a blank import.

```text
port/
  specprovider/
    provider.go        # Interface: Detect(), ReadSpecs(), WriteChange(), Watch()
    registry.go        # Register(), New(), Available()
  pmprovider/
    provider.go        # Interface: Detect(), SyncItems(), CreateItem(), Webhooks()
    registry.go        # Register(), New(), Available()

adapter/
  openspec/            # OpenSpec (openspec/ directory)
  speckit/             # GitHub Spec Kit (.specify/ directory)
  autospec/            # Autospec (specs/spec.yaml)
  plane/               # Plane.so (REST API v1)
  openproject/         # OpenProject (REST API v3)
  github_pm/           # GitHub Issues/Projects (REST + GraphQL)
  gitlab_pm/           # GitLab Issues/Boards (REST + GraphQL)
```

#### Three-Tier Auto-Detection

```text
┌─────────────────────────────────────────────────────────────┐
│                    Auto-Detection Engine                      │
│                                                             │
│  Tier 1: Spec-Driven Detectors (repo files)                 │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐      │
│  │ OpenSpec  │ │ Spec Kit │ │ Autospec │ │ ADR/RFC  │      │
│  │openspec/ │ │.specify/ │ │specs/*.y │ │docs/adr/ │      │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘      │
│                                                             │
│  Tier 2: Platform Detectors (API-based)                     │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐      │
│  │ GitHub   │ │ GitLab   │ │ Plane.so │ │OpenProj. │      │
│  │Issues/PR │ │Issues/MR │ │REST API  │ │REST API  │      │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘      │
│                                                             │
│  Tier 3: File-Based Detectors (simple markers)              │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐      │
│  │ROADMAP.md│ │TASKS.md  │ │backlog/  │ │CHANGELOG │      │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘      │
└─────────────────────────────────────────────────────────────┘
```

Each detector implements the `specprovider.SpecProvider` or `pmprovider.PMProvider` interface and registers itself via `init()`. The detection engine iterates over all registered detectors and returns a list of detected tools.

#### Spec Provider Interface

```go
type SpecProvider interface {
    // Detect checks if this spec format is present in the repo
    Detect(repoPath string) (bool, error)

    // ReadSpecs reads all specs from the repo
    ReadSpecs(repoPath string) ([]Spec, error)

    // WriteChange writes a change (delta format)
    WriteChange(repoPath string, change Change) error

    // Watch observes spec changes (for bidirectional sync)
    Watch(repoPath string, callback func(event SpecEvent)) error

    // Capabilities declares supported operations
    Capabilities() []Capability
}
```

#### PM Provider Interface

```go
type PMProvider interface {
    // Detect checks if this PM platform is configured for the project
    Detect(projectConfig ProjectConfig) (bool, error)

    // SyncItems synchronizes items bidirectionally
    SyncItems(ctx context.Context, direction SyncDirection) (SyncResult, error)

    // CreateItem creates a new item on the platform
    CreateItem(ctx context.Context, item Item) (string, error)

    // RegisterWebhook registers a webhook for real-time sync
    RegisterWebhook(ctx context.Context, callbackURL string) error

    // Capabilities declares supported operations
    Capabilities() []Capability
}
```

#### Bidirectional Sync

```text
┌─────────────────┐              ┌─────────────────┐
│  CodeForge       │  <-- Sync -->  │  External PM     │
│  Roadmap Model   │              │  (Plane/GitHub/  │
│                  │              │   OpenProject)   │
│  Milestone       │  <-------->   │  Initiative/     │
│  Feature         │  <-------->   │  Epic/Issue      │
│  Task            │  <-------->   │  Work Item       │
└────────┬────────┘              └─────────────────┘
         │
         │  <-- Sync -->
         │
┌────────▼────────┐
│  Repo Specs      │
│  (OpenSpec/       │
│   Spec Kit/       │
│   Autospec)       │
└─────────────────┘
```

Import brings PM tool data into the CodeForge roadmap model (issues/epics become features/tasks). Export sends CodeForge data to the PM tool (new features are created as issues). **Bidirectional** sync means changes are synchronized in both directions. Conflict resolution is timestamp-based + user decision on conflicts. Sync triggers include webhook (real-time), poll (periodic), and manual.

#### Roadmap Data Model

```go
// Internal roadmap model -- PM adapters map to this format
type Milestone struct {
    ID          string
    Title       string
    Description string
    DueDate     time.Time
    Features    []Feature
    Status      MilestoneStatus  // planned, active, completed
    LockVersion int              // Optimistic Locking (from OpenProject)
}

type Feature struct {
    ID          string
    Title       string
    Description string
    Priority    Priority
    Tasks       []Task
    Labels      []string         // Label-triggered sync (from Plane)
    SpecRef     string           // Reference to spec file (openspec/specs/feature.md)
    ExternalIDs map[string]string // {"plane": "abc", "github": "123"}
}
```

#### `/ai` Endpoint for LLM Consumption (from Ploi Roadmap)

```text
GET /api/v1/projects/{id}/roadmap/ai?format=json
GET /api/v1/projects/{id}/roadmap/ai?format=yaml
GET /api/v1/projects/{id}/roadmap/ai?format=markdown
```

This endpoint provides the roadmap in an LLM-optimized format. It returns a compact summary of all milestones, features, and tasks with status information and dependencies. AI agents use this to understand project context.

#### Directory Structure (Extension)

```text
internal/
  port/
    specprovider/          # Spec detection interface
      provider.go          # SpecProvider Interface + Capabilities
      registry.go          # Register(), New(), Available()
    pmprovider/            # PM platform interface
      provider.go          # PMProvider Interface + Capabilities
      registry.go          # Register(), New(), Available()
  adapter/
    openspec/              # OpenSpec adapter (openspec/ directory)
    speckit/               # GitHub Spec Kit adapter (.specify/)
    autospec/              # Autospec adapter (specs/spec.yaml)
    plane/                 # Plane.so REST API v1 adapter
    openproject/           # OpenProject REST API v3 adapter
    github_pm/             # GitHub Issues/Projects adapter
    gitlab_pm/             # GitLab Issues/Boards adapter
  domain/
    roadmap/               # Roadmap domain (Milestone, Feature, Task)
  service/
    detection.go           # Auto-Detection Engine
    sync.go                # Bidirectional Sync Service
```

### LLM Integration: LiteLLM Proxy as Sidecar

#### Architecture Decision

After analysis of LiteLLM, OpenRouter, Claude Code Router, and OpenCode CLI, the decision was made that CodeForge does not build its own LLM provider interface. LiteLLM Proxy runs as a Docker sidecar and provides a unified OpenAI-compatible API. Detailed analysis: [docs/research/market-analysis.md](research/market-analysis.md).

#### Integration Architecture

```text
┌─────────────────────────────────────────────────────┐
│                  TypeScript Frontend                 │
│                                                     │
│  ┌──────────────────────────────────────────────┐   │
│  │  Cost Dashboard  │  Provider Config UI       │   │
│  └──────────────────────────────────────────────┘   │
└────────────────────┬────────────────────────────────┘
                     │ REST / WebSocket
┌────────────────────▼────────────────────────────────┐
│                  Go Core Service                     │
│                                                     │
│  ┌──────────────┐  ┌──────────────┐                 │
│  │ LiteLLM      │  │ Scenario     │                 │
│  │ Config Mgr   │  │ Router       │                 │
│  └──────────────┘  └──────────────┘                 │
│  ┌──────────────┐  ┌──────────────┐                 │
│  │ User-Key     │  │ Local Model  │                 │
│  │ Mapping      │  │ Discovery    │                 │
│  └──────────────┘  └──────────────┘                 │
│  ┌──────────────┐                                   │
│  │ Copilot      │                                   │
│  │ Token Exch.  │                                   │
│  └──────────────┘                                   │
└────────────┬────────────────────────┬───────────────┘
             │ OpenAI-compatible API  │
             │ (Port 4000)            │
┌────────────▼────────────────────────┤
│      LiteLLM Proxy (Sidecar)       │
│                                     │
│  ┌──────────────┐  ┌────────────┐  │
│  │  Router      │  │  Budget    │  │
│  │  (6 Strat.)  │  │  Manager   │  │
│  └──────────────┘  └────────────┘  │
│  ┌──────────────┐  ┌────────────┐  │
│  │  Caching     │  │  Callbacks │  │
│  │  (Redis)     │  │(Prometheus)│  │
│  └──────────────┘  └────────────┘  │
└────────────┬────────────────────────┘
             │ Provider APIs
┌────────────▼────────────────────────────────────────┐
│  OpenAI │ Anthropic │ Ollama │ Bedrock │ OpenRouter  │
└─────────────────────────────────────────────────────┘
```

#### What LiteLLM Provides (not built by us)

| Feature | LiteLLM Mechanism |
|---|---|
| Provider abstraction | 127+ providers, unified API |
| Routing | 6 strategies: latency, cost, usage, least-busy, shuffle, tag-based |
| Fallbacks | Fallback chains with cooldown (60s default) |
| Cost tracking | Per call, per model, per key via pricing DB (36,000+ entries) |
| Budgets | Per key, per team, per user, per provider limits |
| Streaming | `CustomStreamWrapper` normalizes all providers to OpenAI SSE |
| Tool calling | Unified via `tools` parameter, provider conversion automatic |
| Structured output | `response_format` cross-provider (native or via tool-call fallback) |
| Caching | In-memory, Redis, semantic (Qdrant), S3, GCS |
| Observability | 42+ integrations (Prometheus, Langfuse, Datadog, etc.) |
| Rate limiting | Per-key TPM/RPM, per-team, per-model |

#### What CodeForge Builds (Custom Development)

| Component | Layer | Description |
|---|---|---|
| LiteLLM Config Manager | Go Core | Generates `litellm_config.yaml` from CodeForge DB. CRUD for models, deployments, keys. |
| User-Key Mapping | Go Core | CodeForge user -> LiteLLM Virtual Keys. API keys stored securely in CodeForge DB, forwarded to LiteLLM. |
| Scenario Router | Go Core | Task type -> LiteLLM tag. `metadata.tags: ["think"]` in request -> LiteLLM routes to matching deployment. |
| Cost Dashboard | Frontend | Query LiteLLM Spend API (`/spend/logs`, `/global/spend/per_team`). Visualization per project/user/agent. |
| Local Model Discovery | Go Core | Query Ollama (`/api/tags`) and LM Studio (`/v1/models`) endpoints. Discovered models automatically added to LiteLLM config. |
| Copilot Token Exchange | Go Core | Read GitHub OAuth token from `~/.config/github-copilot/hosts.json`, exchange for bearer token via `api.github.com/copilot_internal/v2/token`. |

#### Scenario-Based Routing

Inspired by Claude Code Router, different task types are automatically routed to matching models via LiteLLM's tag-based routing.

```yaml
# litellm_config.yaml (generated by Go Core)
model_list:
  - model_name: default
    litellm_params:
      model: anthropic/claude-sonnet-4-20250514
      api_key: os.environ/ANTHROPIC_API_KEY
      tags: ["default", "review"]

  - model_name: background
    litellm_params:
      model: openai/gpt-4o-mini
      api_key: os.environ/OPENAI_API_KEY
      tags: ["background"]

  - model_name: think
    litellm_params:
      model: anthropic/claude-opus-4-20250514
      api_key: os.environ/ANTHROPIC_API_KEY
      tags: ["think", "plan"]

  - model_name: longcontext
    litellm_params:
      model: google/gemini-2.0-pro
      api_key: os.environ/GEMINI_API_KEY
      tags: ["longContext"]

  - model_name: local
    litellm_params:
      model: ollama/llama3
      api_base: http://ollama:11434
      tags: ["background", "default"]

router_settings:
  routing_strategy: "tag-based-routing"
  num_retries: 3
  fallbacks:
    - default: ["local"]
    - think: ["default"]
```

| Scenario | When | Typical Models |
|---|---|---|
| `default` | General coding tasks | Claude Sonnet, GPT-4o |
| `background` | Batch, index, embedding | GPT-4o-mini, DeepSeek, local |
| `think` | Architecture, debugging, complex logic | Claude Opus, o3 |
| `longContext` | Input > 60K tokens | Gemini Pro (1M context) |
| `review` | Code review, quality check | Claude Sonnet |
| `plan` | Feature planning, design documents | Claude Opus |

#### LiteLLM Proxy Configuration

```yaml
# docker-compose.yml (excerpt)
services:
  litellm:
    image: docker.litellm.ai/berriai/litellm:main-stable
    ports:
      - "4000:4000"
    volumes:
      - ./litellm_config.yaml:/app/config.yaml
    command: ["--config", "/app/config.yaml", "--port", "4000"]
    environment:
      - LITELLM_MASTER_KEY=${LITELLM_MASTER_KEY}
      - DATABASE_URL=postgresql://codeforge:${POSTGRES_PASSWORD}@postgres:5432/codeforge?schema=litellm
    depends_on:
      postgres:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:4000/health/liveliness"]
```

#### Frontend Directory Structure (SolidJS)

```text
frontend/
  src/
    features/            # Feature modules (dashboard, roadmap, agents, llm)
    shared/              # Shared components, primitives, utils
    api/                 # API client, WebSocket handler
```
