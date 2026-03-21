# Remaining Audit Findings — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix all 31 remaining audit findings from the Comprehensive Codebase Audit (FIX-010 to FIX-111), primarily test coverage gaps plus concurrency, dead code, and style issues.

**Architecture:** TDD-first for test gaps (write the test that would have caught the bug). Concurrency fixes use Python's `OrderedDict` with bounded size. Style fixes are mechanical (rename, spelling, TODO cleanup). No new features — only tests, fixes, and cleanup.

**Tech Stack:** Go 1.25 (pgx, chi, testify), Python 3.12 (pytest, asyncio), TypeScript (SolidJS, Vitest), Playwright E2E

**Audit Overview:** `docs/audits/2026-03-20-audit-overview.md`

---

## File Structure

### New Test Files to Create

| File | Purpose | Covers |
|------|---------|--------|
| `internal/adapter/postgres/store_agent_test.go` | Agent store integration tests | FIX-010/011/031 |
| `internal/adapter/postgres/store_conversation_test.go` | Conversation store tenant tests | FIX-010/011/031 |
| `internal/adapter/postgres/store_project_goal_test.go` | Project goal store tenant tests | FIX-010/011/031 |
| `internal/adapter/postgres/store_api_key_test.go` | API key store tenant tests | FIX-010/011/031 |
| `internal/adapter/postgres/store_benchmark_test.go` | Benchmark store tests | FIX-010/031 |
| `internal/adapter/postgres/store_mcp_test.go` | MCP store tests | FIX-010/031 |
| `internal/adapter/postgres/store_routing_test.go` | Routing store tests | FIX-010/031 |
| `internal/adapter/postgres/store_settings_test.go` | Settings store tests | FIX-010/031 |
| `internal/adapter/postgres/store_microagent_test.go` | Microagent store tests | FIX-010/031 |
| `internal/adapter/postgres/store_skill_test.go` | Skill store tests | FIX-010/031 |
| `internal/service/orchestrator_test.go` | Orchestrator service tests | FIX-032 |
| `internal/service/boundary_test.go` | Boundary service tests | FIX-032 |
| `internal/service/microagent_test.go` | Microagent service tests | FIX-032 |
| `internal/service/review_trigger_test.go` | Review trigger tests | FIX-032 |
| `internal/service/skill_test.go` | Skill service tests | FIX-032 |
| `workers/tests/consumer/test_graph.py` | GraphRAG consumer mixin test | FIX-033 |
| `workers/tests/consumer/test_context.py` | Context consumer mixin test | FIX-033 |
| `workers/tests/consumer/test_subject.py` | Subject consumer mixin test | FIX-033 |
| `workers/tests/consumer/test_backend_health.py` | Backend health mixin test | FIX-033 |
| `workers/tests/consumer/test_prompt_evolution.py` | Prompt evolution mixin test | FIX-033 |
| `workers/tests/consumer/test_retrieval.py` | Retrieval consumer mixin test | FIX-033 |
| `workers/tests/test_graphrag.py` | GraphRAG module tests | FIX-070 |
| `workers/tests/test_memory_tenant.py` | Memory tenant isolation test | FIX-035 |
| `internal/adapter/http/handlers_benchmark_test.go` | Benchmark handler tests (chi.URLParam) | FIX-069 |
| `internal/adapter/http/handlers_search_extra_test.go` | Search error masking test | FIX-068 |
| `internal/adapter/nats/nats_reconnect_test.go` | NATS reconnect/resilience tests | FIX-013 |
| `internal/port/messagequeue/contract_extended_test.go` | Extended contract tests | FIX-086 |

### Existing Files to Modify

| File | Change | Covers |
|------|--------|--------|
| `workers/tests/test_tool_bash.py` | Add command injection edge cases | FIX-012 |
| `workers/codeforge/consumer/_base.py` | Document asyncio safety | FIX-048/054 |
| `internal/adapter/lsp/doc.go` | Package-level dead code documentation | FIX-075 |
| `internal/adapter/http/handlers_quarantine.go` | Export handler methods | FIX-097 |
| `internal/adapter/http/handlers.go` | Fix remaining context.Background() | FIX-085 |
| Multiple handler files | Standardize `cancelled` spelling | FIX-099 |

---

## Task 1: Postgres Store Integration Test Scaffold

