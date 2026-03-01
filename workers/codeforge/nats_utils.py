"""NATS handler utilities shared by consumer modules."""

from __future__ import annotations

import functools
from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable

    import nats.aio.msg

logger = structlog.get_logger()


def nats_handler(
    *,
    ack_on_error: bool = False,
) -> Callable[
    [Callable[..., Awaitable[None]]],
    Callable[..., Awaitable[None]],
]:
    """Decorator that wraps a NATS message handler with error handling.

    On success the message is acked inside the handler body as usual.
    On unhandled exception:
      - If *ack_on_error* is True the message is acked (prevents re-delivery
        for handlers where retry will not help).
      - Otherwise the message is naked (allows NATS retry / DLQ).
    """

    def decorator(
        fn: Callable[..., Awaitable[None]],
    ) -> Callable[..., Awaitable[None]]:
        @functools.wraps(fn)
        async def wrapper(self: object, msg: nats.aio.msg.Msg) -> None:
            try:
                await fn(self, msg)
            except Exception:
                logger.exception("handler %s failed", fn.__name__)
                if ack_on_error:
                    await msg.ack()
                else:
                    await msg.nak()

        return wrapper

    return decorator
