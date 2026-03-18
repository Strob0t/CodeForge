"""Tests for A2A mixin using execute_a2a_task instead of generic execute.

TDD RED phase: Verifies the A2A handler mixin delegates to the
dedicated execute_a2a_task method on AgentExecutor.
"""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.a2a_protocol import A2ATaskState
from codeforge.consumer._a2a import A2AHandlerMixin
from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.models import A2ATaskCreatedMessage, TaskResult, TaskStatus


class _TestMixin(A2AHandlerMixin, ConsumerBaseMixin):
    def __init__(self) -> None:
        self._js: AsyncMock | None = AsyncMock()
        self._processed_ids: set[str] = set()
        self._processed_ids_max = 10_000
        self._executor = MagicMock()


@pytest.fixture(autouse=True)
def _fresh_state() -> None:
    ConsumerBaseMixin._processed_ids = set()


def _make_msg(data: dict) -> MagicMock:
    msg = MagicMock()
    msg.data = json.dumps(data).encode()
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.headers = {}
    return msg


def _a2a_payload(
    task_id: str = "a2a-1",
    skill_id: str = "code-task",
    prompt: str = "Write hello world",
) -> dict:
    return A2ATaskCreatedMessage(
        task_id=task_id,
        tenant_id="tenant-1",
        skill_id=skill_id,
        prompt=prompt,
    ).model_dump()


# ---------------------------------------------------------------------------
# Tests: A2A mixin should use execute_a2a_task
# ---------------------------------------------------------------------------


async def test_a2a_mixin_calls_execute_a2a_task() -> None:
    """A2A mixin should call executor.execute_a2a_task, not generic execute."""
    mixin = _TestMixin()
    mixin._executor.execute_a2a_task = AsyncMock(
        return_value=TaskResult(
            task_id="a2a-1",
            status=TaskStatus.COMPLETED,
            output="done",
        ),
    )
    msg = _make_msg(_a2a_payload())

    await mixin._handle_a2a_task_created(msg)

    mixin._executor.execute_a2a_task.assert_called_once()
    call_kwargs = mixin._executor.execute_a2a_task.call_args.kwargs
    assert call_kwargs["task_id"] == "a2a-1"
    assert call_kwargs["skill_id"] == "code-task"
    assert call_kwargs["prompt"] == "Write hello world"


async def test_a2a_mixin_publishes_working_then_completed() -> None:
    """A2A mixin should publish WORKING state, then COMPLETED on success."""
    mixin = _TestMixin()
    mixin._executor.execute_a2a_task = AsyncMock(
        return_value=TaskResult(
            task_id="a2a-2",
            status=TaskStatus.COMPLETED,
            output="result",
        ),
    )
    msg = _make_msg(_a2a_payload(task_id="a2a-2"))

    await mixin._handle_a2a_task_created(msg)

    assert mixin._js is not None
    assert mixin._js.publish.call_count >= 2
    # First call: WORKING
    first_payload = json.loads(mixin._js.publish.call_args_list[0].args[1])
    assert first_payload["state"] == A2ATaskState.WORKING
    # Last call: COMPLETED
    last_payload = json.loads(mixin._js.publish.call_args_list[-1].args[1])
    assert last_payload["state"] == A2ATaskState.COMPLETED


async def test_a2a_mixin_publishes_failed_on_error() -> None:
    """A2A mixin should publish FAILED state when executor returns failed."""
    mixin = _TestMixin()
    mixin._executor.execute_a2a_task = AsyncMock(
        return_value=TaskResult(
            task_id="a2a-3",
            status=TaskStatus.FAILED,
            error="timeout",
        ),
    )
    msg = _make_msg(_a2a_payload(task_id="a2a-3"))

    await mixin._handle_a2a_task_created(msg)

    assert mixin._js is not None
    last_payload = json.loads(mixin._js.publish.call_args_list[-1].args[1])
    assert last_payload["state"] == A2ATaskState.FAILED


async def test_a2a_mixin_publishes_failed_on_exception() -> None:
    """A2A mixin should publish FAILED when executor raises."""
    mixin = _TestMixin()
    mixin._executor.execute_a2a_task = AsyncMock(side_effect=RuntimeError("crash"))
    msg = _make_msg(_a2a_payload(task_id="a2a-4"))

    await mixin._handle_a2a_task_created(msg)

    assert mixin._js is not None
    last_payload = json.loads(mixin._js.publish.call_args_list[-1].args[1])
    assert last_payload["state"] == A2ATaskState.FAILED
    assert "crash" in last_payload.get("error", "")
    msg.ack.assert_called_once()
