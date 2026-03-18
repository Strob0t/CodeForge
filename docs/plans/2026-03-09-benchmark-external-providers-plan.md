# External Benchmark Providers, Full-Auto Routing & Prompt Optimization — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Connect the 8 existing external Python benchmark providers to the full data flow (DB -> NATS -> Python -> Evaluation), add full-auto model routing mode, and build an LLM-based prompt optimization feedback loop.

**Architecture:** Suite-based unified architecture. All benchmark sources (local YAML + external providers) unified under `benchmark_suites` DB entity. Frontend sends `suite_id` + `provider_config`, Go resolves suite -> NATS payload with `provider_name`, Python consumer loads tasks from provider registry instead of YAML file. Full-auto routing uses `model: "auto"` to trigger HybridRouter per-task with per-result tracking. Prompt optimizer uses LLM-as-Critic to analyze failures and generate patches for mode YAML `model_adaptations`.

**Tech Stack:** Go 1.25 (chi, pgx, NATS), Python 3.12 (Pydantic, structlog, LiteLLM), TypeScript/SolidJS, PostgreSQL 18

**Design doc:** `docs/plans/2026-03-09-benchmark-external-providers-design.md`

---

## Task 1: DB Migration — Routing Fields on benchmark_results

**Files:**
- Create: `internal/adapter/postgres/migrations/068_benchmark_routing_fields.sql`

**Step 1: Write the migration SQL**

Create migration file:

```sql
-- +goose Up
ALTER TABLE benchmark_results ADD COLUMN IF NOT EXISTS selected_model TEXT DEFAULT '';
ALTER TABLE benchmark_results ADD COLUMN IF NOT EXISTS routing_reason TEXT DEFAULT '';
ALTER TABLE benchmark_results ADD COLUMN IF NOT EXISTS fallback_chain TEXT DEFAULT '';
ALTER TABLE benchmark_results ADD COLUMN IF NOT EXISTS fallback_count INTEGER DEFAULT 0;
ALTER TABLE benchmark_results ADD COLUMN IF NOT EXISTS provider_errors TEXT DEFAULT '';

-- +goose Down
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS selected_model;
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS routing_reason;
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS fallback_chain;
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS fallback_count;
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS provider_errors;
```

**Step 2: Verify migration applies cleanly**

Run: `cd /workspaces/CodeForge && go run ./cmd/codeforge/ migrate up` (or check with `docker compose exec postgres psql -U codeforge -c "\d benchmark_results"` after startup)
Expected: 5 new columns visible in table schema

**Step 3: Commit**

```bash
git add internal/adapter/postgres/migrations/068_benchmark_routing_fields.sql
git commit -m "feat(benchmark): add routing tracking columns to benchmark_results (migration 068)"
```

---

## Task 2: Go Domain — Add Routing Fields to Result

**Files:**
- Modify: `internal/domain/benchmark/benchmark.go:122-148` (Result struct)
- Modify: `internal/domain/benchmark/benchmark.go:150-181` (CreateRunRequest — add ProviderConfig)

**Step 1: Add routing fields to Result struct**

In `internal/domain/benchmark/benchmark.go`, after the `DiversityScore` field (line 147), add:

```go
	// Routing tracking fields (auto-model mode).
	SelectedModel  string `json:"selected_model,omitempty"`
	RoutingReason  string `json:"routing_reason,omitempty"`
	FallbackChain  string `json:"fallback_chain,omitempty"`
	FallbackCount  int    `json:"fallback_count,omitempty"`
	ProviderErrors string `json:"provider_errors,omitempty"`
```

**Step 2: Add ProviderConfig to CreateRunRequest**

In `internal/domain/benchmark/benchmark.go`, add field to `CreateRunRequest` after `RolloutStrategy` (line 160):

```go
	ProviderConfig json.RawMessage `json:"provider_config,omitempty"`
```

**Step 3: Verify it compiles**

Run: `cd /workspaces/CodeForge && go build ./...`
Expected: Clean build, no errors

**Step 4: Commit**

```bash
git add internal/domain/benchmark/benchmark.go
git commit -m "feat(benchmark): add routing tracking fields to Result and ProviderConfig to CreateRunRequest"
```

---

## Task 3: Go Store — Update INSERT/SELECT for Routing Fields

**Files:**
- Modify: `internal/adapter/postgres/store_benchmark.go:19-22` (benchmarkResultColumns)
- Modify: `internal/adapter/postgres/store_benchmark.go:174-179` (INSERT query)
- Modify: the scan function for benchmark results

**Step 1: Update benchmarkResultColumns constant**

Replace existing constant to add routing columns at the end:

```go
const benchmarkResultColumns = `id, tenant_id, run_id, task_id, task_name, scores, actual_output, expected_output,
		tool_calls, cost_usd, tokens_in, tokens_out, duration_ms,
		evaluator_scores, files_changed, functional_test_output,
		rollout_id, rollout_count, is_best_rollout, diversity_score,
		selected_model, routing_reason, fallback_chain, fallback_count, provider_errors`
```

**Step 2: Update INSERT query in CreateBenchmarkResult**

Update the INSERT query to include the 5 new columns and their parameter placeholders ($21-$25):

```go
const q = `INSERT INTO benchmark_results
	(id, tenant_id, run_id, task_id, task_name, scores, actual_output, expected_output,
	 tool_calls, cost_usd, tokens_in, tokens_out, duration_ms,
	 evaluator_scores, files_changed, functional_test_output,
	 rollout_id, rollout_count, is_best_rollout, diversity_score,
	 selected_model, routing_reason, fallback_chain, fallback_count, provider_errors)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25)`
```

Add the new arguments to the Exec call:

```go
res.SelectedModel, res.RoutingReason, res.FallbackChain, res.FallbackCount, res.ProviderErrors,
```

**Step 3: Update the scan function**

In the scan function used by `ListBenchmarkResults`, add the 5 new fields to the Scan call:

```go
&r.SelectedModel, &r.RoutingReason, &r.FallbackChain, &r.FallbackCount, &r.ProviderErrors,
```

**Step 4: Verify it compiles**

Run: `cd /workspaces/CodeForge && go build ./...`
Expected: Clean build

**Step 5: Commit**

```bash
git add internal/adapter/postgres/store_benchmark.go
git commit -m "feat(benchmark): update store queries for routing tracking columns"
```

---

## Task 4: NATS Payloads — Add Provider + Routing Fields

**Files:**
- Modify: `internal/port/messagequeue/schemas.go:505-518` (BenchmarkRunRequestPayload)
- Modify: `internal/port/messagequeue/schemas.go:544-562` (BenchmarkTaskResult)

**Step 1: Add provider fields to BenchmarkRunRequestPayload**

In `schemas.go`, after `RolloutStrategy` (line 517), add:

```go
	ProviderName   string          `json:"provider_name,omitempty"`
	ProviderConfig json.RawMessage `json:"provider_config,omitempty"`
```

**Step 2: Add routing fields to BenchmarkTaskResult**

In `schemas.go`, after `DiversityScore` (line 561), add:

```go
	SelectedModel  string `json:"selected_model,omitempty"`
	RoutingReason  string `json:"routing_reason,omitempty"`
	FallbackChain  string `json:"fallback_chain,omitempty"`
	FallbackCount  int    `json:"fallback_count,omitempty"`
	ProviderErrors string `json:"provider_errors,omitempty"`
```

**Step 3: Verify it compiles**

Run: `cd /workspaces/CodeForge && go build ./...`
Expected: Clean build

**Step 4: Commit**

```bash
git add internal/port/messagequeue/schemas.go
git commit -m "feat(benchmark): add provider_name/config and routing fields to NATS payloads"
```

---

## Task 5: Python Models — Mirror NATS Payload Changes

**Files:**
- Modify: `workers/codeforge/models.py:511-525` (BenchmarkRunRequest)
- Modify: `workers/codeforge/models.py:528-548` (BenchmarkTaskResult)

**Step 1: Add provider fields to BenchmarkRunRequest**

After `rollout_strategy` field (line 525), add:

```python
    provider_name: str = ""
    provider_config: dict[str, Any] = Field(default_factory=dict)
```

Add `from typing import Any` to imports if not present.

**Step 2: Add routing fields to BenchmarkTaskResult**

After `diversity_score` field (line 548), add:

```python
    selected_model: str = ""
    routing_reason: str = ""
    fallback_chain: str = ""
    fallback_count: int = 0
    provider_errors: str = ""
```

**Step 3: Verify Python imports**

Run: `cd /workspaces/CodeForge/workers && python -c "from codeforge.models import BenchmarkRunRequest, BenchmarkTaskResult; print('OK')"`
Expected: `OK`

**Step 4: Commit**

```bash
git add workers/codeforge/models.py
git commit -m "feat(benchmark): add provider_name/config and routing fields to Python models"
```

---

## Task 6: Go Service — Suite-Based StartRun + Provider Config Forwarding

**Files:**
- Modify: `internal/service/benchmark.go:125-183` (StartRun method)

**Step 1: Update StartRun to resolve suite -> provider_name**

Replace the `StartRun` method body. The key change: when `suite_id` is set, load the suite from DB, extract `provider_name` + merge `provider_config`, and pass them via NATS instead of dataset_path.

In `internal/service/benchmark.go`, the payload construction section (lines 153-166) becomes:

```go
	// Resolve provider info from suite (if suite-based run).
	var providerName string
	var providerConfig json.RawMessage
	if run.SuiteID != "" {
		suite, sErr := s.store.GetBenchmarkSuite(ctx, run.SuiteID)
		if sErr != nil {
			slog.Warn("failed to load suite for run, falling back to dataset path", "suite_id", run.SuiteID, "error", sErr)
		} else {
			providerName = suite.ProviderName
			// Merge suite config with request-level provider_config (request overrides).
			providerConfig = mergeProviderConfig(suite.Config, req.ProviderConfig)
			if run.BenchmarkType == "" {
				run.BenchmarkType = suite.Type
			}
		}
	}

	payload := messagequeue.BenchmarkRunRequestPayload{
		RunID:              run.ID,
		TenantID:           tenantctx.FromContext(ctx),
		DatasetPath:        datasetPath,
		Model:              run.Model,
		Metrics:            run.Metrics,
		BenchmarkType:      string(run.BenchmarkType),
		SuiteID:            run.SuiteID,
		ExecMode:           string(run.ExecMode),
		Evaluators:         run.Metrics,
		HybridVerification: run.HybridVerification,
		RolloutCount:       run.RolloutCount,
		RolloutStrategy:    run.RolloutStrategy,
		ProviderName:       providerName,
		ProviderConfig:     providerConfig,
	}
```

