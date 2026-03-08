"""Repomap handler mixin."""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._subjects import SUBJECT_REPOMAP_RESULT
from codeforge.models import RepoMapRequest, RepoMapResult

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()


class RepoMapHandlerMixin:
    """Handles repomap.generate.request messages."""

    async def _handle_repomap(self, msg: nats.aio.msg.Msg) -> None:
        """Process a repo map request: generate map and publish result."""
        await self._handle_request(
            msg=msg,
            request_model=RepoMapRequest,
            dedup_key=lambda r: f"repomap-{r.project_id}",
            handler=self._do_repomap,
            result_subject=SUBJECT_REPOMAP_RESULT,
            log_context=lambda r: {"project_id": r.project_id},
        )

    async def _do_repomap(self, request: RepoMapRequest, log: structlog.BoundLogger) -> RepoMapResult:
        """Business logic for repo map generation."""
        log.info("received repomap request", workspace=request.workspace_path)

        self._repomap_generator._token_budget = request.token_budget
        result: RepoMapResult = await self._repomap_generator.generate(
            workspace_path=request.workspace_path,
            active_files=request.active_files,
        )
        result = result.model_copy(update={"project_id": request.project_id})

        log.info(
            "repomap generated",
            files=result.file_count,
            symbols=result.symbol_count,
            tokens=result.token_count,
        )
        return result
