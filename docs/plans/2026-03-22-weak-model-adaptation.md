# Weak Model Adaptation — Implementation Plan

> Steps use checkbox (`- [ ]`) syntax for tracking. Execute task-by-task, commit after each.

**Goal:** Make CodeForge's agent loop work reliably with weak/local LLMs (qwen3-30b, llama, etc.)

**Spec:** `docs/specs/2026-03-22-weak-model-adaptation-design.md`

---

## Task 1: Tool Filtering by Capability Level (M1)

**Files:**
- Modify: `workers/codeforge/tools/capability.py`
- Modify: `workers/codeforge/agent_loop.py`
- Test: `workers/tests/test_capability.py` (new or extend)

- [ ] **Step 1: Add TOOL_ALLOWLIST per capability level to capability.py**

```python
# Tools allowed per capability level.
# Mode-declared tools (mode.tools) are always added on top.
TOOLS_BY_CAPABILITY: dict[CapabilityLevel, frozenset[str]] = {
    CapabilityLevel.FULL: frozenset(),  # empty = all tools allowed
    CapabilityLevel.API_WITH_TOOLS: frozenset({
        "read_file", "write_file", "edit_file", "bash",
        "search_files", "glob_files", "list_directory",
        "propose_goal", "handoff", "transition_to_act",
    }),
    CapabilityLevel.PURE_COMPLETION: frozenset({
        "read_file", "write_file", "bash", "search_files",
        "propose_goal", "transition_to_act",
    }),
}
```

- [ ] **Step 2: Write test for tool filtering**

```python
def test_pure_completion_filters_tools():
    allowed = TOOLS_BY_CAPABILITY[CapabilityLevel.PURE_COMPLETION]
    assert "read_file" in allowed
    assert "write_file" in allowed
    assert "bash" in allowed
    assert "create_skill" not in allowed
    assert "handoff" not in allowed
    assert "edit_file" not in allowed

def test_full_allows_all():
    allowed = TOOLS_BY_CAPABILITY[CapabilityLevel.FULL]
    assert len(allowed) == 0  # empty = no filtering
```

- [ ] **Step 3: Add _filter_tools_for_capability() to agent_loop.py**

In `AgentLoopExecutor.run()`, after tools_array is built, filter based on capability:

```python
def _filter_tools_for_capability(
    self,
    tools_array: list[dict],
    capability: CapabilityLevel,
    mode_tools: frozenset[str] | None = None,
) -> list[dict]:
    allowed = TOOLS_BY_CAPABILITY.get(capability, frozenset())
    if not allowed:  # FULL capability = no filtering
        return tools_array
    # Merge mode-declared tools
    if mode_tools:
        allowed = allowed | mode_tools
    return [t for t in tools_array if t["function"]["name"] in allowed]
```

- [ ] **Step 4: Integrate into the agent loop iteration**

In `_do_llm_iteration()`, before calling LLM:
```python
filtered_tools = self._filter_tools_for_capability(tools_array, state.capability_level, state.mode_tools)
```

- [ ] **Step 5: Run tests, ruff check/format**
- [ ] **Step 6: Commit**

```bash
git commit -m "feat: filter tools by capability level — weak models see fewer tools (M1)"
```

---

## Task 2: Step-by-Step Pipeline Prompt (M2)

**Files:**
- Create: `internal/service/prompts/model_adaptive/step_by_step.yaml`
- Modify: `workers/codeforge/consumer/_conversation.py`

- [ ] **Step 1: Create step_by_step.yaml**

```yaml
# Injected for pure_completion and api_with_tools models.
# Forces structured workflow and one-tool-per-turn discipline.

WORKFLOW RULES:
1. Make exactly ONE tool call per turn. Wait for the result before proceeding.
2. Before each tool call, write 1-2 sentences explaining what you will do and why.
3. Always read a file before modifying it.
4. Write COMPLETE file contents with write_file — never use placeholders like "..." or "// rest of code".
5. After writing a code file, verify it: run syntax check (python -m py_compile for Python, npx tsc --noEmit for TypeScript).
6. If a tool call fails, read the error carefully. Try a DIFFERENT approach — do not retry the same call.

WORKFLOW FOR MULTI-FILE PROJECTS:
Step 1 — PLAN: List all files you need to create, in order of dependencies.
Step 2 — SETUP: Create directory structure (bash mkdir -p).
Step 3 — IMPLEMENT: Create one file at a time with write_file. Start with files that have no dependencies.
Step 4 — VERIFY: After each file, run a syntax check or test.
Step 5 — INTEGRATE: When all files exist, run the full test suite.
Step 6 — COMMIT: Use bash to run git add and git commit.
```

- [ ] **Step 2: Inject step_by_step prompt for weak models in _conversation.py**

Find where model_adaptive prompts are injected. Add step_by_step for pure_completion and api_with_tools.

- [ ] **Step 3: Validate YAML syntax**
- [ ] **Step 4: Commit**

```bash
git commit -m "feat: step-by-step workflow prompt for weak models (M2)"
```

---

## Task 3: Context Limits by Capability (M3)

**Files:**
- Modify: `workers/codeforge/consumer/_conversation.py`

- [ ] **Step 1: After capability classification, override context limits**

```python
# Adjust context limits based on model capability
CONTEXT_LIMITS = {
    CapabilityLevel.FULL: 120_000,
    CapabilityLevel.API_WITH_TOOLS: 32_000,
    CapabilityLevel.PURE_COMPLETION: 16_000,
}

# In the conversation handler, after classify_model():
cap_limit = CONTEXT_LIMITS.get(capability_level, 120_000)
if run_msg.max_context_tokens == 0 or run_msg.max_context_tokens > cap_limit:
    run_msg.max_context_tokens = cap_limit
```