**Step 2: Add mergeProviderConfig helper**

Below `StartRun`, add:

```go
// mergeProviderConfig merges suite-level config with request-level overrides.
// Request-level values take precedence.
func mergeProviderConfig(suiteConfig, requestConfig json.RawMessage) json.RawMessage {
	if len(requestConfig) == 0 || string(requestConfig) == "null" {
		return suiteConfig
	}
	if len(suiteConfig) == 0 || string(suiteConfig) == "null" {
		return requestConfig
	}
	var base map[string]json.RawMessage
	var override map[string]json.RawMessage
	if err := json.Unmarshal(suiteConfig, &base); err != nil {
		return requestConfig
	}
	if err := json.Unmarshal(override, &override); err != nil {
		return requestConfig // Bug: should unmarshal requestConfig
	}
	// Actually unmarshal requestConfig:
	if err := json.Unmarshal(requestConfig, &override); err != nil {
		return suiteConfig
	}
	for k, v := range override {
		base[k] = v
	}
	merged, _ := json.Marshal(base)
	return merged
}
```

Wait — let me write this correctly without the bug:

```go
func mergeProviderConfig(suiteConfig, requestConfig json.RawMessage) json.RawMessage {
	if len(requestConfig) == 0 || string(requestConfig) == "null" {
		return suiteConfig
	}
	if len(suiteConfig) == 0 || string(suiteConfig) == "null" {
		return requestConfig
	}
	var base map[string]json.RawMessage
	if err := json.Unmarshal(suiteConfig, &base); err != nil {
		return requestConfig
	}
	var override map[string]json.RawMessage
	if err := json.Unmarshal(requestConfig, &override); err != nil {
		return suiteConfig
	}
	for k, v := range override {
		base[k] = v
	}
	merged, _ := json.Marshal(base)
	return merged
}
```

**Step 3: Update HandleBenchmarkRunResult to extract routing fields**

In `internal/service/benchmark.go:590-618`, when building each `benchmark.Result`, add the routing fields:

```go
		result := &benchmark.Result{
			// ...existing fields...
			SelectedModel:  tr.SelectedModel,
			RoutingReason:  tr.RoutingReason,
			FallbackChain:  tr.FallbackChain,
			FallbackCount:  tr.FallbackCount,
			ProviderErrors: tr.ProviderErrors,
		}
```

**Step 4: Verify it compiles**

Run: `cd /workspaces/CodeForge && go build ./...`
Expected: Clean build

**Step 5: Commit**

```bash
git add internal/service/benchmark.go
git commit -m "feat(benchmark): suite-based StartRun with provider_name resolution and routing field extraction"
```

---

## Task 7: Go Service — Suite Seeding on Startup

**Files:**
- Modify: `internal/service/benchmark.go` (add SeedDefaultSuites method)

**Step 1: Add SeedDefaultSuites method**

After `NewBenchmarkService`, add a method that idempotently seeds default suites:

```go
// defaultSuites defines built-in benchmark suites seeded on startup.
var defaultSuites = []benchmark.CreateSuiteRequest{
	// Local YAML datasets
	{Name: "Basic Coding", Type: benchmark.TypeSimple, ProviderName: "codeforge_simple"},
	{Name: "Agent Coding", Type: benchmark.TypeAgent, ProviderName: "codeforge_agent"},
	{Name: "Tool Use Basic", Type: benchmark.TypeToolUse, ProviderName: "codeforge_tool_use"},
	// External providers
	{Name: "HumanEval", Type: benchmark.TypeSimple, ProviderName: "humaneval"},
	{Name: "MBPP", Type: benchmark.TypeSimple, ProviderName: "mbpp"},
	{Name: "SWE-bench", Type: benchmark.TypeAgent, ProviderName: "swebench"},
	{Name: "BigCodeBench", Type: benchmark.TypeSimple, ProviderName: "bigcodebench"},
	{Name: "CRUXEval", Type: benchmark.TypeSimple, ProviderName: "cruxeval"},
	{Name: "LiveCodeBench", Type: benchmark.TypeSimple, ProviderName: "livecodebench"},
	{Name: "SPARCBench", Type: benchmark.TypeAgent, ProviderName: "sparcbench"},
	{Name: "Aider Polyglot", Type: benchmark.TypeAgent, ProviderName: "aider_polyglot"},
}

// SeedDefaultSuites creates built-in benchmark suites if they don't exist.
// Called once on server startup. Uses background context with default tenant.
func (s *BenchmarkService) SeedDefaultSuites(ctx context.Context) {
	existing, err := s.store.ListBenchmarkSuites(ctx)
	if err != nil {
		slog.Warn("failed to list suites for seeding", "error", err)
		return
	}
	seen := make(map[string]bool, len(existing))
	for _, suite := range existing {
		seen[suite.ProviderName] = true
	}
	for _, def := range defaultSuites {
		if seen[def.ProviderName] {
			continue
		}
		if _, err := s.RegisterSuite(ctx, &def); err != nil {
			slog.Warn("failed to seed benchmark suite", "name", def.Name, "error", err)
		} else {
			slog.Info("seeded benchmark suite", "name", def.Name, "provider", def.ProviderName)
		}
	}
}
```

**Step 2: Verify BenchmarkType constants exist**

Check that `benchmark.TypeSimple`, `benchmark.TypeAgent`, `benchmark.TypeToolUse` are defined. If the domain uses string-based `BenchmarkType`, use the string values directly (e.g. `"simple"`, `"agent"`, `"tool_use"`).

**Step 3: Call SeedDefaultSuites from server startup**

Find where `BenchmarkService` is initialized in the server wiring (likely `cmd/codeforge/main.go` or `internal/server/server.go`). Add:

```go
benchSvc.SeedDefaultSuites(ctx)
```

after the service is created and store is connected.

**Step 4: Verify it compiles**

Run: `cd /workspaces/CodeForge && go build ./...`
Expected: Clean build

**Step 5: Commit**

```bash
git add internal/service/benchmark.go cmd/codeforge/main.go
git commit -m "feat(benchmark): seed 11 default benchmark suites on startup"
```

---

## Task 8: Python — Universal Task Filter Function

**Files:**
- Create: `workers/codeforge/evaluation/task_filter.py`
- Test: `workers/tests/evaluation/test_task_filter.py`

**Step 1: Write the failing tests**

Create `workers/tests/evaluation/test_task_filter.py`:

```python
"""Tests for universal task filter function."""
from codeforge.evaluation.providers.base import TaskSpec
from codeforge.evaluation.task_filter import apply_task_filters


def _make_tasks(n: int, difficulties: list[str] | None = None) -> list[TaskSpec]:
    diffs = difficulties or ["easy", "medium", "hard"]
    return [
        TaskSpec(id=f"t-{i}", name=f"task-{i}", input=f"input-{i}", difficulty=diffs[i % len(diffs)])
        for i in range(n)
    ]


def test_no_filters_returns_all():
    tasks = _make_tasks(10)
    result = apply_task_filters(tasks, {})
    assert len(result) == 10


def test_difficulty_filter():
    tasks = _make_tasks(9)  # 3 easy, 3 medium, 3 hard
    result = apply_task_filters(tasks, {"difficulty_filter": ["easy"], "shuffle": False})
    assert all(t.difficulty == "easy" for t in result)
    assert len(result) == 3


def test_max_tasks():
    tasks = _make_tasks(20)
    result = apply_task_filters(tasks, {"max_tasks": 5, "shuffle": False})
    assert len(result) == 5


def test_task_percentage():
    tasks = _make_tasks(100)
    result = apply_task_filters(tasks, {"task_percentage": 10, "shuffle": False})
    assert len(result) == 10


def test_max_tasks_and_percentage_more_restrictive_wins():
    tasks = _make_tasks(100)
    # max_tasks=20 is more restrictive than 50%
    result = apply_task_filters(tasks, {"max_tasks": 20, "task_percentage": 50, "shuffle": False})
    assert len(result) == 20
    # percentage is more restrictive: 10% of 100 = 10 < max_tasks=50
    result2 = apply_task_filters(tasks, {"max_tasks": 50, "task_percentage": 10, "shuffle": False})
    assert len(result2) == 10


def test_shuffle_with_seed_is_deterministic():
    tasks = _make_tasks(20)
    r1 = apply_task_filters(tasks, {"shuffle": True, "seed": 42})
    r2 = apply_task_filters(tasks, {"shuffle": True, "seed": 42})
    assert [t.id for t in r1] == [t.id for t in r2]


def test_shuffle_with_different_seeds_differs():
    tasks = _make_tasks(20)
    r1 = apply_task_filters(tasks, {"shuffle": True, "seed": 1})
    r2 = apply_task_filters(tasks, {"shuffle": True, "seed": 2})
    assert [t.id for t in r1] != [t.id for t in r2]


def test_empty_tasks_returns_empty():
    result = apply_task_filters([], {"max_tasks": 5})
    assert result == []


def test_percentage_at_least_one():
    tasks = _make_tasks(3)
    result = apply_task_filters(tasks, {"task_percentage": 1, "shuffle": False})
    assert len(result) >= 1
```

**Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge/workers && python -m pytest tests/evaluation/test_task_filter.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'codeforge.evaluation.task_filter'`

**Step 3: Write the implementation**

Create `workers/codeforge/evaluation/task_filter.py`:

```python
"""Universal task filter for benchmark providers.

Applies difficulty filtering, shuffling, and task count capping
to any list of TaskSpec objects. Used by all providers uniformly.
"""
from __future__ import annotations

import math
import random

from codeforge.evaluation.providers.base import TaskSpec


def apply_task_filters(tasks: list[TaskSpec], config: dict) -> list[TaskSpec]:
    """Apply universal filters to a task list based on provider config.

    Supports:
    - difficulty_filter: list of allowed difficulty levels
    - shuffle: bool (default True)
    - seed: int for reproducible shuffle (default 42)
    - max_tasks: cap absolute count (0 = unlimited)
    - task_percentage: cap as percentage of total (100 = all)

    When both max_tasks and task_percentage are set, the more restrictive wins.
    """
    if not tasks:
        return []

    result = list(tasks)

    # 1. Difficulty filter
    difficulties = config.get("difficulty_filter", [])
    if difficulties:
        result = [t for t in result if t.difficulty in difficulties]

    # 2. Shuffle
    if config.get("shuffle", True):
        rng = random.Random(config.get("seed", 42))
        rng.shuffle(result)

    # 3. Cap by percentage first, then by max_tasks — more restrictive wins
    total = len(result)
    percentage = config.get("task_percentage", 100)
    if 0 < percentage < 100:
        cap_by_pct = max(1, math.ceil(total * percentage / 100))
        result = result[:cap_by_pct]

    max_tasks = config.get("max_tasks", 0)
    if max_tasks > 0:
        result = result[:max_tasks]

    return result
