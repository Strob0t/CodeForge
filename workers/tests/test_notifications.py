"""The worker's notification consumers (codeforge.notifications, KI-71).

The worker may create only consumers whose name is in the JetStream API
subject, so runs and tasks do not create an ephemeral consumer each: one
named push consumer per notification subject, shared by the worker
instances, delivers into the worker's inbox space, and the hub hands each
message to every listener of its subject. What an instance misses is read
back from the stream. These tests use fakes; test_nats_permissions.py runs
the hub against a real nats-server with the production permissions.
"""

from __future__ import annotations

import asyncio
import itertools
import json
from collections.abc import Awaitable, Callable
from types import SimpleNamespace
from unittest.mock import AsyncMock

import nats.errors
import nats.js.errors
import pytest
from nats.aio.msg import Msg
from nats.js.api import AckPolicy, ConsumerConfig, DeliverPolicy
from nats.js.errors import NotFoundError, ServerError, ServiceUnavailableError

import codeforge.notifications as notifications
from codeforge.consumer import TaskConsumer
from codeforge.consumer._delivery import stream_sequence
from codeforge.nats_subjects import INBOX_PREFIX, STREAM_NAME
from codeforge.notifications import (
    NOTIFICATION_SUBJECTS,
    NotificationConsumerConflictError,
    NotificationHub,
    NotificationReplayError,
    notification_consumer_name,
    notification_deliver_subject,
)
from codeforge.runtime import notification_consumer

Callback = Callable[[Msg], Awaitable[None]]


class _Inbox:
    def __init__(self) -> None:
        self.queue: asyncio.Queue[Msg] = asyncio.Queue()

    async def next_msg(self, timeout: float = 1.0) -> Msg:
        try:
            return await asyncio.wait_for(self.queue.get(), timeout=min(timeout, 0.5))
        except TimeoutError:
            raise nats.errors.TimeoutError from None

    async def unsubscribe(self) -> None:
        return None


class _Client:
    """Core NATS: the hub's subscriptions, and direct gets answered from *stream* (seq = index + 1)."""

    def __init__(self, stream: list[tuple[str, dict[str, str]]] | None = None) -> None:
        self.stream = stream or []
        self.callbacks: dict[str, Callback] = {}
        self.inboxes: dict[str, _Inbox] = {}
        self.read_requests: list[dict[str, object]] = []
        self.failing_reads = 0
        self._ids = itertools.count()

    def new_inbox(self) -> str:
        return f"{INBOX_PREFIX}.test.{next(self._ids)}"

    async def subscribe(self, subject: str, cb: Callback | None = None) -> _Inbox | None:
        if cb is not None:
            self.callbacks[subject] = cb
            return None
        self.inboxes[subject] = _Inbox()
        return self.inboxes[subject]

    async def publish(self, subject: str, payload: bytes = b"", reply: str = "") -> None:
        assert subject == f"$JS.API.DIRECT.GET.{STREAM_NAME}"
        request = json.loads(payload)
        self.read_requests.append(request)
        inbox = self.inboxes[reply].queue
        if self.failing_reads > 0:
            self.failing_reads -= 1
            inbox.put_nowait(self._status("503", "No Responders"))
            return
        matches = [
            (seq, data)
            for seq, (stored, data) in enumerate(self.stream, start=1)
            if seq >= request["seq"] and stored == request["next_by_subj"]
        ]
        if not matches:
            inbox.put_nowait(self._status("404", "Message Not Found"))
            return
        for seq, data in matches[: request["batch"]]:
            headers = {"Nats-Sequence": str(seq), "Nats-Subject": request["next_by_subj"]}
            inbox.put_nowait(Msg(_client=self, subject=reply, data=json.dumps(data).encode(), headers=headers))  # type: ignore[arg-type]
        pending = max(0, len(matches) - request["batch"])
        inbox.put_nowait(self._status("204", "EOB", {"Nats-Num-Pending": str(pending)}))

    def _status(self, code: str, description: str, extra: dict[str, str] | None = None) -> Msg:
        return Msg(_client=self, subject="", headers={"Status": code, "Description": description, **(extra or {})})  # type: ignore[arg-type]

    def published(self, subject: str, data: dict[str, str]) -> int:
        """Store a message in the stream; return its sequence."""
        self.stream.append((subject, data))
        return len(self.stream)

    async def deliver(self, subject: str, data: dict[str, str], stream_seq: int, consumer_seq: int) -> None:
        """Deliver a message of *subject* the way its notification consumer does."""
        reply = f"$JS.ACK.{STREAM_NAME}.{notification_consumer_name(subject)}.1.{stream_seq}.{consumer_seq}.0.0"
        msg = Msg(_client=self, subject=subject, reply=reply, data=json.dumps(data).encode())  # type: ignore[arg-type]
        await self.callbacks[notification_deliver_subject(subject)](msg)


