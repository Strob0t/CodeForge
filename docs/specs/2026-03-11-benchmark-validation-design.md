# Benchmark System Validation — Full Confidence Test Plan

**Date:** 2026-03-11
**Status:** Approved
**Scope:** End-to-end validation of the entire benchmark system through the frontend

## Goal

Prove that the benchmark system works across all suites, benchmark types, metric
combinations, and error scenarios. Tests are triggered through the frontend UI and
verify both API correctness and the interactive display (progress bar, WebSocket
events, scores, cost tracking).

## Constraints

- **Execution mode:** `mount` only (no sandbox/hybrid)
- **Primary model:** `lm_studio/qwen3-30b-a3b` (local, free)
- **Routing test:** 1 run with `model=auto` against free cloud providers
- **Smoke-first:** Each suite tested with 1 task first
- **Automation:** Fully automatic, no manual interaction

## API Field Name Reference

The Go `CreateRunRequest` uses these JSON field names:

| Spec Term      | API Field          | Example Values                           |
|----------------|--------------------|------------------------------------------|
| model          | `model`            | `"lm_studio/qwen3-30b-a3b"`, `"auto"`   |
| benchmark type | `benchmark_type`   | `"simple"`, `"tool_use"`, `"agent"`      |
| evaluators     | `metrics`          | `["llm_judge"]`, `["functional_test"]`   |
| exec mode      | `exec_mode`        | `"mount"`                                |
| dataset        | `dataset`          | `"basic-coding"`, `"humaneval"`          |

Evaluator names are snake_case strings: `llm_judge`, `functional_test`,
`sparc`, `trajectory_verifier`.

## 1. Combination Matrix

### Suite x Benchmark-Type

Based on actual provider implementations (`benchmark_type` property in each
provider class). External providers (humaneval, mbpp, bigcodebench, cruxeval,
livecodebench) all declare `BenchmarkType.SIMPLE` and do not populate tool
metadata — they cannot be run as `tool_use`.

| Suite              | simple | tool_use | agent | Provider Source                          |
|--------------------|--------|----------|-------|------------------------------------------|
| codeforge_simple   | yes    | no       | no    | Built-in, `basic-coding.yaml`            |
| codeforge_tool_use | no     | yes      | no    | Built-in, `tool-use-basic.yaml`          |
| codeforge_agent    | no     | no       | yes   | Built-in, `agent-coding.yaml`            |
| humaneval          | yes    | no       | no    | External, `BenchmarkType.SIMPLE`         |
| mbpp               | yes    | no       | no    | External, `BenchmarkType.SIMPLE`         |
| bigcodebench       | yes    | no       | no    | External, `BenchmarkType.SIMPLE`         |
| cruxeval           | yes    | no       | no    | External, `BenchmarkType.SIMPLE`         |
| livecodebench      | yes    | no       | no    | External, `BenchmarkType.SIMPLE`         |
| swebench           | no     | no       | yes   | External, `BenchmarkType.AGENT`          |
| sparcbench         | no     | no       | yes   | External, `BenchmarkType.AGENT`          |
| aider_polyglot     | no     | no       | yes   | External, `BenchmarkType.AGENT`          |

**Excluded:** `codeforge_synthetic` — requires Git repo workspace + LLM for
task generation; not suitable for automated validation.

**Total valid combinations:** 11

### Evaluator Combinations per Benchmark-Type

**Note on `functional_test`:** This evaluator executes `task.test_command` in the
runner's working directory. For `simple` runs, the `SimpleBenchmarkRunner` does
NOT write files to disk — it only collects the LLM text response. The
`functional_test` evaluator will return `score=0.0` with
`details={"error": "no test_command specified"}` or fail to find the expected
file. Tests that include `functional_test` on `simple` runs verify that this
**graceful degradation** works correctly (no crash, clear error in scores).

For `agent` runs, the `AgentBenchmarkRunner` creates a workspace and writes files,
so `functional_test` produces meaningful results only when `test_command` is set
on the task.

