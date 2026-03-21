# Benchmark E2E Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 5 issues discovered during the 2026-03-19 benchmark E2E test run (REC-1 through REC-5).

**Architecture:** 4 independent quick fixes (REC-2 to REC-5, Go HTTP handlers + domain logic) plus 1 larger change (REC-1, Python worker concurrency). All changes are independent — no task depends on another.

**Tech Stack:** Go (HTTP handlers, domain), Python (asyncio consumer), TDD

**Report:** `docs/testing/2026-03-19-benchmark-e2e-report.md`
**TODO:** `docs/todo.md` (section "Benchmark E2E Full Run 2026-03-19")

---

## Task Dependency Graph

```
Task 1 (REC-2: Trajectory)     — independent
Task 2 (REC-3: Training Export) — independent
Task 3 (REC-4: Suite Type)      — independent
Task 4 (REC-5: Watchdog)        — independent
Task 5 (REC-1: Parallel Runs)   — independent
Task 6 (Docs + Commit)          — depends on Task 1-5
```

Tasks 1-5 can be executed in any order or in parallel. Task 6 is the final documentation + commit step.

---

### Task 1: Trajectory Endpoint Graceful Empty Response (REC-2)

**Files:**
- Modify: `internal/adapter/http/handlers_roadmap.go:444-455`
- Test: `internal/adapter/http/handlers_roadmap_test.go`

- [ ] **Step 1: Write failing Go test — trajectory returns 200 with empty events when no events exist**

```go
func TestGetTrajectory_NoEvents_Returns200Empty(t *testing.T) {
	// Setup: mock event store that returns error for unknown run
	store := &fakeEventStore{
		loadErr: fmt.Errorf("no events found"),
	}
	h := &Handlers{Events: store}
	req := httptest.NewRequest("GET", "/api/v1/runs/nonexistent-run/trajectory?limit=50", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "nonexistent-run")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.GetTrajectory(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "expected 200, got %d: %s", w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotNil(t, resp["events"])
	assert.Equal(t, false, resp["has_more"])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && go test ./internal/adapter/http/ -run TestGetTrajectory_NoEvents -v`
Expected: FAIL (currently returns 500)

- [ ] **Step 3: Fix handler — return empty result on error instead of 500**

In `internal/adapter/http/handlers_roadmap.go:444-455`, replace error handling with graceful fallback:

```go
	page, err := h.Events.LoadTrajectory(r.Context(), runID, filter, cursor, limit)
	if err != nil {
		// Return empty result for runs with no events yet (e.g. still running).
		page = &eventstore.TrajectoryPage{
			Events:  []event.Event{},
			Cursor:  "",
			HasMore: false,
			Total:   0,
		}
	}

	stats, err := h.Events.TrajectoryStats(r.Context(), runID)
	if err != nil {
		stats = &eventstore.TrajectoryStats{}
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge && go test ./internal/adapter/http/ -run TestGetTrajectory -v`
Expected: PASS

- [ ] **Step 5: Run full handler test suite for regressions**

Run: `cd /workspaces/CodeForge && go test ./internal/adapter/http/ -count=1 -timeout 60s`
Expected: All tests pass

---

### Task 2: Training Export Empty Array Response (REC-3)

**Files:**
- Modify: `internal/adapter/http/handlers_benchmark.go:333-339`
- Test: `internal/adapter/http/handlers_benchmark_test.go`

- [ ] **Step 1: Write failing Go test — JSONL export returns empty JSON array when no pairs exist**

```go
func TestExportTrainingData_EmptyPairs_ReturnsEmptyArray(t *testing.T) {
	svc := &fakeBenchmarkService{
		trainingPairs: []benchmark.TrainingPair{}, // empty
	}
	h := &Handlers{Benchmarks: svc}
	req := httptest.NewRequest("GET", "/api/v1/benchmarks/runs/some-id/export/training", nil)
	req.SetPathValue("id", "some-id")
	w := httptest.NewRecorder()

	h.ExportTrainingData(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Should return a valid JSON empty array, not empty body
	body := strings.TrimSpace(w.Body.String())
	assert.Equal(t, "[]", body, "expected empty JSON array, got %q", body)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && go test ./internal/adapter/http/ -run TestExportTrainingData_EmptyPairs -v`
Expected: FAIL (currently returns empty body)

- [ ] **Step 3: Fix handler — return empty JSON array when no pairs exist**

In `internal/adapter/http/handlers_benchmark.go`, replace lines 333-339:

```go
	// Default: JSONL (ndjson) — one JSON object per line.
	// If no pairs, return empty JSON array for consistency.
	if len(pairs) == 0 {
		writeJSON(w, http.StatusOK, pairs)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", "attachment; filename=\"training_pairs.jsonl\"")
	enc := json.NewEncoder(w)
	for i := range pairs {
		_ = enc.Encode(pairs[i])
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge && go test ./internal/adapter/http/ -run TestExportTrainingData -v`
Expected: PASS

- [ ] **Step 5: Verify non-empty JSONL export still works**

Run: `cd /workspaces/CodeForge && go test ./internal/adapter/http/ -run TestExportTraining -v`
Expected: All training export tests pass

---

### Task 3: Suite Type Auto-Derivation from Provider (REC-4)

**Files:**
- Modify: `internal/domain/benchmark/benchmark.go:72-93`
- Modify: `internal/service/benchmark.go:92-110`
- Test: `internal/domain/benchmark/benchmark_test.go`
- Test: `internal/service/benchmark_test.go`

- [ ] **Step 1: Write failing domain test — ProviderDefaultType maps providers to types**

```go
func TestProviderDefaultType(t *testing.T) {
	tests := []struct {
		provider string
		expected BenchmarkType
	}{
		{"codeforge_simple", TypeSimple},
		{"humaneval", TypeSimple},
		{"mbpp", TypeSimple},
		{"swebench", TypeAgent},
		{"sparcbench", TypeAgent},
		{"codeforge_agent", TypeAgent},
		{"codeforge_tool_use", TypeToolUse},
		{"unknown_provider", ""},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got := ProviderDefaultType(tt.provider)
			assert.Equal(t, tt.expected, got)
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && go test ./internal/domain/benchmark/ -run TestProviderDefaultType -v`
Expected: FAIL (function not defined)

- [ ] **Step 3: Add ProviderDefaultType function**

In `internal/domain/benchmark/benchmark.go`, add after the `CreateSuiteRequest` struct:

```go
// providerDefaultType maps provider names to their inherent benchmark type.
var providerDefaultType = map[string]BenchmarkType{
	"codeforge_simple":   TypeSimple,
	"codeforge_tool_use": TypeToolUse,
	"codeforge_agent":    TypeAgent,
	"humaneval":          TypeSimple,
	"mbpp":               TypeSimple,
	"bigcodebench":       TypeSimple,
	"cruxeval":           TypeSimple,
	"livecodebench":      TypeSimple,
	"swebench":           TypeAgent,
	"sparcbench":         TypeAgent,
	"aider_polyglot":     TypeAgent,
	"terminal_bench":     TypeAgent,
	"dpai_arena":         TypeSimple,
}

// ProviderDefaultType returns the default benchmark type for a provider.
// Returns empty string if unknown.
func ProviderDefaultType(provider string) BenchmarkType {
	return providerDefaultType[provider]
}
```

- [ ] **Step 4: Run domain test to verify it passes**

Run: `cd /workspaces/CodeForge && go test ./internal/domain/benchmark/ -run TestProviderDefaultType -v`
Expected: PASS

- [ ] **Step 5: Write failing service test — RegisterSuite auto-derives type when omitted**

```go
func TestRegisterSuite_AutoDerivesType(t *testing.T) {
	store := newFakeBenchmarkStore()
	svc := NewBenchmarkService(store, nil, nil, nil, nil)
	req := &benchmark.CreateSuiteRequest{
		Name:         "Test Suite",
		ProviderName: "humaneval",
		// Type intentionally omitted
	}
	suite, err := svc.RegisterSuite(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, benchmark.TypeSimple, suite.Type)
}
```

