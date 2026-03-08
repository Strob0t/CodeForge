# Integration Testing Strategy -- Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Verify that all 30 CodeForge features work together across Go, Python, and Frontend layers by fixing broken tests, adding contract tests, unit tests for untested modules, smoke tests, and a verification reporter.

**Architecture:** Three-layer test pyramid: Contract Tests (fast, no Docker) at the base, Smoke Tests (full stack) in the middle, Feature Verification Matrix as the tracking layer. Tasks are grouped into 5 parallel tracks that can be executed by independent subagents.

**Tech Stack:** Go 1.25 (table-driven tests, testify-free), Python 3.12 (pytest, Pydantic), Bash (verification reporter), GitHub Actions (CI)

---

## Parallelization Strategy

```
Track 1 (Go):     B1 -> B2-Go -> B3-Go-Adapters -> B3-Go-Domain -> A1+A2
Track 2 (Python):  B2-Py -> B3-Py-Tools -> B3-Py-Consumer -> B3-Py-Memory
Track 3 (CI/Script): C2-Reporter -> A3-CI -> C3-Gate

Dependencies:
- B1 blocks nothing (migration fix is isolated)
- B2-Go generates JSON fixtures that B2-Py consumes (B2-Go before B2-Py)
- B3 tracks are fully independent (Go adapters, Go domain, Py tools, Py consumer, Py memory)
- A1+A2 smoke tests need B1 fixed (migration must work)
- C2 reporter needs some tests to exist first (run after B2+B3)
- A3 CI needs smoke tests written (run after A1+A2)
```

Five independent subagent tracks:

| Track | Scope | Files Modified | Depends On |
|-------|-------|---------------|------------|
| **Track 1** | B1 (migration fix) + B2-Go (contract fixtures) | 2 files | Nothing |
| **Track 2** | B3-Py-Tools (7 tool test files) | 7 new test files | Nothing |
| **Track 3** | B3-Py-Consumer (12 handler tests) + B3-Py-Memory (4 tests) | 2 new test files | Nothing |
| **Track 4** | B3-Go-Adapters (6 adapter tests) + B3-Go-Domain (5 domain tests) | 11 new test files | Nothing |
| **Track 5** | B2-Py (contract validator) -- runs AFTER Track 1 generates fixtures | 1 new test file | Track 1 |

After Tracks 1-5 complete:

| Track | Scope | Files Modified | Depends On |
|-------|-------|---------------|------------|
| **Track 6** | A1+A2 (smoke tests) | 2 new test files | Track 1 (migration fix) |
| **Track 7** | C2 (verification reporter) + A3 (CI) + C3 (gate) | 2 new files | Tracks 1-5 |

---

## Track 1: B1 Migration Fix + B2 Go Contract Fixture Generator

### Task 1.1: Fix Migration 065 (idempotent ADD COLUMN)

**Files:**
- Modify: `internal/adapter/postgres/migrations/065_benchmark_result_rollout_fields.sql`

**Root cause:** `ALTER TABLE benchmark_results ADD COLUMN rollout_id` fails when column already exists. Tests run migrations against a shared DB where an earlier migration (055) already added these columns.

**Step 1: Fix migration with IF NOT EXISTS**

Replace the Up section with idempotent `DO $$ ... END $$` blocks:

```sql
-- +goose Up
-- Phase 28C: Add rollout fields to benchmark_results and benchmark_runs.

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='benchmark_results' AND column_name='rollout_id') THEN
    ALTER TABLE benchmark_results ADD COLUMN rollout_id INTEGER NOT NULL DEFAULT 0;
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='benchmark_results' AND column_name='rollout_count') THEN
    ALTER TABLE benchmark_results ADD COLUMN rollout_count INTEGER NOT NULL DEFAULT 1;
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='benchmark_results' AND column_name='is_best_rollout') THEN
    ALTER TABLE benchmark_results ADD COLUMN is_best_rollout BOOLEAN NOT NULL DEFAULT TRUE;
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='benchmark_results' AND column_name='diversity_score') THEN
    ALTER TABLE benchmark_results ADD COLUMN diversity_score DOUBLE PRECISION NOT NULL DEFAULT 0.0;
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='benchmark_runs' AND column_name='hybrid_verification') THEN
    ALTER TABLE benchmark_runs ADD COLUMN hybrid_verification BOOLEAN NOT NULL DEFAULT FALSE;
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='benchmark_runs' AND column_name='rollout_count') THEN
    ALTER TABLE benchmark_runs ADD COLUMN rollout_count INTEGER NOT NULL DEFAULT 1;
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='benchmark_runs' AND column_name='rollout_strategy') THEN
    ALTER TABLE benchmark_runs ADD COLUMN rollout_strategy TEXT NOT NULL DEFAULT 'best';
  END IF;
END $$;
```

Keep the Down section unchanged (already uses `DROP COLUMN IF EXISTS`).

**Step 2: Verify all 10 tests pass**

Run: `go test ./internal/adapter/postgres/ -count=1 -v 2>&1 | grep -E "PASS|FAIL"`
Expected: All 10 previously-failing tests now PASS.

**Step 3: Verify full Go test suite**

Run: `go test ./internal/... -count=1 2>&1 | tail -5`
Expected: All packages PASS, no FAIL lines.

**Step 4: Commit**

```bash
git add internal/adapter/postgres/migrations/065_benchmark_result_rollout_fields.sql
git commit -m "fix(migration): make 065 idempotent with IF NOT EXISTS

Migration 065 failed when columns already existed from migration 055.
Added IF NOT EXISTS guards to all ADD COLUMN statements.
Fixes 10 failing Postgres store tests."
```

### Task 1.2: Create Go Contract Test Fixture Generator

**Files:**
- Create: `internal/port/messagequeue/contract_test.go`
- Create: `internal/port/messagequeue/testdata/contracts/` (directory with JSON files, auto-generated by test)

**Step 1: Write the contract fixture generator test**

This test marshals every NATS payload struct to JSON with realistic sample data and writes to `testdata/contracts/{name}.json`. It also round-trips (marshal -> unmarshal -> compare).

```go
//go:build !smoke

package messagequeue_test

import (
    "encoding/json"
    "os"
    "path/filepath"
    "testing"

    mq "github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// contractFixture pairs a subject name with a populated payload for fixture generation.
type contractFixture struct {
    Subject string
    Payload any
}

func allFixtures() []contractFixture {
    return []contractFixture{
        {"conversation.run.start", sampleConversationRunStart()},
        {"conversation.run.complete", sampleConversationRunComplete()},
        {"benchmark.run.request", sampleBenchmarkRunRequest()},
        {"benchmark.run.result", sampleBenchmarkRunResult()},
        {"evaluation.gemmas.request", sampleGemmasEvalRequest()},
        {"evaluation.gemmas.result", sampleGemmasEvalResult()},
        {"repomap.generate.request", sampleRepoMapRequest()},
        {"repomap.generate.result", sampleRepoMapResult()},
        {"retrieval.index.request", sampleRetrievalIndexRequest()},
        {"retrieval.index.result", sampleRetrievalIndexResult()},
        {"retrieval.search.request", sampleRetrievalSearchRequest()},
        {"retrieval.search.result", sampleRetrievalSearchResult()},
        {"retrieval.subagent.request", sampleSubAgentSearchRequest()},
        {"retrieval.subagent.result", sampleSubAgentSearchResult()},
        {"graph.build.request", sampleGraphBuildRequest()},
        {"graph.build.result", sampleGraphBuildResult()},
        {"graph.search.request", sampleGraphSearchRequest()},
        {"graph.search.result", sampleGraphSearchResult()},
        {"a2a.task.created", sampleA2ATaskCreated()},
        {"a2a.task.complete", sampleA2ATaskComplete()},
    }
}

func TestContract_GenerateFixtures(t *testing.T) {
    dir := filepath.Join("testdata", "contracts")
    if err := os.MkdirAll(dir, 0o755); err != nil {
        t.Fatalf("create fixtures dir: %v", err)
    }

    for _, f := range allFixtures() {
        t.Run(f.Subject, func(t *testing.T) {
            data, err := json.MarshalIndent(f.Payload, "", "  ")
            if err != nil {
                t.Fatalf("marshal %s: %v", f.Subject, err)
            }
            path := filepath.Join(dir, f.Subject+".json")
            if err := os.WriteFile(path, data, 0o644); err != nil {
                t.Fatalf("write fixture %s: %v", path, err)
            }
            t.Logf("wrote %s (%d bytes)", path, len(data))
        })
    }
}

func TestContract_RoundTrip(t *testing.T) {
    for _, f := range allFixtures() {
        t.Run(f.Subject, func(t *testing.T) {
            data, err := json.Marshal(f.Payload)
            if err != nil {
                t.Fatalf("marshal: %v", err)
            }
            // Unmarshal into a generic map to verify all fields serialize
            var m map[string]any
            if err := json.Unmarshal(data, &m); err != nil {
                t.Fatalf("unmarshal to map: %v", err)
            }
            // Verify key fields exist (non-exhaustive, catches missing json tags)
            if _, ok := m["run_id"]; f.Subject == "conversation.run.start" && !ok {
                t.Error("conversation.run.start missing run_id")
            }
        })
    }
}
```