- [ ] **Step 2: Run ruff, verify**
- [ ] **Step 3: Commit**

```bash
git commit -m "feat: capability-based context limits — 16K for weak models (M3)"
```

---

## Task 4: Complete Tool Guidance (M4)

**Files:**
- Modify: `workers/codeforge/tools/create_skill.py`
- Modify: `workers/codeforge/tools/propose_goal.py`

- [ ] **Step 1: Add guidance to create_skill**

Add when_to_use, common_mistakes to DEFINITION. Focus on "when NOT to use":
```python
when_to_use="Use ONLY when you've discovered a genuinely reusable pattern. Do NOT use for regular file creation — use write_file instead.",
common_mistakes=[
    "Using create_skill for normal coding work — use write_file to create files",
    "Calling create_skill without a real reusable pattern to save",
    "Missing required fields: name, type, description, content",
    "Retrying after UUID/validation error — the error means the skill system is not available for this task",
],
```

- [ ] **Step 2: Add examples to propose_goal**

```python
examples=[
    ToolExample(
        description="Propose a backend development goal",
        tool_call_json='{"title": "Python FastAPI Backend", "description": "Create REST API with weather data endpoints, caching, and CORS", "kind": "requirement", "priority": 1}',
        expected_result="Goal proposed for review: Python FastAPI Backend",
    ),
    ToolExample(
        description="Propose a testing goal",
        tool_call_json='{"title": "Test Coverage", "description": "Write pytest tests for backend and vitest tests for frontend", "kind": "requirement", "priority": 2}',
        expected_result="Goal proposed for review: Test Coverage",
    ),
],
```

- [ ] **Step 3: Run ruff check/format on both files**
- [ ] **Step 4: Commit**

```bash
git commit -m "feat: complete tool guidance for create_skill and propose_goal (M4)"
```

---

## Task 5: Per-Tool Error Counter (M5)

**Files:**
- Modify: `workers/codeforge/agent_loop.py`
- Test: `workers/tests/test_agent_loop.py` (extend)

- [ ] **Step 1: Create _ToolErrorTracker class**

```python
class _ToolErrorTracker:
    """Tracks per-tool error counts to prevent retry loops."""

    __slots__ = ("_counts", "_max_identical")

    def __init__(self, max_identical: int = 2) -> None:
        self._counts: dict[tuple[str, str], int] = {}  # (tool, error_sig) -> count
        self._max_identical = max_identical

    def record_error(self, tool_name: str, error: str) -> bool:
        """Record an error. Returns True if this is NON-RETRYABLE (exceeded max)."""
        sig = self._normalize_error(error)
        key = (tool_name, sig)
        self._counts[key] = self._counts.get(key, 0) + 1
        return self._counts[key] >= self._max_identical

    def get_block_message(self, tool_name: str) -> str:
        return (
            f"[NON-RETRYABLE] Tool '{tool_name}' has failed {self._max_identical} times "
            f"with the same error. Do NOT retry. Continue with your main task "
            f"using read_file, write_file, or bash."
        )

    @staticmethod
    def _normalize_error(error: str) -> str:
        """Strip variable parts (line numbers, paths, UUIDs) for comparison."""
        import re
        s = re.sub(r'\d+', 'N', error)
        s = re.sub(r'[0-9a-f]{8}-[0-9a-f]{4}', 'UUID', s)
        return s[:200]
```

- [ ] **Step 2: Integrate into _execute_tool_call()**

After tool execution, if result indicates error:
```python
if not result.success and error_tracker.record_error(tc.name, result.output):
    block_msg = error_tracker.get_block_message(tc.name)
    self._append_tool_result(tc, block_msg, messages, state)
    return  # Skip normal result append
```

- [ ] **Step 3: Write tests**
- [ ] **Step 4: Run ruff, verify**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: per-tool error counter with NON-RETRYABLE blocking (M5)"
```

---

## Task 6: Sampling Parameters for Local Models (M6)

**Files:**
- Modify: `workers/codeforge/consumer/_conversation.py`

- [ ] **Step 1: Add local model sampling params**

After model classification, if model is local (lm_studio/*, ollama/*):

```python
LOCAL_SAMPLING_PARAMS = {
    "temperature": 0.7,
    "top_p": 0.8,
    "extra_body": {
        "top_k": 20,
        "repetition_penalty": 1.05,
    },
}

# Apply to LLM client config for local models
if model_name.startswith(("lm_studio/", "ollama/")):
    llm_kwargs.update(LOCAL_SAMPLING_PARAMS)
```

- [ ] **Step 2: Verify LLM client passes extra_body through to LiteLLM**

Check `workers/codeforge/llm.py` — ensure extra_body is forwarded in chat_completion().

- [ ] **Step 3: Commit**

```bash
git commit -m "feat: optimized sampling parameters for local models (M6)"
```

---

## Task Summary

| Task | Measure | Files | Est. Steps |
|---|---|---|---|
| 1 | M1: Tool filtering | capability.py, agent_loop.py | 6 |
| 2 | M2: Step-by-step prompt | step_by_step.yaml, _conversation.py | 4 |
| 3 | M3: Context limits | _conversation.py | 3 |
| 4 | M4: Tool guidance | create_skill.py, propose_goal.py | 4 |
| 5 | M5: Error counter | agent_loop.py | 5 |
| 6 | M6: Sampling params | _conversation.py, llm.py | 3 |
| **Total** | | | **25 steps** |
