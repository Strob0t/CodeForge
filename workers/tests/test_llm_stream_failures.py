"""A stream that fails is an LLMError, and one the caller already saw text of is never repeated (KI-196).

LiteLLM reports an upstream failure after the first chunk as an SSE frame
``data: {"error": {...}}`` on an HTTP 200 stream (proxy_server.async_data_generator,
v1.103.1). Dropping that frame turned a crashed or overloaded upstream into a
truncated "successful" answer (R9-2). A retry of a stream whose text the
caller already got (on_chunk) sent the text again (R9-12).
"""

from __future__ import annotations

import json

import httpx
import pytest

from codeforge.llm import LiteLLMClient, LLMClientConfig, LLMError, _StreamAccumulator

MODEL = "anthropic/claude-sonnet-4-20250514"


def _sse(*frames: str) -> bytes:
    return "".join(f"data: {frame}\n\n" for frame in frames).encode()


def _content(text: str, finish: str | None = None) -> str:
    return json.dumps({"choices": [{"index": 0, "delta": {"content": text}, "finish_reason": finish}]})


def _error(code: object = "500", message: str = "AnthropicException - Overloaded") -> str:
    return json.dumps({"error": {"message": message, "type": "None", "param": "None", "code": code}})


class _Body(httpx.AsyncByteStream):
    """A response body that yields *parts* and then fails with *fail* (a broken connection)."""

    def __init__(self, parts: list[bytes], fail: Exception | None = None) -> None:
        self._parts = parts
        self._fail = fail

    async def __aiter__(self):  # type: ignore[override]
        for part in self._parts:
            yield part
        if self._fail is not None:
            raise self._fail


def _client(bodies: list[_Body], max_retries: int = 1) -> tuple[LiteLLMClient, list[str]]:
    """A client whose n-th chat request gets bodies[n]; the second list records the request paths."""
    paths: list[str] = []

    def handler(request: httpx.Request) -> httpx.Response:
        paths.append(request.url.path)
        body = bodies[min(len(paths), len(bodies)) - 1]
        return httpx.Response(200, stream=body, headers={"content-type": "text/event-stream"})

    client = LiteLLMClient(
        base_url="http://litellm",
        config=LLMClientConfig(max_retries=max_retries, backoff_base=0.0, backoff_max=0.0),
    )
    client._client = httpx.AsyncClient(base_url="http://litellm", transport=httpx.MockTransport(handler))
    return client, paths


async def test_an_error_frame_after_text_raises_and_is_not_retried() -> None:
    client, paths = _client([_Body([_sse(_content("Let me fix the "), _error("500"), "[DONE]")])])
    seen: list[str] = []

    with pytest.raises(LLMError) as raised:
        await client.chat_completion_stream(
            messages=[{"role": "user", "content": "hi"}], model=MODEL, on_chunk=seen.append
        )

    assert raised.value.status_code == 500
    assert raised.value.model == MODEL
    assert "Overloaded" in raised.value.body
    assert seen == ["Let me fix the "]
    assert paths == ["/v1/chat/completions"], "text was shown: no retry"


async def test_an_error_frame_before_any_text_is_retried() -> None:
    client, paths = _client(
        [
            _Body([_sse(_error("503", "upstream unavailable"), "[DONE]")]),
            _Body([_sse(_content("Hello", "stop"), "[DONE]")]),
        ]
    )
    seen: list[str] = []

    response = await client.chat_completion_stream(
        messages=[{"role": "user", "content": "hi"}], model=MODEL, on_chunk=seen.append
    )

    assert response.content == "Hello"
    assert seen == ["Hello"]
    assert len(paths) == 2


async def test_an_error_frame_that_is_not_retryable_raises_at_once() -> None:
    client, paths = _client([_Body([_sse(_error("400", "context window exceeded"), "[DONE]")])])

    with pytest.raises(LLMError) as raised:
        await client.chat_completion_stream(messages=[{"role": "user", "content": "hi"}], model=MODEL)

    assert raised.value.status_code == 400
    assert len(paths) == 1


