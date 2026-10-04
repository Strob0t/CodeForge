"""A user's own provider key never reaches another provider on fallback (S7-F review).

The Go Core resolves the user's key for the provider of the run's model
(``ResolveProviderAPIKey``: the prefix of the model name). On a fallback the
worker switches to another model; with a user's key it may switch only to a
model of that provider, and skips the others (with a log line), so the key
is never sent to another provider. Without a user's key the fallback chain is
unchanged.
"""

from __future__ import annotations

import logging
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.agent_loop import AgentLoopExecutor, LoopConfig
from codeforge.consumer._conversation import ConversationHandlerMixin
from codeforge.llm import ChatCompletionResponse, LLMError
from codeforge.models import AgentLoopResult, ConversationRunStartMessage, ToolCallDecision
from codeforge.provider_keys import fallbacks_for_key, model_provider
from codeforge.routing import blocklist, rate_tracker
from codeforge.routing.blocklist import ModelBlocklist
from codeforge.routing.rate_tracker import RateLimitTracker
from codeforge.tools import ToolRegistry

USER_KEY = "sk-user-own-key"


@pytest.fixture(autouse=True)
def _fresh_routing_singletons(monkeypatch: pytest.MonkeyPatch) -> None:
    """Each test gets its own rate tracker and blocklist (fallbacks consult them)."""
    monkeypatch.setattr(rate_tracker, "_tracker", RateLimitTracker())
    monkeypatch.setattr(blocklist, "_blocklist", ModelBlocklist())


class _RecordingLLM:
    """Records the model and key of every call; the models in *failing* time out.

    A timeout (408) is fallback-eligible without marking the provider
    exhausted, so a model of the same provider may follow.
    """

    def __init__(self, failing: set[str]) -> None:
        self.calls: list[tuple[str, str]] = []
        self._failing = failing

    async def chat_completion_stream(self, **kwargs: object) -> ChatCompletionResponse:
        model = str(kwargs["model"])
        self.calls.append((model, str(kwargs.get("provider_api_key", ""))))
        if model in self._failing:
            raise LLMError(status_code=408, model=model, body="upstream timeout")
        return ChatCompletionResponse(
            content="done",
            tool_calls=[],
            finish_reason="stop",
            tokens_in=1,
            tokens_out=1,
            model=model,
            cost_usd=0.0,
        )


def _runtime() -> MagicMock:
    runtime = MagicMock()
    runtime.run_id = "run-key"
    runtime.is_cancelled = False
    runtime.send_output = AsyncMock()
    runtime.request_tool_call = AsyncMock(return_value=ToolCallDecision(call_id="c", decision="allow", reason=""))
    runtime.report_tool_result = AsyncMock()
    runtime.publish_trajectory_event = AsyncMock()
    return runtime


async def _run_loop(cfg: LoopConfig, failing: set[str]) -> tuple[AgentLoopResult, list[tuple[str, str]]]:
    llm = _RecordingLLM(failing)
    executor = AgentLoopExecutor(llm, ToolRegistry(), _runtime(), "/tmp/ws")
    result = await executor.run([{"role": "user", "content": "hi"}], config=cfg)
    return result, llm.calls


@pytest.mark.parametrize(
    ("model", "want"),
    [
        ("openai/gpt-4o", "openai"),
        ("groq/llama-3.1-8b", "groq"),
        ("openrouter/anthropic/claude-3", "openrouter"),
        ("gpt-4o", ""),
        ("", ""),
        ("/gpt-4o", ""),
    ],
)
def test_model_provider(model: str, want: str) -> None:
    assert model_provider(model) == want


def test_fallbacks_for_key() -> None:
    chain = ["groq/llama", "openai/gpt-4o-mini", "anthropic/claude", "openai/o3", "gpt-4o-mini"]
    assert fallbacks_for_key("openai/gpt-4o", chain) == ["openai/gpt-4o-mini", "openai/o3"]
    assert fallbacks_for_key("mistral/large", chain) == []
    # A model without a provider prefix: the key's provider is unknown.
    assert fallbacks_for_key("gpt-4o", chain) == []


