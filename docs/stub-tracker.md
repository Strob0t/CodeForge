# CodeForge — Stub & Placeholder Tracker

> Generated: 2026-03-17 via stub-finder scan.
> This document tracks ALL stubs, placeholders, and unimplemented code paths across the codebase.
> Each item has a unique ID (STUB-NNN) for cross-referencing in commits and todo.md.

---

## CRITICAL — Production Stubs (will fail or silently no-op)

### STUB-001: A2A In-Memory Task Store (Go)
- **File:** `internal/port/a2a/handler.go:3,24,29`
- **Phase:** A2A Phase 2-3
- **Description:** Entire A2A handler is marked `STATUS: Phase 2-3 stub — discovery-only implementation`. Tasks are accepted via `POST /a2a/tasks` but stored only in `map[string]*TaskResponse` — lost on restart, never dispatched to agent backends.
- **Impact:** A2A protocol endpoints accept tasks but they are lost on restart and never executed.
- **Fix:** Persist tasks to PostgreSQL, route to agent backends for execution.
- **Effort:** Large (new migration, service layer, NATS integration)

### STUB-002: StubBackendExecutor.info Raises NotImplementedError (Python)
- **File:** `workers/codeforge/backends/_base.py:99`
- **Phase:** N/A (base class)
- **Description:** The `StubBackendExecutor` base class `info` property raises `NotImplementedError`. Any future backend extending this class without overriding `info` will crash at runtime.
- **Impact:** Current backends all override it, but it's a landmine for new backends.
- **Fix:** Convert to ABC with `@abstractmethod` so missing override is caught at class definition time, not runtime.
- **Effort:** Small (change base class to ABC, add `@abstractmethod`)

### STUB-003: Review Trigger Handler Is a No-Op (Python)
- **File:** `workers/codeforge/consumer/_review.py:35`
- **Phase:** Phase 31 (Contract-First Review/Refactor)
- **Description:** `_do_review_trigger()` logs "review trigger received" then does nothing. Has `# TODO: Dispatch boundary-analyzer run via agent loop`. NATS review trigger messages are silently discarded.
- **Impact:** Phase 31 review triggers (pipeline-completion, branch-merge, manual) are accepted but never executed.
- **Fix:** Implement dispatch to boundary-analyzer agent loop, wire to orchestrator.
- **Effort:** Medium (agent loop dispatch, orchestrator wiring)

---

## HIGH — Incomplete Features (partially implemented)

### STUB-004: Budget Tracking & Stall Detection Hardcoded to Zero (Go)
- **File:** `internal/service/conversation_agent.go:150-151`
- **Phase:** Phase 17 (Agentic Conversation Loop)
- **Description:** Template data for dynamic system reminders uses hardcoded zeros:
  ```go
  "BudgetPercent":   0.0, // TODO: Wire actual budget tracking
  "StallIterations": 0,   // TODO: Wire stall detection
  ```
- **Impact:** Dynamic system reminders cannot warn agents about budget exhaustion or stall conditions.
- **Fix:** Read actual budget percentage from cost tracking service, stall iteration count from agent loop state.
- **Effort:** Medium (wire existing services into template data)

### STUB-005: UpdateConversationMode/Model Are No-Op Stubs (Go)
- **File:** `internal/adapter/postgres/store_conversation.go:117-125`
- **Phase:** Phase 9 (Chat Enhancements)
- **Description:** Both methods are explicitly commented as stubs:
  ```go
  // UpdateConversationMode is a stub — the conversations table does not yet have a mode column.
  func (s *Store) UpdateConversationMode(_ context.Context, _, _ string) error { return nil }
  // UpdateConversationModel is a stub — the conversations table does not yet have a model column.
  func (s *Store) UpdateConversationModel(_ context.Context, _, _ string) error { return nil }
  ```
- **Impact:** Slash commands `/mode` and `/model` appear to succeed but don't persist. Mode/model resets on page reload.
- **Fix:** Add `mode` and `model` columns to conversations table (migration), implement UPDATE queries.
- **Effort:** Small-Medium (migration + 2 SQL queries)

### STUB-006: A2A Agent Card Skills Are Hardcoded (Go)
- **File:** `internal/port/a2a/agentcard.go:4-32`
- **Phase:** A2A Phase 2-3
- **Description:** Comment: "Skills are hardcoded placeholders. In Phase 2-3 these will be populated dynamically." Only 2 static skills ("code-task", "decompose") returned regardless of actual agent capabilities.
- **Impact:** A2A agent card does not reflect actual registered backends or mode configurations.
- **Fix:** Build skills dynamically from mode registry and agent backend capabilities.
- **Effort:** Medium (query mode presets + backend registry at card build time)

