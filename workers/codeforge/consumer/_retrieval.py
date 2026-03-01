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
        try:
            request = RetrievalIndexRequest.model_validate_json(msg.data)
            log = logger.bind(project_id=request.project_id)
            log.info("received retrieval index request", workspace=request.workspace_path)

            status = await self._retriever.build_index(
                project_id=request.project_id,
                workspace_path=request.workspace_path,
                embedding_model=request.embedding_model,
                file_extensions=request.file_extensions or None,
            )

            result = RetrievalIndexResult(
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

            if self._js is not None:
                await self._js.publish(
                    SUBJECT_RETRIEVAL_INDEX_RESULT,
                    result.model_dump_json().encode(),
                )

            await msg.ack()
            log.info(
                "retrieval index built",
                status=result.status,
                files=result.file_count,
                chunks=result.chunk_count,
            )

        except Exception:
            logger.exception("failed to process retrieval index request")
            await msg.nak()

    async def _handle_retrieval_search(self, msg: nats.aio.msg.Msg) -> None:
        """Process a retrieval search request: search index and publish result."""
        try:
            request = RetrievalSearchRequest.model_validate_json(msg.data)
            log = logger.bind(project_id=request.project_id, request_id=request.request_id, scope_id=request.scope_id)
            log.info("received retrieval search request", query=request.query[:80])

            hits = await self._retriever.search(
                project_id=request.project_id,
                query=request.query,
                top_k=request.top_k,
                bm25_weight=request.bm25_weight,
                semantic_weight=request.semantic_weight,
            )

            result = RetrievalSearchResult(
                project_id=request.project_id,
                query=request.query,
                request_id=request.request_id,
                results=hits,
            )

            if self._js is not None:
                await self._js.publish(
                    SUBJECT_RETRIEVAL_SEARCH_RESULT,
                    result.model_dump_json().encode(),
                )

            await msg.ack()
            log.info("retrieval search completed", hits=len(hits))

        except Exception:
            logger.exception("failed to process retrieval search request")
            await self._publish_error_result(
                msg,
                RetrievalSearchRequest,
                RetrievalSearchResult,
                SUBJECT_RETRIEVAL_SEARCH_RESULT,
            )

    async def _handle_subagent_search(self, msg: nats.aio.msg.Msg) -> None:
        """Process a sub-agent search request: expand, search, dedup, rerank, publish."""
        try:
            request = SubAgentSearchRequest.model_validate_json(msg.data)
            log = logger.bind(project_id=request.project_id, request_id=request.request_id, scope_id=request.scope_id)
            log.info("received subagent search request", query=request.query[:80])

            hits, expanded_queries, total_candidates = await self._subagent.search(
                project_id=request.project_id,
                query=request.query,
                top_k=request.top_k,
                max_queries=request.max_queries,
                model=request.model,
                rerank=request.rerank,
                expansion_prompt=request.expansion_prompt,
            )
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

            if self._js is not None:
                await self._js.publish(
                    SUBJECT_SUBAGENT_SEARCH_RESULT,
                    result.model_dump_json().encode(),
                )

            await msg.ack()
            log.info(
                "subagent search completed",
                hits=len(hits),
                queries=len(expanded_queries),
                candidates=total_candidates,
            )

        except Exception:
            logger.exception("failed to process subagent search request")
            await self._publish_error_result(
                msg,
                SubAgentSearchRequest,
                SubAgentSearchResult,
                SUBJECT_SUBAGENT_SEARCH_RESULT,
            )
