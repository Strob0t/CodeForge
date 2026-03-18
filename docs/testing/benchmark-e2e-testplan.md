# Benchmark System E2E Test Plan

Comprehensive, reusable interactive test plan for the CodeForge benchmark evaluation
system (Phase 20 + 26 + 28). Covers all 3 benchmark types, all 4 evaluators, all 12
datasets/suites (4 local + 8 external HuggingFace), advanced features, task filtering,
comparison/analysis endpoints, error scenarios, and suite CRUD.

**Pipeline under test:**
Go Core -> NATS JetStream -> Python Worker -> LiteLLM -> LLM -> Evaluators -> DB -> API

**Philosophy:** Validate pipeline correctness, not model quality. Scores may be 0 with
local models due to context limits -- that is expected and acceptable.

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Setup Variables](#setup-variables)
3. [Task Filtering (provider_config)](#task-filtering-provider_config)
4. [Phase 0: Infrastructure Verification](#phase-0-infrastructure-verification)
5. [Phase 1: Simple Benchmarks](#phase-1-simple-benchmarks-6-runs)
6. [Phase 2: Tool-Use Benchmarks](#phase-2-tool-use-benchmarks-3-runs)
7. [Phase 3: Agent Benchmarks](#phase-3-agent-benchmarks-5-runs)
8. [Phase 3b: External Suite Runs](#phase-3b-external-suite-runs-8-runs)
9. [Phase 4: Advanced Features](#phase-4-advanced-features-3-runs)
10. [Phase 5: Comparison and Analysis](#phase-5-comparison--analysis-endpoints-12-checks)
11. [Phase 6: Error Scenarios and Task Filter Verification](#phase-6-error-scenarios--task-filter-verification-8-runs)
12. [Phase 7: Suite CRUD](#phase-7-suite-crud-6-checks)
13. [Coverage Matrices](#coverage-matrices)
14. [Summary Table](#summary-table)

---

## Prerequisites

```bash
# 1. Start Docker infrastructure
docker compose up -d postgres nats litellm

# 2. Start Go backend in dev mode (required for /api/v1/benchmarks/*)
APP_ENV=development go run ./cmd/codeforge/

# 3. Start Python worker
cd workers && poetry run python -m codeforge.consumer &

# 4. Start LM Studio with a loaded model (e.g. qwen3-30b-a3b)

# 5. (Optional) Start frontend for visual verification
cd frontend && npm run dev
```

**Required services checklist:**

- [ ] PostgreSQL (port 5432)
- [ ] NATS JetStream (port 4222)
- [ ] LiteLLM proxy (port 4000)
- [ ] Go backend (port 8080, APP_ENV=development)
- [ ] Python worker (codeforge.consumer)
- [ ] LM Studio (or other local model server)

---

## Setup Variables

Run these once at the start of a test session. All subsequent curl commands reference
`$TOKEN` and `$MODEL`.

```bash
# Authenticate
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@localhost","password":"Changeme123"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

# Resolve model name from LiteLLM
MODEL=$(curl -s -H "Authorization: Bearer sk-codeforge-dev" \
  http://codeforge-litellm:4000/v1/models \
  | python3 -c "import sys,json; ms=[m['id'] for m in json.load(sys.stdin)['data'] if 'lm_studio' in m['id'] and '*' not in m['id'] and 'embed' not in m['id']]; print(ms[0] if ms else 'NONE')")
echo "Using model: $MODEL"

# API base URL
API=http://localhost:8080/api/v1

# Helper: create a benchmark run and return the run ID
create_run() {
  curl -s -X POST "$API/benchmarks/runs" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "$1" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('id','ERROR: '+str(d)))"
}

# Helper: poll until run completes (timeout 10 min)
wait_run() {
  local RUN_ID=$1
  local TIMEOUT=${2:-600}
  local START=$(date +%s)
  while true; do
    STATUS=$(curl -s -H "Authorization: Bearer $TOKEN" "$API/benchmarks/runs/$RUN_ID" \
      | python3 -c "import sys,json; print(json.load(sys.stdin)['status'])")
    if [ "$STATUS" = "completed" ] || [ "$STATUS" = "failed" ]; then
      echo "$STATUS"
      return
    fi
    NOW=$(date +%s)
    if [ $((NOW - START)) -ge $TIMEOUT ]; then
      echo "TIMEOUT (still $STATUS)"
      return
    fi
    sleep 5
  done
}

# Helper: get result count for a run
result_count() {
  curl -s -H "Authorization: Bearer $TOKEN" "$API/benchmarks/runs/$1/results" \
    | python3 -c "import sys,json; print(len(json.load(sys.stdin)))"
}

# Helper: print run summary
run_summary() {
  curl -s -H "Authorization: Bearer $TOKEN" "$API/benchmarks/runs/$1" \
    | python3 -c "
import sys,json
r=json.load(sys.stdin)
print(f\"  Status: {r['status']}\")
print(f\"  Cost: \${r.get('total_cost',0):.4f}\")
print(f\"  Tokens: {r.get('total_tokens',0)}\")
print(f\"  Duration: {r.get('total_duration_ms',0)}ms\")
print(f\"  Error: {r.get('error_message','')}\")
"
}
```

---

## Task Filtering (provider_config)

External suites contain 100-2294 tasks. **Never run them without filtering.**
The universal task filter (`workers/codeforge/evaluation/task_filter.py`) applies after
`provider.load_tasks()` in the consumer.

### Supported Options

| Option | Type | Example | Effect |
|---|---|---|---|
| `max_tasks` | int | `3` | Absolute cap on task count (0 = unlimited) |
| `task_percentage` | int/float | `10` | Percentage of total tasks (100 = all) |
| `difficulty_filter` | list[str] | `["easy"]` | Only tasks matching these difficulty levels |
| `shuffle` | bool | `true` (default) | Randomize task order before capping |
| `seed` | int | `42` (default) | Deterministic shuffle for reproducibility |

When both `max_tasks` AND `task_percentage` are set, the **more restrictive wins**.

### Pipeline Flow

1. Go API receives `CreateRunRequest.ProviderConfig` (JSON)
2. Go Service merges suite config + request overrides (`mergeProviderConfig()`)
3. NATS message carries `provider_config` to Python worker
4. Consumer creates provider with `config=req.provider_config`
5. Consumer calls `apply_task_filters(tasks, req.provider_config)` -- filtering here

### Key Files

- `workers/codeforge/evaluation/task_filter.py` -- filter implementation
- `workers/codeforge/consumer/_benchmark.py:330-346` -- consumer integration
- `internal/service/benchmark.go` -- Go-side `mergeProviderConfig()`

---

## Phase 0: Infrastructure Verification

No LLM calls. Verifies all components are healthy.

### [0.1] Backend healthy + dev mode

```bash
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/health | python3 -m json.tool
```

- [ ] **Assert:** `status` = `"ok"`, `dev_mode` = `true`

### [0.2] LiteLLM proxy alive

```bash
curl -s http://codeforge-litellm:4000/health/liveliness
```

- [ ] **Assert:** Response contains `"I'm alive!"`

### [0.3] LM Studio model available

```bash
curl -s -H "Authorization: Bearer sk-codeforge-dev" \
  http://codeforge-litellm:4000/v1/models \
  | python3 -c "import sys,json; [print(m['id']) for m in json.load(sys.stdin)['data'] if 'lm_studio' in m['id'] and '*' not in m['id']]"
```

- [ ] **Assert:** At least one non-wildcard `lm_studio/` model listed

### [0.4] Datasets listed

```bash
curl -s -H "Authorization: Bearer $TOKEN" "$API/benchmarks/datasets" | python3 -m json.tool
```

- [ ] **Assert:** 4 datasets: basic-coding, tool-use-basic, agent-coding, e2e-quick

### [0.5] Suites seeded

```bash
curl -s -H "Authorization: Bearer $TOKEN" "$API/benchmarks/suites" \
  | python3 -c "import sys,json; suites=json.load(sys.stdin); print(f'{len(suites)} suites'); [print(f'  {s[\"provider_name\"]:20s} {s[\"type\"]:10s} {s[\"id\"]}') for s in suites]"
```

- [ ] **Assert:** >= 11 suites (3 codeforge + 8 external)

---

## Phase 1: Simple Benchmarks (6 runs)

All use `benchmark_type: "simple"`.

### [1.1] e2e-quick + llm_judge (basic pipeline)

```bash
RUN_1_1=$(create_run '{
  "dataset": "e2e-quick", "model": "'$MODEL'",
  "metrics": ["llm_judge"], "benchmark_type": "simple"
}')
echo "Run 1.1: $RUN_1_1"
STATUS=$(wait_run $RUN_1_1); echo "Status: $STATUS"
run_summary $RUN_1_1
echo "Results: $(result_count $RUN_1_1)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 2
- [ ] **Assert:** each result has `llm_judge` key in scores

### [1.2] e2e-quick + functional_test (graceful degradation)

```bash
RUN_1_2=$(create_run '{
  "dataset": "e2e-quick", "model": "'$MODEL'",
  "metrics": ["functional_test"], "benchmark_type": "simple"
}')
echo "Run 1.2: $RUN_1_2"
STATUS=$(wait_run $RUN_1_2); echo "Status: $STATUS"
run_summary $RUN_1_2
echo "Results: $(result_count $RUN_1_2)"
```

- [ ] **Assert:** status = `completed` (NOT crashed)
- [ ] **Assert:** result count = 2
- [ ] **Assert:** scores contain `functional_test` key (score may be 0)

### [1.3] e2e-quick + llm_judge + functional_test (combined)

```bash
RUN_1_3=$(create_run '{
  "dataset": "e2e-quick", "model": "'$MODEL'",
  "metrics": ["llm_judge", "functional_test"], "benchmark_type": "simple"
}')
echo "Run 1.3: $RUN_1_3"
STATUS=$(wait_run $RUN_1_3); echo "Status: $STATUS"
run_summary $RUN_1_3
echo "Results: $(result_count $RUN_1_3)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 2
- [ ] **Assert:** scores contain BOTH `llm_judge` AND `functional_test` keys

### [1.4] basic-coding + llm_judge (larger dataset)

```bash
RUN_1_4=$(create_run '{
  "dataset": "basic-coding", "model": "'$MODEL'",
  "metrics": ["llm_judge"], "benchmark_type": "simple"
}')
echo "Run 1.4: $RUN_1_4"
STATUS=$(wait_run $RUN_1_4); echo "Status: $STATUS"
run_summary $RUN_1_4
echo "Results: $(result_count $RUN_1_4)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 5

### [1.5] basic-coding + llm_judge + functional_test

```bash
RUN_1_5=$(create_run '{
  "dataset": "basic-coding", "model": "'$MODEL'",
  "metrics": ["llm_judge", "functional_test"], "benchmark_type": "simple"
}')
echo "Run 1.5: $RUN_1_5"
STATUS=$(wait_run $RUN_1_5); echo "Status: $STATUS"
run_summary $RUN_1_5
echo "Results: $(result_count $RUN_1_5)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 5

### [1.6] e2e-quick + llm_judge + suite_id (suite-linked)

```bash
SUITE_SIMPLE=$(curl -s -H "Authorization: Bearer $TOKEN" "$API/benchmarks/suites" \
  | python3 -c "import sys,json; print([s['id'] for s in json.load(sys.stdin) if s['provider_name']=='codeforge_simple'][0])")
echo "Suite ID: $SUITE_SIMPLE"

RUN_1_6=$(create_run '{
  "dataset": "e2e-quick", "model": "'$MODEL'",
  "metrics": ["llm_judge"], "benchmark_type": "simple",
  "suite_id": "'$SUITE_SIMPLE'"
}')
echo "Run 1.6: $RUN_1_6"
STATUS=$(wait_run $RUN_1_6); echo "Status: $STATUS"
run_summary $RUN_1_6
echo "Results: $(result_count $RUN_1_6)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 2
- [ ] **Assert:** run's `suite_id` field matches `$SUITE_SIMPLE`

---

## Phase 2: Tool-Use Benchmarks (3 runs)

All use `benchmark_type: "tool_use"`.

### [2.1] e2e-quick + llm_judge (tool-use type)

```bash
RUN_2_1=$(create_run '{
  "dataset": "e2e-quick", "model": "'$MODEL'",
  "metrics": ["llm_judge"], "benchmark_type": "tool_use"
}')
echo "Run 2.1: $RUN_2_1"
STATUS=$(wait_run $RUN_2_1); echo "Status: $STATUS"
run_summary $RUN_2_1
echo "Results: $(result_count $RUN_2_1)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 2

### [2.2] tool-use-basic + llm_judge

```bash
RUN_2_2=$(create_run '{
  "dataset": "tool-use-basic", "model": "'$MODEL'",
  "metrics": ["llm_judge"], "benchmark_type": "tool_use"
}')
echo "Run 2.2: $RUN_2_2"
STATUS=$(wait_run $RUN_2_2); echo "Status: $STATUS"
run_summary $RUN_2_2
echo "Results: $(result_count $RUN_2_2)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 3

### [2.3] tool-use-basic + llm_judge + functional_test

```bash
RUN_2_3=$(create_run '{
  "dataset": "tool-use-basic", "model": "'$MODEL'",
  "metrics": ["llm_judge", "functional_test"], "benchmark_type": "tool_use"
}')
echo "Run 2.3: $RUN_2_3"
STATUS=$(wait_run $RUN_2_3); echo "Status: $STATUS"
run_summary $RUN_2_3
echo "Results: $(result_count $RUN_2_3)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 3
- [ ] **Assert:** scores contain both `llm_judge` and `functional_test`

---

## Phase 3: Agent Benchmarks (5 runs)

All use `benchmark_type: "agent"`, `exec_mode: "mount"`, dataset `agent-coding` (5 tasks).
These are the longest-running -- full agent loop with file creation/editing.

### [3.1] agent-coding + llm_judge + trajectory_verifier

```bash
RUN_3_1=$(create_run '{
  "dataset": "agent-coding", "model": "'$MODEL'",
  "metrics": ["llm_judge", "trajectory_verifier"],
  "benchmark_type": "agent", "exec_mode": "mount"
}')
echo "Run 3.1: $RUN_3_1"
STATUS=$(wait_run $RUN_3_1 900); echo "Status: $STATUS"
run_summary $RUN_3_1
echo "Results: $(result_count $RUN_3_1)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 5
- [ ] **Assert:** scores contain `trajectory_verifier` aggregated key

### [3.2] agent-coding + llm_judge + sparc

```bash
RUN_3_2=$(create_run '{
  "dataset": "agent-coding", "model": "'$MODEL'",
  "metrics": ["llm_judge", "sparc"],
  "benchmark_type": "agent", "exec_mode": "mount"
}')
echo "Run 3.2: $RUN_3_2"
STATUS=$(wait_run $RUN_3_2 900); echo "Status: $STATUS"
run_summary $RUN_3_2
echo "Results: $(result_count $RUN_3_2)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 5
- [ ] **Assert:** scores contain `sparc` aggregated key

### [3.3] agent-coding + functional_test

```bash
RUN_3_3=$(create_run '{
  "dataset": "agent-coding", "model": "'$MODEL'",
  "metrics": ["functional_test"],
  "benchmark_type": "agent", "exec_mode": "mount"
}')
echo "Run 3.3: $RUN_3_3"
STATUS=$(wait_run $RUN_3_3 900); echo "Status: $STATUS"
run_summary $RUN_3_3
echo "Results: $(result_count $RUN_3_3)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 5

### [3.4] agent-coding + sparc only

```bash
RUN_3_4=$(create_run '{
  "dataset": "agent-coding", "model": "'$MODEL'",
  "metrics": ["sparc"],
  "benchmark_type": "agent", "exec_mode": "mount"
}')
echo "Run 3.4: $RUN_3_4"
STATUS=$(wait_run $RUN_3_4 900); echo "Status: $STATUS"
run_summary $RUN_3_4
echo "Results: $(result_count $RUN_3_4)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 5

### [3.5] agent-coding + ALL 4 evaluators

```bash
RUN_3_5=$(create_run '{
  "dataset": "agent-coding", "model": "'$MODEL'",
  "metrics": ["llm_judge", "functional_test", "sparc", "trajectory_verifier"],
  "benchmark_type": "agent", "exec_mode": "mount"
}')
echo "Run 3.5: $RUN_3_5"
STATUS=$(wait_run $RUN_3_5 900); echo "Status: $STATUS"
run_summary $RUN_3_5
echo "Results: $(result_count $RUN_3_5)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 5
- [ ] **Assert:** scores contain ALL keys: `llm_judge`, `functional_test`, `sparc`, `trajectory_verifier`

---

## Phase 3b: External Suite Runs (8 runs)

Each external suite runs with `max_tasks: 3` via `provider_config`.
Tasks are downloaded from HuggingFace on first run (cached afterwards).
Uses `suite_id` (NOT `dataset` field) -- the provider loads tasks internally.

### Resolve Suite IDs

```bash
# Get all suite IDs at once
eval $(curl -s -H "Authorization: Bearer $TOKEN" "$API/benchmarks/suites" \
  | python3 -c "
import sys,json
for s in json.load(sys.stdin):
    name = s['provider_name'].upper()
    print(f'SUITE_{name}={s[\"id\"]}')
")
echo "humaneval=$SUITE_HUMANEVAL"
echo "mbpp=$SUITE_MBPP"
echo "bigcodebench=$SUITE_BIGCODEBENCH"
echo "cruxeval=$SUITE_CRUXEVAL"
echo "livecodebench=$SUITE_LIVECODEBENCH"
echo "swebench=$SUITE_SWEBENCH"
echo "sparcbench=$SUITE_SPARCBENCH"
echo "aider_polyglot=$SUITE_AIDER_POLYGLOT"
```

### [3b.1] HumanEval (simple, 3 tasks)

```bash
RUN_3B_1=$(create_run '{
  "dataset": "", "model": "'$MODEL'",
  "metrics": ["llm_judge", "functional_test"], "benchmark_type": "simple",
  "suite_id": "'$SUITE_HUMANEVAL'",
  "provider_config": {"max_tasks": 3}
}')
echo "Run 3b.1: $RUN_3B_1"
STATUS=$(wait_run $RUN_3B_1); echo "Status: $STATUS"
run_summary $RUN_3B_1
echo "Results: $(result_count $RUN_3B_1)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 3 (NOT 164)

### [3b.2] MBPP (simple, 3 tasks)

```bash
RUN_3B_2=$(create_run '{
  "dataset": "", "model": "'$MODEL'",
  "metrics": ["llm_judge", "functional_test"], "benchmark_type": "simple",
  "suite_id": "'$SUITE_MBPP'",
  "provider_config": {"max_tasks": 3}
}')
echo "Run 3b.2: $RUN_3B_2"
STATUS=$(wait_run $RUN_3B_2); echo "Status: $STATUS"
run_summary $RUN_3B_2
echo "Results: $(result_count $RUN_3B_2)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 3 (NOT 427)

### [3b.3] BigCodeBench (simple, 3 tasks)

```bash
RUN_3B_3=$(create_run '{
  "dataset": "", "model": "'$MODEL'",
  "metrics": ["llm_judge"], "benchmark_type": "simple",
  "suite_id": "'$SUITE_BIGCODEBENCH'",
  "provider_config": {"max_tasks": 3}
}')
echo "Run 3b.3: $RUN_3B_3"
STATUS=$(wait_run $RUN_3B_3); echo "Status: $STATUS"
run_summary $RUN_3B_3
echo "Results: $(result_count $RUN_3B_3)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 3 (NOT 1140)

### [3b.4] CRUXEval (simple, 3 tasks)

```bash
RUN_3B_4=$(create_run '{
  "dataset": "", "model": "'$MODEL'",
  "metrics": ["llm_judge"], "benchmark_type": "simple",
  "suite_id": "'$SUITE_CRUXEVAL'",
  "provider_config": {"max_tasks": 3}
}')
echo "Run 3b.4: $RUN_3B_4"
STATUS=$(wait_run $RUN_3B_4); echo "Status: $STATUS"
run_summary $RUN_3B_4
echo "Results: $(result_count $RUN_3B_4)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 3 (NOT 800)

### [3b.5] LiveCodeBench (simple, 3 tasks)

```bash
RUN_3B_5=$(create_run '{
  "dataset": "", "model": "'$MODEL'",
  "metrics": ["llm_judge"], "benchmark_type": "simple",
  "suite_id": "'$SUITE_LIVECODEBENCH'",
  "provider_config": {"max_tasks": 3}
}')
echo "Run 3b.5: $RUN_3B_5"
STATUS=$(wait_run $RUN_3B_5); echo "Status: $STATUS"
run_summary $RUN_3B_5
echo "Results: $(result_count $RUN_3B_5)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 3 (NOT 300+)

### [3b.6] SWE-bench (agent, 3 tasks)

```bash
RUN_3B_6=$(create_run '{
  "dataset": "", "model": "'$MODEL'",
  "metrics": ["llm_judge", "functional_test"],
  "benchmark_type": "agent", "exec_mode": "mount",
  "suite_id": "'$SUITE_SWEBENCH'",
  "provider_config": {"max_tasks": 3}
}')
echo "Run 3b.6: $RUN_3B_6"
STATUS=$(wait_run $RUN_3B_6 900); echo "Status: $STATUS"
run_summary $RUN_3B_6
echo "Results: $(result_count $RUN_3B_6)"
```

- [ ] **Assert:** status = `completed` OR `failed` (workspace setup may fail)
- [ ] **Assert:** if completed, result count <= 3
- [ ] **Assert:** if failed, `error_message` is populated

### [3b.7] SPARCBench (agent, 3 tasks)

```bash
RUN_3B_7=$(create_run '{
  "dataset": "", "model": "'$MODEL'",
  "metrics": ["llm_judge", "sparc"],
  "benchmark_type": "agent", "exec_mode": "mount",
  "suite_id": "'$SUITE_SPARCBENCH'",
  "provider_config": {"max_tasks": 3}
}')
echo "Run 3b.7: $RUN_3B_7"
STATUS=$(wait_run $RUN_3B_7 900); echo "Status: $STATUS"
run_summary $RUN_3B_7
echo "Results: $(result_count $RUN_3B_7)"
```

- [ ] **Assert:** status = `completed` OR `failed`
- [ ] **Assert:** if completed, result count <= 3

### [3b.8] Aider Polyglot (agent, 3 tasks)

```bash
RUN_3B_8=$(create_run '{
  "dataset": "", "model": "'$MODEL'",
  "metrics": ["llm_judge", "functional_test"],
  "benchmark_type": "agent", "exec_mode": "mount",
  "suite_id": "'$SUITE_AIDER_POLYGLOT'",
  "provider_config": {"max_tasks": 3}
}')
echo "Run 3b.8: $RUN_3B_8"
STATUS=$(wait_run $RUN_3B_8 900); echo "Status: $STATUS"
run_summary $RUN_3B_8
echo "Results: $(result_count $RUN_3B_8)"
```

- [ ] **Assert:** status = `completed` OR `failed`
- [ ] **Assert:** if completed, result count <= 3

---

## Phase 4: Advanced Features (3 runs)

### [4.1] Multi-rollout (best-of-3)

```bash
RUN_4_1=$(create_run '{
  "dataset": "e2e-quick", "model": "'$MODEL'",
  "metrics": ["llm_judge"], "benchmark_type": "simple",
  "rollout_count": 3, "rollout_strategy": "best"
}')
echo "Run 4.1: $RUN_4_1"
STATUS=$(wait_run $RUN_4_1); echo "Status: $STATUS"
run_summary $RUN_4_1
echo "Results: $(result_count $RUN_4_1)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 6 (2 tasks x 3 rollouts)
- [ ] **Assert:** at least one result per task has `is_best_rollout: true`

### [4.2] Multi-rollout (diversity)

```bash
RUN_4_2=$(create_run '{
  "dataset": "e2e-quick", "model": "'$MODEL'",
  "metrics": ["llm_judge"], "benchmark_type": "simple",
  "rollout_count": 2, "rollout_strategy": "diversity"
}')
echo "Run 4.2: $RUN_4_2"
STATUS=$(wait_run $RUN_4_2); echo "Status: $STATUS"
run_summary $RUN_4_2
echo "Results: $(result_count $RUN_4_2)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 4 (2 tasks x 2 rollouts)
- [ ] **Assert:** results have `diversity_score` populated

### [4.3] Hybrid verification

```bash
RUN_4_3=$(create_run '{
  "dataset": "e2e-quick", "model": "'$MODEL'",
  "metrics": ["functional_test", "llm_judge"], "benchmark_type": "simple",
  "hybrid_verification": true
}')
echo "Run 4.3: $RUN_4_3"
STATUS=$(wait_run $RUN_4_3); echo "Status: $STATUS"
run_summary $RUN_4_3
echo "Results: $(result_count $RUN_4_3)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 2
- [ ] **Assert:** `hybrid_verification` flag is true on the run

---

## Phase 5: Comparison & Analysis Endpoints (12 checks)

No new LLM calls. Uses run IDs from Phases 1-4.

### [5.1] Two-run compare

```bash
curl -s -X POST "$API/benchmarks/compare" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"run_id_a": "'$RUN_1_1'", "run_id_b": "'$RUN_1_4'"}' \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'run_a: {bool(d.get(\"run_a\"))}, run_b: {bool(d.get(\"run_b\"))}, results_a: {len(d.get(\"results_a\",[]))}, results_b: {len(d.get(\"results_b\",[]))}')"
```

- [ ] **Assert:** `run_a`, `run_b`, `results_a`, `results_b` all present

### [5.2] Multi-compare

```bash
curl -s -X POST "$API/benchmarks/compare-multi" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"run_ids": ["'$RUN_1_1'", "'$RUN_1_4'", "'$RUN_1_5'"]}' \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'Entries: {len(d)}')"
```

- [ ] **Assert:** returns array with 3 entries

### [5.3] Cost analysis

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "$API/benchmarks/runs/$RUN_3_5/cost-analysis" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'total_cost: {d.get(\"total_cost_usd\",0):.4f}, tasks: {len(d.get(\"task_breakdown\",[]))}')"
```

- [ ] **Assert:** `total_cost_usd` present, `task_breakdown` has entries

### [5.4] Leaderboard (unfiltered)

```bash
curl -s -H "Authorization: Bearer $TOKEN" "$API/benchmarks/leaderboard" \
  | python3 -c "import sys,json; entries=json.load(sys.stdin); print(f'Entries: {len(entries)}'); [print(f'  {e[\"model\"]:40s} score={e[\"avg_score\"]:.2f}') for e in entries[:5]]"
```

- [ ] **Assert:** >= 1 entry with model matching `$MODEL`

### [5.5] Leaderboard (filtered by suite)

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "$API/benchmarks/leaderboard?suite_id=$SUITE_SIMPLE" \
  | python3 -c "import sys,json; entries=json.load(sys.stdin); print(f'Filtered entries: {len(entries)}')"
```

- [ ] **Assert:** returns filtered entries

### [5.6] Run analysis

```bash
curl -s -X POST "$API/benchmarks/runs/$RUN_3_5/analyze" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'failure_rate: {d.get(\"failure_rate\")}, model_family: {d.get(\"model_family\")}, total_tasks: {d.get(\"total_tasks\")}')"
```

- [ ] **Assert:** `failure_rate`, `model_family`, `total_tasks` present

### [5.7] Export JSON

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "$API/benchmarks/runs/$RUN_1_4/export/results" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'Exported {len(d)} results (JSON)')"
```

- [ ] **Assert:** JSON array with 5 entries

### [5.8] Export CSV

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "$API/benchmarks/runs/$RUN_1_4/export/results?format=csv" \
  | head -2
```

- [ ] **Assert:** valid CSV with header row

### [5.9] Export training data

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "$API/benchmarks/runs/$RUN_4_1/export/training" \
  | head -3
```

- [ ] **Assert:** JSONL with chosen/rejected pairs

### [5.10] Filter runs by status

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "$API/benchmarks/runs?status=completed" \
  | python3 -c "import sys,json; runs=json.load(sys.stdin); statuses=set(r['status'] for r in runs); print(f'Runs: {len(runs)}, statuses: {statuses}')"
```

- [ ] **Assert:** all returned runs have `status=completed`

### [5.11] Filter runs by model

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "$API/benchmarks/runs?model=$MODEL" \
  | python3 -c "import sys,json; runs=json.load(sys.stdin); models=set(r['model'] for r in runs); print(f'Runs: {len(runs)}, models: {models}')"
```

- [ ] **Assert:** all returned runs have our model

### [5.12] Filter runs by benchmark type

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "$API/benchmarks/runs?benchmark_type=agent" \
  | python3 -c "import sys,json; runs=json.load(sys.stdin); types=set(r.get('benchmark_type','') for r in runs); print(f'Runs: {len(runs)}, types: {types}')"
```

- [ ] **Assert:** all returned runs have `benchmark_type=agent`

---

## Phase 6: Error Scenarios & Task Filter Verification (8 runs)

### [6.1] Invalid dataset

```bash
RESP_6_1=$(curl -s -w "\n%{http_code}" -X POST "$API/benchmarks/runs" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"dataset": "nonexistent-xyz", "model": "'$MODEL'", "metrics": ["llm_judge"], "benchmark_type": "simple"}')
echo "$RESP_6_1"
```

- [ ] **Assert:** HTTP error (4xx) OR run created with status = `failed` + `error_message`

### [6.2] Invalid model

```bash
RUN_6_2=$(create_run '{
  "dataset": "e2e-quick", "model": "nonexistent/model-xyz",
  "metrics": ["llm_judge"], "benchmark_type": "simple"
}')
echo "Run 6.2: $RUN_6_2"
STATUS=$(wait_run $RUN_6_2 120); echo "Status: $STATUS"
run_summary $RUN_6_2
```

- [ ] **Assert:** status = `failed`
- [ ] **Assert:** `error_message` mentions model validation

### [6.3] Missing required field (no model)

```bash
RESP_6_3=$(curl -s -w "\n%{http_code}" -X POST "$API/benchmarks/runs" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"dataset": "e2e-quick", "metrics": ["llm_judge"]}')
echo "$RESP_6_3"
```

- [ ] **Assert:** HTTP 400

### [6.4] Unknown evaluator

```bash
RUN_6_4=$(create_run '{
  "dataset": "e2e-quick", "model": "'$MODEL'",
  "metrics": ["nonexistent_evaluator"], "benchmark_type": "simple"
}')
echo "Run 6.4: $RUN_6_4"
STATUS=$(wait_run $RUN_6_4 120); echo "Status: $STATUS"
run_summary $RUN_6_4
```

- [ ] **Assert:** graceful handling (completed with empty scores OR failed cleanly)

### [6.5] Cancel running run

```bash
RUN_6_5=$(create_run '{
  "dataset": "basic-coding", "model": "'$MODEL'",
  "metrics": ["llm_judge"], "benchmark_type": "simple"
}')
echo "Run 6.5: $RUN_6_5"
sleep 2
curl -s -X PATCH "$API/benchmarks/runs/$RUN_6_5" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"status": "failed"}' | python3 -m json.tool
STATUS=$(wait_run $RUN_6_5 60); echo "Status: $STATUS"
```

- [ ] **Assert:** status transitions to `failed`

### [6.6] task_percentage filter (1% of HumanEval = ~2 tasks)

```bash
RUN_6_6=$(create_run '{
  "dataset": "", "model": "'$MODEL'",
  "metrics": ["llm_judge"], "benchmark_type": "simple",
  "suite_id": "'$SUITE_HUMANEVAL'",
  "provider_config": {"task_percentage": 1}
}')
echo "Run 6.6: $RUN_6_6"
STATUS=$(wait_run $RUN_6_6); echo "Status: $STATUS"
RESULTS=$(result_count $RUN_6_6)
echo "Results: $RESULTS (expected ~2, NOT 164)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count ~ 2 (ceil(164 * 0.01) = 2)
- [ ] **Assert:** result count << 164 (task_percentage works)

### [6.7] max_tasks + task_percentage combined (more restrictive wins)

```bash
RUN_6_7=$(create_run '{
  "dataset": "", "model": "'$MODEL'",
  "metrics": ["llm_judge"], "benchmark_type": "simple",
  "suite_id": "'$SUITE_HUMANEVAL'",
  "provider_config": {"max_tasks": 3, "task_percentage": 50}
}')
echo "Run 6.7: $RUN_6_7"
STATUS=$(wait_run $RUN_6_7); echo "Status: $STATUS"
RESULTS=$(result_count $RUN_6_7)
echo "Results: $RESULTS (expected 3: 50% of 164 = 82, capped by max_tasks=3)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count = 3

### [6.8] difficulty_filter + max_tasks

```bash
RUN_6_8=$(create_run '{
  "dataset": "", "model": "'$MODEL'",
  "metrics": ["llm_judge"], "benchmark_type": "simple",
  "suite_id": "'$SUITE_HUMANEVAL'",
  "provider_config": {"max_tasks": 3, "difficulty_filter": ["easy"]}
}')
echo "Run 6.8: $RUN_6_8"
STATUS=$(wait_run $RUN_6_8); echo "Status: $STATUS"
RESULTS=$(result_count $RUN_6_8)
echo "Results: $RESULTS (expected <= 3, all easy tasks)"
```

- [ ] **Assert:** status = `completed`
- [ ] **Assert:** result count <= 3

---

## Phase 7: Suite CRUD (6 checks)

No LLM calls. Tests CRUD operations on benchmark suites.

### [7.1] Create custom suite

```bash
SUITE_RESP=$(curl -s -X POST "$API/benchmarks/suites" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "E2E Test Suite", "type": "simple", "provider_name": "e2e_test_provider", "description": "Created by E2E test"}')
echo "$SUITE_RESP" | python3 -m json.tool
CUSTOM_SUITE_ID=$(echo "$SUITE_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
echo "Created suite: $CUSTOM_SUITE_ID"
```

- [ ] **Assert:** HTTP 201, suite created with ID

### [7.2] Get suite

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "$API/benchmarks/suites/$CUSTOM_SUITE_ID" | python3 -m json.tool
```

- [ ] **Assert:** HTTP 200, matches created suite

### [7.3] Update suite

```bash
curl -s -X PUT "$API/benchmarks/suites/$CUSTOM_SUITE_ID" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "E2E Test Suite Updated", "type": "simple", "provider_name": "e2e_test_provider", "description": "Updated by E2E test"}' \
  | python3 -m json.tool
```

- [ ] **Assert:** HTTP 200, description updated

### [7.4] List suites (includes new)

```bash
curl -s -H "Authorization: Bearer $TOKEN" "$API/benchmarks/suites" \
  | python3 -c "import sys,json; suites=json.load(sys.stdin); found=[s for s in suites if s.get('provider_name')=='e2e_test_provider']; print(f'Found: {len(found)}')"
```

- [ ] **Assert:** new suite appears in list

### [7.5] Delete suite

```bash
curl -s -X DELETE "$API/benchmarks/suites/$CUSTOM_SUITE_ID" \
  -H "Authorization: Bearer $TOKEN" -w "\nHTTP %{http_code}\n"
```

- [ ] **Assert:** HTTP 200

### [7.6] Get deleted suite (404)

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "$API/benchmarks/suites/$CUSTOM_SUITE_ID" -w "\nHTTP %{http_code}\n"
```

- [ ] **Assert:** HTTP 404

---

## Coverage Matrices

### Evaluator x Type

| Evaluator | simple | tool_use | agent |
|---|---|---|---|
| **llm_judge** | 1.1, 1.3, 1.4, 1.5, 1.6, 3b.1-3b.5 | 2.1, 2.2, 2.3 | 3.1, 3.2, 3.5, 3b.6-3b.8 |
| **functional_test** | 1.2, 1.3, 1.5, 3b.1, 3b.2 | 2.3 | 3.3, 3.5, 3b.6, 3b.8 |
| **sparc** | -- | -- | 3.2, 3.4, 3.5, 3b.7 |
| **trajectory_verifier** | -- | -- | 3.1, 3.5 |

### Dataset / Suite x Type

| Dataset / Suite | simple | tool_use | agent |
|---|---|---|---|
| e2e-quick (2, local) | 1.1, 1.2, 1.3, 1.6, 4.1-4.3 | 2.1 | -- |
| basic-coding (5, local) | 1.4, 1.5 | -- | -- |
| tool-use-basic (3, local) | -- | 2.2, 2.3 | -- |
| agent-coding (5, local) | -- | -- | 3.1-3.5 |
| humaneval (3, HF) | 3b.1, 6.6-6.8 | -- | -- |
| mbpp (3, HF) | 3b.2 | -- | -- |
| bigcodebench (3, HF) | 3b.3 | -- | -- |
| cruxeval (3, HF) | 3b.4 | -- | -- |
| livecodebench (3, HF) | 3b.5 | -- | -- |
| swebench (3, HF) | -- | -- | 3b.6 |
| sparcbench (3, HF) | -- | -- | 3b.7 |
| aider_polyglot (3, HF) | -- | -- | 3b.8 |

### Advanced Features

| Feature | Run | Key Parameters |
|---|---|---|
| Suite-linked run | 1.6 | `suite_id` |
| Multi-rollout (best) | 4.1 | `rollout_count: 3, rollout_strategy: best` |
| Multi-rollout (diversity) | 4.2 | `rollout_count: 2, rollout_strategy: diversity` |
| Hybrid verification | 4.3 | `hybrid_verification: true` |
| Cancel run | 6.5 | `PATCH /runs/{id}` |
| task_percentage filter | 6.6 | `provider_config: {task_percentage: 1}` |
| Combined filters | 6.7 | `provider_config: {max_tasks:3, task_percentage:50}` |
| difficulty_filter | 6.8 | `provider_config: {max_tasks:3, difficulty_filter:["easy"]}` |
| Two-run compare | 5.1 | `POST /compare` |
| N-run compare | 5.2 | `POST /compare-multi` |
| Cost analysis | 5.3 | `GET /cost-analysis` |
| Leaderboard | 5.4-5.5 | `GET /leaderboard` |
| Run analysis | 5.6 | `POST /analyze` |
| Export JSON/CSV | 5.7-5.8 | `GET /export/results` |
| Export training | 5.9 | `GET /export/training` |
| Filter by status/model/type | 5.10-5.12 | query parameters |

---

## Summary Table

| Phase | Runs | LLM Calls | Est. Time |
|---|---|---|---|
| 0: Infrastructure | 0 | 0 | 1 min |
| 1: Simple (local) | 6 | ~18 | 5-10 min |
| 2: Tool-Use (local) | 3 | ~8 | 3-6 min |
| 3: Agent (local) | 5 | ~50 | 15-50 min |
| 3b: External Suites (3 each) | 8 | ~24 | 25-50 min |
| 4: Advanced Features | 3 | ~12 | 5-10 min |
| 5: Compare/Analysis | 0 | 0 | 1 min |
| 6: Errors + Filters | 8 | ~12 | 5 min |
| 7: Suite CRUD | 0 | 0 | 1 min |
| **Total** | **33** | **~122** | **~60-130 min** |

---

## Key Files Reference

| Component | Path |
|---|---|
| API handlers | `internal/adapter/http/handlers_benchmark.go` |
| Analysis handler | `internal/adapter/http/handlers_benchmark_analyze.go` |
| Routes | `internal/adapter/http/routes.go:420-444` |
| Domain types | `internal/domain/benchmark/benchmark.go` |
| Service logic | `internal/service/benchmark.go` |
| NATS schemas | `internal/port/messagequeue/schemas.go` |
| Python consumer | `workers/codeforge/consumer/_benchmark.py` |
| Task filter | `workers/codeforge/evaluation/task_filter.py` |
| Evaluators | `workers/codeforge/evaluation/evaluators/` |
| Providers | `workers/codeforge/evaluation/providers/` |
| Local datasets | `configs/benchmarks/*.yaml` |
| E2E test suite | `frontend/e2e/benchmark-validation/` |
| Bug findings | `frontend/e2e/benchmark-validation/FINDINGS.md` |

---

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Run stuck in `running` | Worker not connected to NATS | Restart Python worker, check NATS health |
| `403` on benchmark endpoints | `APP_ENV` not set to `development` | Restart backend with `APP_ENV=development` |
| Model validation error | Model name mismatch | Check `$MODEL` matches LiteLLM `/v1/models` |
| Score = 0 on all tasks | Context overflow with local model | Expected behavior -- validates pipeline, not quality |
| External suite timeout | HuggingFace download on first run | Wait longer, check internet connectivity |
| `error_message: dataset not found` | Invalid dataset name | Use exact names from `GET /benchmarks/datasets` |
| Run failed with no error | Check Python worker logs | `docker compose logs` or `cat /tmp/python-worker.log` |
