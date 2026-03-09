"""Tests for trajectory event publishing in the agent loop."""

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


def _extract_events(runtime: MagicMock, event_type: str) -> list[dict]:
    """Extract trajectory events of a specific type from the mock's call history."""
    events: list[dict] = []
    for call in runtime.publish_trajectory_event.call_args_list:
        evt = call.kwargs.get("event") or (call.args[0] if call.args else {})
        if isinstance(evt, dict) and evt.get("event_type") == event_type:
            events.append(evt)
    return events


# --- Tests ---


async def test_tool_called_event_published() -> None:
    """After a tool executes, a trajectory event with event_type=tool_called is published."""
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
    await executor.run([{"role": "user", "content": "test"}])

    tool_events = _extract_events(runtime, "agent.tool_called")
    assert len(tool_events) >= 1, (
        f"Expected at least 1 tool_called event, got {runtime.publish_trajectory_event.call_args_list}"
    )
    evt = tool_events[0]
    assert evt["tool_name"] == "echo"
    assert evt["success"] is True
    assert "duration_ms" in evt
    assert "timestamp" in evt
    assert evt["step"] == 1


async def test_step_done_event_published() -> None:
    """After each LLM call, a step_done trajectory event is published."""
    llm = FakeLLM([_FakeLLMCall(content="answer", tool_calls=[], finish_reason="stop")])
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    await executor.run(
        [{"role": "user", "content": "hello"}],
        config=LoopConfig(model="test-model"),
    )

    step_events = _extract_events(runtime, "agent.step_done")
    assert len(step_events) >= 1
    evt = step_events[0]
    assert evt["model"] == "fake-model"
    assert evt["tokens_in"] == 10
    assert evt["tokens_out"] == 5
    assert "cost_usd" in evt
    assert "timestamp" in evt


async def test_finished_event_published() -> None:
    """When the loop completes, a finished trajectory event is published."""
    llm = FakeLLM([_FakeLLMCall(content="done", tool_calls=[], finish_reason="stop")])
    runtime = _make_runtime()
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    await executor.run(
        [{"role": "user", "content": "test"}],
        config=LoopConfig(model="test-model"),
    )

    finished_events = _extract_events(runtime, "agent.finished")
    assert len(finished_events) == 1
    evt = finished_events[0]
    assert evt["step_count"] == 0
    assert evt["total_cost"] >= 0
    assert "timestamp" in evt
    assert evt["final_content_length"] == 4  # len("done")
    assert evt["error"] is None


async def test_tool_denied_event_published() -> None:
    """When a tool call is denied, a tool_called event with success=False is published."""
    call_count = 0

    async def _request_tool_call(tool: str, command: str = "", path: str = "") -> ToolCallDecision:
        nonlocal call_count
        call_count += 1
        if tool == "echo":
            return ToolCallDecision(call_id=f"tc-{call_count}", decision="deny", reason="blocked")
        return ToolCallDecision(call_id=f"tc-{call_count}", decision="allow", reason="")

    llm = FakeLLM(
        [
            _FakeLLMCall(
                content="",
                tool_calls=[ToolCallPart(id="c1", name="echo", arguments='{"cmd": "bad"}')],
                finish_reason="tool_calls",
            ),
            _FakeLLMCall(content="ok", tool_calls=[], finish_reason="stop"),
        ]
    )
    runtime = _make_runtime()
    runtime.request_tool_call = AsyncMock(side_effect=_request_tool_call)
    registry = _make_registry()

    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    await executor.run(
        [{"role": "user", "content": "test"}],
        config=LoopConfig(model="test-model", max_iterations=5),
    )

    tool_events = _extract_events(runtime, "agent.tool_called")
    assert len(tool_events) >= 1
    denied_evt = tool_events[0]
    assert denied_evt["success"] is False
    assert denied_evt["tool_name"] == "echo"
    assert denied_evt["duration_ms"] == 0


async def test_tool_exception_event_published() -> None:
    """When a tool raises an exception, a tool_called event with success=False is published."""
    from codeforge.tools._base import ToolDefinition

    registry = ToolRegistry()

    class _FailingTool:
        async def execute(self, arguments: dict, workspace_path: str) -> ToolResult:
            raise RuntimeError("boom")

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
    await executor.run([{"role": "user", "content": "test"}])

    tool_events = _extract_events(runtime, "agent.tool_called")
    assert len(tool_events) >= 1
    evt = tool_events[0]
    assert evt["success"] is False
    assert evt["tool_name"] == "fail_tool"
    assert "duration_ms" in evt
    assert evt["duration_ms"] >= 0


async def test_multiple_events_across_iterations() -> None:
    """A multi-iteration loop publishes step_done, tool_called, and finished events."""
    llm = FakeLLM(
        [
            _FakeLLMCall(
                content="",
                tool_calls=[ToolCallPart(id="c1", name="echo", arguments='{"a": 1}')],
                finish_reason="tool_calls",
                tokens_in=100,
                tokens_out=50,
                cost_usd=0.01,
            ),
            _FakeLLMCall(
                content="All done.",
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
    await executor.run([{"role": "user", "content": "test"}])

    step_events = _extract_events(runtime, "agent.step_done")
    tool_events = _extract_events(runtime, "agent.tool_called")
    finished_events = _extract_events(runtime, "agent.finished")

    assert len(step_events) == 2  # One per LLM call
    assert len(tool_events) == 1  # One tool call
    assert len(finished_events) == 1

    # Verify finished event accumulates totals
    fin = finished_events[0]
    assert fin["step_count"] == 1
    assert fin["total_tokens_in"] == 300
    assert fin["total_tokens_out"] == 150
    assert fin["final_content_length"] == len("All done.")


async def test_input_output_truncated() -> None:
    """Tool call input and output are truncated to 500 chars in trajectory events."""
    from codeforge.tools._base import ToolDefinition

    registry = ToolRegistry()

    class _LongOutputTool:
        async def execute(self, arguments: dict, workspace_path: str) -> ToolResult:
            return ToolResult(output="x" * 1000, success=True)

    registry.register(
        ToolDefinition(name="long_tool", description="Long output", parameters={"type": "object"}),
        _LongOutputTool(),
    )

    long_args = json.dumps({"data": "y" * 1000})
    llm = FakeLLM(
        [
            _FakeLLMCall(
                content="",
                tool_calls=[ToolCallPart(id="c1", name="long_tool", arguments=long_args)],
                finish_reason="tool_calls",
            ),
            _FakeLLMCall(content="done", tool_calls=[], finish_reason="stop"),
        ]
    )
    runtime = _make_runtime()
    executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
    await executor.run([{"role": "user", "content": "test"}])

    tool_events = _extract_events(runtime, "agent.tool_called")
    assert len(tool_events) >= 1
    evt = tool_events[0]
    assert len(evt["input"]) <= 500
    assert len(evt["output"]) <= 500
