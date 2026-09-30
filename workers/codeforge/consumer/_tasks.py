"""Task message handler mixin."""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._subjects import HEADER_REQUEST_ID, SUBJECT_RESULT
from codeforge.models import TaskMessage, TaskResult, TaskStatus

if TYPE_CHECKING:
    import nats.aio.msg

    from codeforge.backends._base import TaskResult as BackendTaskResult

logger = structlog.get_logger()


class TaskHandlerMixin:
    """Handles task.agent.* messages — backend router dispatch."""

    async def _handle_message(self, msg: nats.aio.msg.Msg) -> None:
        """Process a single task message: parse, execute via backend router, report the result.

        Acked on accept (at-most-once): a backend task (Aider, OpenHands, ...)
        changes the workspace and must not run again on a half-changed one.
        A failure is reported as a failed task result instead of being retried.
        """
        request_id = ""
        if msg.headers and HEADER_REQUEST_ID in msg.headers:
            request_id = msg.headers[HEADER_REQUEST_ID]

        log = logger.bind(request_id=request_id) if request_id else logger

        task = await self._parse_request(msg, TaskMessage)
        if task is None:
            return

        # The subject suffix routes the message; tasks published before the
        # payload named the backend only have that.
        backend_name = task.backend or (msg.subject.rsplit(".", 1)[-1] if msg.subject else "unknown")
        log = log.bind(task_id=task.id, backend=backend_name)

        dedup_key = f"task-{task.id}"
        if self._is_duplicate(dedup_key):
            log.warning("duplicate task message, skipping")
            await msg.ack()
            return

        if not await self._accept(msg):
            self._clear_processed(dedup_key)
            return

        def failed(error: str) -> TaskResult:
            return TaskResult(
                task_id=task.id,
                tenant_id=task.tenant_id,
                project_id=task.project_id,
                status=TaskStatus.FAILED,
                error=error,
            )

        async def report_failure(reason: str) -> None:
            await self._publish_result(failed(reason), SUBJECT_RESULT)

        with self._in_flight.track(f"task {task.id}", report_failure):
            try:
                log.info("received task", title=task.title)

                await self._publish_output(
                    task.id, f"Starting task: {task.title}", "stdout", request_id, task.tenant_id
                )

                backend_result: BackendTaskResult = await self._backend_router.execute(
                    backend_name=backend_name,
                    task_id=task.id,
                    prompt=task.prompt,
                    workspace_path=task.workspace_path,
                    config=task.config,
                    on_output=lambda line: self._publish_output(task.id, line, "stdout", request_id, task.tenant_id),
                )
                result = TaskResult(
                    task_id=task.id,
                    tenant_id=task.tenant_id,
                    project_id=task.project_id,
                    status=TaskStatus.COMPLETED if backend_result.status == "completed" else TaskStatus.FAILED,
                    output=backend_result.output,
                    error=backend_result.error,
                )
            except Exception as exc:
                log.exception("task failed", error=str(exc))
                result = failed(str(exc))

            await self._publish_result(result, SUBJECT_RESULT)
        log.info("task completed", status=result.status, backend=backend_name)
