"""Tests for ClaudeCodeExecutor."""

from __future__ import annotations

from unittest.mock import AsyncMock, patch

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
