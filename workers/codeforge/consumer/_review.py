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
        await self._handle_request(
            msg=msg,
            request_model=ReviewTriggerRequestPayload,
            dedup_key=lambda r: f"review-{r.project_id}-{r.commit_sha}",
            handler=self._do_review_trigger,
            log_context=lambda r: {
                "project_id": r.project_id,
                "commit_sha": r.commit_sha,
                "source": r.source,
            },
        )

    async def _do_review_trigger(self, request: ReviewTriggerRequestPayload, log: structlog.BoundLogger) -> None:
        log.info("review trigger received")
        # TODO: Dispatch boundary-analyzer run via agent loop
