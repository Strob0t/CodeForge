# Audit Remediation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix hexagonal architecture violations (F-002, F-034, F-033) and code quality gaps (F-008, F-010) identified in the universal audit.

**Architecture:** Move 55 event constants + 49 payload structs from `adapter/ws/` to `domain/event/`. Move OTEL span helpers to `internal/telemetry/`. Replace 38 silenced DB errors with logged warnings. Add unit tests for critical untested execution paths.

**Tech Stack:** Go 1.25, Python 3.12 (pytest-asyncio), SolidJS (no changes)

**Spec:** `docs/specs/2026-03-23-audit-remediation-design.md`

---

## File Structure

### New Files

| File | Responsibility |
|------|---------------|
| `internal/domain/event/broadcast.go` | 44 broadcast event type constants (moved from `adapter/ws/events.go`) |
| `internal/domain/event/broadcast_payloads.go` | 38 broadcast payload structs (moved from `adapter/ws/events.go`) |
| `internal/domain/event/agui.go` | 11 AG-UI event constants + 11 AG-UI payload structs (moved from `adapter/ws/agui_events.go`) |
| `internal/telemetry/spans.go` | OTEL span helpers (moved from `adapter/otel/spans.go`) |
| `internal/port/codeintel/provider.go` | `CodeIntelligenceProvider` port interface |
| `internal/port/tokenexchange/exchanger.go` | `Exchanger` port interface for Copilot token exchange |
| `internal/adapter/lsp/noop.go` | No-op `codeintel.Provider` implementation |
| `internal/service/log_best_effort.go` | `logBestEffort` helper function |
| `internal/service/log_best_effort_test.go` | Tests for `logBestEffort` |
| `internal/service/runtime_execution_test.go` | Unit tests for execution handlers |
| `internal/service/runtime_lifecycle_test.go` | Unit tests for lifecycle functions |
| `workers/tests/consumer/test_conversation_handler.py` | Unit tests for Python conversation handler |

### Modified Files

| File | Change |
|------|--------|
| `internal/adapter/ws/events.go` | Remove moved constants/structs, keep `BroadcastEvent` method |
| `internal/adapter/ws/agui_events.go` | Remove (all content moved to `domain/event/agui.go`) |
| 31 files in `internal/service/` | Replace `ws.Event*` → `event.Event*`, `ws.*Event{}` → `event.*Event{}` |
| 4 files in `internal/service/` | Replace `cfotel.*` → `telemetry.*` (spans) or `port/metrics.Recorder` (metrics) |
| `internal/service/lsp.go` | Replace `*ws.Hub` → `broadcast.Broadcaster`, `*lspAdapter.Client` → `codeintel.Provider` |
| `internal/adapter/http/handlers.go` | Replace `*litellm.Client` → `llm.Provider`, `*copilot.Client` → `tokenexchange.Exchanger` |
| 38 call sites in `internal/service/` | Replace `_ = s.store.*` → `logBestEffort(ctx, s.store.*(...), ...)` |

---

## Task 1: Create `logBestEffort` helper + tests

**Files:**
- Create: `internal/service/log_best_effort.go`
- Create: `internal/service/log_best_effort_test.go`

- [ ] **Step 1: Write the test file**

```go
// internal/service/log_best_effort_test.go
package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
)

func TestLogBestEffort_NilError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	old := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(old)

	logBestEffort(context.Background(), nil, "UpdateAgentStatus")

	if buf.Len() != 0 {
		t.Errorf("expected no log output for nil error, got: %s", buf.String())
	}
}

func TestLogBestEffort_NonNilError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	old := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(old)

	logBestEffort(context.Background(), errors.New("connection refused"), "UpdateAgentStatus",
		slog.String("agent_id", "a1"))

	out := buf.String()
	if out == "" {
		t.Fatal("expected log output for non-nil error")
	}
	for _, want := range []string{"best-effort", "UpdateAgentStatus", "connection refused", "agent_id", "a1"} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("expected log to contain %q, got: %s", want, out)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /workspaces/CodeForge && go test ./internal/service/ -run TestLogBestEffort -v
```
Expected: FAIL — `logBestEffort` undefined