For each `sample*()` function, create a helper that returns the struct with all fields populated with realistic non-zero values. The subagent implementing this should read `internal/port/messagequeue/schemas.go` to build complete sample data for every struct.

**Key sample functions to implement:**
- `sampleConversationRunStart()` -- must set `Agentic: true`, include MCPServers, Context, MicroagentPrompts
- `sampleConversationRunComplete()` -- must include ToolMessages array, cost fields
- `sampleBenchmarkRunRequest()` -- must set DatasetPath as absolute, include Evaluators
- `sampleBenchmarkRunResult()` -- must include Results with Scores map, rollout fields
- All other 16 sample functions following the same pattern

**Step 2: Run test to generate fixtures**

Run: `go test ./internal/port/messagequeue/ -run Contract -v -count=1`
Expected: All fixtures written to `testdata/contracts/*.json`, all round-trip tests PASS.

**Step 3: Commit**

```bash
git add internal/port/messagequeue/contract_test.go internal/port/messagequeue/testdata/
git commit -m "test(contract): add NATS payload fixture generator

Generates JSON fixtures for all 20 NATS payload types.
Round-trip test verifies marshal/unmarshal consistency.
Fixtures consumed by Python contract validator (test_nats_contracts.py)."
```

---

## Track 2: B3 Python Agent Tool Tests

> Fully independent -- no dependencies on other tracks.
> Each tool gets its own test file for parallel development.

### Task 2.1: Test Agent Tool Base + Registry

**Files:**
- Read: `workers/codeforge/tools/_base.py`, `workers/codeforge/tools/__init__.py`
- Create: `workers/tests/test_tool_registry.py`

Write tests for:
- Tool base class instantiation, `execute()` interface
- Tool registry: register, lookup by name, list all
- Duplicate tool name prevention
- Tool result formatting: success, error, truncation

### Task 2.2: Test Read File Tool

**Files:**
- Read: `workers/codeforge/tools/read_file.py`
- Create: `workers/tests/test_tool_read_file.py`

Write tests for:
- Read existing file (use `tmp_path` fixture)
- Read non-existent file -> error
- Read with offset + limit (line range)
- Path traversal attempt (e.g., `../../etc/passwd`) -> error
- Binary file handling
- Empty file -> empty result
- Large file -> output truncated

### Task 2.3: Test Write File Tool

**Files:**
- Read: `workers/codeforge/tools/write_file.py`
- Create: `workers/tests/test_tool_write_file.py`

