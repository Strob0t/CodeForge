"""Tests for ClaudeCodeExecutor."""

from __future__ import annotations

import sys
import types
from unittest.mock import AsyncMock, patch

import pytest

from codeforge.claude_code_executor import ClaudeCodeExecutor
from codeforge.models import ToolCallDecision


def _make_executor() -> ClaudeCodeExecutor:
    return ClaudeCodeExecutor(workspace_path="/tmp", runtime=AsyncMock())


class TestFormatMessagesAsPrompt:
    def test_single_user_message(self) -> None:
        result = _make_executor()._format_messages_as_prompt(
            [{"role": "user", "content": "Fix the bug"}],
        )
        assert result == "Fix the bug"

    def test_system_messages_excluded(self) -> None:
        result = _make_executor()._format_messages_as_prompt(
            [
                {"role": "system", "content": "You are helpful"},
                {"role": "user", "content": "Hello"},
            ]
        )
        assert result == "Hello"
        assert "system" not in result.lower()

    def test_multi_turn_preserves_history(self) -> None:
        result = _make_executor()._format_messages_as_prompt(
            [
                {"role": "user", "content": "Write a function"},
                {"role": "assistant", "content": "def foo(): pass"},
                {"role": "user", "content": "Now add tests"},
            ]
        )
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
        with patch("codeforge.claude_code_executor.resolve_cost", return_value=0.05) as mock_rc:
            cost = _make_executor()._estimate_equivalent_cost(1000, 500)
            mock_rc.assert_called_once_with(0.0, "anthropic/claude-sonnet-4", 1000, 500)
            assert cost == 0.05


# ---------------------------------------------------------------------------
# Helpers for mocking the claude_code_sdk module (not installed)
# ---------------------------------------------------------------------------


class _FakePermissionResultAllow:
    """Stub for ``claude_code_sdk.types.PermissionResultAllow``."""


class _FakePermissionResultDeny:
    """Stub for ``claude_code_sdk.types.PermissionResultDeny``."""

    def __init__(self, *, message: str = "") -> None:
        self.message = message


@pytest.fixture(autouse=False)
def _mock_claude_code_sdk(monkeypatch: pytest.MonkeyPatch):
    """Inject a fake ``claude_code_sdk`` package into ``sys.modules``.

    This allows the inner import inside ``_policy_callback`` to succeed
    even though the real SDK is not installed.
    """
    sdk_types = types.ModuleType("claude_code_sdk.types")
    sdk_types.PermissionResultAllow = _FakePermissionResultAllow  # type: ignore[attr-defined]
    sdk_types.PermissionResultDeny = _FakePermissionResultDeny  # type: ignore[attr-defined]

    sdk = types.ModuleType("claude_code_sdk")
    sdk.types = sdk_types  # type: ignore[attr-defined]

    monkeypatch.setitem(sys.modules, "claude_code_sdk", sdk)
    monkeypatch.setitem(sys.modules, "claude_code_sdk.types", sdk_types)


# ---------------------------------------------------------------------------
# Policy callback tests
# ---------------------------------------------------------------------------


class TestPolicyCallback:
    """Tests for ``ClaudeCodeExecutor._make_policy_callback``."""

    @pytest.mark.asyncio
    @pytest.mark.usefixtures("_mock_claude_code_sdk")
    async def test_allow_maps_read_to_file_read(self) -> None:
        runtime = AsyncMock()
        runtime.request_tool_call.return_value = ToolCallDecision(
            call_id="c1",
            decision="allow",
        )

        executor = ClaudeCodeExecutor(workspace_path="/tmp", runtime=runtime)
        callback = executor._make_policy_callback()
        result = await callback("Read", {"file_path": "/tmp/foo.py"})

        runtime.request_tool_call.assert_awaited_once_with(
            tool="file:read",
            command="",
            path="/tmp/foo.py",
        )
        assert isinstance(result, _FakePermissionResultAllow)

    @pytest.mark.asyncio
    @pytest.mark.usefixtures("_mock_claude_code_sdk")
    async def test_deny_maps_bash_to_command_execute(self) -> None:
        runtime = AsyncMock()
        runtime.request_tool_call.return_value = ToolCallDecision(
            call_id="c2",
            decision="deny",
            reason="blocked",
        )

        executor = ClaudeCodeExecutor(workspace_path="/tmp", runtime=runtime)
        callback = executor._make_policy_callback()
        result = await callback("Bash", {"command": "rm -rf /"})

        runtime.request_tool_call.assert_awaited_once_with(
            tool="command:execute",
            command="rm -rf /",
            path="",
        )
        assert isinstance(result, _FakePermissionResultDeny)
        assert result.message == "blocked"

    @pytest.mark.asyncio
    @pytest.mark.usefixtures("_mock_claude_code_sdk")
    async def test_unknown_tool_gets_claude_code_prefix(self) -> None:
        runtime = AsyncMock()
        runtime.request_tool_call.return_value = ToolCallDecision(
            call_id="c3",
            decision="allow",
        )

        executor = ClaudeCodeExecutor(workspace_path="/tmp", runtime=runtime)
        callback = executor._make_policy_callback()
        result = await callback("SomeNewTool", {"arg": "val"})

        runtime.request_tool_call.assert_awaited_once_with(
            tool="claude-code:SomeNewTool",
            command="",
            path="",
        )
        assert isinstance(result, _FakePermissionResultAllow)