class _JetStream:
    """Consumer management of the stream; *refusals* are raised by the next add_consumer calls."""

    def __init__(self, client: _Client, refusals: list[Exception] | None = None) -> None:
        self.client = client
        self.created: list[ConsumerConfig] = []
        self.refusals = list(refusals or [])
        self.existing: dict[str, ConsumerConfig] = {}
        self.delete_consumer = AsyncMock()
        self.stream_exists = True

    def _need_stream(self) -> None:
        if not self.stream_exists:
            raise NotFoundError(code=404, err_code=10059, description="stream not found")

    async def stream_info(self, name: str) -> SimpleNamespace:
        assert name == STREAM_NAME
        self._need_stream()
        return SimpleNamespace(state=SimpleNamespace(last_seq=len(self.client.stream)))

    async def add_consumer(self, stream: str, config: ConsumerConfig) -> None:
        assert stream == STREAM_NAME
        self._need_stream()
        if self.refusals:
            raise self.refusals.pop(0)
        self.created.append(config)
        self.existing[config.name] = config

    async def consumer_info(self, stream: str, name: str) -> SimpleNamespace:
        assert stream == STREAM_NAME
        self._need_stream()
        if name not in self.existing:
            raise NotFoundError
        return SimpleNamespace(config=self.existing[name])


async def _hub(
    client: _Client | None = None, js: _JetStream | None = None
) -> tuple[NotificationHub, _Client, _JetStream]:
    client = client or _Client()
    js = js or _JetStream(client)
    hub = NotificationHub(client, js)  # type: ignore[arg-type]
    await hub.start()
    return hub, client, js


async def _ids(sub: object, field: str, count: int) -> list[str]:
    return [json.loads((await sub.next_msg(timeout=1)).data)[field] for _ in range(count)]  # type: ignore[attr-defined]


async def _eventually(check: Callable[[], bool]) -> None:
    for _ in range(200):
        if check():
            return
        await asyncio.sleep(0.01)
    raise AssertionError("condition not met in time")