Write tests for:
- Write new file (creates parent dirs)
- Overwrite existing file
- Empty content -> creates empty file
- Path traversal blocked
- Write to read-only location -> error

### Task 2.4: Test Edit File Tool

**Files:**
- Read: `workers/codeforge/tools/edit_file.py`
- Create: `workers/tests/test_tool_edit_file.py`

Write tests for:
- Exact string replacement (unique match)
- No match found -> error with helpful message
- Multiple matches (non-unique) -> error
- `replace_all=True` -> all occurrences replaced
- Multi-line edit preserving indentation
- Empty old_string -> error

### Task 2.5: Test Bash Tool

**Files:**
- Read: `workers/codeforge/tools/bash.py`
- Create: `workers/tests/test_tool_bash.py`

Write tests for:
- Simple command (`echo hello`) -> output
- Command with exit code != 0 -> error with stderr
- Timeout enforcement (use `sleep 100` with 1s timeout)
- Output truncation (command producing >100KB)
- Dangerous command detection (if implemented)
- Working directory argument

### Task 2.6: Test Search + Glob + ListDir Tools

**Files:**
- Read: `workers/codeforge/tools/search_files.py`, `glob_files.py`, `list_directory.py`
- Create: `workers/tests/test_tool_search_glob_listdir.py`

Write tests for:
- **Search:** regex pattern match, no matches, case sensitivity
- **Glob:** `**/*.py` pattern, no matches, directory scope
- **ListDir:** valid dir, non-existent dir, empty dir, hidden files

### Task 2.7: Run all tool tests and verify

Run: `cd /workspaces/CodeForge/workers && poetry run pytest tests/test_tool_*.py -v`
Expected: All tests PASS.

Commit all tool test files together.

---

## Track 3: B3 Python Consumer Dispatch + Memory Tests

> Fully independent -- no dependencies on other tracks.

### Task 3.1: Test Consumer Dispatch Logic

**Files:**
- Read: `workers/codeforge/consumer/_conversation.py`, `_benchmark.py`, `_retrieval.py`, `_graph.py`, `_memory.py`, `_a2a.py`, `_handoff.py`, `_repomap.py`, `_base.py`
- Create: `workers/tests/test_consumer_dispatch.py`

Write tests using `AsyncMock` for the NATS connection:
- **Conversation dispatch:** `agentic=True` routes to agent loop, `agentic=False` routes to simple chat
- **Benchmark dispatch:** routes by `benchmark_type` (simple/tool_use/agent)
- **Retrieval dispatch:** index vs search based on subject
- **Graph dispatch:** build vs search based on subject
- **Memory dispatch:** store vs recall based on subject
- **A2A dispatch:** task created -> handler called
- **Handoff dispatch:** request -> handler called
- **Duplicate detection:** same `run_id` twice -> second call skipped
- **Error handling:** handler raises exception -> error published to result subject, `msg.ack()` called
- **Subject registration:** verify all expected subjects are subscribed

### Task 3.2: Test Memory System

**Files:**
- Read: `workers/codeforge/memory/scorer.py`, `workers/codeforge/memory/experience.py`
- Create: `workers/tests/test_memory_system.py`

Write tests for:
- **Scorer:** composite score = (semantic * w1) + (recency * w2) + (importance * w3)
  - All weights zero -> score 0
  - Only semantic -> proportional to similarity
  - Old memories -> lower recency score
  - High importance -> boosted score
- **Experience pool:** `@exp_cache` decorator
  - First call -> cache miss -> function executed
  - Second call with same args -> cache hit -> function NOT executed
  - Different args -> cache miss
  - Cache key generation from input args

### Task 3.3: Run and commit

Run: `cd /workspaces/CodeForge/workers && poetry run pytest tests/test_consumer_dispatch.py tests/test_memory_system.py -v`
Expected: All PASS.

Commit both files together.

---

## Track 4: B3 Go Adapter + Domain Model Tests

> Fully independent -- no dependencies on other tracks.
> Each adapter/domain package gets its own test file.

