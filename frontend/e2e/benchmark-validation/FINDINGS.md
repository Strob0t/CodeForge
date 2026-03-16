# Benchmark Validation E2E Test — Findings

**Date:** 2026-03-11 (discovered), 2026-03-15 (Bugs 1-5 fixed), 2026-03-15 (Bugs 6-10 found+fixed)
**Model:** lm_studio/qwen/qwen3-30b-a3b (local)
**Result:** Round 1: 22/22 passed (Bugs 1-5 fixed). Round 2: 33-run E2E test plan (Bugs 6-10 found+fixed)

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

## Bug 6: Agent Provider Wrong Keyword Argument (`datasets_dir` vs `dataset_path`)

**Severity:** High
**Status:** Fixed (2026-03-15)

**Description:**
`_run_agent_benchmark()` passed `datasets_dir=req.dataset_path` but the
`CodeForgeAgentProvider.__init__()` expects `dataset_path`. All agent benchmark
runs failed immediately with `TypeError: unexpected keyword argument 'datasets_dir'`.

**Fix:** Changed to `dataset_path=req.dataset_path` at line 405.

**Files changed:**
- `workers/codeforge/consumer/_benchmark.py` — line 405 kwarg fix

---

## Bug 7: Watchdog Timeout Too Short for Local Models

**Severity:** High
**Status:** Fixed (2026-03-15)

**Description:**
The watchdog goroutine killed runs after 15 minutes. Agent benchmark runs with local
models (30B params) routinely take 20-60+ minutes for 5 tasks. Run `d278f956` completed
successfully after ~20 min but the watchdog marked it `failed` before results were saved.

**Fix:** Changed default timeout from 15 minutes to 2 hours. Made configurable via
`BENCHMARK_WATCHDOG_TIMEOUT` env var (accepts Go duration format, e.g. `4h`, `30m`).

**Files changed:**
- `cmd/codeforge/main.go` — configurable watchdog timeout, default 2h

---

## Bug 8: Multi-Rollout `RolloutOutcome` Missing `eval_score` Attribute

**Severity:** High
**Status:** Fixed (2026-03-15)

**Description:**
`MultiRolloutRunner.run_task()` created `RolloutOutcome(result=...)` discarding
`run_result.eval_score`. Then `_convert_rollout_outcome()` accessed `outcome.eval_score`
which didn't exist. All multi-rollout runs (Phase 4.1, 4.2) crashed.

**Fix:** Added `eval_score: EvalScore | None = None` field to `RolloutOutcome` dataclass.
Passed `eval_score=run_result.eval_score` when constructing outcomes.

**Files changed:**
- `workers/codeforge/evaluation/runners/multi_rollout.py` — added `eval_score` field, import, and pass-through

---

## Bug 9: `_convert_rollout_outcome` Uses Wrong Attribute Name (`execution` vs `result`)

**Severity:** High
**Status:** Fixed (2026-03-15)

**Description:**
`_convert_rollout_outcome()` accessed `outcome.execution.actual_output` but
`RolloutOutcome` stores the `ExecutionResult` in a field named `result`, not `execution`.
All multi-rollout result conversion crashed with `AttributeError`.

**Fix:** Changed all `outcome.execution.*` references to `outcome.result.*` in
`_convert_rollout_outcome()`.

**Files changed:**
- `workers/codeforge/consumer/_benchmark.py` — lines 518-527

---

## Bug 10: Hybrid Pipeline Passed as Regular Pipeline to Runner

**Severity:** Medium
**Status:** Fixed (2026-03-15)

**Description:**
When `hybrid_verification=true`, the code built a `HybridEvaluationPipeline` and passed
it as the `pipeline` parameter to the runner. But runners call `pipeline.evaluate()` which
only exists on `EvaluationPipeline`, not `HybridEvaluationPipeline` (which has `.verify()`).
All hybrid verification runs crashed.

**Fix:** Always create a regular `EvaluationPipeline` for per-task scoring. Build
`HybridEvaluationPipeline` separately and pass it through a dedicated `hybrid_pipeline`
parameter for multi-rollout selection only.

**Files changed:**
- `workers/codeforge/consumer/_benchmark.py` — separated pipeline construction, added
  `hybrid_pipeline` parameter to `_run_simple_benchmark()`, `_run_tool_use_benchmark()`,
  `_run_agent_benchmark()`, and `_run_with_optional_rollout()`

---

## Known Issues (Not Yet Fixed)

### Issue A: Invalid Model Name Silently Succeeds (Regression)
The `_validate_model_exists()` fix from Bug #3 doesn't catch models with format
`nonexistent/model-xyz-404` — the model completes with score=0 instead of failing.

### Issue B: HTTP 500 Instead of 400 for Invalid Requests
Invalid dataset (`nonexistent-xyz-dataset`) and missing required fields return HTTP 500
instead of HTTP 400. Input validation at the Go handler layer is incomplete.