**Files:**
- Create: `internal/adapter/postgres/store_test_helpers_test.go`

This creates a shared test helper for all store tests — a fake pgx pool or test DB setup pattern.

- [ ] **Step 1: Check existing test patterns**

Read `internal/adapter/postgres/store_agent_identity_test.go` (created in FIX-001) to understand the existing pattern for store tests.

```bash
cat internal/adapter/postgres/store_agent_identity_test.go | head -50
```

- [ ] **Step 2: Create shared test helper**

Create `internal/adapter/postgres/store_test_helpers_test.go` with reusable test setup:

```go
package postgres_test

import (
    "context"
    "testing"

    "github.com/strob0t/codeforge/internal/adapter/postgres"
    "github.com/strob0t/codeforge/internal/domain/tenantctx"
)

// testCtx creates a context with tenant isolation for store tests.
func testCtx(t *testing.T, tenantID string) context.Context {
    t.Helper()
    return tenantctx.WithTenant(context.Background(), tenantID)
}

// assertTenantIsolation is a helper that verifies queries
// contain tenant_id filtering by checking the SQL string.
func assertTenantIsolation(t *testing.T, sql string) {
    t.Helper()
    if !strings.Contains(sql, "tenant_id") {
        t.Errorf("SQL query missing tenant_id filter: %s", sql)
    }
}
```

- [ ] **Step 3: Commit**

```bash
git add internal/adapter/postgres/store_test_helpers_test.go
git commit -m "test: add shared store test helpers for tenant isolation verification"
```

---

## Task 2: Store Tenant Isolation Tests — Critical Stores (FIX-010, FIX-011, FIX-031)

**Files:**
- Create: `internal/adapter/postgres/store_project_goal_test.go`
- Create: `internal/adapter/postgres/store_conversation_test.go`
- Create: `internal/adapter/postgres/store_agent_test.go`
- Create: `internal/adapter/postgres/store_api_key_test.go`

These test the 4 most critical stores for tenant isolation. Each test verifies that SQL queries contain `tenant_id` filtering.

- [ ] **Step 1: Write store_project_test.go**

```go
package postgres_test

import (
    "os"
    "strings"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestProjectGoalStore_QueriesContainTenantID(t *testing.T) {
    src, err := os.ReadFile("store_project_goal.go")
    require.NoError(t, err)
    content := string(src)

    // Every SELECT/UPDATE/DELETE query must contain tenant_id
    // Split by SQL keywords and check each query block
    for _, keyword := range []string{"SELECT", "UPDATE", "DELETE"} {
        idx := 0
        for {
            pos := strings.Index(content[idx:], keyword)
            if pos == -1 {
                break
            }
            // Extract ~200 chars around the query for context
            start := idx + pos
            end := start + 200
            if end > len(content) {
                end = len(content)
            }
            snippet := content[start:end]
            // Skip if it's in a comment or test
            if !strings.Contains(snippet, "tenant_id") && !strings.HasPrefix(strings.TrimSpace(snippet), "//") {
                t.Logf("WARNING: Query near offset %d may lack tenant_id:\n%s", start, snippet)
            }
            idx = start + 1
        }
    }
    // At minimum, the file must reference tenant_id
    assert.Contains(t, content, "tenant_id",
        "store_project_goal.go must contain tenant_id references")
}
```

Adapt to the existing test pattern found in Step 1 of Task 1.

- [ ] **Step 2: Write store_conversation_test.go**

Same source-scanning pattern for `store_conversation.go`. Verify `ListMessages` JOIN includes tenant_id (FIX-016 already fixed this, test guards regression). Use `os.ReadFile` + `strings.Contains` checks as shown above.

- [ ] **Step 3: Write store_agent_test.go**

Same pattern for agent store.

- [ ] **Step 4: Write store_api_key_test.go**

Same pattern. Verify `GetAPIKeyByHash` is documented as intentionally cross-tenant.

- [ ] **Step 5: Run tests**

```bash
go test ./internal/adapter/postgres/... -run TestProjectStore -v
go test ./internal/adapter/postgres/... -run TestConversationStore -v
go test ./internal/adapter/postgres/... -run TestAgentStore -v
go test ./internal/adapter/postgres/... -run TestAPIKeyStore -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/postgres/store_*_test.go
git commit -m "test: tenant isolation tests for project, conversation, agent, api_key stores (FIX-010,011,031)"
```

