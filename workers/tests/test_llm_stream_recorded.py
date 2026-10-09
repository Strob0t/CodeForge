"""Streamed tool calls and token usage from recorded LiteLLM streams (KI-125, KI-127).

The SSE data lines below were recorded through the LiteLLM proxy
(docker.litellm.ai/berriai/litellm:main-stable, 2026-10-03) for the prompt
"Read the files app.py and test_app.py." with one read_file tool:

- OLLAMA_CHAT_SPLIT: LiteLLM's ollama_chat provider when Ollama streams each
  tool call in its own chunk (the upstream was a scripted Ollama /api/chat
  replaying that shape, as seen with qwen2.5:3b-instruct in the README
  screenshot run). LiteLLM numbers every call index 0.
- OLLAMA_CHAT_TOGETHER: the ollama_chat provider with Ollama 0.35.1 and
  qwen3:4b-instruct, which sends both calls in one chunk.
- OPENAI_ROUTE: the shipped ollama/* route (openai/* against Ollama's /v1),
  requested with stream_options.include_usage.
"""

from __future__ import annotations

import json
from unittest.mock import patch

import pytest

from codeforge.llm import LiteLLMClient, _build_stream_payload, _StreamAccumulator

OLLAMA_CHAT_SPLIT = [
    '{"id":"b9386d64-c9a3-4811-9c3f-bdb7b8ec6175","created":1791065552,"model":"fakeollama/qwen2.5:3b-instruct","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"id":"33daaec1-c018-4569-86d9-0171e77fbc94","function":{"arguments":"{\\"path\\": \\"app.py\\"}","name":"read_file"},"type":"function","index":0}]}}]}',
    '{"id":"dd677304-eb50-4a9d-a9ca-0bc8a3512d5b","created":1791065552,"model":"fakeollama/qwen2.5:3b-instruct","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"id":"351ec11c-b078-45b3-aa09-92cb81cb2c68","function":{"arguments":"{\\"path\\": \\"test_app.py\\"}","name":"read_file"},"type":"function","index":0}]}}]}',
    '{"id":"9e885300-e193-4b51-ae60-e591b5bdd05a","object":"chat.completion.chunk","created":1791065552,"model":"fakeollama/qwen2.5:3b-instruct","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}',
    '{"id":"9e885300-e193-4b51-ae60-e591b5bdd05a","created":1791065552,"model":"fakeollama/qwen2.5:3b-instruct","object":"chat.completion.chunk","choices":[{"index":0,"delta":{}}],"usage":{"completion_tokens":43,"prompt_tokens":171,"total_tokens":214}}',
    "[DONE]",
]

OLLAMA_CHAT_TOGETHER = [
    '{"id":"b76a0cb8-2942-4971-92e7-c2c0efe10aac","created":1791065429,"model":"ollama_chat/qwen3:4b-instruct","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"id":"d74bf25d-436e-4b49-b4f5-37c0cc34b2cf","function":{"arguments":"{\\"path\\": \\"app.py\\"}","name":"read_file"},"type":"function","index":0},{"id":"3871b77c-ca85-4d9e-a181-4c7db4df868f","function":{"arguments":"{\\"path\\": \\"test_app.py\\"}","name":"read_file"},"type":"function","index":1}]}}]}',
    '{"id":"b76a0cb8-2942-4971-92e7-c2c0efe10aac","object":"chat.completion.chunk","created":1791065429,"model":"ollama_chat/qwen3:4b-instruct","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}',
    '{"id":"b76a0cb8-2942-4971-92e7-c2c0efe10aac","created":1791065429,"model":"ollama_chat/qwen3:4b-instruct","object":"chat.completion.chunk","choices":[{"index":0,"delta":{}}],"usage":{"completion_tokens":43,"prompt_tokens":171,"total_tokens":214}}',
    "[DONE]",
]

