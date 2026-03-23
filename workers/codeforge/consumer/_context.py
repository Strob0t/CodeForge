"""Context re-ranking handler mixin for NATS consumer."""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._subjects import (
    SUBJECT_CONTEXT_RERANK_RESULT,
)
from codeforge.context_reranker import ContextReranker, RerankEntry
from codeforge.models import (
    ContextRerankEntryPayload,
    ContextRerankRequest,
    ContextRerankResult,
)

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()


class ContextHandlerMixin:
    """Handles context.rerank.request — re-ranks context entries via LLM."""

    async def _handle_context_rerank(self, msg: nats.aio.msg.Msg) -> None:
        """Re-rank context entries using LLM and publish result."""
        await self._handle_request(
            msg=msg,
            request_model=ContextRerankRequest,
            dedup_key=lambda r: f"ctx-rerank-{r.request_id}",
            handler=self._do_context_rerank,
            result_subject=SUBJECT_CONTEXT_RERANK_RESULT,
            log_context=lambda r: {
                "request_id": r.request_id,
                "project_id": r.project_id,
            },
        )

    async def _do_context_rerank(
        self, request: ContextRerankRequest, log: structlog.BoundLogger
    ) -> ContextRerankResult:
        """Business logic for context re-ranking."""
        log.info(
            "received context rerank request",
            entry_count=len(request.entries),
        )

        reranker = ContextReranker(llm=self._llm, model=request.model)
        entries = [
            RerankEntry(
                path=e.path,
                kind=e.kind,
                content=e.content,
                priority=e.priority,
                tokens=e.tokens,
            )
            for e in request.entries
        ]

        try:
            result = await reranker.rerank(entries=entries, query=request.query)
        except Exception as exc:
            logger.error("context rerank failed", error=str(exc))
            await self._publish_error(
                ContextRerankResult(
                    request_id=request.request_id,
                    error="internal worker error",
                ),
                SUBJECT_CONTEXT_RERANK_RESULT,
            )
            raise

        payload = ContextRerankResult(
            request_id=request.request_id,
            entries=[
                ContextRerankEntryPayload(
                    path=e.path,
                    kind=e.kind,
                    content=e.content,
                    priority=e.priority,
                    tokens=e.tokens,
                )
                for e in result.entries
            ],
            fallback_used=result.fallback_used,
            tokens_in=result.tokens_in,
            tokens_out=result.tokens_out,
            cost_usd=result.cost_usd,
        )

        log.info("context rerank complete", fallback=result.fallback_used)
        return payload
