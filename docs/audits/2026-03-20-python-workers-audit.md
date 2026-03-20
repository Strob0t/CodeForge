# Python Workers Audit Report

**Date:** 2026-03-20
**Scope:** Architecture + Code Review
**Files Reviewed:** 170 files (26,135 lines)
**Score: 72/100 -- Grade: C**

> Warning: Score below 75 indicates significant issues requiring attention before production deployment.

---

## Executive Summary

| Severity | Count | Category Breakdown |
|----------|------:|---------------------|
| CRITICAL | 1     | Bash tool command sanitization gap |
| HIGH     | 5     | Missing tenant isolation, glob path traversal, quality gate command validation, object type annotations, unbounded memory growth |
| MEDIUM   | 4     | Rollout scoring bug, idempotency set ordering, synchronous HTTP in async context, duplicate embedding code |
| LOW      | 4     | Any type usage, plan_act iteration side effect, signal handler closure, inconsistent logging |
| **Total**| **14** |                     |

### Positive Findings

- **Zero bare except clauses.** Every exception handler uses `except Exception as exc:` with proper logging -- the codebase rigorously follows the coding principles.
- **Consistent path traversal protection.** The `resolve_safe_path()` function in `tools/_base.py` is used by read_file, write_file, edit_file, list_directory, and search_files, providing a single point of defense against directory traversal.
- **Well-structured consumer mixin pattern.** The 17 handler mixins compose cleanly via multiple inheritance with a shared `_handle_request()` template method that enforces dedup, error handling, and ack/nak consistently.
- **Comprehensive stall detection.** `StallDetector` uses sliding window with hash-based dedup, escape prompts, and abort after 2 failed escapes -- matches the documented "Stall Detection" safety layer.
- **Model fallback with rate-limit awareness.** The fallback chain skips providers whose rate-limit tracker shows exhaustion, avoiding wasted retries.
- **Tool output truncation.** Both bash tool (`MAX_OUTPUT_CHARS=50K`) and history manager (`DEFAULT_TOOL_OUTPUT_MAX_CHARS=10K`) enforce output size limits via head-and-tail truncation.
- **Trust annotations on outgoing NATS messages.** All consumer mixins consistently call `_stamp_trust()` before publishing results.
- **Retry with exponential backoff.** `LiteLLMClient._with_retry()` uses configurable backoff with Retry-After header parsing, including Gemini-style duration parsing.
- **Dead letter queue support.** `_move_to_dlq()` in `ConsumerBaseMixin` properly moves failed messages after max retries.
- **Stream accumulator correctly handles cross-chunk think blocks.** The `_strip_think_tokens()` state machine tracks open/close tags across SSE chunks.
- **Clean separation of concerns.** The `RuntimeClient` owns NATS protocol details; `AgentLoopExecutor` owns LLM interaction; `ToolRegistry` owns tool dispatch -- no circular dependencies between core modules.

---

## Architecture Review

### Module Structure and Dependency Direction

The Python worker codebase follows a clean layered architecture:

```
consumer/ (NATS message handlers)
  -> agent_loop.py (core agentic loop)
    -> tools/ (tool execution)
    -> llm.py (LLM client)
    -> runtime.py (NATS protocol)
  -> evaluation/ (benchmark pipeline)
  -> memory/ (memory store + experience pool)
  -> routing/ (hybrid model routing)
  -> trust/ (trust annotations + risk scoring)
```

**Strengths:**
- No circular imports detected. Tools depend on nothing except `_base.py`. Consumer mixins import from models and services but not from each other.
- `TYPE_CHECKING` guards are used throughout for heavy imports, keeping startup fast.
- The mixin composition pattern in `TaskConsumer` provides clean separation -- each handler group is self-contained.

**Concerns:**
- The `_conversation.py` mixin is 933 lines -- the largest single handler. It combines routing, skill injection, MCP setup, Claude Code dispatch, and the LiteLLM loop. This file could benefit from decomposition.
- The `_benchmark.py` mixin is 1037 lines with significant inline logic for task execution, progress tracking, and multi-model evaluation.

