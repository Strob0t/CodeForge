"""Coroutines resolve a default model in a worker thread, never on the event loop (KI-196, R9-7).

resolve_model() and get_available_models() refresh their list with blocking
httpx calls to LiteLLM's /health and /v1/models (5 s timeout each). Called on
the event loop they froze it while LiteLLM was slow or down: heartbeats,
in-progress acks and every other run's stream stalled.
"""

from __future__ import annotations

import json
import threading
import time
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock

import httpx
import pytest

from codeforge import model_resolver
from codeforge.agent_loop import AgentLoopExecutor, LoopConfig
from codeforge.llm import ChatCompletionResponse, LiteLLMClient, LLMClientConfig
from codeforge.models import ToolCallDecision
from codeforge.tools import ToolRegistry

if TYPE_CHECKING:
    from collections.abc import Callable


class _Recorder:
    """A stand-in for a resolver: it returns *value* and records the threads it ran in."""

    def __init__(self, value: str) -> None:
        self.value = value
        self.threads: list[int] = []

    def __call__(self, *_args: object) -> str:
        self.threads.append(threading.get_ident())
        return self.value


def _handler(request: httpx.Request) -> httpx.Response:
    if json.loads(request.content).get("stream"):
        frame = json.dumps({"choices": [{"index": 0, "delta": {"content": "ok"}, "finish_reason": "stop"}]})
        return httpx.Response(200, text=f"data: {frame}\n\ndata: [DONE]\n\n")
    return httpx.Response(200, json={"choices": [{"message": {"content": "ok"}, "finish_reason": "stop"}]})


def _client() -> LiteLLMClient:
    client = LiteLLMClient(base_url="http://litellm", config=LLMClientConfig(max_retries=0))
    client._client = httpx.AsyncClient(base_url="http://litellm", transport=httpx.MockTransport(_handler))
    return client


_MESSAGES: list[dict[str, object]] = [{"role": "user", "content": "hi"}]

_CALLS: dict[str, Callable[[LiteLLMClient], object]] = {
    "completion": lambda c: c.completion(prompt="hi"),
    "chat_completion": lambda c: c.chat_completion(messages=_MESSAGES),
    "chat_completion_stream": lambda c: c.chat_completion_stream(messages=_MESSAGES),
}


@pytest.fixture
def stale_cache(monkeypatch: pytest.MonkeyPatch) -> None:
    """The model cache is due for a refresh (blocking HTTP calls to LiteLLM); no default model is set."""
    monkeypatch.delenv("CODEFORGE_DEFAULT_MODEL", raising=False)
    monkeypatch.setattr(model_resolver._cache, "_last_refresh", 0.0)
    monkeypatch.setattr(model_resolver._cache, "_retry_at", 0.0)


@pytest.fixture
def fresh_cache(monkeypatch: pytest.MonkeyPatch) -> None:
    """The model cache was just refreshed: reading it costs no thread (KI-196 review)."""
    monkeypatch.delenv("CODEFORGE_DEFAULT_MODEL", raising=False)
    monkeypatch.setattr(model_resolver._cache, "_last_refresh", time.monotonic())

    async def no_thread(*_args: object, **_kwargs: object) -> object:
        raise AssertionError("a fresh cache is read on the event loop, without a worker thread")

    monkeypatch.setattr(model_resolver.asyncio, "to_thread", no_thread)


@pytest.mark.usefixtures("stale_cache")
@pytest.mark.parametrize("call", list(_CALLS))
async def test_the_llm_client_resolves_the_default_model_off_the_loop(
    call: str, monkeypatch: pytest.MonkeyPatch
) -> None:
    recorder = _Recorder("openai/gpt-4o")
    monkeypatch.setattr("codeforge.model_resolver.resolve_model", recorder)

    response = await _CALLS[call](_client())  # type: ignore[misc]

    assert response.model == "openai/gpt-4o"
    assert recorder.threads, "the default model was resolved"
    assert threading.get_ident() not in recorder.threads


