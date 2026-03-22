# Weak Model Adaptation — Design Spec

**Date:** 2026-03-22
**Type:** Design specification for making CodeForge work reliably with weak/local LLMs (7B-30B)
**Scope:** 6 measures to improve agent quality for pure_completion and api_with_tools capability levels
**Context:** 3 test runs with qwen3-30b showed tool confusion, error loops, framework hallucination, and context overload

---

## 1. Problem Statement

CodeForge treats all LLMs equally in the agent loop — same tools, same context limits, same prompts. Small local models (qwen3-30b, llama, etc.) fail because:

1. **Too many tools** (13+) — model calls irrelevant tools (create_skill, handoff)
2. **No step-by-step enforcement** — model tries to do everything at once, drifts
3. **Context too large** (120K) — model loses focus, ignores system prompt
4. **Missing tool guidance** — create_skill has zero examples/mistakes/when-to-use
5. **No error loop breaking** — model retries same failing tool call indefinitely
6. **Wrong sampling parameters** — LM Studio defaults degrade Qwen3 output

---

## 2. Measures

### M1: Tool Filtering by Capability Level

**What:** Before sending tools to the LLM, filter based on capability level AND mode. Pure_completion models get a minimal tool set.

**Tool sets by capability:**

| Tool | full | api_with_tools | pure_completion |
|---|---|---|---|
| read_file | Y | Y | Y |
| write_file | Y | Y | Y |
| edit_file | Y | Y | N (use write_file for whole-file replacement) |
| bash | Y | Y | Y |
| search_files | Y | Y | Y |
| glob_files | Y | Y | N (use bash find) |
| list_directory | Y | Y | N (use bash ls) |
| propose_goal | Y | Y | Y (if mode declares it) |
| create_skill | Y | N | N |
| search_skills | Y | N | N |
| search_conversations | Y | N | N |
| handoff | Y | Y | N |
| transition_to_act | Y | Y | Y (if plan_act enabled) |

**Implementation:** Filter `tools_array` in `agent_loop.py` before passing to LLM, based on `capability_level` + `mode.tools` + `mode.denied_tools`.

**Files:**
- `workers/codeforge/agent_loop.py` — add `_filter_tools_for_capability()`
- `workers/codeforge/tools/capability.py` — add `TOOLS_BY_CAPABILITY` mapping

---

### M2: Step-by-Step Pipeline for Weak Models

**What:** For pure_completion and api_with_tools, enforce a structured workflow via system prompt:
1. PLAN: List all files needed (1 turn)
2. SKELETON: Create directory structure + empty files (1 turn)
3. IMPLEMENT: One file at a time, complete content (N turns)
4. TEST: Run tests after each file (N turns)
5. INTEGRATE: Verify everything works together (1 turn)

**Key rules injected into system prompt:**
- "Make exactly ONE tool call per turn. Wait for the result."
- "Before each tool call, state what you will do and why in 1-2 sentences."
- "Write COMPLETE file contents — never use placeholders or '...'."
- "After writing a file, verify it with bash (syntax check, import check)."

**Implementation:** New adaptive prompt file `step_by_step.yaml` injected for weak models.

**Files:**
- `internal/service/prompts/model_adaptive/step_by_step.yaml` — new prompt
- `workers/codeforge/consumer/_conversation.py` — inject for weak models

---

### M3: Context Limits by Capability Level

**What:** Override MaxContextTokens based on capability level:

| Capability | MaxContextTokens | SummarizeThreshold |
|---|---|---|
| full | 120,000 (default) | 80% of window |
| api_with_tools | 32,000 | 70% of window |
| pure_completion | 16,000 | 60% of window |

**Implementation:** In the Python worker, after capability classification, override the context limits from LoopConfig before starting the agent loop.

**Files:**
- `workers/codeforge/consumer/_conversation.py` — adjust limits after classify_model()

---

### M4: Complete Tool Guidance

**What:** Fill in missing tool metadata for all tools, especially those that caused problems.

**create_skill — currently has NO guidance at all:**
```python
when_to_use = "Use ONLY when you've discovered a genuinely reusable pattern during this task. Do NOT use for regular coding — use write_file instead."
common_mistakes = [
    "Using create_skill for normal file creation — use write_file instead",
    "Calling create_skill without a real reusable pattern to save",
    "Missing required fields: name, type, description, content",
]
```

**search_skills:**
```python
when_to_use = "Search for existing reusable patterns before implementing common functionality."
common_mistakes = [
    "Searching too broadly — use specific keywords",
    "Not checking results — 'No skills found' means try different query or implement yourself",
]
```

**propose_goal — add examples:**
```python
examples = [
    ToolExample(
        description="Propose a backend development goal",
        tool_call_json='{"title": "Python FastAPI Backend", "description": "Create REST API with weather endpoints", "kind": "requirement", "priority": 1}',
        expected_result="Goal proposed for review: Python FastAPI Backend",
    ),
]
```

**Files:**
- `workers/codeforge/tools/create_skill.py` — add when_to_use, common_mistakes
- `workers/codeforge/tools/search_skills.py` — add when_to_use, common_mistakes
- `workers/codeforge/tools/propose_goal.py` — add examples

---

### M5: Per-Tool Error Counter with NON-RETRYABLE

**What:** Track `(tool_name, error_signature)` tuples. After 2 identical failures for the same tool+error, inject a NON-RETRYABLE message and suggest an alternative.

**Error message format:**
```
[NON-RETRYABLE] Tool 'create_skill' has failed 2 times with the same error.
Do NOT retry this tool with the same arguments.
Instead, continue with your main task using write_file, read_file, or bash.
```

**Implementation:** Add `_ToolErrorTracker` class to agent_loop.py. Check before each tool execution.

**Files:**
- `workers/codeforge/agent_loop.py` — add `_ToolErrorTracker`, integrate into `_execute_tool_call()`

---

### M6: Sampling Parameters for Local Models

**What:** Pass optimal sampling parameters for Qwen3 via LiteLLM extra_body when the model is classified as local.

**Parameters:**
```python
# For lm_studio/* and ollama/* models:
extra_body = {
    "temperature": 0.7,
    "top_p": 0.8,
    "top_k": 20,
    "repetition_penalty": 1.05,
}
```

**Implementation:** In `_conversation.py`, when model matches local patterns, add extra params to LLM client config.

**Files:**
- `workers/codeforge/consumer/_conversation.py` — add sampling params for local models
- `workers/codeforge/llm.py` — pass extra_body through to LiteLLM

---

## 3. Expected Impact

| Before (Run 1-3) | After |
|---|---|
| Agent calls 13+ tools, confuses create_skill with write_file | Agent sees 5 core tools, no confusion |
| Agent tries everything at once, drifts | Forced step-by-step: plan, skeleton, implement, test |
| 120K context, model loses focus | 16K context, focused and on-task |
| create_skill has no guidance, causes error loops | Full guidance: when to use, when NOT to use, examples |
| Same tool fails 3+ times in a row | Blocked after 2 identical failures, alternative suggested |
| Default sampling causes repetition | Optimized temperature/top_p/repetition_penalty |

---

## 4. Success Criteria

A Run 4 with qwen3-30b on Mode A (Weather Dashboard) should achieve:
- No calls to create_skill or handoff (filtered out)
- Agent follows plan→skeleton→implement→test workflow
- No error loops (max 2 retries per tool+error)
- >= 60% functional checks passing (improvement from Run 1's 57%)
- Backend AND frontend code generated with correct framework (SolidJS, not React)