### Consumer Message Flow

All handlers follow the `_handle_request()` template:
1. Deserialize with Pydantic `model_validate_json()`
2. Check dedup via `_is_duplicate()`
3. Execute handler
4. Publish result (optional)
5. `msg.ack()`

On error: `msg.nak()` (allows redelivery). After `MAX_RETRIES`: `_move_to_dlq()`.

This is sound and consistent across all 17 mixins.

### Agent Loop Design

The agentic loop in `agent_loop.py` (1395 lines) is well-structured:
- Clear separation between `run()` (orchestration), `_do_llm_iteration()` (single LLM call), `_process_llm_response()` (response handling), and `_execute_tool_call()` (tool dispatch).
- Three termination conditions: max iterations, max cost, cancellation -- all checked correctly.
- Plan/Act phase gating correctly blocks non-read-only tools during plan phase.
- Quality tracking with mid-loop model switching provides adaptive behavior.

---

## Code Review Findings

### CRITICAL-001: Bash Tool Has No Command Sanitization

- **File:** `workers/codeforge/tools/bash.py:74-82`
- **Description:** The `BashTool.execute()` method passes the user-provided `command` string directly to `bash -c` without any sanitization, blocklist checking, or escaping. While the Go control plane's policy engine provides a first line of defense (the tool call goes through `runtime.request_tool_call()` which checks policy rules), the Python tool itself performs zero validation. If policy is misconfigured or set to `trusted-mount-autonomous` (autonomy level 4+), any command runs unrestricted -- including destructive filesystem operations, network exfiltration via curl/wget, or privilege escalation.
- **Impact:** Full system compromise if policy layer fails or is configured permissively. In mount execution mode, the bash tool runs directly on the host filesystem.
- **Recommendation:** Add a command blocklist at the Python tool level as defense-in-depth. The Go side has `CommandSafetyEvaluator` but the Python side should also reject obviously destructive patterns (e.g., `rm -rf /`, `mkfs`, `dd if=/dev/zero`, piped curl-to-bash). This matches the "8 Safety Layers" architecture where Command Safety should be enforced at multiple levels.

### HIGH-001: Memory Storage Missing Tenant ID Filter in Recall

- **File:** `workers/codeforge/memory/storage.py:86-99`
- **Description:** The `MemoryStore.recall()` method filters only by `project_id` but does NOT filter by `tenant_id`. The SQL query is `WHERE project_id = %s` without any `AND tenant_id = %s` clause. In a multi-tenant deployment, this allows one tenant's agent to recall memories from another tenant's project if they share the same project ID structure.
- **Impact:** Cross-tenant data leakage in memory recall. Violates the tenant isolation requirement stated in CLAUDE.md ("ALL tenant-scoped queries: AND tenant_id = $N").
- **Recommendation:** Add `AND tenant_id = %s` to the recall query. The `MemoryStore.__init__` already hardcodes tenant_id as `"00000000-0000-0000-0000-000000000000"` for storage -- this should be parameterized and used consistently for both store and recall.

### HIGH-002: Glob Tool Missing Path Traversal Protection

- **File:** `workers/codeforge/tools/glob_files.py:53-58`
- **Description:** The `GlobFilesTool` does NOT call `resolve_safe_path()` to validate the glob pattern. While `Path.glob()` operates relative to the workspace and `relative_to(workspace)` is used, a crafted pattern like `../../etc/passwd` could potentially match files outside the workspace before the `relative_to` call filters results. The `relative_to` call would raise `ValueError` for paths outside workspace, but this error is not caught and would surface as an unhandled exception.
- **Impact:** Potential path traversal via glob patterns. The `relative_to` would raise an exception rather than leak data, but it would crash the tool execution ungracefully.
- **Recommendation:** Add explicit path validation. Either call `resolve_safe_path()` on each matched path before including it, or wrap the `relative_to` call in a try/except to skip paths outside the workspace, or sanitize the glob pattern to reject `..` components.

### HIGH-003: Quality Gate Runs Arbitrary Shell Commands

