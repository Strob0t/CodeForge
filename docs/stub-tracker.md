# CodeForge — Stub & Placeholder Tracker

> Generated: 2026-03-17 via stub-finder scan. Updated: 2026-03-18 (reconciled after STUB-001/010 implementation).
> This document tracks ALL stubs, placeholders, and unimplemented code paths across the codebase.
> Each item has a unique ID (STUB-NNN) for cross-referencing in commits and todo.md.

---

## CRITICAL — Production Stubs (will fail or silently no-op)

### ~~STUB-001: A2A In-Memory Task Store (Go)~~ **FIXED 2026-03-18**
- **File:** ~~`internal/port/a2a/handler.go`~~ (legacy, dead code)
- **Phase:** A2A Phase 2-3
- **Description:** ~~Tasks stored in-memory only.~~ The legacy `internal/port/a2a/handler.go` was superseded by the SDK-based implementation in `internal/adapter/a2a/` (PostgreSQL persistence via migration 054, `store_a2a.go`). Python consumer mixin `workers/codeforge/consumer/_a2a.py` handles inbound A2A tasks via NATS (`a2a.task.created` -> execute -> `a2a.task.complete`).
- **Tests:** `workers/tests/test_consumer_a2a.py` (9 tests), `workers/tests/test_a2a_executor.py` (8 tests), `workers/tests/test_a2a_mixin_executor.py` (4 tests)
- **Cleanup:** `internal/port/a2a/` is dead code (zero imports) — should be deleted. Re-introduced by merge artifact.

### ~~STUB-002: StubBackendExecutor.info Raises NotImplementedError (Python)~~ **FIXED 2026-03-17**
- **File:** `workers/codeforge/backends/_base.py:99`
- **Phase:** N/A (base class)
- **Description:** ~~The `StubBackendExecutor` base class `info` property raises `NotImplementedError`.~~ Converted to ABC with `@abstractmethod`. Missing override now caught at class definition time.
- **Tests:** `workers/tests/test_stub_backend_abc.py` (5 tests)

### STUB-003: Review Trigger Handler Is a No-Op (Python)
- **File:** `workers/codeforge/consumer/_review.py:35`
- **Phase:** Phase 31 (Contract-First Review/Refactor)
- **Description:** `_do_review_trigger()` logs "review trigger received" then does nothing. Has `# TODO: Dispatch boundary-analyzer run via agent loop`. NATS review trigger messages are silently discarded.
- **Impact:** Phase 31 review triggers (pipeline-completion, branch-merge, manual) are accepted but never executed.
- **Fix:** Implement dispatch to boundary-analyzer agent loop, wire to orchestrator.
- **Effort:** Medium (agent loop dispatch, orchestrator wiring)

---

## HIGH — Incomplete Features (partially implemented)

### STUB-004: Budget Tracking & Stall Detection Hardcoded to Zero (Go) — **PARTIALLY FIXED 2026-03-18**
- **File:** `internal/service/conversation_agent.go:149-152`
- **Phase:** Phase 17 (Agentic Conversation Loop)
- **Description:** `StallIterations` now wired via `countStallIterations(history)` which counts consecutive non-progress tool calls. `BudgetPercent` is still hardcoded to `0.0` — Python worker does not yet report running cost back to Go.
- **Remaining:** Wire `BudgetPercent` when Python worker reports running cost via NATS.
- **Tests:** `internal/service/prompt_assembler_test.go` (table-driven `countStallIterations` tests)
- **Effort:** Small (remaining: wire cost from Python worker)

### ~~STUB-005: UpdateConversationMode/Model Are No-Op Stubs (Go)~~ **FIXED 2026-03-17**
- **File:** `internal/adapter/postgres/store_conversation.go`
- **Phase:** Phase 9 (Chat Enhancements)
- **Description:** ~~Both methods were no-op stubs.~~ Added `mode` and `model` columns (migration 076), updated all CRUD queries, implemented real UPDATE methods with tenant isolation.
- **Tests:** `TestConversation_SetMode_Persisted`, `TestConversation_SetModel_Persisted`

### ~~STUB-006: A2A Agent Card Skills Are Hardcoded (Go)~~ **FIXED 2026-03-18**
- **File:** `internal/adapter/a2a/agentcard.go` (SDK-based CardBuilder)
- **Phase:** A2A Phase 2-3
- **Description:** ~~Skills hardcoded.~~ The SDK-based `CardBuilder` in `internal/adapter/a2a/agentcard.go` builds skills dynamically from registered `ModeInfo` entries. Falls back to a single "code-task" skill when no modes are registered. The legacy `internal/port/a2a/agentcard.go` also has dynamic skills but is dead code.
- **Tests:** `internal/port/a2a/handler_test.go` (legacy), `internal/adapter/a2a/` (SDK integration)

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

### ~~STUB-010: SWE-agent Backend Not Yet Implemented (Docs)~~ **FIXED 2026-03-17**
- **File:** `workers/codeforge/backends/sweagent.py`
- **Phase:** Phase 9+
- **Description:** ~~SWE-agent backend marked "Not yet implemented" in feature table.~~ Implemented `SweagentExecutor` as CLI subprocess wrapper following OpenCode pattern. Registered in default router.
- **Tests:** `workers/tests/test_sweagent_backend.py` (15 tests)

---

## MEDIUM — Hardcoded/Placeholder Data (works but with limitations)