### STUB-007: GitHub OAuth Returns 501 When Unconfigured (Go)
- **File:** `internal/adapter/http/handlers_github_oauth.go:10-12,27-28`
- **Phase:** N/A (optional feature)
- **Description:** `StartGitHubOAuth()` and `GitHubOAuthCallback()` return 501 NotImplemented if `h.GitHubOAuth == nil`.
- **Impact:** Feature-gated — works when configured, 501 otherwise. Not broken, but unimplemented unless configured.
- **Fix:** Document as optional, or implement fallback guidance.
- **Effort:** N/A (intentional gate, low priority)

### STUB-008: All 4 Subscription Endpoints Return 501 (Go)
- **File:** `internal/adapter/http/handlers_subscription.go:11-63`
- **Phase:** N/A (optional feature)
- **Description:** ListProviders, StartConnect, GetStatus, DisconnectProvider — all return 501 NotImplemented if `h.Subscription == nil`.
- **Impact:** Subscription features only work if explicitly configured. Same pattern as STUB-007.
- **Fix:** Document as optional, or ensure configuration is straightforward.
- **Effort:** N/A (intentional gate, low priority)

### STUB-009: Event Dedup & WS Reconnect Gap in Benchmark Live Feed (TypeScript)
- **File:** `frontend/src/features/benchmarks/BenchmarkPage.tsx:131-135`
- **Phase:** Benchmark Live Feed
- **Description:** Two TODO comments:
  1. API hydration and WS may produce duplicate events — needs backend `sequence_number`
  2. Events lost during WS disconnect gap — needs re-hydration on reconnect
- **Impact:** Users may see duplicate events in live feed, or miss events during WS reconnection.
- **Fix:** (1) Add monotonic `sequence_number` to trajectory events in Go, dedup in frontend. (2) Re-hydrate from API on WS reconnect.
- **Effort:** Medium (Go backend change + frontend dedup logic)

### STUB-010: SWE-agent Backend Not Yet Implemented (Docs)
- **File:** `docs/features/04-agent-orchestration.md:20`
- **Phase:** Phase 9+
- **Description:** SWE-agent backend marked "Not yet implemented" in feature table. All other backends (Aider, Goose, OpenHands, OpenCode, Plandex) are done.
- **Impact:** Users cannot use SWE-agent as an agent backend.
- **Fix:** Implement SWE-agent backend adapter following existing backend pattern.
- **Effort:** Large (new backend adapter, Docker integration, tests)

---

## MEDIUM — Hardcoded/Placeholder Data (works but with limitations)

### STUB-011: Goal Discovery Returns Partial ProjectGoal Objects (Go)
- **File:** `internal/service/goal_discovery.go:159`
- **Phase:** N/A
- **Description:** `detectGoalFiles()` returns ProjectGoal stubs without ID/ProjectID/TenantID. Calling code fills these in later.
- **Impact:** Goals are temporarily in an invalid state during processing. Fragile if calling code changes.
- **Fix:** Use a builder pattern or intermediate type that doesn't expose incomplete state.
- **Effort:** Small (type refactor)

### STUB-012: Review Pipeline Creates Placeholder StepBindings (Go)
- **File:** `internal/service/review.go:267`
- **Phase:** Phase 31
- **Description:** Creates StepBinding objects with temporary UUIDs for TaskID and AgentID. Orchestrator replaces these later.
- **Impact:** Intermediate state with placeholder UUIDs — works but fragile.
- **Fix:** Defer binding creation until orchestrator assigns real resources.
- **Effort:** Small (refactor binding creation timing)

### STUB-013: _BenchmarkRuntime No-Op Methods (Python)
- **File:** `workers/codeforge/consumer/_benchmark.py:433-457`
- **Phase:** Phase 26 (Benchmark System)
- **Description:** Three methods (`send_output`, `report_tool_result`, `publish_trajectory_event`) are `pass` no-ops. Lightweight runtime stub for isolated benchmark execution without NATS dependency.
- **Impact:** Benchmark runs don't publish trajectory events or output — intentional for isolation.
- **Fix:** N/A — intentional design. Could optionally add local logging.
- **Effort:** N/A (by design)

### STUB-014: StubBackendExecutor.cancel() Is a No-Op (Python)
- **File:** `workers/codeforge/backends/_base.py:120-121`
- **Phase:** N/A (base class)
- **Description:** `cancel()` method is `pass`. Documented as intentional in class docstring.
- **Impact:** Cancelling a stub backend does nothing.
- **Fix:** N/A — intentional.
- **Effort:** N/A

### STUB-015: Empty Async Delete Fallback in useCRUDForm (TypeScript)
- **File:** `frontend/src/hooks/useCRUDForm.ts:50-51`
- **Phase:** N/A
- **Description:** `onDelete ?? (async () => {})` — fallback when no delete handler provided. ESLint disable acknowledged.
- **Impact:** Safe by design — `del.requestConfirm()` won't be called without a delete operation.
- **Fix:** N/A — correct pattern for optional callback.
- **Effort:** N/A