- **File:** `workers/codeforge/qualitygate.py:60-94`
- **Description:** `QualityGateExecutor._run_command()` receives test and lint commands from the Go control plane and runs them via `shlex.split()` + `create_subprocess_exec()`. While `shlex.split()` provides basic parsing, the commands themselves come from project configuration and could contain injected payloads if project config is compromised.
- **Impact:** Arbitrary command execution if project configuration is manipulated. The timeout (120s default) limits duration but not damage.
- **Recommendation:** Validate commands against an allowlist of known test/lint tools (pytest, ruff, eslint, go test, etc.) or at minimum reject commands containing shell metacharacters beyond what `shlex` handles.

### HIGH-004: Use of `object` Type Annotation Instead of Proper Types

- **File:** `workers/codeforge/agent_loop.py:135,151,543,621`
- **Description:** The `LoopConfig.routing_metadata` is typed as `object | None`, `_LoopState.quality_tracker` is typed as `object | None`, and `plan_act` parameters are typed as `object | None`. This violates the project's strict type safety principle ("No `any`/`interface{}`/`Any`"). It prevents IDE autocompletion, type checking, and makes the code harder to maintain.
- **Impact:** Loss of type safety across the core agent loop. Bugs from incorrect attribute access would only be caught at runtime.
- **Recommendation:** Use proper type annotations: `RoutingMetadata | None`, `IterationQualityTracker | None`, `PlanActController | None`. Use `TYPE_CHECKING` guards if needed to avoid circular imports.

### HIGH-005: Unbounded Growth of `_processed_ids` Set

- **File:** `workers/codeforge/consumer/_base.py:32-44`
- **Description:** The `_processed_ids` dedup set uses `ClassVar[set[str]]` with a max of 10,000 entries. When the limit is exceeded, it evicts "the oldest half" -- but `set` is unordered in Python, so the eviction is random, not FIFO. Furthermore, the eviction is not thread-safe: multiple concurrent handlers could trigger eviction simultaneously, and since this is a class variable shared across all mixin instances and accessed from multiple asyncio tasks, there is a potential for race conditions under high load.
- **Impact:** Under sustained high throughput, random eviction could allow recent messages to be evicted while old ones remain, defeating the dedup purpose. The set will continue to grow between eviction events.
- **Recommendation:** Replace with `collections.OrderedDict` for FIFO eviction, or use a TTL-based cache like `cachetools.TTLCache`. Since all access is within a single asyncio event loop (not multi-threaded), the race condition risk is minimal in practice, but the eviction strategy should still be FIFO.

### MEDIUM-001: Rollout Scoring Always Returns 1.0 for Non-Error Results

- **File:** `workers/codeforge/agent_loop.py:1105`
- **Description:** The rollout scoring formula is `len(r.final_content) / max(len(r.final_content), 1)` which always evaluates to 1.0 when `final_content` is non-empty (and 0.0 when it is empty, but then `error` is also set). This means the "best rollout" selection degrades to picking the first non-errored result, making the multi-rollout feature ineffective.
- **Impact:** Multi-rollout (inference-time scaling) does not actually select the best result -- it just picks the first successful one. The feature incurs N times the cost for no quality improvement.
- **Recommendation:** Implement meaningful scoring -- e.g., content length, presence of code blocks, LLM-judge quality, or test pass rate. The comment on line 1104 acknowledges this: "simple: use content length as proxy" -- but the formula is mathematically wrong for that intent.

### MEDIUM-002: Idempotency Set Not Safe Under Concurrent Access Pattern

- **File:** `workers/codeforge/consumer/_base.py:36-44`
- **Description:** The `_is_duplicate()` method modifies `_processed_ids` (a mutable class-level set) without any locking. While asyncio is single-threaded, the `_handle_request()` pattern calls `await handler(...)` between checking `_is_duplicate()` and completing the handler. If two NATS messages with the same dedup key arrive on different subscription loops (which are concurrent coroutines via `asyncio.gather`), both could pass the dedup check before either marks completion.
- **Impact:** Low probability duplicate processing under burst conditions. The current `_is_duplicate()` marks-then-checks in one call which mitigates this somewhat, but the window exists between `_is_duplicate()` returning False and the message being fully processed.
- **Recommendation:** The current design is acceptable for asyncio since set operations are atomic within a single event loop tick, but document this assumption clearly.

