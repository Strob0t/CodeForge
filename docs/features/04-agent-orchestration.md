# Feature: Agent Orchestration (Pillar 4)

> Status: Core implemented (Phases 2-6) -- agent backends, runtime API, policy layer, multi-agent orchestration, 4-tier Code-RAG
> Priority: Phase 2-6 completed; Phase 9+ for additional backends and advanced features
> Architecture reference: [architecture.md](../architecture.md) -- "Agent Execution", "Worker Modules", "Modes System"

### Purpose

Coordination of various AI coding agents through a **unified** orchestration layer. Agents are swappable backends that run in configurable execution modes with safety controls and quality assurance.

### Agent Backends

| Agent | Adapter | Status | Type |
|---|---|---|---|
| Aider | `adapter/aider/` | Registered | Generic NATS-to-LLM dispatcher |
| OpenHands | `adapter/openhands/` | Phase 9+ | Not yet implemented |
| SWE-agent | `adapter/sweagent/` | Phase 9+ | Not yet implemented |
| Goose | `adapter/goose/` | Phase 9+ | Not yet implemented |
| OpenCode | `adapter/opencode/` | Phase 9+ | Not yet implemented |
| Plandex | `adapter/plandex/` | Phase 9+ | Not yet implemented |

All backends implement the `agentbackend.Backend` interface with capability declarations.

> **Current status:** Only the Aider backend is registered. It operates as a generic NATS-to-LLM dispatcher (receives tasks via NATS, calls LiteLLM, returns results) rather than integrating directly with the Aider CLI. Direct CLI integrations for each agent tool are planned for Phase 9+.

### Execution Modes

| Mode | Security | Speed | Use Case |
|---|---|---|---|
| Sandbox | High (isolated container) | Medium | Untrusted agents, batch jobs |
| Mount | Low (direct file access) | High | Trusted agents, local dev |
| Hybrid | Medium (controlled access) | Medium | Review workflows, CI-like |

### Agent Workflow

```text
Plan -> Approve -> Execute -> Review -> Deliver
```

Each step is individually configurable. The **autonomy** level determines who approves.

### Autonomy Spectrum (5 Levels)

| Level | Name | Who Approves | Use Case |
|---|---|---|---|
| 1 | `supervised` | User at every step | Learning, critical codebases |
| 2 | `semi-auto` | User for destructive actions | Everyday development |
| 3 | `auto-edit` | User only for terminal/deploy | Experienced users |
| 4 | `full-auto` | Safety rules | Batch jobs, delegated tasks |
| 5 | `headless` | Safety rules, no UI | CI/CD, cron jobs, API |

### Safety Layer (8 Components)

- Budget Limiter -- hard stop on cost exceeded.
- Command Safety Evaluator -- blocklist + regex matching.
- Branch Isolation -- never on main, always feature branch.
- Test/Lint Gate -- deliver only when tests + lint pass.
- Max Steps -- infinite loop detection.
- Rollback -- automatic on failure (Shadow Git).
- **Path Blocklist** -- sensitive files protected.
- Stall Detection -- re-planning or abort.

### Quality Layer (4 Tiers)

- Action Sampling (light) -- N responses, select best.
- RetryAgent + Reviewer (medium) -- retry + score/chooser evaluation.
- LLM Guardrail Agent (medium) -- dedicated agent checks output.
- **Multi-Agent Debate** (heavy) -- Pro/Con/Moderator.

### Modes System

YAML-configurable agent specializations. Built-in modes include architect, coder, reviewer, debugger, tester, lint-fixer, planner, and researcher. Users can define custom modes in `.codeforge/modes/`. Modes support composition through pipelines and DAG workflows.

### Worker Modules

| Module | Purpose |
|---|---|
| Context (GraphRAG) | Vector search + graph DB + web fallback |
| Quality | Debate, reviewer, sampler, guardrail |
| Routing | Task-based model routing via LiteLLM |
| Safety | Command evaluation, blocklists, policies |
| Execution | Sandbox/mount management, tool provisioning |
| Memory | Composite scoring, context strategies, experience pool |
| History | Context window optimization pipeline |
| Events | Event bus for observability |
| Orchestration | DAG flow, termination conditions, handoff, planning loop |
| Hooks | Agent/environment lifecycle observer |
| Trajectory | Recording, replay, audit trail |
| HITL | Human feedback provider protocol |

