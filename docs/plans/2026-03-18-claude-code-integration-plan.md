# Claude Code Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate Claude Code as a routing target in CodeForge's Hybrid Routing so subscription users can leverage Claude via the existing routing cascade with policy enforcement and fallback.

**Architecture:** Claude Code appears as `claudecode/default` in the routing model pool. When selected, `_execute_conversation_run()` branches to a new `ClaudeCodeExecutor` that uses the `claude-code-sdk` Python package with a `can_use_tool` callback for policy enforcement. Falls back to LiteLLM models if unavailable.

**Tech Stack:** Python 3.12, claude-code-sdk, pytest, pytest-asyncio

**Spec:** `docs/specs/2026-03-18-claude-code-integration-design.md`

**Errata (from plan review):**
- Policy decisions use `"allow"` / `"deny"`, NOT `"approve"` (matches `ToolCallDecision` model)
- `tool_messages` must use `ConversationMessagePayload` Pydantic models, not raw dicts
- `RuntimeClient` has no `send_ag_ui_event()` — use `publish_trajectory_event()` instead

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `workers/codeforge/claude_code_availability.py` | **Create** | CLI probe with async Lock + TTL cache + enabled gate |
| `workers/codeforge/claude_code_executor.py` | **Create** | ClaudeCodeExecutor: SDK query, message handling, policy callback, cost tracking |
| `workers/codeforge/consumer/_conversation.py` | **Modify** | Execution branch for `claudecode/` prefix + availability in model list |
| `workers/codeforge/routing/router.py` | **Modify** | Add `claudecode/default` to COMPLEXITY_DEFAULTS |
| `pyproject.toml` | **Modify** | Add `claude-code-sdk` as optional dependency |
| `workers/tests/test_claude_code_availability.py` | **Create** | Unit tests for availability detection |
| `workers/tests/test_claude_code_executor.py` | **Create** | Unit tests for executor, policy callback, message handling |
| `workers/tests/test_claude_code_routing.py` | **Create** | Integration tests for routing + fallback |

---

### Task 1: Availability Detection

**Files:**
- Create: `workers/codeforge/claude_code_availability.py`
- Test: `workers/tests/test_claude_code_availability.py`

- [ ] **Step 1: Write failing tests for availability detection**

Create `workers/tests/test_claude_code_availability.py`:

```python
"""Tests for Claude Code CLI availability detection."""

from __future__ import annotations

import asyncio
from unittest.mock import AsyncMock, patch

import pytest


@pytest.fixture(autouse=True)
def _reset_cache() -> None:
    """Reset module-level cache between tests."""
    from codeforge import claude_code_availability as mod

    mod._claude_code_available = None
    mod._claude_code_check_time = 0.0


class TestIsClaudeCodeAvailable:
    @pytest.mark.asyncio
    async def test_disabled_returns_false(self) -> None:
        from codeforge.claude_code_availability import is_claude_code_available

        with patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_ENABLED": "false"}):
            assert await is_claude_code_available() is False

    @pytest.mark.asyncio
    async def test_missing_env_returns_false(self) -> None:
        from codeforge.claude_code_availability import is_claude_code_available

        with patch.dict("os.environ", {}, clear=True):
            assert await is_claude_code_available() is False

    @pytest.mark.asyncio
    async def test_enabled_cli_found(self) -> None:
        from codeforge.claude_code_availability import is_claude_code_available

        mock_proc = AsyncMock()
        mock_proc.returncode = 0
        mock_proc.wait = AsyncMock()

        with (
            patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_ENABLED": "true"}),
            patch("asyncio.create_subprocess_exec", return_value=mock_proc),
        ):
            assert await is_claude_code_available() is True

    @pytest.mark.asyncio
    async def test_enabled_cli_not_found(self) -> None:
        from codeforge.claude_code_availability import is_claude_code_available

        with (
            patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_ENABLED": "true"}),
            patch("asyncio.create_subprocess_exec", side_effect=OSError("not found")),
        ):
            assert await is_claude_code_available() is False

    @pytest.mark.asyncio
    async def test_caches_result(self) -> None:
        from codeforge.claude_code_availability import is_claude_code_available

        mock_proc = AsyncMock()
        mock_proc.returncode = 0
        mock_proc.wait = AsyncMock()

        with (
            patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_ENABLED": "true"}),
            patch("asyncio.create_subprocess_exec", return_value=mock_proc) as mock_exec,
        ):
            await is_claude_code_available()
            await is_claude_code_available()
            assert mock_exec.call_count == 1

    @pytest.mark.asyncio
    async def test_cache_expires(self) -> None:
        import time

        from codeforge import claude_code_availability as mod
        from codeforge.claude_code_availability import is_claude_code_available

        mock_proc = AsyncMock()
        mock_proc.returncode = 0
        mock_proc.wait = AsyncMock()

        with (
            patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_ENABLED": "true"}),
            patch("asyncio.create_subprocess_exec", return_value=mock_proc) as mock_exec,
        ):
            await is_claude_code_available()
            mod._claude_code_check_time = time.monotonic() - 400.0
            await is_claude_code_available()
            assert mock_exec.call_count == 2

    @pytest.mark.asyncio
    async def test_timeout_returns_false(self) -> None:
        from codeforge.claude_code_availability import is_claude_code_available

        with (
            patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_ENABLED": "true"}),
            patch("asyncio.create_subprocess_exec", side_effect=TimeoutError()),
        ):
            assert await is_claude_code_available() is False
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/test_claude_code_availability.py -v`
Expected: FAIL with `ModuleNotFoundError`

