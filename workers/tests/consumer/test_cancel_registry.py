"""The worker's registry of task cancels (KI-65).

A tasks.cancel for a task whose message still waits in NATS for a free
worker reached no listener and was lost. Every worker records the cancels it
sees with their stream sequence: a dispatch published before the cancel is
not started, a dispatch published after it (the task sent again) is.
"""

from __future__ import annotations

import asyncio
import json

from nats.aio.msg import Msg

from codeforge.consumer._cancel_registry import CancelRegistry, record_cancels, task_key
from codeforge.consumer._delivery import stream_sequence
from tests.jetstream_fakes import FakeSubscription, jetstream_msg


class _Clock:
    def __init__(self) -> None:
        self.now = 0.0

    def __call__(self) -> float:
        return self.now


def test_a_cancel_applies_to_dispatches_published_before_it() -> None:
    registry = CancelRegistry()
    registry.record("task-1", 20)

    assert registry.cancelled("task-1", 10)
    assert registry.cancelled("task-1", 19)
    assert not registry.cancelled("task-1", 20), "a message is not published after itself"
    assert not registry.cancelled("task-1", 30), "a dispatch after the cancel runs"
    assert not registry.cancelled("task-2", 10)
    assert not registry.cancelled("", 10)


def test_the_latest_cancel_counts() -> None:
    registry = CancelRegistry()
    registry.record("task-1", 20)
    registry.record("task-1", 15)  # an older cancel delivered late

    assert registry.cancelled("task-1", 18)


def test_an_empty_task_id_is_not_recorded() -> None:
    registry = CancelRegistry()
    registry.record("", 20)

    assert len(registry) == 0


def test_the_registry_is_bounded_in_size() -> None:
    registry = CancelRegistry(max_entries=2)
    for i, task_id in enumerate(["a", "b", "c"]):
        registry.record(task_id, 100 + i)

    assert len(registry) == 2
    assert not registry.cancelled("a", 1), "the oldest cancel is forgotten first"
    assert registry.cancelled("b", 1)
    assert registry.cancelled("c", 1)


def test_cancels_expire() -> None:
    clock = _Clock()
    registry = CancelRegistry(ttl=10.0, clock=clock)
    registry.record("old", 20)
    clock.now = 6.0
    registry.record("new", 20)
    clock.now = 11.0

    assert not registry.cancelled("old", 10)
    assert registry.cancelled("new", 10)
    assert len(registry) == 1


def test_a_recorded_again_cancel_is_kept_from_its_latest_record() -> None:
    clock = _Clock()
    registry = CancelRegistry(ttl=10.0, clock=clock)
    registry.record("task-1", 20)
    clock.now = 8.0
    registry.record("task-1", 25)
    clock.now = 12.0

    assert registry.cancelled("task-1", 24)


def test_stream_sequence() -> None:
    msg, _ = jetstream_msg(b"{}", stream_seq=77)

    assert stream_sequence(msg) == 77
    assert stream_sequence(Msg(_client=None, subject="core.nats", data=b"")) is None  # type: ignore[arg-type]


async def test_record_cancels_records_valid_task_cancels() -> None:
    subscription = FakeSubscription("tasks.cancel")
    registry = CancelRegistry()
    listener = asyncio.create_task(record_cancels(subscription, registry))

    subscription.deliver(b"not json", stream_seq=5)
    subscription.deliver(json.dumps({"run_id": "run-1"}).encode(), stream_seq=6)
    subscription.deliver(json.dumps({"task_id": "task-1", "action": "cancel"}).encode(), stream_seq=7)
    subscription.deliver(json.dumps({"task_id": "task-2"}).encode())  # no stream position: cannot be ordered
    await asyncio.sleep(0.1)
    listener.cancel()
    await asyncio.wait({listener})

    assert registry.cancelled(task_key("task-1"), 6)
    assert not registry.cancelled(task_key("task-2"), 0)
    assert len(registry) == 1