- [ ] **Step 3: Write the implementation**

```go
// internal/service/log_best_effort.go
package service

import (
	"context"
	"log/slog"
)

// logBestEffort logs a warning if err is non-nil. Use for best-effort
// operations (state updates, metric recording) that should not block
// the main flow but must never be silently discarded.
func logBestEffort(ctx context.Context, err error, op string, attrs ...slog.Attr) {
	if err != nil {
		attrs = append(attrs, slog.String("operation", op), slog.Any("error", err))
		slog.LogAttrs(ctx, slog.LevelWarn, "best-effort operation failed", attrs...)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /workspaces/CodeForge && go test ./internal/service/ -run TestLogBestEffort -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/log_best_effort.go internal/service/log_best_effort_test.go
git commit -m "feat(service): add logBestEffort helper for non-fatal error logging"
```

---

## Task 2: Replace all 38 silenced errors with `logBestEffort`

**Files:**
- Modify: All files in `internal/service/` containing `_ = s.store.`

- [ ] **Step 1: Replace all occurrences mechanically**

Use targeted sed replacements across service files. For each file, replace patterns like:
```go
_ = s.store.UpdateAgentStatus(ctx, id, agent.StatusIdle)
```
with:
```go
logBestEffort(ctx, s.store.UpdateAgentStatus(ctx, id, agent.StatusIdle), "UpdateAgentStatus")
```

Process files one by one: `runtime.go`, `runtime_execution.go`, `runtime_lifecycle.go`, `agent.go`, `a2a.go`, `benchmark.go`, `orchestrator.go`, `orchestrator_consensus.go`, `review.go`, `session.go`, `auth.go`, `autoagent.go`.

For calls with context attributes, add identifying `slog.String` attrs:
```go
logBestEffort(ctx, s.store.UpdateAgentStatus(ctx, req.AgentID, agent.StatusRunning),
    "UpdateAgentStatus", slog.String("agent_id", req.AgentID))
```

- [ ] **Step 2: Remove `//nolint:errcheck` comments that guarded silenced errors**

- [ ] **Step 3: Verify no remaining silenced store calls**

```bash
cd /workspaces/CodeForge && grep -rn '_ = s\.store\.' internal/service/ | wc -l
```
Expected: `0`

- [ ] **Step 4: Verify compilation**

```bash
cd /workspaces/CodeForge && go build ./...
```
Expected: no errors

- [ ] **Step 5: Run existing tests**

```bash
cd /workspaces/CodeForge && go test ./internal/service/ -count=1
```
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add internal/service/
git commit -m "fix(service): replace 38 silenced store errors with logBestEffort warnings