```

**Step 4: Run tests to verify they pass**

Run: `cd /workspaces/CodeForge/workers && python -m pytest tests/evaluation/test_task_filter.py -v`
Expected: All 9 tests PASS

**Step 5: Commit**

```bash
git add workers/codeforge/evaluation/task_filter.py workers/tests/evaluation/test_task_filter.py
git commit -m "feat(benchmark): universal task filter function with tests"
```

---

## Task 9: Python — Add config Parameter to Provider Constructors

**Files:**
- Modify: `workers/codeforge/evaluation/providers/humaneval.py:48-50`
- Modify: `workers/codeforge/evaluation/providers/mbpp.py` (constructor)
- Modify: `workers/codeforge/evaluation/providers/swebench.py` (constructor)
- Modify: `workers/codeforge/evaluation/providers/bigcodebench.py` (constructor)
- Modify: `workers/codeforge/evaluation/providers/cruxeval.py` (constructor)
- Modify: `workers/codeforge/evaluation/providers/livecodebench.py` (constructor)
- Modify: `workers/codeforge/evaluation/providers/sparcbench.py` (constructor)
- Modify: `workers/codeforge/evaluation/providers/aider_polyglot.py` (constructor)

**Step 1: Inspect each provider constructor**

Read each provider file to find the `__init__` signature. All providers follow the HumanEval pattern: `__init__(self, cache_dir: str = "", tasks: list[dict] | None = None)`.

**Step 2: Add config parameter to each provider**

For each of the 8 external provider files, update `__init__` to accept `config: dict | None = None` and store it:

```python
def __init__(self, cache_dir: str = "", tasks: list[dict] | None = None, config: dict | None = None) -> None:
    self._cache_dir = cache_dir
    self._tasks_raw = tasks
    self._config = config or {}
```

Provider-specific settings are read from `self._config` where relevant (e.g., SWE-bench reads `self._config.get("variant", "full")`). For this initial pass, just add the parameter — provider-specific filtering via config will work through the universal `apply_task_filters` called by the consumer.

**Step 3: Verify imports work**

Run: `cd /workspaces/CodeForge/workers && python -c "from codeforge.evaluation.providers.humaneval import HumanEvalProvider; p = HumanEvalProvider(config={'max_tasks': 10}); print('OK')"`
Expected: `OK`

**Step 4: Commit**

```bash
git add workers/codeforge/evaluation/providers/
git commit -m "feat(benchmark): add config parameter to all 8 external provider constructors"
```

---

## Task 10: Python — Provider Auto-Import in __init__.py

**Files:**
- Modify: `workers/codeforge/evaluation/providers/__init__.py`

**Step 1: Import all external providers for auto-registration**

Currently only `codeforge_synthetic` is imported. Add all providers so they self-register:

```python
# Self-register all providers on import.
import codeforge.evaluation.providers.codeforge_synthetic as _  # noqa: F401
import codeforge.evaluation.providers.humaneval as _h  # noqa: F401
import codeforge.evaluation.providers.mbpp as _m  # noqa: F401
import codeforge.evaluation.providers.swebench as _s  # noqa: F401
import codeforge.evaluation.providers.bigcodebench as _b  # noqa: F401
import codeforge.evaluation.providers.cruxeval as _cr  # noqa: F401
import codeforge.evaluation.providers.livecodebench as _l  # noqa: F401
import codeforge.evaluation.providers.sparcbench as _sp  # noqa: F401
import codeforge.evaluation.providers.aider_polyglot as _ap  # noqa: F401
```

**Step 2: Verify all 8 external + 4 codeforge providers are registered**

Run: `cd /workspaces/CodeForge/workers && python -c "from codeforge.evaluation.providers import list_providers; print(sorted(list_providers()))"`
Expected: List includes `humaneval`, `mbpp`, `swebench`, `bigcodebench`, `cruxeval`, `livecodebench`, `sparcbench`, `aider_polyglot` plus the codeforge built-in ones.

**Step 3: Commit**

```bash
git add workers/codeforge/evaluation/providers/__init__.py
git commit -m "feat(benchmark): auto-register all 8 external providers on import"
```

---

## Task 11: Python Consumer — Provider-Based Task Loading

**Files:**
- Modify: `workers/codeforge/consumer/_benchmark.py:258-287` (runner functions)

**Step 1: Add provider-based task loading helper**

Above `_run_simple_benchmark` (around line 257), add:

```python
async def _load_tasks_for_run(req) -> list:
    """Load tasks from provider registry or legacy YAML dataset."""
    from codeforge.evaluation.providers import get_provider
    from codeforge.evaluation.task_filter import apply_task_filters

    if req.provider_name:
        provider_cls = get_provider(req.provider_name)
        provider = provider_cls(config=req.provider_config)
        tasks = await provider.load_tasks()
        return apply_task_filters(tasks, req.provider_config)

    # Legacy fallback: load from YAML dataset path
    return _dataset_to_task_specs(req.dataset_path)
```

**Step 2: Update the three runner functions to use _load_tasks_for_run**

Replace `_dataset_to_task_specs(req.dataset_path)` calls in the three runner functions:

```python
async def _run_simple_benchmark(req, llm, pipeline) -> list:
    from codeforge.evaluation.runners.simple import SimpleBenchmarkRunner
    runner = SimpleBenchmarkRunner(llm=llm, pipeline=pipeline, model=req.model)
    tasks = await _load_tasks_for_run(req)
    return await _run_with_optional_rollout(runner, tasks, req, pipeline)


async def _run_tool_use_benchmark(req, llm, pipeline) -> list:
    from codeforge.evaluation.runners.tool_use import ToolUseBenchmarkRunner
    runner = ToolUseBenchmarkRunner(llm=llm, pipeline=pipeline, model=req.model)
    tasks = await _load_tasks_for_run(req)
    return await _run_with_optional_rollout(runner, tasks, req, pipeline)


async def _run_agent_benchmark(req, llm, pipeline) -> list:
    from codeforge.agent_loop import AgentLoopExecutor, LoopConfig
    from codeforge.evaluation.runners.agent import AgentBenchmarkRunner

    # For provider-based agent runs, load tasks from provider
    if req.provider_name:
        tasks = await _load_tasks_for_run(req)
    else:
        from codeforge.evaluation.providers.codeforge_agent import CodeForgeAgentProvider
        provider = CodeForgeAgentProvider(datasets_dir=req.dataset_path)
        tasks = await provider.load_tasks()

    config = LoopConfig(model=req.model, max_cost=req.provider_config.get("max_cost", 1.0) if req.provider_config else 1.0)
    executor = AgentLoopExecutor(llm=llm, tools=None, runtime=None)
    runner = AgentBenchmarkRunner(executor=executor, pipeline=pipeline, loop_config=config)
    return await _run_with_optional_rollout(runner, tasks, req, pipeline)
```

**Step 3: Verify Python syntax**

Run: `cd /workspaces/CodeForge/workers && python -c "import codeforge.consumer._benchmark; print('OK')"`
Expected: `OK`

**Step 4: Commit**

```bash
git add workers/codeforge/consumer/_benchmark.py
git commit -m "feat(benchmark): provider-based task loading in Python consumer with legacy fallback"
```

---

## Task 12: Frontend Types — Add Provider Config + Routing Fields

**Files:**
- Modify: `frontend/src/api/types.ts:1685-1694` (CreateBenchmarkRunRequest)
- Modify: `frontend/src/api/types.ts:1662-1683` (BenchmarkResult)
- Add new interfaces after BenchmarkResult

**Step 1: Update CreateBenchmarkRunRequest**

Replace the interface at lines 1685-1694:

```typescript
/** Matches Go domain/benchmark.CreateRunRequest */
export interface CreateBenchmarkRunRequest {
  dataset?: string;
  suite_id?: string;
  model: string;
  metrics: string[];
  benchmark_type?: BenchmarkType;
  exec_mode?: BenchmarkExecMode;
  provider_config?: ProviderConfig;
}
```

**Step 2: Add ProviderConfig interface**

After CreateBenchmarkRunRequest:

```typescript
/** Universal + provider-specific benchmark task settings. */
export interface ProviderConfig {
  max_tasks?: number;
  task_percentage?: number;
  difficulty_filter?: string[];
  shuffle?: boolean;
  seed?: number;
  [key: string]: unknown;
}
```

**Step 3: Add routing fields to BenchmarkResult**

After `diversity_score` (line 1682), add:

```typescript
  selected_model?: string;
  routing_reason?: string;
  fallback_chain?: string;
  fallback_count?: number;
  provider_errors?: string;
```

**Step 4: Add RoutingReport interfaces**

After ProviderConfig:

```typescript
/** Model usage stats within a routing report. */
export interface ModelUsageStats {
  task_count: number;
  task_percentage: number;
  avg_score: number;
  avg_cost_per_task: number;
  difficulty_distribution: Record<string, number>;
}

/** Fallback event detail. */
export interface FallbackEvent {
  task_id: string;
  primary: string;
  fallback_to: string;
  reason: string;
}

/** Provider availability status. */
export interface ProviderStatus {
  reachable: boolean;
  errors: number;
  error_types: string[];
}