### Policy System

The policy layer governs agent permissions, quality gates, and termination conditions.

#### Backend

- Domain: `internal/domain/policy/` -- PolicyProfile, PermissionRule, ToolSpecifier, QualityGate, TerminationCondition.
- Presets (4): plan-readonly, headless-safe-sandbox, headless-permissive-sandbox, trusted-mount-autonomous.
- Service: `internal/service/policy.go` -- first-match-wins rule evaluation, CRUD (SaveProfile, DeleteProfile).
- **Loader**: `internal/domain/policy/loader.go` -- YAML file loading + SaveToFile for custom profiles.
- REST API: GET/POST /policies, GET/DELETE /policies/{name}, POST /policies/{name}/evaluate.

#### Frontend (PolicyPanel)

- Component: `frontend/src/features/project/PolicyPanel.tsx`.
- 3 views: List (presets + custom), Detail (summary + rules table + evaluate tester), Editor (create/clone).
- Evaluate tester lets you test a tool call against a policy and see the decision (allow/deny/ask).
- Types: `PolicyProfile`, `PermissionRule`, `PolicyQualityGate`, `TerminationCondition`, `ResourceLimits`.

#### Deferred

- Scope levels: global (user) to project to run/session (override).
- "Effective Permission Preview" -- show which rule matched and why.
- Run-level policy overrides.

### Retrieval Sub-Agent (Phase 6C)

LLM-guided multi-query retrieval that improves context quality for agents working on complex tasks.

#### Architecture

```text
Go Core (RetrievalService)
  |
  | NATS: retrieval.subagent.request
  v
Python Worker (RetrievalSubAgent)
  |-- 1. LLM query expansion (task prompt -> N focused queries)
  |-- 2. Parallel hybrid searches (existing HybridRetriever.search() x N)
  |-- 3. Deduplication (by filepath+start_line)
  |-- 4. LLM re-ranking (top candidates scored for relevance)
  |-- 5. Return top-K results
  |
  | NATS: retrieval.subagent.result
  v
Go Core (handles result, delivers to waiter or HTTP handler)
```

#### Backend

- Python: `RetrievalSubAgent` in `workers/codeforge/retrieval.py` -- composes `HybridRetriever` + `LiteLLMClient`.
- Go Service: `SubAgentSearchSync()` / `HandleSubAgentSearchResult()` in `internal/service/retrieval.go`.
- **Context Optimizer**: `fetchRetrievalEntries()` tries sub-agent first, falls back to single-shot search.
- REST API: `POST /api/v1/projects/{id}/search/agent`.
- Config: `SubAgentModel`, `SubAgentMaxQueries`, `SubAgentRerank` in `config.Orchestrator`.

#### Frontend (RetrievalPanel)

- Standard/Agent toggle button next to search bar.
- Agent mode shows expanded queries as tags + total candidates count.
- Component: `frontend/src/features/project/RetrievalPanel.tsx`.

#### Deferred

- Configurable expansion prompts per project.
- Streaming results (partial results as queries complete).
- Cost tracking for sub-agent LLM calls.

### Completed (Phase 1-2)

- [x] `agentbackend.Backend` interface definition (`internal/port/agentbackend/`).
- [x] Agent backend registry with self-registration via `init()`.
- [x] Basic queue consumer (Python worker) -- NATS-based async dispatch.
- [x] Aider backend adapter (`internal/adapter/aider/`).
- [x] Simple task to single agent execution.
- [x] Mount mode implementation (direct file access).
- [x] Basic safety evaluator (command blocklist + regex matching).
- [x] Frontend: Agent Monitor (live logs, status via WebSocket).
- [x] Frontend: Task submission form, task list, agent CRUD.

### Completed (Phase 3 -- Reliability and Agent Foundation)

