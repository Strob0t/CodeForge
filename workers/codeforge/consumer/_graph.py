"""Graph build and search handler mixins."""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._subjects import SUBJECT_GRAPH_BUILD_RESULT, SUBJECT_GRAPH_SEARCH_RESULT
from codeforge.models import (
    GraphBuildRequest,
    GraphBuildResult,
    GraphSearchRequest,
    GraphSearchResult,
)

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()


class GraphHandlerMixin:
    """Handles graph.build and graph.search messages."""

    async def _handle_graph_build(self, msg: nats.aio.msg.Msg) -> None:
        """Process a graph build request: build code graph and publish result."""
        try:
            request = GraphBuildRequest.model_validate_json(msg.data)
            log = logger.bind(project_id=request.project_id, scope_id=request.scope_id)
            log.info("received graph build request", workspace=request.workspace_path)

            result: GraphBuildResult = await self._graph_builder.build_graph(
                project_id=request.project_id,
                workspace_path=request.workspace_path,
                db_url=self._db_url,
            )

            if self._js is not None:
                await self._js.publish(
                    SUBJECT_GRAPH_BUILD_RESULT,
                    result.model_dump_json().encode(),
                )

            await msg.ack()
            log.info(
                "graph build completed",
                status=result.status,
                nodes=result.node_count,
                edges=result.edge_count,
            )

        except Exception:
            logger.exception("failed to process graph build request")
            await msg.nak()

    async def _handle_graph_search(self, msg: nats.aio.msg.Msg) -> None:
        """Process a graph search request: search graph and publish result."""
        try:
            request = GraphSearchRequest.model_validate_json(msg.data)
            log = logger.bind(project_id=request.project_id, request_id=request.request_id, scope_id=request.scope_id)
            log.info("received graph search request", seeds=request.seed_symbols)

            hits = await self._graph_searcher.search(
                project_id=request.project_id,
                seed_symbols=request.seed_symbols,
                max_hops=request.max_hops,
                top_k=request.top_k,
                db_url=self._db_url,
            )

            result = GraphSearchResult(
                project_id=request.project_id,
                request_id=request.request_id,
                results=hits,
            )

            if self._js is not None:
                await self._js.publish(
                    SUBJECT_GRAPH_SEARCH_RESULT,
                    result.model_dump_json().encode(),
                )

            await msg.ack()
            log.info("graph search completed", hits=len(hits))

        except Exception:
            logger.exception("failed to process graph search request")
            await self._publish_graph_search_error(msg)

    async def _publish_graph_search_error(self, msg: nats.aio.msg.Msg) -> None:
        """Publish an error result for graph search so the Go waiter gets a response."""
        try:
            req = GraphSearchRequest.model_validate_json(msg.data)
            error_result = GraphSearchResult(
                project_id=req.project_id,
                request_id=req.request_id,
                error="internal worker error",
            )
            if self._js is not None:
                await self._js.publish(
                    SUBJECT_GRAPH_SEARCH_RESULT,
                    error_result.model_dump_json().encode(),
                )
        except Exception:
            logger.exception("failed to publish graph search error result")
        await msg.nak()
