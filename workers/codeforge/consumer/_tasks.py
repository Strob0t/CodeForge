"""Task message handler mixin."""

from __future__ import annotations

import asyncio
import contextlib
from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._cancel_registry import task_key
from codeforge.consumer._delivery import stream_sequence
from codeforge.consumer._subjects import HEADER_REQUEST_ID, SUBJECT_RESULT, SUBJECT_TASK_CANCEL, SUBJECT_TASK_HEARTBEAT
from codeforge.models import TaskMessage, TaskResult, TaskStatus
from codeforge.runtime import heartbeat_interval, heartbeats, listen_for_cancel, notification_consumer

if TYPE_CHECKING:
    from collections.abc import Callable

    import nats.aio.msg
    from nats.js.client import JetStreamContext

    from codeforge.backends._base import TaskResult as BackendTaskResult

logger = structlog.get_logger()


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

        # A task can be dispatched again: its messages differ in their stream
        # position, the redeliveries of one message share it.
        dispatch = stream_sequence(msg)
        dedup_key = f"task-{task.id}" if dispatch is None else f"task-{task.id}@{dispatch}"
        if self._is_duplicate(dedup_key):
            log.warning("duplicate task message, skipping")
            await msg.ack()
            return

        if dispatch is not None and self._cancels.cancelled(task_key(task.id), dispatch):
            # Go marked the task cancelled when it published the cancel; a
            # result here could overwrite the state of a later dispatch.
            log.info("task cancelled while it waited for a worker, skipping")
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
            await self._run_task(task, backend_name, request_id, failed, log, dispatch)

    async def _run_task(
        self,
        task: TaskMessage,
        backend_name: str,
        request_id: str,
        failed: Callable[[str], TaskResult],
        log: structlog.BoundLogger,
        dispatch: int | None,
    ) -> None:
        """Execute an accepted task and publish its result, reporting it alive meanwhile."""
        beat = {"task_id": task.id, "tenant_id": task.tenant_id}
        alive = (
            heartbeats(self._js, SUBJECT_TASK_HEARTBEAT, beat, heartbeat_interval(task.heartbeat_seconds))
            if self._js is not None
            else contextlib.nullcontext()
        )
        async with alive:
            try:
                log.info("received task", title=task.title)

                await self._publish_output(
                    task.id, f"Starting task: {task.title}", "stdout", request_id, task.tenant_id
                )

                backend_result = await self._run_backend(task, backend_name, request_id, dispatch)
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

    async def _run_backend(
        self, task: TaskMessage, backend_name: str, request_id: str, dispatch: int | None = None
    ) -> BackendTaskResult | None:
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
        # The listener replays the cancels published after the task's own
        # message (one stream), so a cancel published while it subscribes
        # is not lost.
        sub = await self._js.subscribe(SUBJECT_TASK_CANCEL, config=notification_consumer(after=dispatch))
        if dispatch is not None and self._cancels.cancelled(task_key(task.id), dispatch):
            # Cancelled after the check in _handle_message, already seen by
            # the registry.
            logger.info("task cancelled by control plane before it started", task_id=task.id)
            await self._unsubscribe_task_cancel(sub, task.id)
            return None
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

        def cancel_execution() -> None:
            logger.info("task cancelled by control plane", task_id=task.id)
            requested.set()
            execution.cancel()

        listener = asyncio.create_task(
            listen_for_cancel(
                sub,
                lambda _run_id, task_id: bool(task_id) and task_id == task.id,
                cancel_execution,
                until=execution.done,
            )
        )
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
            await self._unsubscribe_task_cancel(sub, task.id)

    @staticmethod
    async def _unsubscribe_task_cancel(sub: JetStreamContext.PushSubscription, task_id: str) -> None:
        try:
            await sub.unsubscribe()
        except Exception as exc:
            logger.warning("task cancel listener unsubscribe failed", task_id=task_id, error=str(exc))
