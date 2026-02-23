# Feature: Agent Orchestration (Pillar 4)

> Status: Core implemented (Phases 2-6) -- agent backends, runtime API, policy layer, multi-agent orchestration, 4-tier Code-RAG
> Priority: Phase 2-6 completed; Phase 9+ for additional backends and advanced features
> Architecture reference: [architecture.md](../architecture.md) -- "Agent Execution", "Worker Modules", "Modes System"

### Purpose

Coordination of various AI coding agents through a **unified** orchestration layer. Agents are swappable backends that run in configurable execution modes with safety controls and quality assurance.

### Agent Backends

| Agent | Adapter | Priority | Type |
|---|---|---|---|
| Aider | `adapter/aider/` | Existing | Full-featured (own tools) |
| OpenHands | `adapter/openhands/` | Existing | Full-featured (own tools) |
| SWE-agent | `adapter/sweagent/` | Existing | ReAct loop, ACI |
| Goose | `adapter/goose/` | Priority 1 | MCP-native, Rust |
| OpenCode | `adapter/opencode/` | Priority 1 | Go, LSP-aware |
| Plandex | `adapter/plandex/` | Priority 1 | Planning-first, diff sandbox |

All backends implement the `agentbackend.Backend` interface with capability declarations.

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

### TODOs (Phase 9+)

Tracked in [todo.md](../todo.md) under Phase 9+.

- [ ] Additional backends (OpenHands, Goose, OpenCode, Plandex).
- [ ] Trajectory replay UI and audit trail.
- [ ] Session events as source of truth (Resume/Fork/Rewind).
- [ ] A2A protocol integration (agent discovery, Agent Cards).
- [ ] AG-UI protocol integration (agent to frontend streaming).
