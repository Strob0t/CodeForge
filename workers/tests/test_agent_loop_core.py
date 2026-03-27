"""Tests for AgentLoopExecutor core loop outcomes.

Validates the five critical loop behaviors:
- Max iteration enforcement
- Cost budget enforcement
- Cancellation mid-loop
- Normal completion (LLM returns stop)
- Tool call flow (LLM -> tool -> result -> next iteration)
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.agent_loop import AgentLoopExecutor, LoopConfig
from codeforge.llm import ChatCompletionResponse, ToolCallPart
from codeforge.models import ToolCallDecision
from codeforge.tools import ToolRegistry, ToolResult

# Mock resolve_model globally for all tests in this module.
pytestmark = pytest.mark.usefixtures("_mock_resolve_model")


@pytest.fixture(autouse=True)
def _mock_resolve_model():
    with patch("codeforge.agent_loop.resolve_model", return_value="fake-model"):
        yield


# ---------------------------------------------------------------------------
# Helpers (same pattern as test_agent_loop_events.py)
# ---------------------------------------------------------------------------


@dataclass
class _FakeLLMCall:
    """Represents a single planned LLM response."""

    content: str
    tool_calls: list[ToolCallPart]
    finish_reason: str
    tokens_in: int = 10
    tokens_out: int = 5
    cost_usd: float = 0.001


class FakeLLM:
    """Fake LLM client that returns pre-programmed responses."""

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
                model="fake-model",
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
    """Build a mock RuntimeClient."""
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
    """Build a ToolRegistry with a simple echo tool."""
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
# 1. Normal completion: LLM returns stop -> loop completes successfully
# ---------------------------------------------------------------------------


async def test_normal_completion_returns_final_content() -> None:
    """When LLM returns finish_reason=stop, the loop ends with final_content set."""
    llm = FakeLLM(
        [
            _FakeLLMCall(content="Hello, world!", tool_calls=[], finish_reason="stop"),
        ]
    )
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run([{"role": "user", "content": "greet me"}])

    assert result.final_content == "Hello, world!"
    assert result.error == ""
    assert result.step_count == 0


async def test_normal_completion_accumulates_tokens() -> None:
    """Token counts from the LLM response are accumulated in the result."""
    llm = FakeLLM(
        [
            _FakeLLMCall(
                content="answer",
                tool_calls=[],
                finish_reason="stop",
                tokens_in=100,
                tokens_out=50,
                cost_usd=0.005,
            ),
        ]
    )
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run([{"role": "user", "content": "test"}])

    assert result.total_tokens_in == 100
    assert result.total_tokens_out == 50
    assert result.total_cost > 0


# ---------------------------------------------------------------------------
# 2. Max iteration enforcement
# ---------------------------------------------------------------------------


async def test_max_iteration_stops_loop() -> None:
    """When max_iterations is reached, the loop stops with an error."""
    # Create LLM that always requests tool calls (never stops voluntarily)
    responses = [
        _FakeLLMCall(
            content="",
            tool_calls=[ToolCallPart(id=f"c{i}", name="echo", arguments='{"n": 1}')],
            finish_reason="tool_calls",
        )
        for i in range(10)
    ]
    llm = FakeLLM(responses)
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run(
        [{"role": "user", "content": "loop forever"}],
        config=LoopConfig(max_iterations=3),
    )

    assert "iteration limit" in result.error
    assert result.step_count == 3  # exactly 3 tool calls


async def test_max_iteration_one() -> None:
    """max_iterations=1 allows exactly one iteration."""
    llm = FakeLLM(
        [
            _FakeLLMCall(
                content="",
                tool_calls=[ToolCallPart(id="c1", name="echo", arguments='{"x": 1}')],
                finish_reason="tool_calls",
            ),
        ]
    )
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run(
        [{"role": "user", "content": "once"}],
        config=LoopConfig(max_iterations=1),
    )

    assert "iteration limit" in result.error
    assert result.step_count == 1


# ---------------------------------------------------------------------------
# 3. Cost budget enforcement
# ---------------------------------------------------------------------------


async def test_cost_budget_stops_loop() -> None:
    """When total cost exceeds max_cost, the loop breaks after that iteration."""
    # Each call costs 0.01; budget is 0.015 so the second call should trigger stop
    responses = [
        _FakeLLMCall(
            content="",
            tool_calls=[ToolCallPart(id="c1", name="echo", arguments='{"a": 1}')],
            finish_reason="tool_calls",
            cost_usd=0.01,
        ),
        _FakeLLMCall(
            content="",
            tool_calls=[ToolCallPart(id="c2", name="echo", arguments='{"a": 2}')],
            finish_reason="tool_calls",
            cost_usd=0.01,
        ),
        # Third call should NOT be reached
        _FakeLLMCall(content="should not reach", tool_calls=[], finish_reason="stop"),
    ]
    llm = FakeLLM(responses)
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run(
        [{"role": "user", "content": "expensive task"}],
        config=LoopConfig(max_cost=0.015, max_iterations=50),
    )

    # Loop should have stopped due to cost limit
    assert result.total_cost >= 0.015
    # Should not have executed the third LLM call
    assert result.final_content != "should not reach"


async def test_zero_cost_budget_means_unlimited() -> None:
    """max_cost=0 (default) means no cost limit."""
    llm = FakeLLM(
        [
            _FakeLLMCall(
                content="done",
                tool_calls=[],
                finish_reason="stop",
                cost_usd=100.0,
            ),
        ]
    )
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run(
        [{"role": "user", "content": "test"}],
        config=LoopConfig(max_cost=0.0),
    )

    assert result.error == ""
    assert result.final_content == "done"


# ---------------------------------------------------------------------------
# 4. Cancellation mid-loop
# ---------------------------------------------------------------------------


async def test_cancellation_stops_loop_gracefully() -> None:
    """When runtime.is_cancelled becomes True, the loop stops with 'cancelled' error."""
    call_count = 0

    async def _fake_stream(**kwargs) -> ChatCompletionResponse:
        nonlocal call_count
        call_count += 1
        # After first call, mark as cancelled
        runtime.is_cancelled = True
        return ChatCompletionResponse(
            content="",
            tool_calls=[ToolCallPart(id="c1", name="echo", arguments='{"x": 1}')],
            finish_reason="tool_calls",
            tokens_in=10,
            tokens_out=5,
            model="fake-model",
            cost_usd=0.001,
        )

    runtime = _make_runtime()
    runtime.is_cancelled = False
    llm = MagicMock()
    llm.chat_completion_stream = AsyncMock(side_effect=_fake_stream)
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run(
        [{"role": "user", "content": "test"}],
        config=LoopConfig(max_iterations=50),
    )

    assert result.error == "cancelled"


async def test_cancellation_before_first_iteration() -> None:
    """If runtime is already cancelled before loop starts, it stops immediately."""
    llm = FakeLLM(
        [
            _FakeLLMCall(content="should not run", tool_calls=[], finish_reason="stop"),
        ]
    )
    runtime = _make_runtime(cancelled=True)
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run([{"role": "user", "content": "test"}])

    assert result.error == "cancelled"
    assert result.step_count == 0


# ---------------------------------------------------------------------------
# 5. Tool call flow: LLM -> tool -> result -> next iteration
# ---------------------------------------------------------------------------


async def test_tool_call_flow_single_tool() -> None:
    """LLM requests tool -> tool executes -> result appended -> LLM stops."""
    llm = FakeLLM(
        [
            _FakeLLMCall(
                content="",
                tool_calls=[ToolCallPart(id="c1", name="echo", arguments='{"msg": "hi"}')],
                finish_reason="tool_calls",
            ),
            _FakeLLMCall(content="All done.", tool_calls=[], finish_reason="stop"),
        ]
    )
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run([{"role": "user", "content": "test"}])

    assert result.final_content == "All done."
    assert result.step_count == 1
    assert result.error == ""
    assert len(result.tool_messages) >= 1


async def test_tool_call_flow_multiple_iterations() -> None:
    """Multiple rounds of tool calls before final stop."""
    llm = FakeLLM(
        [
            _FakeLLMCall(
                content="",
                tool_calls=[ToolCallPart(id="c1", name="echo", arguments='{"step": 1}')],
                finish_reason="tool_calls",
                tokens_in=20,
                tokens_out=10,
            ),
            _FakeLLMCall(
                content="",
                tool_calls=[ToolCallPart(id="c2", name="echo", arguments='{"step": 2}')],
                finish_reason="tool_calls",
                tokens_in=30,
                tokens_out=15,
            ),
            _FakeLLMCall(
                content="Finished!",
                tool_calls=[],
                finish_reason="stop",
                tokens_in=10,
                tokens_out=5,
            ),
        ]
    )
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run([{"role": "user", "content": "multi-step"}])

    assert result.final_content == "Finished!"
    assert result.step_count == 2
    assert result.total_tokens_in == 60  # 20 + 30 + 10
    assert result.total_tokens_out == 30  # 10 + 15 + 5
    assert result.error == ""


async def test_tool_denied_continues_loop() -> None:
    """When a tool call is denied, the loop continues and LLM can still stop."""
    call_count = 0

    async def _alternating_decision(tool: str, command: str = "", path: str = "") -> ToolCallDecision:
        nonlocal call_count
        call_count += 1
        if tool == "echo":
            return ToolCallDecision(call_id=f"tc-{call_count}", decision="deny", reason="blocked")
        return ToolCallDecision(call_id=f"tc-{call_count}", decision="allow", reason="")

    llm = FakeLLM(
        [
            _FakeLLMCall(
                content="",
                tool_calls=[ToolCallPart(id="c1", name="echo", arguments='{"x": 1}')],
                finish_reason="tool_calls",
            ),
            _FakeLLMCall(content="Recovered.", tool_calls=[], finish_reason="stop"),
        ]
    )
    runtime = _make_runtime()
    runtime.request_tool_call = AsyncMock(side_effect=_alternating_decision)
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run(
        [{"role": "user", "content": "test"}],
        config=LoopConfig(max_iterations=10),
    )

    assert result.final_content == "Recovered."
    assert result.error == ""


async def test_unknown_tool_returns_error_result() -> None:
    """When LLM requests a tool that doesn't exist, it gets an error result but loop continues."""
    llm = FakeLLM(
        [
            _FakeLLMCall(
                content="",
                tool_calls=[ToolCallPart(id="c1", name="nonexistent_tool", arguments="{}")],
                finish_reason="tool_calls",
            ),
            _FakeLLMCall(content="I recovered.", tool_calls=[], finish_reason="stop"),
        ]
    )
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run([{"role": "user", "content": "test"}])

    assert result.final_content == "I recovered."
    assert result.step_count == 1
    assert result.error == ""


async def test_tool_exception_does_not_crash_loop() -> None:
    """When a tool raises an exception, the loop continues gracefully."""
    from codeforge.tools._base import ToolDefinition

    registry = ToolRegistry()

    class _FailingTool:
        async def execute(self, arguments: dict, workspace_path: str) -> ToolResult:
            raise RuntimeError("kaboom")

    registry.register(
        ToolDefinition(name="fail_tool", description="Always fails", parameters={"type": "object"}),
        _FailingTool(),
    )

    llm = FakeLLM(
        [
            _FakeLLMCall(
                content="",
                tool_calls=[ToolCallPart(id="c1", name="fail_tool", arguments="{}")],
                finish_reason="tool_calls",
            ),
            _FakeLLMCall(content="recovered", tool_calls=[], finish_reason="stop"),
        ]
    )
    runtime = _make_runtime()
    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run([{"role": "user", "content": "test"}])

    assert result.final_content == "recovered"
    assert result.error == ""
    assert result.step_count == 1
