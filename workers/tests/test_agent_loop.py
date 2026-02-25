"""Tests for the core agentic loop (Phase 17E.1)."""

from __future__ import annotations

import json
from dataclasses import dataclass
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.agent_loop import AgentLoopExecutor, LoopConfig
from codeforge.llm import ChatCompletionResponse, ToolCallPart
from codeforge.models import ToolCallDecision
from codeforge.tools import ToolRegistry, ToolResult


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
                model="fake",
                cost_usd=0,
            )
        call = self._responses[self._call_index]
        self._call_index += 1
        # Fire on_chunk if provided.
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


# --- Tests ---


async def test_single_turn_no_tools() -> None:
    """LLM responds with text only, no tool calls."""
    llm = FakeLLM([_FakeLLMCall(content="Hello!", tool_calls=[], finish_reason="stop")])
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run([{"role": "user", "content": "Hi"}])

    assert result.final_content == "Hello!"
    assert result.tool_messages == []
    assert result.step_count == 0
    assert result.error == ""
    runtime.send_output.assert_called_once_with("Hello!")


async def test_single_tool_call() -> None:
    """LLM calls a tool, gets result, then responds with text."""
    llm = FakeLLM(
        [
            _FakeLLMCall(
                content="",
                tool_calls=[ToolCallPart(id="c1", name="echo", arguments='{"msg": "test"}')],
                finish_reason="tool_calls",
            ),
            _FakeLLMCall(content="Done!", tool_calls=[], finish_reason="stop"),
        ]
    )
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run([{"role": "user", "content": "Do something"}])

    assert result.final_content == "Done!"
    assert result.step_count == 1
    # Should have assistant (with tool_calls) + tool result.
    assert len(result.tool_messages) == 2
    assert result.tool_messages[0].role == "assistant"
    assert result.tool_messages[1].role == "tool"
    assert result.tool_messages[1].tool_call_id == "c1"


async def test_multi_tool_calls() -> None:
    """LLM calls multiple tools in sequence."""
    llm = FakeLLM(
        [
            _FakeLLMCall(
                content="",
                tool_calls=[
                    ToolCallPart(id="c1", name="echo", arguments='{"a": 1}'),
                    ToolCallPart(id="c2", name="echo", arguments='{"b": 2}'),
                ],
                finish_reason="tool_calls",
            ),
            _FakeLLMCall(content="All done.", tool_calls=[], finish_reason="stop"),
        ]
    )
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run([{"role": "user", "content": "test"}])

    assert result.step_count == 2
    # 1 assistant msg + 2 tool results.
    assert len(result.tool_messages) == 3
    assert result.final_content == "All done."


async def test_tool_permission_denied() -> None:
    """When a tool call is denied, LLM gets denial message and continues."""
    call_count = 0

    async def _request_tool_call(tool: str, command: str = "", path: str = "") -> ToolCallDecision:
        nonlocal call_count
        call_count += 1
        if tool == "echo":
            return ToolCallDecision(call_id="tc-deny", decision="deny", reason="not allowed")
        return ToolCallDecision(call_id=f"tc-{call_count}", decision="allow", reason="")

    llm = FakeLLM(
        [
            _FakeLLMCall(
                content="",
                tool_calls=[ToolCallPart(id="c1", name="echo", arguments="{}")],
                finish_reason="tool_calls",
            ),
            _FakeLLMCall(content="I see it was denied.", tool_calls=[], finish_reason="stop"),
        ]
    )
    runtime = _make_runtime()
    runtime.request_tool_call = AsyncMock(side_effect=_request_tool_call)
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run([{"role": "user", "content": "test"}])

    assert result.final_content == "I see it was denied."
    # The denied tool result should be in the messages.
    tool_results = [m for m in result.tool_messages if m.role == "tool"]
    assert len(tool_results) == 1
    assert "denied" in tool_results[0].content.lower()


async def test_max_steps_termination() -> None:
    """Loop should stop after max_iterations."""
    # LLM always returns tool calls, never stops.
    responses = [
        _FakeLLMCall(
            content="",
            tool_calls=[ToolCallPart(id=f"c{i}", name="echo", arguments="{}")],
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

    # Should stop at 3 iterations.
    assert result.step_count == 3


async def test_cancellation() -> None:
    """Loop should stop when runtime is cancelled."""
    llm = FakeLLM(
        [
            _FakeLLMCall(content="I should not appear", tool_calls=[], finish_reason="stop"),
        ]
    )
    runtime = _make_runtime(cancelled=True)
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run([{"role": "user", "content": "test"}])

    assert result.error == "cancelled"


async def test_llm_denied() -> None:
    """If the LLM call itself is denied, loop returns immediately."""
    llm = FakeLLM([])
    runtime = _make_runtime(allow_all=False)
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run([{"role": "user", "content": "test"}])

    assert "denied" in result.error.lower()


async def test_cost_accumulation() -> None:
    """Cost and tokens should be accumulated across iterations."""
    llm = FakeLLM(
        [
            _FakeLLMCall(
                content="",
                tool_calls=[ToolCallPart(id="c1", name="echo", arguments="{}")],
                finish_reason="tool_calls",
                tokens_in=100,
                tokens_out=50,
                cost_usd=0.01,
            ),
            _FakeLLMCall(
                content="Done",
                tool_calls=[],
                finish_reason="stop",
                tokens_in=200,
                tokens_out=100,
                cost_usd=0.02,
            ),
        ]
    )
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run([{"role": "user", "content": "test"}])

    assert result.total_tokens_in == 300
    assert result.total_tokens_out == 150
    assert result.total_cost == pytest.approx(0.03)


async def test_unknown_tool() -> None:
    """Unknown tool should return error result, LLM continues."""
    llm = FakeLLM(
        [
            _FakeLLMCall(
                content="",
                tool_calls=[ToolCallPart(id="c1", name="nonexistent", arguments="{}")],
                finish_reason="tool_calls",
            ),
            _FakeLLMCall(content="Ok I see the error.", tool_calls=[], finish_reason="stop"),
        ]
    )
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    result = await executor.run([{"role": "user", "content": "test"}])

    # The tool result should contain an error but the loop should continue.
    assert result.final_content == "Ok I see the error."
    tool_results = [m for m in result.tool_messages if m.role == "tool"]
    assert len(tool_results) == 1
    assert "unknown tool" in tool_results[0].content.lower()