---

## LOW — Documentation TODOs

### STUB-016: Benchmark Event Dedup Needs Backend sequence_number (Docs)
- **File:** `docs/todo.md:433`
- **Marker:** `TODO: Event dedup requires backend sequence_number on trajectory events`
- **Cross-ref:** STUB-009

### STUB-017: WS Reconnect Gap Needs Re-Hydration (Docs)
- **File:** `docs/todo.md:434`
- **Marker:** `TODO: WS reconnect gap requires re-hydration on reconnect`
- **Cross-ref:** STUB-009

### STUB-018: Benchmark Analysis Endpoint Returns Stub Response (Docs)
- **File:** `docs/plans/2026-03-09-benchmark-external-providers-plan.md:2342`
- **Marker:** `// For now, return a stub that the frontend can display`

### STUB-019: Benchmark Analysis Needs NATS Dispatch (Docs)
- **File:** `docs/plans/2026-03-09-benchmark-external-providers-plan.md:2353`
- **Marker:** `// TODO: Dispatch to Python worker via NATS for LLM analysis`

### STUB-020: Allow-Always Policy Persistence TODO (Docs)
- **File:** `docs/plans/2026-03-09-chat-enhancements-plan.md:284`
- **Marker:** `// TODO: persist policy rule for this tool via separate API call`
- **Note:** This was actually implemented (see todo.md "Allow Always" entry) — the plan doc is stale.

### STUB-021: Voice & Video Phase Is Stub Only (Docs)
- **File:** `docs/plans/2026-03-09-chat-enhancements-plan.md:1343`
- **Marker:** `Phase 10: Voice & Video (Stub Only)` — future, unscoped

### STUB-022: Python Worker # TODO Comments (Python)
- **File:** `workers/codeforge/tools/search_files.py` — 1 occurrence
- **File:** `workers/codeforge/llm.py` — 3 occurrences (fallback keyword handling)
- **Impact:** Minor improvement opportunities, not blocking.

### STUB-023: Commented-Out Feature Blocks in Example Config (YAML)
- **File:** `codeforge.example.yaml:79-130`
- **Description:** 4 commented-out feature sections (MCP, benchmark, A2A, routing). All features are implemented and enabled in production config — example file intentionally disables for simplicity.
- **Impact:** None — correct behavior for example configs.

---

## INFO — Test Stubs (expected, no action needed)

| Language | Count | Details |
|----------|-------|---------|
| Go | ~220 mock methods across ~208 `*_test.go` files | Standard interface mocks (e.g., `teststore_test.go` has 50+ stubs) |
| Python | ~35 Fake/Stub classes across 20 `tests/` files | FakeLLM, FakeExecutor, StubEvaluator, _FakeResponse, etc. |
| TypeScript | 0 in `frontend/src/` | Tests live in `frontend/e2e/` |

## Unchecked Items in docs/todo.md

**20 unchecked `[ ]` items** — all in Phase 32 (Visual Design Canvas).
See `docs/todo.md` lines 1167-1200 for full list.

---

## Summary

| Category | Count | IDs |
|----------|-------|-----|
| **CRITICAL** | **3** | STUB-001, STUB-002, STUB-003 |
| **HIGH** | **7** | STUB-004 through STUB-010 |
| **MEDIUM** | **5** | STUB-011 through STUB-015 |
| **LOW** | **8** | STUB-016 through STUB-023 |
| **INFO** | ~255 | Test stubs (no action) |
| **docs/todo.md unchecked** | 20 | Phase 32 tasks |
| **TOTAL (actionable)** | **23** | |

---

## Recommended Fix Order

**Quick wins (small effort, high impact):**
1. STUB-002 — Convert StubBackendExecutor to ABC (`@abstractmethod`)
2. STUB-005 — Add mode/model columns to conversations table
3. STUB-011 — Goal discovery type refactor

**Medium effort:**
4. STUB-004 — Wire budget tracking & stall detection
5. STUB-003 — Implement review trigger dispatch
6. STUB-009 — Event dedup + WS reconnect gap
7. STUB-006 — Dynamic A2A agent card skills
8. STUB-012 — Refactor placeholder StepBindings

**Large effort (phase-level work):**
9. STUB-001 — A2A task persistence & execution
10. STUB-010 — SWE-agent backend implementation

**No action needed:**
- STUB-007, STUB-008 (intentional feature gates)
- STUB-013, STUB-014, STUB-015 (intentional no-ops)
- STUB-016 through STUB-023 (docs/config, tracked elsewhere)