- [x] Configuration management (hierarchical: defaults < YAML < ENV).
- [x] Structured logging (async JSON, Go + Python, request ID propagation).
- [x] Circuit breaker for NATS + LiteLLM calls.
- [x] Graceful 4-phase shutdown, idempotency middleware, dead letter queue.
- [x] Event sourcing for agent trajectory (`agent_events` table, 22+ event types).
- [x] Tiered cache (L1 Ristretto + L2 NATS KV), rate limiting, connection pool tuning.

### Completed (Phase 4 -- Agent Execution Engine)

- [x] Policy layer: 4 presets, YAML custom policies, first-match-wins evaluation, REST API + frontend PolicyPanel.
- [x] Runtime API: step-by-step execution protocol (Go to Python via NATS), per-tool-call policy enforcement.
- [x] Checkpoint system: shadow Git commits for safe rollback.
- [x] Docker Sandbox: container lifecycle management with resource limits.
- [x] Stall detection: FNV-64a hash ring buffer, configurable threshold.
- [x] Quality gate enforcement: test/lint gates via NATS request/result protocol.
- [x] 5 deliver modes: none, patch, commit-local, branch, PR.

### Completed (Phase 5 -- Multi-Agent Orchestration)

- [x] Execution plans: DAG scheduling with 4 protocols (sequential, parallel, ping_pong, consensus).
- [x] Orchestrator agent (meta-agent): LLM-based feature decomposition, agent strategy selection.
- [x] Agent teams: team CRUD, role-based members, protocol selection.
- [x] Context optimizer: token budget management, workspace scanning, context packing.
- [x] Shared context: team-level versioned state with NATS notifications.
- [x] Modes system: 8 built-in presets, ModeService, REST API.

### Completed (Phase 6 -- Code-RAG)

- [x] Tier 1 -- RepoMap: tree-sitter symbol extraction, PageRank file ranking (16+ languages).
- [x] Tier 2 -- Hybrid Retrieval: BM25S keyword + semantic embeddings, RRF fusion.
- [x] Tier 3 -- Retrieval Sub-Agent: LLM multi-query expansion, parallel search, re-ranking.
- [x] Tier 4 -- GraphRAG: PostgreSQL adjacency-list graph, BFS with hop-decay scoring.

### MCP Integration (Phase 15)

Model Context Protocol integration gives agents access to external tools (databases, APIs, cloud services, file systems) and allows external MCP clients (Claude Desktop, VS Code, Cursor) to invoke CodeForge workflows.

#### MCP Server (Go Core)

Exposes CodeForge operations to external MCP clients via the mcp-go SDK with Streamable HTTP transport.

- **Tools**: `list_projects`, `get_project`, `get_run_status`, `get_cost_summary`
- **Resources**: `codeforge://projects`, `codeforge://costs/summary`
- **Auth**: Bearer token / API key middleware
- **Config**: `mcp.enabled`, `mcp.server_port` (default 3001)
- **Code**: `internal/adapter/mcp/` (server.go, tools.go, resources.go, auth.go)

#### MCP Client (Python Workers)

Agents connect to external MCP servers during runs to use their tools.

- **McpWorkbench**: Multi-server container (connect/disconnect, tool discovery, tool call bridging)
- **McpToolRecommender**: BM25-based ranking of relevant tools for task prompts
- **Transport**: stdio and SSE via Python `mcp` SDK
- **Code**: `workers/codeforge/mcp_workbench.py`, `workers/codeforge/mcp_models.py`

#### MCP Server Registry

Persistent storage for MCP server definitions with project-level assignment.

- **Database**: `mcp_servers`, `project_mcp_servers`, `mcp_server_tools` tables (migration 036)
- **HTTP API**: 10 endpoints for CRUD, test connection, tools listing, project assignment
- **Frontend**: MCPServersPage (server list, add/edit modal, test connection, tools discovery)
- **Code**: `internal/adapter/postgres/store_mcp.go`, `internal/adapter/http/handlers_mcp.go`, `frontend/src/features/mcp/MCPServersPage.tsx`

#### Policy Integration

MCP tool calls use namespaced identifiers `mcp:{server}:{tool}` and flow through the existing policy engine with glob matching. Mode-based filtering via `Mode.Tools` and `Mode.DeniedTools` supports the same convention.