| Benchmark-Type | Evaluator Combinations Tested                                         |
|----------------|-----------------------------------------------------------------------|
| simple         | `llm_judge` alone, `llm_judge` + `functional_test` (degradation test) |
| tool_use       | `llm_judge` alone, `llm_judge` + `functional_test`, `functional_test` alone |
| agent          | `llm_judge` + `trajectory_verifier`, `llm_judge` + `sparc`, all 4 combined |

### Routing

- 1 test: `codeforge_simple` with `model=auto` against free cloud providers
- All other tests: `lm_studio/qwen3-30b-a3b` as fixed model

### Difficulty Audit (External Suites)

For each external suite, verify:
- Does the provider populate the `difficulty` field in `TaskSpec`?
- What values exist and what is the distribution?
- Are the values from the original dataset or synthetically estimated?

Known state (from code review — the audit confirms at runtime):
- All providers set `difficulty`, but most use synthetic estimation
  (e.g. humaneval estimates by solution line count, swebench by patch size)
- Expected values: `easy`, `medium`, `hard`

Suites: humaneval, mbpp, bigcodebench, cruxeval, livecodebench, swebench,
sparcbench, aider_polyglot

### Total Estimated Runs

- 11 suite x type combinations (fixed model)
- ~6 evaluator combination variants (beyond the base per-suite test)
- 1 routing proof
- 5 error scenarios
- 1 difficulty validation (no LLM calls)
- **~24 runs total**

## 2. Block Architecture

6 blocks, executed sequentially. Each block is independently runnable.
Block failure does NOT stop the next block.

Block status values:
- `passed` — all tests in the block passed
- `failed` — all tests in the block failed
- `partial` — at least one test passed and at least one failed

### Block 0: Prerequisites and Difficulty Validation

**Goal:** Verify infrastructure + audit difficulty mapping of all external suites.

Tests:
- Health check: Go backend reachable (`GET /health`)
- Health check: LiteLLM reachable (`GET http://localhost:4000/health`)
- Health check: LM Studio model available (via LiteLLM `/v1/models`)
- Health check: NATS connected (via `/health` response field)
- Frontend: Benchmark page loads
- Dataset discovery: All 3 built-in YAML files listed (`basic-coding`,
  `agent-coding`, `tool-use-basic`)
- Suite discovery: All 11 seeded suites listed via `GET /api/v1/benchmarks/suites`
  (Note: "datasets" = YAML files in `configs/benchmarks/`, "suites" = registered
  providers with config in the database)
- **Difficulty audit:** For each external suite:
  - Register suite with the provider via API (if not already seeded)
  - Load tasks and inspect `difficulty` field
  - Record: `has_difficulty`, values found, distribution, estimation method

**No LLM calls** — purely structural validation.

### Block 1: Simple Benchmarks

**Goal:** All suites that support `simple`, 1 task each.

| ID  | Suite            | Metrics (API field)                    | Expectation                           |
|-----|------------------|----------------------------------------|---------------------------------------|
| 1.1 | codeforge_simple | `["llm_judge"]`                       | Score > 0, cost > 0                  |
| 1.2 | humaneval        | `["llm_judge"]`                       | Score > 0                            |
| 1.3 | humaneval        | `["functional_test"]`                  | Graceful degradation: score = 0, no crash, clear error in scores |
| 1.4 | humaneval        | `["llm_judge", "functional_test"]`     | llm_judge score > 0, functional_test = 0 (expected) |
| 1.5 | mbpp             | `["llm_judge"]`                       | Score > 0                            |
| 1.6 | bigcodebench     | `["llm_judge"]`                       | Score > 0                            |
| 1.7 | cruxeval         | `["llm_judge"]`                       | Score > 0                            |
| 1.8 | livecodebench    | `["llm_judge"]`                       | Score > 0                            |

**Frontend checks per run:**
- Progress bar appears and advances
- WebSocket events received (Go hub broadcasts `benchmark.task.completed`
  and `benchmark.run.progress` as WebSocket messages to the frontend)
