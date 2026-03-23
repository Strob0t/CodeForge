# Audit Remediation Design Spec

**Date:** 2026-03-23
**Status:** Reviewed (spec review iteration 1 — 7 issues fixed)
**Audit Report:** `docs/audits/2026-03-23-universal-audit-report.md`
**Scope:** Priority 2 (Hexagonal Architecture Fix) + Priority 3 (Silenced Errors + Test Coverage)

---

## 1. Problem Statement

The universal audit (2026-03-23) identified two structural issues requiring remediation:

**Priority 2 — Hexagonal Architecture Violations:**
- 31 service files import `internal/adapter/ws` for event constants and structs (F-002)
- 4 service files import `internal/adapter/otel` for metrics/spans (F-002)
- 1 service file imports `internal/adapter/lsp` concretely (F-002)
- Event type constants and payload structs live in `adapter/ws/events.go` instead of the domain layer (F-034)
- `LSPService` holds concrete `*ws.Hub` instead of `broadcast.Broadcaster` (Architecture Finding 2)
- `Handlers` struct holds concrete `*litellm.Client` and `*copilot.Client` (F-033)

**Priority 3 — Silenced Errors + Test Coverage Gaps:**
- 38 instances of `_ = s.store.*` silently discard database errors (F-008)
- `runtime_execution.go` (644 LOC, 5 critical functions) has minimal test coverage (F-010) — only `runtime_execution_extended_test.go` covers `HandleToolCallResult` budget alerts; `runtime_execution_source_test.go` covers source quality only
- `runtime_lifecycle.go` (486 LOC, 8 functions) has only `runtime_lifecycle_source_test.go` for source quality (F-010)
- `workers/codeforge/consumer/_conversation.py` (1016 LOC, 20+ methods) has zero unit tests (F-010)

---

## 2. Design Decisions

### 2.1 Event Types: Expand `domain/event/`

**Decision:** Move all 55 broadcast event constants (44 from `events.go` + 11 from `agui_events.go`) and 49 payload/helper structs (38 from `events.go` + 11 from `agui_events.go`) from `internal/adapter/ws/` into `internal/domain/event/`.

**Rationale:**
- `domain/event/` already contains 19 event type constants and the `AgentEvent` struct
- Event names are business vocabulary ("agent started", "run completed"), not infrastructure
- A separate `domain/broadcast/` package creates artificial separation of cohesive concepts
- Putting types in `port/broadcast/` violates the project convention that ports define interfaces, not domain types

**Cross-domain imports:** Some payload structs reference types from other domain packages (e.g., `LSPDiagnosticEvent` uses `lsp.Diagnostic`, `ChannelMessageEvent` uses `channel.Message`). This is acceptable — domain packages importing other domain packages is normal in DDD (aggregates reference value objects from other contexts). The `domain/event/` package already imports `encoding/json` and `time`; adding `domain/lsp` and `domain/channel` imports for payload types is a controlled, intentional coupling between cohesive domain concepts.

**File layout:**
```
internal/domain/event/
  event.go               # existing — AgentEvent struct, event sourcing types
  broadcast.go           # NEW — 44 broadcast event type constants (EventAgentStatus, etc.)
  broadcast_payloads.go  # NEW — 38 payload/helper structs (AgentStatusEvent, etc.)
  agui.go                # NEW — 11 AG-UI event constants + 11 AG-UI payload structs
  event_test.go          # existing
  replay.go              # existing
```

**Migration mechanics:**
1. Create new files in `domain/event/` with moved constants and structs
2. Add `domain/event` import to service files that need event constants/structs
3. For files that use BOTH event types AND `ws.Hub`/`ws.Message`: keep `adapter/ws` import alongside `domain/event`; only remove `adapter/ws` once all symbol references are migrated
4. Update symbol references: `ws.EventAgentStatus` -> `event.EventAgentStatus`
5. Keep `adapter/ws/hub.go` and `adapter/ws/conn.go` untouched (WebSocket infrastructure)
6. Run `goimports -w ./internal/service/` to clean up unused imports
7. Run `go build ./...` to verify compilation
8. In `adapter/ws/events.go`: remove moved constants/structs (they now live in `domain/event/`). Keep only hub-specific helpers like `BroadcastEvent` method on `Hub`

### 2.2 OTEL: Use API Directly, No Custom Port

**Decision:** Replace `internal/adapter/otel` imports in services with direct `go.opentelemetry.io/otel` API imports.

**Rationale:**
- The OTEL Go API is already designed as a port interface (stable, zero external deps, no-op defaults)
- Creating a custom port interface would duplicate what OTEL API already provides
- This is the pattern recommended by OTEL's official instrumentation guide