### Agentic Conversation Mode (Phase 17)

The agentic conversation mode transforms the Chat UI into an autonomous coding agent. Rather than a single LLM call per message, the system runs a multi-turn tool-use loop where the LLM reads files, edits code, runs commands, and iterates until the task is complete.

#### How It Works

1. **User sends a message** via the Chat UI (or API with `?mode=agentic` / `"agentic": true`)
2. **Go Core** stores the message, builds a context pack (system prompt, conversation history, tool definitions, MCP servers, policy profile), and publishes to NATS
3. **Python Worker** receives the job and starts the agent loop (calls LLM with tool definitions, streams text via AG-UI WebSocket events, executes each `tool_calls` response with per-call policy enforcement, appends tool results and feeds back to the LLM, repeats until the LLM responds without tool calls or termination limits are hit)
4. **Go Core** receives the completion, stores all tool messages and the final reply, and broadcasts `agui.run_finished`

#### Built-in Tools

| Tool | Policy Name | Description |
|------|-------------|-------------|
| Read | `Read` | Read file contents with optional line range (offset/limit) |
| Write | `Write` | Create or overwrite a file, creating parent directories |
| Edit | `Edit` | Search-and-replace: validate old_text is unique, replace with new_text |
| Bash | `Bash` | Execute shell command with timeout (default 120s), captures stdout+stderr |
| Search | `Search` | Regex search across files via `grep -rn` |
| Glob | `Glob` | Find files by glob pattern via `pathlib.Path.glob()` |
| ListDir | `ListDir` | List directory contents, optional recursive |

Tools are registered in the `ToolRegistry` (`workers/codeforge/tools/`). MCP-discovered tools merge in with `mcp__{server}__{tool}` naming and route through `McpWorkbench.call_tool()`.

#### Conversation History Management

The `ConversationHistoryManager` assembles messages within a configurable token budget (`MaxContextTokens`, default 120000):

- **Head-and-tail strategy**: System prompt + first few messages + last N messages always included
- **Tool result truncation**: Long outputs capped at `ToolOutputMaxChars` (default 10000) with head+tail preservation
- **Context injection**: RepoMap, retrieval results, and LSP diagnostics embedded in the system prompt

#### Human-in-the-Loop (HITL) Approval

When the policy layer returns `DecisionAsk` for a tool call:

1. Runtime broadcasts `agui.permission_request` via WebSocket (includes tool name, command, path)
2. Frontend displays an inline approval card with Allow/Deny buttons and a countdown timer
3. User decision sent via `POST /api/v1/runs/{id}/approve/{callId}` with `{"decision": "allow"|"deny"}`
4. If approved, tool executes normally; if denied or timeout (default 60s), a "Permission denied" result is returned to the LLM

#### Configuration

```yaml
agent:
  builtin_tools: [Read, Write, Edit, Bash, Search, Glob, ListDir]
  default_model: ""  # Uses project's configured model
  max_context_tokens: 120000
  max_loop_iterations: 50
  agentic_by_default: false
  tool_output_max_chars: 10000

runtime:
  approval_timeout_seconds: 60
```

Environment overrides: `CODEFORGE_AGENT_DEFAULT_MODEL`, `CODEFORGE_AGENT_MAX_CONTEXT_TOKENS`, `CODEFORGE_AGENT_MAX_LOOP_ITERATIONS`, `CODEFORGE_AGENT_AGENTIC_BY_DEFAULT`, `CODEFORGE_APPROVAL_TIMEOUT_SECONDS`.

#### Frontend

- **ToolCallCard**: Tool-type icons (file, terminal, search), collapsible arguments/results, permission denied badge
- **ChatPanel**: Step counter during agentic turns ("Step 3/50"), running cost display, grouped tool calls, agentic mode indicator
- **Approval UI**: Inline card on `permission_request` events with countdown and Allow/Deny buttons

#### Key Files