### MEDIUM-003: Synchronous HTTP Calls in Async Context (Routing)

- **File:** `workers/codeforge/consumer/_conversation.py:688-729,741-769`
- **Description:** The `_load_stats()` and `_llm_call()` closures inside `_get_hybrid_router()` use synchronous `httpx.get()` and `httpx.post()` respectively. These are called from within the async consumer context. While they are wrapped in synchronous functions passed to the routing layer, they will block the asyncio event loop during execution.
- **Impact:** During model routing, the entire consumer event loop blocks for up to 5s (stats) or 30s (meta-router LLM call), preventing other message processing.
- **Recommendation:** Make these async and use `asyncio.to_thread()` if the routing layer requires synchronous callables, or refactor the routing layer to accept async callables.

### MEDIUM-004: Duplicate Embedding Computation Code

- **File:** `workers/codeforge/memory/storage.py:141-149` and `workers/codeforge/memory/experience.py:179-187`
- **Description:** Both `MemoryStore._compute_embedding()` and `ExperiencePool._compute_embedding()` have identical implementations: call `self._llm.embedding(text)`, convert to numpy array, catch exceptions. This violates the DRY principle.
- **Impact:** Maintenance burden -- any change to embedding computation must be replicated in two places.
- **Recommendation:** Extract into a shared utility function in `memory/__init__.py` or a dedicated `memory/embedding.py` module.

### LOW-001: Broad `Any` Type Usage in Tool Framework

- **File:** `workers/codeforge/tools/_error_handler.py:11-13`, `workers/codeforge/tools/_base.py:29`
- **Description:** The `catch_os_error` decorator uses `Any` for the function parameter and wrapper parameters. `ToolDefinition.parameters` uses `dict[str, Any]`. While `Any` is more acceptable for JSON schema dicts, it still violates the strict type safety principle.
- **Impact:** Minor -- reduced type checking coverage in the tool framework.
- **Recommendation:** For the decorator, use `TypeVar` with proper bounds. For parameters, consider a dedicated `JSONSchema` type alias.

### LOW-002: Plan/Act Transition Has Side Effect in Check Method

- **File:** `workers/codeforge/plan_act.py:60-69`
- **Description:** `should_auto_transition()` increments `self.plan_iterations` as a side effect of being called. This means calling the method multiple times per iteration would advance the counter incorrectly. It is currently only called once per iteration in `_check_plan_act_transition()`, but the side effect is surprising and fragile.
- **Impact:** If `should_auto_transition()` is called from a new location, the counter would advance incorrectly, causing premature phase transition.
- **Recommendation:** Separate the increment from the check: add an explicit `advance_plan_iteration()` method.

### LOW-003: Signal Handler Closure Captures Stale Reference

- **File:** `workers/codeforge/consumer/__init__.py:309`
- **Description:** `lambda: asyncio.create_task(consumer.stop())` captures `consumer` by closure. If `consumer` is reassigned before the signal fires (unlikely in the current code), the lambda would reference the wrong object. More importantly, `asyncio.create_task()` within a signal handler is correct for asyncio but relies on the event loop still running.
- **Impact:** Negligible in practice -- the code works correctly as written.
- **Recommendation:** No change needed, but consider adding a comment explaining the pattern.

### LOW-004: Inconsistent Logging Libraries

- **File:** Multiple files across the codebase
- **Description:** Some modules use `logging.getLogger(__name__)` (standard library) while others use `structlog.get_logger()`. For example, `agent_loop.py`, `llm.py`, `tools/*.py` use `logging`, while `consumer/*.py`, `memory/*.py`, `graphrag.py` use `structlog`. Both are configured via `setup_logging()` in `logger.py`, but the inconsistency means log output format differs between modules.
- **Impact:** Minor -- log correlation is harder when some messages use structured fields and others use printf-style formatting.
- **Recommendation:** Standardize on `structlog` throughout, or at minimum use structlog's `stdlib` integration to route standard logging through structlog's processors.