/** Aggregated routing report for auto-model benchmark runs. */
export interface RoutingReport {
  models_used: Record<string, ModelUsageStats>;
  fallback_events: number;
  fallback_details: FallbackEvent[];
  provider_availability: Record<string, ProviderStatus>;
  system_score: number;
  system_cost: number;
}
```

**Step 5: Verify TypeScript compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: No type errors

**Step 6: Commit**

```bash
git add frontend/src/api/types.ts
git commit -m "feat(benchmark): add ProviderConfig, routing fields, and RoutingReport types"
```

---

## Task 13: Frontend i18n — Add New Benchmark Keys

**Files:**
- Modify: `frontend/src/i18n/en.ts` (benchmark section)
- Modify: `frontend/src/i18n/locales/de.ts` (benchmark section)

**Step 1: Add English keys**

Add the following keys to the benchmark section of `en.ts` (after existing benchmark keys):

```typescript
  "benchmark.suite": "Benchmark Suite",
  "benchmark.suiteLocal": "Local",
  "benchmark.suiteExternal": "External",
  "benchmark.taskSettings": "Task Settings",
  "benchmark.allTasks": "All ({count})",
  "benchmark.limitTasks": "Limit to",
  "benchmark.percentTasks": "Percentage",
  "benchmark.difficulty": "Difficulty",
  "benchmark.shuffle": "Shuffle",
  "benchmark.seed": "Random Seed",
  "benchmark.estimatedCost": "Estimated: ~{tasks} tasks x ~${cost}/task = ${total}",
  "benchmark.providerSettings": "Provider Settings",
  "benchmark.modelAuto": "Auto (intelligent routing)",
  "benchmark.routingReport": "Routing Report",
  "benchmark.modelDistribution": "Model Distribution",
  "benchmark.fallbackEvents": "Fallback Events",
  "benchmark.providerStatus": "Provider Availability",
  "benchmark.promptOptimization": "Prompt Optimization",
  "benchmark.analyzePrompts": "Analyze & Suggest Improvements",
  "benchmark.promptDiff": "Suggested Changes",
  "benchmark.acceptPatch": "Accept",
  "benchmark.rejectPatch": "Reject",
  "benchmark.rerunBenchmark": "Re-Run to Validate",
  "benchmark.scoreImproved": "Score improved: {before} -> {after} (+{delta})",
  "benchmark.scoreRegressed": "Score regressed: {before} -> {after} ({delta})",
```

**Step 2: Add German keys**

Add matching keys to `de.ts`:

```typescript
  "benchmark.suite": "Benchmark-Suite",
  "benchmark.suiteLocal": "Lokal",
  "benchmark.suiteExternal": "Extern",
  "benchmark.taskSettings": "Aufgaben-Einstellungen",
  "benchmark.allTasks": "Alle ({count})",
  "benchmark.limitTasks": "Begrenzen auf",
  "benchmark.percentTasks": "Prozent",
  "benchmark.difficulty": "Schwierigkeit",
  "benchmark.shuffle": "Mischen",
  "benchmark.seed": "Zufalls-Seed",
  "benchmark.estimatedCost": "Geschätzt: ~{tasks} Aufgaben x ~${cost}/Aufgabe = ${total}",
  "benchmark.providerSettings": "Provider-Einstellungen",
  "benchmark.modelAuto": "Auto (intelligentes Routing)",
  "benchmark.routingReport": "Routing-Bericht",
  "benchmark.modelDistribution": "Modellverteilung",
  "benchmark.fallbackEvents": "Fallback-Ereignisse",
  "benchmark.providerStatus": "Provider-Verfügbarkeit",
  "benchmark.promptOptimization": "Prompt-Optimierung",
  "benchmark.analyzePrompts": "Analysieren & Verbesserungen vorschlagen",
  "benchmark.promptDiff": "Vorgeschlagene Änderungen",
  "benchmark.acceptPatch": "Akzeptieren",
  "benchmark.rejectPatch": "Ablehnen",
  "benchmark.rerunBenchmark": "Erneut ausführen zur Validierung",
  "benchmark.scoreImproved": "Verbesserung: {before} -> {after} (+{delta})",
  "benchmark.scoreRegressed": "Verschlechterung: {before} -> {after} ({delta})",
```

**Step 3: Verify frontend builds**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: No errors

**Step 4: Commit**

```bash
git add frontend/src/i18n/en.ts frontend/src/i18n/locales/de.ts
git commit -m "feat(benchmark): add i18n keys for suite picker, task settings, routing report, prompt optimization (EN+DE)"
```

---

## Task 14: Frontend — TaskSettings Component

**Files:**
- Create: `frontend/src/features/benchmarks/TaskSettings.tsx`

**Step 1: Create TaskSettings component**

This collapsible component renders universal task settings (max_tasks, percentage, difficulty, shuffle, seed) plus provider-specific settings:

```tsx
import { createSignal, For, Show } from "solid-js";
import { Button, Card, FormField, Input, Select } from "../../components";
import { useI18n } from "../../i18n";
import type { ProviderConfig } from "../../api/types";

const DIFFICULTIES = ["easy", "medium", "hard"];

// Provider-specific settings schema
const PROVIDER_SETTINGS: Record<string, { key: string; label: string; type: "text" | "select"; options?: string[] }[]> = {
  swebench: [
    { key: "variant", label: "Variant", type: "select", options: ["full", "lite", "verified"] },
    { key: "repo_filter", label: "Repository Filter", type: "text" },
  ],
  bigcodebench: [
    { key: "subset", label: "Subset", type: "select", options: ["complete", "hard", "instruct"] },
  ],
  cruxeval: [
    { key: "task_type", label: "Task Type", type: "select", options: ["input_prediction", "output_prediction"] },
  ],
  livecodebench: [
    { key: "date_range", label: "Date Range", type: "text" },
    { key: "contest_filter", label: "Contest Filter", type: "text" },
  ],
  sparcbench: [
    { key: "category_filter", label: "Category Filter", type: "text" },
  ],
  aider_polyglot: [
    { key: "language_filter", label: "Language", type: "select", options: ["python", "javascript", "typescript", "java", "go", "rust"] },
  ],
};

interface TaskSettingsProps {
  providerName: string;
  config: ProviderConfig;
  onChange: (config: ProviderConfig) => void;
  taskCount?: number;
}

export function TaskSettings(props: TaskSettingsProps) {
  const { t } = useI18n();
  const [expanded, setExpanded] = createSignal(false);

  const updateConfig = (key: string, value: unknown) => {
    props.onChange({ ...props.config, [key]: value });
  };

  const toggleDifficulty = (d: string) => {
    const current = (props.config.difficulty_filter as string[]) || [];
    const next = current.includes(d) ? current.filter((x) => x !== d) : [...current, d];
    updateConfig("difficulty_filter", next);
  };

  const providerFields = () => PROVIDER_SETTINGS[props.providerName] || [];

  return (
    <Card class="p-3">
      <button
        type="button"
        class="flex w-full items-center justify-between text-sm font-medium"
        onClick={() => setExpanded(!expanded())}
      >
        <span>{t("benchmark.taskSettings")}</span>
        <span class="text-xs">{expanded() ? "▲" : "▼"}</span>
      </button>

      <Show when={expanded()}>
        <div class="mt-3 space-y-3">
          {/* Task count limit */}
          <FormField label={t("benchmark.limitTasks")} id="ts-max-tasks">
            <Input
              type="number"
              min={0}
              value={String(props.config.max_tasks ?? 0)}
              onInput={(e) => updateConfig("max_tasks", parseInt(e.currentTarget.value) || 0)}
              placeholder="0 = all"
            />
          </FormField>

          {/* Percentage */}
          <FormField label={t("benchmark.percentTasks")} id="ts-percentage">
            <Input
              type="number"
              min={1}
              max={100}
              value={String(props.config.task_percentage ?? 100)}
              onInput={(e) => updateConfig("task_percentage", parseInt(e.currentTarget.value) || 100)}
            />
          </FormField>

          {/* Difficulty filter */}
          <FormField label={t("benchmark.difficulty")} id="ts-difficulty">
            <div class="flex gap-2">
              <For each={DIFFICULTIES}>
                {(d) => (
                  <Button
                    type="button"
                    size="sm"
                    variant={((props.config.difficulty_filter as string[]) || []).includes(d) ? "primary" : "ghost"}
                    onClick={() => toggleDifficulty(d)}
                  >
                    {d}
                  </Button>
                )}
              </For>
            </div>
          </FormField>

          {/* Shuffle + Seed */}
          <div class="flex items-center gap-4">
            <label class="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={props.config.shuffle !== false}
                onChange={(e) => updateConfig("shuffle", e.currentTarget.checked)}
              />
              {t("benchmark.shuffle")}
            </label>
            <FormField label={t("benchmark.seed")} id="ts-seed">
              <Input
                type="number"
                value={String(props.config.seed ?? 42)}
                onInput={(e) => updateConfig("seed", parseInt(e.currentTarget.value) || 42)}
                class="w-24"
              />
            </FormField>
          </div>

          {/* Provider-specific settings */}
          <Show when={providerFields().length > 0}>
            <div class="border-t pt-3">
              <span class="text-xs font-medium text-[var(--color-text-secondary)]">
                {t("benchmark.providerSettings")}
              </span>
              <div class="mt-2 space-y-2">
                <For each={providerFields()}>
                  {(field) => (
                    <FormField label={field.label} id={`ts-${field.key}`}>
                      <Show
                        when={field.type === "select" && field.options}
                        fallback={
                          <Input
                            value={String(props.config[field.key] ?? "")}
                            onInput={(e) => updateConfig(field.key, e.currentTarget.value)}
                          />
                        }
                      >
                        <Select
                          value={String(props.config[field.key] ?? "")}
                          onChange={(e) => updateConfig(field.key, e.currentTarget.value)}
                        >
                          <option value="">All</option>
                          <For each={field.options!}>
                            {(opt) => <option value={opt}>{opt}</option>}
                          </For>
                        </Select>
                      </Show>
                    </FormField>
                  )}
                </For>
              </div>
            </div>
          </Show>
        </div>
      </Show>
    </Card>
  );
}
```

**Step 2: Verify TypeScript compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: No errors (adapt imports to match actual component library exports)

**Step 3: Commit**

```bash
git add frontend/src/features/benchmarks/TaskSettings.tsx
git commit -m "feat(benchmark): TaskSettings component with universal + provider-specific settings"
```

---

## Task 15: Frontend — Replace Dataset Dropdown with Suite Dropdown

**Files:**
- Modify: `frontend/src/features/benchmarks/BenchmarkPage.tsx:88-132` (form state + handler)
- Modify: `frontend/src/features/benchmarks/BenchmarkPage.tsx:183-208` (dataset dropdown)

**Step 1: Update form state**

Replace `formDefaults` (line 90-96) — replace `dataset: ""` with `suiteId: ""` and add `providerConfig`:

```typescript
const formDefaults = {
  suiteId: "",
  model: "",
  metrics: ["correctness"] as string[],
  benchmarkType: "simple" as BenchmarkType,
  execMode: "mount" as BenchmarkExecMode,
  providerConfig: {} as ProviderConfig,
};
```

**Step 2: Update handleCreate**

Replace the `req` construction (lines 116-122):

```typescript
const req: CreateBenchmarkRunRequest = {
  suite_id: form.state.suiteId,
  model: form.state.model,
  metrics: form.state.metrics,
  benchmark_type: form.state.benchmarkType,
  exec_mode: form.state.benchmarkType === "agent" ? form.state.execMode : undefined,
  provider_config: Object.keys(form.state.providerConfig).length > 0 ? form.state.providerConfig : undefined,
};
```

**Step 3: Replace dataset dropdown with suite dropdown**

Replace the dataset `<FormField>` block (lines 183-208) with a suite dropdown grouped by local/external:

```tsx
<FormField label={t("benchmark.suite")} id="benchmark-suite">
  <Select
    value={form.state.suiteId}
    onChange={(e) => {
      const id = e.currentTarget.value;
      form.setState("suiteId", id);
      // Auto-fill benchmark type from suite
      const suite = suites()?.find((s) => s.id === id);
      if (suite) {
        form.setState("benchmarkType", suite.type as BenchmarkType);
      }
    }}
  >
    <option value="">{t("common.select")}</option>
    <optgroup label={t("benchmark.suiteLocal")}>
      <For each={suites()?.filter((s) => s.provider_name.startsWith("codeforge_"))}>
        {(s) => <option value={s.id}>{s.name} ({s.task_count} tasks)</option>}
      </For>
    </optgroup>
    <optgroup label={t("benchmark.suiteExternal")}>
      <For each={suites()?.filter((s) => !s.provider_name.startsWith("codeforge_"))}>
        {(s) => <option value={s.id}>{s.name} ({s.task_count} tasks)</option>}
      </For>
    </optgroup>
  </Select>