| File | Purpose |
|------|---------|
| `workers/codeforge/agent_loop.py` | Core agentic loop executor |
| `workers/codeforge/history.py` | Conversation history manager |
| `workers/codeforge/tools/` | Built-in tool registry (7 tools) |
| `internal/service/conversation.go` | Agentic dispatch and completion handler |
| `internal/service/runtime.go` | HITL approval (waitForApproval, ResolveApproval) |
| `internal/adapter/http/handlers.go` | HTTP handlers (agentic routing, approval endpoint) |
| `frontend/src/features/project/ChatPanel.tsx` | Chat UI with agentic enhancements |
| `frontend/src/features/project/ToolCallCard.tsx` | Tool call display component |

### Benchmark Mode (Phase 20, Dev-Only)

Structured evaluation framework for measuring agent and model quality. Only accessible when `APP_ENV=development`.

#### Architecture

Three-pillar evaluation stack running in the Python worker:

1. **DeepEval** — LLM-as-judge metrics (correctness, faithfulness, relevancy, tool correctness) via `LiteLLMJudge` wrapper
2. **AgentNeo** — Optional tracing for tool selection accuracy, goal decomposition, and plan adaptability
3. **GEMMAS Collaboration** — Information Diversity Score (IDS) and Unnecessary Path Ratio (UPR) for multi-agent workflows

#### Workflow

1. User creates a benchmark run via `/benchmarks` page (selects dataset, model, metrics)
2. Go Core stores run in `benchmark_runs` table and publishes `benchmark.run.request` to NATS
3. Python worker loads YAML dataset, executes tasks against LLM, evaluates with selected metrics
4. Results published back via `benchmark.run.result`, stored in `benchmark_results` table
5. Frontend displays per-task scores, summary, and supports run-to-run comparison

#### API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/v1/benchmarks/runs` | Create benchmark run |
| GET | `/api/v1/benchmarks/runs` | List all runs |
| GET | `/api/v1/benchmarks/runs/{id}` | Get run details |
| DELETE | `/api/v1/benchmarks/runs/{id}` | Delete run |
| GET | `/api/v1/benchmarks/runs/{id}/results` | List results for run |
| GET | `/api/v1/benchmarks/datasets` | List available datasets |
| POST | `/api/v1/benchmarks/compare` | Compare two runs |

All endpoints gated by `DevModeOnly` middleware.

#### Key Files

| File | Purpose |
|------|---------|
| `workers/codeforge/evaluation/runner.py` | BenchmarkRunner (dataset execution + evaluation) |
| `workers/codeforge/evaluation/metrics.py` | DeepEval metric wrappers |
| `workers/codeforge/evaluation/litellm_judge.py` | LiteLLM judge for DeepEval |
| `workers/codeforge/evaluation/datasets.py` | Dataset loading and result persistence |
| `workers/codeforge/evaluation/collaboration.py` | IDS + UPR collaboration metrics |
| `workers/codeforge/evaluation/dag_builder.py` | CollaborationDAG from agent messages |
| `workers/codeforge/tracing/setup.py` | TracingManager with AgentNeo/NoOp fallback |
| `workers/codeforge/tracing/metrics.py` | AgentNeo metric wrappers |
| `internal/service/benchmark.go` | Go benchmark service (CRUD + dataset listing) |
| `internal/adapter/postgres/benchmark.go` | PostgreSQL benchmark store |
| `internal/adapter/http/handlers_benchmark.go` | HTTP handlers for benchmark API |
| `configs/benchmarks/basic-coding.yaml` | Sample benchmark dataset |
| `frontend/src/features/benchmarks/BenchmarkPage.tsx` | Benchmark dashboard UI |

#### ADR

See [ADR-008: Benchmark Evaluation Framework](../architecture/adr/008-benchmark-evaluation-framework.md).

### TODOs (Phase 9+)

Tracked in [todo.md](../todo.md) under Phase 9+.

- [ ] Additional backends (OpenHands, Goose, OpenCode, Plandex).
- [ ] Trajectory replay UI and audit trail.
- [ ] Session events as source of truth (Resume/Fork/Rewind).
- [ ] A2A protocol integration (agent discovery, Agent Cards).
- [ ] AG-UI protocol integration (agent to frontend streaming).
