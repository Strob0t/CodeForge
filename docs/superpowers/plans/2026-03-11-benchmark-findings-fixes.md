# Benchmark Findings Fixes — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix all 5 backend bugs discovered by the benchmark validation E2E tests (documented in `frontend/e2e/benchmark-validation/FINDINGS.md`).

**Architecture:** Fixes span the Go Core (validation, watchdog) and Python Workers (score key normalization, model validation, prompt compression, routing guard). Each bug is an independent task with its own TDD cycle.

**Tech Stack:** Go 1.25, Python 3.12 (Poetry, Pytest), PostgreSQL 18, NATS JetStream, LiteLLM, DeepEval

---

## File Structure

### Files to Create
| File | Purpose |
|------|---------|
| `workers/codeforge/evaluation/evaluators/prompt_compressor.py` | Prompt compression utility for LLM Judge (Bug 5) |
| `workers/tests/test_score_key_normalization.py` | Tests for Bug 1 score key fix |
| `workers/tests/test_model_validation.py` | Tests for Bug 3 model validation |
| `workers/tests/test_prompt_compressor.py` | Tests for Bug 5 prompt compression |

### Files to Modify
| File | Lines | Purpose |
|------|-------|---------|
| `workers/codeforge/consumer/_benchmark.py` | 442-471, 547-592, 152-163 | Bug 1: score normalization in `_convert_result`; Bug 3: model validation; Bug 4: routing guard in `_resolve_effective_llm` |
| `internal/domain/benchmark/benchmark.go` | 171-189 | Bug 2: dataset existence validation in `Validate()` |
| `internal/service/benchmark.go` | 164-240, 26-37 | Bug 2: fail on missing dataset in `StartRun()`; Bug 2: watchdog goroutine; Bug 3: model validation |
| `internal/domain/benchmark/benchmark_test.go` | append | Bug 2+3: new validation test cases |
| `workers/codeforge/evaluation/evaluators/llm_judge.py` | 49-69 | Bug 5: prompt compression before LLM call |
| `workers/codeforge/evaluation/evaluators/trajectory_verifier.py` | 86-117 | Bug 5: prompt compression for trajectory |
| `workers/codeforge/evaluation/metrics.py` | 27-51 | Bug 5: truncation before DeepEval G-Eval |

---

## Chunk 1: Bug 1 — Score Key Names Don't Match Metric Request Names

### Task 1: Normalize score keys in `_convert_result()`

**Problem:** When a user requests `metrics: ["llm_judge"]`, the API returns scores under `{"correctness": 0.8}` instead of `{"llm_judge": 0.8}`. This is because `LLMJudgeEvaluator` produces `EvalDimension(name="correctness", ...)` and `_convert_result()` uses `dim.name` as the dict key.

**The full mapping that needs normalization:**

| Evaluator | `evaluator.name` | Produces `EvalDimension.name` values |
|-----------|-------------------|--------------------------------------|
| `LLMJudgeEvaluator` | `"llm_judge"` | `"correctness"`, `"faithfulness"`, `"answer_relevancy"`, `"tool_correctness"` |
| `SPARCEvaluator` | `"sparc"` | `"sparc_steps"`, `"sparc_time"`, `"sparc_cost"`, `"sparc_complexity"`, `"sparc_code_quality"`, `"sparc_security"` |
| `TrajectoryVerifierEvaluator` | `"trajectory_verifier"` | `"trajectory_solution_quality"`, `"trajectory_approach_efficiency"`, `"trajectory_code_quality"`, `"trajectory_error_recovery"`, `"trajectory_completeness"`, (error fallback: `"trajectory_quality"`) |
| `FunctionalTestEvaluator` | `"functional_test"` | `"functional_test"` (already matches) |

**Fix approach:** In `_convert_result()`, after building `scores` from raw dimension names, add aggregated metric-level keys. Each evaluator's dimensions are averaged into a single score under the metric request name. The raw dimension scores are preserved in a `_details` sub-dict.

**Files:**
- Modify: `workers/codeforge/consumer/_benchmark.py:442-471`
- Test: `workers/tests/test_score_key_normalization.py` (new)

- [ ] **Step 1: Write failing test for score key normalization**

Create `workers/tests/test_score_key_normalization.py`:

```python
"""Tests for Bug 1: score key normalization in _convert_result."""

from __future__ import annotations

from dataclasses import dataclass, field

import pytest


@dataclass
class FakeEvalDimension:
    name: str
    score: float
    details: dict = field(default_factory=dict)
    cost_usd: float = 0.0


@dataclass
class FakeEvalScore:
    dimensions: list = field(default_factory=list)

    def average_score(self) -> float:
        if not self.dimensions:
            return 0.0
        return sum(d.score for d in self.dimensions) / len(self.dimensions)


@dataclass
class FakeToolCall:
    name: str = "test_tool"
    args: dict = field(default_factory=dict)


@dataclass
class FakeExecution:
    actual_output: str = "def hello(): return 'hi'"
    tool_calls: list = field(default_factory=list)
    cost_usd: float = 0.01
    tokens_in: int = 100
    tokens_out: int = 50
    duration_ms: int = 1000
    files_changed: list = field(default_factory=list)
    test_output: str = ""


@dataclass
class FakeTask:
    id: str = "task-1"
    name: str = "hello_world"
    expected_output: str = "def hello(): return 'hello'"


@dataclass
class FakeRunResult:
    task: FakeTask = field(default_factory=FakeTask)
    execution: FakeExecution = field(default_factory=FakeExecution)
    eval_score: FakeEvalScore | None = None


class TestScoreKeyNormalization:
    """Verify that _convert_result produces metric-request-level keys."""

    def test_llm_judge_scores_include_aggregated_key(self):
        """When llm_judge produces 'correctness', scores must also have 'llm_judge'."""
        from codeforge.consumer._benchmark import _convert_result

        result = FakeRunResult(
            eval_score=FakeEvalScore(
                dimensions=[FakeEvalDimension(name="correctness", score=0.8)]
            ),
        )
        converted = _convert_result(result)
        scores = converted.scores

        # Raw dimension key preserved
        assert "correctness" in scores
        assert scores["correctness"] == 0.8

        # Aggregated metric key added
        assert "llm_judge" in scores
        assert scores["llm_judge"] == 0.8

    def test_sparc_scores_include_aggregated_key(self):
        """SPARC dimensions should aggregate into a 'sparc' key."""
        from codeforge.consumer._benchmark import _convert_result

        result = FakeRunResult(
            eval_score=FakeEvalScore(
                dimensions=[
                    FakeEvalDimension(name="sparc_steps", score=0.9),
                    FakeEvalDimension(name="sparc_time", score=0.7),
                    FakeEvalDimension(name="sparc_cost", score=0.8),
                    FakeEvalDimension(name="sparc_complexity", score=0.75),
                    FakeEvalDimension(name="sparc_code_quality", score=0.6),
                    FakeEvalDimension(name="sparc_security", score=1.0),
                ]
            ),
        )
        converted = _convert_result(result)
        scores = converted.scores

        # All raw dimension keys preserved
        assert "sparc_steps" in scores
        assert "sparc_cost" in scores

        # Aggregated sparc key = average of all sparc_* dimensions
        assert "sparc" in scores
        expected_avg = (0.9 + 0.7 + 0.8 + 0.75 + 0.6 + 1.0) / 6
        assert abs(scores["sparc"] - expected_avg) < 0.001

    def test_trajectory_verifier_scores_include_aggregated_key(self):
        """Trajectory dimensions should aggregate into 'trajectory_verifier'."""
        from codeforge.consumer._benchmark import _convert_result

        result = FakeRunResult(
            eval_score=FakeEvalScore(
                dimensions=[
                    FakeEvalDimension(name="trajectory_solution_quality", score=0.8),
                    FakeEvalDimension(name="trajectory_approach_efficiency", score=0.7),
                    FakeEvalDimension(name="trajectory_code_quality", score=0.9),
                    FakeEvalDimension(name="trajectory_error_recovery", score=0.6),
                    FakeEvalDimension(name="trajectory_completeness", score=0.5),
                ]
            ),
        )
        converted = _convert_result(result)
        scores = converted.scores

        assert "trajectory_verifier" in scores
        expected_avg = (0.8 + 0.7 + 0.9 + 0.6 + 0.5) / 5
        assert abs(scores["trajectory_verifier"] - expected_avg) < 0.001

    def test_trajectory_quality_error_fallback(self):
        """When trajectory verifier fails, 'trajectory_quality' maps to 'trajectory_verifier'."""
        from codeforge.consumer._benchmark import _convert_result

        result = FakeRunResult(
            eval_score=FakeEvalScore(
                dimensions=[FakeEvalDimension(name="trajectory_quality", score=0.0)]
            ),
        )
        converted = _convert_result(result)
        scores = converted.scores

        assert "trajectory_verifier" in scores
        assert scores["trajectory_verifier"] == 0.0

    def test_functional_test_unchanged(self):
        """functional_test already matches — should not be duplicated."""
        from codeforge.consumer._benchmark import _convert_result

        result = FakeRunResult(
            eval_score=FakeEvalScore(
                dimensions=[FakeEvalDimension(name="functional_test", score=1.0)]
            ),
        )
        converted = _convert_result(result)
        scores = converted.scores

        assert "functional_test" in scores
        assert scores["functional_test"] == 1.0

    def test_mixed_evaluators(self):
        """Multiple evaluators in one run produce all aggregated keys."""
        from codeforge.consumer._benchmark import _convert_result

        result = FakeRunResult(
            eval_score=FakeEvalScore(
                dimensions=[
                    FakeEvalDimension(name="correctness", score=0.8),
                    FakeEvalDimension(name="functional_test", score=1.0),
                    FakeEvalDimension(name="sparc_steps", score=0.9),
                    FakeEvalDimension(name="sparc_time", score=0.7),
                    FakeEvalDimension(name="sparc_cost", score=0.8),
                    FakeEvalDimension(name="sparc_complexity", score=0.75),
                    FakeEvalDimension(name="sparc_code_quality", score=0.6),
                    FakeEvalDimension(name="sparc_security", score=1.0),
                ]
            ),
        )
        converted = _convert_result(result)
        scores = converted.scores

        assert "llm_judge" in scores
        assert "functional_test" in scores
        assert "sparc" in scores

    def test_no_eval_score(self):
        """No eval_score produces empty scores dict."""
        from codeforge.consumer._benchmark import _convert_result

        result = FakeRunResult(eval_score=None)
        converted = _convert_result(result)
        assert converted.scores == {}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && poetry run pytest workers/tests/test_score_key_normalization.py -v`