- [ ] **Step 3: Implement availability detection**

Create `workers/codeforge/claude_code_availability.py`:

```python
"""Claude Code CLI availability detection with caching."""

from __future__ import annotations

import asyncio
import os
import time

_cache_lock = asyncio.Lock()
_claude_code_available: bool | None = None
_claude_code_check_time: float = 0.0
_CACHE_TTL = 300.0


async def is_claude_code_available() -> bool:
    """Check if Claude Code CLI is installed and the feature is enabled.

    Cached for 5 minutes behind asyncio.Lock.
    Returns False immediately if CODEFORGE_CLAUDECODE_ENABLED != 'true'.
    """
    global _claude_code_available, _claude_code_check_time  # noqa: PLW0603

    if os.environ.get("CODEFORGE_CLAUDECODE_ENABLED", "false").lower() != "true":
        return False

    async with _cache_lock:
        now = time.monotonic()
        if _claude_code_available is not None and (now - _claude_code_check_time) < _CACHE_TTL:
            return _claude_code_available

        cli_path = os.environ.get("CODEFORGE_CLAUDECODE_PATH", "claude")
        try:
            proc = await asyncio.create_subprocess_exec(
                cli_path,
                "--version",
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
            )
            await asyncio.wait_for(proc.wait(), timeout=5.0)
            _claude_code_available = proc.returncode == 0
        except (OSError, TimeoutError):
            _claude_code_available = False

        _claude_code_check_time = now
        return _claude_code_available
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/test_claude_code_availability.py -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add workers/codeforge/claude_code_availability.py workers/tests/test_claude_code_availability.py
git commit -m "feat(routing): add Claude Code CLI availability detection with caching"
```

---

### Task 2: ClaudeCodeExecutor Core

**Files:**
- Create: `workers/codeforge/claude_code_executor.py`
- Test: `workers/tests/test_claude_code_executor.py`

- [ ] **Step 1: Write failing tests for message formatting and cost estimation**

Create `workers/tests/test_claude_code_executor.py`:

```python
"""Tests for ClaudeCodeExecutor."""

from __future__ import annotations

from unittest.mock import AsyncMock

import pytest

from codeforge.claude_code_executor import ClaudeCodeExecutor


def _make_executor() -> ClaudeCodeExecutor:
    return ClaudeCodeExecutor(workspace_path="/tmp", runtime=AsyncMock())


class TestFormatMessagesAsPrompt:
    def test_single_user_message(self) -> None:
        result = _make_executor()._format_messages_as_prompt(
            [{"role": "user", "content": "Fix the bug"}],
        )
        assert result == "Fix the bug"

    def test_system_messages_excluded(self) -> None:
        result = _make_executor()._format_messages_as_prompt([
            {"role": "system", "content": "You are helpful"},
            {"role": "user", "content": "Hello"},
        ])
        assert result == "Hello"
        assert "system" not in result.lower()

    def test_multi_turn_preserves_history(self) -> None:
        result = _make_executor()._format_messages_as_prompt([
            {"role": "user", "content": "Write a function"},
            {"role": "assistant", "content": "def foo(): pass"},
            {"role": "user", "content": "Now add tests"},
        ])
        assert "<conversation_history>" in result
        assert "[USER]: Write a function" in result
        assert "[ASSISTANT]: def foo(): pass" in result
        assert "Now add tests" in result
        assert "[USER]: Now add tests" not in result

    def test_empty_messages(self) -> None:
        assert _make_executor()._format_messages_as_prompt([]) == ""

    def test_only_system_messages(self) -> None:
        result = _make_executor()._format_messages_as_prompt(
            [{"role": "system", "content": "system only"}],
        )
        assert result == ""


class TestEstimateEquivalentCost:
    def test_returns_float(self) -> None:
        cost = _make_executor()._estimate_equivalent_cost(1000, 500)
        assert isinstance(cost, float)
        assert cost >= 0.0

    def test_zero_tokens_returns_zero(self) -> None:
        assert _make_executor()._estimate_equivalent_cost(0, 0) == 0.0

    def test_calls_resolve_cost_with_correct_args(self) -> None:
        from unittest.mock import patch as mock_patch

        with mock_patch("codeforge.claude_code_executor.resolve_cost", return_value=0.05) as mock_rc:
            cost = _make_executor()._estimate_equivalent_cost(1000, 500)
            mock_rc.assert_called_once_with(0.0, "anthropic/claude-sonnet-4", 1000, 500)
            assert cost == 0.05
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/test_claude_code_executor.py -v`
Expected: FAIL with `ModuleNotFoundError`

- [ ] **Step 3: Implement the full ClaudeCodeExecutor**

Create `workers/codeforge/claude_code_executor.py` with the complete implementation from the spec (message formatting, cost estimation, SDK run, CLI fallback, policy callback, SDK message handling, cancellation). See spec Part 3 for full code.

Key elements:
- `_format_messages_as_prompt()`: convert chat messages to single prompt
- `_estimate_equivalent_cost()`: calls `resolve_cost(0.0, "anthropic/claude-sonnet-4", tokens_in, tokens_out)`
- `run()`: guarded by `asyncio.Semaphore(_MAX_CONCURRENT)`, dispatches to SDK or CLI
- `_run_via_sdk()`: uses `claude_code_sdk.query()` with `can_use_tool` callback
- `_run_via_cli()`: subprocess with `--output-format stream-json`
- `_make_policy_callback()`: maps tool names to policy categories, calls `runtime.request_tool_call()`, checks `decision.decision == "allow"` (NOT "approve")
- `_handle_sdk_message()`: processes `AssistantMessage` and `ResultMessage`, builds `ConversationMessagePayload` objects (NOT raw dicts) for `tool_messages`
- Uses `runtime.publish_trajectory_event()` for structured events and `runtime.send_output()` for text streaming (NOT `send_ag_ui_event` which does not exist)
- `cancel()`: sets flag + terminates subprocess

**Critical implementation notes (from plan review):**

1. **Policy decision value is `"allow"`**, not `"approve"`:
   ```python
   if decision.decision == "allow":
       return PermissionResultAllow()
   ```

2. **Tool messages must use Pydantic models**:
   ```python
   from codeforge.models import ConversationMessagePayload, ConversationToolCallPayload, ConversationToolCallFunction

   # ToolUseBlock -> assistant message with tool_calls
   tool_messages.append(ConversationMessagePayload(
       role="assistant",
       tool_calls=[ConversationToolCallPayload(
           id=block.id,
           function=ConversationToolCallFunction(
               name=block.name,
               arguments=json.dumps(block.input) if block.input else "{}",
           ),
       )],
   ))

   # ToolResultBlock -> tool message
   tool_messages.append(ConversationMessagePayload(
       role="tool",
       tool_call_id=block.tool_use_id,
       content=content,
   ))
   ```

