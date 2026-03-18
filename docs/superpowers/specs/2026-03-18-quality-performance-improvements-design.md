# CodeForge Quality & Performance Improvements — Design Spec

> Date: 2026-03-18
> Status: Draft
> Scope: 12 measures across 3 categories (Agent Loop, Context Quality, Routing & Evaluation)
> Basis: Current research (LLMs for Coding 2025/26), codebase analysis of all relevant subsystems

---

## Category A: Agent Loop Quality

### A1. Stall Detection + Escape

**Goal:** Detect when the agent repeats the same actions and force a different approach.

**Problem:** `agent_loop.py` has no repetition detection. The agent can loop indefinitely (e.g., reading the same files 10x without writing). Agent-Eval showed 43 min wasted on repeated reads with 0/300 score.

**Design:**
- Sliding window of last 5 tool calls stored as `(tool_name, args_hash)` tuples
- `args_hash` = SHA-256 of first 200 chars of serialized args (cheap, collision-resistant)
- If >= 3 of last 5 entries are identical → inject escape prompt into next LLM call
- Escape prompt: `"<SYSTEM: You are repeating the same action without progress. Stop and try a fundamentally different approach. If you were reading, start writing. If you were searching, use what you found.>"`
- If stall detected again within 3 iterations after escape → abort loop with structured error `{"error": "stall_detected", "repeated_action": "...", "iterations": N}`
- Publish `trajectory.stall_detected` event for observability

**Files:**
- `workers/codeforge/agent_loop.py` — stall detection logic in main loop
- No Go/NATS/frontend changes required

**Atomic TODOs:**

- [ ] A1.1: Write test — 3 identical consecutive tool calls triggers stall detection
  - File: `workers/tests/test_stall_detection.py`
  - Test: `detect_stall([("Read", hash_a), ("Read", hash_a), ("Read", hash_a), ("Glob", hash_b), ("Search", hash_c)])` returns `True`
  - Test: `detect_stall([("Read", hash_a), ("Write", hash_b), ("Read", hash_a), ("Write", hash_c), ("Read", hash_a)])` returns `False` (not consecutive enough)
  - Test: `detect_stall([])` returns `False` (empty history)
  - Test: `detect_stall([("Read", hash_a)])` returns `False` (single entry)
  - Run: `cd workers && poetry run pytest tests/test_stall_detection.py -v`

- [ ] A1.2: Write test — stall detection with args_hash differentiates same tool with different args
  - File: `workers/tests/test_stall_detection.py`
  - Test: `detect_stall([("Read", hash_a), ("Read", hash_b), ("Read", hash_c), ...])` returns `False` (different files)
  - Test: `detect_stall([("Read", hash_a), ("Read", hash_a), ("Read", hash_a), ...])` returns `True` (same file)

- [ ] A1.3: Write test — escape prompt injection after stall detection
  - File: `workers/tests/test_stall_detection.py`
  - Test: After stall detected, next `_build_messages()` call includes escape system message
  - Test: Escape message content contains "different approach"

- [ ] A1.4: Write test — double stall triggers loop abort
  - File: `workers/tests/test_stall_detection.py`
  - Test: Stall detected → escape injected → 3 more identical calls → loop returns with `stall_detected` error
  - Test: Error payload contains `repeated_action` and `iterations`

- [ ] A1.5: Implement `StallDetector` class
  - File: `workers/codeforge/agent_loop.py` (new class, ~40 lines)
  - `record(tool_name: str, args: dict) -> None` — append `(tool_name, hashlib.sha256(json.dumps(args, sort_keys=True)[:200]).hexdigest())` to deque(maxlen=5)
  - `is_stalled() -> bool` — check if >= 3 of last 5 entries identical
  - `escape_count: int` — track how many escapes have been injected
  - `should_abort() -> bool` — return `escape_count >= 2`

- [ ] A1.6: Integrate `StallDetector` into `AgentLoopExecutor.run()` and `_execute_tool_call()`
  - File: `workers/codeforge/agent_loop.py`
  - After each tool call in `_execute_tool_call()`: `stall_detector.record(tool_name, tool_args)`
  - Before LLM call in `_do_llm_iteration()`: if `stall_detector.is_stalled()` → inject escape message, increment `escape_count`
  - After escape check: if `stall_detector.should_abort()` → break loop, publish error event

- [ ] A1.7: Publish `trajectory.stall_detected` event
  - File: `workers/codeforge/agent_loop.py`
  - Reuse existing `self._runtime.publish_trajectory_event()` with `event_type="stall_detected"`
  - Payload: `{"repeated_tool": tool_name, "escape_count": N, "iteration": current_iteration}`

- [ ] A1.8: Run all agent_loop tests to verify no regressions
  - Run: `cd workers && poetry run pytest tests/ -k "agent_loop or stall" -v`

---

### A2. Conversation Summarization at Context Exhaustion

**Goal:** When conversation exceeds 40 turns, summarize early turns instead of dropping them.

**Problem:** `context_budget.go` decays budget to 0 after 60 messages. `history.py` uses head-and-tail but never summarizes. Important early decisions vanish. Long conversations degrade in quality.

**Design:**
- Trigger: when history length > `SUMMARIZE_THRESHOLD` (default 40 messages)
- Summarize turns 1 through `len(history) - TAIL_SIZE` (keep last 20 intact)
- Use cheapest available model (Haiku-class via `tags=["background"]` for scenario-based routing)
- Summarization prompt: `"Summarize the key decisions, findings, and context from this conversation so far. Be concise. Use bullet points. Max 200 words."`
- Store summary as synthetic `{"role": "system", "content": "## Conversation Summary\n..."}` at history position 1 (after system prompt)
- Cache summary keyed by `(conversation_id, message_count_at_summarization)` — only re-summarize when 20+ new messages added since last summary
- Cost guard: max 1 summarization per 20 messages (not per iteration)

**Files:**
- `workers/codeforge/history.py` — summarization logic
- `workers/codeforge/llm.py` — LLM call for summarization (reuse existing `chat_completion`)

**Atomic TODOs:**

- [ ] A2.1: Write test — summarization triggers at threshold
  - File: `workers/tests/test_conversation_summarization.py`
  - Test: 39 messages → `should_summarize()` returns `False`
  - Test: 41 messages → `should_summarize()` returns `True`
  - Test: 41 messages but summary exists for count 40 → `should_summarize()` returns `False` (cached)
  - Test: 61 messages with summary from count 40 → `should_summarize()` returns `True` (20+ new)

- [ ] A2.2: Write test — summarization preserves tail messages
  - File: `workers/tests/test_conversation_summarization.py`
  - Test: 50 messages, tail_size=20 → summarize messages 1-30, keep 31-50 intact
  - Test: Summary inserted at position 1 (after system prompt)
  - Test: Original messages 1-30 removed from final history