Expected: FAIL — `"llm_judge" not in scores` (aggregated keys don't exist yet)

- [ ] **Step 3: Implement score key normalization in `_convert_result()`**

Edit `workers/codeforge/consumer/_benchmark.py:442-471`. After building `scores` from raw dimension names, add aggregated metric-level keys:

```python
# Dimension-name-to-metric-request-name mapping.
# Each evaluator's dimension names are grouped and averaged into a single
# aggregated score under the metric request name that the user originally specified.
_DIMENSION_TO_METRIC: dict[str, str] = {
    # LLMJudgeEvaluator dimensions → "llm_judge"
    "correctness": "llm_judge",
    "faithfulness": "llm_judge",
    "answer_relevancy": "llm_judge",
    "tool_correctness": "llm_judge",
    # SPARCEvaluator dimensions → "sparc"
    "sparc_steps": "sparc",
    "sparc_time": "sparc",
    "sparc_cost": "sparc",
    "sparc_complexity": "sparc",
    "sparc_code_quality": "sparc",
    "sparc_security": "sparc",
    # TrajectoryVerifierEvaluator dimensions → "trajectory_verifier"
    "trajectory_solution_quality": "trajectory_verifier",
    "trajectory_approach_efficiency": "trajectory_verifier",
    "trajectory_code_quality": "trajectory_verifier",
    "trajectory_error_recovery": "trajectory_verifier",
    "trajectory_completeness": "trajectory_verifier",
    "trajectory_quality": "trajectory_verifier",  # error fallback key
    # FunctionalTestEvaluator → already "functional_test" (identity)
    "functional_test": "functional_test",
}


def _aggregate_metric_scores(scores: dict[str, float]) -> None:
    """Add aggregated metric-request-level keys to scores dict (in-place).

    For each dimension key in scores, look up its parent metric name.
    Dimensions sharing a parent are averaged into one aggregated score.
    Raw dimension keys are preserved; aggregated keys are added.
    """
    metric_sums: dict[str, float] = {}
    metric_counts: dict[str, int] = {}
    for dim_name, dim_score in list(scores.items()):
        metric = _DIMENSION_TO_METRIC.get(dim_name)
        if metric and metric != dim_name:
            metric_sums[metric] = metric_sums.get(metric, 0.0) + dim_score
            metric_counts[metric] = metric_counts.get(metric, 0) + 1

    for metric, total in metric_sums.items():
        if metric not in scores:
            scores[metric] = round(total / metric_counts[metric], 4)
```

Then in `_convert_result()`, after the `for dim in r.eval_score.dimensions:` loop, add:

```python
    _aggregate_metric_scores(scores)
```

Also add the same call in `_convert_rollout_outcome()` if it exists with a similar pattern.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge && poetry run pytest workers/tests/test_score_key_normalization.py -v`
Expected: All 8 tests PASS

- [ ] **Step 5: Run existing benchmark tests for regressions**

Run: `cd /workspaces/CodeForge && poetry run pytest workers/tests/test_benchmark_runners.py workers/tests/test_evaluation_pipeline.py -v`
Expected: All PASS (no regressions)

- [ ] **Step 6: Commit**

```bash
git add workers/codeforge/consumer/_benchmark.py workers/tests/test_score_key_normalization.py
git commit -m "fix(benchmark): normalize score keys to match metric request names (Bug 1)

Score keys like 'correctness' now also produce an aggregated 'llm_judge'
key. Same for sparc_* → 'sparc' and trajectory_* → 'trajectory_verifier'.
Raw dimension keys are preserved for detailed analysis."
```

---

## Chunk 2: Bug 2 — Error Runs Stay "running" Instead of Transitioning to "failed"

Three-tier fix: (A) Go API validation rejects bad datasets at request time, (B) Python error handler robustness, (C) Go watchdog goroutine for orphaned runs.

### Task 2A: Fail on non-existent dataset in Go `StartRun()`

**Problem:** `StartRun()` at line 187-189 logs a warning when dataset path resolution fails but continues publishing to NATS. The Python worker may never receive a valid dataset, leaving the run stuck in `"running"`.

**Fix:** Return an error from `StartRun()` when the dataset path cannot be resolved and no suite/provider fallback exists.

**Files:**
- Modify: `internal/service/benchmark.go:175-190`
- Test: `internal/domain/benchmark/benchmark_test.go` (append)

- [ ] **Step 1: Write failing test for dataset validation in StartRun**

Add to `internal/domain/benchmark/benchmark_test.go`:

```go
func TestCreateRunRequest_Validate_DatasetAutoModel(t *testing.T) {
	// model=auto without routing should still pass Validate() —
	// that's an API-level concern in StartRun, not domain validation.
	req := benchmark.CreateRunRequest{
		Dataset: "basic-coding",
		Model:   "auto",
		Metrics: []string{"llm_judge"},
	}
	if err := req.Validate(); err != nil {
		t.Errorf("Validate() should accept model=auto, got %v", err)
	}
}
```

For the `StartRun` test, we need a service-level test. Add to an existing or new file `internal/service/benchmark_test.go`:

```go
func TestStartRun_InvalidDataset_ReturnsError(t *testing.T) {
	// StartRun should fail when dataset path doesn't exist and there's no suite fallback.
	svc := NewBenchmarkService(fakeStore{}, "/tmp/nonexistent-datasets-dir")
	ctx := tenantctx.WithTenant(context.Background(), "test-tenant")

	req := &benchmark.CreateRunRequest{
		Dataset: "nonexistent-dataset-xyz",
		Model:   "test-model",
		Metrics: []string{"llm_judge"},
	}

	_, err := svc.StartRun(ctx, req)
	if err == nil {
		t.Error("expected error for non-existent dataset, got nil")
	}
	if !strings.Contains(err.Error(), "dataset") {
		t.Errorf("error should mention dataset, got: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestStartRun_InvalidDataset -v`
Expected: FAIL — `StartRun` currently succeeds despite invalid dataset

- [ ] **Step 3: Implement dataset validation in StartRun**

Edit `internal/service/benchmark.go:184-190`. Change the warning to an error:

```go
	if _, statErr := os.Stat(absCandidate); statErr == nil {
		datasetPath = absCandidate
		slog.Info("resolved dataset path", "original", run.Dataset, "resolved", datasetPath)
	} else if run.SuiteID == "" {
		// No suite fallback — dataset must exist as a file.
		slog.Error("dataset not found", "original", run.Dataset, "candidate", absCandidate, "error", statErr)
		run.Status = benchmark.StatusFailed
		_ = s.store.UpdateBenchmarkRun(ctx, run)
		return nil, fmt.Errorf("dataset %q not found: %w", run.Dataset, statErr)
	} else {
		slog.Warn("dataset path resolution failed, relying on suite provider", "original", run.Dataset, "candidate", absCandidate)
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestStartRun_InvalidDataset -v`
Expected: PASS

- [ ] **Step 5: Run all benchmark domain tests**

Run: `cd /workspaces/CodeForge && go test ./internal/domain/benchmark/ -v && go test ./internal/service/ -run Benchmark -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/service/benchmark.go internal/domain/benchmark/benchmark_test.go
git commit -m "fix(benchmark): reject runs with non-existent dataset at API level (Bug 2a)

StartRun now returns an error when the dataset file doesn't exist and
there is no suite/provider fallback. Previously it logged a warning
and continued, leaving the run stuck in 'running' state forever."
```

### Task 2B: Add watchdog goroutine for orphaned runs

**Problem:** Even with validation, if the Python worker crashes or NATS loses a message, runs can get stuck. A background watchdog should periodically scan for `"running"` runs that have exceeded a timeout and mark them `"failed"`.

**Files:**
- Modify: `internal/service/benchmark.go:26-37` (add watchdog fields + method)
- Test: service-level test

- [ ] **Step 1: Write failing test for watchdog**

```go
func TestBenchmarkService_WatchdogMarksStaleRuns(t *testing.T) {
	// Create a run with created_at 20 minutes ago and status "running".
	// After RunWatchdog executes, it should be marked "failed".
	store := &fakeStoreWithStaleRun{
		staleRun: &benchmark.Run{
			ID:        "stale-run-1",
			Status:    benchmark.StatusRunning,
			CreatedAt: time.Now().Add(-20 * time.Minute),
		},
	}
	svc := NewBenchmarkService(store, "")

	ctx := context.Background()
	svc.RunWatchdogOnce(ctx, 15*time.Minute)

	if store.staleRun.Status != benchmark.StatusFailed {
		t.Errorf("expected stale run to be failed, got %q", store.staleRun.Status)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestBenchmarkService_WatchdogMarksStaleRuns -v`
Expected: FAIL — `RunWatchdogOnce` method doesn't exist

- [ ] **Step 3: Implement watchdog**

Add to `internal/service/benchmark.go`:

```go
// RunWatchdogOnce scans for benchmark runs stuck in "running" state longer than
// the given timeout and marks them as "failed". Called periodically by StartWatchdog.
func (s *BenchmarkService) RunWatchdogOnce(ctx context.Context, timeout time.Duration) {
	runs, err := s.store.ListBenchmarkRuns(ctx, &benchmark.RunFilter{Status: string(benchmark.StatusRunning)})
	if err != nil {
		slog.Error("watchdog: failed to list running benchmark runs", "error", err)
		return
	}

	cutoff := time.Now().Add(-timeout)
	for i := range runs {
		r := &runs[i]
		if r.CreatedAt.Before(cutoff) {
			slog.Warn("watchdog: marking stale benchmark run as failed",
				"run_id", r.ID,
				"created_at", r.CreatedAt,
				"age", time.Since(r.CreatedAt).String(),
			)
			r.Status = benchmark.StatusFailed
			r.ErrorMessage = fmt.Sprintf("watchdog timeout: run exceeded %s without completion", timeout)
			if err := s.store.UpdateBenchmarkRun(ctx, r); err != nil {
				slog.Error("watchdog: failed to update stale run", "run_id", r.ID, "error", err)
			}
		}
	}
}

// StartWatchdog launches a background goroutine that periodically checks for
// orphaned runs. Call cancel() on the returned context to stop.
func (s *BenchmarkService) StartWatchdog(interval, timeout time.Duration) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.RunWatchdogOnce(ctx, timeout)
			}
		}
	}()
	return cancel
}
```

- [ ] **Step 4: Wire up watchdog in main startup**

Find where `NewBenchmarkService` is called in `cmd/codeforge/` and add after service creation:

```go
// Start benchmark watchdog: check every 5 minutes, timeout at 15 minutes.
cancelWatchdog := benchSvc.StartWatchdog(5*time.Minute, 15*time.Minute)
defer cancelWatchdog()
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestBenchmarkService_WatchdogMarksStaleRuns -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/service/benchmark.go cmd/codeforge/
git commit -m "fix(benchmark): add watchdog for orphaned runs stuck in 'running' (Bug 2b)

Background goroutine checks every 5 minutes for runs older than 15 minutes
still in 'running' status and marks them 'failed' with a timeout message."
```

---

## Chunk 3: Bug 3 — Invalid Model Silently Succeeds

### Task 3: Validate model existence before creating run

**Problem:** `CreateRunRequest.Validate()` only checks `Model != ""`. LiteLLM may silently fall back to a default model for unknown model names, producing misleading results.

**Fix:** Add model validation in the Python worker's `_handle_benchmark_run()` that checks the model against LiteLLM's `/v1/models` endpoint before executing.

**Files:**
- Modify: `workers/codeforge/consumer/_benchmark.py:72-86` (add validation after request parse)
- Test: `workers/tests/test_model_validation.py` (new)

- [ ] **Step 1: Write failing test for model validation**

Create `workers/tests/test_model_validation.py`:

```python
"""Tests for Bug 3: model validation against LiteLLM."""

from __future__ import annotations

import pytest


class TestValidateModelExists:
    """Verify _validate_model_exists rejects unknown models."""

    @pytest.mark.asyncio
    async def test_known_model_passes(self):
        from codeforge.consumer._benchmark import _validate_model_exists

        # Should not raise for a model in the available list
        await _validate_model_exists("gpt-4", available_models=["gpt-4", "claude-3"])

    @pytest.mark.asyncio
    async def test_unknown_model_raises(self):
        from codeforge.consumer._benchmark import _validate_model_exists

        with pytest.raises(ValueError, match="not available"):
            await _validate_model_exists(
                "nonexistent/model-xyz",
                available_models=["gpt-4", "claude-3"],
            )

    @pytest.mark.asyncio
    async def test_auto_model_skips_validation(self):
        from codeforge.consumer._benchmark import _validate_model_exists

        # "auto" is routed, not validated
        await _validate_model_exists("auto", available_models=["gpt-4"])

    @pytest.mark.asyncio
    async def test_empty_available_list_skips(self):
        from codeforge.consumer._benchmark import _validate_model_exists

        # If we can't fetch models, skip validation (don't block)
        await _validate_model_exists("any-model", available_models=[])
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && poetry run pytest workers/tests/test_model_validation.py -v`
Expected: FAIL — `_validate_model_exists` doesn't exist

- [ ] **Step 3: Implement model validation**

Add to `workers/codeforge/consumer/_benchmark.py` (after the imports, before `_build_evaluators`):

```python
async def _validate_model_exists(
    model: str,
    available_models: list[str] | None = None,
) -> None:
    """Validate that the requested model exists in LiteLLM.

    Raises ValueError if model is not in the available models list.
    Skips validation for 'auto' (routed) or if the model list is empty
    (LiteLLM unreachable — don't block the run).
    """
    if model == "auto":
        return

    if available_models is None:
        available_models = await _fetch_available_models()

    if not available_models:
        # Can't reach LiteLLM /v1/models — skip validation, don't block
        logger.warning("cannot validate model: LiteLLM model list unavailable")
        return

    if model not in available_models:
        raise ValueError(
            f"model {model!r} not available in LiteLLM. "
            f"Available: {', '.join(sorted(available_models)[:10])}"
        )


async def _fetch_available_models() -> list[str]:
    """Fetch model IDs from LiteLLM /v1/models endpoint."""
    import os

    import httpx

    litellm_url = os.environ.get("LITELLM_BASE_URL", "http://localhost:4000")
    api_key = os.environ.get("LITELLM_MASTER_KEY", "sk-codeforge-dev")
    headers = {"Authorization": f"Bearer {api_key}"}

    try:
        async with httpx.AsyncClient(timeout=5.0) as client:
            resp = await client.get(f"{litellm_url}/v1/models", headers=headers)
        if resp.status_code != 200:
            logger.warning("LiteLLM /v1/models returned %d", resp.status_code)
            return []
        data = resp.json()
        return [m.get("id", "") for m in data.get("data", []) if m.get("id")]
    except Exception:
        logger.warning("failed to fetch LiteLLM models", exc_info=True)
        return []
```

Then in `_handle_benchmark_run()`, after `req = BenchmarkRunRequest.model_validate_json(msg.data)` (line 73), add:

```python
            # Validate model exists in LiteLLM (Bug 3 fix).
            await _validate_model_exists(req.model)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge && poetry run pytest workers/tests/test_model_validation.py -v`
Expected: All 4 tests PASS

- [ ] **Step 5: Run existing tests for regressions**

Run: `cd /workspaces/CodeForge && poetry run pytest workers/tests/test_benchmark_runners.py -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add workers/codeforge/consumer/_benchmark.py workers/tests/test_model_validation.py
git commit -m "fix(benchmark): validate model against LiteLLM before execution (Bug 3)

Benchmark runs now check the model name against LiteLLM /v1/models
before starting execution. Unknown models raise ValueError, which
the error handler catches and publishes as status=failed."
```

---

## Chunk 4: Bug 4 — `model=auto` Routing Without CODEFORGE_ROUTING_ENABLED

### Task 4: Return clear error when routing not available for `model=auto`

**Problem:** `_resolve_effective_llm()` at line 152-163 silently falls back to the raw LLM client when `model=auto` but routing is not enabled. This passes `"auto"` as a literal model name to LiteLLM, which either fails cryptically or falls back to an unintended model.

**Fix:** When `model=auto` and the router is None, raise a clear error instead of falling back.

**Files:**
- Modify: `workers/codeforge/consumer/_benchmark.py:152-163`
- Test: add to `workers/tests/test_model_validation.py`

- [ ] **Step 1: Write failing test**

Add to `workers/tests/test_model_validation.py`:

```python
class TestResolveEffectiveLlm:
    """Verify _resolve_effective_llm fails for model=auto without routing."""

    @pytest.mark.asyncio
    async def test_auto_without_router_raises(self):
        """model=auto must fail if HybridRouter is not available."""
        from unittest.mock import AsyncMock, MagicMock, patch

        from codeforge.consumer._benchmark import BenchmarkHandlerMixin

        mixin = BenchmarkHandlerMixin.__new__(BenchmarkHandlerMixin)
        mixin._llm = MagicMock()

        req = MagicMock()
        req.model = "auto"
        log = MagicMock()

        with patch.object(mixin, "_get_hybrid_router", new_callable=AsyncMock, return_value=None):
            with pytest.raises(ValueError, match="routing.*not.*enabled"):
                await mixin._resolve_effective_llm(req, log)

    @pytest.mark.asyncio
    async def test_non_auto_returns_llm_directly(self):
        """Non-auto models should return the raw LLM client."""
        from unittest.mock import MagicMock

        from codeforge.consumer._benchmark import BenchmarkHandlerMixin

        mixin = BenchmarkHandlerMixin.__new__(BenchmarkHandlerMixin)
        mixin._llm = MagicMock()

        req = MagicMock()
        req.model = "lm_studio/qwen3-30b-a3b"
        log = MagicMock()

        result = await mixin._resolve_effective_llm(req, log)
        assert result is mixin._llm
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && poetry run pytest workers/tests/test_model_validation.py::TestResolveEffectiveLlm -v`
Expected: FAIL — `_resolve_effective_llm` currently returns raw LLM silently

- [ ] **Step 3: Fix `_resolve_effective_llm()`**

Edit `workers/codeforge/consumer/_benchmark.py:152-163`:

```python
    async def _resolve_effective_llm(self, req: object, log: structlog.BoundLogger) -> object:
        """Resolve the effective LLM client, wrapping with router for auto mode."""
        if req.model != "auto":
            return self._llm
        try:
            router = await self._get_hybrid_router()
            if router is not None:
                log.info("auto-routing enabled for benchmark run")
                return _RoutingLLMWrapper(self._llm, router)
        except Exception:
            log.warning("HybridRouter initialization failed", exc_info=True)

        raise ValueError(
            "model='auto' requires intelligent routing, but routing is not enabled. "
            "Set CODEFORGE_ROUTING_ENABLED=true or specify an explicit model name."
        )
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge && poetry run pytest workers/tests/test_model_validation.py -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add workers/codeforge/consumer/_benchmark.py workers/tests/test_model_validation.py
git commit -m "fix(benchmark): fail fast when model=auto but routing not enabled (Bug 4)

_resolve_effective_llm now raises ValueError instead of silently
falling back to the raw LLM when model='auto' and HybridRouter
is not available. The error handler publishes status=failed with
a clear message."
```

---

## Chunk 5: Bug 5 — LLM Judge Context Overflow with Local Models

### Task 5A: Create prompt compression utility

**Problem:** The LLM Judge sends task input + model output + grading rubric to the LLM. With local models (8K-32K context), this often exceeds the context limit, causing a 400 error and `score: 0`.

**Fix:** Add prompt compression that truncates inputs to fit within the model's context window before sending to the evaluator.

**Files:**
- Create: `workers/codeforge/evaluation/evaluators/prompt_compressor.py`
- Test: `workers/tests/test_prompt_compressor.py` (new)

- [ ] **Step 1: Write failing test for prompt compressor**

Create `workers/tests/test_prompt_compressor.py`:

```python
"""Tests for Bug 5: prompt compression for local model context limits."""

from __future__ import annotations

import pytest

from codeforge.evaluation.evaluators.prompt_compressor import compress_for_context


class TestPromptCompressor:
    def test_short_input_unchanged(self):
        """Inputs within budget are returned unchanged."""
        result = compress_for_context(
            text="Hello world",
            max_chars=1000,
        )
        assert result == "Hello world"

    def test_long_input_truncated(self):
        """Inputs exceeding budget are truncated with marker."""
        long_text = "x" * 5000
        result = compress_for_context(text=long_text, max_chars=500)
        assert len(result) <= 500
        assert "[truncated]" in result

    def test_preserves_head_and_tail(self):
        """Truncation keeps beginning and end of text."""
        text = "START " + "m" * 5000 + " END"
        result = compress_for_context(text=text, max_chars=200)
        assert result.startswith("START")
        assert result.endswith("END")

    def test_zero_max_returns_empty(self):
        """Zero budget returns empty string."""
        result = compress_for_context(text="hello", max_chars=0)
        assert result == ""

    def test_negative_max_returns_empty(self):
        result = compress_for_context(text="hello", max_chars=-1)
        assert result == ""
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && poetry run pytest workers/tests/test_prompt_compressor.py -v`
Expected: FAIL — module doesn't exist

- [ ] **Step 3: Implement prompt compressor**

Create `workers/codeforge/evaluation/evaluators/prompt_compressor.py`:

```python
"""Prompt compression for evaluators running against context-limited models.

Uses head-and-tail truncation: keeps the beginning (problem statement) and
end (final answer) of text, removing the middle (verbose intermediate output).
"""

from __future__ import annotations

_TRUNCATION_MARKER = "\n\n[truncated — middle section removed to fit context window]\n\n"


def compress_for_context(text: str, max_chars: int) -> str:
    """Truncate text to fit within max_chars using head+tail strategy.

    Returns the original text if it fits. Otherwise, keeps the first ~60%
    and last ~40% of the budget (minus marker length), preserving the
    problem statement at the start and the final answer at the end.
    """
    if max_chars <= 0:
        return ""
    if len(text) <= max_chars:
        return text

    marker_len = len(_TRUNCATION_MARKER)
    available = max_chars - marker_len
    if available <= 0:
        return text[:max_chars]

    head_size = int(available * 0.6)
    tail_size = available - head_size

    return text[:head_size] + _TRUNCATION_MARKER + text[-tail_size:]
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge && poetry run pytest workers/tests/test_prompt_compressor.py -v`
Expected: All 5 tests PASS

- [ ] **Step 5: Commit**

```bash
git add workers/codeforge/evaluation/evaluators/prompt_compressor.py workers/tests/test_prompt_compressor.py
git commit -m "feat(benchmark): add prompt compressor for context-limited models (Bug 5a)"
```

### Task 5B: Integrate compression into LLM Judge evaluator

**Problem:** `evaluate_correctness()` in `metrics.py` passes full `user_input`, `actual_output`, and `expected_output` to DeepEval's G-Eval. These can exceed the context window.

**Files:**
- Modify: `workers/codeforge/evaluation/evaluators/llm_judge.py:49-69`
- Modify: `workers/codeforge/evaluation/metrics.py:27-51`

- [ ] **Step 1: Write failing test**

Add to `workers/tests/test_prompt_compressor.py`:

```python
class TestLLMJudgeCompression:
    """Verify LLM Judge truncates inputs before evaluation."""

    @pytest.mark.asyncio
    async def test_long_output_is_compressed(self):
        """LLM Judge should compress long actual_output before calling G-Eval."""
        from unittest.mock import AsyncMock, patch

        from codeforge.evaluation.evaluators.llm_judge import LLMJudgeEvaluator
        from codeforge.evaluation.providers.base import ExecutionResult, TaskSpec

        evaluator = LLMJudgeEvaluator(judge=None, metrics=["correctness"])

        task = TaskSpec(
            id="t1",
            name="long_task",
            input="Write hello world" * 100,
            expected_output="def hello(): return 'hello'" * 100,
        )
        result = ExecutionResult(
            actual_output="x" * 50_000,  # 50K chars — should be compressed
        )

        # Mock _run_metric to capture what inputs it receives
        with patch.object(evaluator, "_run_metric", new_callable=AsyncMock, return_value=0.5) as mock_run:
            dims = await evaluator.evaluate(task, result)
            assert len(dims) == 1
            assert dims[0].score == 0.5
            # The method was called — actual compression happens inside _run_metric → metrics.py
            mock_run.assert_called_once()
```

- [ ] **Step 2: Add compression to `LLMJudgeEvaluator.evaluate()`**

Edit `workers/codeforge/evaluation/evaluators/llm_judge.py`. Add import and compression before the metric loop:

```python
from codeforge.evaluation.evaluators.prompt_compressor import compress_for_context

# Max chars for LLM judge inputs — conservative for local models (8K context ≈ 6K chars).
_MAX_INPUT_CHARS = 4000
_MAX_OUTPUT_CHARS = 4000
_MAX_EXPECTED_CHARS = 2000
```

In `evaluate()` method, before the `for metric_name` loop, compress the inputs:

```python
        # Compress inputs for context-limited models (Bug 5 fix).
        compressed_task = TaskSpec(
            id=task.id,
            name=task.name,
            input=compress_for_context(task.input, _MAX_INPUT_CHARS),
            expected_output=compress_for_context(task.expected_output, _MAX_EXPECTED_CHARS),
            context=task.context,
            expected_tools=task.expected_tools,
        )
        compressed_result = ExecutionResult(
            actual_output=compress_for_context(result.actual_output, _MAX_OUTPUT_CHARS),
            tool_calls=result.tool_calls,
            cost_usd=result.cost_usd,
            tokens_in=result.tokens_in,
            tokens_out=result.tokens_out,
            duration_ms=result.duration_ms,
            files_changed=result.files_changed,
            test_output=result.test_output,
        )
```

Then use `compressed_task` and `compressed_result` in `_run_metric()` calls instead of `task` and `result`.

- [ ] **Step 3: Add compression to trajectory verifier**

Edit `workers/codeforge/evaluation/evaluators/trajectory_verifier.py:86-95`. Compress inputs before building the prompt:

```python
from codeforge.evaluation.evaluators.prompt_compressor import compress_for_context

_MAX_TRAJECTORY_CHARS = 4000
_MAX_TASK_INPUT_CHARS = 2000
```

In `evaluate()`, compress before formatting:

```python
        compressed_input = compress_for_context(task.input, _MAX_TASK_INPUT_CHARS)
        compressed_expected = compress_for_context(task.expected_output, 1000)
        compressed_trajectory = compress_for_context(trajectory_text, _MAX_TRAJECTORY_CHARS)

        prompt = _VERIFIER_PROMPT.format(
            task_input=compressed_input,
            expected_output=compressed_expected or "N/A",
            trajectory=compressed_trajectory,
            files_changed="\n".join(result.files_changed) or "None",
            test_output=compress_for_context(result.test_output, 2000) or "N/A",
        )
```

- [ ] **Step 4: Update error fallback to include structured error info**

In `llm_judge.py` line 66-68, enhance the error dimension:

```python
            except Exception as exc:
                error_msg = str(exc)
                is_context_overflow = "context" in error_msg.lower() or "400" in error_msg
                logger.exception(
                    "llm_judge metric failed",
                    metric=metric_name,
                    task_id=task.id,
                    error=error_msg,
                    context_overflow=is_context_overflow,
                )
                dimensions.append(EvalDimension(
                    name=metric_name,
                    score=0.0,
                    details={
                        "error": "context_overflow" if is_context_overflow else "evaluation_failed",
                        "error_message": error_msg[:200],
                    },
                ))
```

- [ ] **Step 5: Run all evaluator tests**

Run: `cd /workspaces/CodeForge && poetry run pytest workers/tests/test_prompt_compressor.py workers/tests/test_evaluation_pipeline.py -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add workers/codeforge/evaluation/evaluators/llm_judge.py \
       workers/codeforge/evaluation/evaluators/trajectory_verifier.py \
       workers/codeforge/evaluation/evaluators/prompt_compressor.py \
       workers/tests/test_prompt_compressor.py
git commit -m "fix(benchmark): compress evaluator prompts for local models (Bug 5)

LLM Judge and Trajectory Verifier now compress inputs using head+tail
truncation before sending to the LLM. This prevents context overflow
errors with local models (8K-32K context windows). Error responses
now include structured error info (context_overflow vs evaluation_failed)."
```

---

## Chunk 6: Final Integration & E2E Test Update

### Task 6: Update E2E tests to use stricter assertions

Now that the bugs are fixed, the E2E test workarounds can be tightened.

**Files:**
- Modify: `frontend/e2e/benchmark-validation/block-5-errors.spec.ts`
- Modify: `frontend/e2e/benchmark-validation/block-3-agent.spec.ts`

- [ ] **Step 1: Tighten Block 5 error assertions**

In `block-5-errors.spec.ts`:

Test 5.1 (invalid dataset): Should now get 400/404 from API (not 201 + stuck run):
```typescript
// Old: if (status === 201) { expect(["failed", "running"]).toContain(...) }
// New: API should reject at creation time
expect([400, 404]).toContain(status);
```

Test 5.2 (invalid model): Should now get `status=failed` from worker:
```typescript
if (status === 201) {
  const finalRun = await waitForRunCompletion(runBody.id, 120_000);
  expect(finalRun.status).toBe("failed");
  expect(finalRun.error_message).toBeTruthy();
}
```

Test 5.4 (unknown evaluator): Remains the same (graceful degradation is correct behavior).

- [ ] **Step 2: Tighten Block 3 agent assertions for score keys**

In `block-3-agent.spec.ts`, re-enable per-metric assertions since score keys now match:
```typescript
for (const metric of tc.metrics) {
  for (const r of results) {
    expect(
      r.scores?.[metric] !== undefined,
      `Missing aggregated score key '${metric}' for task ${r.task_id}`
    ).toBe(true);
    expect(r.scores[metric]).toBeGreaterThanOrEqual(0);
  }
}
```

- [ ] **Step 3: Tighten Block 4 routing assertion**

In `block-4-routing.spec.ts`, `model=auto` without routing should now fail at the worker level with a clear error:
```typescript
expect(
  ["completed", "failed"].includes(finalRun.status),
  `Routing run stuck in ${finalRun.status}`,
).toBe(true);

if (finalRun.status === "failed") {
  // Should have a clear error message about routing
  expect(finalRun.error_message).toContain("routing");
}
```

- [ ] **Step 4: Run E2E tests (Block 0 prerequisites only — fast validation)**

Run: `cd /workspaces/CodeForge/frontend && npx playwright test block-0 --config=playwright.llm.config.ts`
Expected: PASS

- [ ] **Step 5: Update FINDINGS.md status**

Mark all bugs as "Fixed" in `frontend/e2e/benchmark-validation/FINDINGS.md`.

- [ ] **Step 6: Commit**

```bash
git add frontend/e2e/benchmark-validation/block-5-errors.spec.ts \
       frontend/e2e/benchmark-validation/block-3-agent.spec.ts \
       frontend/e2e/benchmark-validation/block-4-routing.spec.ts \
       frontend/e2e/benchmark-validation/FINDINGS.md
git commit -m "test(benchmark): tighten E2E assertions after bug fixes

Block 3: Re-enable per-metric score key assertions (Bug 1 fixed).
Block 4: Expect clear routing error message (Bug 4 fixed).
Block 5: Expect API rejection for invalid datasets (Bug 2 fixed),
         expect failed status for invalid models (Bug 3 fixed)."
```

---

## Summary

| Task | Bug | Severity | Files Changed | Tests Added |
|------|-----|----------|---------------|-------------|
| 1 | Score Key Mismatch | Medium | `_benchmark.py` | `test_score_key_normalization.py` (8 tests) |
| 2A | Stuck Runs — Dataset Validation | High | `benchmark.go` | `benchmark_test.go` (1 test) |
| 2B | Stuck Runs — Watchdog | High | `benchmark.go`, `main.go` | service test (1 test) |
| 3 | Invalid Model | Medium | `_benchmark.py` | `test_model_validation.py` (4 tests) |
| 4 | model=auto Without Routing | Low | `_benchmark.py` | `test_model_validation.py` (2 tests) |
| 5A | Prompt Compressor | Low | `prompt_compressor.py` | `test_prompt_compressor.py` (5 tests) |
| 5B | Integration | Low | `llm_judge.py`, `trajectory_verifier.py` | `test_prompt_compressor.py` (1 test) |
| 6 | E2E Test Tightening | — | 3 spec files, FINDINGS.md | — |

**Total: 7 implementation tasks + 1 integration task, ~22 new tests**

**Execution order:** Tasks 1-5 are independent and can be parallelized. Task 6 depends on all previous tasks being complete.

**IMPORTANT:** `DEFAULT_MODEL = "lm_studio/qwen3-30b-a3b"` — NEVER change this value.
