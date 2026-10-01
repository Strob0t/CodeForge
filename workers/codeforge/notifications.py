"""The worker's notifications: cancels and tool-call decisions (KI-71).

Runs, tasks and tool calls wait for messages the Go Core publishes on shared
subjects (runs.cancel, tasks.cancel, conversation.run.cancel,
runs.toolcall.response). They used to create an ephemeral push consumer
each, which needs the JetStream right to create consumers without a name
in the API subject; with that right the worker could reconfigure any
consumer of the stream, the Go Core's included.

The NotificationHub instead keeps one named push consumer per subject for
the whole worker (codeforge-py-notify-<subject>), created through the API
subject that carries its name (the server refuses another name in the
request), and delivering to a fixed subject in the worker's inbox space.
Every worker instance subscribes to that subject, so each one sees every
message, and hands it to the subscriptions of its runs and tasks
(``subscribe``, the same call as JetStreamContext.subscribe).

A subscription that starts at a stream sequence (a run's or task's start
message) first gets the messages published since then, read from the
stream by subject (STREAM.MSG.GET next_by_subj), and then the live ones:
a cancel published between the start message and the subscription is not
lost. A message may arrive twice (read and delivered); every listener is
idempotent.
"""

from __future__ import annotations

import asyncio
from collections import defaultdict
from typing import TYPE_CHECKING, Protocol

import nats.errors
import structlog
from nats.aio.msg import Msg
from nats.js.api import AckPolicy, ConsumerConfig, DeliverPolicy
from nats.js.errors import APIError, NotFoundError

from codeforge.nats_subjects import (
    INBOX_PREFIX,
    STREAM_NAME,
    SUBJECT_CONVERSATION_RUN_CANCEL,
    SUBJECT_RUN_CANCEL,
    SUBJECT_TASK_CANCEL,
    SUBJECT_TOOLCALL_RESPONSE,
)

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable

    from nats.aio.client import Client as NATSClient
    from nats.js.client import JetStreamContext

logger = structlog.get_logger()

NOTIFICATION_SUBJECTS = (
    SUBJECT_RUN_CANCEL,
    SUBJECT_TASK_CANCEL,
    SUBJECT_CONVERSATION_RUN_CANCEL,
    SUBJECT_TOOLCALL_RESPONSE,
)
# A hub consumer without a subscribed worker is removed after this long.
_INACTIVE_THRESHOLD_SECONDS = 300.0
# Most messages one subscription reads back from the stream.
_MAX_REPLAYED = 10_000


def notification_consumer_name(subject: str) -> str:
    """Name of the worker's push consumer for a notification subject."""
    return "codeforge-py-notify-" + subject.replace(".", "-")


def notification_deliver_subject(subject: str) -> str:
    """The subject in the worker's inbox space that consumer delivers to."""
    return f"{INBOX_PREFIX}.notify.{notification_consumer_name(subject)}"


class NotificationSubscription(Protocol):
    """What a listener does with its notifications (a JetStream push subscription has the same calls)."""

    async def next_msg(self, timeout: float = 1.0) -> Msg: ...

    async def unsubscribe(self) -> None: ...


class Notifications(Protocol):
    """Where runs, tasks and the cancel registry subscribe to notifications."""

    async def subscribe(self, subject: str, config: ConsumerConfig | None = None) -> NotificationSubscription: ...


class HubSubscription:
    """The messages of one notification subject for one listener (next_msg, unsubscribe)."""

    def __init__(self, hub: NotificationHub, subject: str) -> None:
        self.subject = subject
        self._hub = hub
        # None: the hub closed.
        self._queue: asyncio.Queue[Msg | None] = asyncio.Queue()

    def put(self, msg: Msg | None) -> None:
        self._queue.put_nowait(msg)

    async def next_msg(self, timeout: float = 1.0) -> Msg:
        """The next message; raises nats.errors.TimeoutError when none arrives within *timeout*
        seconds, and nats.errors.ConnectionClosedError once the hub closed.
        """
        try:
            msg = await asyncio.wait_for(self._queue.get(), timeout=timeout)
        except TimeoutError:
            raise nats.errors.TimeoutError from None
        if msg is None:
            self._queue.put_nowait(None)
            raise nats.errors.ConnectionClosedError
        return msg

    async def unsubscribe(self) -> None:
        self._hub.remove(self)


