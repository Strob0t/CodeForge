"""Tests for the consumer message handling."""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.consumer import TaskConsumer
from codeforge.models import ContextEntry, RunStartMessage, TaskMessage, TaskResult, TaskStatus


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
    msg.headers = {"X-Request-ID": "req-abc-123"}
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
    # Two publishes: one output line ("Starting task: ...") + one result
    assert consumer._js.publish.call_count == 2
    subjects = [call.args[0] for call in consumer._js.publish.call_args_list]
    assert "tasks.output" in subjects
    assert "tasks.result" in subjects
    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


async def test_handle_message_invalid_json(consumer: TaskConsumer) -> None:
    """_handle_message should nack on invalid JSON."""
    msg = MagicMock()
    msg.data = b"not valid json"
    msg.headers = None
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
    msg.headers = None
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


async def test_handle_message_request_id_propagated(consumer: TaskConsumer) -> None:
    """_handle_message should propagate request_id from NATS headers to output publishes."""
    task_json = TaskMessage(
        id="task-3",
        project_id="proj-1",
        title="ID test",
        prompt="Check request ID",
    ).model_dump_json()

    msg = MagicMock()
    msg.data = task_json.encode()
    msg.headers = {"X-Request-ID": "req-propagated-456"}
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()

    result = TaskResult(task_id="task-3", status=TaskStatus.COMPLETED, output="OK")

    consumer._js = AsyncMock()
    consumer._executor = MagicMock()
    consumer._executor.execute = AsyncMock(return_value=result)

    await consumer._handle_message(msg)

    # The output publish should include headers with the request ID
    output_call = consumer._js.publish.call_args_list[0]
    assert output_call.kwargs.get("headers") == {"X-Request-ID": "req-propagated-456"}


async def test_handle_run_start_with_context(consumer: TaskConsumer) -> None:
    """_handle_run_start should enrich the prompt with context entries."""
    run_msg = RunStartMessage(
        run_id="run-1",
        task_id="task-1",
        project_id="proj-1",
        agent_id="agent-1",
        prompt="Fix the login bug",
        context=[
            ContextEntry(kind="file", path="src/auth.py", content="def login(): pass", tokens=5, priority=80),
            ContextEntry(kind="shared", path="", content="step-1 completed OK", tokens=5, priority=90),
        ],
    )
    msg = MagicMock()
    msg.data = run_msg.model_dump_json().encode()
    msg.headers = None
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()

    consumer._js = AsyncMock()
    consumer._executor = MagicMock()
    consumer._executor.execute_with_runtime = AsyncMock()

    await consumer._handle_run_start(msg)

    # Verify executor was called with enriched prompt
    call_args = consumer._executor.execute_with_runtime.call_args
    task_arg = call_args.args[0]
    assert "--- Relevant Context ---" in task_arg.prompt
    assert "src/auth.py" in task_arg.prompt
    assert "def login(): pass" in task_arg.prompt
    assert "step-1 completed OK" in task_arg.prompt
    msg.ack.assert_called_once()


async def test_handle_run_start_without_context(consumer: TaskConsumer) -> None:
    """_handle_run_start should use raw prompt when no context entries present."""
    run_msg = RunStartMessage(
        run_id="run-2",
        task_id="task-2",
        project_id="proj-1",
        agent_id="agent-1",
        prompt="Refactor utils module",
    )
    msg = MagicMock()
    msg.data = run_msg.model_dump_json().encode()
    msg.headers = None
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()

    consumer._js = AsyncMock()
    consumer._executor = MagicMock()
    consumer._executor.execute_with_runtime = AsyncMock()

    await consumer._handle_run_start(msg)

    call_args = consumer._executor.execute_with_runtime.call_args
    task_arg = call_args.args[0]
    assert task_arg.prompt == "Refactor utils module"
    assert "--- Relevant Context ---" not in task_arg.prompt
    msg.ack.assert_called_once()