Addresses audit finding F-008: errors should never pass silently."
```

---

## Task 3: Move broadcast event constants to `domain/event/broadcast.go`

**Files:**
- Create: `internal/domain/event/broadcast.go`
- Modify: `internal/adapter/ws/events.go` (remove constants only, keep structs temporarily)

- [ ] **Step 1: Create `broadcast.go` with all 44 event type constants**

Copy the `const (...)` block (lines 13-112) from `adapter/ws/events.go` into a new file `internal/domain/event/broadcast.go`. Change `package ws` to `package event`. No other changes needed — these are plain string constants with zero imports.

- [ ] **Step 2: Verify compilation**

```bash
cd /workspaces/CodeForge && go build ./internal/domain/event/
```

- [ ] **Step 3: Commit**

```bash
git add internal/domain/event/broadcast.go
git commit -m "refactor(domain): move 44 broadcast event constants to domain/event"
```

---

## Task 4: Move broadcast payload structs to `domain/event/broadcast_payloads.go`

**Files:**
- Create: `internal/domain/event/broadcast_payloads.go`
- Modify: `internal/adapter/ws/events.go` (remove structs, keep `BroadcastEvent` method)

- [ ] **Step 1: Create `broadcast_payloads.go` with all 38 payload structs**

Copy all struct type definitions (lines 114-473) from `adapter/ws/events.go`. Change `package ws` to `package event`. Update imports: keep `domain/channel` and `domain/lsp` imports that are already present in `events.go`.

- [ ] **Step 2: Verify compilation**

```bash
cd /workspaces/CodeForge && go build ./internal/domain/event/
```

- [ ] **Step 3: Commit**

```bash
git add internal/domain/event/broadcast_payloads.go
git commit -m "refactor(domain): move 38 broadcast payload structs to domain/event"
```

---

## Task 5: Move AG-UI events to `domain/event/agui.go`

**Files:**
- Create: `internal/domain/event/agui.go`

- [ ] **Step 1: Create `agui.go` with all 11 AG-UI constants + 11 structs**

Copy the entire content of `adapter/ws/agui_events.go` (120 lines). Change `package ws` to `package event`. Keep the `encoding/json` import (used by `AGUIToolResultEvent.Diff json.RawMessage`).

- [ ] **Step 2: Verify compilation**

```bash
cd /workspaces/CodeForge && go build ./internal/domain/event/
```

- [ ] **Step 3: Commit**

```bash
git add internal/domain/event/agui.go
git commit -m "refactor(domain): move 11 AG-UI event types + 11 structs to domain/event"
```

---

## Task 6: Update all 31 service files — replace `adapter/ws` imports

**Files:**
- Modify: All 31 service files listed in the spec that import `adapter/ws`

- [ ] **Step 1: Mechanically update imports and symbol references**

For each service file:
1. Add import: `"github.com/Strob0t/CodeForge/internal/domain/event"`
2. Replace all `ws.Event*` constant references → `event.Event*`
3. Replace all `ws.*Event{` struct literals → `event.*Event{`
4. Replace all `ws.*Event` type references → `event.*Event`
5. If the file no longer uses any `ws.` symbols, remove the `adapter/ws` import
6. If the file still uses `ws.Hub` (only `lsp.go`), keep both imports

Key pattern: `ws.EventAgentStatus` → `event.EventAgentStatus`, `ws.AgentStatusEvent{` → `event.AgentStatusEvent{`

- [ ] **Step 2: Run `goimports` to clean up**

```bash
cd /workspaces/CodeForge && goimports -w ./internal/service/
```

- [ ] **Step 3: Verify no service files import `adapter/ws` (except lsp.go temporarily)**

```bash
cd /workspaces/CodeForge && grep -rn 'adapter/ws' internal/service/ | grep -v '_test.go'
```
Expected: only `lsp.go` (which still uses `ws.Hub` — fixed in Task 9)

- [ ] **Step 4: Verify compilation**

```bash
cd /workspaces/CodeForge && go build ./...
```

- [ ] **Step 5: Run all tests**

```bash
cd /workspaces/CodeForge && go test ./internal/service/ -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/service/
git commit -m "refactor(service): replace adapter/ws imports with domain/event

31 service files now import event types from the domain layer
instead of the WebSocket adapter. Addresses audit finding F-002."
```

---

## Task 7: Clean up `adapter/ws/events.go` and delete `agui_events.go`

**Files:**
- Modify: `internal/adapter/ws/events.go` — remove moved constants and structs
- Delete: `internal/adapter/ws/agui_events.go`

- [ ] **Step 1: Update `events.go`**

Remove the constants block (lines 13-112) and all struct definitions (lines 114-473). Keep only:
- The package declaration and necessary imports
- The `BroadcastEvent` method on `*Hub` (lines 475-487) — update it to import event types from `domain/event`

The resulting file should be ~20 lines: package, import, and the `BroadcastEvent` convenience method.

- [ ] **Step 2: Delete `agui_events.go`**

```bash
rm internal/adapter/ws/agui_events.go
```

- [ ] **Step 3: Update any remaining `adapter/ws` imports in non-service code**

Check if `adapter/http/handlers*.go`, `adapter/ws/hub.go`, or tests still reference the old types. Update them to import from `domain/event`.

```bash
cd /workspaces/CodeForge && grep -rn 'ws\.Event\|ws\.AGUI\|ws\.Task\|ws\.Run\|ws\.Agent' internal/adapter/ --include='*.go' | grep -v '_test.go'
```

Fix any remaining references.

- [ ] **Step 4: Verify compilation**

```bash
cd /workspaces/CodeForge && go build ./...
```

- [ ] **Step 5: Run all tests**

```bash
cd /workspaces/CodeForge && go test ./... -count=1 2>&1 | tail -20
```

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/ws/
git commit -m "refactor(adapter/ws): remove moved event types, keep only Hub infrastructure"
```

---

## Task 8: Move OTEL span helpers to `internal/telemetry/`

**Files:**
- Create: `internal/telemetry/spans.go`
- Modify: 4 service files that import `adapter/otel`
- Modify: Service files that hold `*cfotel.Metrics` — change to `metrics.Recorder`

- [ ] **Step 1: Create `internal/telemetry/spans.go`**

Copy `adapter/otel/spans.go` content. Change `package otel` to `package telemetry`. The imports (`go.opentelemetry.io/otel`, `otel/attribute`, `otel/trace`) stay the same — these are the OTEL API, not the SDK.

- [ ] **Step 2: Update service imports**

In `conversation.go`, `runtime.go`, `runtime_execution.go`, `runtime_lifecycle.go`:
- Replace `cfotel "github.com/Strob0t/CodeForge/internal/adapter/otel"` with:
  - `"github.com/Strob0t/CodeForge/internal/telemetry"` (for span helpers)
  - `cfmetrics "github.com/Strob0t/CodeForge/internal/port/metrics"` (for `Recorder` interface)
- Replace `cfotel.StartRunSpan(` → `telemetry.StartRunSpan(`
- Replace `cfotel.StartToolCallSpan(` → `telemetry.StartToolCallSpan(`
- Replace `cfotel.StartDeliverySpan(` → `telemetry.StartDeliverySpan(`
- Replace `*cfotel.Metrics` field type → `cfmetrics.Recorder`
- Replace `SetMetrics(m *cfotel.Metrics)` → `SetMetrics(m cfmetrics.Recorder)`

- [ ] **Step 3: Delete span helpers from adapter/otel/spans.go**

The `adapter/otel/spans.go` file can be deleted — its content now lives in `telemetry/spans.go`. The `adapter/otel/` package keeps `setup.go` (SDK config), `metrics.go` (Recorder implementation), and `middleware.go`.

- [ ] **Step 4: Verify no service imports `adapter/otel`**

```bash
cd /workspaces/CodeForge && grep -rn 'adapter/otel' internal/service/
```
Expected: 0 results

- [ ] **Step 5: Verify compilation + tests**

```bash
cd /workspaces/CodeForge && go build ./... && go test ./internal/service/ -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/telemetry/ internal/service/ internal/adapter/otel/
git commit -m "refactor: move OTEL span helpers to internal/telemetry, use port/metrics.Recorder

Services now depend on the OTEL API (via telemetry/) and the
metrics port interface, not the adapter. Addresses audit F-002."
```

---

## Task 9: Create `port/codeintel/` interface + fix LSP service

**Files:**
- Create: `internal/port/codeintel/provider.go`
- Create: `internal/adapter/lsp/noop.go`
- Modify: `internal/service/lsp.go`

- [ ] **Step 1: Create the port interface**

```go
// internal/port/codeintel/provider.go
package codeintel

import "context"

// Provider abstracts language intelligence capabilities (go-to-definition,
// diagnostics, etc.). Implemented by the LSP adapter; a no-op fallback
// is used when LSP is not available.
type Provider interface {
	Initialize(ctx context.Context, projectID, workspacePath string) error
	Shutdown(ctx context.Context, projectID string) error
	Diagnostics(ctx context.Context, projectID, filePath string) ([]Diagnostic, error)
}

// Diagnostic represents a code issue reported by a language server.
type Diagnostic struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}
```

- [ ] **Step 2: Create no-op adapter**

```go
// internal/adapter/lsp/noop.go
package lsp

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/port/codeintel"
)

// NoOpProvider satisfies codeintel.Provider when LSP is unavailable.
type NoOpProvider struct{}

func (NoOpProvider) Initialize(context.Context, string, string) error             { return nil }
func (NoOpProvider) Shutdown(context.Context, string) error                       { return nil }
func (NoOpProvider) Diagnostics(context.Context, string, string) ([]codeintel.Diagnostic, error) {
	return nil, nil
}
```

- [ ] **Step 3: Update `lsp.go`**

- Replace `hub *ws.Hub` field → `broadcaster broadcast.Broadcaster`
- Replace `*lspAdapter.Client` usage → `codeintel.Provider` interface
- Update constructor to accept `broadcast.Broadcaster` instead of `*ws.Hub`
- Remove `adapter/ws` and `adapter/lsp` imports

- [ ] **Step 4: Verify compilation + tests**

```bash
cd /workspaces/CodeForge && go build ./... && go test ./internal/service/ -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/port/codeintel/ internal/adapter/lsp/noop.go internal/service/lsp.go
git commit -m "refactor: create codeintel port interface, decouple LSP service from adapters"
```

---

## Task 10: Replace concrete adapter types in Handlers struct

**Files:**
- Create: `internal/port/tokenexchange/exchanger.go`
- Modify: `internal/adapter/http/handlers.go`
- Modify: Handler files that use `h.LiteLLM` or `h.Copilot`

- [ ] **Step 1: Create `port/tokenexchange/exchanger.go`**

```go
// internal/port/tokenexchange/exchanger.go
package tokenexchange

import "context"

// Token represents an exchanged access token.
type Token struct {
	AccessToken string
	ExpiresIn   int
}

// Exchanger abstracts token exchange operations (e.g., GitHub Copilot).
type Exchanger interface {
	ExchangeToken(ctx context.Context, code string) (*Token, error)
}
```

- [ ] **Step 2: Update `handlers.go` struct fields**

- Replace `LiteLLM *litellm.Client` → `LLM llm.Provider` (using existing `port/llm` interfaces)
- Replace `Copilot *copilot.Client` → `TokenExchanger tokenexchange.Exchanger`
- Update the import block: remove `adapter/litellm` and `adapter/copilot`, add `port/llm` and `port/tokenexchange`

- [ ] **Step 3: Update all handler call sites**

- Replace `h.LiteLLM.` → `h.LLM.` in all handler files
- Replace `h.Copilot.` → `h.TokenExchanger.` in handler files
- Update `cmd/codeforge/main.go` where the Handlers struct is constructed

- [ ] **Step 4: Verify compilation + tests**

```bash
cd /workspaces/CodeForge && go build ./... && go test ./internal/adapter/http/ -count=1
```

- [ ] **Step 5: Verify zero adapter imports in service and handler struct**

```bash
cd /workspaces/CodeForge && grep -rn 'adapter/litellm\|adapter/copilot' internal/adapter/http/handlers.go
```
Expected: 0 results

- [ ] **Step 6: Commit**

```bash
git add internal/port/tokenexchange/ internal/adapter/http/ cmd/codeforge/
git commit -m "refactor(handlers): replace concrete adapter types with port interfaces

Handlers.LiteLLM -> Handlers.LLM (llm.Provider)
Handlers.Copilot -> Handlers.TokenExchanger (tokenexchange.Exchanger)
Addresses audit finding F-033."
```

---

## Task 11: Final verification — zero adapter imports in service layer

**Files:** None (verification only)

- [ ] **Step 1: Verify zero forbidden adapter imports in service layer**

```bash
cd /workspaces/CodeForge && grep -rn 'adapter/ws\|adapter/otel\|adapter/lsp\|adapter/litellm\|adapter/copilot' internal/service/ | grep -v '_test.go'
```
Expected: 0 results

- [ ] **Step 2: Full build**

```bash
cd /workspaces/CodeForge && go build ./...
```

- [ ] **Step 3: Full test suite**

```bash
cd /workspaces/CodeForge && go test ./... -count=1 2>&1 | tail -5
```

- [ ] **Step 4: Lint**

```bash
cd /workspaces/CodeForge && golangci-lint run ./...
```

- [ ] **Step 5: Commit any lint fixes**

If lint reports issues from the refactor, fix and commit.

---

## Task 12: Write `runtime_execution_test.go` — `HandleToolCallRequest` tests

**Files:**
- Create: `internal/service/runtime_execution_test.go`

- [ ] **Step 1: Write test scaffolding with mock types**

Create the test file with mock broadcaster, mock publisher, and mock policy service. Reuse `runtimeMockStore` from `runtime_test.go` (same package `service_test` or `service`).

Mock types needed:
```go
type mockBroadcaster struct {
	mu     sync.Mutex
	events []broadcastCall
}
type broadcastCall struct {
	eventType string
	payload   any
}

type mockPublisher struct {
	mu       sync.Mutex
	messages []publishCall
}
type publishCall struct {
	subject string
	data    []byte
}
```

- [ ] **Step 2: Write table-driven tests for `HandleToolCallRequest`**

Test cases:
1. Policy allows → tool call response "allow" published
2. Policy denies → tool call response "deny" published
3. Cancelled run → returns early, no publish
4. Termination check triggers → run completed, response "deny"

- [ ] **Step 3: Run tests to verify they pass**

```bash
cd /workspaces/CodeForge && go test ./internal/service/ -run TestHandleToolCallRequest -v
```

- [ ] **Step 4: Write tests for `HandleRunComplete`**

Test cases:
1. No quality gate → direct finalize
2. Artifact validation required → gate triggered
3. Already completed run → no-op

- [ ] **Step 5: Write tests for `HandleQualityGateResult`**

Test cases:
1. Gate passed → delivery triggered
2. Gate failed with checkpoint → rollback
3. Gate failed without checkpoint → finalize as failed

- [ ] **Step 6: Run all execution tests**

```bash
cd /workspaces/CodeForge && go test ./internal/service/ -run 'TestHandle' -v
```

- [ ] **Step 7: Commit**

```bash
git add internal/service/runtime_execution_test.go
git commit -m "test(service): add unit tests for runtime execution handlers

Covers HandleToolCallRequest, HandleRunComplete, HandleQualityGateResult.
Addresses audit finding F-010."
```

---

## Task 13: Write `runtime_lifecycle_test.go`

**Files:**
- Create: `internal/service/runtime_lifecycle_test.go`

- [ ] **Step 1: Write table-driven tests for `checkTermination`**

Test cases:
1. MaxSteps reached → returns "max steps" reason
2. MaxCost exceeded → returns "budget" reason
3. Timeout exceeded → returns "timeout" reason
4. HeartbeatTimeout → returns "heartbeat" reason
5. No termination condition → returns ""
6. Zero limits (disabled) → returns ""

- [ ] **Step 2: Write tests for `cancelRunWithReason`**

Test cases:
1. Timeout cancel → run marked timeout, cancel published, events broadcast
2. Already completed run → returns error
3. NATS publish failure → error propagated

- [ ] **Step 3: Write tests for `finalizeRun`**

Test cases:
1. Success finalization → run completed, agent idle, callback invoked
2. Failure finalization → run failed, agent idle, no delivery
3. Sandbox cleanup called when sandbox service available
4. Checkpoint cleanup called when checkpoint service available

- [ ] **Step 4: Run all lifecycle tests**

```bash
cd /workspaces/CodeForge && go test ./internal/service/ -run 'TestCheckTermination\|TestCancelRun\|TestFinalizeRun' -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/service/runtime_lifecycle_test.go
git commit -m "test(service): add unit tests for runtime lifecycle functions

Covers checkTermination, cancelRunWithReason, finalizeRun.
Addresses audit finding F-010."
```

---

## Task 14: Write Python conversation handler tests

**Files:**
- Create: `workers/tests/consumer/test_conversation_handler.py`

- [ ] **Step 1: Create test file with fixtures**

```python
# workers/tests/consumer/test_conversation_handler.py
import json
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.consumer._conversation import ConversationHandlerMixin


@pytest.fixture
def mock_js():
    js = AsyncMock()
    js.publish = AsyncMock()
    return js


@pytest.fixture
def mock_llm():
    llm = AsyncMock()
    return llm


@pytest.fixture
def handler(mock_js, mock_llm):
    """Create a minimal ConversationHandlerMixin instance for testing."""
    h = ConversationHandlerMixin.__new__(ConversationHandlerMixin)
    h._js = mock_js
    h._llm = mock_llm
    h._log = MagicMock()
    h._db_url = ""
    h._active_runs = set()
    return h
```

- [ ] **Step 2: Write test for `_handle_conversation_run` — valid message**

Test that a valid NATS message is decoded, the run ID is added to `_active_runs`, and execution is attempted.

- [ ] **Step 3: Write test for `_handle_conversation_run` — invalid JSON**

Test that an invalid JSON payload causes the message to be nack'd.

- [ ] **Step 4: Write test for `_handle_conversation_run` — duplicate run ID**

Test that a run ID already in `_active_runs` is skipped (dedup).

- [ ] **Step 5: Write test for `_publish_completion`**

Test that the completion message is published to the correct NATS subject with the correct payload.

- [ ] **Step 6: Run Python tests**

```bash
cd /workspaces/CodeForge && PYTHONPATH=workers .venv/bin/python -m pytest workers/tests/consumer/test_conversation_handler.py -v
```

- [ ] **Step 7: Lint**

```bash
cd /workspaces/CodeForge && .venv/bin/ruff check workers/tests/consumer/test_conversation_handler.py
```

- [ ] **Step 8: Commit**

```bash
git add workers/tests/consumer/test_conversation_handler.py
git commit -m "test(workers): add unit tests for conversation handler mixin

Covers _handle_conversation_run (valid/invalid/dedup) and _publish_completion.
Addresses audit finding F-010."
```

---

## Task 15: Final verification + documentation

**Files:**
- Modify: `docs/audits/2026-03-23-universal-audit-report.md` (mark findings as resolved)
- Modify: `docs/todo.md` (mark completed, add follow-up items)

- [ ] **Step 1: Run full Go test suite**

```bash
cd /workspaces/CodeForge && go test ./... -count=1
```

- [ ] **Step 2: Run full Python test suite**

```bash
cd /workspaces/CodeForge && PYTHONPATH=workers .venv/bin/python -m pytest workers/tests/ -x
```

- [ ] **Step 3: Run linters**

```bash
cd /workspaces/CodeForge && golangci-lint run ./... && .venv/bin/ruff check workers/
```

- [ ] **Step 4: Verify success criteria**

```bash
# Zero adapter imports in service layer
grep -rn 'adapter/ws\|adapter/otel\|adapter/lsp' internal/service/ | grep -v _test.go | wc -l
# Zero silenced store errors
grep -rn '_ = s\.store\.' internal/service/ | wc -l
```
Both must output `0`.

- [ ] **Step 5: Update audit report — mark findings as resolved**

In `docs/audits/2026-03-23-universal-audit-report.md`, add a "Resolved" annotation to findings F-002, F-008, F-010, F-033, F-034.

- [ ] **Step 6: Update `docs/todo.md`**

Mark audit remediation tasks as complete. Add follow-up items for remaining audit findings (F-004 through F-016).

- [ ] **Step 7: Final commit**

```bash
git add docs/
git commit -m "docs: mark audit findings F-002, F-008, F-010, F-033, F-034 as resolved"
```