async def test_the_agent_loop_resolves_the_default_model_off_the_loop(monkeypatch: pytest.MonkeyPatch) -> None:
    recorder = _Recorder("openai/gpt-4o")
    monkeypatch.setattr("codeforge.agent_loop.resolve_model", recorder)
    llm = MagicMock()
    llm.chat_completion_stream = AsyncMock(
        return_value=ChatCompletionResponse(
            content="done", tool_calls=[], finish_reason="stop", tokens_in=1, tokens_out=1, model="openai/gpt-4o"
        )
    )
    runtime = MagicMock()
    runtime.run_id = "run-1"
    runtime.is_cancelled = False
    runtime.send_output = AsyncMock()
    runtime.request_tool_call = AsyncMock(return_value=ToolCallDecision(call_id="c", decision="allow"))
    runtime.report_tool_result = AsyncMock()
    runtime.publish_trajectory_event = AsyncMock()

    await AgentLoopExecutor(llm, ToolRegistry(), runtime, "/tmp/ws").run(_MESSAGES, LoopConfig(model=""))

    assert recorder.threads
    assert threading.get_ident() not in recorder.threads
    assert llm.chat_completion_stream.await_args.kwargs["model"] == "openai/gpt-4o"


async def test_the_skill_safety_check_resolves_its_model_off_the_loop(monkeypatch: pytest.MonkeyPatch) -> None:
    from codeforge.skills import safety

    recorder = _Recorder("")
    monkeypatch.setattr(safety, "resolve_skill_selection_model", recorder)

    result = await safety.check_skill_safety("print('hi')", MagicMock())

    assert result.safe is False  # no model: fails closed, as before
    assert recorder.threads
    assert threading.get_ident() not in recorder.threads


@pytest.mark.usefixtures("fresh_cache")
@pytest.mark.parametrize("call", list(_CALLS))
async def test_a_fresh_cache_is_read_on_the_loop(call: str, monkeypatch: pytest.MonkeyPatch) -> None:
    """KI-196 review: every call took a default-executor thread, also when the cache was fresh."""
    recorder = _Recorder("openai/gpt-4o")
    monkeypatch.setattr("codeforge.model_resolver.resolve_model", recorder)

    response = await _CALLS[call](_client())  # type: ignore[misc]

    assert response.model == "openai/gpt-4o"
    assert recorder.threads == [threading.get_ident()]


@pytest.mark.usefixtures("stale_cache")
async def test_the_benchmark_verifier_model_is_resolved_off_the_loop(monkeypatch: pytest.MonkeyPatch) -> None:
    from codeforge.consumer._benchmark import _verifier_model

    recorder = _Recorder("openai/gpt-4o")
    monkeypatch.setattr("codeforge.model_resolver.resolve_model", recorder)

    assert await _verifier_model("auto") == "openai/gpt-4o"
    assert recorder.threads
    assert threading.get_ident() not in recorder.threads


async def test_the_benchmark_routes_off_the_loop() -> None:
    """HybridRouter.route is synchronous: it may resolve a model and call the meta-router's LLM."""
    from codeforge.consumer._benchmark import _RoutingLLMWrapper

    routed: list[int] = []

    def route(_prompt: str) -> object:
        routed.append(threading.get_ident())
        return MagicMock(model="openai/gpt-4o", routing_layer="complexity", reasoning="")

    llm = MagicMock()
    llm.chat_completion = AsyncMock(return_value="ok")
    wrapper = _RoutingLLMWrapper(llm, MagicMock(route=route))

    await wrapper.chat_completion(messages=[{"role": "user", "content": "fix the bug"}])

    assert routed
    assert threading.get_ident() not in routed
    assert llm.chat_completion.await_args.kwargs["model"] == "openai/gpt-4o"


@pytest.mark.usefixtures("stale_cache")
async def test_the_benchmark_fallback_model_is_resolved_off_the_loop(monkeypatch: pytest.MonkeyPatch) -> None:
    from codeforge.consumer._benchmark import _RoutingLLMWrapper

    recorder = _Recorder("openai/gpt-4o-mini")
    monkeypatch.setattr("codeforge.model_resolver.resolve_model", recorder)
    llm = MagicMock()
    llm.chat_completion_stream = AsyncMock(return_value="ok")
    wrapper = _RoutingLLMWrapper(llm, MagicMock(route=MagicMock(return_value=None)))

    await wrapper.chat_completion_stream(messages=[{"role": "user", "content": "fix the bug"}])

    assert recorder.threads
    assert threading.get_ident() not in recorder.threads
    assert wrapper.routing_log[-1]["model"] == "openai/gpt-4o-mini"