**Changes:**
- Move `StartRunSpan()`, `StartToolCallSpan()`, `StartDeliverySpan()` helpers to a thin package (e.g., `internal/telemetry/spans.go`) that imports only `go.opentelemetry.io/otel` API
- Move `Metrics` struct to `internal/telemetry/metrics.go`
- Services import `internal/telemetry` instead of `internal/adapter/otel`
- `internal/adapter/otel/` keeps only SDK configuration (provider setup, exporter config) — called from `cmd/codeforge/main.go`

**Why `internal/telemetry/` and not keep in `adapter/otel/`:**
- The span/metric helpers use only the OTEL API (interfaces), not the SDK (implementation)
- They are application-level helpers, not infrastructure adapters
- Separating them makes it clear: `adapter/otel/` = SDK setup, `telemetry/` = API usage

### 2.3 LSP: Create `port/codeintel/` Interface

**Decision:** Create a slim `CodeIntelligenceProvider` port interface. The LSP adapter implements it.

**Rationale:**
- LSP is currently unused (Phase 15D) but wired into routes
- A port interface decouples the service from the LSP protocol
- No-op adapter satisfies the interface when LSP is unavailable

**Interface:**
```go
// internal/port/codeintel/provider.go
package codeintel

import "context"

type Provider interface {
    Initialize(ctx context.Context, projectID, workspacePath string) error
    Shutdown(ctx context.Context, projectID string) error
    Diagnostics(ctx context.Context, projectID, filePath string) ([]Diagnostic, error)
}

type Diagnostic struct {
    File     string
    Line     int
    Severity string
    Message  string
}
```

**Changes:**
- `LSPService.hub *ws.Hub` -> `LSPService.broadcaster broadcast.Broadcaster`
- `LSPService.clients map[...]]*lspAdapter.Client` -> use `codeintel.Provider` interface
- Create `internal/adapter/lsp/provider.go` implementing `codeintel.Provider`
- Create `internal/adapter/lsp/noop.go` as fallback

### 2.4 Handler Struct: Concrete Adapters -> Port Interfaces

**Decision:** Replace concrete adapter types in `Handlers` struct with existing port interfaces.

**Changes:**
- `Handlers.LiteLLM *litellm.Client` -> `Handlers.LLM llm.Provider` — the existing `port/llm/provider.go` already defines `Provider` (with `ChatCompletion`, `ListModels`, `Health`), `ModelDiscoverer`, and `ModelAdmin` interfaces that cover all 7 methods handlers call. Handler methods that need discovery/admin capabilities accept those interfaces as needed.
- `Handlers.Copilot *copilot.Client` -> `Handlers.TokenExchanger tokenexchange.Exchanger` (new interface in `port/tokenexchange/`) — named by capability, not vendor, consistent with other ports (`port/broadcast/`, `port/llm/`, `port/database/`). Interface: `ExchangeToken(ctx, code string) (*Token, error)`

### 2.5 Silenced Errors: `logBestEffort` Helper

**Decision:** Create a `logBestEffort` helper function and replace all 38 `_ = s.store.*` occurrences.

**Location:** `internal/service/helpers.go` (alongside existing service-level helpers)

**Signature:**
```go
func logBestEffort(ctx context.Context, err error, op string, attrs ...slog.Attr) {
    if err != nil {
        attrs = append(attrs, slog.String("operation", op), slog.Any("error", err))
        slog.LogAttrs(ctx, slog.LevelWarn, "best-effort operation failed", attrs...)
    }
}
```

Accepts `ctx` to preserve trace IDs, request IDs, and tenant context for log correlation.

**Usage:**
```go
// Before:
_ = s.store.UpdateAgentStatus(ctx, id, agent.StatusIdle)
// After:
logBestEffort(ctx, s.store.UpdateAgentStatus(ctx, id, agent.StatusIdle),
    "UpdateAgentStatus", slog.String("agent_id", id))
```

**Rationale:**
- Explicit at every call site (no hidden behavior)
- Structured logging with operation name and entity ID
- Does not change control flow (non-blocking, no error propagation)
- Follows project principle: "Errors should never pass silently"

### 2.6 Test Coverage: Table-Driven Unit Tests

**Decision:** Add unit tests for the three critical untested modules using existing patterns.

#### Go Tests

**Files to create/extend:**
- `internal/service/runtime_execution_test.go` — NEW, core handler tests
- `internal/service/runtime_lifecycle_test.go` — NEW, lifecycle function tests

