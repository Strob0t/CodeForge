"""One cancel listener for runs (runs.cancel, ...) and backend tasks (tasks.cancel).

RuntimeClient and the task handler each had their own copy, and the copies had
drifted: only the task copy survived a message whose data is not bytes.
"""

from __future__ import annotations

import asyncio
import json
from types import SimpleNamespace

import pytest

from codeforge.models import TerminationConfig
from codeforge.runtime import RuntimeClient, listen_for_cancel
from tests.jetstream_fakes import FakeSubscription, RecordingJetStream

MALFORMED: list[object] = [
    b"not json",
    b"\xff\xfe",
    b"[1, 2]",
    b'"run-1"',
    json.dumps({"run_id": 5}).encode(),
    None,
    object(),
]


def _deliver_raw(sub: FakeSubscription, data: object) -> None:
    sub._incoming.put_nowait(SimpleNamespace(data=data))  # type: ignore[arg-type]


async def test_listener_skips_malformed_messages_and_stops_at_the_match() -> None:
    sub = FakeSubscription("tasks.cancel")
    for data in MALFORMED:
        _deliver_raw(sub, data)
    sub.deliver(json.dumps({"task_id": "other"}).encode())
    sub.deliver(json.dumps({"task_id": "task-1"}).encode())
    cancels: list[str] = []

    await asyncio.wait_for(
        listen_for_cancel(sub, lambda _run, task: task == "task-1", lambda: cancels.append("x"), until=lambda: False),
        timeout=2,
    )

    assert cancels == ["x"]


async def test_listener_returns_when_its_work_ended() -> None:
    sub = FakeSubscription("runs.cancel")

    await asyncio.wait_for(listen_for_cancel(sub, lambda *_: True, lambda: None, until=lambda: True), timeout=2)


async def test_listener_returns_when_the_subscription_ends() -> None:
    class _Closed(FakeSubscription):
        async def next_msg(self, timeout: float = 1.0) -> object:
            raise ConnectionError("subscription closed")

    await asyncio.wait_for(
        listen_for_cancel(_Closed("runs.cancel"), lambda *_: True, lambda: None, until=lambda: False), timeout=2
    )


@pytest.mark.parametrize("data", MALFORMED, ids=[repr(d)[:20] for d in MALFORMED])
async def test_run_listener_survives_malformed_messages(data: object) -> None:
    js = RecordingJetStream()
    runtime = RuntimeClient(
        js=js,  # type: ignore[arg-type]
        run_id="run-1",
        task_id="task-1",
        project_id="p",
        termination=TerminationConfig(),
    )
    await runtime.start_cancel_listener()
    try:
        _deliver_raw(js.subscriptions[0], data)
        js.subscriptions[0].deliver(json.dumps({"run_id": "run-1"}).encode())
        for _ in range(200):
            if runtime.is_cancelled:
                break
            await asyncio.sleep(0.01)
        assert runtime.is_cancelled
    finally:
        await runtime.close()