- Run status transitions: `running` -> `completed`
- Result scores displayed in UI
- Cost/token values > 0

### Block 2: Tool-Use Benchmarks

**Goal:** All suites that support `tool_use`, 1 task each.

Only `codeforge_tool_use` supports tool_use natively (has tool definitions in
task metadata).

| ID  | Suite              | Metrics (API field)                    | Expectation                      |
|-----|--------------------|----------------------------------------|----------------------------------|
| 2.1 | codeforge_tool_use | `["llm_judge"]`                       | Score > 0, tool_calls present    |
| 2.2 | codeforge_tool_use | `["functional_test"]`                  | Tool call validated              |
| 2.3 | codeforge_tool_use | `["llm_judge", "functional_test"]`     | Both evaluator scores present    |

**Frontend checks per run:**
- All checks from Block 1, plus:
- Tool calls displayed in result view

### Block 3: Agent Benchmarks

**Goal:** All suites that support `agent`, 1 task each, `exec_mode=mount`.

| ID  | Suite           | Metrics (API field)                                              | Expectation                       |
|-----|-----------------|------------------------------------------------------------------|-----------------------------------|
| 3.1 | codeforge_agent | `["llm_judge", "trajectory_verifier"]`                          | Multi-turn, files_changed > 0    |
| 3.2 | codeforge_agent | `["llm_judge", "sparc"]`                                        | SPARC score present              |
| 3.3 | codeforge_agent | `["llm_judge", "functional_test", "sparc", "trajectory_verifier"]` | All evaluator scores present (functional_test may be 0 if no test_command) |
| 3.4 | swebench        | `["llm_judge", "trajectory_verifier"]`                          | Agent loop ran, files changed    |
| 3.5 | sparcbench      | `["llm_judge", "sparc"]`                                        | SPARC score present              |
| 3.6 | aider_polyglot  | `["llm_judge", "trajectory_verifier"]`                          | Multi-language output            |

**Frontend checks per run:**
- All checks from Block 1, plus:
- Step count > 1 (multi-turn)
- `files_changed` displayed
- Longer duration correctly rendered

### Block 4: Intelligent Routing

**Goal:** Prove once that `model=auto` works.

| ID  | Test                                        | Expectation                                     |
|-----|---------------------------------------------|-------------------------------------------------|
| 4.1 | codeforge_simple with `model=auto`          | Run completed, `selected_model` is non-empty    |
| 4.2 | Check `routing_reason` in result            | Non-empty, contains router decision (e.g. `complexity`, `mab`, `meta`) |
| 4.3 | Check `selected_model` is populated         | Proves routing made an active selection          |

Note: We do NOT assert that `selected_model` differs from the local model —
the router may legitimately select any available model depending on MAB state
and cold-start fallback behavior.

**Frontend checks:**
- Routing info displayed in result view (if UI supports it)

### Block 5: Error Scenarios

**Goal:** System behaves correctly under failure conditions.

| ID  | Scenario                                | Expectation                               |
|-----|-----------------------------------------|-------------------------------------------|
| 5.1 | Invalid dataset (nonexistent path)      | Run status = `failed`, error message present |
| 5.2 | Invalid model (`nonexistent/model-xyz`) | Run status = `failed`, clear error        |
| 5.3 | Empty dataset (0 tasks)                 | Graceful handling, no infinite loop       |
| 5.4 | Unknown evaluator (`["nonexistent_evaluator"]`) | Graceful degradation: system logs warning, falls back to default `llm_judge` (no crash) |
| 5.5 | Duplicate run (same params immediately) | Idempotent, no double processing          |

Note on 5.4: `_build_evaluators()` silently skips unknown evaluator names
and falls back to `LLMJudgeEvaluator`. The test verifies this graceful
degradation, not an error.

**Frontend checks:**
- Failed status shown in red (5.1, 5.2, 5.3)
- Error message visible in UI
- For 5.4: run completes normally with llm_judge scores