- [ ] A2.3: Write test — summarization prompt format
  - File: `workers/tests/test_conversation_summarization.py`
  - Test: `_build_summarization_prompt(messages[1:30])` includes all user/assistant content
  - Test: Tool-call messages are compressed to `"[Tool: {name}] → {result[:100]}"`
  - Test: Output format starts with `"## Conversation Summary"`

- [ ] A2.4: Write test — summarization uses background routing tag (cheapest model)
  - File: `workers/tests/test_conversation_summarization.py`
  - Test: Mock LLM client → verify call uses `tags=["background"]` for scenario-based routing

- [ ] A2.5: Implement `ConversationSummarizer` class
  - File: `workers/codeforge/history.py` (new class, ~80 lines)
  - `should_summarize(history_len: int, last_summary_at: int | None) -> bool`
  - `build_summarization_messages(history: list[dict], tail_size: int) -> list[dict]`
  - `inject_summary(history: list[dict], summary: str, tail_size: int) -> list[dict]`
  - Summary cache: `dict[str, tuple[int, str]]` keyed by conversation_id → (msg_count, summary_text)

- [ ] A2.6: Implement `_summarize_history()` async method
  - File: `workers/codeforge/history.py`
  - Call `llm_client.chat_completion()` with summarization prompt
  - Parse response → extract summary text
  - Store in cache with current message count
  - Return summary string

- [ ] A2.7: Add `async summarize_if_needed()` pre-processing step (called before `build_messages()`)
  - File: `workers/codeforge/history.py`
  - Note: `build_messages()` is synchronous — do NOT make it async. Instead, add a separate
    `async summarize_if_needed(history, llm_client, conversation_id)` method that returns
    the (possibly summarized) history. Call this in `agent_loop.py` before `build_messages()`.
  - If `should_summarize()`: await `_summarize_history()`, then `inject_summary()`, return modified history
  - If not: return history unchanged
  - Callers to update: `AgentLoopExecutor.run()` in `agent_loop.py` (add await before build_messages)

- [ ] A2.8: Add `CODEFORGE_SUMMARIZE_THRESHOLD` env var (default 40)
  - File: `workers/codeforge/history.py`
  - Read via `os.environ.get("CODEFORGE_SUMMARIZE_THRESHOLD", "40")`

- [ ] A2.9: Run all history tests to verify no regressions
  - Run: `cd workers && poetry run pytest tests/ -k "history or summariz" -v`

---

### A3. Plan/Act Mode Toggle

**Goal:** Separate agent execution into a planning phase (read-only) and an acting phase (full tools), with different models per phase.

**Problem:** The agent plans and executes simultaneously. Complex tasks devolve into chaotic interleaving of reads and writes. The Mistral agent-eval failure (0/300) was caused by the agent never transitioning from reading to writing.

**Design:**
- Two-phase execution within `AgentLoopExecutor`:
  - **Plan phase:** max `PLAN_MAX_ITERATIONS` (default 5) iterations, tools restricted to `READ_ONLY_TOOLS = {"Read", "Search", "Glob", "ListDir"}`, routing scenario `"think"` (stronger model)
  - **Act phase:** remaining iterations, all tools allowed, routing scenario `"default"`
- Transition: Plan phase ends when agent emits a tool call with `name not in READ_ONLY_TOOLS` OR when `PLAN_MAX_ITERATIONS` reached
  - If ended by non-read tool: that tool call is queued as first action in Act phase
  - If ended by iteration limit: inject prompt `"Planning phase complete. Now execute your plan using Write, Edit, and Bash tools."`
- Plan/Act configurable per conversation via `plan_act_enabled` field (default: `True` for autonomy levels 4+5, `False` for 1-3)
- System prompt augmentation in Plan phase: append `"\n\nYou are in PLANNING mode. Only use Read, Search, Glob, and ListDir to understand the codebase. Do NOT write or edit files yet. Build a complete plan first."`

**Files:**
- `workers/codeforge/agent_loop.py` — phase management in `run()` and `_do_llm_iteration()`
- `internal/service/conversation_agent.go` — pass `plan_act_enabled` in NATS payload
- `internal/port/messagequeue/schemas.go` — add field to `ConversationRunStartPayload`

**Atomic TODOs:**

- [ ] A3.1: Write test — plan phase restricts tools to read-only set
  - File: `workers/tests/test_plan_act_mode.py`
  - Test: In plan phase, tool call `Write` is rejected with policy message `"Write not allowed in planning phase"`
  - Test: In plan phase, tool call `Read` is allowed
  - Test: In plan phase, tool call `Bash` is rejected

- [ ] A3.2: Write test — plan phase transitions to act phase on non-read tool call
  - File: `workers/tests/test_plan_act_mode.py`
  - Test: Agent calls `Read`, `Read`, `Search`, `Write` → phase switches after iteration 3, `Write` executes in act phase
  - Test: Transition preserves the pending `Write` call (not dropped)

- [ ] A3.3: Write test — plan phase transitions after max iterations
  - File: `workers/tests/test_plan_act_mode.py`
  - Test: Agent calls `Read` x5 (PLAN_MAX_ITERATIONS=5) → transition prompt injected
  - Test: Transition prompt contains "execute your plan"

- [ ] A3.4: Write test — plan/act disabled for autonomy levels 1-3
  - File: `workers/tests/test_plan_act_mode.py`
  - Test: `plan_act_enabled=False` → all tools available from iteration 1
  - Test: No plan-phase system prompt augmentation

- [ ] A3.5: Write test — plan phase uses "think" routing tag
  - File: `workers/tests/test_plan_act_mode.py`
  - Test: Mock LLM client → plan phase calls use `tags=["think"]` for scenario routing
  - Test: Act phase calls use `tags=["default"]` or no tag override
  - Note: Routing scenarios are resolved via tags at model selection time, not as LLM client params

- [ ] A3.6: Add `plan_act_enabled` to NATS payload (Go + Python)
  - File: `internal/port/messagequeue/schemas.go`
  - Add: `PlanActEnabled bool \`json:"plan_act_enabled"\`` to `ConversationRunStartPayload` struct
  - File: `workers/codeforge/models.py`
  - Add: `plan_act_enabled: bool = False` to `ConversationRunStartMessage` Pydantic model (line ~427)

- [ ] A3.7: Set `plan_act_enabled` based on mode autonomy level in dispatcher
  - File: `internal/service/conversation_agent.go`
  - In `SendMessageAgentic()`: `payload.PlanActEnabled = modeAutonomy >= 4` (uses existing `modeAutonomy` local var, lines 268-276)
  - In `SendMessageAgenticWithMode()`: same pattern — resolve autonomy from mode, check >= 4
  - Note: Autonomy is a property of the mode (`mode.Mode.Autonomy`), NOT the project

