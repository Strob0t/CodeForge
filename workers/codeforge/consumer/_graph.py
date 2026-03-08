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
        await self._handle_request(
            msg=msg,
            request_model=GraphBuildRequest,
            dedup_key=lambda r: f"graphbuild-{r.project_id}-{r.scope_id}",
            handler=self._do_graph_build,
            result_subject=SUBJECT_GRAPH_BUILD_RESULT,
            log_context=lambda r: {"project_id": r.project_id, "scope_id": r.scope_id},
        )

    async def _do_graph_build(self, request: GraphBuildRequest, log: structlog.BoundLogger) -> GraphBuildResult:
        """Business logic for graph building."""
        log.info("received graph build request", workspace=request.workspace_path)

        result: GraphBuildResult = await self._graph_builder.build_graph(
            project_id=request.project_id,
            workspace_path=request.workspace_path,
            db_url=self._db_url,
        )

        log.info(
            "graph build completed",
            status=result.status,
            nodes=result.node_count,
            edges=result.edge_count,
        )
        return result

    async def _handle_graph_search(self, msg: nats.aio.msg.Msg) -> None:
        """Process a graph search request: search graph and publish result."""
        await self._handle_request(
            msg=msg,
            request_model=GraphSearchRequest,
            dedup_key=lambda r: f"graphsearch-{r.request_id}",
            handler=self._do_graph_search,
            result_subject=SUBJECT_GRAPH_SEARCH_RESULT,
            log_context=lambda r: {
                "project_id": r.project_id,
                "request_id": r.request_id,
                "scope_id": r.scope_id,
            },
        )

    async def _do_graph_search(self, request: GraphSearchRequest, log: structlog.BoundLogger) -> GraphSearchResult:
        """Business logic for graph searching."""
        log.info("received graph search request", seeds=request.seed_symbols)

        try:
            hits = await self._graph_searcher.search(
                project_id=request.project_id,
                seed_symbols=request.seed_symbols,
                max_hops=request.max_hops,
                top_k=request.top_k,
                db_url=self._db_url,
            )
        except Exception:
            # Publish error result so the Go waiter gets a response, then re-raise
            # so _handle_request performs the nak.
            await self._publish_graph_search_error(request)
            raise

        result = GraphSearchResult(
            project_id=request.project_id,
            request_id=request.request_id,
            results=hits,
        )

        log.info("graph search completed", hits=len(hits))
        return result

    async def _publish_graph_search_error(self, request: GraphSearchRequest) -> None:
        """Publish an error result for graph search so the Go waiter gets a response."""
        try:
            error_result = GraphSearchResult(
                project_id=request.project_id,
                request_id=request.request_id,
                error="internal worker error",
            )
            if self._js is not None:
                await self._js.publish(
                    SUBJECT_GRAPH_SEARCH_RESULT,
                    error_result.model_dump_json().encode(),
                )
        except Exception as exc:
            logger.exception("failed to publish graph search error result", error=str(exc))
