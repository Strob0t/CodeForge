# Autonomous Goal-to-Program Test Report

**Date:** 2026-03-19
**Scenario:** S1 (Easy - CSV-to-JSON Converter)
**Runs:** 4 (Run 1+2 blocked by infra bugs, Run 3+4 executed successfully)
**Model:** openai/container (LM Studio qwen3-30b-a3b local)

---

## Phase Results

| Phase | Result | Notes |
|-------|--------|-------|
| 0 - Environment | PASS | All 6 services started, NATS purged, container IPs resolved |
| 1 - Project Setup | PASS | TestRepo cloned, branch test/s1-run3-172236 |
| 2 - Goal Discovery | PASS | 5 goals (4 requirements + 1 vision from README) |
| 3 - Goal Validation | PASS | 5/6 keyword coverage |
| 4 - Roadmap | PASS | 2 milestones, 8 features |
| 5 - Execution | PASS | Dispatched with openai/container, agentic mode active |
| 6 - Monitoring | PASS | 23 steps, 46 tool calls, completed (stall exit) |
| 7 - Validation | PARTIAL | Program works, test file has syntax error |
| 7b - Code Quality | PARTIAL | csv2json.py clean, test file syntax error + lint |
| 8 - Report | THIS FILE |

## Tool Call Breakdown

| Tool | Count | Notes |
|------|-------|-------|
| LLM | 23 | Reasoning iterations |
| edit_file | 15 | Code modifications (iterative fixes!) |
| bash | 5 | pytest, py_compile runs |
| write_file | 2 | Initial file creation |
| read_file | 1 | Reading existing files |
| **TOTAL** | **46** | |

## Program Validation (Phase 7)

| Check | Result |
|-------|--------|
| csv2json.py exists | PASS |
| test_csv2json.py exists | PASS |
| `--help` exits 0 | PASS |
| Converts valid CSV to JSON | PASS (correct output) |
| Missing file returns error + exit 1 | PASS |
| pytest passes | FAIL (syntax error in test file line 47: `}}`) |
| Git commit | FAIL (agent stalled before committing) |

## Code Quality (Phase 7b)

| Check | csv2json.py | test_csv2json.py |
|-------|-------------|------------------|
| Syntax (py_compile) | PASS | FAIL (`}}` on line 47) |
| Lint (ruff) | 3 findings (cosmetic) | 16 findings (syntax + whitespace) |
| Import sort | 1 finding (auto-fixable) | N/A (syntax error) |

### Lint Details (csv2json.py)
- I001: Import block unsorted (auto-fixable)
- UP015: Unnecessary `'r'` mode argument (auto-fixable)
- C416: Unnecessary list comprehension (auto-fixable)
- W292: No newline at end of file (auto-fixable)

### Test File Syntax Error
```python
# Line 47 — agent wrote:
f.write('name,age\nAlice,30\nBob,"25'}}
# Should be:
f.write('name,age\nAlice,30\nBob,"25')
```
The agent tried to create a CSV with unclosed quotes to trigger csv.Error but introduced a `}}` typo.

## Metrics

| Metric | Value |
|--------|-------|
| Agent steps | 23 |
| Tool calls | 46 |
| LLM cost | $0.00 (local model) |
| Execution time | ~12 minutes |
| Stall detection | Yes (repeated None after 2 escape attempts) |
| Files created | 2 (csv2json.py, test_csv2json.py) |
| Git commits | 0 (agent stalled before commit) |

## Bugs Found & Fixed During Testing

| Bug | Priority | Status |
|-----|----------|--------|
| WSL2 Docker port mapping | LOW | FIXED (container IPs) |
| Worker env var LITELLM_URL vs LITELLM_BASE_URL | LOW | FIXED |
| Model router ignores health status | HIGH | FIXED |
| NATS backlog from cancelled conversations | CRITICAL | FIXED |
| Auto-onboarding blocks NATS pipeline | HIGH | FIXED (disabled) |
| Goal-researcher autonomy too low | MEDIUM | FIXED (2->4) |
| NATS sequential processing blocks pipeline | CRITICAL | FIXED (goroutine dispatch) |
| NATS consumers not recreated after purge | HIGH | DOCUMENTED (restart backend after purge) |

## Key Learnings

1. **NATS stream MUST be purged before each test run** — old messages accumulate and block consumers
2. **Go backend MUST be restarted AFTER NATS purge** — consumers hold stale references
3. **Routing MUST be disabled** (`CODEFORGE_ROUTING_ENABLED=false`) — auto-router picks unhealthy models
4. **Explicit model in NATS payload** overrides router (Bug 1 fix)
5. **Local models (LM Studio) are slow** — 12min for S1 vs expected 10-15min
6. **Agent self-correction works** — 15 edit_file calls show iterative fixing
7. **Stall detection works** — agent correctly aborted after repeated failures
8. **Quality instructions in prompt help** — agent ran bash (5x) for testing

## Run 4: Clean Slate (post all fixes, clean project)

| Metric | Value |
|--------|-------|
| Agent steps | ~5 |
| Tool calls | 10 (6 LLM, 2 write_file, 1 edit_file, 1 bash) |
| Files created | 1 (csv2json.py only — test file missing) |
| Git commits | 0 |
| Duration | ~10 min |
| Exit reason | Run completed (no stall) |

**Validation:**
- csv2json.py: SYNTAX OK, --help PASS, conversion PASS, error handling PASS (exit 1)
- test_csv2json.py: NOT CREATED
- Git commit: NOT DONE
- Tool calls: 10 (below S1 minimum of 15)

**Assessment:** The goroutine NATS fix works (no more policy timeouts). The local model
produces a correct csv2json.py but stops before creating tests or committing.
Tool call count is low — model doesn't follow the full pipeline.

## Overall Result

**PARTIAL** — The autonomous pipeline works end-to-end. The main program (csv2json.py) is
functionally correct across all runs. Test file quality varies by run (syntax errors from
local model). A stronger model (Claude, GPT-4) would likely produce clean code on first attempt.

### Comparison Across Runs

| Run | Model | Steps | Tool Calls | Program | Tests | Commit | Notes |
|-----|-------|-------|------------|---------|-------|--------|-------|
| 1 | N/A | 0 | 0 | N/A | N/A | N/A | Blocked by infra (NATS timeout) |
| 2 | openai/container | 4 | 4 | PASS | FAIL | PASS | First successful end-to-end |
| 3 | openai/container | 23 | 46 | PASS | FAIL | FAIL | Most thorough (quality prompt) |
| 4 | openai/container | ~5 | 10 | PASS | SKIP | FAIL | Clean slate, test file not created |

### Infrastructure Fix Impact

| Fix | Run 1-2 | Run 3 | Run 4 |
|-----|---------|-------|-------|
| NATS goroutine dispatch | No | No | **Yes** |
| NATS purge before run | No | Yes | Yes |
| Auto-onboarding disabled | No | Yes | Yes |
| Policy timeout | **30s blocked** | **30s blocked** | **No timeout** |
| Tool calls processed | 4 (then blocked) | 46 (with delays) | 10 (instant) |

Run 4 proves the goroutine fix works: zero NATS policy timeouts, instant tool call processing.
The lower tool call count is a model quality issue, not infrastructure.