**Note:** `runtime_execution_extended_test.go` already covers `HandleToolCallResult` budget alert thresholds with table-driven tests. The new `runtime_execution_test.go` must NOT duplicate those tests — it covers the remaining 4 functions and the non-budget paths of `HandleToolCallResult`.

**Test approach:**
- Reuse `runtimeMockStore` from `runtime_test.go` (2494 LOC mock store)
- Table-driven tests with subtests for each function
- Test categories per function: happy path, error paths, edge cases

**Coverage targets:**

| Function | Lines | Test Cases | Notes |
|----------|-------|------------|-------|
| `HandleToolCallRequest` | 155 | Policy allow, policy deny, HITL approval, HITL timeout, termination check, cancelled run | |
| `handleConversationToolCall` | 113 | Auto-approve, bypass-all, policy deny, missing project | |
| `HandleToolCallResult` | 130 | Normal accumulation, stall detection, cost/token update failure logging | Budget threshold tests already in `_extended_test.go` — do NOT duplicate |
| `HandleRunComplete` | 127 | No gates (direct finalize), artifact validation, quality gate trigger, already completed | |
| `HandleQualityGateResult` | 85 | Gate pass -> delivery, gate fail -> rollback, gate fail no checkpoint | |
| `cleanupRunState` | 26 | Cleanup all resources, partial resources (nil services) | |
| `cancelRunWithReason` | 56 | Timeout cancel, already completed run, publish failure | |
| `finalizeRun` | 130 | Success finalization, failure finalization, callback invocation, sandbox cleanup | |
| `checkTermination` | 34 | MaxSteps, MaxCost, Timeout, HeartbeatTimeout, no termination | |

**Mocking strategy:**
- `runtimeMockStore` for database (existing)
- `mockBroadcaster` for event broadcasts (simple: record calls)
- `mockPublisher` for NATS publish (record subject + payload)
- `mockPolicyService` for policy evaluation (return configurable decisions)

#### Python Tests

**File to create:**
- `workers/tests/consumer/test_conversation_handler.py`

**Test approach:**
- Use `pytest-asyncio` with `AsyncMock` (existing pattern from `test_consumer.py`)
- Test `ConversationHandlerMixin` methods directly
- Mock NATS JetStream, LiteLLM client, store

**Coverage targets:**

| Method | Test Cases |
|--------|------------|
| `_handle_conversation_run` | Valid message -> execute, invalid JSON -> nack, duplicate run ID -> skip |
| `_execute_conversation_run` | Normal loop, max iterations, budget exceeded, cancelled mid-loop |
| `_build_system_prompt` | With skills, without skills, with mode, default mode |
| `_publish_completion` | Success result, error result, publish failure |

---

## 3. Scope Exclusions

The following audit findings are NOT addressed in this spec:

- F-001 (API keys) — already handled by user
- F-004 through F-005 (TLS, NATS auth) — infrastructure changes, separate spec
- F-006, F-007 (Store/Handlers god objects) — larger decomposition effort, separate spec
- F-009 (Safety fail-open) — security fix, can be a standalone 1-line change
- F-011, F-012 (RuntimeService god object) — requires architectural redesign, separate spec
- F-015, F-016 (GDPR) — compliance work, separate spec
- All MEDIUM/LOW/INFO findings — tracked in audit report for future work

---

## 4. Risk Assessment

| Risk | Mitigation |
|------|------------|
| Import rename breaks compilation | `go build ./...` after every mechanical change; single atomic commit |
| Event type name collisions in `domain/event/` | Existing types use `Type` prefix (e.g., `TypeRunStarted`), broadcast uses `Event` prefix (e.g., `EventRunStatus`) — no collision |
| OTEL API changes break services | OTEL API has backward compatibility guarantees; major versions are rare |
| Mock store diverges from real store | Use same `runtimeMockStore` as existing tests; update together |
| Python test fixtures become stale | Keep fixtures minimal; test handler logic, not NATS transport |

---

## 5. Success Criteria

- [ ] Zero service files import `internal/adapter/ws`, `internal/adapter/otel`, or `internal/adapter/lsp`
- [ ] `go build ./...` passes with zero errors
- [ ] All existing tests pass (`go test ./...`, `pytest`)
- [ ] Zero `_ = s.store.*` occurrences in `internal/service/`
- [ ] `runtime_execution_test.go` covers all 5 exported functions
- [ ] `runtime_lifecycle_test.go` covers `finalizeRun`, `cancelRunWithReason`, `checkTermination`
- [ ] `test_conversation_handler.py` covers core handler methods
- [ ] `golangci-lint run ./...` passes
- [ ] `ruff check workers/` passes
