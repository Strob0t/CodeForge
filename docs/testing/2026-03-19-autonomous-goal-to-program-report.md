# Autonomous Goal-to-Program Test Report

**Date:** 2026-03-19
**Scenario:** S1 (Easy - CSV-to-JSON Converter)
**Runs:** 3 (Run 1+2 blocked by infra bugs, Run 3 successful)
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

## Overall Result

**PARTIAL** — The autonomous pipeline works end-to-end. The program is functionally correct.
Test file has a minor syntax error from the local model. A stronger model (Claude, GPT-4) would
likely produce clean code on the first attempt.

### Comparison Across Runs

| Run | Model | Steps | Tool Calls | Program | Tests | Commit |
|-----|-------|-------|------------|---------|-------|--------|
| 1 | N/A | 0 | 0 | N/A | N/A | N/A | (blocked by infra) |
| 2 | openai/container | 4 | 4 | PASS | FAIL | PASS | (previous session) |
| 3 | openai/container | 23 | 46 | PASS | FAIL | FAIL | (stall, no commit) |

Run 3 was much more thorough (46 vs 4 tool calls, 15 edit iterations) due to the quality
instructions in the prompt. The agent actively tried to fix issues but the local model
introduced a syntax error it couldn't self-correct.
