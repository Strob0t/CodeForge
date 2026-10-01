"""The worker's notification consumers (codeforge.notifications, KI-71).

The worker may create only consumers whose name is in the JetStream API
subject, so runs and tasks no longer create an ephemeral consumer each: one
named push consumer per notification subject delivers into the worker's
inbox space, and the hub hands each message to every listener of its
subject. These tests use fakes; test_nats_permissions.py runs the hub
against a real nats-server with the production permissions.
"""

from __future__ import annotations

import json
from collections.abc import Awaitable, Callable
from unittest.mock import AsyncMock

import nats.errors
import pytest
from nats.aio.msg import Msg
from nats.js.api import AckPolicy, ConsumerConfig, DeliverPolicy, RawStreamMsg
from nats.js.errors import NotFoundError, ServerError

from codeforge.consumer import TaskConsumer
from codeforge.consumer._delivery import stream_sequence
from codeforge.nats_subjects import INBOX_PREFIX, STREAM_NAME
from codeforge.notifications import (
    NOTIFICATION_SUBJECTS,
    NotificationHub,
    notification_consumer_name,
    notification_deliver_subject,
)
from codeforge.runtime import notification_consumer

Callback = Callable[[Msg], Awaitable[None]]


class _Client:
    """Records the hub's core NATS subscriptions."""

    def __init__(self) -> None:
        self.callbacks: dict[str, Callback] = {}

    async def subscribe(self, subject: str, cb: Callback) -> None:
        self.callbacks[subject] = cb

    async def deliver(self, subject: str, data: dict[str, str], seq: int) -> None:
        """Deliver a message of *subject* the way its notification consumer does."""
        reply = f"$JS.ACK.{STREAM_NAME}.{notification_consumer_name(subject)}.1.{seq}.{seq}.0.0"
        msg = Msg(_client=self, subject=subject, reply=reply, data=json.dumps(data).encode())  # type: ignore[arg-type]
        await self.callbacks[notification_deliver_subject(subject)](msg)


class _JetStream:
    """Records consumer changes and serves the stream's messages by subject."""

    def __init__(self, stream: list[tuple[str, dict[str, str]]] | None = None, refuse_create: int = 0) -> None:
        self.stream = stream or []
        self.created: list[ConsumerConfig] = []
        self.deleted: list[str] = []
        self.refuse_create = refuse_create

    async def add_consumer(self, stream: str, config: ConsumerConfig) -> None:
        assert stream == STREAM_NAME
        if self.refuse_create > 0:
            self.refuse_create -= 1
            raise ServerError(code=500, err_code=10012, description="ack policy can not be updated")
        self.created.append(config)

    async def delete_consumer(self, stream: str, name: str) -> None:
        assert stream == STREAM_NAME
        self.deleted.append(name)

    async def get_msg(self, stream: str, seq: int, subject: str, next: bool) -> RawStreamMsg:  # noqa: A002
        assert stream == STREAM_NAME
        assert next
        for position, (stored_subject, data) in enumerate(self.stream, start=1):
            if position >= seq and stored_subject == subject:
                return RawStreamMsg(subject=subject, seq=position, data=json.dumps(data).encode())
        raise NotFoundError


async def _hub(js: _JetStream | None = None) -> tuple[NotificationHub, _Client, _JetStream]:
    client, js = _Client(), js or _JetStream()
    hub = NotificationHub(client, js)  # type: ignore[arg-type]
    await hub.start()
    return hub, client, js


async def test_one_named_consumer_per_subject_delivers_into_the_worker_inbox() -> None:
    _, client, js = await _hub()

    assert [c.filter_subject for c in js.created] == list(NOTIFICATION_SUBJECTS)
    for config in js.created:
        assert config.name == "codeforge-py-notify-" + config.filter_subject.replace(".", "-")
        assert config.deliver_subject == f"{INBOX_PREFIX}.notify.{config.name}"
        assert config.ack_policy == AckPolicy.NONE
        assert config.deliver_policy == DeliverPolicy.NEW
        assert config.durable_name is None
    assert set(client.callbacks) == {c.deliver_subject for c in js.created}