---

## File Inventory

### Consumer Mixins (18 files, ~3,500 lines)

| File | Lines | Purpose |
|------|------:|---------|
| `consumer/__init__.py` | 318 | TaskConsumer class, main(), mixin composition |
| `consumer/__main__.py` | 7 | Entry point |
| `consumer/_base.py` | 150 | Shared helpers (dedup, DLQ, trust, _handle_request) |
| `consumer/_conversation.py` | 933 | Agentic conversation loop dispatch |
| `consumer/_benchmark.py` | 1037 | Benchmark run orchestration |
| `consumer/_tasks.py` | 76 | Backend router task dispatch |
| `consumer/_runs.py` | 80 | Run protocol execution |
| `consumer/_compact.py` | 132 | Conversation summarization |
| `consumer/_memory.py` | 196 | Memory store + recall |
| `consumer/_retrieval.py` | 185 | Index build + search + sub-agent |
| `consumer/_context.py` | 94 | Context re-ranking |
| `consumer/_graph.py` | 102 | GraphRAG build + search |
| `consumer/_handoff.py` | 95 | Agent handoff dispatch |
| `consumer/_a2a.py` | 113 | A2A task execution |
| `consumer/_review.py` | 126 | Review trigger dispatch |
| `consumer/_prompt_evolution.py` | 140 | Prompt mutation + reflection |
| `consumer/_quality_gate.py` | 41 | Quality gate dispatch |
| `consumer/_repomap.py` | 49 | Repo map generation |
| `consumer/_backend_health.py` | 54 | Backend health checks |
| `consumer/_subjects.py` | 116 | NATS subject constants |

### Core Agent System (6 files, ~4,700 lines)

| File | Lines | Purpose |
|------|------:|---------|
| `agent_loop.py` | 1395 | Agentic tool-use loop + rollout executor |
| `plan_act.py` | 88 | Plan/Act phase controller |
| `llm.py` | 917 | LiteLLM proxy client |
| `runtime.py` | 352 | NATS run protocol client |
| `executor.py` | 267 | Fire-and-forget task executor |
| `history.py` | 372 | Conversation history manager |

### Tools (15 files, ~1,700 lines)

| File | Lines | Purpose |
|------|------:|---------|
| `tools/__init__.py` | 138 | ToolRegistry + build_default_registry |
| `tools/_base.py` | 80 | ToolDefinition, ToolResult, resolve_safe_path |
| `tools/_error_handler.py` | 23 | catch_os_error decorator |
| `tools/bash.py` | 101 | Shell command execution |
| `tools/read_file.py` | 79 | File reading with offset/limit |
| `tools/write_file.py` | 84 | File creation/overwrite |
| `tools/edit_file.py` | 101 | Search-and-replace editing |
| `tools/search_files.py` | 130 | Grep-based content search |
| `tools/glob_files.py` | 75 | Glob pattern file finding |
| `tools/list_directory.py` | 100 | Directory listing |
| `tools/handoff.py` | 107 | Agent handoff tool |
| `tools/search_conversations.py` | 91 | Conversation search tool |
| `tools/search_skills.py` | 81 | Skill search tool |
| `tools/create_skill.py` | 166 | Skill creation tool |
| `tools/propose_goal.py` | 106 | Goal proposal tool |
| `tools/capability.py` | 119 | Model capability classification |
| `tools/tool_guide.py` | 93 | Adaptive tool guide for weaker models |

### Memory & Experience (5 files, ~530 lines)

| File | Lines | Purpose |
|------|------:|---------|
| `memory/__init__.py` | 13 | Module init |
| `memory/models.py` | 77 | Memory data models |
| `memory/scorer.py` | 58 | Composite memory scoring |
| `memory/storage.py` | 149 | PostgreSQL memory store |
| `memory/experience.py` | 236 | Experience pool with @exp_cache |

### Routing (8 files, ~1,180 lines)