## 3. LLM-Debug Report Format

Reports are written as JSON files, one per block, plus a final aggregated report.
Optimized for LLM-based root cause analysis.

### File Structure

```
frontend/e2e/benchmark-validation/
  reports/
    block-0-prerequisites.json
    block-1-simple.json
    block-2-tool-use.json
    block-3-agent.json
    block-4-routing.json
    block-5-errors.json
    full-report.json
```

### Per-Block Report Schema

```json
{
  "block": {
    "name": "block-1-simple",
    "status": "passed | failed | partial",
    "started_at": "ISO8601",
    "finished_at": "ISO8601",
    "duration_ms": 1320000,
    "summary": { "total": 8, "passed": 7, "failed": 1, "skipped": 0 }
  },
  "environment": {
    "backend_url": "http://localhost:8080",
    "litellm_url": "http://localhost:4000",
    "app_env": "development",
    "default_model": "lm_studio/qwen3-30b-a3b",
    "litellm_models_available": ["lm_studio/qwen3-30b-a3b", "..."],
    "git_commit": "abc123"
  },
  "tests": [
    {
      "id": "1.3",
      "name": "humaneval simple functional_test",
      "status": "failed",
      "suite": "humaneval",
      "benchmark_type": "simple",
      "metrics": ["functional_test"],
      "model": "lm_studio/qwen3-30b-a3b",
      "duration_ms": 45000,
      "request": {
        "method": "POST",
        "url": "/api/v1/benchmarks/runs",
        "body": {
          "dataset": "humaneval",
          "model": "lm_studio/qwen3-30b-a3b",
          "benchmark_type": "simple",
          "metrics": ["functional_test"],
          "exec_mode": "mount"
        }
      },
      "response": {
        "status_code": 200,
        "body": { "run_id": "run_abc123", "status": "completed" }
      },
      "run_result": {
        "status": "completed",
        "total_cost": 0.0,
        "total_tokens": 1542,
        "results": [
          {
            "task_id": "HumanEval/0",
            "scores": { "functional_test": 0.0 },
            "actual_output": "def has_close_elements(numbers, threshold): ...",
            "expected_output": "def has_close_elements(numbers, threshold): ...",
            "functional_test_output": "error: no test_command specified",
            "duration_ms": 38000
          }
        ]
      },
      "frontend_checks": {
        "progress_bar_appeared": true,
        "status_transition": ["running", "completed"],
        "scores_displayed": false,
        "cost_displayed": true,
        "websocket_events_received": 3
      },
      "failure": {
        "assertion": "expect(scoreElement).toBeVisible()",
        "message": "Score element not found in results view",
        "screenshot": "screenshots/1.3-humaneval-simple-functional-failure.png"
      },
      "debug_context": {
        "console_errors": ["TypeError: Cannot read property 'scores' of undefined"],
        "network_log": [
          { "method": "GET", "url": "/api/v1/benchmarks/runs/run_abc123", "status": 200, "duration_ms": 45 },
          { "method": "WS", "event": "benchmark.task.completed", "data": "..." }
        ]
      }
    }
  ]
}
```

### Full Report Schema (full-report.json)

