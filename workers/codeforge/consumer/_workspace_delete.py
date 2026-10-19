"""Workspace deletion handler mixin (KI-96 D11): a deleted project's workspace, removed as the tenant's tool UID."""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._subjects import SUBJECT_WORKSPACE_DELETE_RESULT
from codeforge.models import WorkspaceDeleteRequest, WorkspaceDeleteResult
from codeforge.workspace_deletion import delete_workspace

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()


class WorkspaceDeleteHandlerMixin:
    """Handles workspace.delete.request messages (at-least-once; removing twice is harmless)."""

    async def _handle_workspace_delete(self, msg: nats.aio.msg.Msg) -> None:
        await self._handle_request(  # type: ignore[attr-defined]
            msg=msg,
            request_model=WorkspaceDeleteRequest,
            dedup_key=lambda r: f"wsdelete-{r.deletion_id}",
            handler=self._do_workspace_delete,
            result_subject=SUBJECT_WORKSPACE_DELETE_RESULT,
            log_context=lambda r: {"deletion_id": r.deletion_id, "project_id": r.project_id},
        )

    async def _do_workspace_delete(
        self, request: WorkspaceDeleteRequest, log: structlog.BoundLogger
    ) -> WorkspaceDeleteResult:
        result = WorkspaceDeleteResult(deletion_id=request.deletion_id, tenant_id=request.tenant_id)
        try:
            await delete_workspace(request.tenant_id, request.tool_uid, request.workspace_path)
        except OSError as exc:  # ToolIsolationError (refused) included
            log.error("workspace deletion failed", error=str(exc))
            result.error = str(exc)
            return result
        log.info("workspace deleted", workspace=request.workspace_path)
        result.ok = True
        return result
