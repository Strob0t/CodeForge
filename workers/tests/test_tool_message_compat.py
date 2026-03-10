"""Tests for tool-message format compatibility across LLM providers (F6).

Verifies that tool messages always include required fields (content, tool_call_id)
regardless of which provider generated them, preventing errors like Groq's:
  'messages.3' : for 'role:tool' the following must be satisfied
  [('messages.3.content' : property 'content' is missing)]
"""

from __future__ import annotations

import pytest


class TestPayloadToDict:
    """_payload_to_dict must always include 'content' for role:tool messages."""

    def _make_tool_msg(self, content: str, tool_call_id: str = "tc_1", name: str = "bash"):
        from codeforge.models import ConversationMessagePayload

        return ConversationMessagePayload(
            role="tool",
            content=content,
            tool_call_id=tool_call_id,
            name=name,
        )

    def test_tool_message_with_content(self):
        """Tool message with non-empty content should include it."""
        from codeforge.agent_loop import _payload_to_dict

        msg = self._make_tool_msg(content="file created successfully")
        d = _payload_to_dict(msg)
        assert d["role"] == "tool"
        assert d["content"] == "file created successfully"
        assert d["tool_call_id"] == "tc_1"

    def test_tool_message_with_empty_content(self):
        """Tool message with empty content MUST still include 'content' key."""
        from codeforge.agent_loop import _payload_to_dict

        msg = self._make_tool_msg(content="")
        d = _payload_to_dict(msg)
        assert d["role"] == "tool"
        assert "content" in d, "content key must be present even when empty"
        assert d["content"] == ""
        assert d["tool_call_id"] == "tc_1"

    def test_tool_message_with_default_empty_content(self):
        """Tool message with default empty content should still have 'content' key.

        ConversationMessagePayload.content is ``str = ""``, so None is not valid.
        This tests the default-empty case.
        """
        from codeforge.models import ConversationMessagePayload

        msg = ConversationMessagePayload(
            role="tool",
            content="",
            tool_call_id="tc_1",
            name="bash",
        )
        from codeforge.agent_loop import _payload_to_dict

        d = _payload_to_dict(msg)
        assert d["role"] == "tool"
        assert "content" in d, "content key must be present for tool messages"
        assert d["content"] == ""

    def test_non_tool_message_without_content_omits_key(self):
        """Non-tool messages (user, assistant) may omit 'content' when empty."""
        from codeforge.agent_loop import _payload_to_dict
        from codeforge.models import ConversationMessagePayload

        msg = ConversationMessagePayload(role="assistant", content="")
        d = _payload_to_dict(msg)
        # For non-tool roles, omitting empty content is acceptable
        assert d["role"] == "assistant"


class TestSanitizeMessages:
    """sanitize_tool_messages ensures all tool messages are provider-compatible."""

    def test_sanitize_adds_missing_content(self):
        """Tool messages missing 'content' key should get it added."""
        from codeforge.agent_loop import sanitize_tool_messages

        messages = [
            {"role": "user", "content": "hello"},
            {
                "role": "assistant",
                "content": "I'll run bash",
                "tool_calls": [{"id": "tc_1", "type": "function", "function": {"name": "bash", "arguments": "{}"}}],
            },
            {"role": "tool", "tool_call_id": "tc_1", "name": "bash"},  # missing content
        ]
        sanitized = sanitize_tool_messages(messages)
        assert sanitized[2]["content"] == ""

    def test_sanitize_preserves_existing_content(self):
        """Tool messages with content should not be modified."""
        from codeforge.agent_loop import sanitize_tool_messages

        messages = [
            {"role": "user", "content": "hello"},
            {"role": "tool", "content": "output here", "tool_call_id": "tc_1", "name": "bash"},
        ]
        sanitized = sanitize_tool_messages(messages)
        assert sanitized[1]["content"] == "output here"

    def test_sanitize_adds_missing_tool_call_id(self):
        """Tool messages missing tool_call_id should get a placeholder."""
        from codeforge.agent_loop import sanitize_tool_messages

        messages = [
            {"role": "tool", "content": "ok", "name": "bash"},  # missing tool_call_id
        ]
        sanitized = sanitize_tool_messages(messages)
        assert "tool_call_id" in sanitized[0]
        assert sanitized[0]["tool_call_id"]  # non-empty

    def test_sanitize_does_not_modify_non_tool_messages(self):
        """User/assistant messages should pass through unchanged."""
        from codeforge.agent_loop import sanitize_tool_messages

        messages = [
            {"role": "user", "content": "hello"},
            {"role": "assistant", "content": "hi"},
        ]
        sanitized = sanitize_tool_messages(messages)
        assert sanitized == messages

    def test_sanitize_converts_none_content_to_empty_string(self):
        """Tool messages with content=None should become content=''."""
        from codeforge.agent_loop import sanitize_tool_messages

        messages = [
            {"role": "tool", "content": None, "tool_call_id": "tc_1", "name": "bash"},
        ]
        sanitized = sanitize_tool_messages(messages)
        assert sanitized[0]["content"] == ""

    def test_sanitize_handles_empty_message_list(self):
        """Empty message list should return empty list."""
        from codeforge.agent_loop import sanitize_tool_messages

        assert sanitize_tool_messages([]) == []

    def test_sanitize_mixed_messages(self):
        """Full conversation with multiple tool results — all should be sanitized."""
        from codeforge.agent_loop import sanitize_tool_messages

        messages = [
            {"role": "user", "content": "create two files"},
            {
                "role": "assistant",
                "content": "",
                "tool_calls": [
                    {"id": "tc_1", "type": "function", "function": {"name": "write", "arguments": "{}"}},
                    {"id": "tc_2", "type": "function", "function": {"name": "write", "arguments": "{}"}},
                ],
            },
            {"role": "tool", "tool_call_id": "tc_1", "name": "write"},  # missing content
            {"role": "tool", "content": "done", "tool_call_id": "tc_2", "name": "write"},
        ]
        sanitized = sanitize_tool_messages(messages)
        assert sanitized[2]["content"] == ""  # filled in
        assert sanitized[3]["content"] == "done"  # preserved


class TestSanitizeWiredIntoLLMCall:
    """Verify sanitize_tool_messages is called before LLM calls."""

    @pytest.mark.asyncio
    async def test_routing_wrapper_sanitizes_messages(self):
        """_RoutingLLMWrapper should sanitize messages before forwarding."""
        from unittest.mock import AsyncMock, MagicMock, patch

        from codeforge.consumer._benchmark import _RoutingLLMWrapper

        mock_llm = AsyncMock()
        mock_llm.chat_completion_stream = AsyncMock(return_value="response")

        mock_router = MagicMock()
        mock_router.route = MagicMock(return_value=None)

        wrapper = _RoutingLLMWrapper(mock_llm, mock_router)

        messages = [
            {"role": "user", "content": "test"},
            {"role": "tool", "tool_call_id": "tc_1", "name": "bash"},  # missing content
        ]

        with patch("codeforge.model_resolver.resolve_model", return_value="openai/gpt-4o-mini"):
            await wrapper.chat_completion_stream(messages=messages)

        # Verify the messages passed to the real LLM have content on tool messages
        call_kwargs = mock_llm.chat_completion_stream.call_args
        forwarded_msgs = call_kwargs.kwargs.get("messages", [])
        tool_msg = next(m for m in forwarded_msgs if m.get("role") == "tool")
        assert "content" in tool_msg
        assert tool_msg["content"] == ""
