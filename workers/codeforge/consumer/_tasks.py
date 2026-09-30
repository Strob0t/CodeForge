"""Task message handler mixin."""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._delivery import delivery_attempt
from codeforge.consumer._subjects import HEADER_REQUEST_ID, SUBJECT_RESULT
from codeforge.models import TaskMessage, TaskResult, TaskStatus

if TYPE_CHECKING:
    import nats.aio.msg

    from codeforge.backends._base import TaskResult as BackendTaskResult

logger = structlog.get_logger()


class TaskHandlerMixin:
    """Handles task.agent.* messages — backend router dispatch."""

    async def _handle_message(self, msg: nats.aio.msg.Msg) -> None:
        """Process a single task message: parse, execute via backend router, ack/nack."""
        request_id = ""
        if msg.headers and HEADER_REQUEST_ID in msg.headers:
            request_id = msg.headers[HEADER_REQUEST_ID]

        log = logger.bind(request_id=request_id) if request_id else logger

        task = await self._parse_request(msg, TaskMessage)
        if task is None:
            return

        backend_name = msg.subject.rsplit(".", 1)[-1] if msg.subject else "unknown"
        log = log.bind(task_id=task.id, backend=backend_name)

        dedup_key = f"task-{task.id}"
        if self._is_duplicate(dedup_key):
            log.warning("duplicate task message, skipping")
            await msg.ack()
            return

        try:
            log.info("received task", title=task.title)

            await self._publish_output(task.id, f"Starting task: {task.title}", "stdout", request_id, task.tenant_id)

            backend_result: BackendTaskResult = await self._backend_router.execute(
                backend_name=backend_name,
                task_id=task.id,
                prompt=task.prompt,
                workspace_path=task.config.get("workspace_path", ""),
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

            if self._js is not None:
                await self._js.publish(SUBJECT_RESULT, result.model_dump_json().encode())

            await msg.ack()
            log.info("task completed", status=result.status, backend=backend_name)

        except Exception as exc:
            log.exception("failed to process message", attempt=delivery_attempt(msg), error=str(exc))
            # Not processed: the redelivery must not be skipped as a duplicate.
            self._clear_processed(dedup_key)
            await self._retry_or_dead_letter(msg)
