# Audit: Documentation vs. Codebase — Discrepancy Analysis

> **Date:** 2026-02-26
> **Scope:** Phases 0-21, all feature docs, CLAUDE.md, todo.md, project-status.md
> **Method:** Automated grep sweeps + targeted code review across Go, Python, TypeScript layers

---

## Executive Summary

| Category | Count | Severity |
|----------|-------|----------|
| **A — Stub as COMPLETED** | 1 | HIGH |
| **B — Defined-but-Unwired (Dead Code)** | 1 | MEDIUM |
| **C — Dispatch-Only (No Consumer)** | 4 | MEDIUM |
| **D — Status-Label Outdated** | 4 | LOW |
| **E — CLAUDE.md Overclaim** | 8 | MEDIUM |
| **F — Feature-Doc Underclaim** | 5 | LOW |
| **G — Contradictory Status** | 2 | MEDIUM |
| **Total** | **25** | |

**HIGH findings:** 1 | **MEDIUM findings:** 15 (5 resolved) | **LOW findings:** 9

---

## Detailed Findings

### Category A — Stub Marked as COMPLETED (HIGH)

| # | Feature | Doc Location | Code Location | Evidence | Fix Type |
|---|---------|-------------|---------------|----------|----------|
| 1 | A2A Protocol | `project-status.md:550` `[x]` | `internal/port/a2a/handler.go` | Line 3-8: "Phase 2-3 stub — discovery-only implementation". In-memory task map (line 26), tasks created with `Status: "queued"` but never dispatched (lines 51-76). 269 LOC total, 107 LOC tests. | **docs-fix** — Doc says `[x]` but correctly describes it as "stub". Label is technically accurate since it says "A2A Protocol Stub". No action needed unless the phase claims full A2A. |

**Note on A2A:** The documentation at `project-status.md:550` explicitly says "A2A Protocol Stub" and describes the scope correctly. This is a borderline case — the stub IS complete, but the A2A protocol itself is not functional. The `features/04-agent-orchestration.md:439` correctly lists full A2A as `[ ]` TODO.

---

### Category B — Defined-but-Unwired / Dead Code (MEDIUM)

| # | Feature | Doc Location | Code Location | Evidence | Fix Type |
|---|---------|-------------|---------------|----------|----------|
| 2 | OTEL Agent Spans + Metrics | `project-status.md:549` `[x]` | `internal/adapter/otel/spans.go`, `metrics.go` | **RESOLVED (2026-02-26).** All 3 span helpers and 6 metric instruments now called from `runtime.go` (6 methods) and `conversation.go` (3 methods). `NewMetrics()` instantiated in `main.go`, injected via `SetMetrics()`. Run spans stored in `sync.Map`, cleaned up in `cleanupRunState()`. | **Resolved** — code-fix applied in commit `4b1d362`. |

---

### Category C — Dispatch-Only / No Python Consumer (MEDIUM)

| # | Feature | Doc Location | Code Location | Evidence | Fix Type |
|---|---------|-------------|---------------|----------|----------|
| 3 | Goose Backend | `features/04:436` | `internal/adapter/goose/backend.go` (74 LOC) | **RESOLVED (2026-02-26).** Python `GooseExecutor` stub with correct `BackendInfo`, `check_available()`, and explicit "not yet implemented" error. Consumer routes via `BackendRouter`. | **Resolved** — code-fix applied in commit `4b1d362`. |
| 4 | OpenHands Backend | `features/04:436` | `internal/adapter/openhands/backend.go` (74 LOC) | **RESOLVED (2026-02-26).** Python `OpenHandsExecutor` stub. Same pattern as Goose. | **Resolved** |
| 5 | OpenCode Backend | `features/04:436` | `internal/adapter/opencode/backend.go` (73 LOC) | **RESOLVED (2026-02-26).** Python `OpenCodeExecutor` stub. Same pattern as Goose. | **Resolved** |
| 6 | Plandex Backend | `features/04:436` | `internal/adapter/plandex/backend.go` (73 LOC) | **RESOLVED (2026-02-26).** Python `PlandexExecutor` stub. Same pattern as Goose. | **Resolved** |