- [ ] **Step 6: Run service test to verify it fails**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestRegisterSuite_AutoDerivesType -v`
Expected: FAIL (validation rejects empty type)

- [ ] **Step 7: Add auto-derivation to RegisterSuite before Validate()**

In `internal/service/benchmark.go:93`, add before `req.Validate()`:

```go
func (s *BenchmarkService) RegisterSuite(ctx context.Context, req *benchmark.CreateSuiteRequest) (*benchmark.Suite, error) {
	// Auto-derive type from provider when not explicitly set.
	if req.Type == "" {
		req.Type = benchmark.ProviderDefaultType(req.ProviderName)
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
```

- [ ] **Step 8: Run tests to verify all pass**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestRegisterSuite -v && go test ./internal/domain/benchmark/ -v`
Expected: PASS

---

### Task 4: Per-Type Watchdog Timeout (REC-5)

**Files:**
- Modify: `internal/service/benchmark.go` (watchdog function)
- Test: `internal/service/benchmark_test.go`

- [ ] **Step 1: Find the watchdog function**

Run: `cd /workspaces/CodeForge && grep -n 'watchdog\|WatchdogTimeout\|StaleRuns\|exceeded.*without completion' internal/service/benchmark.go`
Read the watchdog logic to identify the exact function and line numbers.

- [ ] **Step 2: Write failing test — different timeout per benchmark type**

```go
func TestWatchdogTimeout_PerType(t *testing.T) {
	tests := []struct {
		benchType benchmark.BenchmarkType
		expected  time.Duration
	}{
		{benchmark.TypeSimple, 30 * time.Minute},
		{benchmark.TypeToolUse, 60 * time.Minute},
		{benchmark.TypeAgent, 4 * time.Hour},
		{"", 2 * time.Hour}, // fallback to global default
	}
	for _, tt := range tests {
		t.Run(string(tt.benchType), func(t *testing.T) {
			got := watchdogTimeoutForType(tt.benchType, 2*time.Hour)
			assert.Equal(t, tt.expected, got)
		})
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestWatchdogTimeout_PerType -v`
Expected: FAIL (function not defined)

- [ ] **Step 4: Add watchdogTimeoutForType function**

In `internal/service/benchmark.go`, add:

```go
// watchdogTimeoutForType returns the watchdog timeout based on benchmark type.
// Falls back to globalDefault for unknown types.
func watchdogTimeoutForType(bt benchmark.BenchmarkType, globalDefault time.Duration) time.Duration {
	switch bt {
	case benchmark.TypeSimple:
		return 30 * time.Minute
	case benchmark.TypeToolUse:
		return 60 * time.Minute
	case benchmark.TypeAgent:
		return 4 * time.Hour
	default:
		return globalDefault
	}
}
```

- [ ] **Step 5: Integrate into existing watchdog goroutine**

Find the existing watchdog loop and replace the single global timeout with `watchdogTimeoutForType(run.BenchmarkType, globalTimeout)`. The exact code depends on the watchdog implementation found in Step 1.

- [ ] **Step 6: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestWatchdog -v`
Expected: PASS

---

### Task 5: Parallel Benchmark Run Processing (REC-1)

**Files:**
- Modify: `workers/codeforge/consumer/_benchmark.py:133-256`
- Create: `workers/tests/test_benchmark_parallel.py`

- [ ] **Step 1: Write failing test — concurrent benchmark runs complete in parallel**

```python
# workers/tests/test_benchmark_parallel.py
import asyncio
import time

import pytest

from codeforge.consumer._benchmark import BenchmarkHandlerMixin


class FakeBenchmarkHandler(BenchmarkHandlerMixin):
    """Minimal fake for testing concurrency."""

    def __init__(self):
        self._js = None
        self._llm = None
        self._duplicate_set = set()
        self._completed_runs = []
        self._semaphore = asyncio.Semaphore(3)

    def _is_duplicate(self, key: str) -> bool:
        if key in self._duplicate_set:
            return True
        self._duplicate_set.add(key)
        return False


@pytest.mark.asyncio
async def test_semaphore_limits_concurrency():
    """Verify that the semaphore limits concurrent runs."""
    sem = asyncio.Semaphore(2)
    active = 0
    max_active = 0

    async def fake_run():
        nonlocal active, max_active
        async with sem:
            active += 1
            max_active = max(max_active, active)
            await asyncio.sleep(0.05)
            active -= 1

    tasks = [asyncio.create_task(fake_run()) for _ in range(5)]
    await asyncio.gather(*tasks)

    assert max_active == 2, f"Expected max 2 concurrent, got {max_active}"


@pytest.mark.asyncio
async def test_semaphore_default_from_env(monkeypatch):
    """Verify BENCHMARK_MAX_PARALLEL env var is respected."""
    monkeypatch.setenv("BENCHMARK_MAX_PARALLEL", "5")
    # Re-import to pick up env var
    from codeforge.consumer._benchmark import _get_benchmark_semaphore
    sem = _get_benchmark_semaphore()
    assert sem._value == 5
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/test_benchmark_parallel.py -v`
Expected: FAIL (`_get_benchmark_semaphore` not defined)

- [ ] **Step 3: Add semaphore and _execute_benchmark_run to handler**

In `workers/codeforge/consumer/_benchmark.py`, add at module level:

```python
import os

def _get_benchmark_semaphore() -> asyncio.Semaphore:
    """Create semaphore with configurable max parallel runs."""
    max_parallel = int(os.environ.get("BENCHMARK_MAX_PARALLEL", "3"))
    return asyncio.Semaphore(max_parallel)

_benchmark_semaphore: asyncio.Semaphore | None = None

def _ensure_benchmark_semaphore() -> asyncio.Semaphore:
    global _benchmark_semaphore
    if _benchmark_semaphore is None:
        _benchmark_semaphore = _get_benchmark_semaphore()
    return _benchmark_semaphore
```

- [ ] **Step 4: Refactor _handle_benchmark_run to spawn concurrent task**

Split `_handle_benchmark_run` into two parts:
1. `_handle_benchmark_run` — validates, deduplicates, acks, spawns task
2. `_execute_benchmark_run` — the actual run logic (moved from the try block)

```python
async def _handle_benchmark_run(self, msg: nats.aio.msg.Msg) -> None:
    """Handle a benchmark run request — dispatch to concurrent executor."""
    import os
    from codeforge.models import BenchmarkRunRequest

    if os.getenv("APP_ENV") != "development":
        logger.warning("benchmark run ignored (not in dev mode)")
        await msg.ack()
        return

    try:
        req = BenchmarkRunRequest.model_validate_json(msg.data)
    except Exception:
        logger.exception("invalid benchmark request payload")
        await msg.ack()
        return

    run_id = req.run_id
    tenant_id = getattr(req, "tenant_id", "")
    benchmark_type = getattr(req, "benchmark_type", "simple")
    log = logger.bind(run_id=run_id, benchmark_type=benchmark_type, model=req.model)

    if self._is_duplicate(f"bench-{req.run_id}"):
        log.warning("duplicate benchmark run, skipping")
        await msg.ack()
        return

    await msg.ack()

    # Spawn as concurrent task with semaphore guard.
    task = asyncio.create_task(
        self._execute_benchmark_run(req, log),
        name=f"bench-{run_id}",
    )
    task.add_done_callback(_handle_task_exception)


def _handle_task_exception(task: asyncio.Task) -> None:
    """Log unhandled exceptions from spawned benchmark tasks."""
    if task.cancelled():
        return
    exc = task.exception()
    if exc is not None:
        logger.error(
            "benchmark task failed with unhandled exception",
            task_name=task.get_name(),
            error=str(exc),
        )
```

The `_execute_benchmark_run` method contains the existing try/except block from line 181-256, wrapped with `async with _ensure_benchmark_semaphore():`.

```python
async def _execute_benchmark_run(self, req, log) -> None:
    """Execute a benchmark run with semaphore-guarded concurrency."""
    from codeforge.evaluation.pipeline import EvaluationPipeline
    from codeforge.models import BenchmarkRunResult

    sem = _ensure_benchmark_semaphore()
    async with sem:
        run_id = req.run_id
        tenant_id = getattr(req, "tenant_id", "")
        benchmark_type = getattr(req, "benchmark_type", "simple")

        try:
            log.info("benchmark run started")
            start = time.monotonic()
            # ... (rest of existing logic from lines 184-242, unchanged)

        except Exception as exc:
            log.exception("benchmark run failed")
            await self._publish_error(
                BenchmarkRunResult(
                    run_id=run_id,
                    tenant_id=tenant_id,
                    status="failed",
                    error=str(exc),
                ),
                SUBJECT_BENCHMARK_RUN_RESULT,
            )
```

- [ ] **Step 5: Run tests**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/test_benchmark_parallel.py -v`
Expected: PASS

- [ ] **Step 6: Run existing benchmark tests for regressions**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/test_score_key_normalization.py workers/tests/test_model_validation.py -v`
Expected: All existing tests pass

---

### Task 6: Documentation Update & Commit

**Files:**
- Modify: `docs/todo.md` (mark REC items done)
- Modify: `docs/dev-setup.md` (document BENCHMARK_MAX_PARALLEL env var)
- Modify: `docs/testing/2026-03-19-benchmark-e2e-report.md` (add fix status)

- [ ] **Step 1: Update todo.md — mark completed RECs**

Mark all REC-1 through REC-5 items as `[x]` with date `(2026-03-20)`.

- [ ] **Step 2: Document BENCHMARK_MAX_PARALLEL in dev-setup.md**

Add to the environment variables section:

```markdown
| `BENCHMARK_MAX_PARALLEL` | Max concurrent benchmark runs in Python worker | `3` |
```

- [ ] **Step 3: Run pre-commit checks**

Run: `cd /workspaces/CodeForge && pre-commit run --all-files`
Expected: All checks pass

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "fix(benchmark): parallel runs, trajectory 200, training export, suite type, watchdog per-type

- REC-1: Parallel benchmark run processing via asyncio.Semaphore + create_task
  (BENCHMARK_MAX_PARALLEL env var, default 3)
- REC-2: Trajectory endpoint returns 200 with empty events instead of 500
- REC-3: Training JSONL export returns [] when no pairs exist
- REC-4: Suite creation auto-derives type from provider_name
- REC-5: Watchdog timeout per benchmark type (simple 30m, tool_use 1h, agent 4h)"
```
