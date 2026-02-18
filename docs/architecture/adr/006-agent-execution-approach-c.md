# ADR-006: Agent Execution — Approach C (Go Control Plane + Python Runtime)

> **Status:** accepted
> **Date:** 2026-02-17
> **Deciders:** Project lead + Claude Code analysis

## Context

CodeForge needs an execution architecture for running AI coding agents. The core question is: **where does the agent loop run, and who controls state?**

Three approaches were analyzed (from the project analysis document, Section 13):

- **Approach A: Go-only execution** — Go Core runs the agent loop directly
- **Approach B: Python-only execution** — Python Workers own all state and execution
- **Approach C: Go Control Plane + Python Runtime** — Go manages state/policies, Python executes tools/LLM calls

The agent execution loop involves: receiving a task → planning → requesting tool calls → executing tools → evaluating results → checking policies → delivering results. This loop has both state-heavy components (session tracking, policy evaluation, checkpoints) and compute-heavy components (LLM calls, tool execution, code analysis).

## Decision

**Approach C: Go Core as "Control Plane" (state, policies, sessions), Python Workers as "Data/Execution Plane" (models, tools, agent loop execution).**

### Architecture

```
Go Core (Control Plane)              Python Worker (Execution Plane)
========================             =============================
- Run lifecycle (create,             - LLM calls via LiteLLM
  start, complete, cancel)           - Tool execution (file, shell, git)
- Policy evaluation per              - Agent loop (plan → execute → check)
  tool call (allow/deny/ask)         - Quality gate execution (test/lint)
- Termination enforcement            - Repo map generation (tree-sitter)
  (steps, cost, timeout, stall)      - Hybrid retrieval (BM25 + embeddings)
- Checkpoint creation                - Context packing
  (shadow git commits)
- Quality gate orchestration
- Deliver (commit/branch/PR)
- WebSocket events to frontend
- Session state in PostgreSQL
```

### Communication Protocol (NATS JetStream)

```
Go ──runs.start──────────> Python    (start new run)
Go <──runs.toolcall.request── Python (request permission for tool)
Go ──runs.toolcall.response──> Python (allow/deny/ask)
Go <──runs.toolcall.result─── Python (tool execution result)
Go <──runs.complete────────── Python (run finished)
Go ──runs.cancel───────────> Python  (cancel running run)
Go <──runs.output──────────── Python (streaming output)
```

Each tool call is individually approved by the Go control plane's policy engine before the Python worker executes it. This enables per-tool-call safety enforcement without the Python worker needing to know about policies.

### State Ownership

| State | Owner | Storage |
|---|---|---|
| Run lifecycle (status, steps, cost) | Go Core | PostgreSQL |
| Policy profiles | Go Core | YAML files + in-memory |
| Checkpoints (shadow git) | Go Core | Git repository |
| Stall detection | Go Core | In-memory (per-run) |
| Agent loop state | Python Worker | In-memory (per-run) |
| LLM conversation history | Python Worker | In-memory (per-run) |
| Tool execution results | Python Worker → Go Core | NATS → PostgreSQL |

### Execution Modes

The control plane manages three execution modes:

| Mode | Implementation | Resource Management |
|---|---|---|
| **Mount** | Python worker operates on host filesystem directly | No container overhead |
| **Sandbox** | Go Core creates Docker container, Python worker runs tools via `docker exec` | Resource limits (memory, CPU, PIDs, network) |
| **Hybrid** | Container with mounted volumes (deferred) | Configurable read/write permissions |

## Consequences

### Positive

- **Clear separation of concerns:** Go handles what it's good at (concurrency, state, HTTP/WS), Python handles what it's good at (AI ecosystem, LLM SDKs, tree-sitter)
- **Policy enforcement is centralized:** All policy decisions happen in Go — Python workers never bypass safety rules
- **Horizontal scaling:** Python workers are stateless between runs; any worker can pick up any task
- **Single source of truth:** Run state lives in PostgreSQL, not scattered across worker processes
- **Frontend reactivity:** Go Core emits WebSocket events immediately on state changes (no polling Python workers)
- **Checkpoint safety:** Shadow git commits happen in Go, close to the state machine — not in a remote worker

### Negative

- **NATS round-trip per tool call:** Each tool call requires Go → Python → Go communication, adding ~1-5ms latency per step
  - Mitigation: acceptable for AI agent tasks where LLM calls take 1-30 seconds
- **Split debugging:** Issues may span Go and Python — requires correlating logs across services
  - Mitigation: Request ID propagation (ADR-005) and structured logging (ADR-004)
- **Protocol complexity:** 7 NATS subjects for the run protocol, typed payloads with schema validation
  - Mitigation: well-defined, tested; schemas prevent silent failures

### Neutral

- Python workers do not access PostgreSQL directly — all persistent state goes through Go Core via NATS
- The NATS protocol is versioned via payload schemas (`internal/port/messagequeue/schemas.go`)
- Workers can be replaced with alternative implementations (e.g., Rust) without changing the Go control plane

## Alternatives Considered

| Alternative | Pros | Cons | Why Not |
|---|---|---|---|
| **Approach A: Go-only** | Single language, no NATS round-trips, simpler deployment | Go lacks AI ecosystem (no LiteLLM, no tree-sitter, no LangGraph), CGO bindings are fragile, agent backends (Aider, OpenHands) are Python | Fighting the ecosystem; Python is required for AI workloads |
| **Approach B: Python-only** | Single language for backend, direct LLM access | Python is slow for HTTP/WS server, poor concurrency model, state management in asyncio is error-prone, need Redis/DB for coordination | Go Core already handles 10K+ concurrent connections efficiently; Python would need significant infrastructure for the same |
| **Approach C with gRPC** | Type-safe protocol, bidirectional streaming | Adds protobuf compilation step, tighter coupling, less flexible than subject-based routing | NATS JetStream already provides persistence, fan-out, and subject routing — gRPC would duplicate infrastructure |

## References

- `internal/service/runtime.go` — RuntimeService (Go control plane)
- `internal/domain/run/run.go` — Run entity and status machine
- `internal/domain/run/toolcall.go` — ToolCall request/response types
- `internal/service/checkpoint.go` — Shadow git checkpoint system
- `internal/service/sandbox.go` — Docker sandbox lifecycle
- `workers/codeforge/runtime.py` — Python RuntimeClient
- `internal/port/messagequeue/schemas.go` — NATS payload schemas
- Project analysis document, Section 13 — Approach comparison
