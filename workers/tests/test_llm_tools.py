"""Tests for LiteLLM client tool-calling support (Phase 17A.1)."""

from __future__ import annotations

from unittest.mock import AsyncMock, patch

import httpx
import pytest

from codeforge.llm import (
    ChatCompletionResponse,
    LiteLLMClient,
    ToolCallPart,
    _parse_tool_calls,
)

_FAKE_REQUEST = httpx.Request("POST", "http://test:4000/v1/chat/completions")


@pytest.fixture
def client() -> LiteLLMClient:
    return LiteLLMClient(base_url="http://test:4000", api_key="test-key")


# -- _parse_tool_calls --


def test_parse_tool_calls_valid() -> None:
    raw = [
        {
            "id": "call_1",
            "type": "function",
            "function": {"name": "read_file", "arguments": '{"path": "main.py"}'},
        },
        {
            "id": "call_2",
            "type": "function",
            "function": {"name": "bash", "arguments": '{"command": "ls"}'},
        },
    ]
    result = _parse_tool_calls(raw)
    assert len(result) == 2
    assert result[0] == ToolCallPart(id="call_1", name="read_file", arguments='{"path": "main.py"}')
    assert result[1] == ToolCallPart(id="call_2", name="bash", arguments='{"command": "ls"}')


def test_parse_tool_calls_none() -> None:
    assert _parse_tool_calls(None) == []


def test_parse_tool_calls_not_list() -> None:
    assert _parse_tool_calls("invalid") == []


def test_parse_tool_calls_invalid_entries() -> None:
    raw = [42, {"no_function": True}, {"function": "not_a_dict"}]
    result = _parse_tool_calls(raw)
    assert len(result) == 0


# -- chat_completion --


async def test_chat_completion_with_tool_calls(client: LiteLLMClient) -> None:
    """chat_completion() should parse tool_calls and finish_reason."""
    mock_response = httpx.Response(
        200,
        json={
            "choices": [
                {
                    "message": {
                        "content": None,
                        "tool_calls": [
                            {
                                "id": "call_abc",
                                "type": "function",
                                "function": {
                                    "name": "read_file",
                                    "arguments": '{"file_path": "README.md"}',
                                },
                            }
                        ],
                    },
                    "finish_reason": "tool_calls",
                }
            ],
            "usage": {"prompt_tokens": 100, "completion_tokens": 20},
        },
        request=_FAKE_REQUEST,
    )

    tools = [
        {
            "type": "function",
            "function": {
                "name": "read_file",
                "description": "Read a file",
                "parameters": {"type": "object", "properties": {"file_path": {"type": "string"}}},
            },
        }
    ]

    with patch.object(client._client, "post", new_callable=AsyncMock, return_value=mock_response):
        result = await client.chat_completion(
            messages=[{"role": "user", "content": "Read the readme"}],
            model="gpt-4o",
            tools=tools,
        )

    assert isinstance(result, ChatCompletionResponse)
    assert result.finish_reason == "tool_calls"
    assert result.content == ""
    assert len(result.tool_calls) == 1
    assert result.tool_calls[0].id == "call_abc"
    assert result.tool_calls[0].name == "read_file"
    assert result.tool_calls[0].arguments == '{"file_path": "README.md"}'
    assert result.tokens_in == 100
    assert result.tokens_out == 20


async def test_chat_completion_text_only(client: LiteLLMClient) -> None:
    """chat_completion() should work without tools (backward compatible)."""
    mock_response = httpx.Response(
        200,
        json={
            "choices": [
                {
                    "message": {"content": "Hello there!", "tool_calls": None},
                    "finish_reason": "stop",
                }
            ],
            "usage": {"prompt_tokens": 10, "completion_tokens": 5},
        },
        request=_FAKE_REQUEST,
    )

    with patch.object(client._client, "post", new_callable=AsyncMock, return_value=mock_response):
        result = await client.chat_completion(
            messages=[{"role": "user", "content": "Hi"}],
        )

    assert result.content == "Hello there!"
    assert result.tool_calls == []
    assert result.finish_reason == "stop"


async def test_chat_completion_empty_choices(client: LiteLLMClient) -> None:
    """chat_completion() should handle empty choices."""
    mock_response = httpx.Response(
        200,
        json={"choices": [], "usage": {}},
        request=_FAKE_REQUEST,
    )

    with patch.object(client._client, "post", new_callable=AsyncMock, return_value=mock_response):
        result = await client.chat_completion(
            messages=[{"role": "user", "content": "test"}],
        )

    assert result.content == ""
    assert result.tool_calls == []
    assert result.finish_reason == "stop"


async def test_chat_completion_sends_tools_in_payload(client: LiteLLMClient) -> None:
    """chat_completion() should include tools and tool_choice in the request."""
    mock_response = httpx.Response(
        200,
        json={
            "choices": [{"message": {"content": "ok"}, "finish_reason": "stop"}],
            "usage": {"prompt_tokens": 10, "completion_tokens": 5},
        },
        request=_FAKE_REQUEST,
    )

    tools = [{"type": "function", "function": {"name": "test", "description": "test"}}]

    with patch.object(client._client, "post", new_callable=AsyncMock, return_value=mock_response) as mock_post:
        await client.chat_completion(
            messages=[{"role": "user", "content": "test"}],
            tools=tools,
            tool_choice="auto",
            tags=["default"],
            max_tokens=500,
        )

    payload = mock_post.call_args.kwargs["json"]
    assert payload["tools"] == tools
    assert payload["tool_choice"] == "auto"
    assert payload["tags"] == ["default"]
    assert payload["max_tokens"] == 500