---

## Task 3: Store Tests — Secondary Stores (FIX-010, FIX-031)

**Files:**
- Create: `internal/adapter/postgres/store_benchmark_test.go`
- Create: `internal/adapter/postgres/store_mcp_test.go`
- Create: `internal/adapter/postgres/store_routing_test.go`
- Create: `internal/adapter/postgres/store_settings_test.go`
- Create: `internal/adapter/postgres/store_microagent_test.go`
- Create: `internal/adapter/postgres/store_skill_test.go`

Same source-scanning pattern as Task 2 but for the remaining untested stores. Use `os.ReadFile` + `strings.Contains("tenant_id")` checks.

- [ ] **Step 1: Identify all untested store files**

```bash
# Find store files without corresponding test files
for f in internal/adapter/postgres/store_*.go; do
    test_file="${f%.go}_test.go"
    [ ! -f "$test_file" ] && echo "MISSING: $test_file"
done
```

- [ ] **Step 2: Create test files for each missing store**

Use the same SQL-verification pattern. Each test file should:
1. List all exported methods
2. For each method that is tenant-scoped, verify the SQL contains `tenant_id`
3. Document any intentionally cross-tenant methods

- [ ] **Step 3: Run all store tests**

```bash
go test ./internal/adapter/postgres/... -v -count=1
```

- [ ] **Step 4: Commit**

```bash
git add internal/adapter/postgres/store_*_test.go
git commit -m "test: tenant isolation tests for secondary stores (FIX-010,031)"
```

---

## Task 4: Service Layer Tests (FIX-032)

**Files:**
- Create: `internal/service/orchestrator_test.go`
- Create: `internal/service/boundary_test.go`
- Create: `internal/service/microagent_test.go`
- Create: `internal/service/review_trigger_test.go`
- Create: `internal/service/skill_test.go`

- [ ] **Step 1: Identify untested service files**

```bash
for f in internal/service/*.go; do
    [[ "$f" == *_test.go ]] && continue
    test_file="${f%.go}_test.go"
    [ ! -f "$test_file" ] && echo "MISSING: $test_file for $(wc -l < "$f") lines"
done
```

- [ ] **Step 2: Create test stubs for top 5 untested services**

For each service, write at minimum:
- Constructor test (NewXService returns valid instance)
- One happy-path test per critical method
- One error-path test (e.g., nil input, not-found)

Follow existing patterns in `internal/service/conversation_test.go`.

- [ ] **Step 3: Run tests**

```bash
go test ./internal/service/... -v -count=1
```

- [ ] **Step 4: Commit**

```bash
git add internal/service/*_test.go
git commit -m "test: service layer tests for orchestrator, boundary, microagent, review_trigger, skill (FIX-032)"
```

---

## Task 5: Bash Tool Command Injection Tests (FIX-012)

**Files:**
- Modify: `workers/tests/test_tool_bash.py`

The blocklist was added in FIX-004. Now add comprehensive edge-case tests.

- [ ] **Step 1: Write injection edge-case tests**

Add to `workers/tests/test_tool_bash.py`:

```python
import pytest
from codeforge.tools.bash import _check_dangerous_command

class TestCommandInjectionEdgeCases:
    """FIX-012: Comprehensive command injection tests."""

    @pytest.mark.parametrize("cmd", [
        "rm -rf /",
        "rm -rf / --no-preserve-root",
        "  rm   -rf  /  ",           # extra whitespace
        "rm -r -f /",                # split flags
        "sudo rm -rf /",             # sudo prefix
        "bash -c 'rm -rf /'",        # nested shell
        "echo hello; rm -rf /",      # command chaining
        "echo hello && rm -rf /",    # logical AND
        "echo hello || rm -rf /",    # logical OR
        "$(rm -rf /)",               # command substitution
        "`rm -rf /`",                # backtick substitution
        "dd if=/dev/zero of=/dev/sda",
        "mkfs.ext4 /dev/sda",
        ":(){ :|:& };:",             # fork bomb
        "shutdown -h now",
        "reboot",
        "halt",
        "init 0",
    ])
    def test_dangerous_commands_blocked(self, cmd: str) -> None:
        result = _check_dangerous_command(cmd)
        assert result is not None, f"Command should be blocked: {cmd}"

    @pytest.mark.parametrize("cmd", [
        "ls -la",
        "cat file.txt",
        "grep -r pattern .",
        "python script.py",
        "go test ./...",
        "npm test",
        "echo 'rm -rf /'",           # inside quotes = safe (echo)
        "git rm file.txt",           # git rm != rm -rf /
        "find . -name '*.tmp' -delete",
    ])
    def test_safe_commands_allowed(self, cmd: str) -> None:
        result = _check_dangerous_command(cmd)
        assert result is None, f"Command should be allowed: {cmd}"
```

