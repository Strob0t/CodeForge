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
        try:
            request = RepoMapRequest.model_validate_json(msg.data)
            log = logger.bind(project_id=request.project_id)
            log.info("received repomap request", workspace=request.workspace_path)

            self._repomap_generator._token_budget = request.token_budget
            result: RepoMapResult = await self._repomap_generator.generate(
                workspace_path=request.workspace_path,
                active_files=request.active_files,
            )
            result = result.model_copy(update={"project_id": request.project_id})

            if self._js is not None:
                await self._js.publish(
                    SUBJECT_REPOMAP_RESULT,
                    result.model_dump_json().encode(),
                )

            await msg.ack()
            log.info(
                "repomap generated",
                files=result.file_count,
                symbols=result.symbol_count,
                tokens=result.token_count,
            )

        except Exception:
            logger.exception("failed to process repomap request")
            await msg.nak()
