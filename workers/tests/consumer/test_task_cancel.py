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
from codeforge.consumer._cancel_registry import task_key
from codeforge.models import TaskMessage
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
    worker._notifications = worker._js
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

    assert config.ack_policy == AckPolicy.NONE  # type: ignore[union-attr]


async def test_the_task_listens_for_cancels_from_its_dispatch_on(consumer: TaskConsumer, backend: _Backend) -> None:
    """S2-F review, F12: a cancel published just before the task's listener subscribed was lost.

    The listener saw new messages only, and the cancel registry had not
    processed it yet. The listener replays every cancel published after
    the task's dispatch message (same stream), so none falls in between.
    """
    handler = await _start(consumer, backend)  # dispatched at stream sequence 10

    config = _cancel_subscription(consumer).config
    backend.release.set()
    await asyncio.wait_for(handler, timeout=2)

    assert config.deliver_policy == DeliverPolicy.BY_START_SEQUENCE  # type: ignore[union-attr]
    assert config.opt_start_seq == 11  # type: ignore[union-attr]


async def test_a_replayed_cancel_stops_the_task(consumer: TaskConsumer, backend: _Backend) -> None:
    """A cancel published between the dispatch and the listener's subscription reaches the task."""
    js = consumer._js
    subscribe = js.subscribe  # type: ignore[union-attr]

    async def subscribe_with_replay(subject: str, config: object = None) -> FakeSubscription:
        sub = await subscribe(subject, config=config)
        if subject == "tasks.cancel":
            sub.deliver(json.dumps({"task_id": "task-1"}).encode(), stream_seq=15)
        return sub

    js.subscribe = subscribe_with_replay  # type: ignore[union-attr,method-assign]
    msg, _ = jetstream_msg(_payload(), subject="tasks.agent.aider", stream_seq=10)

    await asyncio.wait_for(consumer._handle_message(msg), timeout=2)

    assert _results(consumer) == [("task-1", "cancelled")]


async def test_a_task_without_stream_position_listens_for_new_cancels(
    consumer: TaskConsumer, backend: _Backend
) -> None:
    backend.release.set()
    task = TaskMessage.model_validate_json(_payload())

    await consumer._run_backend(task, "aider", "", dispatch=None)

    config = _cancel_subscription(consumer).config
    assert config.deliver_policy == DeliverPolicy.NEW  # type: ignore[union-attr]


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


# ---------------------------------------------------------------------------
# A cancel for a task still queued in NATS (KI-65)
# ---------------------------------------------------------------------------


async def test_a_task_cancelled_while_queued_is_not_started(consumer: TaskConsumer, backend: _Backend) -> None:
    """The cancel was published after the task's dispatch: the task never runs, Go already marked it cancelled."""
    consumer._cancels.record(task_key("task-1"), 20)
    msg, client = jetstream_msg(_payload(), subject="tasks.agent.aider", stream_seq=10)

    await consumer._handle_message(msg)

    assert not backend.started.is_set()
    assert client.settlements() == ["ack"]
    assert _results(consumer) == [], "no result: it could overwrite the state of a later dispatch of the task"


async def test_a_dispatch_after_the_cancel_runs(consumer: TaskConsumer, backend: _Backend) -> None:
    """A task dispatched again after it was cancelled (and after an earlier dispatch of it ran) is not skipped."""
    backend.release.set()
    first, _ = jetstream_msg(_payload(), subject="tasks.agent.aider", stream_seq=10)
    await consumer._handle_message(first)
    consumer._cancels.record(task_key("task-1"), 20)

    again, client = jetstream_msg(_payload(), subject="tasks.agent.aider", stream_seq=30)
    await asyncio.wait_for(consumer._handle_message(again), timeout=2)

    assert client.settlements() == ["ack(sync)"]
    assert _results(consumer) == [("task-1", "completed"), ("task-1", "completed")]


async def test_a_cancel_arriving_before_the_task_listens_stops_it(consumer: TaskConsumer, backend: _Backend) -> None:
    """A cancel recorded after the queued-cancel check but before the task's own listener subscribed is not lost."""
    js = consumer._js
    subscribe = js.subscribe  # type: ignore[union-attr]

    async def subscribe_after_cancel(subject: str, config: object = None) -> FakeSubscription:
        if subject == "tasks.cancel":
            consumer._cancels.record(task_key("task-1"), 20)
        return await subscribe(subject, config=config)

    js.subscribe = subscribe_after_cancel  # type: ignore[union-attr,method-assign]
    msg, _ = jetstream_msg(_payload(), subject="tasks.agent.aider", stream_seq=10)

    await asyncio.wait_for(consumer._handle_message(msg), timeout=2)

    assert not backend.started.is_set()
    assert _results(consumer) == [("task-1", "cancelled")]


async def test_the_worker_records_every_task_cancel(consumer: TaskConsumer) -> None:
    await consumer._start_cancel_registry()
    subscription = _cancel_subscription(consumer)
    subscription.deliver(json.dumps({"task_id": "task-9"}).encode(), stream_seq=42)
    await asyncio.sleep(0.05)

    assert consumer._cancels.cancelled(task_key("task-9"), 41)
    assert subscription.config.deliver_policy == DeliverPolicy.NEW  # type: ignore[union-attr]
    assert subscription.config.ack_policy == AckPolicy.NONE  # type: ignore[union-attr]

    await consumer._in_flight.abort([], 0, "test over")
    assert subscription.unsubscribed