3. **Structured events use `publish_trajectory_event()`**:
   ```python
   await self._runtime.publish_trajectory_event({
       "event_type": "agent.tool_called",
       "tool": block.name,
       "tool_call_id": block.id,
   })
   ```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/test_claude_code_executor.py -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add workers/codeforge/claude_code_executor.py workers/tests/test_claude_code_executor.py
git commit -m "feat(routing): add ClaudeCodeExecutor with SDK/CLI, policy callback, cost tracking"
```

---

### Task 3: Policy Callback Tests

**Files:**
- Modify: `workers/tests/test_claude_code_executor.py`

- [ ] **Step 1: Add policy callback tests**

Append to `workers/tests/test_claude_code_executor.py`:

```python
from unittest.mock import patch

from codeforge.models import ToolCallDecision


class TestPolicyCallback:
    @pytest.mark.asyncio
    async def test_approve_maps_read_to_file_read(self) -> None:
        runtime = AsyncMock()
        runtime.request_tool_call = AsyncMock(
            return_value=ToolCallDecision(call_id="c1", decision="allow", reason=""),
        )
        executor = ClaudeCodeExecutor(workspace_path="/tmp", runtime=runtime)
        callback = executor._make_policy_callback()

        with patch("claude_code_sdk.PermissionResultAllow", create=True):
            await callback("Read", {"file_path": "/tmp/foo.py"})

        runtime.request_tool_call.assert_awaited_once_with(
            tool="file:read", command="", path="/tmp/foo.py",
        )

    @pytest.mark.asyncio
    async def test_deny_maps_bash_to_command_execute(self) -> None:
        runtime = AsyncMock()
        runtime.request_tool_call = AsyncMock(
            return_value=ToolCallDecision(call_id="c2", decision="deny", reason="blocked"),
        )
        executor = ClaudeCodeExecutor(workspace_path="/tmp", runtime=runtime)
        callback = executor._make_policy_callback()

        with patch("claude_code_sdk.PermissionResultDeny", create=True):
            await callback("Bash", {"command": "rm -rf /"})

        runtime.request_tool_call.assert_awaited_once_with(
            tool="command:execute", command="rm -rf /", path="",
        )

    @pytest.mark.asyncio
    async def test_unknown_tool_gets_claudecode_prefix(self) -> None:
        runtime = AsyncMock()
        runtime.request_tool_call = AsyncMock(
            return_value=ToolCallDecision(call_id="c3", decision="allow", reason=""),
        )
        executor = ClaudeCodeExecutor(workspace_path="/tmp", runtime=runtime)
        callback = executor._make_policy_callback()

        with patch("claude_code_sdk.PermissionResultAllow", create=True):
            await callback("SomeNewTool", {"arg": "val"})

        runtime.request_tool_call.assert_awaited_once_with(
            tool="claudecode:SomeNewTool", command="", path="",
        )
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/test_claude_code_executor.py::TestPolicyCallback -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add workers/tests/test_claude_code_executor.py
git commit -m "test(routing): add policy callback tests for ClaudeCodeExecutor"
```

---

### Task 4: Routing Defaults

**Files:**
- Modify: `workers/codeforge/routing/router.py:44-55`
- Test: `workers/tests/test_claude_code_routing.py`

- [ ] **Step 1: Write failing routing tests**

Create `workers/tests/test_claude_code_routing.py`:

```python
"""Tests for Claude Code routing integration."""

from __future__ import annotations

from codeforge.routing.complexity import ComplexityAnalyzer
from codeforge.routing.models import ComplexityTier, RoutingConfig
from codeforge.routing.router import COMPLEXITY_DEFAULTS, HybridRouter


