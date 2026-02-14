"""Tests for the consumer message handling."""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.consumer import TaskConsumer
from codeforge.models import TaskMessage, TaskResult, TaskStatus


@pytest.fixture
def consumer() -> TaskConsumer:
    """Create a TaskConsumer for testing."""
    return TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")


async def test_handle_message_success(consumer: TaskConsumer) -> None:
    """_handle_message should parse, execute, publish result, and ack."""
    task_json = TaskMessage(
        id="task-1",
        project_id="proj-1",
        title="Test task",
        prompt="Do something",
    ).model_dump_json()

    msg = MagicMock()
    msg.data = task_json.encode()
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()

    expected_result = TaskResult(
        task_id="task-1",
        status=TaskStatus.COMPLETED,
        output="Done",
    )

    consumer._js = AsyncMock()
    consumer._executor = MagicMock()
    consumer._executor.execute = AsyncMock(return_value=expected_result)

    await consumer._handle_message(msg)

    consumer._executor.execute.assert_called_once()
    consumer._js.publish.assert_called_once()
    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


async def test_handle_message_invalid_json(consumer: TaskConsumer) -> None:
    """_handle_message should nack on invalid JSON."""
    msg = MagicMock()
    msg.data = b"not valid json"
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()

    consumer._js = AsyncMock()

    await consumer._handle_message(msg)

    msg.nak.assert_called_once()
    msg.ack.assert_not_called()


async def test_handle_message_executor_failure(consumer: TaskConsumer) -> None:
    """_handle_message should still ack after executor returns a FAILED result."""
    task_json = TaskMessage(
        id="task-2",
        project_id="proj-1",
        title="Failing task",
        prompt="This will fail",
    ).model_dump_json()

    msg = MagicMock()
    msg.data = task_json.encode()
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()

    failed_result = TaskResult(
        task_id="task-2",
        status=TaskStatus.FAILED,
        error="LLM timeout",
    )

    consumer._js = AsyncMock()
    consumer._executor = MagicMock()
    consumer._executor.execute = AsyncMock(return_value=failed_result)

    await consumer._handle_message(msg)

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()
