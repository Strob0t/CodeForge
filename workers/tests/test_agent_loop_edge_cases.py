"""Edge-case tests for the core agentic loop (budget, cancellation, fallback, experience cache)."""

from __future__ import annotations

import json
from dataclasses import dataclass
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.agent_loop import AgentLoopExecutor, LoopConfig
from codeforge.llm import ChatCompletionResponse, LLMError, ToolCallPart
from codeforge.models import ToolCallDecision
from codeforge.tools import ToolRegistry, ToolResult

# Mock resolve_model globally so the agent loop doesn't contact LiteLLM.
pytestmark = pytest.mark.usefixtures("_mock_resolve_model")


@pytest.fixture(autouse=True)
def _mock_resolve_model():
    with patch("codeforge.agent_loop.resolve_model", return_value="fake-model"):
        yield


# ---------------------------------------------------------------------------
# Helpers (mirror test_agent_loop.py)
# ---------------------------------------------------------------------------


@dataclass
class _FakeLLMCall:
    content: str
    tool_calls: list[ToolCallPart]
    finish_reason: str
    tokens_in: int = 10
    tokens_out: int = 5
    cost_usd: float = 0.001


class FakeLLM:
    def __init__(self, responses: list[_FakeLLMCall]) -> None:
        self._responses = list(responses)
        self._call_index = 0

    async def chat_completion_stream(self, **kwargs) -> ChatCompletionResponse:
        if self._call_index >= len(self._responses):
            return ChatCompletionResponse(
                content="(exhausted)",
                tool_calls=[],
                finish_reason="stop",
                tokens_in=0,
                tokens_out=0,
                model="fake",
                cost_usd=0,
            )
        call = self._responses[self._call_index]
        self._call_index += 1
        on_chunk = kwargs.get("on_chunk")
        if on_chunk and call.content:
            on_chunk(call.content)
        return ChatCompletionResponse(
            content=call.content,
            tool_calls=call.tool_calls,
            finish_reason=call.finish_reason,
            tokens_in=call.tokens_in,
            tokens_out=call.tokens_out,
            model="fake-model",
            cost_usd=call.cost_usd,
        )


def _make_runtime(*, allow_all: bool = True, cancelled: bool = False) -> MagicMock:
    runtime = MagicMock()
    runtime.run_id = "run-1"
    runtime.project_id = "proj-1"
    runtime.is_cancelled = cancelled
    runtime.send_output = AsyncMock()
    runtime.request_tool_call = AsyncMock(
        return_value=ToolCallDecision(
            call_id="tc-1",
            decision="allow" if allow_all else "deny",
            reason="",
        )
    )
    runtime.report_tool_result = AsyncMock()
    runtime.publish_trajectory_event = AsyncMock()
    return runtime


def _make_registry() -> ToolRegistry:
    from codeforge.tools._base import ToolDefinition

    registry = ToolRegistry()

    class _EchoTool:
        async def execute(self, arguments: dict, workspace_path: str) -> ToolResult:
            return ToolResult(output=json.dumps(arguments), success=True)

    registry.register(
        ToolDefinition(name="echo", description="Echo arguments", parameters={"type": "object"}),
        _EchoTool(),
    )
    return registry


# ---------------------------------------------------------------------------
# Budget tests
# ---------------------------------------------------------------------------


async def test_budget_depletion_mid_loop() -> None:
    """Loop stops when accumulated cost exceeds max_cost mid-loop."""
    responses = [
        _FakeLLMCall(
            content="",
            finish_reason="tool_calls",
            cost_usd=0.01,
            tool_calls=[ToolCallPart(id="c1", name="echo", arguments="{}")],
        ),
        _FakeLLMCall(
            content="",
            finish_reason="tool_calls",
            cost_usd=0.01,
            tool_calls=[ToolCallPart(id="c2", name="echo", arguments="{}")],
        ),
        _FakeLLMCall(content="should not run", tool_calls=[], finish_reason="stop", cost_usd=0.01),
    ]
    llm = FakeLLM(responses)
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/ws")
    result = await executor.run(
        [{"role": "user", "content": "test"}],
        config=LoopConfig(max_cost=0.015),
    )

    # After 2nd call, total_cost = 0.02 >= 0.015, loop should stop
    assert result.total_cost >= 0.015
    assert result.step_count == 2


async def test_budget_zero_means_unlimited() -> None:
    """max_cost=0.0 never triggers the budget limit."""
    responses = [
        _FakeLLMCall(
            content="",
            finish_reason="tool_calls",
            cost_usd=0.1,
            tool_calls=[ToolCallPart(id="c1", name="echo", arguments="{}")],
        ),
        _FakeLLMCall(content="Done", tool_calls=[], finish_reason="stop", cost_usd=0.1),
    ]
    llm = FakeLLM(responses)
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/ws")
    result = await executor.run(
        [{"role": "user", "content": "test"}],
        config=LoopConfig(max_cost=0.0),
    )

    assert result.final_content == "Done"
    assert result.error == ""