| File | Lines | Purpose |
|------|------:|---------|
| `routing/__init__.py` | 31 | Module exports |
| `routing/models.py` | 161 | Routing data models |
| `routing/complexity.py` | 326 | Rule-based complexity analyzer |
| `routing/mab.py` | 281 | UCB1 multi-armed bandit selector |
| `routing/meta_router.py` | 198 | LLM-based meta-router |
| `routing/router.py` | 316 | HybridRouter orchestrator |
| `routing/rate_tracker.py` | 136 | Provider rate-limit tracker |
| `routing/reward.py` | 67 | Reward computation |
| `routing/blocklist.py` | 97 | Model blocklist |
| `routing/key_filter.py` | 84 | Keyless model filtering |

### Evaluation (25+ files, ~4,500 lines)

| File | Lines | Purpose |
|------|------:|---------|
| `evaluation/runner.py` | 141 | Basic benchmark runner |
| `evaluation/evaluators/base.py` | 43 | Evaluator protocol |
| `evaluation/evaluators/llm_judge.py` | 133 | LLM-as-Judge evaluator |
| `evaluation/evaluators/functional_test.py` | 95 | Functional test evaluator |
| `evaluation/evaluators/sparc.py` | 186 | SPARC evaluator |
| `evaluation/evaluators/filesystem_state.py` | 119 | Filesystem state evaluator |
| `evaluation/evaluators/trajectory_verifier.py` | 213 | Trajectory verifier |
| `evaluation/evaluators/logprob_verifier.py` | 140 | Logprob verifier |
| `evaluation/runners/simple.py` | 74 | Simple runner |
| `evaluation/runners/agent.py` | 236 | Agent runner |
| `evaluation/runners/tool_use.py` | 105 | Tool-use runner |
| `evaluation/runners/multi_rollout.py` | 268 | Multi-rollout runner |
| `evaluation/providers/base.py` | 176 | Provider protocol |
| `evaluation/providers/humaneval.py` | 126 | HumanEval provider |
| `evaluation/providers/swebench.py` | 221 | SWE-bench provider |

### Trust (4 files, ~175 lines)

| File | Lines | Purpose |
|------|------:|---------|
| `trust/__init__.py` | 15 | Module exports |
| `trust/levels.py` | 45 | Trust level enum + helpers |
| `trust/middleware.py` | 35 | Stamp/validate trust annotations |
| `trust/scorer.py` | 79 | Risk scoring (mirrors Go scorer) |

---

## Summary & Recommendations

### Priority 1 (Critical -- fix before production)

1. **CRITICAL-001:** Add defense-in-depth command validation to the bash tool. Implement a blocklist of obviously destructive commands at the Python layer, independent of the Go policy engine.

### Priority 2 (High -- fix in next sprint)

2. **HIGH-001:** Add `AND tenant_id = %s` to `MemoryStore.recall()` query. Parameterize tenant_id instead of hardcoding.
3. **HIGH-002:** Add path traversal protection to `GlobFilesTool` -- either validate matched paths or sanitize the glob pattern.
4. **HIGH-003:** Add command allowlisting to `QualityGateExecutor` for defense-in-depth.
5. **HIGH-004:** Replace `object` type annotations with proper types in `agent_loop.py`.
6. **HIGH-005:** Replace `_processed_ids` set with an ordered data structure for FIFO eviction.

### Priority 3 (Medium -- plan for improvement)

7. **MEDIUM-001:** Fix rollout scoring formula to actually differentiate between results.
8. **MEDIUM-003:** Convert synchronous HTTP calls in routing to async.
9. **MEDIUM-004:** Extract shared embedding computation to a utility function.

### Priority 4 (Low -- address opportunistically)

10. **LOW-002:** Separate side effect from `should_auto_transition()` check.
11. **LOW-004:** Standardize on structlog across all modules.

### Architectural Recommendations

- **Decompose `_conversation.py`** (933 lines): Extract routing setup, skill injection, and MCP management into separate helper modules.
- **Add integration tests** for the NATS consumer message flow -- currently there appear to be no tests for the consumer mixins.
- **Document the `_handle_request()` template** pattern formally, as it is the backbone of all consumer behavior.
