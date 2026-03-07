# Code Review Report — 2026-03-05

> **Scope:** Full codebase review of `staging` branch (47 commits, 1,003 files changed)
> **Method:** Automated linting + AI-assisted manual review across 7 parallel passes

---

## Phase 0: Automated Checks

| Check | Result |
|-------|--------|
| golangci-lint | 0 issues |
| go test ./... | All pass (80+ packages) |
| Ruff check + format | All pass (194 files) |
| pytest workers/tests/ | 897 pass, 0 fail |
| TypeScript tsc --noEmit | 0 errors |

---

## Phase 1: Critical Findings

### Pass 1A: NATS Contracts (Go <-> Python)

| # | Sev | Finding | Status |
|---|-----|---------|--------|
| CR-N01 | CRIT | `deliver_mode` missing from Python `RunStartMessage` | **FIXED** |
| CR-N02 | HIGH | `tasks.created` no subscriber (dead-letter messages) | **FIXED** — removed orphaned publish |
| CR-N03 | HIGH | `BenchmarkRunResultPayload.Summary` uses `map[string]any` | **FIXED** — concrete `BenchmarkSummary` struct |
| CR-N04 | MED | `RecallRequest.Kind` omitempty dependency | Documented |

### Pass 1B: Security

| # | Sev | Finding | Status |
|---|-----|---------|--------|
| CR-S01 | MED | HITL feedback endpoint missing RBAC guard | **FIXED** |
| CR-S02 | MED | Agent inbox/state endpoints missing RBAC guards | **FIXED** |
| CR-S03 | LOW | `constantTimeContains` early-return timing leak | **FIXED** |

Clean: XSS (Markdown.tsx), command injection (SVN/git), path traversal, SQL injection, secrets.

### Pass 1C: Concurrency

| # | Sev | Finding | Status |
|---|-----|---------|--------|
| CR-C01 | CRIT | WS hub Broadcast `go h.remove(c)` under RLock | **FIXED** |
| CR-C02 | HIGH | `RegisterFeedbackProvider` unprotected slice | **FIXED** |
| CR-C03 | MED | Timeout goroutine uses `context.Background()` | Downgraded; safe in practice |
| CR-C04 | HIGH | `WaitForCompletion` overwrites waiter | **FIXED** |
| CR-C05 | MED | `cleanupRunState` TOCTOU false 404 | Documented |
| CR-C06 | MED | `AddOnPlanComplete` unprotected slice | **FIXED** |

Clean: syncWaiter, sync.Map cleanup, MemoryService.RecallSync, OrchestratorService.advancePlan.

---

## Phase 2: Important Findings

### Pass 2A: Architecture Compliance

| # | Sev | Finding | Status |
|---|-----|---------|--------|
| CR-A01 | MED | Service layer imports concrete adapters (ws, litellm, otel, lsp) instead of port interfaces | Documented; pragmatic trade-off |

Domain layer is clean (no adapter imports).

### Pass 2B: Error Handling

| # | Sev | Finding | Status |
|---|-----|---------|--------|
| CR-E01 | HIGH | `_a2a.py:79` bare `except Exception:` without `as exc` | **FIXED** |
| CR-E02 | MED | Silently ignored feedback audit DB error | **FIXED** |
| CR-E03 | LOW | Discarded tiered cache object in `main.go:162` | Documented; intentional stub |
| CR-E04 | LOW | `ChatPanel.tsx` swallows stop-conversation error | Documented |

Clean: All NATS handlers call `msg.ack()` on error. All `error=str(exc)` log calls correct.

### Pass 2C: Database Migrations

| # | Sev | Finding | Status |
|---|-----|---------|--------|
| CR-M01 | MED | `agent_events` table missing composite indexes for (tenant_id + run_id/task_id/agent_id) queries | **FIXED** — migration 058 |

Clean: 57 migrations, sequential numbering, all have up+down markers. Latest 5 have good index coverage.

---

## Pre-Review Bug Fixes

| Bug | Status |
|-----|--------|
| SVN nil pool (`register.go:7`) | Non-issue: Pool.Run handles nil |
| Python recall silent failure (`_memory.py:101`) | **FIXED** |
| Routing test failing (`test_routing_integration.py`) | **FIXED** |

---

## Final Summary

| Severity | Total | Fixed | Remaining |
|----------|-------|-------|-----------|
| CRITICAL | 2 | 2 | 0 |
| HIGH | 5 | 5 | 0 |
| MEDIUM | 8 | 3 | 5 (documented) |
| LOW | 3 | 1 | 2 (documented) |

### All tracked items resolved

1. **CR-N02:** `tasks.created` dead-letter — **FIXED**: removed orphaned publish from `TaskService.Create`
2. **CR-N03:** `BenchmarkRunResultPayload.Summary` typed as `any` — **FIXED**: concrete `BenchmarkSummary` struct
3. **CR-M01:** `agent_events` composite indexes — **FIXED**: migration 058 adds tenant-prefixed indexes

### Merge Gate: MET
- 0 CRITICAL remaining
- 0 blocking HIGH remaining
- All automated checks pass (lint, type-check, tests)

---

## Files Modified in This Review

**Bug fixes:**
- `workers/codeforge/consumer/_memory.py` — publish error on embedding failure
- `workers/tests/test_routing_integration.py` — patch YAML config in test

**NATS contract fix:**
- `workers/codeforge/models.py` — add `deliver_mode` field to `RunStartMessage`

**Security fixes:**
- `internal/adapter/http/routes.go` — RBAC guards on feedback + agent inbox/state endpoints
- `internal/middleware/a2a_auth.go` — constant-time key comparison without early return

**Concurrency fixes:**
- `internal/adapter/ws/handler.go` — collect dead conns, remove after RLock release
- `internal/service/runtime.go` — mutex-protected feedback providers slice
- `internal/service/runtime_approval.go` — snapshot providers under RLock
- `internal/service/conversation_agent.go` — reject duplicate completion waiters
- `internal/service/orchestrator.go` — mutex-protected callback registration

**Error handling fixes:**
- `workers/codeforge/consumer/_a2a.py` — capture exception in bare except
- `internal/adapter/http/handlers_agent_features.go` — log feedback audit errors

**Follow-up fixes (CR-N02, CR-N03, CR-M01):**
- `internal/service/task.go` — removed orphaned `tasks.created` NATS publish
- `internal/service/task_test.go` — updated tests for no-publish behavior
- `internal/port/messagequeue/schemas.go` — concrete `BenchmarkSummary` struct replaces `map[string]any`
- `internal/service/benchmark.go` — use typed `BenchmarkSummary` fields, remove `avgFromSummary` helper
- `internal/adapter/postgres/migrations/058_agent_events_tenant_composite_indexes.sql` — tenant-prefixed indexes