**RESOLVED (2026-02-26):** Python consumer now extracts backend name from NATS subject (`tasks.agent.<name>`) and routes via `BackendRouter` to backend-specific executors. `AiderExecutor` wraps the real CLI. Goose/OpenHands/OpenCode/Plandex return explicit "not yet implemented" errors instead of silently falling back to generic LLM completion. 40 new tests (router, aider subprocess mocks, stub executors, consumer).

---

### Category D — Status-Label Outdated (LOW)

| # | Feature | Doc Location | Evidence | Fix Type |
|---|---------|-------------|----------|----------|
| 7 | Phase 10 Header | `project-status.md:562` | Header says `(IN PROGRESS)` but all 36 sub-items (10A-10G) are `[x]`. Zero `[ ]` items remain. | **docs-fix** — Change to `(COMPLETED)` |
| 8 | Phase 9D AG-UI checkbox | `project-status.md:551` | Phase 9D header says `(COMPLETED)` but AG-UI line 551 is `[ ]`. However, code shows AG-UI events ARE emitted: `conversation.go:195,216,387` and `runtime.go:319,457` broadcast AG-UI events via `s.hub.BroadcastEvent()`. Frontend `websocket.ts` has AG-UI event handlers. `todo.md:286` correctly shows `[x]`. | **docs-fix** — Change `[ ]` to `[x]` at line 551, update description to reflect actual implementation. |
| 9 | Phase 18 Label Format | `project-status.md:999` | Uses `(COMPLETE)` instead of standard `(COMPLETED)` used in all other phases. | **docs-fix** — Change to `(COMPLETED)` |
| 10 | Blue-Green Deployment | `project-status.md:552` `[x]` | `docker-compose.blue-green.yml` (74 LOC) and `scripts/deploy-blue-green.sh` (87 LOC) exist and work. But zero CI/CD integration — `.github/workflows/ci.yml` has no blue-green jobs. No automated tests. Manual deployment only. | **docs-fix** — Add note: "manual deployment only, no CI integration". |

---

### Category E — CLAUDE.md Overclaim / Pattern Not Implemented (MEDIUM)

CLAUDE.md lists these as "adopted patterns" or architecture components, but no implementation exists in the codebase. They were added via commit `0466649` ("Integrate LangGraph, CrewAI, AutoGen, MetaGPT analysis and patterns") as aspirational design documentation, not as descriptions of implemented features.

| # | Pattern | CLAUDE.md Section | Code Search | Evidence | Fix Type |
|---|---------|-------------------|-------------|----------|----------|
| 11 | Composite Memory Scoring | "Framework Insights" | `grep -rn "composite.*memory\|memory.*scor" internal/ workers/` | 0 matches. Described in `docs/architecture.md:1076,1112-1141` as design only. | **docs-fix** — Clarify as "planned pattern" not "adopted" |
| 12 | Experience Pool (@exp_cache) | "Framework Insights" | `grep -rn "exp_cache\|experience.*pool" internal/ workers/` | 0 matches. Described in `docs/architecture.md:1078,1143-1147` as design only. | **docs-fix** |
| 13 | Copilot Token Exchange | "LLM Integration" | `grep -rn "copilot.*token\|CopilotToken" internal/` | 0 matches. `todo.md:1753` correctly marks as `[ ]`. CLAUDE.md states it as existing: "GitHub Copilot Token Exchange as provider (Go Core)". Also `features/03:164` correctly `[ ]`. | **docs-fix** — Add "planned" qualifier |
| 14 | HandoffMessage Pattern | "Framework Insights" | `grep -rn "HandoffMessage\|handoff" internal/ workers/` | 0 matches in code. Only in `docs/architecture.md:1090,1316-1330` as design. | **docs-fix** |
| 15 | Human Feedback Provider Protocol | "Framework Insights" | `grep -rn "FeedbackProvider\|HumanFeedback" internal/ workers/` | 0 matches. Basic HITL via `DecisionAsk` exists in `agent_loop.py` but no extensible provider system (Slack/Email). | **docs-fix** |
| 16 | MicroAgent System | "Coding Agent Insights" | `grep -rn "MicroAgent\|microagent" internal/ workers/` | 0 matches. No YAML+Markdown trigger-driven agent system. | **docs-fix** |
| 17 | RouterLLM | "Coding Agent Insights" | `grep -rn "RouterLLM\|router.*llm" internal/ workers/` | 0 matches. LiteLLM Proxy handles all routing via tags. No local RouterLLM. | **docs-fix** |
| 18 | Skills System | "Coding Agent Insights" | `grep -rn "SkillsSystem\|skills.*system" internal/ workers/` | 0 matches. Agent loop uses fixed 7 built-in tools. No dynamic skill loading. | **docs-fix** |