async def test_budget_exact_equals_triggers_stop() -> None:
    """When total_cost == max_cost exactly, loop stops."""
    responses = [
        _FakeLLMCall(
            content="",
            finish_reason="tool_calls",
            cost_usd=0.01,
            tool_calls=[ToolCallPart(id="c1", name="echo", arguments="{}")],
        ),
        _FakeLLMCall(content="should not appear", tool_calls=[], finish_reason="stop", cost_usd=0.01),
    ]
    llm = FakeLLM(responses)
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/ws")
    result = await executor.run(
        [{"role": "user", "content": "test"}],
        config=LoopConfig(max_cost=0.01),
    )

    # After 1st call cost=0.01 == max_cost -> stop
    assert result.step_count == 1


# ---------------------------------------------------------------------------
# Cancellation tests
# ---------------------------------------------------------------------------


async def test_cancellation_before_first_iteration() -> None:
    """When is_cancelled is True at start, 0 LLM calls are made."""
    llm = FakeLLM([_FakeLLMCall(content="Hi", tool_calls=[], finish_reason="stop")])
    runtime = _make_runtime(cancelled=True)
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/ws")
    result = await executor.run([{"role": "user", "content": "test"}])

    assert result.error == "cancelled"
    assert result.step_count == 0
    assert llm._call_index == 0


async def test_cancellation_during_tool_execution() -> None:
    """Cancel between tool calls: remaining get 'Cancelled' placeholder."""
    call_count = 0

    async def _cancel_after_first(tool: str, command: str = "", path: str = "") -> ToolCallDecision:
        nonlocal call_count
        call_count += 1
        if call_count > 1:
            runtime.is_cancelled = True
        return ToolCallDecision(call_id=f"tc-{call_count}", decision="allow", reason="")

    llm = FakeLLM(
        [
            _FakeLLMCall(
                content="",
                finish_reason="tool_calls",
                tool_calls=[
                    ToolCallPart(id="c1", name="echo", arguments="{}"),
                    ToolCallPart(id="c2", name="echo", arguments="{}"),
                    ToolCallPart(id="c3", name="echo", arguments="{}"),
                ],
            ),
        ]
    )
    runtime = _make_runtime()
    runtime.request_tool_call = AsyncMock(side_effect=_cancel_after_first)
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/ws")
    result = await executor.run([{"role": "user", "content": "test"}])

    tool_results = [m for m in result.tool_messages if m.role == "tool"]
    assert len(tool_results) == 3
    assert tool_results[2].content == "Cancelled"


# ---------------------------------------------------------------------------
# Fallback tests
# ---------------------------------------------------------------------------


async def test_llm_429_triggers_fallback() -> None:
    """LLMError(429) causes model switch to fallback."""
    call_count = 0

    async def _stream(**kwargs):
        nonlocal call_count
        call_count += 1
        if call_count == 1:
            raise LLMError(429, "anthropic/claude-sonnet-4", "rate limit exceeded")
        return ChatCompletionResponse(
            content="Fallback reply",
            tool_calls=[],
            finish_reason="stop",
            tokens_in=10,
            tokens_out=5,
            model="mistral/mistral-large-latest",
            cost_usd=0.001,
        )

    llm = MagicMock()
    llm.chat_completion_stream = AsyncMock(side_effect=_stream)
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/ws")

    with (
        patch("codeforge.agent_loop.get_tracker") as mock_tracker,
        patch("codeforge.agent_loop.get_blocklist") as mock_blocklist,
    ):
        mock_tracker.return_value = MagicMock()
        mock_blocklist.return_value = MagicMock()
        result = await executor.run(
            [{"role": "user", "content": "test"}],
            config=LoopConfig(
                model="anthropic/claude-sonnet-4",
                fallback_models=["mistral/mistral-large-latest"],
            ),
        )

    assert result.final_content == "Fallback reply"
    assert result.error == ""


async def test_all_fallbacks_exhausted() -> None:
    """When all models fail, error message is set."""

    async def _always_fail(**kwargs):
        raise LLMError(429, "model", "rate limit")

    llm = MagicMock()
    llm.chat_completion_stream = AsyncMock(side_effect=_always_fail)
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/ws")

    with (
        patch("codeforge.agent_loop.get_tracker") as mock_tracker,
        patch("codeforge.agent_loop.get_blocklist") as mock_blocklist,
    ):
        mock_tracker.return_value = MagicMock()
        mock_blocklist.return_value = MagicMock()
        result = await executor.run(
            [{"role": "user", "content": "test"}],
            config=LoopConfig(
                model="model-a",
                fallback_models=["model-b"],
            ),
        )

    assert "failed" in result.error.lower()


async def test_non_fallback_error_stops_immediately() -> None:
    """LLMError(422) without fallback keywords does not trigger fallback."""

    async def _bad_request(**kwargs):
        raise LLMError(422, "model", "invalid request format")

    llm = MagicMock()
    llm.chat_completion_stream = AsyncMock(side_effect=_bad_request)
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/ws")
    result = await executor.run(
        [{"role": "user", "content": "test"}],
        config=LoopConfig(model="model-a", fallback_models=["model-b"]),
    )

    assert "failed" in result.error.lower()
    # Model should NOT have switched to model-b
    assert "model-b" not in result.error