async def test_a_consumer_with_other_settings_is_recreated() -> None:
    _, _, js = await _hub(_JetStream(refuse_create=1))

    assert js.deleted == [notification_consumer_name(NOTIFICATION_SUBJECTS[0])]
    assert [c.filter_subject for c in js.created] == list(NOTIFICATION_SUBJECTS)


async def test_every_listener_of_a_subject_gets_every_message() -> None:
    hub, client, _ = await _hub()
    first = await hub.subscribe("runs.cancel")
    second = await hub.subscribe("runs.cancel", config=notification_consumer())
    other = await hub.subscribe("tasks.cancel")

    await client.deliver("runs.cancel", {"run_id": "r1"}, seq=7)

    for sub in (first, second):
        msg = await sub.next_msg(timeout=1)
        assert json.loads(msg.data) == {"run_id": "r1"}
        assert stream_sequence(msg) == 7
    with pytest.raises(nats.errors.TimeoutError):
        await other.next_msg(timeout=0.01)

    await first.unsubscribe()
    await client.deliver("runs.cancel", {"run_id": "r2"}, seq=8)
    with pytest.raises(nats.errors.TimeoutError):
        await first.next_msg(timeout=0.01)
    assert json.loads((await second.next_msg(timeout=1)).data) == {"run_id": "r2"}


async def test_a_listener_from_a_stream_sequence_gets_what_was_published_since() -> None:
    """A cancel published between the work's start message and the subscription is not lost (ADR-016)."""
    stream = [
        ("tasks.cancel", {"task_id": "before"}),  # 1: before the start
        ("tasks.agent.aider", {"task_id": "t1"}),  # 2: the start message
        ("tasks.cancel", {"task_id": "t1"}),  # 3: published while subscribing
        ("runs.cancel", {"run_id": "other subject"}),  # 4
        ("tasks.cancel", {"task_id": "t2"}),  # 5
    ]
    hub, client, _ = await _hub(_JetStream(stream))

    sub = await hub.subscribe("tasks.cancel", config=notification_consumer(after=2))
    await client.deliver("tasks.cancel", {"task_id": "live"}, seq=6)

    received = [await sub.next_msg(timeout=1) for _ in range(3)]
    assert [json.loads(m.data)["task_id"] for m in received] == ["t1", "t2", "live"]
    assert [stream_sequence(m) for m in received] == [3, 5, 6]


async def test_only_notification_subjects() -> None:
    hub, _, _ = await _hub()

    with pytest.raises(ValueError, match=r"runs\.start"):
        await hub.subscribe("runs.start")


async def test_closing_ends_every_listener() -> None:
    hub, _, _ = await _hub()
    sub = await hub.subscribe("runs.toolcall.response")

    hub.close()

    for _ in range(2):
        with pytest.raises(nats.errors.ConnectionClosedError):
            await sub.next_msg(timeout=1)
    with pytest.raises(nats.errors.ConnectionClosedError):
        await hub.subscribe("runs.cancel")


async def test_the_worker_starts_the_hub_and_restores_it_after_a_reconnect(monkeypatch: pytest.MonkeyPatch) -> None:
    """A server that lost the consumers (restart, or longer than their inactivity threshold
    without a worker) gets them back on the reconnect; runs would miss their cancels otherwise.
    """
    consumer = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    js = _JetStream()
    client = _Client()
    connect = AsyncMock(return_value=client)
    monkeypatch.setattr("codeforge.consumer.nats.connect", connect)
    monkeypatch.setattr("codeforge.consumer.TracingJetStreamContext", lambda _nc: js)

    async def subscribe_all(_js: object) -> list[object]:
        consumer.request_stop()  # start() returns once it subscribed
        return []

    monkeypatch.setattr(consumer, "_subscribe_all", subscribe_all)
    await consumer.start()

    assert [c.filter_subject for c in js.created] == list(NOTIFICATION_SUBJECTS)
    assert set(client.callbacks) == {notification_deliver_subject(s) for s in NOTIFICATION_SUBJECTS}
    reconnected = connect.call_args.kwargs["reconnected_cb"]
    await reconnected()
    assert [c.filter_subject for c in js.created] == list(NOTIFICATION_SUBJECTS) * 2

    js.refuse_create = 99  # the server refuses: logged, the callback does not raise
    await reconnected()