### Task 4.1: Test Go Adapters (6 packages)

For each adapter, read the source file, then write a `*_test.go` in the same package:

**`internal/adapter/natskv/natskv_test.go`:**
- Get/Put/Delete operations against a mock KV interface
- Key not found -> error
- TTL behavior (if applicable)

**`internal/adapter/ristretto/ristretto_test.go`:**
- Cache get (hit/miss), set, delete
- Eviction behavior
- TTL expiration

**`internal/adapter/otel/otel_test.go`:**
- Provider creation (enabled mode)
- No-op provider (disabled mode)
- Shutdown behavior

**`internal/adapter/email/email_test.go`:**
- Send email with mock SMTP
- Template rendering
- Connection error handling

**`internal/adapter/lsp/lsp_test.go`:**
- Server lifecycle (start, capabilities, shutdown)
- Language detection
- Timeout on unresponsive server

**`internal/adapter/a2a/a2a_test.go`:**
- AgentCard endpoint (GET /.well-known/agent.json)
- Task creation (POST)
- Task status (GET)
- Tenant isolation on all endpoints

### Task 4.2: Test Go Domain Models (5 packages)

For each domain, read the source file, then write a `*_test.go`:

**`internal/domain/conversation/conversation_test.go`:**
- Entity creation with valid fields
- Validation: empty project_id -> error
- Status transitions

**`internal/domain/orchestration/orchestration_test.go`:**
- HandoffMessage validation
- Agent routing logic

**`internal/domain/microagent/microagent_test.go`:**
- Trigger keyword matching (case-insensitive)
- Priority ordering
- No match -> empty result

**`internal/domain/memory/memory_test.go`:**
- Memory entity creation
- Vector dimension validation

**`internal/domain/skill/skill_test.go`:**
- Skill entity creation
- Content parsing

### Task 4.3: Run and commit

Run: `go test ./internal/adapter/natskv/ ./internal/adapter/ristretto/ ./internal/adapter/otel/ ./internal/adapter/email/ ./internal/adapter/lsp/ ./internal/adapter/a2a/ ./internal/domain/conversation/ ./internal/domain/orchestration/ ./internal/domain/microagent/ ./internal/domain/memory/ ./internal/domain/skill/ -v -count=1`
Expected: All PASS.

Commit all test files together.

---

## Track 5: B2 Python Contract Validator

> **Depends on Track 1** (needs JSON fixtures generated by Task 1.2).

### Task 5.1: Create Python Contract Validator

**Files:**
- Read: `workers/codeforge/models.py` (all Pydantic models)
- Read: `internal/port/messagequeue/testdata/contracts/*.json` (fixtures from Track 1)
- Create: `workers/tests/test_nats_contracts.py`

**Step 1: Write the contract validator**

```python
"""Validate Go-generated JSON fixtures against Python Pydantic models.

Each test loads a JSON fixture from internal/port/messagequeue/testdata/contracts/
and validates it against the corresponding Pydantic model. This catches field name
mismatches, type incompatibilities, and missing required fields.
"""
import json
from pathlib import Path
import pytest
from codeforge.models import (
    ConversationRunStartMessage,
    ConversationRunCompleteMessage,
    BenchmarkRunRequestMessage,
    BenchmarkRunResultMessage,
    # ... all other models
)

FIXTURES_DIR = Path(__file__).parent.parent.parent / "internal" / "port" / "messagequeue" / "testdata" / "contracts"

# Map NATS subject -> Python Pydantic model class
SUBJECT_MODEL_MAP = {
    "conversation.run.start": ConversationRunStartMessage,
    "conversation.run.complete": ConversationRunCompleteMessage,
    "benchmark.run.request": BenchmarkRunRequestMessage,
    "benchmark.run.result": BenchmarkRunResultMessage,
    # ... all 20 subjects
}

@pytest.mark.parametrize("subject,model_cls", SUBJECT_MODEL_MAP.items())
def test_go_fixture_validates_against_pydantic(subject: str, model_cls: type) -> None:
    fixture_path = FIXTURES_DIR / f"{subject}.json"
    if not fixture_path.exists():
        pytest.skip(f"Fixture not yet generated: {fixture_path}")

    raw = json.loads(fixture_path.read_text())
    instance = model_cls.model_validate(raw)

    # Re-serialize and compare key fields
    roundtrip = json.loads(instance.model_dump_json())

    # Verify no fields were silently dropped
    for key in raw:
        assert key in roundtrip or key in getattr(model_cls, '__optional_fields__', set()), \
            f"Field '{key}' from Go fixture not present in Python model {model_cls.__name__}"
```