async def test_unexpected_exception_wrapped() -> None:
    """RuntimeError in LLM call is wrapped as LLMError(500).

    A 500 with body "timeout" matches fallback keywords, so it triggers fallback.
    """
    call_count = 0

    async def _timeout_then_ok(**kwargs):
        nonlocal call_count
        call_count += 1
        if call_count == 1:
            raise RuntimeError("Timeout on reading data from socket")
        return ChatCompletionResponse(
            content="Recovered",
            tool_calls=[],
            finish_reason="stop",
            tokens_in=10,
            tokens_out=5,
            model="model-b",
            cost_usd=0.001,
        )

    llm = MagicMock()
    llm.chat_completion_stream = AsyncMock(side_effect=_timeout_then_ok)
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/ws")

    with (
        patch("codeforge.agent_loop.get_tracker") as mock_tracker,
        patch("codeforge.agent_loop.get_blocklist") as mock_blocklist,
    ):
        mock_tracker.return_value = MagicMock()
        mock_blocklist.return_value = MagicMock()
        result = await executor.run(
            [{"role": "user", "content": "test"}],
            config=LoopConfig(
                model="model-a",
                fallback_models=["model-b"],
            ),
        )

    assert result.final_content == "Recovered"


# ---------------------------------------------------------------------------
# Tool execution edge cases
# ---------------------------------------------------------------------------


async def test_tool_execution_error_continues() -> None:
    """Tool raising an exception produces error in history, LLM continues."""
    from codeforge.tools._base import ToolDefinition

    registry = ToolRegistry()

    class _FailingTool:
        async def execute(self, arguments: dict, workspace_path: str) -> ToolResult:
            raise ValueError("tool boom")

    registry.register(
        ToolDefinition(name="fail_tool", description="Fails", parameters={"type": "object"}),
        _FailingTool(),
    )

    llm = FakeLLM(
        [
            _FakeLLMCall(
                content="",
                finish_reason="tool_calls",
                tool_calls=[ToolCallPart(id="c1", name="fail_tool", arguments="{}")],
            ),
            _FakeLLMCall(content="Handled", tool_calls=[], finish_reason="stop"),
        ]
    )
    runtime = _make_runtime()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/ws")
    result = await executor.run([{"role": "user", "content": "test"}])

    assert result.final_content == "Handled"
    tool_results = [m for m in result.tool_messages if m.role == "tool"]
    assert any("error" in tr.content.lower() for tr in tool_results)


async def test_tool_not_found_returns_error() -> None:
    """Missing tool returns 'unknown tool' error message."""
    llm = FakeLLM(
        [
            _FakeLLMCall(
                content="",
                finish_reason="tool_calls",
                tool_calls=[ToolCallPart(id="c1", name="nonexistent_tool", arguments="{}")],
            ),
            _FakeLLMCall(content="Ok", tool_calls=[], finish_reason="stop"),
        ]
    )
    runtime = _make_runtime()
    registry = _make_registry()  # Only has "echo"

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/ws")
    result = await executor.run([{"role": "user", "content": "test"}])

    tool_results = [m for m in result.tool_messages if m.role == "tool"]
    assert len(tool_results) == 1
    assert "unknown tool" in tool_results[0].content.lower()


# ---------------------------------------------------------------------------
# Experience cache tests
# ---------------------------------------------------------------------------


async def test_experience_cache_hit() -> None:
    """Experience pool hit returns cached result, 0 LLM calls."""
    llm = FakeLLM([_FakeLLMCall(content="nope", tool_calls=[], finish_reason="stop")])
    runtime = _make_runtime()
    registry = _make_registry()

    pool = MagicMock()
    pool.lookup = AsyncMock(
        return_value={
            "id": "exp-1",
            "similarity": 0.95,
            "result_output": "Cached answer",
        }
    )

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/ws", experience_pool=pool)
    result = await executor.run([{"role": "user", "content": "test query"}])

    assert result.final_content == "Cached answer"
    assert result.step_count == 0
    assert result.total_cost == 0.0
    assert llm._call_index == 0


async def test_experience_cache_miss() -> None:
    """Experience pool returns None: normal execution proceeds."""
    llm = FakeLLM([_FakeLLMCall(content="Normal reply", tool_calls=[], finish_reason="stop")])
    runtime = _make_runtime()
    registry = _make_registry()

    pool = MagicMock()
    pool.lookup = AsyncMock(return_value=None)
    pool.store = AsyncMock()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/ws", experience_pool=pool)
    result = await executor.run([{"role": "user", "content": "test query"}])

    assert result.final_content == "Normal reply"
    assert llm._call_index == 1


# ---------------------------------------------------------------------------
# Empty messages edge case
# ---------------------------------------------------------------------------


async def test_empty_messages_list() -> None:
    """Empty history still triggers the LLM call."""
    llm = FakeLLM([_FakeLLMCall(content="Hello", tool_calls=[], finish_reason="stop")])
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/ws")
    result = await executor.run([])

    assert result.final_content == "Hello"
    assert llm._call_index == 1
