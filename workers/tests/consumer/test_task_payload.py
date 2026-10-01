"""tasks.agent.* carries the backend and the project workspace (KI-23).

Before, the Go Core sent the domain task without either, so every backend ran
with workspace_path "" (in the worker's own working directory).
"""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.backends._base import TaskResult as BackendTaskResult
from codeforge.consumer import TaskConsumer
from tests.jetstream_fakes import RecordingJetStream, jetstream_msg


@pytest.fixture
def consumer() -> TaskConsumer:
    worker = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    worker._js = RecordingJetStream()  # type: ignore[assignment]
    worker._notifications = worker._js
    worker._backend_router = MagicMock()
    worker._backend_router.execute = AsyncMock(return_value=BackendTaskResult(status="completed", output="ok"))
    return worker


def _go_payload(**fields: object) -> bytes:
    """A tasks.agent payload as the Go Core publishes it (messagequeue.TaskAgentPayload)."""
    payload: dict[str, object] = {
        "task_id": "task-1",
        "project_id": "proj-1",
        "tenant_id": "tenant-1",
        "agent_id": "agent-1",
        "title": "Fix bug",
        "prompt": "fix the null pointer",
        "backend": "aider",
        "workspace_path": "/data/workspaces/proj-1",
    }
    payload.update(fields)
    return json.dumps(payload).encode()


async def test_backend_runs_in_the_payload_workspace(consumer: TaskConsumer) -> None:
    msg, _ = jetstream_msg(_go_payload(), subject="tasks.agent.aider")

    await consumer._handle_message(msg)

    kwargs = consumer._backend_router.execute.call_args.kwargs
    assert kwargs["workspace_path"] == "/data/workspaces/proj-1"
    assert kwargs["backend_name"] == "aider"
    assert kwargs["task_id"] == "task-1"


async def test_payload_backend_names_the_backend(consumer: TaskConsumer) -> None:
    """The payload names the backend; the subject suffix is only the routing key."""
    msg, _ = jetstream_msg(_go_payload(backend="goose"), subject="tasks.agent.goose")

    await consumer._handle_message(msg)

    assert consumer._backend_router.execute.call_args.kwargs["backend_name"] == "goose"


async def test_payload_without_backend_uses_the_subject(consumer: TaskConsumer) -> None:
    """A task published before the upgrade (domain task JSON: "id", no backend) still runs."""
    legacy = json.dumps(
        {"id": "task-1", "project_id": "proj-1", "title": "t", "prompt": "p", "status": "queued", "version": 1}
    ).encode()
    msg, _ = jetstream_msg(legacy, subject="tasks.agent.opencode")

    await consumer._handle_message(msg)

    kwargs = consumer._backend_router.execute.call_args.kwargs
    assert kwargs["backend_name"] == "opencode"
    assert kwargs["task_id"] == "task-1"
    assert kwargs["workspace_path"] == ""
