"""Tests for the conversation history manager (Phase 17E.2)."""

from __future__ import annotations

from codeforge.history import (
    ConversationHistoryManager,
    HistoryConfig,
    estimate_tokens,
    truncate_tool_result,
)
from codeforge.models import (
    ContextEntry,
    ConversationMessagePayload,
    ConversationToolCallFunction,
    ConversationToolCallPayload,
)


def test_estimate_tokens_basic() -> None:
    assert estimate_tokens("hello") == 1  # 5 chars / 4 = 1
    assert estimate_tokens("a" * 100) == 25  # 100 / 4 = 25
    assert estimate_tokens("") == 1  # min 1


def test_truncate_tool_result_short() -> None:
    text = "short output"
    assert truncate_tool_result(text, 100) == text


def test_truncate_tool_result_long() -> None:
    text = "A" * 200
    result = truncate_tool_result(text, 100)
    assert len(result) < 200
    assert "characters omitted" in result
    assert result.startswith("A" * 50)
    assert result.endswith("A" * 50)


def test_build_messages_basic() -> None:
    mgr = ConversationHistoryManager()
    history = [
        ConversationMessagePayload(role="user", content="Hello"),
        ConversationMessagePayload(role="assistant", content="Hi there!"),
    ]
    msgs = mgr.build_messages("You are a helpful assistant.", history)

    assert len(msgs) == 3
    assert msgs[0]["role"] == "system"
    assert msgs[0]["content"] == "You are a helpful assistant."
    assert msgs[1]["role"] == "user"
    assert msgs[2]["role"] == "assistant"


def test_build_messages_with_context() -> None:
    mgr = ConversationHistoryManager()
    context = [
        ContextEntry(kind="repomap", path="", content="# File structure\nsrc/main.py"),
    ]
    msgs = mgr.build_messages("Base prompt.", [], context_entries=context)

    assert len(msgs) == 1
    content = msgs[0]["content"]
    assert isinstance(content, str)
    assert "Base prompt." in content
    assert "Repomap" in content
    assert "src/main.py" in content


def test_build_messages_truncates_tool_results() -> None:
    mgr = ConversationHistoryManager(HistoryConfig(tool_output_max_chars=50))
    history = [
        ConversationMessagePayload(role="tool", content="X" * 200, tool_call_id="c1", name="read_file"),
    ]
    msgs = mgr.build_messages("system", history)
    tool_msg = msgs[1]
    content = tool_msg["content"]
    assert isinstance(content, str)
    assert len(content) < 200
    assert "characters omitted" in content


def test_build_messages_includes_tool_calls() -> None:
    mgr = ConversationHistoryManager()
    history = [
        ConversationMessagePayload(
            role="assistant",
            content="",
            tool_calls=[
                ConversationToolCallPayload(
                    id="call_1",
                    type="function",
                    function=ConversationToolCallFunction(name="read_file", arguments='{"file_path":"x.py"}'),
                ),
            ],
        ),
        ConversationMessagePayload(role="tool", content="file contents", tool_call_id="call_1", name="read_file"),
    ]
    msgs = mgr.build_messages("sys", history)

    assert len(msgs) == 3
    assistant_msg = msgs[1]
    assert "tool_calls" in assistant_msg
    tool_calls = assistant_msg["tool_calls"]
    assert len(tool_calls) == 1
    assert tool_calls[0]["id"] == "call_1"

    tool_msg = msgs[2]
    assert tool_msg["tool_call_id"] == "call_1"
    assert tool_msg["name"] == "read_file"


def test_build_messages_budget_drops_old() -> None:
    """When history exceeds token budget, old messages are dropped."""
    # Use a very small budget so most messages won't fit.
    mgr = ConversationHistoryManager(
        HistoryConfig(
            max_context_tokens=50,
            min_recent_messages=2,
        )
    )
    history = [
        ConversationMessagePayload(role="user", content="A" * 100),
        ConversationMessagePayload(role="assistant", content="B" * 100),
        ConversationMessagePayload(role="user", content="C" * 10),
        ConversationMessagePayload(role="assistant", content="D" * 10),
    ]
    msgs = mgr.build_messages("sys", history)

    # System + at least the last 2 messages (tail), old ones may be dropped.
    assert msgs[0]["role"] == "system"
    # The tail (last 2) should always be present.
    roles = [m["role"] for m in msgs[1:]]
    assert roles[-2:] == ["user", "assistant"]


def test_build_messages_empty_history() -> None:
    mgr = ConversationHistoryManager()
    msgs = mgr.build_messages("prompt", [])
    assert len(msgs) == 1
    assert msgs[0]["role"] == "system"
