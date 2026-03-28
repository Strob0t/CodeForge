# Autonomous Goal-to-Program Test Report — 2026-03-28

**Date:** 2026-03-28
**Testplan:** `docs/testing/autonomous-goal-to-program-testplan.md`
**Model:** `lm_studio/qwen/qwen3-30b-a3b` (local, ~30B params)
**Branch:** `staging` (post audit remediation v2, 20 worktrees merged)

---

## S1: Easy — Build Your Own `wc` Tool

**Scenario:** Greenfield single-file Python script
**Workspace:** `/workspaces/CodeForge/data/workspaces/s1-wc-tool`
**Duration:** ~15 minutes

### Agent Metrics

| Metric | Value |
|--------|-------|
| Total messages | 26 |
| Tool calls | 12 |
| Tools used | `write_file`, `bash`, `edit_file`, `propose_roadmap` |
| Unique tools | 4 |
| LLM iterations | 13 |
| Git commits | 1 |

### Validation Results

| Check | Expected | Actual | Result |
|-------|----------|--------|--------|
| `ccwc.py` exists | file present | present | **PASS** |
| `test_ccwc.py` exists | file present | present | **PASS** |
| Syntax valid (py_compile) | no errors | no errors | **PASS** |
| `-c` byte count | 35100 | 35100 | **PASS** |
| `-l` line count | 100 | 100 | **PASS** |
| `-w` word count | 7011 | 7011 | **PASS** |
| Default output (lines words bytes) | 3 numbers | 3 numbers | **PASS** |
| Stdin mode (`cat \| ccwc -l`) | 100 | 100 | **PASS** |
| Error handling (missing file) | non-zero exit | exit 0 | **FAIL** |
| Pytest tests | >= 3/5 pass | 7/8 pass | **PASS** |
| Git commit | at least 1 | 1 commit | **PASS** |

### Result: **PASS** (9/11 checks, meets all success criteria)

### Notes
- Error handling: program prints error message but exits 0 instead of non-zero
- Pytest: 1 test failure — off-by-one in stdin byte count test (`11` vs `12`, likely `\n` counting)
- Agent used `propose_roadmap` tool (unexpected but not harmful)
- Agent did NOT use `read_file`, `search_files`, `glob_files`, `list_directory` — went straight to writing
- Total 12 tool calls is below the expected 15-30 range, but all outputs are correct

---

## S2: Medium — Build Your Own `cut` Tool

**Scenario:** Greenfield multi-module Python package
**Workspace:** `/workspaces/CodeForge/data/workspaces/s2-cut-tool`
**Duration:** ~30 minutes

### Agent Metrics

| Metric | Value |
|--------|-------|
| Total messages | 53 |
| Tool calls | 25 |
| Tools used | `write_file`, `bash`, `edit_file`, `propose_goal`, `propose_roadmap` |
| Unique tools | 5 |
| LLM iterations | ~29 |
| Git commits | 0 (agent did not commit) |

### Validation Results

| Check | Expected | Actual | Result |
|-------|----------|--------|--------|
| Package structure (cccut/) | 4 files | 4 files | **PASS** |
| Modules (parser.py, cutter.py) | present | present | **PASS** |
| Tests directory | present | present | **PASS** |
| pyproject.toml | present | present | **PASS** |
| Syntax valid (py_compile) | no errors | no errors | **PASS** |
| `-f2` tab extraction | matches `cut` | mismatch | **FAIL** |
| `-d, -f1,3` CSV | matches `cut` | mismatch | **FAIL** |
| Stdin mode | matches `cut` | matches | **PASS** |
| Missing file error | non-zero exit | non-zero | **PASS** |
| No -f error | non-zero exit | non-zero | **PASS** |
| Pytest tests | >= 3 pass | 0/8 pass | **FAIL** |
| Git commit | at least 1 | 0 commits | **FAIL** |

### Result: **PARTIAL** (7/12 checks, does NOT meet success criteria)

### Notes
- Package structure and error handling are solid
- Core field extraction logic has bugs (field indexing or delimiter handling)
- All 8 tests fail: parser tests get `TypeError` (argument parsing interface mismatch), cutter tests get `SystemExit` (calling `sys.exit` instead of raising)
- Agent did not commit to git
- Agent spent many iterations on test fixing but could not resolve the parser interface issue
- This is typical for qwen3-30b at medium complexity — structure correct, details buggy

---

## Summary

| Scenario | Status | Score | Duration |
|----------|--------|-------|----------|
| S1 Easy | **PASS** | 9/11 | ~15 min |
| S2 Medium | **PARTIAL** | 7/12 | ~30 min |
| S3 Hard | NOT RUN | — | — |
| S4 Expert | NOT RUN | — | — |

### Key Observations

1. **S1 demonstrates the pipeline works end-to-end**: project creation, goal setting, agentic dispatch, tool execution, file creation, testing, and git commit all function correctly.
2. **S2 shows model capability limits**: the 30B local model handles structure/scaffolding well but struggles with multi-module integration and test correctness.
3. **Agent tool usage is conservative**: both runs used fewer tools than expected (12 and 25 vs 20-30 and 33-60). The agent writes more code per tool call rather than exploring first.
4. **Infra is stable**: zero NATS timeouts, zero crashes, all services remained healthy throughout both runs.
5. **Recommendation**: Run S1 and S2 with a stronger model (Claude/GPT-4o via API) to validate the pipeline produces correct programs when model capability is not the bottleneck.