- [ ] A3.8: Implement `PlanActController` class
  - File: `workers/codeforge/agent_loop.py` (new class, ~50 lines)
  - `__init__(enabled: bool, max_plan_iterations: int = 5)`
  - `phase: Literal["plan", "act"]` — starts as `"plan"` if enabled, else `"act"`
  - `is_tool_allowed(tool_name: str) -> bool` — check against `READ_ONLY_TOOLS` in plan phase
  - `record_iteration(tool_name: str | None) -> None` — track iteration count, check transition
  - `should_transition() -> bool` — max iterations reached
  - `transition() -> str | None` — return transition prompt if transitioning, set phase to `"act"`
  - `get_system_prompt_suffix() -> str` — return plan-phase prompt addition or empty string
  - `get_routing_tags() -> list[str]` — return `["think"]` or `["default"]`

- [ ] A3.9: Integrate `PlanActController` into `AgentLoopExecutor.run()` and `_do_llm_iteration()`
  - File: `workers/codeforge/agent_loop.py`
  - Before tool execution in `_execute_tool_call()`: check `controller.is_tool_allowed(tool_name)`
  - If not allowed: skip tool, append rejection message to history
  - After each iteration in `run()`: `controller.record_iteration(tool_name)`
  - If `controller.should_transition()`: inject transition prompt
  - System prompt: append `controller.get_system_prompt_suffix()`
  - LLM call: use `tags=controller.get_routing_tags()` for scenario-based model selection

- [ ] A3.10: Add `CODEFORGE_PLAN_MAX_ITERATIONS` env var (default 5)
  - File: `workers/codeforge/agent_loop.py`
  - Read via `os.environ.get("CODEFORGE_PLAN_MAX_ITERATIONS", "5")`

- [ ] A3.11: Run full test suite
  - Run: `cd workers && poetry run pytest tests/ -k "plan_act" -v`
  - Run: `cd /workspaces/CodeForge && go test ./internal/port/messagequeue/ -v`

---

### A4. Inference-Time Scaling for Conversations

**Goal:** Enable multi-rollout execution for agent conversations (not just benchmarks), with confidence-based early stopping.

**Problem:** Normal conversations run as single-shot — one trajectory, one chance. The `MultiRolloutRunner` exists only in the benchmark path. Research shows best-of-4 brings ~15% absolute improvement.

**Design:**
- New field `rollout_count` on `ConversationRunStartPayload` (default 1)
- Configurable per project via `Agent.ConversationRolloutCount` config key
- When `rollout_count > 1`:
  1. Agent loop executes N times independently (sequential, same workspace snapshot)
  2. Each rollout gets a unique `rollout_id` (0 to N-1)
  3. Before each rollout: git stash/restore workspace to clean state
  4. After all rollouts: select best via hybrid verification (reuse existing pipeline)
  5. Apply selected rollout's changes to workspace
  6. Return selected rollout's messages as conversation history
- Confidence-based early stopping:
  - After each rollout: compute pairwise similarity with all previous rollouts
  - If >= 3 rollouts have output similarity > 0.9 AND all exit_code == 0 → stop early
  - Similarity: normalized Levenshtein on final diff (reuse `_diversity_score` from multi_rollout.py)
- Cost tracking: all rollouts' costs summed, `selected_rollout` and `total_rollouts` in trajectory
- Restriction: only for autonomy level >= 4 (no HITL approval conflicts across rollouts)

**Files:**
- `workers/codeforge/agent_loop.py` — multi-rollout wrapper
- `internal/service/conversation_agent.go` — payload field, config lookup
- `internal/port/messagequeue/schemas.go` — new field
- `workers/codeforge/models.py` — Pydantic model update

**Atomic TODOs:**

- [ ] A4.1: Write test — single rollout (default) behaves identically to current behavior
  - File: `workers/tests/test_conversation_rollout.py`
  - Test: `rollout_count=1` → no workspace snapshot, no selection, same output as before
  - Test: No git stash/restore operations called

- [ ] A4.2: Write test — multi-rollout executes N independent loops
  - File: `workers/tests/test_conversation_rollout.py`
  - Test: `rollout_count=3` → agent loop called 3 times with mock LLM
  - Test: Each rollout receives unique `rollout_id` in context
  - Test: Workspace restored between rollouts (git stash mock called N-1 times)

- [ ] A4.3: Write test — best rollout selected via hybrid verification
  - File: `workers/tests/test_conversation_rollout.py`
  - Test: 3 rollouts with scores [0.3, 0.9, 0.6] → rollout 1 selected
  - Test: Selected rollout's messages returned as conversation history
  - Test: Selected rollout's diff applied to workspace

- [ ] A4.4: Write test — confidence-based early stopping
  - File: `workers/tests/test_conversation_rollout.py`
  - Test: `rollout_count=8`, first 3 rollouts identical (similarity > 0.9, exit_code=0) → stops after 3
  - Test: `skipped_rollouts=5` in result metadata
  - Test: 3 rollouts with similarity < 0.9 → continues to rollout 4

- [ ] A4.5: Write test — cost aggregation across rollouts
  - File: `workers/tests/test_conversation_rollout.py`
  - Test: 3 rollouts costing $0.01 each → total_cost = $0.03
  - Test: Early-stopped at rollout 3 of 8 → total_cost = 3 * per_rollout_cost

- [ ] A4.5b: Write test — non-git workspace falls back to single rollout
  - File: `workers/tests/test_conversation_rollout.py`
  - Test: `rollout_count=3` with workspace lacking `.git` → logs warning, executes single rollout
  - Test: Result metadata contains `fallback_reason="no_git_repo"`

- [ ] A4.6: Add `rollout_count` to NATS payload (Go + Python)
  - File: `internal/port/messagequeue/schemas.go`
  - Add: `RolloutCount int \`json:"rollout_count"\`` to `ConversationRunStartPayload` (default 1)
  - File: `workers/codeforge/models.py`
  - Add: `rollout_count: int = 1` to `ConversationRunStartMessage` Pydantic model

- [ ] A4.7: Add `Agent.ConversationRolloutCount` config key
  - File: `internal/service/conversation_agent.go`
  - Lookup from project config, default 1, cap at 8
  - Only enable if `autonomy_level >= 4`

- [ ] A4.8: Implement `ConversationRolloutExecutor` class
  - File: `workers/codeforge/agent_loop.py` (new class, ~120 lines)
  - `__init__(agent_loop_executor, rollout_count, workspace_path)`
  - `async execute(payload) -> RolloutResult`
  - Internal: snapshot workspace (git stash), run loop, restore, collect results
  - Selection: reuse `HybridEvaluationPipeline` from evaluation module
  - Early stopping: pairwise similarity check after each rollout