async def test_chat_completion_cost_from_header(client: LiteLLMClient) -> None:
    """chat_completion() should extract cost from response header."""
    mock_response = httpx.Response(
        200,
        json={
            "choices": [{"message": {"content": "ok"}, "finish_reason": "stop"}],
            "usage": {"prompt_tokens": 10, "completion_tokens": 5},
        },
        headers={"x-litellm-response-cost": "0.005"},
        request=_FAKE_REQUEST,
    )

    with patch.object(client._client, "post", new_callable=AsyncMock, return_value=mock_response):
        result = await client.chat_completion(
            messages=[{"role": "user", "content": "test"}],
        )

    assert result.cost_usd == pytest.approx(0.005)


# -- chat_completion_stream --


class FakeStreamResponse:
    """Mock for httpx streaming response with SSE data."""

    def __init__(self, lines: list[str], headers: dict[str, str] | None = None) -> None:
        self._lines = lines
        self.headers = headers or {}

    async def aiter_lines(self):
        for line in self._lines:
            yield line

    def raise_for_status(self) -> None:
        pass

    async def __aenter__(self):
        return self

    async def __aexit__(self, *args):
        pass


async def test_stream_text_only(client: LiteLLMClient) -> None:
    """Streaming with text-only response (no tool calls)."""
    lines = [
        'data: {"choices":[{"delta":{"content":"Hello "},"finish_reason":null}]}',
        'data: {"choices":[{"delta":{"content":"world"},"finish_reason":null}]}',
        'data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2}}',
        "data: [DONE]",
    ]
    chunks: list[str] = []

    with patch.object(client._client, "stream", return_value=FakeStreamResponse(lines)):
        result = await client.chat_completion_stream(
            messages=[{"role": "user", "content": "Hi"}],
            on_chunk=lambda c: chunks.append(c),
        )

    assert result.content == "Hello world"
    assert result.tool_calls == []
    assert result.finish_reason == "stop"
    assert chunks == ["Hello ", "world"]


async def test_stream_tool_call_assembly(client: LiteLLMClient) -> None:
    """Streaming should accumulate tool_call deltas across chunks by index."""
    lines = [
        # First chunk: tool call header with id and function name.
        'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read_file","arguments":""}}]},"finish_reason":null}]}',
        # Second chunk: argument fragment.
        'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\\"file"}}]},"finish_reason":null}]}',
        # Third chunk: rest of arguments.
        'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"_path\\":\\"a.py\\"}"}}]},"finish_reason":null}]}',
        # Final chunk with finish_reason.
        'data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":50,"completion_tokens":10}}',
        "data: [DONE]",
    ]

    tool_calls_received: list[ToolCallPart] = []

    with patch.object(client._client, "stream", return_value=FakeStreamResponse(lines)):
        result = await client.chat_completion_stream(
            messages=[{"role": "user", "content": "Read a.py"}],
            tools=[{"type": "function", "function": {"name": "read_file"}}],
            on_tool_call=lambda tc: tool_calls_received.append(tc),
        )

    assert result.finish_reason == "tool_calls"
    assert len(result.tool_calls) == 1
    tc = result.tool_calls[0]
    assert tc.id == "call_1"
    assert tc.name == "read_file"
    assert tc.arguments == '{"file_path":"a.py"}'
    assert tool_calls_received == [tc]


async def test_stream_multiple_tool_calls(client: LiteLLMClient) -> None:
    """Streaming should handle multiple parallel tool calls by index."""
    lines = [
        # Two tool calls starting in the same chunk.
        'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","function":{"name":"read_file","arguments":"{\\"p\\": \\"x\\"}"}},{"index":1,"id":"call_b","function":{"name":"bash","arguments":"{\\"c\\": \\"ls\\"}"}}]},"finish_reason":null}]}',
        'data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}',
        "data: [DONE]",
    ]

    with patch.object(client._client, "stream", return_value=FakeStreamResponse(lines)):
        result = await client.chat_completion_stream(
            messages=[{"role": "user", "content": "test"}],
        )

    assert len(result.tool_calls) == 2
    assert result.tool_calls[0].name == "read_file"
    assert result.tool_calls[1].name == "bash"


async def test_stream_ignores_non_data_lines(client: LiteLLMClient) -> None:
    """Streaming should skip non-SSE lines and invalid JSON."""
    lines = [
        ": keep-alive",
        "",
        "event: message",
        'data: {"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}',
        "data: not-json",
        "data: [DONE]",
    ]

    with patch.object(client._client, "stream", return_value=FakeStreamResponse(lines)):
        result = await client.chat_completion_stream(
            messages=[{"role": "user", "content": "test"}],
        )

    assert result.content == "ok"
    assert result.finish_reason == "stop"


async def test_stream_usage_from_final_chunk(client: LiteLLMClient) -> None:
    """Streaming should extract usage from the final chunk."""
    lines = [
        'data: {"choices":[{"delta":{"content":"hi"},"finish_reason":null}]}',
        'data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":42,"completion_tokens":7}}',
        "data: [DONE]",
    ]

    with patch.object(
        client._client,
        "stream",
        return_value=FakeStreamResponse(lines, headers={"x-litellm-response-cost": "0.001"}),
    ):
        result = await client.chat_completion_stream(
            messages=[{"role": "user", "content": "test"}],
        )

    assert result.tokens_in == 42
    assert result.tokens_out == 7
    assert result.cost_usd == pytest.approx(0.001)
