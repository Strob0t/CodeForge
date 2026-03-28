# Agent Pipeline Improvement TODOs

**Date:** 2026-03-28
**Source:** S1-S4 test results + research on SWE-agent, OpenHands, Aider, MagenticOne, RouteLLM
**Priority:** Ordered by impact/effort ratio (highest first)

---

## CRITICAL — Fixes for Observed Failures

### TODO-1: Fix tool-calling for pure_completion models (S4 root cause)
**File:** `workers/codeforge/agent_loop.py:364-368`
**Problem:** `tools` array is passed to ALL models, including those that don't support OpenAI function-calling format. Qwen3 via LM Studio ignores the `tools` param, resulting in 0 tool calls.
**Fix:**
- [x] When `capability_level == "pure_completion"`, set `tools=None` in the `chat_completion_stream()` call
- [x] Instead, inject tool definitions into the system prompt via `tool_guide.py` (already exists)
- [x] Add a text-extraction parser that detects tool calls from free-text responses (regex for JSON blocks like `{"tool": "write_file", "args": {...}}`)
- [x] Test with qwen3-30b: send agentic message, verify tool calls are extracted from text
**Ref:** mini-SWE-agent achieves 74% on SWE-bench Lite with bash-only (no function calling). LiteLLM has `add_function_to_prompt = True` fallback.

### TODO-2: Dynamic context window limits based on model (S3 root cause)
**File:** `workers/codeforge/consumer/_conversation.py:202-206`
**Problem:** Hardcoded context limits (120K/32K/16K per capability tier) don't account for actual model context window. Qwen3-30b has 32K but may be classified differently.
**Fix:**
- [x] Query `model_info.max_input_tokens` from LiteLLM API (`GET /model/info`) at run start
- [x] Set context limit to `min(actual_model_window * 0.85, tier_default)` (0.85 leaves room for output)
- [x] Scale `DEFAULT_TOOL_OUTPUT_MAX_CHARS` proportionally: `min(10000, context_limit * 4 // 12)`
- [x] Log actual context budget in run metadata for debugging

### TODO-3: Add "no tool call" stall detection (S4 mitigation)
**File:** `workers/codeforge/stall_detection.py`
**Problem:** Current stall detector only catches repeated tool calls. S4's text-only response (0 tool calls) is undetected.
**Fix:**
- [x] After each LLM response, check: if `finish_reason == "stop"` AND `tool_calls == []` AND `iteration == 0` AND `len(content) > 100`
- [x] Re-prompt with: "You have tools available. Use them to complete the task. Call list_directory or read_file to start."
- [x] Allow max 2 re-prompts before falling back to text-extraction parser (TODO-1)
- [x] Add metric: `agent.no_tool_call_reprompts`

---

## HIGH — Improve Agent Behavior Quality

### TODO-4: Enforce explore-before-write for brownfield tasks
**File:** `workers/codeforge/tools/tool_guide.py` + `workers/codeforge/agent_loop.py`
**Problem:** Agent writes files without reading them first (S1: 0 read_file calls, S3: stalled before exploring).
**Fix:**
- [x] Add to base system prompt (not just tool guide): "MANDATORY: Before editing any file, read it first with read_file. Before modifying a codebase, explore with list_directory and glob_files."
- [x] In `ToolExecutor.execute()`: when `edit_file` is called for a path not yet `read_file`'d in this session, inject warning into tool result: "WARNING: Editing {path} without reading it first."
- [x] Track `files_read` set in `_LoopState` to enable this check
**Ref:** SWE-agent ACI: mandatory exploration step. AutoCodeRover: AST-guided exploration. Aider: repo map for orientation.

### TODO-5: Post-write auto-verification nudge
**File:** `workers/codeforge/agent_loop.py` (iteration quality tracking)
**Problem:** Agent writes 3+ files without running tests (S2: logic bugs undetected until validation).
**Fix:**
- [x] Track `writes_since_last_verify` counter in `_LoopState`
- [x] Increment on `write_file`/`edit_file`, reset on `bash` calls containing `test`/`pytest`/`compile`/`tsc`
- [x] When counter >= 3, inject nudge into next LLM turn: "You've written 3 files without verification. Run tests or syntax checks now."
- [x] Add to system prompt: "After writing each file, verify it. Never write multiple files without testing between them."
**Ref:** SWE-agent: edit command includes linting. AutoCodeRover: test-first approach.

### TODO-6: Context-aware stall escape prompts
**File:** `workers/codeforge/stall_detection.py`
**Problem:** Generic escape prompt ("try something different") insufficient for S3 — model couldn't break pattern.
**Fix:**
- [x] Replace generic `STALL_ESCAPE_PROMPT` with context-specific prompts based on stall type:
  - Stuck reading same files → "You've explored enough. Write the code now."
  - Stuck writing same file → "Read the error output and fix the specific issue."
  - Stuck on bash errors → "The approach isn't working. Try a different implementation strategy."
  - No progress tools at all → "Use write_file or bash to make concrete progress."
- [x] Include summary of recent tool results in escape prompt (what the agent has accomplished)
- [x] After 2 escape attempts, try MagenticOne-style re-planning: summarize accomplishments, ask for new plan, reset stall counter, resume

### TODO-7: Post-action state injection (SWE-agent pattern)
**File:** `workers/codeforge/tool_executor.py`
**Problem:** Agent loses context about what it's already done, causing repeated exploration.
**Fix:**
- [x] After each tool call, append brief state summary to tool result (~50 tokens):
  ```
  [State: files_modified=[main.py], files_read=[utils.py, config.py], tests_run=0, iteration=3/50]
  ```