- [ ] A4.9: Implement workspace snapshot/restore helpers
  - File: `workers/codeforge/agent_loop.py`
  - `_snapshot_workspace(path)` → `git stash push -m "rollout-{id}"` via subprocess
  - `_restore_workspace(path)` → `git stash pop` via subprocess
  - `_apply_rollout(path, rollout_id)` → checkout the selected rollout's stash

- [ ] A4.10: Integrate into NATS consumer dispatch
  - File: `workers/codeforge/consumer/_conversation.py` (or equivalent handler)
  - If `payload.rollout_count > 1`: wrap `AgentLoopExecutor` in `ConversationRolloutExecutor`
  - Else: direct execution (no change)

- [ ] A4.11: Add rollout metadata to trajectory events
  - File: `workers/codeforge/agent_loop.py`
  - Trajectory event: `{"rollout_id": N, "total_rollouts": M, "is_selected": bool}`
  - Final event: `{"selected_rollout": N, "selection_score": float, "early_stopped": bool}`

- [ ] A4.12: Run full test suite
  - Run: `cd workers && poetry run pytest tests/ -k "rollout" -v`
  - Run: `cd /workspaces/CodeForge && go test ./internal/... -count=1`

---

## Category B: Context Quality

### B1. LLM-Based Re-Ranking of Retrieval Results

**Goal:** After retrieval, use a cheap LLM call to re-rank the top-20 context candidates by relevance to the user's prompt.

**Problem:** `context_optimizer.go` merges 7 context sources, normalizes scores to 60-85, and packs by priority. No LLM-based relevance assessment — irrelevant chunks dilute the token budget.

**Design:**
- After `BuildConversationContext()` assembles candidates, before packing:
  1. Take top-20 candidates (sorted by current priority score)
  2. Send to Python reranker via NATS request-reply (`context.rerank.request`)
  3. Python calls LiteLLM with "background" scenario (cheapest model)
  4. Prompt: `"Rate each chunk's relevance (1-10) to the query: '{user_prompt}'\n\nChunks:\n1. {chunk_1[:200]}\n2. {chunk_2[:200]}\n..."`
  5. Parse scores, reorder candidates, pack top-8 into budget
- Fallback: if reranker times out (2s) or fails, use original priority ordering
- Cost: ~200 tokens input per query → ~$0.0001 per rerank call
- Config: `Agent.ContextReranking` (default: `true`), disable for cost-sensitive setups

**Files:**
- `internal/service/context_optimizer.go` — rerank call integration
- `workers/codeforge/consumer/_context.py` (new) — rerank handler
- `internal/port/messagequeue/queue.go` — new subject constants

**Atomic TODOs:**

- [ ] B1.1: Write test — reranker reorders candidates by LLM relevance scores
  - File: `workers/tests/test_context_reranker.py`
  - Test: 5 chunks with original priorities [85, 80, 75, 70, 65], LLM scores [3, 9, 5, 8, 1] → reordered to [80, 70, 75, 85, 65]
  - Test: Only top-K (configurable, default 8) returned after reranking

- [ ] B1.2: Write test — reranker prompt format is correct
  - File: `workers/tests/test_context_reranker.py`
  - Test: Prompt includes user query
  - Test: Each chunk truncated to 200 chars in prompt
  - Test: Numbered list format matches expected pattern

- [ ] B1.3: Write test — reranker graceful fallback on timeout/error
  - File: `workers/tests/test_context_reranker.py`
  - Test: LLM call raises `TimeoutError` → return original ordering unchanged
  - Test: LLM returns unparseable response → return original ordering unchanged
  - Test: Empty candidates list → return empty list (no LLM call)

- [ ] B1.4: Write test — reranker uses background routing tag (cheapest model)
  - File: `workers/tests/test_context_reranker.py`
  - Test: Mock LLM client → verify `tags=["background"]` for scenario-based routing

- [ ] B1.5: Add `context.rerank.request` / `context.rerank.response` NATS subjects
  - File: `internal/port/messagequeue/queue.go` — add Go constants: `SubjectContextRerankRequest`, `SubjectContextRerankResponse`
  - File: `workers/codeforge/consumer/_subjects.py` — add matching Python constants
  - Verify `context.>` wildcard is included in `STREAM_SUBJECTS` list in `_subjects.py` (line ~10)

- [ ] B1.6: Implement `ContextReranker` in Python worker
  - File: `workers/codeforge/consumer/_context.py` (new, ~80 lines)
  - `async def rerank(chunks: list[dict], query: str, llm_client, top_k: int = 8) -> list[dict]`
  - Build prompt with numbered chunks (truncated to 200 chars each)
  - Parse LLM response: extract per-chunk score (regex: `r"(\d+)\.\s*(\d+)"` for chunk_num → score)
  - Sort by score descending, return top_k
  - Timeout: 2s on LLM call

- [ ] B1.7: Add NATS request-reply handler for reranking
  - File: `workers/codeforge/consumer/_context.py`
  - Subscribe to `context.rerank.request`
  - Parse payload: `{query: str, chunks: [{content, kind, priority}]}`
  - Call `rerank()`, publish result to reply subject

- [ ] B1.7b: Write Go test — rerank request published and response integrated
  - File: `internal/service/context_optimizer_test.go`
  - Test: Mock NATS → verify rerank request published with correct subject and payload
  - Test: Mock NATS reply with reordered chunks → verify candidates reprioritized
  - Test: NATS timeout (2s) → verify original ordering preserved (fallback)

- [ ] B1.8: Integrate reranking into Go `ContextOptimizerService`
  - File: `internal/service/context_optimizer.go`
  - After candidate assembly, before packing:
  - If `config.Agent.ContextReranking` enabled and candidates > 8:
  - Publish rerank request to NATS, await reply (2s timeout)
  - On success: replace candidate priorities with reranked order
  - On timeout/error: log warning, proceed with original priorities

- [ ] B1.9: Add `Agent.ContextReranking` config key
  - File: `internal/config/config.go`
  - Default: `true`
  - Env var: `CODEFORGE_CONTEXT_RERANKING`

- [ ] B1.10: Verify JetStream stream config includes `context.>` wildcard
  - File: `workers/codeforge/consumer/_subjects.py` — verify `"context.>"` in `STREAM_SUBJECTS`
  - File: `internal/port/messagequeue/queue.go` — verify Go-side stream creation includes `context.>` (or add if missing)

