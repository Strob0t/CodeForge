"""Task message handler mixin."""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._subjects import HEADER_REQUEST_ID, MAX_RETRIES, SUBJECT_RESULT
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

        try:
            task = TaskMessage.model_validate_json(msg.data)

            backend_name = msg.subject.rsplit(".", 1)[-1] if msg.subject else "unknown"
            log = log.bind(task_id=task.id, backend=backend_name)
            log.info("received task", title=task.title)

            await self._publish_output(task.id, f"Starting task: {task.title}", "stdout", request_id)

            backend_result: BackendTaskResult = await self._backend_router.execute(
                backend_name=backend_name,
                task_id=task.id,
                prompt=task.prompt,
                workspace_path=task.config.get("workspace_path", ""),
                config=task.config,
                on_output=lambda line: self._publish_output(task.id, line, "stdout", request_id),
            )

            result = TaskResult(
                task_id=task.id,
                status=TaskStatus.COMPLETED if backend_result.status == "completed" else TaskStatus.FAILED,
                output=backend_result.output,
                error=backend_result.error,
            )

            if self._js is not None:
                await self._js.publish(SUBJECT_RESULT, result.model_dump_json().encode())

            await msg.ack()
            log.info("task completed", status=result.status, backend=backend_name)

        except Exception:
            retries = self._retry_count(msg)
            log.exception("failed to process message", retry=retries)

            if retries >= MAX_RETRIES:
                log.warning("max retries reached, moving to DLQ", retry=retries)
                await self._move_to_dlq(msg)
            else:
                await msg.nak()