</FormField>
```

**Step 4: Add TaskSettings below the suite dropdown**

After the suite dropdown FormField:

```tsx
<Show when={form.state.suiteId}>
  <TaskSettings
    providerName={suites()?.find((s) => s.id === form.state.suiteId)?.provider_name ?? ""}
    config={form.state.providerConfig}
    onChange={(c) => form.setState("providerConfig", c)}
    taskCount={suites()?.find((s) => s.id === form.state.suiteId)?.task_count}
  />
</Show>
```

Import `TaskSettings` at top of file and `ProviderConfig` from types.

**Step 5: Add "Auto" option to model combobox**

In the ModelCombobox or model selector area, ensure `"auto"` is a valid option. If ModelCombobox doesn't support custom entries, add a checkbox or special entry above it:

```tsx
<label class="flex items-center gap-2 text-sm mb-2">
  <input
    type="checkbox"
    checked={form.state.model === "auto"}
    onChange={(e) => form.setState("model", e.currentTarget.checked ? "auto" : "")}
  />
  {t("benchmark.modelAuto")}
</label>
<Show when={form.state.model !== "auto"}>
  <ModelCombobox ... />
</Show>
```

**Step 6: Ensure suites resource is loaded**

Check that there's a `createResource` for suites. If the page already loads suites (for the Suites tab), reuse it. If not, add:

```typescript
const [suites] = createResource(() => api.benchmarks.listSuites());
```

**Step 7: Verify it compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: No errors

**Step 8: Commit**

```bash
git add frontend/src/features/benchmarks/BenchmarkPage.tsx
git commit -m "feat(benchmark): replace dataset dropdown with suite dropdown, add TaskSettings and auto-model option"
```

---

## Task 16: Frontend — RoutingReport Component

**Files:**
- Create: `frontend/src/features/benchmarks/RoutingReport.tsx`

**Step 1: Create the RoutingReport component**

```tsx
import { For, Show } from "solid-js";
import { Card } from "../../components";
import { useI18n } from "../../i18n";
import type { RoutingReport as RoutingReportType } from "../../api/types";

interface RoutingReportProps {
  report: RoutingReportType;
}

export function RoutingReport(props: RoutingReportProps) {
  const { t } = useI18n();

  const models = () => Object.entries(props.report.models_used).sort(([, a], [, b]) => b.task_count - a.task_count);
  const totalTasks = () => models().reduce((sum, [, s]) => sum + s.task_count, 0);

  return (
    <Card class="p-4 space-y-4">
      <h3 class="text-lg font-semibold">{t("benchmark.routingReport")}</h3>

      {/* Model Distribution Bar */}
      <div>
        <h4 class="text-sm font-medium mb-2">{t("benchmark.modelDistribution")}</h4>
        <div class="flex h-6 rounded overflow-hidden">
          <For each={models()}>
            {([name, stats], i) => (
              <div
                class="flex items-center justify-center text-xs text-white"
                style={{
                  width: `${stats.task_percentage}%`,
                  "background-color": `hsl(${i() * 60}, 70%, 50%)`,
                }}
                title={`${name}: ${stats.task_count} tasks (${stats.task_percentage.toFixed(1)}%)`}
              >
                {stats.task_percentage > 10 ? `${stats.task_percentage.toFixed(0)}%` : ""}
              </div>
            )}
          </For>
        </div>
      </div>

      {/* Per-model stats table */}
      <table class="w-full text-sm">
        <thead>
          <tr class="border-b text-left">
            <th class="py-1">Model</th>
            <th class="py-1 text-right">Tasks</th>
            <th class="py-1 text-right">Avg Score</th>
            <th class="py-1 text-right">Avg Cost</th>
          </tr>
        </thead>
        <tbody>
          <For each={models()}>
            {([name, stats]) => (
              <tr class="border-b border-[var(--color-border)]">
                <td class="py-1 font-mono text-xs">{name}</td>
                <td class="py-1 text-right">{stats.task_count} ({stats.task_percentage.toFixed(1)}%)</td>
                <td class="py-1 text-right">{(stats.avg_score * 100).toFixed(1)}%</td>
                <td class="py-1 text-right">${stats.avg_cost_per_task.toFixed(4)}</td>
              </tr>
            )}
          </For>
        </tbody>
      </table>

      {/* Fallback events */}
      <Show when={props.report.fallback_events > 0}>
        <div>
          <h4 class="text-sm font-medium mb-1">{t("benchmark.fallbackEvents")} ({props.report.fallback_events})</h4>
          <div class="space-y-1">
            <For each={props.report.fallback_details}>
              {(event) => (
                <div class="text-xs bg-[var(--color-surface-alt)] rounded p-2">
                  <span class="font-mono">{event.task_id}</span>: {event.primary} → {event.fallback_to}
                  <span class="text-[var(--color-text-secondary)]"> ({event.reason})</span>
                </div>
              )}
            </For>
          </div>
        </div>
      </Show>

      {/* Provider availability */}
      <div>
        <h4 class="text-sm font-medium mb-1">{t("benchmark.providerStatus")}</h4>
        <div class="flex gap-3 flex-wrap">
          <For each={Object.entries(props.report.provider_availability)}>
            {([name, status]) => (
              <div class="flex items-center gap-1 text-xs">
                <span
                  class="inline-block w-2 h-2 rounded-full"
                  classList={{
                    "bg-green-500": status.reachable && status.errors === 0,
                    "bg-yellow-500": status.reachable && status.errors > 0,
                    "bg-red-500": !status.reachable,
                  }}
                />
                <span>{name}</span>
                <Show when={status.errors > 0}>
                  <span class="text-[var(--color-text-secondary)]">({status.errors} errors)</span>
                </Show>
              </div>
            )}
          </For>
        </div>
      </div>

      {/* Summary */}
      <div class="flex gap-4 text-sm border-t pt-2">
        <span>System Score: <strong>{(props.report.system_score * 100).toFixed(1)}%</strong></span>
        <span>System Cost: <strong>${props.report.system_cost.toFixed(4)}</strong></span>
      </div>
    </Card>
  );
}
```

**Step 2: Verify TypeScript compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: No errors

**Step 3: Commit**

```bash
git add frontend/src/features/benchmarks/RoutingReport.tsx
git commit -m "feat(benchmark): RoutingReport component with model distribution, fallback timeline, provider status"
```

---

## Task 17: Frontend — Integrate RoutingReport into BenchmarkRunDetail

**Files:**
- Modify: `frontend/src/features/benchmarks/BenchmarkRunDetail.tsx`

**Step 1: Read BenchmarkRunDetail.tsx to understand structure**

Read the full file to find where run details and results are displayed.

**Step 2: Add RoutingReport section**

When the run model is "auto" and summary_scores contains routing_report data, render the RoutingReport component. Add after the summary section:

```tsx
<Show when={run()?.model === "auto" && run()?.summary_scores?.routing_report}>
  <RoutingReport report={run()!.summary_scores.routing_report as RoutingReportType} />
</Show>
```

Import `RoutingReport` from `./RoutingReport` and `RoutingReport as RoutingReportType` from types.

**Step 3: Verify it compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: No errors

**Step 4: Commit**

```bash
git add frontend/src/features/benchmarks/BenchmarkRunDetail.tsx
git commit -m "feat(benchmark): integrate RoutingReport into run detail view for auto-model runs"
```

---

## Task 18: Frontend — Update SuiteManagement for Provider Dropdown

**Files:**
- Modify: `frontend/src/features/benchmarks/SuiteManagement.tsx`

**Step 1: Read SuiteManagement.tsx**

Read the file to understand the current form structure.

**Step 2: Replace freetext provider_name with dropdown**

Replace the provider_name input with a select dropdown of known providers:

```tsx
const KNOWN_PROVIDERS = [
  { value: "codeforge_simple", label: "CodeForge Simple", type: "simple" },
  { value: "codeforge_tool_use", label: "CodeForge Tool Use", type: "tool_use" },
  { value: "codeforge_agent", label: "CodeForge Agent", type: "agent" },
  { value: "humaneval", label: "HumanEval", type: "simple" },
  { value: "mbpp", label: "MBPP", type: "simple" },
  { value: "swebench", label: "SWE-bench", type: "agent" },
  { value: "bigcodebench", label: "BigCodeBench", type: "simple" },
  { value: "cruxeval", label: "CRUXEval", type: "simple" },
  { value: "livecodebench", label: "LiveCodeBench", type: "simple" },
  { value: "sparcbench", label: "SPARCBench", type: "agent" },
  { value: "aider_polyglot", label: "Aider Polyglot", type: "agent" },
];
```

Auto-fill type when provider_name changes. Show lock icon for seeded (built-in) suites.

**Step 3: Verify it compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: No errors

**Step 4: Commit**

```bash
git add frontend/src/features/benchmarks/SuiteManagement.tsx
git commit -m "feat(benchmark): provider dropdown and auto-fill type in SuiteManagement"
```

---

## Task 19: Python — Auto-Routing Integration in Benchmark Consumer

**Files:**
- Modify: `workers/codeforge/consumer/_benchmark.py:68-93` (inside _handle_benchmark_run)

**Step 1: Add auto-routing logic**

In `_handle_benchmark_run`, after constructing `req` (line 69), before calling the runner (lines 87-92), add auto-routing logic:

When `req.model == "auto"`, wrap the LLM client to use HybridRouter per-call. The simplest approach: create a routing-aware wrapper that the runners use transparently. Add this before the runner dispatch:

```python
            # Auto-routing: wrap LLM with HybridRouter for per-task model selection
            effective_llm = self._llm
            if req.model == "auto":
                from codeforge.routing.hybrid_router import HybridRouter
                router = HybridRouter()
                effective_llm = _RoutingLLMWrapper(self._llm, router)