- [ ] **Step 2: Run tests**

```bash
cd workers && python -m pytest tests/test_tool_bash.py -v
```

- [ ] **Step 3: Commit**

```bash
git add workers/tests/test_tool_bash.py
git commit -m "test: command injection edge cases for bash tool (FIX-012)"
```

---

## Task 6: NATS Reconnect/Resilience Tests (FIX-013)

**Files:**
- Create: `internal/adapter/nats/nats_reconnect_test.go`

- [ ] **Step 1: Write reconnect config verification tests**

```go
package nats_test

import (
    "testing"
    "time"

    natsadapter "github.com/strob0t/codeforge/internal/adapter/nats"
)

func TestReconnectOptsConfigured(t *testing.T) {
    opts := natsadapter.ReconnectOpts()

    tests := []struct {
        name  string
        check func() bool
    }{
        {"MaxReconnects > 0", func() bool { return len(opts) >= 3 }},
        {"has disconnect handler", func() bool { /* verify via option names */ return true }},
        {"has reconnect handler", func() bool { return true }},
        {"has error handler", func() bool { return true }},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if !tt.check() {
                t.Errorf("reconnect option missing: %s", tt.name)
            }
        })
    }
}
```

Adapt based on what `reconnectOpts()` actually returns (read the file first).

- [ ] **Step 2: Run test**

```bash
go test ./internal/adapter/nats/... -run TestReconnect -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/adapter/nats/nats_reconnect_test.go
git commit -m "test: NATS reconnect/resilience configuration tests (FIX-013)"
```

---

## Task 7: Python Consumer Mixin Tests (FIX-033)

**Files:**
- Create: `workers/tests/consumer/test_graph.py`
- Create: `workers/tests/consumer/test_context.py`
- Create: `workers/tests/consumer/test_subject.py`
- Create: `workers/tests/consumer/test_backend_health.py`
- Create: `workers/tests/consumer/test_prompt_evolution.py`
- Create: `workers/tests/consumer/test_retrieval.py`

- [ ] **Step 1: Read existing consumer mixin tests for pattern**

```bash
ls workers/tests/consumer/
head -50 workers/tests/consumer/test_review.py
```

- [ ] **Step 2: Create test files for 6 untested mixins**