### ~~STUB-011: Goal Discovery Returns Partial ProjectGoal Objects (Go)~~ **FIXED 2026-03-17**
- **File:** `internal/service/goal_discovery.go`
- **Phase:** N/A
- **Description:** ~~`detectGoalFiles()` returned partial ProjectGoal objects.~~ Introduced `DetectedGoal` intermediate type (no ID/ProjectID/TenantID). `ToProjectGoal(projectID)` converts to full `ProjectGoal` with required fields.
- **Tests:** `TestDetectedGoal_NoProjectIDOrTenantID`, `TestDetectedGoal_ToProjectGoal` + all existing detection tests pass

### ~~STUB-012: Review Pipeline Creates Placeholder StepBindings (Go)~~ **FIXED 2026-03-18**
- **File:** `internal/service/review.go:267`, `internal/domain/pipeline/pipeline.go:144`
- **Phase:** Phase 31
- **Description:** ~~Placeholder UUIDs.~~ `pipeline.Instantiate()` now auto-generates TaskID/AgentID UUIDs when bindings are nil or have empty fields. `review.go` passes nil bindings, removing manual placeholder creation.
- **Tests:** `TestInstantiate_EmptyBinding_AutoFills`, `TestInstantiate_PartialBindings_AutoFill`

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

### STUB-024: Disabled Re-Run Benchmark Button (TypeScript)
- **File:** `frontend/src/features/benchmarks/PromptOptimizationPanel.tsx:173`
- **Phase:** Benchmark System
- **Description:** "Re-run Benchmark" button permanently disabled (`disabled` attr, no `onClick`). Comment: `{/* Re-run placeholder */}`.
- **Impact:** Users cannot rerun benchmarks from the Prompt Optimization panel.
- **Fix:** Wire onClick to trigger a new benchmark run for the current prompt variant.
- **Effort:** Small

### STUB-025: Deprecated activeTool Prop in CanvasModal (TypeScript)
- **File:** `frontend/src/features/canvas/CanvasModal.tsx:33-34`
- **Phase:** Phase 32 (Visual Design Canvas)
- **Description:** `activeTool?: CanvasTool` prop marked `@deprecated`. Unused at runtime.
- **Impact:** None.
- **Fix:** Remove deprecated prop from interface.
- **Effort:** Trivial

### STUB-026: GitHub Copilot Provider Commented Out in LiteLLM Config (YAML)
- **File:** `litellm/config.yaml:76-85`
- **Description:** `github_copilot/*` provider block commented out. Device-code auth blocks LiteLLM startup. Token exchange handled by Go Core.
- **Impact:** None — intentional exclusion.
- **Fix:** N/A unless LiteLLM adds async auth.
- **Effort:** N/A

---

## INFO — Test Stubs (expected, no action needed)

| Language | Count | Details |
|----------|-------|---------|
| Go | ~220 mock methods across ~208 `*_test.go` files | Standard interface mocks (e.g., `teststore_test.go` has 50+ stubs) |
| Python | ~35 Fake/Stub classes across 20 `tests/` files | FakeLLM, FakeExecutor, StubEvaluator, _FakeResponse, etc. |
| TypeScript | 0 in `frontend/src/` | Tests live in `frontend/e2e/` |

## Unchecked Items in docs/todo.md

**23 unchecked `[ ]` items** — Phase 32 (Visual Design Canvas) + remaining stubs.
See `docs/todo.md` for full list.

---

## Summary

| Category | Count | IDs |
|----------|-------|-----|
| **CRITICAL** | **0** (~~3~~) | ~~STUB-001~~, ~~STUB-002~~, STUB-003 (demoted to HIGH — no-op, not data loss) |
| **HIGH** | **4** (~~8~~) | STUB-003, STUB-004 (partial), ~~STUB-005~~, ~~STUB-006~~, STUB-007, STUB-008, STUB-009, ~~STUB-010~~, STUB-024 |
| **MEDIUM** | **3** (~~5~~) | ~~STUB-011~~, ~~STUB-012~~, STUB-013, STUB-014, STUB-015 |
| **LOW** | **10** | STUB-016 through STUB-023, STUB-025, STUB-026 |
| **INFO** | ~255 | Test stubs (no action) |
| **docs/todo.md unchecked** | 23 | Phase 32 + remaining stubs |
| **TOTAL (actionable)** | **17** | (down from 23) |

---

## Recommended Fix Order

**COMPLETED 2026-03-17:**
1. ~~STUB-002 — Convert StubBackendExecutor to ABC~~
2. ~~STUB-005 — Add mode/model columns to conversations table~~
3. ~~STUB-011 — Goal discovery type refactor~~

**COMPLETED 2026-03-18:**
4. ~~STUB-001 — A2A task persistence & execution (Python consumer + SDK integration)~~
5. ~~STUB-006 — Dynamic A2A agent card skills (already in SDK CardBuilder)~~
6. ~~STUB-010 — SWE-agent backend implementation~~
7. ~~STUB-012 — Pipeline auto-generates StepBindings~~
8. ~~STUB-004 (partial) — Stall detection wired~~

**Remaining — small effort:**
9. STUB-004 (remaining) — Wire BudgetPercent from Python worker cost reporting
10. STUB-024 — Wire re-run benchmark button onClick
11. STUB-025 — Remove deprecated CanvasModal activeTool prop
12. Cleanup: delete dead code `internal/port/a2a/` (merge artifact, zero imports)

**Remaining — medium effort:**
13. STUB-003 — Implement review trigger dispatch
14. STUB-009 — Event dedup + WS reconnect gap

**No action needed:**
- STUB-007, STUB-008 (intentional feature gates)
- STUB-013, STUB-014, STUB-015 (intentional no-ops)
- STUB-016 through STUB-023, STUB-026 (docs/config, tracked elsewhere)