```json
{
  "generated_at": "ISO8601",
  "total_duration_ms": 5400000,
  "git_commit": "abc123",
  "summary": {
    "blocks_total": 6,
    "blocks_passed": 5,
    "blocks_failed": 1,
    "tests_total": 24,
    "tests_passed": 22,
    "tests_failed": 1,
    "tests_skipped": 1,
    "total_llm_cost_usd": 0.00,
    "total_llm_tokens": 48000
  },
  "matrix": [
    {
      "suite": "humaneval",
      "results": {
        "simple_llm_judge": "passed",
        "simple_functional_test": "passed",
        "simple_both": "passed"
      }
    },
    {
      "suite": "codeforge_tool_use",
      "results": {
        "tool_use_llm_judge": "passed",
        "tool_use_functional_test": "passed",
        "tool_use_both": "passed"
      }
    }
  ],
  "difficulty_audit": [
    {
      "suite": "humaneval",
      "has_difficulty": true,
      "estimation_method": "synthetic (solution line count)",
      "values_found": ["easy", "medium", "hard"],
      "distribution": { "easy": 80, "medium": 45, "hard": 19 }
    },
    {
      "suite": "swebench",
      "has_difficulty": true,
      "estimation_method": "synthetic (patch line count)",
      "values_found": ["easy", "medium", "hard"],
      "distribution": { "easy": 100, "medium": 150, "hard": 50 }
    }
  ],
  "failures": [
    {
      "test_id": "1.3",
      "one_line": "humaneval simple functional_test: Score element not visible in UI",
      "root_hint": "Browser console: TypeError on scores property"
    }
  ],
  "recommendations": [
    "functional_test scores not displayed by frontend for simple-type runs -- UI bug?",
    "swebench may need higher max_iterations for local models"
  ]
}
```

### Report Design Principles

1. **Failures first** — `failures[]` at top of full report with one-liner + root hint
2. **Full context** — request, response, run result, frontend state, console errors, network log
3. **Screenshots on failure** — automatic, with descriptive filenames
4. **Matrix overview** — see at a glance which combinations work
5. **Difficulty audit** — separate section with estimation method transparency

## 4. Implementation Architecture

### New Files

```
frontend/e2e/benchmark-validation/
  playwright.validation.config.ts     # Separate config (sequential, high timeouts)
  reporter.ts                         # Custom LLM-Debug-Reporter (Playwright Reporter interface)
  helpers.ts                          # Shared utilities (API calls, waiters, frontend checks)
  types.ts                            # TypeScript types for report schema
  matrix.ts                           # Combination matrix definition (Suite x Type x Metrics)
  block-0-prerequisites.spec.ts       # Health, discovery, difficulty audit
  block-1-simple.spec.ts              # Simple benchmarks all suites
  block-2-tool-use.spec.ts            # Tool-use benchmarks
  block-3-agent.spec.ts               # Agent benchmarks
  block-4-routing.spec.ts             # Intelligent routing proof
  block-5-errors.spec.ts              # Error scenarios
  reports/                            # Generated reports (gitignored)
  screenshots/                        # Failure screenshots (gitignored)
```

### Playwright Config (playwright.validation.config.ts)

Separate config because requirements differ significantly from normal E2E tests:

- **Sequential execution:** `workers: 1`, `fullyParallel: false`
- **High timeouts:** `timeout: 600_000` (10 min per test, local model is slow)
- **Expect timeout:** `expect.timeout: 300_000` (5 min for benchmark completion polling)
- **Custom reporter:** `reporter: [['./reporter.ts']]`
- **Ordered projects:** Blocks run in order, but block failure does not stop the next

Invocation:
```bash
# All blocks automatically sequential
npx playwright test --config=e2e/benchmark-validation/playwright.validation.config.ts

# Single block (for debugging)
npx playwright test --config=e2e/benchmark-validation/playwright.validation.config.ts block-3-agent

# View failures from report
cat frontend/e2e/benchmark-validation/reports/full-report.json | jq '.failures'
```

### Custom Reporter (reporter.ts)

Implements Playwright `Reporter` interface:

- **onTestBegin:** Starts timing, opens context collector
- **onTestEnd:** Collects: result, error, attachments (screenshots, traces), duration
- **onEnd:** Per block: writes `block-N-*.json`. At end: aggregates to `full-report.json`,
  generates `failures[]`, `matrix`, `recommendations`

Captures additional data via **test attachments** — each test writes its
request/response/frontend-check state as an attachment, the reporter collects them.

### Helpers (helpers.ts)

Reusable functions:

- `createBenchmarkSuite(page, config)` -> suiteId
- `startBenchmarkRun(page, params)` -> runId
- `waitForRunCompletion(page, runId, timeoutMs)` -> RunResult
- `getRunDetails(apiContext, runId)` -> FullRunData
- `verifyProgressBar(page)` -> ProgressCheckResult
- `verifyStatusTransition(page, expected[])` -> boolean
- `verifyScoresDisplayed(page, runId)` -> boolean
- `captureWebSocketEvents(page, filter)` -> WSEvent[]
- `captureConsoleErrors(page)` -> string[]
- `captureNetworkLog(page, filter)` -> NetworkEntry[]
- `attachDebugContext(testInfo, context)` -> void
- `attachScreenshotOnFailure(page, testInfo)` -> void

### Matrix Definition (matrix.ts)

Declarative data structure defining all test combinations:

```typescript
const VALIDATION_MATRIX: TestCase[] = [
  // Block 1: Simple
  { id: "1.1", block: 1, suite: "codeforge_simple", type: "simple", metrics: ["llm_judge"] },
  { id: "1.2", block: 1, suite: "humaneval", type: "simple", metrics: ["llm_judge"] },
  { id: "1.3", block: 1, suite: "humaneval", type: "simple", metrics: ["functional_test"] },
  { id: "1.4", block: 1, suite: "humaneval", type: "simple", metrics: ["llm_judge", "functional_test"] },
  { id: "1.5", block: 1, suite: "mbpp", type: "simple", metrics: ["llm_judge"] },
  { id: "1.6", block: 1, suite: "bigcodebench", type: "simple", metrics: ["llm_judge"] },
  { id: "1.7", block: 1, suite: "cruxeval", type: "simple", metrics: ["llm_judge"] },
  { id: "1.8", block: 1, suite: "livecodebench", type: "simple", metrics: ["llm_judge"] },
  // Block 2: Tool-Use
  { id: "2.1", block: 2, suite: "codeforge_tool_use", type: "tool_use", metrics: ["llm_judge"] },
  { id: "2.2", block: 2, suite: "codeforge_tool_use", type: "tool_use", metrics: ["functional_test"] },
  { id: "2.3", block: 2, suite: "codeforge_tool_use", type: "tool_use", metrics: ["llm_judge", "functional_test"] },
  // Block 3: Agent
  { id: "3.1", block: 3, suite: "codeforge_agent", type: "agent", metrics: ["llm_judge", "trajectory_verifier"] },
  { id: "3.2", block: 3, suite: "codeforge_agent", type: "agent", metrics: ["llm_judge", "sparc"] },
  { id: "3.3", block: 3, suite: "codeforge_agent", type: "agent", metrics: ["llm_judge", "functional_test", "sparc", "trajectory_verifier"] },
  { id: "3.4", block: 3, suite: "swebench", type: "agent", metrics: ["llm_judge", "trajectory_verifier"] },
  { id: "3.5", block: 3, suite: "sparcbench", type: "agent", metrics: ["llm_judge", "sparc"] },
  { id: "3.6", block: 3, suite: "aider_polyglot", type: "agent", metrics: ["llm_judge", "trajectory_verifier"] },
  // Block 4: Routing
  { id: "4.1", block: 4, suite: "codeforge_simple", type: "simple", metrics: ["llm_judge"], model: "auto" },
]
```

Each spec file filters the matrix by its block and iterates with `for...of`.

### Standard Test Flow Per Run

```
 1. Navigate to benchmark page
 2. Start console error listener + network logger
 3. Select / create suite
 4. Set run parameters (model, benchmark_type, metrics, exec_mode="mount")
 5. Start run (button click)
 6. Verify progress bar (appears, advances)
 7. Collect WebSocket events (benchmark.task.completed, benchmark.run.progress)
 8. Wait for run status = completed|failed (polling with timeout)
 9. Fetch result data via API (full run + results)
10. Verify frontend display (scores, cost, tokens, status badge)
11. Attach everything as debug context to TestInfo
12. Run assertions (score > 0, cost > 0, expected evaluator scores present)
```