class TestClaudeCodeInDefaults:
    def test_in_complex_tier(self) -> None:
        assert "claudecode/default" in COMPLEXITY_DEFAULTS[ComplexityTier.COMPLEX]

    def test_in_reasoning_tier(self) -> None:
        assert "claudecode/default" in COMPLEXITY_DEFAULTS[ComplexityTier.REASONING]

    def test_not_in_simple(self) -> None:
        assert "claudecode/default" not in COMPLEXITY_DEFAULTS[ComplexityTier.SIMPLE]

    def test_not_in_medium(self) -> None:
        assert "claudecode/default" not in COMPLEXITY_DEFAULTS[ComplexityTier.MEDIUM]

    def test_first_in_complex(self) -> None:
        assert COMPLEXITY_DEFAULTS[ComplexityTier.COMPLEX][0] == "claudecode/default"

    def test_first_in_reasoning(self) -> None:
        assert COMPLEXITY_DEFAULTS[ComplexityTier.REASONING][0] == "claudecode/default"


class TestClaudeCodeRoutingSelection:
    def test_selects_claudecode_for_complex_when_available(self) -> None:
        router = HybridRouter(
            complexity=ComplexityAnalyzer(),
            mab=None,
            meta=None,
            available_models=["claudecode/default", "openai/gpt-4o"],
            config=RoutingConfig(enabled=True),
        )
        decision = router.route(
            "Analyze the microservice architecture, review API design patterns, "
            "refactor the database layer, and implement comprehensive integration tests "
            "with error handling for all edge cases."
        )
        assert decision is not None
        assert decision.model == "claudecode/default"

    def test_skips_claudecode_when_not_available(self) -> None:
        router = HybridRouter(
            complexity=ComplexityAnalyzer(),
            mab=None,
            meta=None,
            available_models=["openai/gpt-4o", "anthropic/claude-sonnet-4"],
            config=RoutingConfig(enabled=True),
        )
        decision = router.route(
            "Analyze the microservice architecture, review API design patterns, "
            "refactor the database layer, and implement comprehensive integration tests."
        )
        assert decision is not None
        assert decision.model != "claudecode/default"

    def test_fallback_chain_has_litellm_after_claudecode(self) -> None:
        router = HybridRouter(
            complexity=ComplexityAnalyzer(),
            mab=None,
            meta=None,
            available_models=["claudecode/default", "openai/gpt-4o", "anthropic/claude-sonnet-4"],
            config=RoutingConfig(enabled=True),
        )
        plan = router.route_with_fallbacks(
            "Analyze microservice architecture and refactor the database layer "
            "with comprehensive testing and error handling.",
        )
        assert plan.primary is not None
        assert plan.primary.model == "claudecode/default"
        assert len(plan.fallbacks) > 0
        assert all(not f.startswith("claudecode/") for f in plan.fallbacks)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/test_claude_code_routing.py -v`
Expected: FAIL on `test_in_complex_tier`

- [ ] **Step 3: Add `claudecode/default` to COMPLEXITY_DEFAULTS**

Edit `workers/codeforge/routing/router.py`. Insert `"claudecode/default"` as first entry in COMPLEX and REASONING lists:

```
Line 44: ComplexityTier.COMPLEX: [
  ADD ->     "claudecode/default",
Line 45:     "github_copilot/gpt-4o",
...
Line 50: ComplexityTier.REASONING: [
  ADD ->     "claudecode/default",
Line 51:     "github_copilot/o3-mini",
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/test_claude_code_routing.py -v`
Expected: ALL PASS

- [ ] **Step 5: Run existing routing tests for regression**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/test_routing_integration.py workers/tests/test_routing_complexity.py -v`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add workers/codeforge/routing/router.py workers/tests/test_claude_code_routing.py
git commit -m "feat(routing): add claudecode/default to COMPLEX and REASONING tiers"
```

---

### Task 5: Conversation Handler Integration

**Files:**
- Modify: `workers/codeforge/consumer/_conversation.py:234-290`
- Modify: `workers/codeforge/consumer/_conversation.py` (`_get_available_models`)

- [ ] **Step 1: Extract existing agentic code into `_execute_litellm_loop`**

In `_conversation.py:234-290`, extract the body of the agentic branch (lines 255-290) into a new method `_execute_litellm_loop()`. Keep `_execute_conversation_run()` calling it directly. This is a pure refactoring step.

- [ ] **Step 2: Run existing tests to confirm no regression**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/test_consumer.py workers/tests/test_consumer_dispatch.py -v`
Expected: ALL PASS

- [ ] **Step 3: Add Claude Code branch to `_execute_conversation_run`**

Before the `_execute_litellm_loop()` call, add:

```python
if primary_model.startswith("claudecode/"):
    from codeforge.claude_code_executor import ClaudeCodeExecutor

    executor = ClaudeCodeExecutor(
        workspace_path=run_msg.workspace_path,
        runtime=runtime,
    )
    result = await executor.run(
        messages=messages,
        model=primary_model,
        max_turns=run_msg.termination.max_steps or 50,
        system_prompt=run_msg.system_prompt,
    )
    if result.error and fallback_models:
        next_model = fallback_models[0]
        remaining = fallback_models[1:]
        await runtime.send_output(
            f"\n[Claude Code unavailable. Switching to {next_model}]\n"
        )
        return await self._execute_litellm_loop(
            run_msg, messages, next_model, routing, runtime, registry, remaining,
        )
    return result
```

- [ ] **Step 4: Extend `_get_available_models` with Claude Code probe**

At the end of `_get_available_models()`, before the final return, add:

```python
from codeforge.claude_code_availability import is_claude_code_available
if await is_claude_code_available():
    models.append("claudecode/default")
```

- [ ] **Step 5: Add integration test for fallback behavior**

Append to `workers/tests/test_claude_code_routing.py`:

```python
from unittest.mock import AsyncMock, patch

from codeforge.models import AgentLoopResult


class TestClaudeCodeFallback:
    @pytest.mark.asyncio
    async def test_claudecode_error_triggers_fallback(self) -> None:
        """When ClaudeCodeExecutor returns an error, the handler falls back to LiteLLM."""
        from codeforge.claude_code_executor import ClaudeCodeExecutor

        error_result = AgentLoopResult(error="CLI not found", model="claudecode/default")

        with patch.object(ClaudeCodeExecutor, "run", return_value=error_result):
            # Verify the branch logic: error + fallback_models -> switch model
            executor = ClaudeCodeExecutor(workspace_path="/tmp", runtime=AsyncMock())
            result = await executor.run(messages=[], model="claudecode/default")
            assert result.error == "CLI not found"
```

- [ ] **Step 6: Run all tests**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/ -v --timeout=60 -x`
Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add workers/codeforge/consumer/_conversation.py workers/tests/test_claude_code_routing.py
git commit -m "feat(routing): add Claude Code execution branch and model availability"
```

---

### Task 6: Optional Dependency + Documentation

**Files:**
- Modify: `pyproject.toml`
- Modify: `docs/todo.md`

- [ ] **Step 1: Add `claude-code-sdk` as optional dependency in pyproject.toml**

Add under `[tool.poetry.dependencies]`:
```toml
claude-code-sdk = {version = "^0.1", optional = true}
```

Extend `[tool.poetry.extras]`:
```toml
claudecode = ["claude-code-sdk"]
```

- [ ] **Step 2: Run pre-commit hooks**

Run: `cd /workspaces/CodeForge && pre-commit run --all-files`
Expected: ALL PASS

- [ ] **Step 3: Update docs/todo.md**

Add under the appropriate section:

```markdown
- [x] Claude Code as routing target with execution branch (2026-03-18)
- [ ] Claude Code: E2E manual test with live CLI
- [ ] Claude Code: model selection override (claudecode/claude-sonnet-4)
- [ ] Claude Code: read CODEFORGE_CLAUDECODE_MAX_TURNS, _TIMEOUT, _TIERS from env
- [ ] Claude Code: update docs/dev-setup.md with new env vars
```

- [ ] **Step 4: Final commit and push**

```bash
git add pyproject.toml docs/todo.md
git commit -m "build: add claude-code-sdk optional dep, update todo"
git push
```
