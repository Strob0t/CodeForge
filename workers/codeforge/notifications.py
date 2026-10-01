"""The worker's notifications: cancels and tool-call decisions (KI-71).

Runs, tasks and tool calls wait for messages the Go Core publishes on shared
subjects (runs.cancel, tasks.cancel, conversation.run.cancel,
runs.toolcall.response). The worker may create only consumers whose name is
in the JetStream API subject (configs/nats/nats-server.conf): a name is one
subject token, and the rights list exact names, so the worker has one named
push consumer per notification subject (codeforge-py-notify-<subject>),
shared by all worker instances. It delivers to a fixed subject in the
worker's inbox space; every instance subscribes to it (core NATS fan-out)
and hands each message to the subscriptions of its runs and tasks
(``subscribe``, the same call as JetStreamContext.subscribe).

No notification is lost for an instance:

- A subscription that starts at a stream sequence (a run's or task's start
  message) first gets the messages published since then, read back from
  the stream by subject (batched direct get), then the live ones.
- The hub remembers the stream sequence of the last message it handed out
  per subject. The consumer sequences of the deliveries are contiguous
  (every instance sees every delivery), so a gap means this instance missed
  deliveries (a disconnect, a slow-consumer drop, a recreated consumer); the
  hub then reads the subject back from the last sequence it handed out. It
  does the same after every reconnect, once the consumers exist again.
- Reading back never stops early: a subscription whose read-back would
  exceed the cap fails (the run or task then fails closed), a read-back of a
  gap is retried until it succeeded; meanwhile ``ready`` is false.

The hub never deletes a consumer: another instance may use it. A consumer
whose settings cannot change in place (another worker version) is used as
it is when it delivers the right subject to the right place, else the hub
refuses to start. A message may arrive twice (read back and delivered);
every listener is idempotent.
"""

from __future__ import annotations

import asyncio
import contextlib
import json
from collections import defaultdict
from typing import TYPE_CHECKING, Protocol

import nats.errors
import structlog
from nats.aio.msg import Msg
from nats.errors import NotJSMessageError
from nats.js.api import AckPolicy, ConsumerConfig, DeliverPolicy
from nats.js.errors import APIError

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
# A consumer no instance subscribes to is removed after this long.
_INACTIVE_THRESHOLD_SECONDS = 300.0
# Most messages a subscription reads back before it starts (fails beyond).
MAX_REPLAYED = 100_000
# Messages per direct-get request, and how long to wait for each of them.
_READ_BATCH = 256
_READ_TIMEOUT_SECONDS = 5.0
# Retries of a restore or read-back: first wait, longest wait.
_RETRY_FIRST_SECONDS = 1.0
_RETRY_MAX_SECONDS = 30.0
_DIRECT_GET = f"$JS.API.DIRECT.GET.{STREAM_NAME}"


class NotificationReplayError(RuntimeError):
    """The stream could not be read back completely; nothing may run as if it had been."""


class NotificationConsumerConflictError(RuntimeError):
    """A notification consumer exists with settings that do not deliver what the hub needs."""


def notification_consumer_name(subject: str) -> str:
    """Name of the worker's push consumer for a notification subject."""
    return "codeforge-py-notify-" + subject.replace(".", "-")


def notification_deliver_subject(subject: str) -> str:
    """The subject in the worker's inbox space that consumer delivers to."""
    return f"{INBOX_PREFIX}.notify.{notification_consumer_name(subject)}"


def _consumer_config(subject: str) -> ConsumerConfig:
    return ConsumerConfig(
        name=notification_consumer_name(subject),
        deliver_subject=notification_deliver_subject(subject),
        filter_subject=subject,
        ack_policy=AckPolicy.NONE,
        deliver_policy=DeliverPolicy.NEW,
        inactive_threshold=_INACTIVE_THRESHOLD_SECONDS,
    )