@pytest.fixture(autouse=True)
def _fast_retries(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(notifications, "_RETRY_FIRST_SECONDS", 0.01)


async def test_one_named_consumer_per_subject_delivers_into_the_worker_inbox() -> None:
    hub, client, js = await _hub()

    assert [c.filter_subject for c in js.created] == list(NOTIFICATION_SUBJECTS)
    for config in js.created:
        assert config.name == "codeforge-py-notify-" + config.filter_subject.replace(".", "-")
        assert config.deliver_subject == f"{INBOX_PREFIX}.notify.{config.name}"
        assert config.ack_policy == AckPolicy.NONE
        assert config.deliver_policy == DeliverPolicy.NEW
        assert config.durable_name is None
    assert set(client.callbacks) == {c.deliver_subject for c in js.created}
    assert hub.ready


async def test_every_listener_of_a_subject_gets_every_message() -> None:
    hub, client, _ = await _hub()
    first = await hub.subscribe("runs.cancel")
    second = await hub.subscribe("runs.cancel", config=notification_consumer())
    other = await hub.subscribe("tasks.cancel")

    await client.deliver("runs.cancel", {"run_id": "r1"}, stream_seq=7, consumer_seq=1)

    for sub in (first, second):
        msg = await sub.next_msg(timeout=1)
        assert json.loads(msg.data) == {"run_id": "r1"}
        assert stream_sequence(msg) == 7
    with pytest.raises(nats.errors.TimeoutError):
        await other.next_msg(timeout=0.01)

    await first.unsubscribe()
    await client.deliver("runs.cancel", {"run_id": "r2"}, stream_seq=8, consumer_seq=2)
    with pytest.raises(nats.errors.TimeoutError):
        await first.next_msg(timeout=0.01)
    assert json.loads((await second.next_msg(timeout=1)).data) == {"run_id": "r2"}


async def test_a_listener_from_a_stream_sequence_gets_what_was_published_since() -> None:
    """A cancel published between the work's start message and the subscription is not lost (ADR-016)."""
    client = _Client(
        [
            ("tasks.cancel", {"task_id": "before"}),  # 1: before the start
            ("tasks.agent.aider", {"task_id": "t1"}),  # 2: the start message
            ("tasks.cancel", {"task_id": "t1"}),  # 3: published while subscribing
            ("runs.cancel", {"run_id": "other subject"}),  # 4
            ("tasks.cancel", {"task_id": "t2"}),  # 5
        ]
    )
    hub, client, _ = await _hub(client)

    sub = await hub.subscribe("tasks.cancel", config=notification_consumer(after=2))
    await client.deliver("tasks.cancel", {"task_id": "live"}, stream_seq=6, consumer_seq=1)

    received = [await sub.next_msg(timeout=1) for _ in range(3)]
    assert [json.loads(m.data)["task_id"] for m in received] == ["t1", "t2", "live"]
    assert [stream_sequence(m) for m in received] == [3, 5, 6]


async def test_reading_back_uses_batches(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(notifications, "_READ_BATCH", 2)
    client = _Client([("runs.cancel", {"run_id": f"r{i}"}) for i in range(5)])
    hub, client, _ = await _hub(client)

    sub = await hub.subscribe("runs.cancel", config=notification_consumer(after=0))

    assert await _ids(sub, "run_id", 5) == ["r0", "r1", "r2", "r3", "r4"]
    assert [r["seq"] for r in client.read_requests] == [1, 3, 5]


async def test_a_read_back_beyond_the_cap_fails_the_subscription(monkeypatch: pytest.MonkeyPatch) -> None:
    """Never past a lost cancel: the run or task fails instead (finding 4)."""
    monkeypatch.setattr(notifications, "MAX_REPLAYED", 3)
    monkeypatch.setattr(notifications, "_READ_BATCH", 2)
    client = _Client([("tasks.cancel", {"task_id": f"t{i}"}) for i in range(4)])
    hub, client, _ = await _hub(client)

    with pytest.raises(NotificationReplayError, match="more than 3"):
        await hub.subscribe("tasks.cancel", config=notification_consumer(after=0))

    await client.deliver("tasks.cancel", {"task_id": "live"}, stream_seq=5, consumer_seq=1)
    assert not hub._subscriptions["tasks.cancel"], "the failed subscription must not stay registered"


async def test_a_failed_read_back_fails_the_subscription() -> None:
    client = _Client([("runs.cancel", {"run_id": "r1"})])
    hub, client, _ = await _hub(client)
    client.failing_reads = 1

    with pytest.raises(NotificationReplayError, match="503"):
        await hub.subscribe("runs.cancel", config=notification_consumer(after=0))


async def test_missed_deliveries_are_read_back() -> None:
    """A gap in the consumer sequence: this instance missed deliveries (a disconnect while
    another instance kept the shared consumer busy, a slow-consumer drop)."""
    hub, client, _ = await _hub()
    sub = await hub.subscribe("runs.cancel")
    first = client.published("runs.cancel", {"run_id": "seen"})
    await client.deliver("runs.cancel", {"run_id": "seen"}, stream_seq=first, consumer_seq=1)
    client.published("runs.cancel", {"run_id": "missed"})
    last = client.published("runs.cancel", {"run_id": "after"})

    await client.deliver("runs.cancel", {"run_id": "after"}, stream_seq=last, consumer_seq=3)

    received = set(await _ids(sub, "run_id", 3))
    assert "missed" in received
    await _eventually(lambda: hub.ready)


async def test_a_reconnect_restores_the_consumers_and_reads_back_what_was_missed() -> None:
    hub, client, js = await _hub()
    sub = await hub.subscribe("tasks.cancel")
    client.published("tasks.cancel", {"task_id": "while away"})

    hub.reconnected()
    assert not hub.ready
    await _eventually(lambda: hub.ready)

    assert await _ids(sub, "task_id", 1) == ["while away"]
    assert [c.filter_subject for c in js.created] == list(NOTIFICATION_SUBJECTS) * 2


async def test_a_failing_restore_is_retried_and_the_hub_is_not_ready_meanwhile() -> None:
    hub, _, js = await _hub()
    js.refusals = [ServiceUnavailableError(code=503, description="no responders")] * 3

    hub.reconnected()
    await asyncio.sleep(0)
    assert not hub.ready
    await _eventually(lambda: hub.ready)
    js.delete_consumer.assert_not_awaited()


async def test_a_transient_error_never_deletes_the_shared_consumer() -> None:
    client = _Client()
    js = _JetStream(client, refusals=[ServerError(code=500, description="internal error")])
    with pytest.raises(nats.js.errors.APIError, match="internal error"):
        await _hub(client, js)
    js.delete_consumer.assert_not_awaited()


async def test_a_consumer_with_other_settings_is_used_as_it_is() -> None:
    """Another worker version's consumer that cannot be updated in place: used if it delivers the
    subject to the hub, never deleted (other instances use it)."""
    client = _Client()
    js = _JetStream(client)
    subject = NOTIFICATION_SUBJECTS[0]
    other_version = ConsumerConfig(
        name=notification_consumer_name(subject),
        deliver_subject=notification_deliver_subject(subject),
        filter_subject=subject,
        ack_policy=AckPolicy.NONE,
        inactive_threshold=60.0,
    )
    js.existing[other_version.name] = other_version
    js.refusals = [ServerError(code=500, err_code=10012, description="ack policy can not be updated")]

    hub, _, _ = await _hub(client, js)

    assert hub.ready
    js.delete_consumer.assert_not_awaited()


async def test_a_consumer_that_delivers_elsewhere_stops_the_hub() -> None:
    client = _Client()
    js = _JetStream(client)
    subject = NOTIFICATION_SUBJECTS[0]
    js.existing[notification_consumer_name(subject)] = ConsumerConfig(
        name=notification_consumer_name(subject),
        deliver_subject=f"{INBOX_PREFIX}.elsewhere",
        filter_subject=subject,
        ack_policy=AckPolicy.NONE,
    )
    js.refusals = [ServerError(code=500, err_code=10012, description="cannot be updated")]

    with pytest.raises(NotificationConsumerConflictError, match="elsewhere"):
        await _hub(client, js)
    js.delete_consumer.assert_not_awaited()


async def test_only_notification_subjects() -> None:
    hub, _, _ = await _hub()

    with pytest.raises(ValueError, match=r"runs\.start"):
        await hub.subscribe("runs.start")


async def test_closing_ends_every_listener() -> None:
    hub, _, _ = await _hub()
    sub = await hub.subscribe("runs.toolcall.response")

    hub.close()

    assert not hub.ready
    for _ in range(2):
        with pytest.raises(nats.errors.ConnectionClosedError):
            await sub.next_msg(timeout=1)
    with pytest.raises(nats.errors.ConnectionClosedError):
        await hub.subscribe("runs.cancel")


async def test_the_worker_waits_for_the_stream_then_starts_the_hub(monkeypatch: pytest.MonkeyPatch) -> None:
    """Finding 1: the hub needs the stream the Go Core creates. Only the connection, the stream
    lookups and the durables are faked; TaskConsumer.start, _subscribe_all and the hub are real.
    """
    monkeypatch.setattr("codeforge.consumer._STREAM_POLL_SECONDS", 0.01)
    consumer = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    client = _Client()
    js = _JetStream(client)
    js.stream_exists = False  # the Go Core creates it on the third lookup
    lookups = 0

    async def find_stream_name_by_subject(_subject: str) -> str:
        nonlocal lookups
        lookups += 1
        js.stream_exists = lookups >= 3
        if not js.stream_exists:
            raise NotFoundError(code=404, err_code=10059, description="stream not found")
        return STREAM_NAME

    js.find_stream_name_by_subject = find_stream_name_by_subject  # type: ignore[attr-defined]

    async def idle_durable(_js: object, _name: str, _subject: str) -> SimpleNamespace:
        async def fetch(**_kwargs: object) -> list[object]:
            await asyncio.sleep(0.01)
            raise nats.errors.TimeoutError

        return SimpleNamespace(fetch=fetch, unsubscribe=AsyncMock())

    connect = AsyncMock(return_value=client)
    monkeypatch.setattr("codeforge.consumer.nats.connect", connect)
    monkeypatch.setattr("codeforge.consumer.TracingJetStreamContext", lambda _nc: js)
    monkeypatch.setattr("codeforge.consumer.ensure_durable", idle_durable)
    client.is_connected = True  # type: ignore[attr-defined]
    client.drain = AsyncMock()  # type: ignore[attr-defined]
    client.close = AsyncMock()  # type: ignore[attr-defined]

    started = asyncio.create_task(consumer.start())
    await _eventually(lambda: consumer.ready or started.done())
    assert not started.done(), started.exception()

    assert lookups == 3
    assert [c.filter_subject for c in js.created] == list(NOTIFICATION_SUBJECTS)
    assert connect.call_args.kwargs["reconnected_cb"] == consumer._restore_notifications

    # Finding 2: after a reconnect the worker is not ready until the notifications are restored.
    js.refusals = [ServiceUnavailableError(code=503, description="no responders")] * 2
    await consumer._restore_notifications()
    assert not consumer.ready
    await _eventually(lambda: consumer.ready)

    await consumer.stop()
    await asyncio.wait_for(started, timeout=5)
