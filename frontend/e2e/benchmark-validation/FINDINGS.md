# Benchmark Validation E2E Test — Findings

**Date:** 2026-03-11 (discovered), 2026-03-15 (all fixed)
**Model:** lm_studio/qwen3-30b-a3b (local)
**Result:** 22/22 passed, all 5 bugs fixed

## Summary

The benchmark validation E2E test suite exercises the full pipeline:
Go Core -> NATS JetStream -> Python Worker -> LiteLLM -> LM Studio -> Evaluators -> DB -> API

**22 tests across 6 blocks** validate all benchmark types (simple, tool_use, agent),
evaluator combinations, routing, and error handling.

---

## Bug 1: Score Key Names Don't Match Metric Request Names

**Severity:** Medium
**Status:** Fixed (2026-03-15)

**Description:**
When requesting metrics like `llm_judge`, `trajectory_verifier`, or `sparc`, the API
returned scores under different key names (e.g., `correctness` instead of `llm_judge`).

**Fix:** Added `_aggregate_metric_scores()` to `workers/codeforge/consumer/_benchmark.py`.
After building raw dimension scores, aggregated metric-level keys are added as averages
of their dimension group. Raw dimension keys are preserved for detailed analysis.

**Files changed:**
- `workers/codeforge/consumer/_benchmark.py` — `_DIMENSION_TO_METRIC` mapping, `_aggregate_metric_scores()` helper
- `workers/tests/test_score_key_normalization.py` — 16 tests

---

## Bug 2: Error Runs Stay "running" Instead of Transitioning to "failed"

**Severity:** High
**Status:** Fixed (2026-03-15) — three-tier fix

**Description:**
Benchmark runs with invalid parameters stayed in `"running"` status indefinitely.

**Fix (2A):** `StartRun()` now returns an error when dataset path resolution fails and
no suite fallback exists. The run is marked `"failed"` with an `ErrorMessage` before
the NATS message is published. Added `ErrorMessage` field to the `Run` struct with
DB migration.

**Fix (2B):** Added a watchdog goroutine (`RunWatchdogOnce` + `StartWatchdog`) that
scans every 5 minutes for runs stuck in `"running"` longer than 15 minutes and marks
them `"failed"` with a timeout message.

**Files changed:**
- `internal/domain/benchmark/benchmark.go` — `ErrorMessage` field
- `internal/service/benchmark.go` — dataset validation in `StartRun()`, watchdog methods
- `internal/adapter/postgres/store_benchmark.go` — SQL queries updated
- `internal/adapter/postgres/migrations/072_benchmark_run_error_message.sql` — migration
- `internal/service/benchmark_test.go` — 5 new tests (2 StartRun + 3 watchdog)
- `cmd/codeforge/main.go` — watchdog wiring

---

## Bug 3: Invalid Model Silently Succeeds

**Severity:** Medium
**Status:** Fixed (2026-03-15)

**Description:**
A benchmark run with an invalid model name was accepted and completed with results
because LiteLLM silently fell back to a default model.

**Fix:** Added `_validate_model_exists()` that checks the model against LiteLLM's
`/v1/models` endpoint before execution. Unknown models raise `ValueError`, caught
by the existing error handler and published as `status=failed`.

**Files changed:**
- `workers/codeforge/consumer/_benchmark.py` — `_validate_model_exists()`, `_fetch_available_models()`
- `workers/tests/test_model_validation.py` — 6 tests

---

## Bug 4: `model=auto` Routing Without CODEFORGE_ROUTING_ENABLED

**Severity:** Low
**Status:** Fixed (2026-03-15)

**Description:**
When `model=auto` was sent without routing enabled, `_resolve_effective_llm()` silently
fell back to the raw LLM client, passing `"auto"` literally to LiteLLM.

**Fix:** `_resolve_effective_llm()` now raises `ValueError` when `model=auto` and the
HybridRouter is not available. The error message instructs users to enable routing or
specify an explicit model name.

**Files changed:**
- `workers/codeforge/consumer/_benchmark.py` — `_resolve_effective_llm()` fix
- `workers/tests/test_model_validation.py` — 2 additional tests

---

## Bug 5: LLM Judge Context Overflow with Local Models

**Severity:** Low
**Status:** Fixed (2026-03-15)

**Description:**
The LLM Judge and Trajectory Verifier exceeded local model context limits (8K-32K),
causing 400 errors and `score: 0` for all evaluations.

**Fix:** Added prompt compression using head+tail truncation (`compress_for_context()`).
Inputs are compressed to fit within conservative budgets (4K chars for input/output,
2K for expected output). Uses `model_copy(update=...)` to preserve all Pydantic fields.
Error fallback now distinguishes `context_overflow` from `evaluation_failed`.

**Files changed:**
- `workers/codeforge/evaluation/evaluators/prompt_compressor.py` — new compression utility
- `workers/codeforge/evaluation/evaluators/llm_judge.py` — compression integration + enhanced error fallback
- `workers/codeforge/evaluation/evaluators/trajectory_verifier.py` — compression integration
- `workers/tests/test_prompt_compressor.py` — 18 tests

---

## Test Architecture Decisions

### External Suite Exclusion
External suites (humaneval, mbpp, bigcodebench, cruxeval, livecodebench, swebench,
sparcbench, aider_polyglot) have 100-1000+ tasks each. At ~4 min/task with LM Studio,
a single humaneval run would take ~11 hours. These are validated for **registration only**
in Block 0 (difficulty audit). Execution testing uses the `e2e-quick` dataset (2 tasks).

### Dataset: e2e-quick
A minimal 2-task dataset (`configs/benchmarks/e2e-quick.yaml`) created specifically for
E2E testing. Tasks are trivial Python functions (hello world, add two numbers) to minimize
LLM processing time while still exercising the full pipeline.

### Assertion Strategy
Tests validate the **pipeline** (run created -> tasks processed -> scores produced ->
status completed), not **model quality** (score values). With local models, scores are
often 0 due to context limitations — this is expected and acceptable for pipeline validation.
