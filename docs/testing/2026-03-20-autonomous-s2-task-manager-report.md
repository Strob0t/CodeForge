# Autonomous Goal-to-Program Test Report — S2 CLI Task Manager

**Date:** 2026-03-20
**Scenario:** S2 (Medium) — Python CLI Task Manager
**Model:** `lm_studio/qwen/qwen3-30b-a3b` (local, Alibaba Qwen 30B MoE)
**Testplan:** `docs/testing/2026-03-19-autonomous-goal-to-program-testplan.md`
**Tester:** Claude Code (Opus 4.6) via Playwright-MCP + API monitoring

---

## Executive Summary

CodeForge **successfully built a functional Python CLI task manager** autonomously using a local 30B model. The generated program supports all CRUD operations (add, list, complete, delete) with argparse CLI, dataclass models, and JSON persistence. However, the generated test suite has bugs (argparse argument passing), and `__main__.py` was not created (preventing `python -m task_manager`).

**Overall Grade: C** (PARTIAL — core functionality works, tests fail, minor structural issues)

---

## Environment

| Component | Version/Config |
|-----------|---------------|
| Backend | Go (dev mode, port 8080) |
| Frontend | Vite dev server (port 3000) |
| Worker | Python 3.12, NATS JetStream |
| LLM | LM Studio `qwen/qwen3-30b-a3b` at `192.168.88.21:1234` |
| LiteLLM | Proxy at container IP `172.18.0.6:4000` |
| NATS | Container IP `172.18.0.3:4222` |
| PostgreSQL | Container IP `172.18.0.5:5432` |
| Autonomy | Level 4 (full-auto), policy: `trusted-mount-autonomous` |
| HITL | Bypassed via `POST /conversations/{id}/bypass-approvals` |

---

## Phase Results

| Phase | Status | Duration | Notes |
|-------|--------|----------|-------|
| **0: Service Startup** | PASS | ~2 min | Docker healthy, NATS purged, backend + worker started |
| **1: Project Setup** | PASS | ~30s | Project created, workspace initialized, conversation created |
| **2: Goal Discovery** | SKIP | — | Direct execution prompt used (no goal-discovery phase) |
| **3: Goal Validation** | SKIP | — | No goals created (agent went straight to implementation) |
| **4: Roadmap Creation** | SKIP | — | No roadmap created (direct execution) |
| **5: Execution** | PASS | ~17 min | 26 messages, 24 LLM calls, agent completed autonomously |
| **6: Monitoring** | PASS | — | Monitored via API polling (30s intervals) |
| **7: Validation** | PARTIAL | ~1 min | CRUD works, tests fail, `__main__.py` missing |
| **8: Report** | PASS | — | This document |

---

## Execution Timeline

```
11:33:40  Run dispatched (conversation b66d53e6)
11:33:40  Worker received conversation.run.start
11:33:58  Skills + tool guide injected (capability: pure_completion)
11:34:00  First LLM call to LM Studio (toolcall.request -> policy -> allow)
11:34-41  LLM generating response (15 calls visible via streaming)
11:41:29  py_files changed from 1 to 2 (first file written)
11:50:37  26 messages in conversation (run complete)
          Agent self-correction: 2 edit cycles on cli.py
          Git commit: "Initial implementation of task manager CLI" (13 files, 228 insertions)
```

**Total execution time: ~17 minutes** (typical for local 30B model with complex prompt)

---

## Agent Tool Call Sequence

| Step | Tool | Target | Result |
|------|------|--------|--------|
| 1 | write_file | `task_manager/__init__.py` | 0 bytes (empty init) |
| 2 | write_file | `task_manager/models.py` | 282 bytes (Task dataclass, Priority enum) |
| 3 | write_file | `task_manager/cli.py` | 2563 bytes (argparse + CRUD commands) |
| 4 | write_file | `README.md` | 568 bytes (usage docs) |
| 5 | write_file | `task_manager/tests/test_cli.py` | 1704 bytes (4 test functions) |
| 6 | write_file | `setup.py` | 348 bytes (package setup with entry_points) |
| 7 | bash | (unknown) | Error: exit code 2 |
| 8 | edit_file | `cli.py` | 1 line replaced (fix) |
| 9 | edit_file | `cli.py` | 3 lines replaced with 9 (expanded fix) |
| 10 | bash | (unknown) | Error: exit code 2 |
| 11 | bash | `pip install -e .` | Package installed |
| 12 | bash | `git add -A && git commit` | 13 files committed |

**Total tool calls: ~12** (below S2 minimum of 33, but functional)
**Unique tools: 4** (write_file, edit_file, bash, git — below target of 6-7)
**Missing tools: list_directory, glob_files, search_files** (no exploration phase)

---

## Generated Project Structure

```
task_manager/
  __init__.py          (0 bytes, empty)
  models.py            (282 bytes, Task dataclass + Priority enum)
  cli.py               (2563 bytes, argparse CLI with CRUD)
  tests/
    test_cli.py        (1704 bytes, 4 test functions)
setup.py               (348 bytes, package config)
README.md              (568 bytes, usage documentation)
hello.py               (22 bytes, leftover from test message)
```

**Missing:** `__main__.py`, `task_manager/storage.py` (storage inline in cli.py), `tests/conftest.py`

---

## Validation Results (Phase 7)

### S2 Validation Matrix

