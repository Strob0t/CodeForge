"""Task message handler mixin."""

from __future__ import annotations

import asyncio
import contextlib
import json
from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._subjects import HEADER_REQUEST_ID, SUBJECT_RESULT, SUBJECT_TASK_CANCEL
from codeforge.models import TaskMessage, TaskResult, TaskStatus
from codeforge.runtime import notification_consumer

if TYPE_CHECKING:
    import nats.aio.msg
    from nats.js.client import JetStreamContext

    from codeforge.backends._base import TaskResult as BackendTaskResult

logger = structlog.get_logger()


def _names_task(data: bytes, task_id: str) -> bool:
    """Whether a tasks.cancel message names *task_id*; a malformed one names no task."""
    try:
        payload = json.loads(data)
    except (ValueError, TypeError):  # invalid JSON or UTF-8, or no bytes at all
        return False
    return isinstance(payload, dict) and bool(task_id) and payload.get("task_id") == task_id


async def _cancel_on_request(
    sub: JetStreamContext.PushSubscription,
    task_id: str,
    execution: asyncio.Task[BackendTaskResult],
    requested: asyncio.Event,
) -> None:
    """Cancel *execution* when a tasks.cancel for *task_id* arrives."""
    while not execution.done():
        try:
            msg = await sub.next_msg(timeout=1.0)
        except TimeoutError:
            continue
        except Exception as exc:
            # The subscription is closed or the connection is gone.
            logger.debug("task cancel listener stopped", task_id=task_id, error=str(exc))
            return
        if _names_task(msg.data, task_id):
            logger.info("task cancelled by control plane", task_id=task_id)
            requested.set()
            execution.cancel()
            return


class TaskHandlerMixin:
    """Handles task.agent.* messages — backend router dispatch."""

    async def _handle_message(self, msg: nats.aio.msg.Msg) -> None:
        """Process a single task message: parse, execute via backend router, report the result.

        Acked on accept (at-most-once): a backend task (Aider, OpenHands, ...)
        changes the workspace and must not run again on a half-changed one.
        A failure is reported as a failed task result instead of being retried,
        a tasks.cancel for the task stops the backend and reports it cancelled.
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

                backend_result = await self._run_backend(task, backend_name, request_id)
                if backend_result is None:
                    result = TaskResult(
                        task_id=task.id,
                        tenant_id=task.tenant_id,
                        project_id=task.project_id,
                        status=TaskStatus.CANCELLED,
                        error="cancelled by user",
                    )
                else:
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

    async def _run_backend(self, task: TaskMessage, backend_name: str, request_id: str) -> BackendTaskResult | None:
        """Run *task* on its backend; None if a tasks.cancel for it stopped the run.

        The backend runs as a task of its own, so that a cancel stops it while
        this handler waits; cancelling it stops the backend's process group
        (CLI backends) or remote conversation (OpenHands). A cancellation of
        this handler (the worker stops) is passed on, not reported as a cancel.
        """
        if self._js is None:
            msg = "JetStream not available for the task cancel listener"
            raise RuntimeError(msg)
        # Every worker sees every cancel: the task may run on any of them.
        sub = await self._js.subscribe(SUBJECT_TASK_CANCEL, config=notification_consumer())
        execution = asyncio.create_task(
            self._backend_router.execute(
                backend_name=backend_name,
                task_id=task.id,
                prompt=task.prompt,
                workspace_path=task.workspace_path,
                config=task.config,
                on_output=lambda line: self._publish_output(task.id, line, "stdout", request_id, task.tenant_id),
            ),
            name=f"backend task {task.id}",
        )
        requested = asyncio.Event()
        listener = asyncio.create_task(_cancel_on_request(sub, task.id, execution, requested))
        try:
            return await execution
        except asyncio.CancelledError:
            current = asyncio.current_task()
            if requested.is_set() and (current is None or not current.cancelling()):
                return None
            raise
        finally:
            listener.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await listener
            try:
                await sub.unsubscribe()
            except Exception as exc:
                logger.warning("task cancel listener unsubscribe failed", task_id=task.id, error=str(exc))