The subagent implementing this should read `workers/codeforge/models.py` to find the exact Pydantic model class names and build the complete `SUBJECT_MODEL_MAP`.

**Step 2: Run contract tests**

Run: `cd /workspaces/CodeForge/workers && poetry run pytest tests/test_nats_contracts.py -v`
Expected: All 20 contract tests PASS (or SKIP if fixture not yet generated).

**Step 3: Commit**

```bash
git add workers/tests/test_nats_contracts.py
git commit -m "test(contract): add Python NATS payload contract validator

Validates Go-generated JSON fixtures against Pydantic models.
Catches field mismatches, type errors, and missing required fields."
```

---

## Track 6: A1+A2 Smoke Tests (after Tracks 1-5)

> **Depends on Track 1** (migration fix required for tests to run against real DB).
> Requires running Docker Compose stack.

### Task 6.1: Stack Health Smoke Tests

**Files:**
- Create: `tests/integration/smoke_test.go`

```go
//go:build smoke

package integration_test

// Test that all infrastructure services are healthy and connected.
// Run: go test -tags=smoke -timeout=300s ./tests/integration/... -run TestSmoke
```

Write tests for:
- `TestSmoke_HealthEndpoint` -- GET /health returns 200
- `TestSmoke_PostgresConnection` -- SELECT 1 succeeds
- `TestSmoke_NATSJetStream` -- CODEFORGE stream exists with correct subjects
- `TestSmoke_LiteLLMHealth` -- GET litellm:4000/health returns 200 (skip if not available)
- `TestSmoke_WebSocketUpgrade` -- WS upgrade on /ws with JWT succeeds
- `TestSmoke_ModelAvailability` -- GET /api/v1/llm/available returns at least 1 model (skip if no LLM configured)

### Task 6.2: Critical Flow Smoke Tests

**Files:**
- Create: `tests/integration/flows_test.go`

```go
//go:build smoke

package integration_test

// Test end-to-end flows through the real stack.
// Each test creates data, verifies, and cleans up.
// Run: go test -tags=smoke -timeout=300s ./tests/integration/... -run TestFlow
```

Write tests for:
- `TestFlow_ProjectLifecycle` -- Create -> Get -> List -> Delete -> 404
- `TestFlow_SimpleConversation` -- Create conv -> Send msg -> Poll response -> Verify NATS roundtrip (skip if no LLM)
- `TestFlow_AgenticConversation` -- Send agentic msg -> Verify tool calls + AG-UI events (skip if no LLM)
- `TestFlow_BenchmarkRun` -- Create run -> Poll status -> Verify results (skip if no LLM)
- `TestFlow_RetrievalPipeline` -- Index -> Search -> Verify results
- `TestFlow_MemoryRoundtrip` -- Store -> Recall -> Verify match

Each test should use `t.Skip()` for LLM-dependent tests when `SMOKE_LLM_MODE=skip`.

### Task 6.3: Commit

```bash
git add tests/integration/smoke_test.go tests/integration/flows_test.go
git commit -m "test(smoke): add stack health and critical flow smoke tests

8 health checks + 6 end-to-end flows.
LLM-dependent tests gracefully skip when SMOKE_LLM_MODE=skip.
Run: go test -tags=smoke -timeout=300s ./tests/integration/..."
```