- [x] Track in `_LoopState`: `files_modified`, `files_read`, `tests_run`, `errors_seen`
- [x] Only inject for models with < 64K context (unnecessary overhead for large-context models)

---

## MEDIUM — Evaluation & Metrics Infrastructure

### TODO-8: Request payload logging for debugging
**File:** `workers/codeforge/llm.py`
**Problem:** Cannot diagnose S3/S4 failures without seeing the exact request sent to LiteLLM.
**Fix:**
- [x] Add optional request logging: when `APP_ENV=development`, log the full request body (messages, tools, tool_choice, model) to a file before each `chat_completion_stream()` call
- [x] Log path: `/tmp/codeforge-llm-requests/{run_id}/{iteration}.json`
- [x] Include response summary (finish_reason, tool_calls count, content length)
- [x] Add env var `CODEFORGE_LOG_LLM_REQUESTS=true` to enable in non-dev environments

### TODO-9: Trajectory analysis metrics
**File:** `workers/codeforge/agent_loop.py` (run completion)
**Problem:** No structured data about HOW the agent solved (or failed) a task.
**Fix:**
- [x] At run completion, compute and publish trajectory metrics:
  - `tool_diversity`: unique tools used / total tools available
  - `explore_ratio`: (read_file + search + glob + list_dir) / total tool calls
  - `verify_frequency`: bash calls with test/compile / total tool calls
  - `error_recovery_rate`: successful edits after failed bash / total failed bash
  - `write_without_read_count`: edit_file calls on un-read files
- [x] Publish as part of `runs.complete` NATS payload
- [x] Display in run detail view in frontend

### TODO-10: Multi-run evaluation with pass@k
**Problem:** Single runs are noisy. Need statistical methodology.
**Fix:**
- [x] Add `repeat_count` parameter to conversation dispatch (run same prompt N times)
- [x] Compute pass@k: `1 - C(n-c, k) / C(n, k)` where n=total runs, c=passing runs, k=attempts
- [x] For S1-S4, run 5x each and report pass@5 instead of single-run pass/fail
- [x] Store results in `benchmark_runs` table (already exists)

### TODO-11: Model-capability-aware task gating
**File:** `workers/codeforge/routing/complexity.py`
**Problem:** COMPLEX tasks dispatched to 30B local models without warning.
**Fix:**
- [x] After complexity analysis, check if selected model can handle the tier
- [x] If `complexity >= COMPLEX` and `model.max_tokens <= 32768`, inject decomposition prompt: "This is a complex task. Break it into smaller subtasks and tackle each one sequentially."
- [x] Reduce `max_iterations` for SIMPLE tasks on local models (15 instead of 50)
- [x] Set `max_tokens` output from `_estimate_output_tokens()` (already computed but unused)

---

## LOW — Polish & Optimization

### TODO-12: Tiered system prompt size
**File:** `workers/codeforge/consumer/_conversation.py`
**Problem:** System prompt + tool guide + context = 5-8K tokens. For 32K model, that's 15-25% of context.
**Fix:**
- [x] Create 3 prompt tiers: full (120K+), compressed (32-64K), minimal (< 32K)
- [x] Compressed: remove examples from tool guide, shorten descriptions
- [x] Minimal: only essential rules, no examples, no context entries
- [x] Select tier based on actual model context window (from TODO-2)

### TODO-13: Proactive context summarization
**File:** `workers/codeforge/history.py`
**Problem:** Summarizer triggers at 60+ messages regardless of token count. For 32K models, overflow happens much earlier.
**Fix:**
- [x] Before each LLM call, check `estimated_tokens / context_limit`
- [x] If ratio > 0.7, trigger `ConversationSummarizer` immediately
- [x] If ratio > 0.9, aggressive truncation: remove all tool results except last 5

### TODO-14: Automated S1-S4 regression test
**Problem:** Manual test execution — should be automated.
**Fix:**
- [x] Create `scripts/run-agent-eval.sh` that:
  - Sets up S1 workspace
  - Starts services (if not running)
  - Creates project + conversation via API
  - Dispatches agentic message
  - Polls until completion (with timeout)
  - Runs validation checks
  - Outputs PASS/FAIL/PARTIAL with metrics JSON
- [x] Add to CI as a nightly job (not on every PR — too slow for local models)
- [x] Support `--model` flag to test with different models

---

## Research References

| Topic | Source | Key Finding |
|-------|--------|-------------|
| Tool format for open-weight | mini-SWE-agent | Bash-only (no function calling) achieves 74% SWE-bench Lite |
| Tool format fallback | LiteLLM docs | `add_function_to_prompt = True` for non-FC models |
| Qwen3 tool calling | HuggingFace model card | Uses Qwen-Agent templates, not raw OpenAI format |
| LM Studio tool support | Ollama blog | `tool_choice` not yet implemented; Qwen3 not in confirmed list |
| Explore-before-write | SWE-agent ACI | Custom file viewer, directory search returns names only, state after every action |
| Explore-before-write | AutoCodeRover | AST-guided exploration, spectrum-based fault localization |
| Verify-fix loop | SWE-agent | Edit command includes built-in linter, review-on-submit |
| Context management | Aider | Repo map (1/8 of context), chat history (1/16 of context) |
| Context management | OpenHands | LLM-generated summaries replace event chunks in-place |
| Stall recovery | MagenticOne | Dual-loop ledger (Task + Progress), re-planning after stall_count > 2 |
| Stall recovery | SWE-agent | Post-action state command returns JSON with context |
| Model routing | RouteLLM | Preference-based routing, 85% cost reduction at 95% GPT-4 quality |
| Model routing | Aider | Explicit weak/strong model pairing per config |
