"""Retrieval index, search, and sub-agent handler mixins."""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._subjects import (
    SUBJECT_RETRIEVAL_INDEX_RESULT,
    SUBJECT_RETRIEVAL_SEARCH_RESULT,
    SUBJECT_SUBAGENT_SEARCH_RESULT,
)
from codeforge.models import (
    RetrievalIndexRequest,
    RetrievalIndexResult,
    RetrievalSearchRequest,
    RetrievalSearchResult,
    SubAgentSearchRequest,
    SubAgentSearchResult,
)

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()


class RetrievalHandlerMixin:
    """Handles retrieval.index, retrieval.search, and retrieval.subagent messages."""

    async def _handle_retrieval_index(self, msg: nats.aio.msg.Msg) -> None:
        """Process a retrieval index request: build index and publish result."""
        await self._handle_request(
            msg,
            request_model=RetrievalIndexRequest,
            dedup_key=lambda r: f"retidx-{r.project_id}",
            handler=self._do_retrieval_index,
            result_subject=SUBJECT_RETRIEVAL_INDEX_RESULT,
            log_context=lambda r: {"project_id": r.project_id},
        )

    async def _do_retrieval_index(
        self, request: RetrievalIndexRequest, log: structlog.BoundLogger
    ) -> RetrievalIndexResult:
        """Business logic for retrieval index building."""
        log.info("received retrieval index request", workspace=request.workspace_path)
        status = await self._retriever.build_index(
            project_id=request.project_id,
            workspace_path=request.workspace_path,
            embedding_model=request.embedding_model,
            file_extensions=request.file_extensions or None,
        )
        return RetrievalIndexResult(
            project_id=status.project_id,
            status=status.status,
            file_count=status.file_count,
            chunk_count=status.chunk_count,
            embedding_model=status.embedding_model,
            error=status.error,
            incremental=status.incremental,
            files_changed=status.files_changed,
            files_unchanged=status.files_unchanged,
        )

    async def _handle_retrieval_search(self, msg: nats.aio.msg.Msg) -> None:
        """Process a retrieval search request: search index and publish result."""
        await self._handle_request(
            msg=msg,
            request_model=RetrievalSearchRequest,
            dedup_key=lambda r: f"retsearch-{r.request_id}",
            handler=self._do_retrieval_search,
            result_subject=SUBJECT_RETRIEVAL_SEARCH_RESULT,
            log_context=lambda r: {
                "project_id": r.project_id,
                "request_id": r.request_id,
                "scope_id": r.scope_id,
            },
        )

    async def _do_retrieval_search(
        self, request: RetrievalSearchRequest, log: structlog.BoundLogger
    ) -> RetrievalSearchResult:
        """Business logic for retrieval search."""
        log.info("received retrieval search request", query=request.query[:80])

        try:
            hits = await self._retriever.search(
                project_id=request.project_id,
                query=request.query,
                top_k=request.top_k,
                bm25_weight=request.bm25_weight,
                semantic_weight=request.semantic_weight,
            )
        except Exception:
            # Publish error result so the Go waiter gets a response, then re-raise
            # so _handle_request performs the nak.
            await self._publish_retrieval_search_error(request)
            raise

        result = RetrievalSearchResult(
            project_id=request.project_id,
            query=request.query,
            request_id=request.request_id,
            results=hits,
        )

        log.info("retrieval search completed", hits=len(hits))
        return result

    async def _publish_retrieval_search_error(self, request: RetrievalSearchRequest) -> None:
        """Publish an error result for retrieval search so Go waiter gets a response."""
        try:
            error_result = RetrievalSearchResult(
                project_id=request.project_id,
                query=request.query,
                request_id=request.request_id,
                error="internal worker error",
            )
            if self._js is not None:
                await self._js.publish(
                    SUBJECT_RETRIEVAL_SEARCH_RESULT,
                    error_result.model_dump_json().encode(),
                )
        except Exception as exc:
            logger.exception(
                "failed to publish retrieval search error result",
                error=str(exc),
            )

    async def _handle_subagent_search(self, msg: nats.aio.msg.Msg) -> None:
        """Process a sub-agent search request: expand, search, dedup, rerank, publish."""
        await self._handle_request(
            msg=msg,
            request_model=SubAgentSearchRequest,
            dedup_key=lambda r: f"subagent-{r.request_id}",
            handler=self._do_subagent_search,
            result_subject=SUBJECT_SUBAGENT_SEARCH_RESULT,
            log_context=lambda r: {
                "project_id": r.project_id,
                "request_id": r.request_id,
                "scope_id": r.scope_id,
            },
        )

    async def _do_subagent_search(
        self, request: SubAgentSearchRequest, log: structlog.BoundLogger
    ) -> SubAgentSearchResult:
        """Business logic for sub-agent search."""
        log.info("received subagent search request", query=request.query[:80])

        try:
            hits, expanded_queries, total_candidates = await self._subagent.search(
                project_id=request.project_id,
                query=request.query,
                top_k=request.top_k,
                max_queries=request.max_queries,
                model=request.model,
                rerank=request.rerank,
                expansion_prompt=request.expansion_prompt,
            )
        except Exception:
            # Publish error result so the Go waiter gets a response, then re-raise
            # so _handle_request performs the nak.
            await self._publish_subagent_search_error(request)
            raise

        cost = self._subagent.last_cost

        result = SubAgentSearchResult(
            project_id=request.project_id,
            query=request.query,
            request_id=request.request_id,
            results=hits,
            expanded_queries=expanded_queries,
            total_candidates=total_candidates,
            model=cost.model,
            tokens_in=cost.tokens_in,
            tokens_out=cost.tokens_out,
            cost_usd=cost.cost_usd,
        )

        log.info(
            "subagent search completed",
            hits=len(hits),
            queries=len(expanded_queries),
            candidates=total_candidates,
        )
        return result

    async def _publish_subagent_search_error(self, request: SubAgentSearchRequest) -> None:
        """Publish an error result for subagent search so Go waiter gets a response."""
        try:
            error_result = SubAgentSearchResult(
                project_id=request.project_id,
                query=request.query,
                request_id=request.request_id,
                error="internal worker error",
            )
            if self._js is not None:
                await self._js.publish(
                    SUBJECT_SUBAGENT_SEARCH_RESULT,
                    error_result.model_dump_json().encode(),
                )
        except Exception as exc:
            logger.exception(
                "failed to publish subagent search error result",
                error=str(exc),
            )