OPENAI_ROUTE = [
    '{"id":"chatcmpl-647","created":1791065494,"model":"ollama/qwen3:4b-instruct","object":"chat.completion.chunk","system_fingerprint":"fp_ollama","choices":[{"index":0,"delta":{"content":"","role":"assistant","tool_calls":[{"id":"AZAgo6RVjZvuUhRK9DnI13UfyLQQiY7j","function":{"arguments":"{\\"path\\":\\"app.py\\"}","name":"read_file"},"type":"function","index":0},{"id":"25S76Dy4Erlrk3XJmCEUv1USdExbVGA5","function":{"arguments":"{\\"path\\":\\"test_app.py\\"}","name":"read_file"},"type":"function","index":1}]}}]}',
    '{"id":"chatcmpl-647","created":1791065494,"model":"ollama/qwen3:4b-instruct","object":"chat.completion.chunk","system_fingerprint":"fp_ollama","choices":[{"finish_reason":"tool_calls","index":0,"delta":{}}]}',
    '{"id":"chatcmpl-647","created":1791065494,"model":"ollama/qwen3:4b-instruct","object":"chat.completion.chunk","system_fingerprint":"fp_ollama","choices":[{"index":0,"delta":{}}],"usage":{"completion_tokens":43,"prompt_tokens":171,"total_tokens":214,"prompt_tokens_details":{"cached_tokens":170}}}',
    "[DONE]",
]


def _accumulate(lines: list[str]) -> _StreamAccumulator:
    acc = _StreamAccumulator()
    for raw in lines:
        if raw != "[DONE]":
            acc.process_chunk(raw, None)
    return acc


def _delta(tool_calls: list[dict[str, object]]) -> str:
    return json.dumps({"choices": [{"index": 0, "delta": {"tool_calls": tool_calls}}]})


@pytest.mark.parametrize(
    ("lines", "ids"),
    [
        pytest.param(
            OLLAMA_CHAT_SPLIT,
            ["33daaec1-c018-4569-86d9-0171e77fbc94", "351ec11c-b078-45b3-aa09-92cb81cb2c68"],
            id="ollama_chat-one-call-per-chunk-all-index-0",
        ),
        pytest.param(
            OLLAMA_CHAT_TOGETHER,
            ["d74bf25d-436e-4b49-b4f5-37c0cc34b2cf", "3871b77c-ca85-4d9e-a181-4c7db4df868f"],
            id="ollama_chat-calls-in-one-chunk",
        ),
        pytest.param(
            OPENAI_ROUTE,
            ["AZAgo6RVjZvuUhRK9DnI13UfyLQQiY7j", "25S76Dy4Erlrk3XJmCEUv1USdExbVGA5"],
            id="openai-route",
        ),
    ],
)
def test_recorded_parallel_calls_are_separate(lines: list[str], ids: list[str]) -> None:
    acc = _accumulate(lines)

    calls = acc.build_tool_calls(None)

    assert [c.id for c in calls] == ids
    assert [c.name for c in calls] == ["read_file", "read_file"]
    assert [json.loads(c.arguments) for c in calls] == [{"path": "app.py"}, {"path": "test_app.py"}]
    assert acc.finish_reason == "tool_calls"


@pytest.mark.parametrize("lines", [OLLAMA_CHAT_SPLIT, OLLAMA_CHAT_TOGETHER, OPENAI_ROUTE])
def test_recorded_usage_is_counted(lines: list[str]) -> None:
    acc = _accumulate(lines)

    assert (acc.tokens_in, acc.tokens_out) == (171, 43)


def test_same_index_new_name_without_ids_starts_a_new_call() -> None:
    acc = _StreamAccumulator()
    acc.process_chunk(_delta([{"index": 0, "function": {"name": "read_file", "arguments": '{"path": "a"}'}}]), None)
    acc.process_chunk(_delta([{"index": 0, "function": {"name": "bash", "arguments": '{"command": "ls"}'}}]), None)

    calls = acc.build_tool_calls(None)

    assert [(c.name, c.arguments) for c in calls] == [
        ("read_file", '{"path": "a"}'),
        ("bash", '{"command": "ls"}'),
    ]


def test_same_index_same_name_without_ids_starts_a_new_call() -> None:
    acc = _StreamAccumulator()
    for path in ("a", "b"):
        acc.process_chunk(
            _delta([{"index": 0, "function": {"name": "read_file", "arguments": f'{{"path": "{path}"}}'}}]), None
        )

    calls = acc.build_tool_calls(None)

    assert [json.loads(c.arguments) for c in calls] == [{"path": "a"}, {"path": "b"}]


def test_fragments_repeating_the_id_stay_one_call() -> None:
    acc = _StreamAccumulator()
    acc.process_chunk(_delta([{"index": 0, "id": "c1", "function": {"name": "read_file", "arguments": '{"pa'}}]), None)
    acc.process_chunk(_delta([{"index": 0, "id": "c1", "function": {"arguments": 'th": "a"}'}}]), None)

    calls = acc.build_tool_calls(None)

    assert [(c.id, c.name, c.arguments) for c in calls] == [("c1", "read_file", '{"path": "a"}')]


