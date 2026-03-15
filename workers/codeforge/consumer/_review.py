"""Review trigger handler mixin."""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from codeforge.models import ReviewTriggerRequestPayload

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()


class ReviewHandlerMixin:
    """Handles review.trigger.request NATS messages."""

    async def _handle_review_trigger(self, msg: nats.aio.msg.Msg) -> None:
        try:
            payload = ReviewTriggerRequestPayload.model_validate_json(msg.data)
            logger.info(
                "review trigger received",
                project_id=payload.project_id,
                commit_sha=payload.commit_sha,
                source=payload.source,
            )
            # TODO: Dispatch boundary-analyzer run via agent loop
            await msg.ack()
        except Exception as exc:
            logger.error("review trigger failed", error=str(exc))
            await msg.ack()  # ack to prevent infinite redelivery