**Patterns that ARE implemented (no fix needed):**

| Pattern | Code Location | Status |
|---------|---------------|--------|
| BM25S Keyword Retrieval | `workers/codeforge/retrieval.py` (700+ LOC), `mcp_workbench.py` | Fully implemented + tested |
| GraphRAG | `workers/codeforge/graphrag.py` (753 LOC), migration 016, REST API | Fully implemented + tested |
| Shadow Git Checkpoints | `internal/service/checkpoint.go` (162 LOC) | Fully implemented + tested |
| Stall Detection (MagenticOne) | `internal/domain/run/stall.go` (103 LOC), 13 tests | Fully implemented. Note: `ReplanStep()` exists at `orchestrator.go:793` but is not auto-triggered from stall detection. |

---

### Category F — Feature-Doc Underclaim / Code Exists but Docs Say TODO (LOW)

| # | Feature | Doc Location | Code Location | LOC | Tests | Fix Type |
|---|---------|-------------|---------------|-----|-------|----------|
| 19 | Spec Kit Adapter | `features/02:106` `[ ]` | `internal/adapter/speckit/provider.go` | 115 | Yes (180 LOC, 7 tests) | **docs-fix** — Change to `[x]` |
| 20 | Autospec Adapter | `features/02:107` `[ ]` | `internal/adapter/autospec/provider.go` | ~115 | Yes | **docs-fix** — Change to `[x]` |
| 21 | SVN Adapter | `features/01:92` `[ ]` | `internal/adapter/svn/provider.go` | 221 | Yes (77 LOC, 7 tests) | **docs-fix** — Change to `[x]` |
| 22 | Scenario Router | `features/03:162` `[ ]` | `internal/domain/mode/mode.go` (Go), `workers/codeforge/executor.py` + `llm.py` (Python) | Dispersed | Yes (66+ tests in `test_llm.py`) | **docs-fix** — Change to `[x]` |
| 23 | Local Model Discovery | `features/03:163` — note: doc section (lines 106-113) describes implementation in detail, but TODO at line 163 says `[ ]` | `internal/adapter/litellm/client.go:186-373` (`DiscoverModels`, `DiscoverOllamaModels`, `SelectStrongestModel`), `internal/service/model_registry.go` (180 LOC) | 951 total | Yes (10+ tests) | **docs-fix** — Remove `[ ]` at line 163 or change to `[x]`. The feature section already describes the implementation. |

**Total underclaimed code:** 1,544+ LOC of production-ready, tested implementation marked as TODO.

---

### Category G — Contradictory Status Across Documents (MEDIUM)

| # | Feature | Location 1 | Status 1 | Location 2 | Status 2 | Evidence | Fix Type |
|---|---------|-----------|----------|-----------|----------|----------|----------|
| 24 | SyncService pushToPM | `todo.md:1675-1678` | `[ ]` "currently a stub returning nil" | `todo.md:1815` | `[x]` (2026-02-26) "replace stub logs with actual provider calls" | Same work package (WP2) appears in two sections: the original "Stub Discoveries" section (unchecked) and the "Stub Audit Remediation" section (checked). The remediation was completed on 2026-02-26. | **docs-fix** — Remove or strike through lines 1675-1678 |
| 25 | AG-UI Implementation Status | `project-status.md:551` | `[ ]` "no events emitted" | `todo.md:286` | `[x]` (2026-02-24) "events emitted in runtime.go + conversation.go" | Code confirms todo.md is correct: 5 active BroadcastEvent calls in service layer. project-status.md is stale. | **docs-fix** — Update project-status.md:551 to `[x]` |

---

## Recommendations by Priority

### Priority 1 — Fix Now (Contradictions & Misleading Status)