- [ ] B1.11: Run tests
  - Run: `cd workers && poetry run pytest tests/test_context_reranker.py -v`
  - Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run Context -v`

---

### B2. Semantic Deduplication of Context Candidates

**Goal:** Remove redundant context entries that overlap in content before packing into the token budget.

**Problem:** The same file/function can appear via multiple sources (workspace scan, retrieval, GraphRAG). No deduplication — agent receives 3-5 near-identical chunks, wasting 20-40% of the token budget.

**Design:**
- After candidate assembly in `context_optimizer.go`, before reranking (B1):
  1. Group candidates by filepath (cheap, O(n))
  2. Within same filepath: merge overlapping line ranges (keep highest-priority entry)
  3. Cross-file dedup: compute content fingerprint (simhash of first 500 chars)
  4. If two entries from different files have simhash similarity > 0.92 → keep only the higher-priority one
- Implementation in Go (no NATS round-trip needed, pure CPU)
- Simhash: 64-bit fingerprint, Hamming distance < 5 = similar (well-established threshold)

**Files:**
- `internal/service/context_optimizer.go` — dedup logic before packing

**Atomic TODOs:**

- [ ] B2.1: Write test — same-file overlapping line ranges merged
  - File: `internal/service/context_optimizer_test.go`
  - Test: Two entries for `main.go` lines 1-30 (priority 80) and lines 20-50 (priority 70) → merged into 1-50, priority 80
  - Test: Two entries for `main.go` lines 1-10 and lines 50-60 → NOT merged (no overlap)
  - Test: Three entries for same file with cascading overlaps → single merged entry

- [ ] B2.2: Write test — cross-file near-duplicate removal
  - File: `internal/service/context_optimizer_test.go`
  - Test: Two entries from different files with identical content → keep higher priority
  - Test: Two entries with slightly different content (simhash distance > 5) → keep both
  - Test: Empty candidate list → no panic, return empty

- [ ] B2.3: Write test — dedup preserves entry count when no duplicates
  - File: `internal/service/context_optimizer_test.go`
  - Test: 10 unique entries → all 10 returned
  - Test: Ordering preserved (highest priority first)

- [ ] B2.3b: Write test — simhash edge cases
  - File: `internal/service/context_optimizer_test.go`
  - Test: `simhash64("")` → returns valid uint64 (no panic)
  - Test: `simhash64("ab")` → returns valid uint64 (fewer than 3 chars, no 3-gram shingles possible)
  - Test: `simhash64("日本語コード")` → returns valid uint64 (unicode content)
  - Test: Two identical strings → Hamming distance = 0
  - Test: Two completely different strings → Hamming distance > 5

- [ ] B2.4: Implement `simhash64(content string) uint64` helper
  - File: `internal/service/context_optimizer.go` (~20 lines)
  - FNV-1a hash of 3-gram shingles, bitwise majority vote into 64-bit fingerprint
  - `hammingDistance(a, b uint64) int` — popcount of XOR

- [ ] B2.5: Implement `deduplicateCandidates(candidates []ContextCandidate) []ContextCandidate`
  - File: `internal/service/context_optimizer.go` (~50 lines)
  - Step 1: Group by filepath, merge overlapping line ranges
  - Step 2: Compute simhash for each remaining candidate
  - Step 3: Pairwise comparison (O(n^2) but n < 100, so acceptable)
  - Step 4: Remove lower-priority entry if Hamming distance < 5

- [ ] B2.6: Integrate `deduplicateCandidates()` into `BuildConversationContext()`
  - File: `internal/service/context_optimizer.go`
  - Call after candidate assembly, before reranking/packing
  - Log: `"dedup removed %d candidates (%d -> %d)"`

- [ ] B2.7: Run tests
  - Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run "Dedup\|ContextOptimizer" -v`

---

### B3. Adaptive Context Budget Based on Task Complexity

**Goal:** Scale the context token budget based on the ComplexityAnalyzer's classification instead of using a fixed value.

**Problem:** Static budget (2048 tokens for conversations). A one-line fix gets the same context as a cross-module refactoring. The ComplexityAnalyzer (Phase 29) already classifies tasks into 4 tiers but this classification is only used for model selection, not context budgeting.

**Design:**
- Map complexity tier to context budget multiplier:
  - `SIMPLE` → 0.25x (512 tokens) — less noise, fewer irrelevant chunks
  - `MEDIUM` → 1.0x (2048 tokens) — current default
  - `COMPLEX` → 2.0x (4096 tokens) — needs more context
  - `REASONING` → 2.0x + mandatory GraphRAG (4096 tokens + graph traversal)