```

Then pass `effective_llm` instead of `self._llm` to the runner functions.

**Step 2: Create _RoutingLLMWrapper class**

At the bottom of the file, add a thin wrapper that intercepts `chat_completion()` to route per-call:

```python
class _RoutingLLMWrapper:
    """Wraps LiteLLMClient to inject per-call model selection via HybridRouter."""

    def __init__(self, inner, router):
        self._inner = inner
        self._router = router
        self.routing_log: list[dict] = []

    async def chat_completion(self, *, messages, model="auto", **kwargs):
        # Let the router pick the model
        selected = await self._router.select_model(
            task_input=messages[-1].get("content", "") if messages else "",
            scenario="benchmark",
        )
        model_name = selected.get("model", model)
        reason = selected.get("reason", "")

        try:
            result = await self._inner.chat_completion(messages=messages, model=model_name, **kwargs)
            self.routing_log.append({
                "model": model_name,
                "reason": reason,
                "fallback": False,
                "error": "",
            })
            return result
        except Exception as exc:
            # Record error and try fallback
            self.routing_log.append({
                "model": model_name,
                "reason": reason,
                "fallback": True,
                "error": str(exc),
            })
            raise

    def __getattr__(self, name):
        return getattr(self._inner, name)
```

**Step 3: After run completes, attach routing metadata to results**

After the runner returns results, if `req.model == "auto"`, annotate each result with routing info from the wrapper's log:

```python
            if req.model == "auto" and hasattr(effective_llm, "routing_log"):
                _annotate_routing(results, effective_llm.routing_log)
```

Add helper:

```python
def _annotate_routing(results: list, routing_log: list[dict]) -> None:
    """Attach routing metadata to benchmark results for auto-model tracking."""
    for i, r in enumerate(results):
        if i < len(routing_log):
            entry = routing_log[i]
            r["selected_model"] = entry.get("model", "")
            r["routing_reason"] = entry.get("reason", "")
            if entry.get("fallback"):
                r["fallback_count"] = 1
                r["provider_errors"] = entry.get("error", "")
```

**Step 4: Verify syntax**

Run: `cd /workspaces/CodeForge/workers && python -c "import codeforge.consumer._benchmark; print('OK')"`
Expected: `OK`

**Step 5: Commit**

```bash
git add workers/codeforge/consumer/_benchmark.py
git commit -m "feat(benchmark): auto-routing via HybridRouter wrapper in benchmark consumer"
```

---

## Task 20: Python — Compute Routing Report in BenchmarkRunResult

**Files:**
- Modify: `workers/codeforge/consumer/_benchmark.py` (after results are computed, before publishing)

**Step 1: Add _compute_routing_report function**

```python
def _compute_routing_report(results: list) -> dict:
    """Aggregate per-result routing data into a run-level report."""
    from collections import defaultdict

    models_used: dict[str, dict] = defaultdict(lambda: {
        "task_count": 0, "scores": [], "costs": [], "difficulties": defaultdict(int),
    })
    fallback_details = []
    provider_errors: dict[str, dict] = defaultdict(lambda: {"reachable": True, "errors": 0, "error_types": []})

    for r in results:
        model = r.get("selected_model", "unknown")
        m = models_used[model]
        m["task_count"] += 1
        scores = r.get("scores", {})
        if scores:
            m["scores"].append(sum(scores.values()) / len(scores))
        m["costs"].append(r.get("cost_usd", 0.0))
        m["difficulties"][r.get("difficulty", "medium")] += 1

        if r.get("fallback_count", 0) > 0:
            fallback_details.append({
                "task_id": r.get("task_id", ""),
                "primary": model,
                "fallback_to": model,  # simplified
                "reason": r.get("provider_errors", ""),
            })

        if r.get("provider_errors"):
            for provider in [model.split("/")[0]] if "/" in model else [model]:
                provider_errors[provider]["errors"] += 1
                provider_errors[provider]["error_types"].append(r.get("provider_errors", ""))

    total_tasks = sum(m["task_count"] for m in models_used.values())
    report = {
        "models_used": {},
        "fallback_events": len(fallback_details),
        "fallback_details": fallback_details[:20],  # cap at 20
        "provider_availability": dict(provider_errors),
        "system_score": 0.0,
        "system_cost": 0.0,
    }

    all_scores = []
    total_cost = 0.0
    for name, m in models_used.items():
        avg_score = sum(m["scores"]) / len(m["scores"]) if m["scores"] else 0.0
        avg_cost = sum(m["costs"]) / len(m["costs"]) if m["costs"] else 0.0
        all_scores.extend(m["scores"])
        total_cost += sum(m["costs"])
        report["models_used"][name] = {
            "task_count": m["task_count"],
            "task_percentage": round(m["task_count"] / total_tasks * 100, 1) if total_tasks else 0,
            "avg_score": round(avg_score, 4),
            "avg_cost_per_task": round(avg_cost, 6),
            "difficulty_distribution": dict(m["difficulties"]),
        }

    report["system_score"] = round(sum(all_scores) / len(all_scores), 4) if all_scores else 0.0
    report["system_cost"] = round(total_cost, 6)
    return report
```

**Step 2: Inject routing_report into summary**

In `_handle_benchmark_run`, after `summary = _compute_summary(results, elapsed_ms)` (line 95), add:

```python
            # Inject routing report for auto-model runs
            if req.model == "auto":
                routing_report = _compute_routing_report(
                    [r if isinstance(r, dict) else r.model_dump() for r in results]
                )
                summary["routing_report"] = routing_report
```

**Step 3: Verify syntax**

Run: `cd /workspaces/CodeForge/workers && python -c "import codeforge.consumer._benchmark; print('OK')"`
Expected: `OK`

**Step 4: Commit**

```bash
git add workers/codeforge/consumer/_benchmark.py
git commit -m "feat(benchmark): compute and inject routing_report into auto-model run summaries"
```

---

## Task 21: Go Domain — ModelFamily Utility

**Files:**
- Create: `internal/domain/benchmark/model_family.go`
- Test: `internal/domain/benchmark/model_family_test.go`

**Step 1: Write the failing test**

Create `internal/domain/benchmark/model_family_test.go`:

```go
package benchmark

import "testing"

