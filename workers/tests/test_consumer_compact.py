"""Tests for the conversation compact handler mixin."""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._compact import CompactHandlerMixin
from codeforge.consumer._subjects import SUBJECT_CONVERSATION_COMPACT_COMPLETE

# ---------------------------------------------------------------------------
# Test harness
# ---------------------------------------------------------------------------


class _TestMixin(CompactHandlerMixin, ConsumerBaseMixin):
    def __init__(self) -> None:
        self._js: AsyncMock | None = AsyncMock()
        self._processed_ids: set[str] = set()
        self._processed_ids_max = 10_000
        self._llm = MagicMock()


@pytest.fixture(autouse=True)
def _fresh_state() -> None:
    ConsumerBaseMixin._processed_ids = set()


def _make_msg(data: dict) -> MagicMock:
    msg = MagicMock()
    msg.data = json.dumps(data).encode()
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.headers = {}
    return msg


def _compact_payload(conversation_id: str = "conv-1") -> dict:
    return {
        "conversation_id": conversation_id,
        "tenant_id": "tenant-1",
    }


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


async def test_compact_success() -> None:
    """Compact request with messages produces a summary and publishes it."""
    mixin = _TestMixin()
    messages = [
        {"role": "user", "content": "Hello"},
        {"role": "assistant", "content": "Hi there"},
    ]

    # Mock the internal HTTP fetch to return messages
    mixin._fetch_conversation_messages = AsyncMock(return_value=messages)

    # Mock LLM summarization
    mock_response = MagicMock()
    mock_response.content = "Summary of conversation"
    mixin._llm.chat_completion = AsyncMock(return_value=mock_response)

    msg = _make_msg(_compact_payload())

    await mixin._handle_conversation_compact(msg)

    assert mixin._js is not None
    mixin._js.publish.assert_called_once()
    subject = mixin._js.publish.call_args.args[0]
    assert subject == SUBJECT_CONVERSATION_COMPACT_COMPLETE
    result = json.loads(mixin._js.publish.call_args.args[1])
    assert result["summary"] == "Summary of conversation"
    assert result["original_count"] == 2
    assert result["status"] == "completed"
    msg.ack.assert_called_once()


async def test_compact_no_conversation_id() -> None:
    """A request without conversation_id is invalid: dead-lettered and terminated."""
    mixin = _TestMixin()
    msg = _make_msg({"conversation_id": "", "tenant_id": "t"})
    msg.subject = "conversation.compact.request"
    msg.term = AsyncMock()

    await mixin._handle_conversation_compact(msg)

    assert mixin._js is not None
    mixin._js.publish.assert_awaited_once_with("conversation.compact.request.dlq", msg.data, headers=None)
    msg.term.assert_awaited_once()
    msg.ack.assert_not_called()


async def test_compact_no_messages() -> None:
    """When fetch returns no messages, acks once without publishing."""
    mixin = _TestMixin()
    mixin._fetch_conversation_messages = AsyncMock(return_value=[])
    msg = _make_msg(_compact_payload())

    await mixin._handle_conversation_compact(msg)

    assert mixin._js is not None
    mixin._js.publish.assert_not_called()
    # A second ack of a real JetStream message raises MsgAlreadyAckdError.
    msg.ack.assert_called_once()


async def test_compact_llm_failure() -> None:
    """LLM failure returns a fallback summary and still acks."""
    mixin = _TestMixin()
    messages = [{"role": "user", "content": "Hello"}]
    mixin._fetch_conversation_messages = AsyncMock(return_value=messages)
    mixin._llm.chat_completion = AsyncMock(side_effect=RuntimeError("LLM down"))

    msg = _make_msg(_compact_payload())

    await mixin._handle_conversation_compact(msg)

    assert mixin._js is not None
    mixin._js.publish.assert_called_once()
    result = json.loads(mixin._js.publish.call_args.args[1])
    assert "Compact failed" in result["summary"]
    msg.ack.assert_called_once()


async def test_compact_truncates_long_conversations() -> None:
    """Very long conversations are truncated before being sent to LLM."""
    mixin = _TestMixin()
    # Create messages that exceed the 50000 char limit
    long_content = "x" * 60000
    messages = [{"role": "user", "content": long_content}]
    mixin._fetch_conversation_messages = AsyncMock(return_value=messages)

    mock_response = MagicMock()
    mock_response.content = "Truncated summary"
    mixin._llm.chat_completion = AsyncMock(return_value=mock_response)

    msg = _make_msg(_compact_payload())

    await mixin._handle_conversation_compact(msg)

    # Verify the prompt passed to LLM was truncated
    call_args = mixin._llm.chat_completion.call_args
    user_msg_content = call_args.kwargs["messages"][1]["content"]
    assert "... (truncated)" in user_msg_content
    msg.ack.assert_called_once()


async def test_compact_api_fetch_failure() -> None:
    """When API fetch fails, no messages are returned and acks."""
    mixin = _TestMixin()
    mixin._fetch_conversation_messages = AsyncMock(return_value=[])
    msg = _make_msg(_compact_payload())

    await mixin._handle_conversation_compact(msg)

    assert mixin._js is not None
    mixin._js.publish.assert_not_called()
    assert msg.ack.call_count >= 1


async def test_compact_invalid_json_is_dead_lettered() -> None:
    """Invalid JSON goes to the DLQ and is terminated (never NAK'd, never acked)."""
    mixin = _TestMixin()
    msg = MagicMock()
    msg.subject = "conversation.compact.request"
    msg.data = b"{{invalid"
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.term = AsyncMock()
    msg.headers = {}

    await mixin._handle_conversation_compact(msg)

    assert mixin._js is not None
    mixin._js.publish.assert_awaited_once_with("conversation.compact.request.dlq", b"{{invalid", headers=None)
    msg.term.assert_awaited_once()
    msg.ack.assert_not_called()
    msg.nak.assert_not_called()


async def test_compact_failure_is_acked_once() -> None:
    """A failure after a valid request is logged and acked exactly once (not retried)."""
    mixin = _TestMixin()
    mixin._fetch_conversation_messages = AsyncMock(side_effect=RuntimeError("core down"))
    msg = _make_msg(_compact_payload())

    await mixin._handle_conversation_compact(msg)

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()