- Complexity tier determined by Go Core before dispatch (call Python ComplexityAnalyzer via NATS, or re-implement the rule-based analyzer in Go since it's pure heuristic)
- Preferred approach: Re-implement in Go (no NATS round-trip, < 1ms, the analyzer is 7 weighted heuristics — no ML needed)
- Pass `complexity_tier` in NATS payload → Python worker uses for routing (already exists) + context budget

**Files:**
- `internal/service/context_budget.go` — tier-based budget scaling
- `internal/service/context_optimizer.go` — pass tier to budget calculation
- `internal/service/conversation_agent.go` — compute complexity before dispatch

**Atomic TODOs:**

- [ ] B3.1: Write test — complexity tier maps to correct budget
  - File: `internal/service/context_budget_test.go`
  - Test: `ComplexityBudget("simple", 2048)` returns 512
  - Test: `ComplexityBudget("medium", 2048)` returns 2048
  - Test: `ComplexityBudget("complex", 2048)` returns 4096
  - Test: `ComplexityBudget("reasoning", 2048)` returns 4096
  - Test: `ComplexityBudget("unknown", 2048)` returns 2048 (default fallback)

- [ ] B3.2: Write test — adaptive budget + phase scaling compose correctly
  - File: `internal/service/context_budget_test.go`
  - Test: tier=`complex` (2.0x) + phase=`reviewer` (50%) + turn 30 (50% decay) → `4096 * 0.5 * 0.5 = 1024`
  - Test: tier=`simple` (0.25x) + phase="" (100%) + turn 0 (100% decay) → `512`

- [ ] B3.3: Write test — Go-side complexity classifier
  - File: `internal/service/complexity_test.go`
  - Test: Short prompt "fix typo in README" → `SIMPLE`
  - Test: Prompt with code block + multi-step instructions → `COMPLEX`
  - Test: Prompt with "analyze", "compare", "design" → `REASONING`
  - Test: Medium-length prompt without special markers → `MEDIUM`

- [ ] B3.4: Implement `ClassifyComplexity(prompt string) string` in Go
  - File: `internal/service/complexity.go` (new, ~80 lines)
  - Port the 7 heuristics from `workers/codeforge/routing/complexity.py`:
    1. Code presence (backtick blocks, file paths)
    2. Reasoning indicators (analyze, compare, design, explain why)
    3. Technical term density
    4. Prompt length (short < 100 chars, long > 500)
    5. Multi-step indicators (numbered lists, "then", "after that")
    6. Context dependency (references to files, functions, previous conversation)
    7. Output complexity (generate, implement, refactor vs. fix, rename)
  - Weighted score → tier mapping (same thresholds as Python `complexity.py` lines 19-24: <0.25 SIMPLE, <0.50 MEDIUM, <0.75 COMPLEX, >=0.75 REASONING)

- [ ] B3.5: Implement `ComplexityBudget(tier string, baseBudget int) int`
  - File: `internal/service/context_budget.go` (~15 lines)
  - Map tier to multiplier, apply to base budget, return result

- [ ] B3.6: Add `complexity_tier` to NATS payload (Go + Python) and integrate into dispatch
  - File: `internal/port/messagequeue/schemas.go`
  - Add: `ComplexityTier string \`json:"complexity_tier"\`` to `ConversationRunStartPayload`
  - File: `workers/codeforge/models.py`
  - Add: `complexity_tier: str = "medium"` to `ConversationRunStartMessage`
  - File: `internal/service/conversation_agent.go`
  - In `SendMessageAgentic()`: call `ClassifyComplexity(userMessage)` before context assembly
  - Set `payload.ComplexityTier = tier`
  - Pass tier to `BuildConversationContext()` for budget calculation

- [ ] B3.7: Run tests
  - Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run "Complexity\|Budget" -v`

---

## Category C: Routing & Evaluation

### C1. Routing Transparency + Mid-Loop Model Switching

**Goal:** Make routing decisions visible (observable) and enable model switching during agent loop execution when the current model underperforms.

**Problem:** Routing decisions are fire-and-forget. No trace of why a model was selected. If a model fails mid-loop, no dynamic switching — the agent continues with the same model until loop exhaustion.

**Design:**
- **Transparency:**
  - Routing metadata published as trajectory event after each LLM call
  - Payload: `{complexity_tier, mab_score, selected_model, reason, alternatives: [{model, score}]}`
  - Frontend: model badge on each assistant message (clickable for routing detail popover)
- **Mid-Loop Switching:**
  - After each iteration: compute `iteration_quality_signal` based on:
    - Tool call success rate (last 3 iterations)
    - Whether agent made progress (new files read/written vs. repeats)
    - Whether output was meaningful (>50 chars, not just whitespace)
  - If `iteration_quality_signal < 0.3` for 2 consecutive iterations:
    - Bump complexity tier by 1 level
    - Request new model from MAB selector with bumped tier
    - Log model switch in trajectory
  - Maximum 2 model switches per loop (prevent oscillation)

**Files:**
- `workers/codeforge/agent_loop.py` — quality signal + switch logic
- `workers/codeforge/routing/router.py` — expose routing metadata
- Frontend: `ChatPanel.tsx` — model badge display (optional, separate PR)

**Atomic TODOs:**

- [ ] C1.1: Write test — routing metadata published as trajectory event
  - File: `workers/tests/test_routing_transparency.py`
  - Test: After LLM call, trajectory event contains `complexity_tier`, `selected_model`, `reason`
  - Test: `alternatives` list contains at least 1 other model option with score

- [ ] C1.2: Write test — iteration quality signal computation
  - File: `workers/tests/test_routing_transparency.py`
  - Test: 3 successful tool calls → signal = 1.0
  - Test: 3 failed tool calls → signal = 0.0
  - Test: 1 success, 1 fail, 1 empty output → signal ≈ 0.33
  - Test: No tool calls (pure text response) → signal = 0.5 (neutral)

- [ ] C1.3: Write test — model switch triggers on low quality signal
  - File: `workers/tests/test_routing_transparency.py`
  - Test: 2 consecutive iterations with signal < 0.3 → model switch requested
  - Test: 1 low + 1 high → no switch (not consecutive)
  - Test: Complexity tier bumped from SIMPLE to MEDIUM on switch

- [ ] C1.4: Write test — max 2 model switches per loop
  - File: `workers/tests/test_routing_transparency.py`
  - Test: 3rd switch attempt → ignored, log warning, continue with current model
  - Test: Switch count tracked in trajectory metadata

- [ ] C1.5: Extend `HybridRouter.route()` to return routing metadata
  - File: `workers/codeforge/routing/router.py` (method at line ~95)
  - Change return type: `(model: str, metadata: RoutingMetadata)` where `RoutingMetadata = {tier, mab_score, reason, alternatives}`
  - Update all callers to accept tuple (backward-compatible via `metadata` default `None`)

- [ ] C1.6: Implement `IterationQualityTracker` class
  - File: `workers/codeforge/agent_loop.py` (new class, ~40 lines)
  - `record(tool_success: bool, output_length: int) -> None`
  - `signal() -> float` — weighted average of last 3 iterations
  - `should_switch() -> bool` — 2+ consecutive low signals AND switches < max
  - `switch_count: int`

- [ ] C1.7: Integrate quality tracker + routing metadata into agent loop
  - File: `workers/codeforge/agent_loop.py`
  - After each tool call: `quality_tracker.record(success, len(output))`
  - After each LLM call: publish routing metadata as trajectory event
  - If `quality_tracker.should_switch()`: bump tier, re-select model via router

- [ ] C1.8: Run tests
  - Run: `cd workers && poetry run pytest tests/test_routing_transparency.py -v`

---

### C2. Confidence-Based Early Stopping for Multi-Rollout

**Goal:** Stop multi-rollout execution early when enough rollouts agree, saving 40-60% cost.

**Problem:** `MultiRolloutRunner` runs all N rollouts unconditionally. At N=8, cost is 8x even when first 3 produce identical results.

**Design:**
- After each rollout (starting from rollout 2):
  1. Compute pairwise similarity with all completed rollouts (reuse `_diversity_score`)
  2. Form agreement clusters: group rollouts where all pairwise similarities > `EARLY_STOP_THRESHOLD` (default 0.9)
  3. If any cluster size >= `EARLY_STOP_QUORUM` (default 3) AND all members have `exit_code == 0`:
     → Stop early, select representative from largest cluster (highest eval score)
- Track in result: `early_stopped: bool`, `completed_rollouts: int`, `skipped_rollouts: int`
- No early stop if `rollout_count <= 3` (quorum impossible with fewer rollouts)

**Files:**
- `workers/codeforge/evaluation/runners/multi_rollout.py` — early stopping logic

**Atomic TODOs:**

- [ ] C2.1: Write test — early stopping when quorum reached
  - File: `workers/tests/test_early_stopping.py`
  - Test: 3 identical rollouts (similarity > 0.9, exit_code=0), rollout_count=8 → stops after 3
  - Test: Result contains `early_stopped=True`, `completed_rollouts=3`, `skipped_rollouts=5`

- [ ] C2.2: Write test — no early stopping when similarity below threshold
  - File: `workers/tests/test_early_stopping.py`
  - Test: 3 rollouts with similarity 0.7 → continues to rollout 4
  - Test: 8 diverse rollouts → all 8 executed

- [ ] C2.3: Write test — no early stopping when exit_code != 0
  - File: `workers/tests/test_early_stopping.py`
  - Test: 3 identical rollouts but exit_code=1 on one → no early stop
  - Test: Only exit_code=0 rollouts count toward quorum

- [ ] C2.4: Write test — no early stopping when rollout_count <= 3
  - File: `workers/tests/test_early_stopping.py`
  - Test: `rollout_count=3` → all 3 always executed regardless of similarity
  - Test: `rollout_count=2` → both always executed

- [ ] C2.5: Write test — cluster detection with partial agreement
  - File: `workers/tests/test_early_stopping.py`
  - Test: 5 rollouts, first 3 agree, last 2 different → stops after 3 (cluster [0,1,2] >= quorum)
  - Test: 5 rollouts, pairs agree but no group of 3 → continues all 5

- [ ] C2.6: Implement `EarlyStopChecker` class
  - File: `workers/codeforge/evaluation/runners/multi_rollout.py` (~50 lines)
  - `__init__(threshold: float = 0.9, quorum: int = 3)`
  - `add_rollout(rollout_id: int, output: str, exit_code: int) -> None`
  - `should_stop() -> bool` — cluster analysis
  - `best_from_cluster() -> int` — return rollout_id of highest-scoring member in largest cluster

- [ ] C2.7: Integrate `EarlyStopChecker` into `MultiRolloutRunner.run()`
  - File: `workers/codeforge/evaluation/runners/multi_rollout.py`
  - After each rollout: `checker.add_rollout(i, output, exit_code)`
  - If `checker.should_stop()`: break loop, select via `checker.best_from_cluster()`
  - Add `early_stopped`, `completed_rollouts`, `skipped_rollouts` to result metadata

- [ ] C2.8: Add `EARLY_STOP_THRESHOLD` and `EARLY_STOP_QUORUM` config
  - File: `workers/codeforge/evaluation/runners/multi_rollout.py`
  - Env vars: `CODEFORGE_EARLY_STOP_THRESHOLD` (default 0.9), `CODEFORGE_EARLY_STOP_QUORUM` (default 3)

- [ ] C2.9: Run tests
  - Run: `cd workers && poetry run pytest tests/test_early_stopping.py tests/test_multi_rollout_runner.py -v`

---

### C3. New Benchmark Providers: DPAI Arena + Terminal-Bench

**Goal:** Add two research-grade benchmarks that test exactly the capabilities CodeForge orchestrates.

**Problem:** SWE-bench is saturating (~75% top scores). CodeForge needs harder, more realistic benchmarks. DPAI Arena tests full engineering lifecycle (what CodeForge does). Terminal-Bench tests CLI workflows (what CodeForge's Bash tool does).

**Design:**
- Both follow existing `BenchmarkProvider` protocol (self-registering)
- **DPAI Arena:** Multi-file tasks with PR review + static analysis scoring
  - Source: JetBrains DPAI Arena API or dataset download
  - Type: `AGENT` (full agent loop needed)
  - Evaluation: functional tests + code quality metrics
- **Terminal-Bench:** Shell command sequences with filesystem verification
  - Source: Terminal-Bench dataset (GitHub)
  - Type: `TOOL_USE` (Bash tool + verification)
  - Evaluation: filesystem state comparison (expected vs actual)

**Files:**
- `workers/codeforge/evaluation/providers/dpai_arena.py` (new)
- `workers/codeforge/evaluation/providers/terminal_bench.py` (new)
- `workers/codeforge/evaluation/cache.py` — dataset download helpers

**Atomic TODOs:**

- [ ] C3.1: Research DPAI Arena dataset format and access method
  - Document: dataset URL, authentication, task schema, evaluation criteria
  - Write findings to `docs/research/dpai-arena-integration.md`

- [ ] C3.2: Research Terminal-Bench dataset format and access method
  - Document: dataset URL, authentication, task schema, evaluation criteria
  - Write findings to `docs/research/terminal-bench-integration.md`

- [ ] C3.3: Write test — DPAI Arena provider loads tasks
  - File: `workers/tests/test_dpai_arena_provider.py`
  - Test: Mock dataset → `load_tasks()` returns list of `BenchmarkTask` with correct fields
  - Test: `benchmark_type` == `AGENT`
  - Test: `capabilities.functional_tests` == True

- [ ] C3.4: Write test — Terminal-Bench provider loads tasks
  - File: `workers/tests/test_terminal_bench_provider.py`
  - Test: Mock dataset → `load_tasks()` returns list of `BenchmarkTask` with correct fields
  - Test: `benchmark_type` == `TOOL_USE`
  - Test: Task contains `expected_filesystem_state` for verification

- [ ] C3.5: Implement `DPAIArenaProvider`
  - File: `workers/codeforge/evaluation/providers/dpai_arena.py` (~120 lines)
  - Inherit `BenchmarkProvider` protocol
  - `load_tasks()`: download/cache dataset, parse into `BenchmarkTask` instances
  - Register via `register_provider("dpai_arena", DPAIArenaProvider)`

- [ ] C3.6: Implement `TerminalBenchProvider`
  - File: `workers/codeforge/evaluation/providers/terminal_bench.py` (~100 lines)
  - Inherit `BenchmarkProvider` protocol
  - `load_tasks()`: download/cache dataset, parse into `BenchmarkTask` instances
  - Register via `register_provider("terminal_bench", TerminalBenchProvider)`

- [ ] C3.7: Add filesystem state evaluator for Terminal-Bench
  - File: `workers/codeforge/evaluation/evaluators/filesystem_verifier.py` (new, ~60 lines)
  - Compare expected vs actual directory tree after agent execution
  - Score: Jaccard similarity of file paths + content match ratio

- [ ] C3.8: Add `dpai_arena` and `terminal_bench` to Go `ValidMetrics` and suite defaults
  - File: `internal/domain/benchmark/benchmark.go`
  - Add to provider type mapping so Go validates correctly

- [ ] C3.9: Run tests
  - Run: `cd workers && poetry run pytest tests/test_dpai_arena_provider.py tests/test_terminal_bench_provider.py -v`

---

### C4. RLVR Training Pipeline Export

**Goal:** Export agent trajectories in RLVR-compatible format for fine-tuning local models with verifiable rewards.

**Problem:** CodeForge exports DPO pairs but not full RLVR datasets. Agent runs already produce the perfect RLVR data: prompts, responses, and verifiable rewards (test pass/fail, SPARC scores). No competitor offers this.

**Design:**
- New endpoint: `GET /api/v1/benchmarks/runs/{id}/export/rlvr`
- Export format: JSONL with one line per trajectory step:
  ```json
  {
    "prompt": "system prompt + history up to this step",
    "response": "assistant message at this step",
    "reward": 0.85,
    "reward_components": {
      "test_pass": 1.0,
      "sparc_score": 0.7,
      "trajectory_score": 0.85
    },
    "metadata": {
      "run_id": "...",
      "task_id": "...",
      "step": 3,
      "model": "...",
      "total_cost": 0.02
    }
  }
  ```
- Reward computation:
  - `test_pass`: binary (1.0 if exit_code == 0, 0.0 otherwise) — verifiable
  - `sparc_score`: SPARC evaluator output (0.0-1.0) — heuristic
  - `trajectory_score`: Trajectory verifier output (0.0/0.5/1.0) — LLM-judged
  - Combined: `0.5 * test_pass + 0.3 * sparc_score + 0.2 * trajectory_score`
- Compatible with: OpenRLHF, TRL (HuggingFace), veRL format
- Multi-run aggregation: `POST /api/v1/benchmarks/export/rlvr` with `{"run_ids": [...]}` → merged JSONL

**Files:**
- `internal/adapter/http/handlers_benchmark_analyze.go` — new export endpoint
- `internal/service/benchmark.go` — `ExportRLVRDataset()` method
- `workers/codeforge/evaluation/export.py` (new) — RLVR formatting logic

**Atomic TODOs:**

- [ ] C4.1: Write test — RLVR export produces valid JSONL
  - File: `internal/service/benchmark_test.go`
  - Test: Run with 3 results → JSONL with 3+ lines (one per trajectory step per result)
  - Test: Each line has required fields: `prompt`, `response`, `reward`, `reward_components`, `metadata`
  - Test: JSON parses correctly per line

- [ ] C4.2: Write test — reward computation is correct
  - File: `workers/tests/test_rlvr_export.py`
  - Test: `test_pass=1.0, sparc=0.8, trajectory=0.5` → reward = `0.5*1.0 + 0.3*0.8 + 0.2*0.5 = 0.84`
  - Test: `test_pass=0.0, sparc=0.0, trajectory=0.0` → reward = 0.0
  - Test: Missing sparc_score → reward uses only available components (re-weighted)

- [ ] C4.3: Write test — multi-run aggregation merges correctly
  - File: `internal/service/benchmark_test.go`
  - Test: 2 runs with 3 results each → merged JSONL with 6+ lines
  - Test: Each line's `metadata.run_id` matches source run
  - Test: Empty run list → empty response (not error)

- [ ] C4.4: Write test — export endpoint returns correct HTTP response
  - File: `internal/adapter/http/handlers_test.go`
  - Test: `GET /api/v1/benchmarks/runs/{id}/export/rlvr` → HTTP 200, Content-Type `application/jsonl`
  - Test: Non-existent run → HTTP 404
  - Test: Run still running → HTTP 409 (conflict)
  - Test: Completed run with 0 results → HTTP 200 with empty JSONL body (not error)

- [ ] C4.5: Implement `compute_rlvr_reward()` in Python
  - File: `workers/codeforge/evaluation/export.py` (new, ~60 lines)
  - `compute_rlvr_reward(test_pass: float, sparc: float | None, trajectory: float | None) -> tuple[float, dict]`
  - Return `(combined_reward, reward_components_dict)`
  - Handle missing components: re-weight remaining to sum 1.0

- [ ] C4.6: Implement `format_rlvr_entry()` in Python
  - File: `workers/codeforge/evaluation/export.py`
  - `format_rlvr_entry(task, result, step_messages, reward, metadata) -> str`
  - Return JSON string (one line) with all fields

- [ ] C4.7: Implement `ExportRLVRDataset()` in Go service
  - File: `internal/service/benchmark.go`
  - Load run + all results + trajectory data
  - For each result: reconstruct prompt/response pairs from trajectory
  - Compute reward from result scores (call Python formatter or compute in Go)
  - Return as `[]byte` (JSONL)

- [ ] C4.8: Add HTTP endpoint `GET /api/v1/benchmarks/runs/{id}/export/rlvr`
  - File: `internal/adapter/http/handlers_benchmark_analyze.go`
  - Call `benchmarkService.ExportRLVRDataset(ctx, runID)`
  - Set `Content-Type: application/jsonl`, `Content-Disposition: attachment; filename=rlvr-{runID}.jsonl`

- [ ] C4.9: Add HTTP endpoint `POST /api/v1/benchmarks/export/rlvr` for multi-run
  - File: `internal/adapter/http/handlers_benchmark_analyze.go`
  - Parse `{"run_ids": ["...", "..."]}` from body
  - Call `ExportRLVRDataset()` per run, concatenate results
  - Set same headers as single-run endpoint

- [ ] C4.10: Add route registration
  - File: `internal/adapter/http/routes.go`
  - `GET /api/v1/benchmarks/runs/{id}/export/rlvr` → handler
  - `POST /api/v1/benchmarks/export/rlvr` → handler

- [ ] C4.11: Run tests
  - Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run RLVR -v`
  - Run: `cd /workspaces/CodeForge && go test ./internal/adapter/http/ -run RLVR -v`
  - Run: `cd workers && poetry run pytest tests/test_rlvr_export.py -v`

---

## Implementation Order

Recommended sequence (respects dependencies, maximizes early value):

```
Phase 1 (Quick Wins, no dependencies):
  A1 (Stall Detection)        — 3h, standalone
  B3 (Adaptive Context Budget) — 2h, standalone
  C2 (Early Stopping)          — 2h, standalone

Phase 2 (Core Quality, A1 helps validate):
  A3 (Plan/Act Mode)           — 5h, benefits from A1 stall detection
  B2 (Semantic Dedup)           — 3h, standalone

Phase 3 (Context Intelligence):
  B1 (LLM Re-Ranking)          — 4h, benefits from B2 dedup first
  A2 (Summarization)            — 6h, standalone

Phase 4 (Advanced Features):
  C1 (Routing Transparency)    — 4h, makes A4 debuggable
  A4 (Inference-Time Scaling)  — 8h, depends on C2's EarlyStopChecker + C1 for debugging
    Note: A4.8 ConversationRolloutExecutor reuses HybridEvaluationPipeline from
    evaluation module — this is a cross-module dependency

Phase 5 (Ecosystem, independent of each other):
  C3 (New Benchmarks)           — 6h, standalone, needs research first
  C4 (RLVR Export)              — 8h, standalone (works on any benchmark run data, no C3 dependency)
```

Total estimated effort: ~51h across 12 measures.