func TestModelFamily(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		{"anthropic/claude-sonnet-4", "anthropic"},
		{"openai/gpt-4o", "openai"},
		{"mistral/mistral-large-latest", "mistral"},
		{"meta-llama/llama-3.1-70b", "meta-llama"},
		{"google/gemini-2.0-flash", "google"},
		{"ollama/llama3", "local"},
		{"lm-studio/model", "local"},
		{"claude-sonnet-4", "anthropic"},
		{"gpt-4o", "openai"},
		{"gemini-pro", "google"},
		{"unknown-model", "unknown"},
		{"", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := ModelFamily(tt.model); got != tt.expected {
				t.Errorf("ModelFamily(%q) = %q, want %q", tt.model, got, tt.expected)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && go test ./internal/domain/benchmark/ -run TestModelFamily -v`
Expected: FAIL (undefined: ModelFamily)

**Step 3: Write implementation**

Create `internal/domain/benchmark/model_family.go`:

```go
package benchmark

import "strings"

// ModelFamily extracts the model family from a model name string.
// Examples: "anthropic/claude-sonnet-4" -> "anthropic", "gpt-4o" -> "openai"
func ModelFamily(model string) string {
	if model == "" {
		return "unknown"
	}

	// Prefix-based: "provider/model"
	if idx := strings.Index(model, "/"); idx > 0 {
		prefix := model[:idx]
		switch prefix {
		case "anthropic":
			return "anthropic"
		case "openai":
			return "openai"
		case "mistral":
			return "mistral"
		case "meta-llama":
			return "meta-llama"
		case "google":
			return "google"
		case "ollama", "lm-studio":
			return "local"
		default:
			return prefix
		}
	}

	// Infer from model name prefix
	lower := strings.ToLower(model)
	switch {
	case strings.HasPrefix(lower, "claude"):
		return "anthropic"
	case strings.HasPrefix(lower, "gpt") || strings.HasPrefix(lower, "o1"):
		return "openai"
	case strings.HasPrefix(lower, "gemini"):
		return "google"
	case strings.HasPrefix(lower, "mistral") || strings.HasPrefix(lower, "mixtral"):
		return "mistral"
	case strings.HasPrefix(lower, "llama"):
		return "meta-llama"
	}
	return "unknown"
}
```

**Step 4: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/domain/benchmark/ -run TestModelFamily -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/domain/benchmark/model_family.go internal/domain/benchmark/model_family_test.go
git commit -m "feat(benchmark): ModelFamily utility for model name to family classification"
```

---

## Task 22: Go Service — Model Adaptation in Prompt Assembly

**Files:**
- Modify: `internal/service/conversation.go` or `internal/service/conversation_agent.go` (where system prompt is assembled)

**Step 1: Find prompt assembly location**

Search for where the system prompt is constructed before sending to NATS. Look for mode prompt concatenation.

**Step 2: Add model_adaptations layer**

After mode instructions are added to the prompt, check if the mode has `model_adaptations` and append the family-specific adaptation:

```go
// Layer 3: Model-family adaptation (if mode defines it)
if adaptations, ok := modeConfig["model_adaptations"].(map[string]interface{}); ok {
	family := benchmark.ModelFamily(model)
	if adaptation, ok := adaptations[family].(string); ok && adaptation != "" {
		systemPrompt += "\n\n" + adaptation
	}
}
```

This change is minimal — it only appends text if `model_adaptations.{family}` exists in the mode config.

**Step 3: Verify it compiles**

Run: `cd /workspaces/CodeForge && go build ./...`
Expected: Clean build

**Step 4: Commit**

```bash
git add internal/service/conversation_agent.go
git commit -m "feat(benchmark): inject model-family prompt adaptations from mode config (Layer 3)"
```

---

## Task 23: Python — Prompt Optimizer (Analyze Phase)

**Files:**
- Create: `workers/codeforge/evaluation/prompt_optimizer.py`
- Test: `workers/tests/evaluation/test_prompt_optimizer.py`

**Step 1: Write the failing test**

Create `workers/tests/evaluation/test_prompt_optimizer.py`:

```python
"""Tests for prompt optimizer analysis."""
import pytest
from codeforge.evaluation.prompt_optimizer import (
    PromptAnalysisReport,
    TacticalFix,
    analyze_failures,
)


def test_analyze_failures_returns_report():
    failures = [
        {
            "task_id": "t-1",
            "input": "Write a function to reverse a string",
            "expected_output": "def reverse(s): return s[::-1]",
            "actual_output": "def reverse(s): return reversed(s)",
            "scores": {"correctness": 0.3},
        },
    ]
    report = analyze_failures(
        failures=failures,
        mode="coder",
        model_family="meta-llama",
        llm_client=None,  # uses mock path
    )
    assert isinstance(report, PromptAnalysisReport)
    assert report.mode == "coder"
    assert report.model_family == "meta-llama"
    assert report.total_tasks >= 1
    assert report.failed_tasks >= 1


def test_tactical_fix_structure():
    fix = TacticalFix(
        task_id="t-1",
        failure_description="returned generator instead of string",
        root_cause="model confused reversed() with slicing",
        proposed_addition="Always use slice notation s[::-1] for string reversal",
        confidence=0.8,
    )
    assert fix.confidence == 0.8
    assert "slice" in fix.proposed_addition
```

**Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge/workers && python -m pytest tests/evaluation/test_prompt_optimizer.py -v`
Expected: FAIL with `ModuleNotFoundError`

**Step 3: Write implementation**

Create `workers/codeforge/evaluation/prompt_optimizer.py`:

```python
"""Prompt optimizer — analyzes benchmark failures and proposes prompt improvements.

Hybrid approach inspired by:
- SICA (Self-Improving Coding Agent, ICLR 2025)
- SCOPE (Prompt Evolution, Dec 2025)
- MIPROv2 Bootstrap (DSPy/Stanford)
"""
from __future__ import annotations

from dataclasses import dataclass, field


@dataclass
class TacticalFix:
    """SCOPE-style: specific failure -> specific fix."""

    task_id: str
    failure_description: str
    root_cause: str
    proposed_addition: str
    confidence: float


@dataclass
class PromptPatch:
    """A concrete prompt change to apply to a mode's YAML."""

    mode: str
    model_family: str
    patch_type: str  # "tactical" | "strategic" | "few_shot"
    action: str  # "add" | "replace" | "remove"
    content: str
    location: str  # "model_adaptations" | "prompt_template"
    rationale: str
    source_task_ids: list[str] = field(default_factory=list)


@dataclass
class PromptAnalysisReport:
    """Result of analyzing benchmark failures for a mode + model-family."""

    suite_id: str
    run_id: str
    mode: str
    model_family: str
    total_tasks: int
    failed_tasks: int
    failure_rate: float
    tactical_fixes: list[TacticalFix] = field(default_factory=list)
    strategic_principles: list[str] = field(default_factory=list)
    few_shot_candidates: list[str] = field(default_factory=list)


_ANALYSIS_PROMPT = """\
You are a prompt optimization expert. Analyze the following benchmark failures and suggest improvements.

Mode: {mode}
Model Family: {model_family}

## Failed Tasks ({count}):
{failures_text}

## Instructions:
1. Identify common failure patterns (tactical fixes)
2. Extract overarching principles (strategic improvements)
3. Suggest 1-3 successful patterns as few-shot candidates

Respond in JSON:
{{
  "tactical_fixes": [
    {{"task_id": "...", "failure_description": "...", "root_cause": "...", "proposed_addition": "...", "confidence": 0.8}}
  ],
  "strategic_principles": ["principle 1", "principle 2"],
  "few_shot_candidates": ["example trace 1"]
}}
"""


def analyze_failures(
    failures: list[dict],
    mode: str,
    model_family: str,
    llm_client=None,
    suite_id: str = "",
    run_id: str = "",
) -> PromptAnalysisReport:
    """Analyze benchmark failures and produce a PromptAnalysisReport.

    When llm_client is None, returns a basic structural report
    without LLM analysis (useful for testing).
    """
    total = len(failures)
    failed = sum(1 for f in failures if _is_failed(f))

    report = PromptAnalysisReport(
        suite_id=suite_id,
        run_id=run_id,
        mode=mode,
        model_family=model_family,
        total_tasks=total,
        failed_tasks=failed,
        failure_rate=failed / total if total > 0 else 0.0,
    )

    if llm_client is None:
        # Mock/test mode: return structural report without LLM analysis
        for f in failures:
            if _is_failed(f):
                report.tactical_fixes.append(
                    TacticalFix(
                        task_id=f.get("task_id", ""),
                        failure_description=f"Expected: {f.get('expected_output', '')[:50]}",
                        root_cause="Analysis requires LLM",
                        proposed_addition="",
                        confidence=0.0,
                    )
                )
        return report

    # Full LLM analysis would go here — call llm_client.chat_completion()
    # with _ANALYSIS_PROMPT formatted with failure data, parse JSON response
    return report


async def analyze_failures_async(
    failures: list[dict],
    mode: str,
    model_family: str,
    llm_client,
    suite_id: str = "",
    run_id: str = "",
) -> PromptAnalysisReport:
    """Async version that calls LLM for analysis."""
    import json as json_mod

    total = len(failures)
    failed_items = [f for f in failures if _is_failed(f)]

    failures_text = "\n".join(
        f"- Task {f.get('task_id')}: expected={f.get('expected_output', '')[:100]}, "
        f"got={f.get('actual_output', '')[:100]}, scores={f.get('scores', {})}"
        for f in failed_items[:20]  # cap at 20 failures for context window
    )

    prompt = _ANALYSIS_PROMPT.format(
        mode=mode,
        model_family=model_family,
        count=len(failed_items),
        failures_text=failures_text,
    )

    response = await llm_client.chat_completion(
        messages=[{"role": "user", "content": prompt}],
        model="auto",
    )

    # Parse LLM response
    content = response.get("choices", [{}])[0].get("message", {}).get("content", "{}")
    try:
        data = json_mod.loads(content)
    except json_mod.JSONDecodeError:
        data = {}

    report = PromptAnalysisReport(
        suite_id=suite_id,
        run_id=run_id,
        mode=mode,
        model_family=model_family,
        total_tasks=total,
        failed_tasks=len(failed_items),
        failure_rate=len(failed_items) / total if total > 0 else 0.0,
    )

    for fix_data in data.get("tactical_fixes", []):
        report.tactical_fixes.append(TacticalFix(**fix_data))

    report.strategic_principles = data.get("strategic_principles", [])
    report.few_shot_candidates = data.get("few_shot_candidates", [])

    return report


def _is_failed(result: dict) -> bool:
    """Determine if a benchmark result is a failure."""
    scores = result.get("scores", {})
    if not scores:
        return True
    avg = sum(scores.values()) / len(scores)
    return avg < 0.5
```

**Step 4: Run tests**

Run: `cd /workspaces/CodeForge/workers && python -m pytest tests/evaluation/test_prompt_optimizer.py -v`
Expected: All tests PASS

**Step 5: Commit**

```bash
git add workers/codeforge/evaluation/prompt_optimizer.py workers/tests/evaluation/test_prompt_optimizer.py
git commit -m "feat(benchmark): prompt optimizer with analyze phase (SICA+SCOPE+MIPROv2 hybrid)"
```

---

## Task 24: Frontend — PromptOptimizationPanel Component

**Files:**
- Create: `frontend/src/features/benchmarks/PromptOptimizationPanel.tsx`

**Step 1: Create the component**

This component shows analysis results and prompt patches with accept/reject controls:

```tsx
import { createResource, createSignal, For, Show } from "solid-js";
import { Button, Card } from "../../components";
import { useI18n } from "../../i18n";
import { api } from "../../api/client";

interface TacticalFix {
  task_id: string;
  failure_description: string;
  root_cause: string;
  proposed_addition: string;
  confidence: number;
}

interface AnalysisReport {
  mode: string;
  model_family: string;
  total_tasks: number;
  failed_tasks: number;
  failure_rate: number;
  tactical_fixes: TacticalFix[];
  strategic_principles: string[];
}

interface PromptOptimizationPanelProps {
  runId: string;
  suiteId: string;
}

export function PromptOptimizationPanel(props: PromptOptimizationPanelProps) {
  const { t } = useI18n();
  const [analyzing, setAnalyzing] = createSignal(false);
  const [report, setReport] = createSignal<AnalysisReport | null>(null);
  const [accepted, setAccepted] = createSignal<Set<string>>(new Set());

  const handleAnalyze = async () => {
    setAnalyzing(true);
    try {
      // POST /api/v1/benchmarks/runs/{id}/analyze
      const resp = await api.benchmarks.analyzeRun(props.runId);
      setReport(resp);
    } catch {
      // Handle error
    } finally {
      setAnalyzing(false);
    }
  };

  const toggleAccept = (taskId: string) => {
    const next = new Set(accepted());
    if (next.has(taskId)) {
      next.delete(taskId);
    } else {
      next.add(taskId);
    }
    setAccepted(next);
  };

  return (
    <Card class="p-4 space-y-4">
      <h3 class="text-lg font-semibold">{t("benchmark.promptOptimization")}</h3>

      <Show
        when={report()}
        fallback={
          <Button onClick={handleAnalyze} disabled={analyzing()} size="sm">
            {analyzing() ? "Analyzing..." : t("benchmark.analyzePrompts")}
          </Button>
        }
      >
        {(r) => (
          <div class="space-y-4">
            {/* Summary */}
            <div class="text-sm">
              <span>Mode: <strong>{r().mode}</strong></span>
              <span class="ml-4">Family: <strong>{r().model_family}</strong></span>
              <span class="ml-4">
                Failure rate: <strong>{(r().failure_rate * 100).toFixed(1)}%</strong>
                ({r().failed_tasks}/{r().total_tasks})
              </span>
            </div>

            {/* Strategic principles */}
            <Show when={r().strategic_principles.length > 0}>
              <div>
                <h4 class="text-sm font-medium mb-1">Strategic Principles</h4>
                <ul class="list-disc pl-5 text-sm space-y-1">
                  <For each={r().strategic_principles}>
                    {(p) => <li>{p}</li>}
                  </For>
                </ul>
              </div>
            </Show>

            {/* Tactical fixes */}
            <div>
              <h4 class="text-sm font-medium mb-2">{t("benchmark.promptDiff")}</h4>
              <div class="space-y-2">
                <For each={r().tactical_fixes}>
                  {(fix) => (
                    <div class="border rounded p-3 text-sm" classList={{
                      "border-green-500 bg-green-50 dark:bg-green-950": accepted().has(fix.task_id),
                    }}>
                      <div class="flex justify-between items-start">
                        <div>
                          <span class="font-mono text-xs">{fix.task_id}</span>
                          <span class="ml-2 text-[var(--color-text-secondary)]">
                            confidence: {(fix.confidence * 100).toFixed(0)}%
                          </span>
                        </div>
                        <div class="flex gap-1">
                          <Button
                            size="sm"
                            variant={accepted().has(fix.task_id) ? "primary" : "ghost"}
                            onClick={() => toggleAccept(fix.task_id)}
                          >
                            {t("benchmark.acceptPatch")}
                          </Button>
                        </div>
                      </div>
                      <p class="mt-1 text-[var(--color-text-secondary)]">{fix.failure_description}</p>
                      <p class="mt-1"><strong>Root cause:</strong> {fix.root_cause}</p>
                      <Show when={fix.proposed_addition}>
                        <pre class="mt-1 bg-green-100 dark:bg-green-900 p-2 rounded text-xs whitespace-pre-wrap">
                          + {fix.proposed_addition}
                        </pre>
                      </Show>
                    </div>
                  )}
                </For>
              </div>
            </div>

            {/* Re-run button */}
            <div class="flex gap-2 border-t pt-3">
              <Button size="sm" variant="primary" disabled={accepted().size === 0}>
                Apply {accepted().size} patches
              </Button>
              <Button size="sm">
                {t("benchmark.rerunBenchmark")}
              </Button>
            </div>
          </div>
        )}
      </Show>
    </Card>
  );
}
```

**Step 2: Verify TypeScript compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: No errors (the `api.benchmarks.analyzeRun` method will be wired in a follow-up — for now it can be typed as a placeholder)

**Step 3: Commit**

```bash
git add frontend/src/features/benchmarks/PromptOptimizationPanel.tsx
git commit -m "feat(benchmark): PromptOptimizationPanel with analyze, tactical fixes, and accept/reject UI"
```

---

## Task 25: Go HTTP — Prompt Optimization Endpoint

**Files:**
- Modify: `internal/adapter/http/routes.go:402-425` (benchmark routes)
- Modify: `internal/adapter/http/handlers_benchmark.go` (add analyze handler)

**Step 1: Add route**

In `routes.go`, within the benchmark routes group, add:

```go
r.Post("/runs/{id}/analyze", h.handleBenchmarkAnalyzeRun)
```

**Step 2: Add handler**

In `handlers_benchmark.go`, add:

```go
func (h *Handler) handleBenchmarkAnalyzeRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	if runID == "" {
		writeJSONError(w, http.StatusBadRequest, "run id required")
		return
	}

	// Load run and results
	run, err := h.benchmarkSvc.GetRun(r.Context(), runID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "run not found")
		return
	}

	results, err := h.benchmarkSvc.ListResults(r.Context(), runID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load results")
		return
	}

	// Convert results to failure dicts for Python analysis
	// NOTE: Implemented — returns structural report (see handlers_benchmark.go)
	failures := make([]map[string]interface{}, 0)
	for _, res := range results {
		failures = append(failures, map[string]interface{}{
			"task_id":         res.TaskID,
			"actual_output":   res.ActualOutput,
			"expected_output": res.ExpectedOutput,
			"scores":          res.Scores,
		})
	}

	// NOTE: Structural report implemented; NATS dispatch deferred to Phase 28+
	report := map[string]interface{}{
		"run_id":               runID,
		"mode":                 "coder",
		"model_family":         benchmark.ModelFamily(run.Model),
		"total_tasks":          len(results),
		"failed_tasks":         countFailed(results),
		"failure_rate":         0.0,
		"tactical_fixes":       []interface{}{},
		"strategic_principles": []string{},
	}
	if len(results) > 0 {
		report["failure_rate"] = float64(countFailed(results)) / float64(len(results))
	}

	writeJSON(w, http.StatusOK, report)
}

func countFailed(results []benchmark.Result) int {
	count := 0
	for _, r := range results {
		var scores map[string]float64
		_ = json.Unmarshal(r.Scores, &scores)
		if len(scores) == 0 {
			count++
			continue
		}
		var total float64
		for _, v := range scores {
			total += v
		}
		if total/float64(len(scores)) < 0.5 {
			count++
		}
	}
	return count
}
```

**Step 3: Verify it compiles**

Run: `cd /workspaces/CodeForge && go build ./...`
Expected: Clean build

**Step 4: Commit**

```bash
git add internal/adapter/http/routes.go internal/adapter/http/handlers_benchmark.go
git commit -m "feat(benchmark): add POST /runs/{id}/analyze endpoint for prompt optimization"
```

---

## Task 26: Frontend API Client — Add analyzeRun Method

**Files:**
- Modify: `frontend/src/api/client.ts` (benchmarks section)

**Step 1: Find the benchmarks API section**

Look for `benchmarks:` or `createRun` in client.ts.

**Step 2: Add analyzeRun method**

```typescript
analyzeRun: (runId: string) =>
  fetchJSON<AnalysisReport>(`/api/v1/benchmarks/runs/${runId}/analyze`, { method: "POST" }),
```

Add `AnalysisReport` type to types.ts or use inline type that matches the PromptOptimizationPanel expectations.

**Step 3: Verify it compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: No errors

**Step 4: Commit**

```bash
git add frontend/src/api/client.ts frontend/src/api/types.ts
git commit -m "feat(benchmark): add analyzeRun API client method"
```

---

## Task 27: Frontend — Wire PromptOptimizationPanel into BenchmarkRunDetail

**Files:**
- Modify: `frontend/src/features/benchmarks/BenchmarkRunDetail.tsx`

**Step 1: Add PromptOptimizationPanel to run detail**

After the results section (or after the RoutingReport), add:

```tsx
<Show when={run()?.status === "completed"}>
  <PromptOptimizationPanel
    runId={run()!.id}
    suiteId={run()!.suite_id ?? ""}
  />
</Show>
```

Import `PromptOptimizationPanel` from `./PromptOptimizationPanel`.

**Step 2: Verify it compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: No errors

**Step 3: Commit**

```bash
git add frontend/src/features/benchmarks/BenchmarkRunDetail.tsx
git commit -m "feat(benchmark): wire PromptOptimizationPanel into completed run detail view"
```

---

## Task 28: Documentation Updates

**Files:**
- Modify: `docs/todo.md` — add tasks for remaining optimization features
- Modify: `docs/project-status.md` — add new section
- Modify: `docs/features/04-agent-orchestration.md` — mention prompt optimization

**Step 1: Update project-status.md**

Add new section after last completed phase:

```markdown
### External Benchmark Providers, Full-Auto Routing & Prompt Optimization (IN PROGRESS)

Suite-based unified architecture (8 external providers: HumanEval, MBPP, SWE-bench, BigCodeBench, CRUXEval, LiveCodeBench, SPARCBench, Aider Polyglot), provider config with universal task filters, full-auto model routing with per-result tracking and routing report, prompt optimization feedback loop (analyze phase), model-family prompt adaptations, frontend TaskSettings/RoutingReport/PromptOptimizationPanel components.
```

**Step 2: Update todo.md**

Mark completed tasks, add remaining items:
- `[x]` External provider data flow (suite -> NATS -> Python provider registry)
- `[x]` Routing fields migration + domain model changes
- `[x]` Frontend suite dropdown + task settings
- `[ ]` Full LLM-backed prompt analysis (NATS dispatch to Python)
- `[ ]` Prompt patch application to mode YAML via API
- `[ ]` Auto-iterate optimization loop (Phase B+C)
- `[ ]` Benchmark E2E tests for external providers

**Step 3: Commit**

```bash
git add docs/project-status.md docs/todo.md
git commit -m "docs: update project status and todo for benchmark external providers phase"
```

---

## Summary

| Task | Scope | Description |
|------|-------|-------------|
| 1 | DB | Migration 068: routing columns on benchmark_results |
| 2 | Go Domain | Routing fields on Result, ProviderConfig on CreateRunRequest |
| 3 | Go Store | UPDATE INSERT/SELECT for routing columns |
| 4 | Go NATS | ProviderName/Config + routing fields on payloads |
| 5 | Python Models | Mirror NATS payload changes |
| 6 | Go Service | Suite-based StartRun with provider_name resolution |
| 7 | Go Service | Suite seeding on startup (11 default suites) |
| 8 | Python | Universal task filter function + tests |
| 9 | Python | Add config param to 8 external provider constructors |
| 10 | Python | Auto-register all providers in __init__.py |
| 11 | Python Consumer | Provider-based task loading with legacy fallback |
| 12 | Frontend Types | ProviderConfig, routing fields, RoutingReport interfaces |
| 13 | Frontend i18n | 25 new keys (EN + DE) |
| 14 | Frontend | TaskSettings component |
| 15 | Frontend | Suite dropdown replacing dataset dropdown |
| 16 | Frontend | RoutingReport component |
| 17 | Frontend | Integrate RoutingReport in run detail |
| 18 | Frontend | SuiteManagement provider dropdown |
| 19 | Python Consumer | Auto-routing via HybridRouter wrapper |
| 20 | Python Consumer | Compute routing_report in summary |
| 21 | Go Domain | ModelFamily utility + tests |
| 22 | Go Service | Model adaptation prompt injection |
| 23 | Python | Prompt optimizer (Analyze phase) + tests |
| 24 | Frontend | PromptOptimizationPanel component |
| 25 | Go HTTP | POST /runs/{id}/analyze endpoint |
| 26 | Frontend API | analyzeRun client method |
| 27 | Frontend | Wire PromptOptimizationPanel into RunDetail |
| 28 | Docs | Update project-status.md + todo.md |

**Dependency chain:**
- Tasks 1-5 (data layer) must complete before Tasks 6-7 (service) and 11 (consumer)
- Tasks 8-10 (Python providers) must complete before Task 11 (consumer)
- Task 12 (types) must complete before Tasks 14-18 (frontend components)
- Tasks 19-20 depend on Task 11
- Task 21 should complete before Task 22
- Tasks 23-27 (prompt optimization) are independent of routing (Tasks 19-20)
- Task 28 (docs) runs last

**Parallelizable groups:**
- Group A: Tasks 1-5 (data layer, sequential)
- Group B: Tasks 8-10 (Python providers, can parallelize with Group A)
- Group C: Tasks 12-13 (frontend foundation, after Group A)
- Group D: Tasks 14-18 (frontend components, after Group C)
- Group E: Tasks 19-22 (routing + model family, after Group A+B)
- Group F: Tasks 23-27 (prompt optimization, after Group A)
