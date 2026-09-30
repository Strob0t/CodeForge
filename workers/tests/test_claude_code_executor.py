"""Tests for ClaudeCodeExecutor."""

from __future__ import annotations

import os
import sys
import types
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, patch

import pytest

from codeforge.claude_code_executor import ClaudeCodeExecutor
from codeforge.config import get_settings
from codeforge.models import ToolCallDecision
from codeforge.runtime import arguments_preview

if TYPE_CHECKING:
    from collections.abc import Iterator


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

    # The callback sends Claude Code's own tool names; the Go policy layer maps
    # them to canonical names (Read, Edit, Bash, ...), so a Read request must
    # no longer be sent as a "file:read" category that no preset rule matches.
    @pytest.mark.asyncio
    @pytest.mark.usefixtures("_mock_claude_code_sdk")
    async def test_allow_sends_read_with_file_path(self) -> None:
        runtime = AsyncMock()
        runtime.request_tool_call.return_value = ToolCallDecision(
            call_id="c1",
            decision="allow",
        )

        executor = ClaudeCodeExecutor(workspace_path="/tmp", runtime=runtime)
        callback = executor._make_policy_callback()
        result = await callback("Read", {"file_path": "/tmp/foo.py"})

        runtime.request_tool_call.assert_awaited_once_with(
            tool="Read",
            command="",
            path="/tmp/foo.py",
            arguments_preview='{"file_path": "/tmp/foo.py"}',
        )
        assert isinstance(result, _FakePermissionResultAllow)

    @pytest.mark.asyncio
    @pytest.mark.usefixtures("_mock_claude_code_sdk")
    async def test_deny_sends_bash_command(self) -> None:
        runtime = AsyncMock()
        runtime.request_tool_call.return_value = ToolCallDecision(
            call_id="c2",
            decision="deny",
            reason="blocked",
        )

        executor = ClaudeCodeExecutor(workspace_path="/tmp", runtime=runtime)
        callback = executor._make_policy_callback()
        result = await callback("Bash", {"command": "rm -rf /", "description": "cleanup"})

        runtime.request_tool_call.assert_awaited_once_with(
            tool="Bash",
            command="rm -rf /",
            path="",
            arguments_preview='{"command": "rm -rf /", "description": "cleanup"}',
        )
        assert isinstance(result, _FakePermissionResultDeny)
        assert result.message == "blocked"

    @pytest.mark.asyncio
    @pytest.mark.usefixtures("_mock_claude_code_sdk")
    async def test_unknown_tool_keeps_its_name(self) -> None:
        runtime = AsyncMock()
        runtime.request_tool_call.return_value = ToolCallDecision(
            call_id="c3",
            decision="allow",
        )

        executor = ClaudeCodeExecutor(workspace_path="/tmp", runtime=runtime)
        callback = executor._make_policy_callback()
        result = await callback("SomeNewTool", {"arg": "val", "command": "curl x"})

        # The arguments are shown to the approver, never evaluated.
        runtime.request_tool_call.assert_awaited_once_with(
            tool="SomeNewTool",
            command="",
            path="",
            arguments_preview='{"arg": "val", "command": "curl x"}',
        )
        assert isinstance(result, _FakePermissionResultAllow)

    @pytest.mark.asyncio
    @pytest.mark.usefixtures("_mock_claude_code_sdk")
    @pytest.mark.parametrize(
        ("tool", "tool_input", "expected_path"),
        [
            ("Edit", {"file_path": "/ws/.env", "old_string": "a", "new_string": "b"}, "/ws/.env"),
            ("MultiEdit", {"file_path": "/ws/a.go", "edits": []}, "/ws/a.go"),
            ("Write", {"file_path": "/ws/b.go", "content": "x"}, "/ws/b.go"),
            ("NotebookEdit", {"notebook_path": "/ws/n.ipynb", "new_source": "x"}, "/ws/n.ipynb"),
            ("Grep", {"pattern": "TODO", "path": "/ws/src"}, "/ws/src"),
            ("Glob", {"pattern": "**/*.go"}, ""),
            ("LS", {"path": "/ws"}, "/ws"),
            ("Edit", {"file_path": 42}, ""),
        ],
    )
    async def test_sends_path_argument(self, tool: str, tool_input: dict[str, object], expected_path: str) -> None:
        runtime = AsyncMock()
        runtime.request_tool_call.return_value = ToolCallDecision(call_id="c4", decision="allow")

        executor = ClaudeCodeExecutor(workspace_path="/ws", runtime=runtime)
        await executor._make_policy_callback()(tool, tool_input)

        runtime.request_tool_call.assert_awaited_once_with(
            tool=tool, command="", path=expected_path, arguments_preview=arguments_preview(tool_input)
        )


# ---------------------------------------------------------------------------
# Environment variable configuration tests
# ---------------------------------------------------------------------------


class TestEnvVarConfig:
    """Tests for CODEFORGE_CLAUDECODE_* env var support."""

    @pytest.fixture(autouse=True)
    def _fresh_settings(self, monkeypatch: pytest.MonkeyPatch) -> Iterator[None]:
        """get_settings() is lru_cached: rebuild it from each test's patched env, without a local codeforge.yaml."""
        monkeypatch.setattr("codeforge.config.load_yaml_config", dict)
        get_settings.cache_clear()
        yield
        get_settings.cache_clear()

    def test_default_max_turns(self) -> None:
        from codeforge.claude_code_executor import get_default_max_turns

        with patch.dict("os.environ", {}, clear=False):
            os.environ.pop("CODEFORGE_CLAUDECODE_MAX_TURNS", None)
            assert get_default_max_turns() == 50

    def test_custom_max_turns(self) -> None:
        from codeforge.claude_code_executor import get_default_max_turns

        with patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_MAX_TURNS": "100"}):
            assert get_default_max_turns() == 100

    def test_default_timeout(self) -> None:
        from codeforge.claude_code_executor import get_timeout_seconds

        with patch.dict("os.environ", {}, clear=False):
            os.environ.pop("CODEFORGE_CLAUDECODE_TIMEOUT", None)
            assert get_timeout_seconds() == 300

    def test_custom_timeout(self) -> None:
        from codeforge.claude_code_executor import get_timeout_seconds

        with patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_TIMEOUT": "600"}):
            assert get_timeout_seconds() == 600

    def test_default_tiers(self) -> None:
        from codeforge.claude_code_executor import get_enabled_tiers

        with patch.dict("os.environ", {}, clear=False):
            os.environ.pop("CODEFORGE_CLAUDECODE_TIERS", None)
            assert get_enabled_tiers() == {"COMPLEX", "REASONING"}

    def test_custom_tiers(self) -> None:
        from codeforge.claude_code_executor import get_enabled_tiers

        with patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_TIERS": "SIMPLE,MEDIUM,COMPLEX"}):
            assert get_enabled_tiers() == {"SIMPLE", "MEDIUM", "COMPLEX"}