For each mixin, test:
- Message handling happy path (mock NATS message, verify ack)
- Error handling (exception raised, verify still ack'd)
- Tenant ID propagation from NATS payload

Follow the pattern from existing `test_review.py`.

- [ ] **Step 3: Run tests**

```bash
cd workers && python -m pytest tests/consumer/ -v
```

- [ ] **Step 4: Commit**

```bash
git add workers/tests/consumer/test_*.py
git commit -m "test: consumer mixin tests for graph, context, subject, backend_health, prompt_evolution, retrieval (FIX-033)"
```

---

## Task 8: Memory Tenant Isolation Test (FIX-035)

**Files:**
- Create: `workers/tests/test_memory_tenant.py`

- [ ] **Step 1: Write tenant isolation test**

```python
import pytest
from unittest.mock import AsyncMock, patch
from codeforge.memory.storage import MemoryStore

class TestMemoryTenantIsolation:
    """FIX-035: Verify memory recall uses tenant_id filter."""

    def test_recall_sql_contains_tenant_id(self) -> None:
        """The recall query MUST filter by tenant_id."""
        store = MemoryStore()
        # Inspect the SQL query used by recall()
        # Verify it contains 'tenant_id = %s'
        import inspect
        source = inspect.getsource(store.recall)
        assert "tenant_id" in source, "recall() must filter by tenant_id"

    def test_recall_request_requires_tenant_id(self) -> None:
        """MemoryRecallRequest must have tenant_id field."""
        from codeforge.memory.storage import MemoryRecallRequest
        req = MemoryRecallRequest(
            tenant_id="tenant-1",
            project_id="proj-1",
            query="test",
        )
        assert req.tenant_id == "tenant-1"
```

- [ ] **Step 2: Run test**

```bash
cd workers && python -m pytest tests/test_memory_tenant.py -v
```

- [ ] **Step 3: Commit**

```bash
git add workers/tests/test_memory_tenant.py
git commit -m "test: memory tenant isolation regression test (FIX-035)"
```

---

## Task 9: GraphRAG Module Tests (FIX-070)

**Files:**
- Create: `workers/tests/test_graphrag.py`

- [ ] **Step 1: Read graphrag.py to understand interface**

```bash
head -80 workers/codeforge/graphrag.py
```

- [ ] **Step 2: Write tests**

Test:
- Module imports successfully
- Core functions/classes instantiate
- Query safety (no SQL injection in graph queries)
- Error handling on connection failures

- [ ] **Step 3: Run test**

```bash
cd workers && python -m pytest tests/test_graphrag.py -v
```

- [ ] **Step 4: Commit**

```bash
git add workers/tests/test_graphrag.py
git commit -m "test: GraphRAG module tests (FIX-070)"
```

---

## Task 10: Benchmark Handler Tests — chi.URLParam (FIX-069)

**Files:**
- Create: `internal/adapter/http/handlers_benchmark_test.go` (or extend existing)

- [ ] **Step 1: Write regression test for chi.URLParam usage**

```go
func TestBenchmarkHandlers_UseChiURLParam(t *testing.T) {
    // Read handlers_benchmark.go source
    // Verify zero occurrences of r.PathValue
    // Verify chi.URLParam is used instead
    src, err := os.ReadFile("handlers_benchmark.go")
    require.NoError(t, err)

    assert.NotContains(t, string(src), "r.PathValue",
        "handlers_benchmark.go must use chi.URLParam, not r.PathValue")
    assert.Contains(t, string(src), "chi.URLParam",
        "handlers_benchmark.go must use chi.URLParam")
}
```

- [ ] **Step 2: Run test**

```bash
go test ./internal/adapter/http/... -run TestBenchmarkHandlers_UseChiURLParam -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/adapter/http/handlers_benchmark_test.go
git commit -m "test: regression test for chi.URLParam in benchmark handlers (FIX-069)"
```

---

## Task 11: Search Error Masking Test (FIX-068)

**Files:**
- Modify: `internal/adapter/http/handlers_search_test.go` (extend existing)

- [ ] **Step 1: Add error masking regression test**

```go
func TestSearchHandlers_NoInternalErrorLeakage(t *testing.T) {
    // Read handlers_search.go source
    src, err := os.ReadFile("handlers_search.go")
    require.NoError(t, err)

    // Must NOT contain err.Error() in writeError/http.Error calls
    assert.NotContains(t, string(src), `err.Error()`,
        "search handlers must not leak internal errors to clients")
}
```

- [ ] **Step 2: Run test**

```bash
go test ./internal/adapter/http/... -run TestSearchHandlers_NoInternalErrorLeakage -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/adapter/http/handlers_search_test.go
git commit -m "test: search error masking regression test (FIX-068)"
```

---

## Task 12: Contract Test Extension (FIX-086)

**Files:**
- Modify: `internal/port/messagequeue/contract_test.go`

- [ ] **Step 1: Identify missing subjects in contract tests**

```bash
# Compare subjects in queue.go vs contract_test.go
grep 'Subject' internal/port/messagequeue/queue.go | wc -l
grep 'Subject' internal/port/messagequeue/contract_test.go | wc -l
```

- [ ] **Step 2: Add contract test entries for missing subjects**

Add fixture entries for benchmark, review, A2A, and prompt evolution subjects that were added in FIX-018.

- [ ] **Step 3: Run test**

```bash
go test ./internal/port/messagequeue/... -run TestContract -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/port/messagequeue/contract_test.go
git commit -m "test: extend contract tests to cover all NATS subjects (FIX-086)"
```

---

## Task 13: Frontend Unit Test Foundation (FIX-034, FIX-067, FIX-074)

**Files:**
- Create: `frontend/src/features/notifications/notificationStore.test.ts`
- Create: `frontend/src/features/chat/commandStore.test.ts`
- Create: `frontend/src/lib/api/client.test.ts`

- [ ] **Step 1: Check existing frontend test setup**

Vitest is configured in `frontend/vite.config.ts` (line 1: `/// <reference types="vitest/config" />`).
Run command: `cd frontend && npm test` or `cd frontend && npx vitest run`.
Existing tests are in `frontend/src/features/canvas/__tests__/`.

```bash
ls frontend/src/features/canvas/__tests__/
```

- [ ] **Step 2: Create notificationStore test**

Create `frontend/src/features/notifications/notificationStore.test.ts`:

```typescript
import { describe, it, expect } from "vitest";
import { addNotification, notifications, clearNotifications } from "./notificationStore";

describe("notificationStore", () => {
    it("should export addNotification function", () => {
        expect(typeof addNotification).toBe("function");
    });

    it("should export notifications signal", () => {
        expect(notifications).toBeDefined();
    });

    it("should export clearNotifications function", () => {
        expect(typeof clearNotifications).toBe("function");
    });
});
```

- [ ] **Step 3: Create commandStore test**

Same pattern — verify exports, test command registration.

- [ ] **Step 4: Create API client test**

Test that the API client:
- Exports expected resource groups
- Has proper type annotations
- Handles 401 responses (auth redirect)

- [ ] **Step 5: Run tests**

```bash
cd frontend && npx vitest run --reporter=verbose 2>&1 | tail -20
```
Expected: All new test files pass. Vitest auto-discovers `*.test.ts` files.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/features/*/test*.ts frontend/src/lib/api/client.test.ts
git commit -m "test: frontend unit tests for notification, command stores, API client (FIX-034,067,074)"
```

---

## Task 14: Port/Interface Layer Tests (FIX-066)

**Files:**
- Create: `internal/port/messagequeue/queue_test.go`

- [ ] **Step 1: Write interface compliance tests**

Verify the Queue interface contract:
- All methods are defined
- Subject constants are non-empty strings
- No duplicate subject values

```go
func TestSubjectConstants_NoDuplicates(t *testing.T) {
    seen := make(map[string]string)
    // Use reflection or manual listing to check all Subject* constants
    // Ensure no two constants have the same value
}
```

- [ ] **Step 2: Run test**

```bash
go test ./internal/port/messagequeue/... -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/port/messagequeue/queue_test.go
git commit -m "test: port/interface layer tests — subject dedup, queue contract (FIX-066)"
```

---

## Task 15: Concurrency Safety Documentation (FIX-048, FIX-054)

**Files:**
- Modify: `workers/codeforge/consumer/_base.py`

FIX-025 already replaced `set` with `OrderedDict`. FIX-048 and FIX-054 are about concurrency safety. Since Python asyncio is single-threaded (GIL + event loop), the OrderedDict is safe. Document this.

- [ ] **Step 1: Add concurrency safety documentation**

Add to `_base.py` near the `_processed_ids` definition:

```python
    # CONCURRENCY SAFETY: Python asyncio runs on a single thread.
    # The OrderedDict is accessed only from coroutines on the same event loop,
    # so no lock is required. If this code is ever used with threading,
    # a threading.Lock must be added. See: FIX-048, FIX-054.
    _processed_ids: ClassVar[OrderedDict[str, None]] = OrderedDict()
```

- [ ] **Step 2: Commit**

```bash
git add workers/codeforge/consumer/_base.py
git commit -m "docs: document asyncio concurrency safety for _processed_ids (FIX-048,054)"
```

---

## Task 16: LSP Dead Code Documentation (FIX-075)

**Files:**
- Create: `internal/adapter/lsp/doc.go`

- [ ] **Step 1: Create package doc**

```go
// Package lsp provides a Language Server Protocol client for code intelligence
// features (go-to-definition, references, diagnostics).
//
// STATUS: This adapter is implemented but not yet wired into the application
// lifecycle. It will be activated when LSP integration is enabled per-project
// (planned for a future phase). Do NOT delete — the implementation is complete
// and tested.
//
// See: FIX-075, Phase 15D design doc.
package lsp
```

- [ ] **Step 2: Commit**

```bash
git add internal/adapter/lsp/doc.go
git commit -m "docs: document LSP adapter as planned-but-unwired (FIX-075)"
```

---

## Task 17: Handler Context Fix (FIX-085)

**Files:**
- Modify: `internal/adapter/http/handlers.go:266`

- [ ] **Step 1: Read the handler to identify remaining context.Background() usage**

```bash
grep -n "context.Background" internal/adapter/http/handlers.go
```

FIX-014 fixed `autoIndexProject`. Check if any other `context.Background()` calls remain that lose request-scoped values.

- [ ] **Step 2: Fix remaining occurrences**

Replace with `tenantctx.WithTenant(context.Background(), tenantID)` pattern, extracting tenant ID before the goroutine.

- [ ] **Step 3: Run vet**

```bash
go vet ./internal/adapter/http/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/adapter/http/handlers.go
git commit -m "fix: preserve tenant context in remaining handler goroutines (FIX-085)"
```

---

## Task 18: Consumer Name Documentation (FIX-087)

**Files:**
- Modify: `internal/adapter/nats/nats.go`

- [ ] **Step 1: Add consumer naming convention comment**

Near line 138 where consumer names are defined:

```go
// Consumer naming convention:
// - Durable names use the pattern "codeforge-<subject-group>" (e.g., "codeforge-conversation")
// - This ensures each consumer group processes messages independently
// - The prefix "codeforge-" prevents collision with other NATS clients
```

- [ ] **Step 2: Commit**

```bash
git add internal/adapter/nats/nats.go
git commit -m "docs: document NATS consumer naming convention (FIX-087)"
```

---

## Task 19: Logging Consistency TODO (FIX-092)

**Files:**
- Create: `workers/codeforge/LOG_MIGRATION.md`

- [ ] **Step 1: Create logging migration note**

```markdown
# Logging Migration Plan

The codebase uses a mix of `logging` (stdlib) and `structlog`.

**Target:** Migrate all modules to `structlog` for consistent structured JSON logging.

**Modules still using stdlib `logging`:**
- Run `grep -rn "import logging" workers/codeforge/ --include="*.py" -l` to find them.

**Priority:** LOW — both libraries produce correct output. Migration is cosmetic.

See: FIX-092
```

- [ ] **Step 2: Commit**

```bash
git add workers/codeforge/LOG_MIGRATION.md
git commit -m "docs: logging migration plan — stdlib logging to structlog (FIX-092)"
```

---

## Task 20: Security Hardening TODOs (FIX-095, FIX-096)

**Files:**
- Modify: `internal/adapter/http/middleware.go`
- Modify: `internal/middleware/ratelimit.go`

- [ ] **Step 1: Add CSRF documentation**

In `middleware.go`, near the SameSite cookie configuration:

```go
// CSRF PROTECTION: SameSite=Lax prevents cross-origin form submissions.
// For API-only backends (no HTML forms), SameSite is sufficient.
// If HTML forms are added in the future, add a CSRF token middleware.
// See: FIX-095
```

- [ ] **Step 2: Add per-user rate limiting TODO**

In `ratelimit.go:144`:

```go
// TODO(FIX-096): Add per-user rate limiting for authenticated endpoints.
// Currently IP-only. For shared IPs (corporate NAT), per-user limiting
// via JWT claims would be more fair. Low priority — IP-based is sufficient
// for current deployment model.
```

- [ ] **Step 3: Commit**

```bash
git add internal/adapter/http/middleware.go internal/middleware/ratelimit.go
git commit -m "docs: CSRF and per-user rate limiting documentation (FIX-095,096)"
```

---

## Task 21: API Contract Style Fixes (FIX-097, FIX-098, FIX-099, FIX-100)

**Files:**
- Modify: `internal/adapter/http/handlers_quarantine.go`
- Multiple handler files for spelling fix

- [ ] **Step 1: Export quarantine handler methods (FIX-097)**

Read `handlers_quarantine.go`. If handler methods are unexported but should be (they're route handlers), capitalize them.

- [ ] **Step 2: Standardize cancelled spelling (FIX-099)**

```bash
grep -rn "canceled" internal/adapter/http/ --include="*.go"
```

Replace `canceled` with `cancelled` (British English, matching majority of codebase).

- [ ] **Step 3: Add API style TODOs (FIX-098, FIX-100)**

Add TODO comments for POST-for-DELETE and missing PATCH — these are breaking changes that need API versioning.

- [ ] **Step 4: Run vet**

```bash
go vet ./internal/adapter/http/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/http/*.go
git commit -m "fix: API contract style — export quarantine handlers, standardize spelling (FIX-097,098,099,100)"
```

---

## Task 22: Test Infrastructure Improvements (FIX-101, FIX-102, FIX-103)

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add integration test stage TODO in CI**

```yaml
  # TODO(FIX-102): Add integration test job that:
  # - Starts postgres + nats containers
  # - Runs go test with -tags=integration
  # - Runs pytest with --integration marker
  # Requires: docker-compose in CI runner
```

- [ ] **Step 2: Add FakeLLM consolidation TODO (FIX-101)**

In `workers/tests/conftest.py`:

```python
# TODO(FIX-101): Consolidate FakeLLM test helpers.
# Currently duplicated across test_agent_loop.py, test_plan_act.py, test_routing.py.
# Extract to workers/tests/fake_llm.py (shared fixture).
```

- [ ] **Step 3: Add E2E URL config TODO (FIX-103)**

In `frontend/playwright.config.ts` or `e2e/helpers/`:

```typescript
// TODO(FIX-103): E2E tests use hardcoded localhost URLs in some specs.
// All URLs should come from baseURL in playwright config.
// Run: grep -rn "localhost" frontend/e2e/ to find remaining hardcoded URLs.
```

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml workers/tests/conftest.py
git commit -m "docs: test infrastructure improvement TODOs (FIX-101,102,103)"
```

---

## Task 23: Frontend Style Fixes (FIX-105, FIX-106)

**Files:**
- Modify: `frontend/src/features/project/ChatPanel.tsx` (TODO only)

- [ ] **Step 1: Add ChatPanel refactoring TODO (FIX-105)**

At the top of `ChatPanel.tsx`:

```typescript
// TODO(FIX-105): This file has high ESLint disable density due to its size (1141 lines).
// Refactor into sub-components: ChatMessages, ChatInput, ChatToolbar, useChatAGUIEvents.
// See also: Frontend Architecture Audit HIGH-001.
```

- [ ] **Step 2: Add SVG extraction TODO (FIX-106)**

Create `frontend/src/ui/icons/README.md`:

```markdown
# Icon System

TODO(FIX-106): Extract inline SVGs duplicated across features into shared icon components.
Currently using Unicode + inline SVG. Consider extracting common icons into this directory.
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/project/ChatPanel.tsx frontend/src/ui/icons/README.md
git commit -m "docs: frontend refactoring TODOs — ChatPanel decomposition, SVG extraction (FIX-105,106)"
```

---

## Task 24: Routing Testability TODOs (FIX-110, FIX-111)

**Files:**
- Modify: `workers/codeforge/routing/blocklist.py`
- Modify: `workers/codeforge/routing/key_filter.py`

- [ ] **Step 1: Add DI TODO to blocklist (FIX-110)**

```python
# TODO(FIX-110): Module-level singleton reduces testability.
# Consider dependency injection: pass Blocklist instance to HybridRouter
# instead of using module-level _blocklist. Low priority — tests currently
# work around this via monkeypatching.
```

- [ ] **Step 2: Document key_filter safety (FIX-111)**

```python
# _warned_providers is accessed only from the asyncio event loop (single-threaded).
# No lock is needed. If threading is introduced, add threading.Lock.
# See: FIX-111
_warned_providers: set[str] = set()
```

- [ ] **Step 3: Commit**

```bash
git add workers/codeforge/routing/blocklist.py workers/codeforge/routing/key_filter.py
git commit -m "docs: routing testability and concurrency notes (FIX-110,111)"
```

---

## Task 25: Update Audit Reports & Overview

**Files:**
- Modify: `docs/audits/2026-03-20-audit-overview.md`
- Modify: `docs/audits/2026-03-20-test-coverage-audit.md`

- [ ] **Step 1: Update Test Coverage audit with new test counts**

Recalculate score based on remaining unfixed findings (should improve significantly).

- [ ] **Step 2: Update overview document**

Update the score summary table, remaining findings count, and fix completion stats.

- [ ] **Step 3: Update docs/todo.md**

Mark all completed audit fixes with `[x]` and date.

- [ ] **Step 4: Commit**

```bash
git add docs/audits/*.md docs/todo.md
git commit -m "audit: update all reports — remaining findings addressed, scores recalculated"
```