def _delivery_position(msg: Msg) -> tuple[int, int] | None:
    """(stream sequence, consumer sequence) of a consumer delivery; None for anything else."""
    try:
        meta = msg.metadata
    except NotJSMessageError:
        return None
    return int(meta.sequence.stream), int(meta.sequence.consumer)


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
    """The worker's notification consumers and the listeners of its runs and tasks."""

    def __init__(self, nc: NATSClient, js: JetStreamContext) -> None:
        self._nc = nc
        self._js = js
        self._subscriptions: dict[str, set[HubSubscription]] = defaultdict(set)
        # Stream sequence of the last message handed out, consumer sequence
        # of the last delivery, per subject.
        self._last_seq: dict[str, int] = {}
        self._last_consumer_seq: dict[str, int] = {}
        # Restores and read-backs still to do (retried until they succeed).
        self._pending: set[asyncio.Task[None]] = set()
        self._started = False
        self._closed = False

    @property
    def ready(self) -> bool:
        """Whether every notification consumer exists and nothing missed is still to be read back."""
        return self._started and not self._closed and not self._pending

    async def start(self) -> None:
        """Subscribe to every notification consumer's deliver subject and make sure the consumers exist.

        Needs the stream (the Go Core creates it): call it once the worker waited for it.
        """
        for subject in NOTIFICATION_SUBJECTS:
            await self._nc.subscribe(notification_deliver_subject(subject), cb=self._dispatcher(subject))
        info = await self._js.stream_info(STREAM_NAME)
        for subject in NOTIFICATION_SUBJECTS:
            self._last_seq[subject] = info.state.last_seq
        await self._restore()
        self._started = True
        logger.info("notification consumers ready", subjects=list(NOTIFICATION_SUBJECTS))

    def reconnected(self) -> None:
        """After a reconnect: make sure the consumers exist (a server that lost them, or one that
        removed them after their inactivity threshold) and read back what this instance missed.
        Retried until it succeeded; ``ready`` is false meanwhile.
        """
        if self._started and not self._closed:
            self._retry("restore notification consumers", self._restore)

    async def _restore(self) -> None:
        for subject in NOTIFICATION_SUBJECTS:
            await self._ensure_consumer(subject)
        for subject in NOTIFICATION_SUBJECTS:
            await self._catch_up(subject, self._last_seq.get(subject, 0) + 1)

    async def _ensure_consumer(self, subject: str) -> None:
        """Create the subject's consumer, or use the existing one; never delete it (another
        instance may use it).
        """
        config = _consumer_config(subject)
        try:
            # Creating an existing consumer with the same settings succeeds.
            await self._js.add_consumer(STREAM_NAME, config=config)
            return
        except APIError as exc:
            refusal = exc
        try:
            info = await self._js.consumer_info(STREAM_NAME, config.name)
        except APIError as exc:
            raise refusal from exc  # not a consumer with other settings: the error stands
        existing = info.config
        if (
            existing.deliver_subject != config.deliver_subject
            or existing.filter_subject != config.filter_subject
            or existing.ack_policy != AckPolicy.NONE
        ):
            msg = (
                f"notification consumer {config.name} delivers {existing.filter_subject} to "
                f"{existing.deliver_subject} (ack {existing.ack_policy}); it cannot be updated: {refusal}"
            )
            raise NotificationConsumerConflictError(msg) from refusal
        # Settings of another worker version that cannot change in place; it delivers what is needed.
        logger.warning("using a notification consumer with other settings", consumer=config.name, error=str(refusal))

    def _dispatcher(self, subject: str) -> Callable[[Msg], Awaitable[None]]:
        async def dispatch(msg: Msg) -> None:
            position = _delivery_position(msg)
            if position is not None:
                stream_seq, consumer_seq = position
                previous = self._last_consumer_seq.get(subject)
                self._last_consumer_seq[subject] = consumer_seq
                if previous is not None and consumer_seq != previous + 1:
                    # Deliveries this instance did not get, or a recreated consumer.
                    start = self._last_seq.get(subject, 0) + 1
                    self._retry(f"read back {subject}", lambda: self._catch_up(subject, start))
                self._advance(subject, stream_seq)
            self._hand_out(subject, msg)

        return dispatch

    def _advance(self, subject: str, stream_seq: int) -> None:
        self._last_seq[subject] = max(self._last_seq.get(subject, 0), stream_seq)

    def _hand_out(self, subject: str, msg: Msg) -> None:
        for sub in list(self._subscriptions[subject]):
            sub.put(msg)

    async def _catch_up(self, subject: str, start_seq: int) -> None:
        """Hand every listener of *subject* the messages from *start_seq* on."""
        if not self._subscriptions[subject]:
            return  # nobody listens: nothing to hand out (new listeners start from now)

        def hand_out(msg: Msg, seq: int) -> None:
            self._hand_out(subject, msg)
            self._advance(subject, seq)

        await self._read_back(subject, start_seq, hand_out, limit=None)

    def _retry(self, what: str, attempt: Callable[[], Awaitable[None]]) -> None:
        async def until_done() -> None:
            delay = _RETRY_FIRST_SECONDS
            while not self._closed:
                try:
                    await attempt()
                    return
                except Exception as exc:
                    logger.error("notifications: retrying", what=what, retry_in=delay, error=str(exc))
                await asyncio.sleep(delay)
                delay = min(delay * 2, _RETRY_MAX_SECONDS)

        task = asyncio.create_task(until_done(), name=what)
        self._pending.add(task)
        task.add_done_callback(self._pending.discard)

    async def subscribe(self, subject: str, config: ConsumerConfig | None = None) -> HubSubscription:
        """Listen to *subject*: messages from now on, or, with *config* starting at a stream
        sequence (``runtime.notification_consumer(after)``), also those published since then.
        Raises NotificationReplayError when those cannot be read back completely.
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
                await self._read_back(subject, config.opt_start_seq, lambda msg, _seq: sub.put(msg), MAX_REPLAYED)
            except BaseException:
                self.remove(sub)
                raise
        return sub

    def remove(self, sub: HubSubscription) -> None:
        self._subscriptions[sub.subject].discard(sub)

    async def _read_back(
        self, subject: str, start_seq: int, sink: Callable[[Msg, int], None], limit: int | None
    ) -> None:
        """Pass *sink* every message of *subject* from stream sequence *start_seq* on, in order.

        Batched direct gets (the stream allows direct access). Raises
        NotificationReplayError if the stream cannot be read or more than
        *limit* messages would be passed: never stops early.
        """
        seq, passed = start_seq, 0
        while True:
            messages, pending = await self._read_batch(subject, seq)
            for msg, msg_seq in messages:
                passed += 1
                if limit is not None and passed > limit:
                    error = f"more than {limit} messages on {subject} since stream sequence {start_seq}"
                    raise NotificationReplayError(error)
                sink(msg, msg_seq)
                seq = msg_seq + 1
            if pending == 0 or not messages:
                return

    async def _read_batch(self, subject: str, seq: int) -> tuple[list[tuple[Msg, int]], int]:
        """Up to _READ_BATCH messages of *subject* from *seq* on, and how many more there are."""
        inbox = self._nc.new_inbox()
        replies = await self._nc.subscribe(inbox)
        try:
            request = {"seq": seq, "next_by_subj": subject, "batch": _READ_BATCH}
            await self._nc.publish(_DIRECT_GET, json.dumps(request).encode(), reply=inbox)
            messages: list[tuple[Msg, int]] = []
            while True:
                try:
                    reply = await replies.next_msg(timeout=_READ_TIMEOUT_SECONDS)
                except nats.errors.TimeoutError:
                    error = f"reading {subject} back from stream sequence {seq}: no answer"
                    raise NotificationReplayError(error) from None
                headers = reply.headers or {}
                status = headers.get("Status")
                if status is None:
                    msg_seq = int(headers["Nats-Sequence"])
                    messages.append((self._stored_message(subject, reply.data, msg_seq), msg_seq))
                elif status == "204":  # end of the batch
                    return messages, int(headers.get("Nats-Num-Pending", "0"))
                elif status == "404":  # no message (left) on the subject
                    return messages, 0
                else:
                    error = (
                        f"reading {subject} back from stream sequence {seq}: {status} {headers.get('Description', '')}"
                    )
                    raise NotificationReplayError(error.strip())
        finally:
            with contextlib.suppress(Exception):
                await replies.unsubscribe()

    def _stored_message(self, subject: str, data: bytes, seq: int) -> Msg:
        # The reply subject carries the stream sequence like a delivery's
        # (nothing is acked: ack policy none); consumer sequence 0 marks a
        # message read back.
        reply = f"$JS.ACK.{STREAM_NAME}.{notification_consumer_name(subject)}.1.{seq}.0.0.0"
        return Msg(_client=self._nc, subject=subject, reply=reply, data=data)

    def close(self) -> None:
        """End every listener's subscription and stop restoring (after the connection drained)."""
        self._closed = True
        for task in list(self._pending):
            task.cancel()
        for subs in self._subscriptions.values():
            for sub in subs:
                sub.put(None)
            subs.clear()
