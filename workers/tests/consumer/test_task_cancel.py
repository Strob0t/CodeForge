"""tasks.cancel stops a running backend task and reports it cancelled (KI-22).

Every Go backend's Stop() publishes tasks.cancel, but no worker path stopped
the Aider/OpenHands/Goose/OpenCode/Plandex process: the task ran on.
"""

from __future__ import annotations

import asyncio
import json
from unittest.mock import MagicMock

import pytest
from nats.js.api import AckPolicy, DeliverPolicy

from codeforge.backends._base import TaskResult as BackendTaskResult
from codeforge.consumer import TaskConsumer
from tests.jetstream_fakes import FakeSubscription, RecordingJetStream, jetstream_msg


class _Backend:
    """A backend run that lasts until it is cancelled or released."""

    def __init__(self) -> None:
        self.started = asyncio.Event()
        self.release = asyncio.Event()
        self.cancelled = False

    async def __call__(self, **_kwargs: object) -> BackendTaskResult:
        self.started.set()
        try:
            await self.release.wait()
        except asyncio.CancelledError:
            self.cancelled = True
            raise
        return BackendTaskResult(status="completed", output="done")


@pytest.fixture
def backend() -> _Backend:
    return _Backend()


@pytest.fixture
def consumer(backend: _Backend) -> TaskConsumer:
    worker = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    worker._js = RecordingJetStream()  # type: ignore[assignment]
    worker._backend_router = MagicMock()
    worker._backend_router.execute = backend
    return worker


def _payload(task_id: str = "task-1") -> bytes:
    return json.dumps(
        {
            "task_id": task_id,
            "project_id": "proj-1",
            "title": "t",
            "prompt": "p",
            "backend": "aider",
            "workspace_path": "/data/workspaces/proj-1",
        }
    ).encode()


def _results(worker: TaskConsumer) -> list[tuple[str, str]]:
    published = worker._js.published  # type: ignore[union-attr]
    return [(r["task_id"], r["status"]) for r in (json.loads(d) for s, d in published if s == "tasks.result")]


def _cancel_subscription(worker: TaskConsumer) -> FakeSubscription:
    subs = [s for s in worker._js.subscriptions if s.subject == "tasks.cancel"]  # type: ignore[union-attr]
    assert len(subs) == 1
    return subs[0]


async def _start(worker: TaskConsumer, backend: _Backend) -> asyncio.Task[None]:
    msg, _ = jetstream_msg(_payload(), subject="tasks.agent.aider")
    handler = asyncio.create_task(worker._handle_message(msg))
    await asyncio.wait_for(backend.started.wait(), timeout=2)
    return handler


async def test_cancel_stops_the_backend_and_reports_cancelled(consumer: TaskConsumer, backend: _Backend) -> None:
    handler = await _start(consumer, backend)

    _cancel_subscription(consumer).deliver(json.dumps({"task_id": "task-1", "action": "cancel"}).encode())
    await asyncio.wait_for(handler, timeout=2)

    assert backend.cancelled, "the backend run (and with it its process) must be stopped"
    assert _results(consumer) == [("task-1", "cancelled")]
    assert _cancel_subscription(consumer).unsubscribed


@pytest.mark.parametrize(
    "message",
    [
        json.dumps({"task_id": "task-2"}).encode(),
        json.dumps({"task_id": ""}).encode(),
        b"not json",
        b"[1, 2]",
    ],
    ids=["other task", "empty id", "invalid json", "not an object"],
)
async def test_other_cancel_messages_do_not_stop_the_task(
    consumer: TaskConsumer, backend: _Backend, message: bytes
) -> None:
    handler = await _start(consumer, backend)

    subscription = _cancel_subscription(consumer)
    subscription.deliver(message)
    await asyncio.sleep(0.05)
    backend.release.set()
    await asyncio.wait_for(handler, timeout=2)

    assert not backend.cancelled
    assert _results(consumer) == [("task-1", "completed")]
    assert subscription.unsubscribed


async def test_cancel_subscription_needs_no_acks(consumer: TaskConsumer, backend: _Backend) -> None:
    """Every worker sees every cancel (the task may run on any of them); nothing is acked."""
    handler = await _start(consumer, backend)

    config = _cancel_subscription(consumer).config
    backend.release.set()
    await asyncio.wait_for(handler, timeout=2)

    assert config.deliver_policy == DeliverPolicy.NEW  # type: ignore[union-attr]
    assert config.ack_policy == AckPolicy.NONE  # type: ignore[union-attr]


async def test_worker_abort_stops_the_backend_without_reporting_cancelled(
    consumer: TaskConsumer, backend: _Backend
) -> None:
    """A handler cancelled by the worker's abort is not a user cancel: the in-flight registry fails it."""
    handler = await _start(consumer, backend)

    handler.cancel()
    with pytest.raises(asyncio.CancelledError):
        await handler

    assert backend.cancelled
    assert _results(consumer) == []
    assert _cancel_subscription(consumer).unsubscribed