---

## Track 7: C2 Reporter + A3 CI + C3 Gate (after Tracks 1-6)

### Task 7.1: Automated Verification Reporter

**Files:**
- Create: `scripts/verify-features.sh`

Write a bash script that:
1. Runs `go test -json ./internal/...` and parses pass/fail per package
2. Runs `cd workers && poetry run pytest --json-report --json-report-file=/tmp/pytest-report.json` and parses
3. Maps packages/modules to the 30-feature matrix
4. Generates markdown table to stdout (pipe to `docs/feature-verification-matrix.md`)
5. Generates JSON summary to `/tmp/verification-summary.json`
6. Exit code: 0 if all critical features pass, 1 otherwise

### Task 7.2: CI Integration

**Files:**
- Modify: `.github/workflows/ci.yml`

Add two new jobs:

```yaml
contract:
  name: Contract Tests
  needs: [go, python]
  runs-on: ubuntu-latest
  steps:
    - checkout + setup-go + setup-python
    - Generate Go fixtures: go test ./internal/port/messagequeue/ -run Contract -v
    - Validate in Python: cd workers && poetry run pytest tests/test_nats_contracts.py -v

smoke:
  name: Smoke Tests
  needs: [contract]
  runs-on: ubuntu-latest
  services:
    postgres: (same as go job)
    nats: (same as go job)
  env:
    SMOKE_LLM_MODE: skip
  steps:
    - checkout + build Go backend
    - Start backend in background
    - Start Python worker in background
    - Run: go test -tags=smoke -timeout=300s ./tests/integration/...
```

### Task 7.3: Update Feature Verification Matrix

**Files:**
- Modify: `docs/feature-verification-matrix.md`

Run verification reporter and update all statuses based on actual test results.

### Task 7.4: Commit everything

```bash
git add scripts/verify-features.sh .github/workflows/ci.yml docs/feature-verification-matrix.md
git commit -m "ci: add contract tests, smoke tests, and verification reporter

- Contract test CI job validates Go/Python NATS payload compatibility
- Smoke test CI job runs stack health + critical flows
- Verification reporter generates feature matrix from test results"
```

---

## Execution Summary

### Phase 1: Parallel (Tracks 1-4 simultaneously)

| Track | Subagent | Est. Tasks | Independent |
|-------|----------|-----------|-------------|
| Track 1 | Fix migration + Go contract fixtures | 2 tasks | Yes |
| Track 2 | Python tool tests (7 tools) | 7 tasks | Yes |
| Track 3 | Python consumer + memory tests | 3 tasks | Yes |
| Track 4 | Go adapter + domain tests (11 pkgs) | 3 tasks | Yes |

### Phase 2: Sequential (Track 5 after Track 1)

| Track | Subagent | Est. Tasks | Depends On |
|-------|----------|-----------|------------|
| Track 5 | Python contract validator | 1 task | Track 1 fixtures |

### Phase 3: Sequential (Track 6 after Phase 1)

| Track | Subagent | Est. Tasks | Depends On |
|-------|----------|-----------|------------|
| Track 6 | Smoke tests | 3 tasks | Track 1 migration fix |

### Phase 4: Final (Track 7 after all)

| Track | Subagent | Est. Tasks | Depends On |
|-------|----------|-----------|------------|
| Track 7 | Reporter + CI + matrix update | 4 tasks | All tracks |

### Success Criteria

- [ ] `go test ./internal/...` -- 0 FAIL (migration fix)
- [ ] 20 NATS contract fixtures generated and validated bidirectionally
- [ ] 7 Python tool test files with >80% coverage on `workers/codeforge/tools/`
- [ ] Consumer dispatch tests covering all 9 handler mixins
- [ ] Memory scorer + experience pool tests
- [ ] 11 Go adapter/domain test files
- [ ] 8 smoke health checks + 6 flow tests
- [ ] CI runs contract + smoke tests on every push
- [ ] Feature verification matrix updated with real results