async def test_same_provider_fallback_keeps_the_key() -> None:
    cfg = LoopConfig(
        model="openai/gpt-4o",
        provider_api_key=USER_KEY,
        fallback_models=["groq/llama-3.1-8b", "openai/gpt-4o-mini"],
    )

    result, calls = await _run_loop(cfg, failing={"openai/gpt-4o"})

    assert result.error == ""
    assert calls == [("openai/gpt-4o", USER_KEY), ("openai/gpt-4o-mini", USER_KEY)]


async def test_cross_provider_fallback_is_skipped_with_a_user_key(caplog: pytest.LogCaptureFixture) -> None:
    cfg = LoopConfig(
        model="openai/gpt-4o",
        provider_api_key=USER_KEY,
        fallback_models=["groq/llama-3.1-8b", "anthropic/claude-sonnet-4"],
    )

    with caplog.at_level(logging.WARNING, logger="codeforge.agent_loop"):
        result, calls = await _run_loop(cfg, failing={"openai/gpt-4o"})

    assert calls == [("openai/gpt-4o", USER_KEY)]
    assert "LLM call failed" in result.error
    assert "groq/llama-3.1-8b" in caplog.text
    assert "another provider" in caplog.text
    assert USER_KEY not in caplog.text


async def test_fallback_without_a_user_key_is_unchanged() -> None:
    cfg = LoopConfig(model="openai/gpt-4o", fallback_models=["groq/llama-3.1-8b"])

    result, calls = await _run_loop(cfg, failing={"openai/gpt-4o"})

    assert result.error == ""
    assert calls == [("openai/gpt-4o", ""), ("groq/llama-3.1-8b", "")]


async def test_model_without_provider_prefix_does_not_fall_back_with_a_key() -> None:
    cfg = LoopConfig(model="gpt-4o", provider_api_key=USER_KEY, fallback_models=["gpt-4o-mini"])

    result, calls = await _run_loop(cfg, failing={"gpt-4o"})

    assert calls == [("gpt-4o", USER_KEY)]
    assert "LLM call failed" in result.error


# --- The conversation run hands only same-provider fallbacks to every path ---


def _run_msg(*, agentic: bool, key: str) -> ConversationRunStartMessage:
    return ConversationRunStartMessage(
        run_id="run-key",
        conversation_id="conv-key",
        project_id="proj-1",
        messages=[],
        system_prompt="You are a helper.",
        model="openai/gpt-4o",
        agentic=agentic,
        workspace_path="/tmp/ws",
        provider_api_key=key,
    )


async def _execute(run_msg: ConversationRunStartMessage, fallbacks: list[str]) -> MagicMock:
    mixin = type("_TestMixin", (ConversationHandlerMixin,), {})()
    mixin._execute_litellm_loop = AsyncMock(return_value=AgentLoopResult(final_content="ok"))
    mixin._run_simple_chat = AsyncMock(return_value=AgentLoopResult(final_content="ok"))
    await mixin._execute_conversation_run(
        run_msg=run_msg,
        messages=[{"role": "user", "content": "hi"}],
        primary_model=run_msg.model,
        routing=MagicMock(),
        runtime=_runtime(),
        registry=MagicMock(),
        fallback_models=fallbacks,
    )
    return mixin


@pytest.mark.parametrize("agentic", [True, False])
async def test_conversation_run_with_a_user_key_keeps_same_provider_fallbacks(agentic: bool) -> None:
    mixin = await _execute(_run_msg(agentic=agentic, key=USER_KEY), ["groq/llama", "openai/gpt-4o-mini"])

    called = mixin._execute_litellm_loop if agentic else mixin._run_simple_chat
    called.assert_awaited_once()
    assert called.await_args.kwargs.get("fallback_models", called.await_args.args[-1]) == ["openai/gpt-4o-mini"]


@pytest.mark.parametrize("agentic", [True, False])
async def test_conversation_run_without_a_user_key_keeps_every_fallback(agentic: bool) -> None:
    mixin = await _execute(_run_msg(agentic=agentic, key=""), ["groq/llama", "openai/gpt-4o-mini"])

    called = mixin._execute_litellm_loop if agentic else mixin._run_simple_chat
    called.assert_awaited_once()
    assert called.await_args.kwargs.get("fallback_models", called.await_args.args[-1]) == [
        "groq/llama",
        "openai/gpt-4o-mini",
    ]
