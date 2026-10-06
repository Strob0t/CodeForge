"""Streamed completions carry their real cost (KI-196, R9-1).

LiteLLM v1.103.1 (the image docker-compose.prod.yml pins) builds the headers of a
stream before the stream starts, so ``x-litellm-response-cost`` is empty, and it
injects ``usage.cost`` into stream chunks (``include_cost_in_streaming_usage``) only
on its /v1/messages and pass-through routes, not on /v1/chat/completions
(proxy_server.async_data_generator). Every agent-loop and chat turn is streamed:
they were all costed 0 and budgets never stopped a run. The client prices a
stream from the per-token prices LiteLLM's /model/info reports for the model
(its cost map, wildcard routes expanded per model) when the stream carries no
cost of its own.
"""

from __future__ import annotations

import json

import httpx
import pytest

from codeforge.llm import LiteLLMClient, LLMClientConfig, ModelMetadata, _model_info_table

MODEL = "anthropic/claude-sonnet-4-20250514"


def _sse(*frames: str) -> str:
    return "".join(f"data: {frame}\n\n" for frame in frames)


def _stream(usage: dict[str, object] | None) -> str:
    frames = [json.dumps({"choices": [{"index": 0, "delta": {"content": "ok"}, "finish_reason": "stop"}]})]
    if usage is not None:
        frames.append(json.dumps({"choices": [], "usage": usage}))
    frames.append("[DONE]")
    return _sse(*frames)


def _info(rows: list[dict[str, object]]) -> dict[str, object]:
    return {"data": rows}


def _row(name: str, cost_in: object, cost_out: object) -> dict[str, object]:
    return {
        "model_name": name,
        "litellm_params": {"model": name},
        "model_info": {"input_cost_per_token": cost_in, "output_cost_per_token": cost_out},
    }


class _Proxy:
    """A LiteLLM stand-in: one stream answer and one /model/info answer; it counts /model/info requests."""

    def __init__(self, stream: str, info: object, *, info_status: int = 200, headers: dict[str, str] | None = None):
        self.stream = stream
        self.info = info
        self.info_status = info_status
        self.headers = headers or {}
        self.info_requests = 0

    def handler(self, request: httpx.Request) -> httpx.Response:
        if request.url.path == "/model/info":
            self.info_requests += 1
            return httpx.Response(self.info_status, json=self.info)
        return httpx.Response(200, text=self.stream, headers={"content-type": "text/event-stream", **self.headers})

    def client(self) -> LiteLLMClient:
        client = LiteLLMClient(base_url="http://litellm", config=LLMClientConfig(max_retries=0))
        client._client = httpx.AsyncClient(base_url="http://litellm", transport=httpx.MockTransport(self.handler))
        return client


async def _cost(proxy: _Proxy) -> float:
    response = await proxy.client().chat_completion_stream(messages=[{"role": "user", "content": "hi"}], model=MODEL)
    return response.cost_usd


async def test_a_stream_without_cost_is_priced_from_model_info() -> None:
    proxy = _Proxy(
        _stream({"prompt_tokens": 1000, "completion_tokens": 500}),
        _info([_row(MODEL, 3e-06, 1.5e-05)]),
    )

    assert await _cost(proxy) == pytest.approx(1000 * 3e-06 + 500 * 1.5e-05)
    assert proxy.info_requests == 1


async def test_a_cost_in_the_usage_chunk_wins() -> None:
    proxy = _Proxy(
        _stream({"prompt_tokens": 1000, "completion_tokens": 500, "cost": 0.042}),
        _info([_row(MODEL, 3e-06, 1.5e-05)]),
    )

    assert await _cost(proxy) == pytest.approx(0.042)
    assert proxy.info_requests == 0


async def test_a_cost_header_wins() -> None:
    proxy = _Proxy(
        _stream({"prompt_tokens": 1000, "completion_tokens": 500}),
        _info([_row(MODEL, 3e-06, 1.5e-05)]),
        headers={"x-litellm-response-cost": "0.01"},
    )

    assert await _cost(proxy) == pytest.approx(0.01)
    assert proxy.info_requests == 0


async def test_a_stream_without_tokens_costs_nothing_and_asks_nothing() -> None:
    proxy = _Proxy(_stream(None), _info([_row(MODEL, 3e-06, 1.5e-05)]))

    assert await _cost(proxy) == 0.0
    assert proxy.info_requests == 0


@pytest.mark.parametrize(
    "info",
    [
        pytest.param(_info([_row("openai/gpt-4o", 2.5e-06, 1e-05)]), id="model-not-listed"),
        pytest.param(_info([_row(MODEL, None, None)]), id="no-prices"),
    ],
)
async def test_a_model_without_prices_costs_zero(info: dict[str, object]) -> None:
    """Zero here: the caller's fallback table (pricing.resolve_cost) and its warning take over."""
    proxy = _Proxy(_stream({"prompt_tokens": 1000, "completion_tokens": 500}), info)

    assert await _cost(proxy) == 0.0


async def test_model_info_unavailable_costs_zero() -> None:
    proxy = _Proxy(_stream({"prompt_tokens": 1000, "completion_tokens": 500}), {}, info_status=500)

    assert await _cost(proxy) == 0.0


async def test_one_price_missing_prices_the_other_side() -> None:
    proxy = _Proxy(
        _stream({"prompt_tokens": 1000, "completion_tokens": 500}),
        _info([_row(MODEL, 3e-06, None)]),
    )

    assert await _cost(proxy) == pytest.approx(1000 * 3e-06)


@pytest.mark.parametrize(
    ("cost_in", "cost_out", "expected"),
    [
        pytest.param(
            3e-06, 1.5e-05, ModelMetadata(input_cost_per_token=3e-06, output_cost_per_token=1.5e-05), id="floats"
        ),
        pytest.param(0, 0, ModelMetadata(input_cost_per_token=0.0, output_cost_per_token=0.0), id="free"),
        pytest.param("3e-06", True, ModelMetadata(), id="wrong-types"),
        pytest.param(-1.0, float("nan"), ModelMetadata(), id="negative-and-nan"),
    ],
)
def test_model_info_prices(cost_in: object, cost_out: object, expected: ModelMetadata) -> None:
    assert _model_info_table(_info([_row(MODEL, cost_in, cost_out)]))[MODEL] == expected