| Check | Expected | Result | Status |
|-------|----------|--------|--------|
| Package structure | `task_manager/` with `__init__.py` | Present | PASS |
| `--help` exits 0 | Shows usage + commands | Shows 4 subcommands | PASS |
| Add command | Creates task | "Added task 1: Buy groceries" | PASS |
| List command | Shows tasks | Shows task with priority | PASS |
| Complete command | Marks done | "Marked task 1 as done" | PASS |
| Delete command | Removes task | Works via `cli.py` | PASS |
| `python -m task_manager` | Exits 0 | `__main__.py` missing | FAIL |
| Tests pass | pytest green | 4/4 FAILED (argparse bug) | FAIL |
| JSON valid | `json.tool` parses | Empty/malformed | FAIL |
| README exists | File present | Present with usage examples | PASS |
| Git commit | At least 1 | 1 commit, 13 files | PASS |

**Score: 8/11 checks passed**

### Test Failure Analysis

All 4 tests fail with `SystemExit: 2` — the test functions call the argparse parser without providing required arguments. The CLI works correctly when invoked directly via command line, but the test harness doesn't properly mock `sys.argv`.

**Root cause:** Tests call `main()` or parser functions without setting `sys.argv` to include the subcommand. This is a common mistake with argparse-based CLIs in test code.

---

## Infrastructure Issues Encountered

| Issue | Impact | Resolution |
|-------|--------|-----------|
| **Stale VSCode debug binary** (PID 12110) | Go backend had stale NATS consumers, toolcall requests timed out | Killed debug binary, started fresh `go run` |
| **NATS toolcall timeout** (30s) | Python worker got "NATS response timeout waiting for policy decision" | Caused by Go backend not subscribing after NATS purge |
| **LM Studio unreachable** at `host.docker.internal:1234` | Worker fell back to `ollama/llama2` (also offline) | User provided correct IP `192.168.88.21:1234` |
| **`SendMessageRequest` has no `model` field** | API `model` parameter silently ignored | Used config-level `default_model` in `codeforge.yaml` |
| **Multiple startup failures** | `pkill` sending signal 144 to bash subprocess | Separated kill/start into distinct commands |

### Critical Finding: NATS Consumer Startup Order

The correct startup sequence is **strictly ordered**:
1. Kill all processes
2. Purge NATS JetStream (with no active consumers)
3. Start Go backend (creates fresh JetStream consumers)
4. Start Python worker (creates fresh JetStream consumers)

If the Go backend has stale consumers from before a NATS purge, toolcall requests are silently dropped (no subscriber). This caused 3 failed run attempts before the root cause was identified.

---

## Bugfix Verification

This run validates the 5 bugfixes implemented earlier today:

| Fix | Verified | Evidence |
|-----|----------|---------|
| 1. Goal auto-persist | Not tested | Agent didn't use `propose_goal` (direct execution prompt) |
| 2. Config policy_preset fallback | PASS | `config.policy_preset: trusted-mount-autonomous` was read correctly |
| 3. HITL bypass for full-auto | PASS | No HITL timeouts, all tool calls approved instantly |
| 4. Stack detection persistence | Not tested | Setup returned `stack_detected: true` but no languages (empty workspace) |
| 5. FilePanel text fix | PASS | Browser showed "Select a file from the tree to view or edit." |

---

## Comparison with Previous Runs

| Metric | Run 6 (2026-03-19) | This Run (2026-03-20) |
|--------|--------------------|-----------------------|
| Model | `lm_studio/qwen/qwen3-30b-a3b` | `lm_studio/qwen/qwen3-30b-a3b` |
| Steps | 26 | ~12 tool calls |
| Files created | 2 (csv2json.py + test) | 8 (full package) |
| Git commits | 0 | 1 (13 files, 228 insertions) |
| NATS timeouts | 0 | 0 (after fixing startup order) |
| HITL blocks | Yes (Permission Denied) | No (bypass + full-auto policy) |
| Functional | Partial (argparse bug) | CRUD works, tests fail |
| Duration | ~30 min (S1 scenario) | ~17 min (S2 scenario) |

**Improvement:** Full CRUD cycle works, git commit made, proper package structure, README with docs.

---

## Recommendations

1. **Fix `SendMessageRequest.Model` field** — Add `Model string` to the request struct so API callers can specify the model. Currently the field is silently ignored, forcing config-level changes.

2. **Add `__main__.py` to agent prompt** — The agent didn't create `__main__.py`, preventing `python -m task_manager`. The system prompt or tool guide should mention this common Python pattern.

3. **Test harness guidance** — The agent wrote tests that don't properly mock `sys.argv` for argparse. The tool guide for `pure_completion` models could include a hint about testing argparse CLIs.

4. **Startup order documentation** — The NATS consumer startup order issue is critical and underdocumented. Add a warning to `docs/dev-setup.md`.

5. **Goal discovery vs direct execution** — When the prompt says "build X", the agent skips goal discovery entirely. Consider splitting the flow: always run goal discovery first, then execution.

---

## Raw Data

- **Project ID:** `3cab72a9-2919-43a1-9d52-8a93b8611b20`
- **Conversation ID:** `b66d53e6-5136-4994-916b-4d5c7d8bdc92`
- **Workspace:** `/workspaces/CodeForge/data/workspaces/00000000-0000-0000-0000-000000000000/3cab72a9-2919-43a1-9d52-8a93b8611b20`
- **Git commit:** `0324780 Initial implementation of task manager CLI`
- **Worker log:** `/tmp/codeforge-worker.log`
- **Backend log:** (lost due to process restart — create `/tmp/codeforge-backend-{timestamp}.log` in future runs)
