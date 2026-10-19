"""Retrieval index, search, and sub-agent handler mixins."""

from __future__ import annotations

import uuid
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
from codeforge.workspace_fs import PathLeavesWorkspaceError, WorkspaceRoot

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()

# The one answer for a knowledge_path outside the tenant's area or missing (KI-105).
_KNOWLEDGE_UNAVAILABLE = (
    "knowledge_path is not available in this tenant's knowledge area (knowledge.content_root/<tenant>)"
)


def _is_tenant_id(value: str) -> bool:
    """Whether *value* is a tenant ID in canonical UUID form (one path component)."""
    try:
        return str(uuid.UUID(value)) == value
    except ValueError:
        return False


class RetrievalHandlerMixin:
    """Handles retrieval.index, retrieval.search, and retrieval.subagent messages."""

    # Knowledge bases are indexed below this directory only (KI-105); the
    # consumer sets it from knowledge.content_root.
    _knowledge_content_root: str = ""

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
        log.info(
            "received retrieval index request",
            workspace=request.workspace_path,
            knowledge_path=request.knowledge_path,
        )
        workspace_path, below = request.workspace_path, "."
        if request.project_id.startswith("kb:") or request.knowledge_path:
            refusal = self._knowledge_refusal(request)
            if refusal:
                log.warning("knowledge index refused", reason=refusal)
                return RetrievalIndexResult(project_id=request.project_id, status="error", error=refusal)
            workspace_path, tenant, below = self._knowledge_content_root, request.tenant_id, request.knowledge_path
        elif not workspace_path.strip():
            # An empty path would index the worker's working directory.
            refusal = "workspace_path is required for a project index"
            log.warning("retrieval index refused", reason=refusal)
            return RetrievalIndexResult(project_id=request.project_id, status="error", error=refusal)
        else:
            tenant = ""
        status = await self._retriever.build_index(
            project_id=request.project_id,
            workspace_path=workspace_path,
            embedding_model=request.embedding_model,
            file_extensions=request.file_extensions or None,
            tenant=tenant,
            below=below,
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
            bm25_only=status.bm25_only,
        )

    def _knowledge_refusal(self, request: RetrievalIndexRequest) -> str:
        """Why a knowledge-base index request is refused, or "" (KI-105).

        A knowledge base is indexed from knowledge_path inside its tenant's
        area <content root>/<tenant_id>/ of the worker's own knowledge content
        root, never from a workspace_path or a path (or a symlink) leading out
        of that area. A path outside the area and a missing one get the same
        answer, so nothing can be probed.
        """
        if not request.project_id.startswith("kb:"):
            return "knowledge_path is only accepted for knowledge bases"
        if request.workspace_path:
            return "knowledge bases are indexed below the knowledge content root, not from workspace_path"
        if not request.knowledge_path:
            return "knowledge_path is required"
        if not _is_tenant_id(request.tenant_id):
            return "a knowledge index request needs the tenant_id of its knowledge base"
        if not self._knowledge_content_root:
            return "no knowledge content root is configured (knowledge.content_root)"
        try:
            if request.knowledge_path.startswith("/"):
                raise PathLeavesWorkspaceError(request.knowledge_path)
            with (
                WorkspaceRoot.operator_dir(self._knowledge_content_root) as content,
                content.subroot(request.tenant_id) as area,
            ):
                area.resolve(request.knowledge_path)
        except OSError as exc:
            logger.info(
                "knowledge path refused",
                tenant_id=request.tenant_id,
                knowledge_path=request.knowledge_path,
                reason=str(exc),
            )
            return _KNOWLEDGE_UNAVAILABLE
        return ""

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
        except Exception as exc:
            # The error result answers the Go waiter and settles the request:
            # repeating the search after Go got its answer would only cost money.
            logger.error("retrieval search failed", error=str(exc))
            return RetrievalSearchResult(
                project_id=request.project_id,
                query=request.query,
                request_id=request.request_id,
                error="internal worker error",
            )

        result = RetrievalSearchResult(
            project_id=request.project_id,
            query=request.query,
            request_id=request.request_id,
            results=hits,
        )

        log.info("retrieval search completed", hits=len(hits))
        return result

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
        except Exception as exc:
            # The error result answers the Go waiter and settles the request:
            # repeating the LLM query expansion after Go got its answer would only cost money.
            logger.error("subagent search failed", error=str(exc))
            return SubAgentSearchResult(
                project_id=request.project_id,
                query=request.query,
                request_id=request.request_id,
                error="internal worker error",
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

        log.info(
            "subagent search completed",
            hits=len(hits),
            queries=len(expanded_queries),
            candidates=total_candidates,
        )
        return result
