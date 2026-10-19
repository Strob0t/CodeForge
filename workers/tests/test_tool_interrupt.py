"""A run's cancel interrupts the tool that runs, and no tool process survives it (KI-194, R8-6).

A cancel (the Stop button, tasks.cancel) only set a flag the agent loop
checked between tool calls: a long bash command ran to its own timeout, and
the next turn could already run while it kept changing the workspace.
"""

from __future__ import annotations

import asyncio
import json
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.agent_loop import _LoopState
from codeforge.llm import ToolCallPart
from codeforge.models import ToolCallDecision
from codeforge.tool_executor import ToolExecutor
from codeforge.tools import ToolRegistry, ToolResult
from codeforge.tools._base import ToolDefinition
from codeforge.tools.bash import DEFINITION as BASH
from codeforge.tools.bash import BashTool
from tests.processes import SPAWN, gone, spawned

if TYPE_CHECKING:
    from pathlib import Path


def _runtime() -> MagicMock:
    runtime = MagicMock()
    runtime.run_id = "run-1"
    runtime.is_cancelled = False
    runtime.request_tool_call = AsyncMock(return_value=ToolCallDecision(call_id="c1", decision="allow"))
    runtime.report_tool_result = AsyncMock()
    runtime.publish_trajectory_event = AsyncMock()
    return runtime


class _SlowTool:
    """A tool that runs until it is interrupted, and records that it was."""

    def __init__(self) -> None:
        self.started = asyncio.Event()
        self.interrupted = False

    async def execute(self, _arguments: dict[str, object], _workspace_path: str) -> ToolResult:
        self.started.set()
        try:
            await asyncio.sleep(60)
        except asyncio.CancelledError:
            self.interrupted = True
            raise
        return ToolResult(output="finished")


def _registry(slow: _SlowTool | None = None) -> ToolRegistry:
    registry = ToolRegistry()
    registry.register(BASH, BashTool())
    if slow is not None:
        registry.register(ToolDefinition(name="slow", description="slow", parameters={"type": "object"}), slow)
    return registry


async def test_a_run_cancel_interrupts_the_running_tool() -> None:
    slow = _SlowTool()
    runtime = _runtime()
    messages: list[dict[str, object]] = []
    call = asyncio.create_task(
        ToolExecutor(_registry(slow), runtime, "/tmp/ws").execute(
            ToolCallPart(id="t1", name="slow", arguments="{}"), messages, _LoopState()
        )
    )
    await asyncio.wait_for(slow.started.wait(), 5)

    runtime.is_cancelled = True
    await asyncio.wait_for(call, 5)

    assert slow.interrupted
    assert messages[-1]["role"] == "tool"
    assert "cancelled" in str(messages[-1]["content"]).lower()
    reported = runtime.report_tool_result.await_args.kwargs
    assert reported["success"] is False
    assert "cancelled" in reported["error"].lower()


async def test_a_run_cancel_leaves_no_bash_process(tmp_path: Path) -> None:
    runtime = _runtime()
    arguments = json.dumps({"command": SPAWN, "timeout": 120})
    call = asyncio.create_task(
        ToolExecutor(_registry(), runtime, str(tmp_path)).execute(
            ToolCallPart(id="t1", name="bash", arguments=arguments), [], _LoopState()
        )
    )
    shell, child = await spawned(tmp_path)

    runtime.is_cancelled = True
    await asyncio.wait_for(call, 5)

    assert await gone(shell), "the command survived the Stop"
    assert await gone(child), "a process the command started survived the Stop"


async def test_a_cancelled_run_task_leaves_no_bash_process(tmp_path: Path) -> None:
    """The run's wall clock (wait_for) and a worker abort cancel the whole task."""
    arguments = json.dumps({"command": SPAWN, "timeout": 120})
    call = asyncio.create_task(
        ToolExecutor(_registry(), _runtime(), str(tmp_path)).execute(
            ToolCallPart(id="t1", name="bash", arguments=arguments), [], _LoopState()
        )
    )
    shell, child = await spawned(tmp_path)

    call.cancel()
    with pytest.raises(asyncio.CancelledError):
        await call

    assert await gone(shell)
    assert await gone(child), "a process the command started survived the cancel"


async def test_a_tool_that_finishes_is_not_interrupted() -> None:
    runtime = _runtime()
    messages: list[dict[str, object]] = []

    await ToolExecutor(_registry(), runtime, "/tmp").execute(
        ToolCallPart(id="t1", name="bash", arguments=json.dumps({"command": "echo done"})), messages, _LoopState()
    )

    assert "done" in str(messages[-1]["content"])
    assert runtime.report_tool_result.await_args.kwargs["success"] is True