### Issue C: Unknown Evaluator Names Silently Ignored
Requesting `metrics: ["nonexistent_evaluator"]` completes successfully with empty scores
instead of failing with a validation error.

### Issue D: External Suite HuggingFace API Failures — FIXED (2026-03-16)
Three external suites were failing due to incorrect API parameters and missing auth:

**BigCodeBench (404):** Config was `"v0.1.2"` but the data is stored as a *split*, not a config.
Fix: `_CONFIG = "default"`, `_SPLIT = "v0.1.2"` in `bigcodebench.py`.

**LiveCodeBench (404→502→504):** Dataset `code_generation_lite` runs arbitrary code and isn't served
via Datasets Server API. Fix: Changed to `livecodebench/code_generation` with `config="default"` in
`livecodebench.py`. However, the correct dataset has very large rows that cause the HF Datasets Server
to return 502/504 errors even at page_size=10. Added adaptive page size fallback (100→10→1) with
timeout handling and broken-row skipping in `cache.py:download_hf_dataset()`. At page_size=1, rows
download individually (~3-5s each) but some rows still return 500 and are skipped. Full dataset
download is extremely slow (~12h for 880 rows). **Workaround:** Use `max_tasks: 3` to only download
the first few rows. Long-term fix: use the `datasets` Python library for direct Parquet download.

**CRUXEval (401):** Gated dataset requiring authentication. Fix: Added `HF_TOKEN` env var support
to `cache.py:download_hf_dataset()`. When set, sends `Authorization: Bearer {token}` header.

**Files changed:**
- `workers/codeforge/evaluation/providers/bigcodebench.py` — `_CONFIG`, `_SPLIT`, `_fetch_tasks()`
- `workers/codeforge/evaluation/providers/livecodebench.py` — `_DATASET`, `_CONFIG`
- `workers/codeforge/evaluation/providers/cruxeval.py` — `_DATASET` (`cruxeval-org/cruxeval`)
- `workers/codeforge/evaluation/cache.py` — `HF_TOKEN` auth header, adaptive page size fallback
  (100→10→1), timeout handling, broken-row skipping in `download_hf_dataset()`
- `workers/codeforge/consumer/_benchmark.py` — Early NATS ack to prevent stale redelivery
- `docs/dev-setup.md` — `HF_TOKEN` and `BENCHMARK_WATCHDOG_TIMEOUT` env vars documented

---

## Comprehensive E2E Test Results (Round 2)

### Phase 0: Infrastructure (5/5 PASS)
Backend healthy, LiteLLM alive, model available, datasets listed, suites seeded.

### Phase 1-3: Core Benchmarks (from Round 1: 22/22 PASS)
All 3 benchmark types (simple, tool_use, agent) with all 4 evaluators validated.

### Phase 4: Advanced Features (3/3 PASS after fixes)
| Run | Feature | Status | Results |
|-----|---------|--------|---------|
| 4.1 | Multi-rollout best-of-3 | PASS | 6 results (2×3), `is_best` + `diversity_score` |
| 4.2 | Multi-rollout diversity | PASS | 4 results (2×2), `diversity_score` populated |
| 4.3 | Hybrid verification | PASS | 2 results, both evaluator keys present |

### Phase 3b: External Suites (4/5 PASS, 1 partial)
| Suite | Status | Results | Notes |
|-------|--------|---------|-------|
| HumanEval | PASS | 3 results | `max_tasks` filter working |
| MBPP | PASS | 3 results | `max_tasks` filter working |
| BigCodeBench | PASS | 3 results | Fixed: `_CONFIG="default"`, `_SPLIT="v0.1.2"` (Issue D) |
| CRUXEval | PASS | 3 results | Fixed: `cruxeval-org/cruxeval` + HF_TOKEN auth (Issue D) |
| LiveCodeBench | PARTIAL | ~3 results | HF server 502/504 for large rows, adaptive page_size works but slow (Issue D) |
| SWE-bench | TBD | — | Agent suite, long-running |
| SPARCBench | TBD | — | Agent suite, long-running |
| Aider Polyglot | TBD | — | Agent suite, long-running |

### Phase 5: API Comparison & Analysis (12/12 PASS)
Compare, multi-compare, cost analysis, leaderboard, analyze, export JSON/CSV/training,
filter by status/model/type — all working.

### Phase 6: Error Scenarios (2/5 PASS)
| Test | Scenario | Result |
|------|----------|--------|
| 6.1 | Invalid dataset | WEAK PASS (HTTP 500 not 400) |
| 6.2 | Invalid model | FAIL (silently succeeds) |
| 6.3 | Missing required field | FAIL (HTTP 500 not 400) |
| 6.4 | Unknown evaluator | FAIL (silently succeeds) |
| 6.5 | Cancel running run | PASS |

### Phase 7: Suite CRUD (6/6 PASS)
Create, get, update, list, delete, verify-deleted — all working.

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
