"""Shared context event handler mixin for NATS consumer."""

from __future__ import annotations

import json
from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()


class ContextEventsHandlerMixin:
    """Handles context.shared.updated -- awareness event for Python workers."""

    async def _handle_shared_context_updated(self, msg: nats.aio.msg.Msg) -> None:
        """Process a shared context updated notification.

        This is an awareness event: the handler logs the update so Python
        workers know that a team's shared context has changed. No further
        action is needed since the Python side does not cache shared context.
        """
        try:
            data = json.loads(msg.data)
            logger.info(
                "shared context updated",
                team_id=data.get("team_id", ""),
                key=data.get("key", ""),
                author=data.get("author", ""),
                version=data.get("version"),
            )
        except (json.JSONDecodeError, Exception) as exc:
            logger.warning("failed to parse shared context update", error=str(exc))
        await msg.ack()