def test_fragments_with_null_id_and_name_stay_one_call() -> None:
    acc = _StreamAccumulator()
    acc.process_chunk(_delta([{"index": 0, "id": "c1", "function": {"name": "bash", "arguments": ""}}]), None)
    acc.process_chunk(_delta([{"index": 0, "id": None, "function": {"name": None, "arguments": '{"command":'}}]), None)
    acc.process_chunk(_delta([{"index": 0, "id": "", "function": {"name": "", "arguments": ' "ls"}'}}]), None)

    calls = acc.build_tool_calls(None)

    assert [(c.id, c.name, c.arguments) for c in calls] == [("c1", "bash", '{"command": "ls"}')]


def test_interleaved_fragments_of_two_indices() -> None:
    acc = _StreamAccumulator()
    acc.process_chunk(_delta([{"index": 0, "id": "a", "function": {"name": "read_file", "arguments": ""}}]), None)
    acc.process_chunk(_delta([{"index": 1, "id": "b", "function": {"name": "bash", "arguments": ""}}]), None)
    acc.process_chunk(_delta([{"index": 0, "function": {"arguments": '{"path": "x"}'}}]), None)
    acc.process_chunk(_delta([{"index": 1, "function": {"arguments": '{"command": "ls"}'}}]), None)

    calls = acc.build_tool_calls(None)

    assert [(c.id, c.name, c.arguments) for c in calls] == [
        ("a", "read_file", '{"path": "x"}'),
        ("b", "bash", '{"command": "ls"}'),
    ]


def test_usage_chunk_without_choices_is_counted() -> None:
    """OpenAI sends the include_usage chunk with an empty choices list (KI-127)."""
    acc = _StreamAccumulator()
    acc.process_chunk(
        json.dumps({"choices": [{"index": 0, "delta": {"content": "hi"}, "finish_reason": "stop"}]}), None
    )
    acc.process_chunk(json.dumps({"choices": [], "usage": {"prompt_tokens": 12, "completion_tokens": 3}}), None)

    assert (acc.tokens_in, acc.tokens_out) == (12, 3)
    assert acc.finish_reason == "stop"


def test_stream_payload_requests_usage() -> None:
    payload = _build_stream_payload(
        model="ollama/qwen3:4b-instruct",
        messages=[{"role": "user", "content": "hi"}],
        temperature=0.2,
        tools=None,
        tool_choice=None,
        tags=None,
        max_tokens=None,
        provider_api_key="",
        top_p=None,
        extra_body=None,
    )

    assert payload["stream_options"] == {"include_usage": True}
    assert "response_format" not in payload


def test_stream_payload_carries_response_format() -> None:
    fmt: dict[str, object] = {"type": "json_schema", "json_schema": {"name": "codeforge_turn", "schema": {}}}
    payload = _build_stream_payload(
        model="ollama/llama3",
        messages=[{"role": "user", "content": "hi"}],
        temperature=0.2,
        tools=None,
        tool_choice=None,
        tags=None,
        max_tokens=8192,
        provider_api_key="",
        top_p=None,
        extra_body=None,
        response_format=fmt,
    )

    assert payload["response_format"] == fmt
    assert payload["max_tokens"] == 8192


class _FakeStream:
    status_code = 200

    def __init__(self, lines: list[str]) -> None:
        self._lines = lines
        self.headers: dict[str, str] = {}

    async def aiter_lines(self):
        for line in self._lines:
            yield line

    async def __aenter__(self):
        return self

    async def __aexit__(self, *args: object) -> None:
        return None


async def test_streamed_openai_route_reports_tokens() -> None:
    """A streamed request asks for usage and returns the token counts of the OpenAI-compatible route."""
    client = LiteLLMClient(base_url="http://test:4000", api_key="k")
    sent: dict[str, object] = {}

    def fake_stream(method: str, url: str, *, json: dict[str, object]) -> _FakeStream:
        sent.update(json)
        return _FakeStream([f"data: {raw}" for raw in OPENAI_ROUTE])

    with patch.object(client._client, "stream", side_effect=fake_stream):
        result = await client.chat_completion_stream(
            messages=[{"role": "user", "content": "Read the files app.py and test_app.py."}],
            model="ollama/qwen3:4b-instruct",
        )

    assert sent["stream_options"] == {"include_usage": True}
    assert (result.tokens_in, result.tokens_out) == (171, 43)
    assert len(result.tool_calls) == 2
