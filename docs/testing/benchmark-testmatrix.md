# Benchmark System — Full Validation Test Matrix

## Overview

Comprehensive test matrix for the CodeForge benchmark evaluation system.
Validates all benchmark types, metric combinations, routing, and error handling.

**Model Strategy:**
- **1 test** uses `model=auto` (intelligent routing via HybridRouter)
- **All other tests** use `lm_studio/qwen/qwen3-30b-a3b` (local model)

**Dataset:** `e2e-quick` (2 tasks) for fast validation; `basic-coding` (5 tasks) for data-backed runs.

## Prerequisites

```bash
# 1. Docker services
docker compose up -d postgres nats litellm

# 2. Go backend (dev mode)
APP_ENV=development NATS_URL=nats://<nats-ip>:4222 go run ./cmd/codeforge/

# 3. Python worker (correct env var!)
cd workers && NATS_URL=nats://<nats-ip>:4222 \
  LITELLM_BASE_URL=http://<litellm-ip>:4000 \
  APP_ENV=development \
  LITELLM_MASTER_KEY=sk-codeforge-dev \
  python -m codeforge.consumer

# 4. Frontend (optional, for browser tests)
cd frontend && npm run dev
```

**Important:** Use `LITELLM_BASE_URL` (not `LITELLM_URL`) for the Python worker.

## Test Matrix

### Block 0: Prerequisites (5 tests)

| ID  | Test | Expected Result |
|-----|------|-----------------|
| 0.1 | Backend healthy + dev mode | `/health` → `ok`, `dev_mode: true` |
| 0.2 | LiteLLM proxy reachable | `/health/liveliness` → 200 |
| 0.3 | Default model available | `lm_studio/qwen/qwen3-30b-a3b` in `/v1/models` |
| 0.4 | Suites registered | `codeforge_simple`, `codeforge_tool_use`, `codeforge_agent` exist |
| 0.5 | Datasets discoverable | `e2e-quick` in `/benchmarks/datasets` |

### Block 1: Simple Benchmarks (3 tests, ~4 min each)

| ID  | Metrics | Expected Result |
|-----|---------|-----------------|
| 1.1 | `llm_judge` | Status: completed, scores ≥ 0 |
| 1.2 | `functional_test` | Graceful degradation (score=0, no crash) |
| 1.3 | `llm_judge + functional_test` | Both evaluators produce scores |

### Block 2: Tool-Use Benchmarks (3 tests, ~4 min each)

| ID  | Metrics | Expected Result |
|-----|---------|-----------------|
| 2.1 | `llm_judge` | Status: completed, tool calls scored |
| 2.2 | `functional_test` | Scores present (may be 0) |
| 2.3 | `llm_judge + functional_test` | Both evaluators produce scores |

### Block 3: Agent Benchmarks (3 tests, ~6 min each)

| ID  | Metrics | Expected Result |
|-----|---------|-----------------|
| 3.1 | `llm_judge + trajectory_verifier` | Agent loop runs, trajectory scored |
| 3.2 | `llm_judge + sparc` | SPARC multi-dimensional scoring |
| 3.3 | All 4 evaluators | All evaluator scores present |

### Block 4: Intelligent Routing (1 test, ~4 min)

| ID  | Model | Expected Result |
|-----|-------|-----------------|
| 4.1 | `auto` | HybridRouter selects model, run completes OR fails with clear routing error |

### Block 5: Error Scenarios (5 tests, < 1 min each)

| ID  | Scenario | Expected Result |
|-----|----------|-----------------|
| 5.1 | Invalid dataset | HTTP 400 or failed run with error message |
| 5.2 | Invalid model | Failed run, error contains "model" |
| 5.3 | Empty dataset | Graceful failure, no infinite loop |
| 5.4 | Unknown evaluator | HTTP 400 (Go validation rejects) |
| 5.5 | Duplicate run | Both runs complete independently |

### Block 6: Multi-Metric & Extended Names (3 tests, ~4 min each)

| ID  | Metrics | Expected Result |
|-----|---------|-----------------|
| 6.1 | `correctness + faithfulness` | Both LLM judge dimensions scored |
| 6.2 | `correctness + tool_correctness + answer_relevancy` | Extended names accepted |
| 6.3 | `correctness + contextual_precision + faithfulness` | All 3 metrics produce scores |

## Summary

| Block | Tests | Model | Est. Duration |
|-------|-------|-------|---------------|
| 0 Prerequisites | 5 | — | < 1 min |
| 1 Simple | 3 | qwen3-30b-a3b | ~12 min |
| 2 Tool-Use | 3 | qwen3-30b-a3b | ~12 min |
| 3 Agent | 3 | qwen3-30b-a3b | ~18 min |
| 4 Routing | 1 | auto | ~4 min |
| 5 Errors | 5 | qwen3-30b-a3b | ~3 min |
| 6 Multi-Metric | 3 | qwen3-30b-a3b | ~12 min |
| **Total** | **23** | | **~60 min** |

## Running the Tests

```bash
# Run the full validation suite
cd frontend && npx playwright test e2e/benchmark-validation/full-validation.spec.ts \
  --config=e2e/benchmark-validation/playwright.validation.config.ts

# Run a specific block only
npx playwright test e2e/benchmark-validation/full-validation.spec.ts \
  --config=e2e/benchmark-validation/playwright.validation.config.ts \
  -g "Block 4"
```

## Bugs Fixed (2026-03-18)

| # | Bug | Fix | File |
|---|-----|-----|------|
| 1 | Error message not stored in DB | Store `payload.Error` in `run.ErrorMessage` | `internal/service/benchmark.go:832` |
| 2 | Worker env var mismatch | Use `LITELLM_BASE_URL` (not `LITELLM_URL`) | Operational |
| 3 | `tool_correctness` etc. not recognized | Add to `llm_judge_metrics` set | `workers/codeforge/consumer/_benchmark.py:724` |
| 4 | LiteLLMJudge hardcoded Docker URL | Read from `LITELLM_BASE_URL` env var | `workers/codeforge/evaluation/litellm_judge.py:29` |
| 5 | `json_object` format rejected by LM Studio | Switch to `json_schema` with actual schema | `workers/codeforge/evaluation/litellm_judge.py:53` |
