"""Tests for the A2A task handler mixin."""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.a2a_protocol import A2ATaskState
from codeforge.consumer._a2a import A2AHandlerMixin
from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._subjects import SUBJECT_A2A_TASK_COMPLETE
from codeforge.models import A2ATaskCreatedMessage

# ---------------------------------------------------------------------------
# Test harness
# ---------------------------------------------------------------------------


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


def _a2a_payload(task_id: str = "a2a-1") -> dict:
    return A2ATaskCreatedMessage(
        task_id=task_id,
        tenant_id="tenant-1",
        skill_id="code",
        prompt="Write hello world",
    ).model_dump()


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


async def test_a2a_task_created_success() -> None:
    """Successful task publishes WORKING then COMPLETED states."""
    from codeforge.backends._base import TaskResult
    from codeforge.models import TaskStatus

    mixin = _TestMixin()
    mixin._executor.execute = AsyncMock(return_value=TaskResult(status=TaskStatus.COMPLETED, output="done"))
    msg = _make_msg(_a2a_payload())

    await mixin._handle_a2a_task_created(msg)

    assert mixin._js is not None
    # Should have published at least 2 messages: WORKING + COMPLETED
    assert mixin._js.publish.call_count >= 2
    subjects = [c.args[0] for c in mixin._js.publish.call_args_list]
    assert all(s == SUBJECT_A2A_TASK_COMPLETE for s in subjects)

    # Last publish should be completed
    last_payload = json.loads(mixin._js.publish.call_args_list[-1].args[1])
    assert last_payload["state"] == A2ATaskState.COMPLETED
    msg.ack.assert_called_once()


async def test_a2a_executor_failure_publishes_failed() -> None:
    """When executor returns failed status, FAILED is published."""
    from codeforge.backends._base import TaskResult
    from codeforge.models import TaskStatus

    mixin = _TestMixin()
    mixin._executor.execute = AsyncMock(return_value=TaskResult(status=TaskStatus.FAILED, error="timeout"))
    msg = _make_msg(_a2a_payload())

    await mixin._handle_a2a_task_created(msg)

    assert mixin._js is not None
    last_payload = json.loads(mixin._js.publish.call_args_list[-1].args[1])
    assert last_payload["state"] == A2ATaskState.FAILED
    msg.ack.assert_called_once()


async def test_a2a_exception_publishes_failure() -> None:
    """When executor raises, failure completion is published and msg is acked."""
    mixin = _TestMixin()
    mixin._executor.execute = AsyncMock(side_effect=RuntimeError("boom"))
    msg = _make_msg(_a2a_payload())

    await mixin._handle_a2a_task_created(msg)

    assert mixin._js is not None
    # The exception handler should publish a FAILED result
    last_payload = json.loads(mixin._js.publish.call_args_list[-1].args[1])
    assert last_payload["state"] == A2ATaskState.FAILED
    assert "boom" in last_payload.get("error", "")
    msg.ack.assert_called_once()


async def test_a2a_duplicate_skipped() -> None:
    """Duplicate task_id is acked but not executed a second time."""
    from codeforge.backends._base import TaskResult
    from codeforge.models import TaskStatus

    mixin = _TestMixin()
    mixin._executor.execute = AsyncMock(return_value=TaskResult(status=TaskStatus.COMPLETED, output="ok"))
    msg1 = _make_msg(_a2a_payload("dup-1"))
    msg2 = _make_msg(_a2a_payload("dup-1"))

    await mixin._handle_a2a_task_created(msg1)
    await mixin._handle_a2a_task_created(msg2)

    # Executor called only once
    assert mixin._executor.execute.call_count == 1
    msg2.ack.assert_called_once()


async def test_a2a_trust_stamped() -> None:
    """Completed payload has trust annotation."""
    from codeforge.backends._base import TaskResult
    from codeforge.models import TaskStatus

    mixin = _TestMixin()
    mixin._executor.execute = AsyncMock(return_value=TaskResult(status=TaskStatus.COMPLETED, output="ok"))
    msg = _make_msg(_a2a_payload())

    await mixin._handle_a2a_task_created(msg)

    assert mixin._js is not None
    last_payload = json.loads(mixin._js.publish.call_args_list[-1].args[1])
    assert "trust" in last_payload


async def test_a2a_invalid_json_acks() -> None:
    """Invalid JSON data is caught and message is acked."""
    mixin = _TestMixin()
    msg = MagicMock()
    msg.data = b"{{bad json"
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.headers = {}

    await mixin._handle_a2a_task_created(msg)

    msg.ack.assert_called_once()


async def test_a2a_cancel_acks() -> None:
    """Cancel handler acks the message."""
    mixin = _TestMixin()
    msg = _make_msg({"task_id": "cancel-1"})

    await mixin._handle_a2a_task_cancel(msg)

    msg.ack.assert_called_once()


async def test_a2a_cancel_invalid_json_acks() -> None:
    """Cancel handler with invalid JSON still acks."""
    mixin = _TestMixin()
    msg = MagicMock()
    msg.data = b"not json"
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.headers = {}

    await mixin._handle_a2a_task_cancel(msg)

    msg.ack.assert_called_once()


async def test_a2a_no_js_handling() -> None:
    """When _js is None, executor still runs but no publish occurs."""
    from codeforge.backends._base import TaskResult
    from codeforge.models import TaskStatus

    mixin = _TestMixin()
    mixin._js = None
    mixin._executor.execute = AsyncMock(return_value=TaskResult(status=TaskStatus.COMPLETED, output="ok"))
    msg = _make_msg(_a2a_payload())

    await mixin._handle_a2a_task_created(msg)

    mixin._executor.execute.assert_called_once()
    msg.ack.assert_called_once()
