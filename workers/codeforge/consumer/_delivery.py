"""JetStream delivery semantics of the worker (ADR-016).

Each work subject has one durable pull consumer shared by every worker
instance, so a message is processed by one of them. A new durable starts at
the next published message; re-attaching keeps its position. Retries are
counted by JetStream (``num_delivered``) and bounded by ``MAX_DELIVER``. The Go
Core applies the same rules to its own subscriptions.
"""

from __future__ import annotations

import asyncio
import contextlib
import dataclasses
from typing import TYPE_CHECKING

import structlog
from nats.errors import NotJSMessageError
from nats.js.api import AckPolicy, ConsumerConfig, DeliverPolicy
from nats.js.errors import NotFoundError

from codeforge.nats_subjects import (
    ACK_WAIT_SECONDS,
    HEADER_ORIGINAL_MSG_ID,
    MAX_DELIVER,
    MAX_IN_PROGRESS_SECONDS,
    STREAM_NAME,
)

if TYPE_CHECKING:
    from collections.abc import AsyncIterator, Callable

    from nats.aio.msg import Msg
    from nats.js.api import ConsumerInfo
    from nats.js.client import JetStreamContext

logger = structlog.get_logger()

# In-progress acks are sent three times per ack wait, so a single late one
# does not let JetStream redeliver a message that is still being handled.
PROGRESS_INTERVAL_SECONDS = ACK_WAIT_SECONDS / 3


def durable_config(name: str, subject: str) -> ConsumerConfig:
    """Consumer settings of a work subject's durable pull consumer.

    No inactivity threshold: a durable must survive worker downtime.
    """
    return ConsumerConfig(
        name=name,
        durable_name=name,
        filter_subject=subject,
        ack_policy=AckPolicy.EXPLICIT,
        ack_wait=ACK_WAIT_SECONDS,
        max_deliver=MAX_DELIVER,
        deliver_policy=DeliverPolicy.NEW,
    )


def _keep_position(wanted: ConsumerConfig, existing: ConsumerConfig) -> ConsumerConfig:
    """Update settings but keep the start policy, which JetStream cannot change in place."""
    return dataclasses.replace(
        wanted,
        deliver_policy=existing.deliver_policy,
        opt_start_seq=existing.opt_start_seq,
        opt_start_time=existing.opt_start_time,
    )


def _resume_after(wanted: ConsumerConfig, info: ConsumerInfo) -> ConsumerConfig:
    """Start a replacement consumer after the last message the old one acknowledged."""
    acked_up_to = info.ack_floor.stream_seq if info.ack_floor else 0
    if acked_up_to <= 0:
        return wanted
    return dataclasses.replace(wanted, deliver_policy=DeliverPolicy.BY_START_SEQUENCE, opt_start_seq=acked_up_to + 1)


async def ensure_durable(js: JetStreamContext, name: str, subject: str) -> JetStreamContext.PullSubscription:
    """Create the durable consumer of *subject* on first use, re-attach to it afterwards.

    Safe to call from every worker instance on every start: creating or
    recreating the durable never replays the stream history.
    """
    wanted = durable_config(name, subject)
    try:
        info = await js.consumer_info(STREAM_NAME, name)
    except NotFoundError:
        await js.add_consumer(STREAM_NAME, config=wanted)
    else:
        if info.config.deliver_subject:
            # Earlier releases may have left a push consumer under this name. It
            # cannot become a pull consumer in place, so it is replaced by one
            # that continues after the messages it already acknowledged.
            logger.warning("replacing push consumer with pull consumer", consumer=name, stream=STREAM_NAME)
            await js.delete_consumer(STREAM_NAME, name)
            await js.add_consumer(STREAM_NAME, config=_resume_after(wanted, info))
        else:
            await js.add_consumer(STREAM_NAME, config=_keep_position(wanted, info.config))
    return await js.pull_subscribe_bind(consumer=name, stream=STREAM_NAME)


def delivery_attempt(msg: Msg) -> int:
    """Return how often JetStream has delivered *msg* (1 on the first delivery).

    Messages without JetStream metadata count as a first delivery.
    """
    try:
        return int(msg.metadata.num_delivered)
    except NotJSMessageError:
        return 1


def is_last_attempt(msg: Msg) -> bool:
    """Whether JetStream will not redeliver *msg* after this attempt."""
    return delivery_attempt(msg) >= MAX_DELIVER


def dlq_headers(headers: dict[str, str] | None) -> dict[str, str] | None:
    """Headers for the dead-letter copy of a message with *headers*.

    The JetStream publish-control headers (``Nats-*``) are dropped: with the
    original ``Nats-Msg-Id`` the stream would discard the copy as a duplicate of
    the original. The original ID is kept in ``X-Original-Msg-Id``.
    """
    if not headers:
        return None
    copied = {key: value for key, value in headers.items() if not key.lower().startswith("nats-")}
    original_id = headers.get("Nats-Msg-Id")
    if original_id:
        copied[HEADER_ORIGINAL_MSG_ID] = original_id
    return copied or None


@contextlib.asynccontextmanager
async def keep_in_progress(
    msg: Msg,
    interval: float = PROGRESS_INTERVAL_SECONDS,
    limit: float = MAX_IN_PROGRESS_SECONDS,
    clock: Callable[[], float] | None = None,
) -> AsyncIterator[None]:
    """Tell JetStream every *interval* seconds that *msg* is still being handled.

    Stops when the block exits, once the message has been settled (for example
    acked on accept), or after *limit* seconds measured with *clock* (the event
    loop's clock by default): a handler slower than the ack wait is not
    redelivered to another worker while it runs, but a hung one is redelivered
    instead of holding its message forever.
    """
    now = clock or asyncio.get_running_loop().time
    deadline = now() + limit

    async def _beat() -> None:
        while True:
            await asyncio.sleep(interval)
            if msg.is_acked:
                return
            if now() > deadline:
                logger.warning("handler exceeded the in-progress limit, JetStream will redeliver", subject=msg.subject)
                return
            try:
                await msg.in_progress()
            except Exception as exc:
                logger.warning("in-progress ack failed", subject=msg.subject, error=str(exc))

    task = asyncio.create_task(_beat())
    try:
        yield
    finally:
        task.cancel()
        with contextlib.suppress(asyncio.CancelledError):
            await task
