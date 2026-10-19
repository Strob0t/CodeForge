"""Publishing messages whose loss would leave accepted work unreported (ADR-016).

A run, task or benchmark acked on accept is never redelivered, so its
completion is the only way the Go Core learns the outcome. Such completions
are published with a few retries instead of once.
"""

from __future__ import annotations

import asyncio
import uuid
from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    from nats.js.client import JetStreamContext

logger = structlog.get_logger()

PUBLISH_ATTEMPTS = 3
# Delay before the first retry; it doubles for every further one.
PUBLISH_BACKOFF_SECONDS = 0.5

_MSG_ID_HEADER = "Nats-Msg-Id"


async def publish_with_retry(
    js: JetStreamContext,
    subject: str,
    data: bytes,
    headers: dict[str, str] | None = None,
) -> None:
    """Publish *data*, retrying a failed attempt; raise the last error if every attempt failed.

    Every attempt carries the same Nats-Msg-Id (the caller's, or a generated
    one), so JetStream stores the message once even if an attempt reached the
    stream but its PubAck was lost.
    """
    headers = dict(headers or {})
    headers.setdefault(_MSG_ID_HEADER, f"{subject}-{uuid.uuid4()}")
    for attempt in range(1, PUBLISH_ATTEMPTS + 1):
        try:
            await js.publish(subject, data, headers=headers)
        except Exception as exc:
            if attempt == PUBLISH_ATTEMPTS:
                raise
            logger.warning("publish failed, retrying", subject=subject, attempt=attempt, error=str(exc))
            await asyncio.sleep(PUBLISH_BACKOFF_SECONDS * 2 ** (attempt - 1))
        else:
            return