@pytest.mark.parametrize(
    ("code", "status"),
    [
        pytest.param("529", 529, id="string-code"),
        pytest.param(429, 429, id="int-code"),
        pytest.param("None", 500, id="none-string"),
        pytest.param(None, 500, id="null"),
        pytest.param("abc", 500, id="not-a-number"),
        pytest.param(True, 500, id="bool"),
        pytest.param("200", 500, id="not-an-error-status"),
        pytest.param("999", 500, id="out-of-range"),
    ],
)
def test_the_status_of_an_error_frame(code: object, status: int) -> None:
    acc = _StreamAccumulator(model=MODEL)

    with pytest.raises(LLMError) as raised:
        acc.process_chunk(_error(code), None)

    assert raised.value.status_code == status
    assert raised.value.model == MODEL


@pytest.mark.parametrize(
    "frame",
    [
        pytest.param(json.dumps({"error": "plain text error"}), id="string-error"),
        pytest.param(json.dumps({"error": {}}), id="empty-error-object"),
    ],
)
def test_any_error_frame_raises(frame: str) -> None:
    with pytest.raises(LLMError) as raised:
        _StreamAccumulator(model=MODEL).process_chunk(frame, None)
    assert raised.value.status_code == 500


def test_a_null_error_field_is_not_an_error() -> None:
    acc = _StreamAccumulator(model=MODEL)
    acc.process_chunk(json.dumps({"error": None, "choices": [{"delta": {"content": "ok"}}]}), None)
    assert acc.content_parts == ["ok"]


async def test_a_stream_broken_after_shown_text_is_not_retried() -> None:
    """R9-12: the UI received 'Hello Hello world' and two requests were billed."""
    client, paths = _client(
        [
            _Body([_sse(_content("Hello "))], fail=httpx.ReadError("peer closed connection")),
            _Body([_sse(_content("Hello "), _content("world", "stop"), "[DONE]")]),
        ]
    )
    seen: list[str] = []

    with pytest.raises(LLMError) as raised:
        await client.chat_completion_stream(
            messages=[{"role": "user", "content": "hi"}], model=MODEL, on_chunk=seen.append
        )

    assert raised.value.status_code == 408  # a transport error, as before
    assert "".join(seen) == "Hello "
    assert len(paths) == 1


async def test_a_stream_broken_before_any_text_is_retried() -> None:
    client, paths = _client(
        [
            _Body([], fail=httpx.ReadError("peer closed connection")),
            _Body([_sse(_content("Hello "), _content("world", "stop"), "[DONE]")]),
        ]
    )
    seen: list[str] = []

    response = await client.chat_completion_stream(
        messages=[{"role": "user", "content": "hi"}], model=MODEL, on_chunk=seen.append
    )

    assert response.content == "Hello world"
    assert "".join(seen) == "Hello world"
    assert len(paths) == 2


async def test_a_broken_stream_nobody_watched_is_retried_without_duplicates() -> None:
    """Without on_chunk no text left the client: the retry starts a fresh answer."""
    client, paths = _client(
        [
            _Body([_sse(_content("Hello "))], fail=httpx.ReadError("peer closed connection")),
            _Body([_sse(_content("Hello "), _content("world", "stop"), "[DONE]")]),
        ]
    )

    response = await client.chat_completion_stream(messages=[{"role": "user", "content": "hi"}], model=MODEL)

    assert response.content == "Hello world"
    assert len(paths) == 2


async def test_thinking_only_text_does_not_block_a_retry() -> None:
    """Text inside <think> never reached on_chunk: the caller saw nothing, a retry repeats nothing."""
    client, paths = _client(
        [
            _Body([_sse(_content("<think>plan"))], fail=httpx.ReadError("peer closed connection")),
            _Body([_sse(_content("Done", "stop"), "[DONE]")]),
        ]
    )
    seen: list[str] = []

    response = await client.chat_completion_stream(
        messages=[{"role": "user", "content": "hi"}], model=MODEL, on_chunk=seen.append
    )

    assert response.content == "Done"
    assert seen == ["Done"]
    assert len(paths) == 2