| Finding | Action |
|---------|--------|
| #25 AG-UI in project-status.md | Change line 551 from `[ ]` to `[x]`, update description |
| #7 Phase 10 header | Change line 562 from `(IN PROGRESS)` to `(COMPLETED)` |
| #24 pushToPM duplicate | Remove or mark lines 1675-1678 as resolved |
| #9 Phase 18 label | Change `(COMPLETE)` to `(COMPLETED)` at line 999 |

### Priority 2 — Update Feature Docs (Underclaims)

| Finding | Action |
|---------|--------|
| #19 Spec Kit | `features/02:106` — change `[ ]` to `[x] (2026-02-19)` |
| #20 Autospec | `features/02:107` — change `[ ]` to `[x] (2026-02-19)` |
| #21 SVN | `features/01:92` — change `[ ]` to `[x] (2026-02-19)` |
| #22 Scenario Router | `features/03:162` — change `[ ]` to `[x] (2026-02-23)` |
| #23 Local Model Discovery | `features/03:163` — change `[ ]` to `[x] (2026-02-19)` |

### Priority 3 — Clarify CLAUDE.md (Overclaims) — RESOLVED (2026-02-26)

Added `— *planned*` qualifiers to 8 unimplemented patterns in CLAUDE.md:
- **Framework Insights:** Composite Memory Scoring, Experience Pool, HandoffMessage Pattern, Human Feedback Provider Protocol
- **Coding Agent Insights (OpenHands, SWE-agent):** Microagents, Skills System, RouterLLM
- **LLM Integration:** GitHub Copilot Token Exchange

Also added clarifying note to Framework Insights section header:
> Reference patterns from framework analysis. Items without a file path are planned, not yet implemented.

### Priority 4 — Code Improvements — RESOLVED (2026-02-26)

Both findings resolved in commit `4b1d362` on `staging`:

| Finding | Action | Resolution |
|---------|--------|------------|
| #2 OTEL spans/metrics | Wire `StartRunSpan`/`StartToolCallSpan` into `runtime.go` and `conversation.go`. Instantiate `NewMetrics()` and call `RunsStarted.Add()` etc. at execution points. | **Done.** `NewMetrics()` instantiated in `main.go`, injected via setters. `runtime.go` instruments 6 methods (StartRun, HandleToolCallRequest, finalizeRun, cancelRunWithReason, cleanupRunState, triggerDelivery). `conversation.go` instruments 3 methods (SendMessage, SendMessageAgentic, HandleConversationRunComplete). Run spans stored in `sync.Map`, nil-guarded metrics. |
| #3-6 Agent backends | Implement backend-specific Python consumers or remove Go adapter stubs to avoid dead code. The generic fallback silently degrades all backends to LLM completion. | **Done.** Python `BackendRouter` dispatcher with 5 registered executors. `AiderExecutor` wraps real CLI subprocess. Goose/OpenHands/OpenCode/Plandex return explicit "not yet implemented" errors. Consumer extracts backend name from NATS subject and routes accordingly. 40 new Python tests. |

---

## Appendix: Verification Commands

```bash
# Verify OTEL wiring (Finding #2) — RESOLVED
grep -rn "StartRunSpan\|StartToolCallSpan\|RunsStarted\|RunsCompleted\|NewMetrics" internal/service/ cmd/ --include="*.go"
# Expected: multiple matches in runtime.go, conversation.go, main.go

# Verify agent backend Python routing (Findings #3-6) — RESOLVED
grep -rn "goose\|openhands\|opencode\|plandex\|BackendRouter\|BackendExecutor" workers/codeforge/ --include="*.py"
# Expected: matches in backends/*.py, consumer.py

# Verify CLAUDE.md overclaims (Findings #11-18)
grep -rn "exp_cache\|HandoffMessage\|FeedbackProvider\|MicroAgent\|RouterLLM\|SkillsSystem" internal/ workers/ --include="*.go" --include="*.py"
# Expected: 0 matches

# Verify underclaim code exists (Findings #19-23)
wc -l internal/adapter/speckit/provider.go internal/adapter/svn/provider.go internal/service/model_registry.go
# Expected: 115 + 221 + 180 = 516 lines
```