class NotificationHub:
    """One named push consumer per notification subject, shared by the worker's runs and tasks."""

    def __init__(self, nc: NATSClient, js: JetStreamContext) -> None:
        self._nc = nc
        self._js = js
        self._subscriptions: dict[str, set[HubSubscription]] = defaultdict(set)
        self._closed = False

    async def start(self) -> None:
        """Subscribe to every notification consumer's deliver subject and make sure the consumers exist."""
        for subject in NOTIFICATION_SUBJECTS:
            await self._nc.subscribe(notification_deliver_subject(subject), cb=self._dispatcher(subject))
        await self.restore()
        logger.info("notification consumers ready", subjects=list(NOTIFICATION_SUBJECTS))

    async def restore(self) -> None:
        """Make sure every notification consumer exists: after a reconnect, a restarted
        server has lost them (they are not durable), and every run would miss its cancels
        and tool-call decisions. The client restores the subscriptions itself.
        """
        for subject in NOTIFICATION_SUBJECTS:
            await self._ensure_consumer(subject, notification_deliver_subject(subject))

    async def _ensure_consumer(self, subject: str, deliver: str) -> None:
        name = notification_consumer_name(subject)
        config = ConsumerConfig(
            name=name,
            deliver_subject=deliver,
            filter_subject=subject,
            ack_policy=AckPolicy.NONE,
            deliver_policy=DeliverPolicy.NEW,
            inactive_threshold=_INACTIVE_THRESHOLD_SECONDS,
        )
        try:
            # Creating an existing consumer with the same settings succeeds.
            await self._js.add_consumer(STREAM_NAME, config=config)
        except APIError as exc:
            # Settings that cannot change in place (another worker version):
            # recreate it. Workers still subscribed keep receiving, the deliver
            # subject is the same.
            logger.warning("recreating notification consumer", consumer=name, error=str(exc))
            await self._js.delete_consumer(STREAM_NAME, name)
            await self._js.add_consumer(STREAM_NAME, config=config)

    def _dispatcher(self, subject: str) -> Callable[[Msg], Awaitable[None]]:
        async def dispatch(msg: Msg) -> None:
            for sub in list(self._subscriptions[subject]):
                sub.put(msg)

        return dispatch

    async def subscribe(self, subject: str, config: ConsumerConfig | None = None) -> HubSubscription:
        """Listen to *subject*: messages from now on, or, with *config* starting at a stream
        sequence (``runtime.notification_consumer(after)``), also those published since then.
        """
        if subject not in NOTIFICATION_SUBJECTS:
            msg = f"{subject} is not a notification subject"
            raise ValueError(msg)
        if self._closed:
            raise nats.errors.ConnectionClosedError
        sub = HubSubscription(self, subject)
        # Registered first: a message published while the stream is read is
        # delivered to it (possibly twice), never lost.
        self._subscriptions[subject].add(sub)
        if config is not None and config.deliver_policy == DeliverPolicy.BY_START_SEQUENCE and config.opt_start_seq:
            try:
                await self._replay(sub, config.opt_start_seq)
            except BaseException:
                self.remove(sub)
                raise
        return sub

    def remove(self, sub: HubSubscription) -> None:
        self._subscriptions[sub.subject].discard(sub)

    async def _replay(self, sub: HubSubscription, start_seq: int) -> None:
        """Hand *sub* the messages of its subject from stream sequence *start_seq* on."""
        seq = start_seq
        for _ in range(_MAX_REPLAYED):
            try:
                raw = await self._js.get_msg(STREAM_NAME, seq=seq, subject=sub.subject, next=True)
            except NotFoundError:
                return
            consumer = notification_consumer_name(sub.subject)
            # The reply subject carries the stream sequence like a delivery's
            # (nothing is acked: ack policy none).
            reply = f"$JS.ACK.{STREAM_NAME}.{consumer}.1.{raw.seq}.0.0.0"
            sub.put(Msg(_client=self._nc, subject=raw.subject or sub.subject, reply=reply, data=raw.data or b""))
            seq = (raw.seq or seq) + 1
        logger.warning("stopped reading notifications back from the stream", subject=sub.subject, start=start_seq)

    def close(self) -> None:
        """End every listener's subscription (after the connection drained)."""
        self._closed = True
        for subs in self._subscriptions.values():
            for sub in subs:
                sub.put(None)
            subs.clear()
