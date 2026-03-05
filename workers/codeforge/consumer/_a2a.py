"""A2A (Agent-to-Agent) task handler mixin."""

from __future__ import annotations

import json
from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._subjects import (
    SUBJECT_A2A_TASK_COMPLETE,
)
from codeforge.models import A2ATaskCompleteMessage, A2ATaskCreatedMessage

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()


class A2AHandlerMixin:
    """Handles a2a.task.created and a2a.task.cancel messages."""

    async def _handle_a2a_task_created(self, msg: nats.aio.msg.Msg) -> None:
        """Process an inbound A2A task: run agent and publish completion."""
        try:
            req = A2ATaskCreatedMessage.model_validate_json(msg.data)
            log = logger.bind(task_id=req.task_id, skill_id=req.skill_id)

            if self._is_duplicate(f"a2a-{req.task_id}"):
                log.warning("duplicate A2A task, skipping")
                await msg.ack()
                return

            log.info("received A2A task", prompt_len=len(req.prompt))

            # Execute via the simple executor path.
            from codeforge.models import TaskMessage

            task = TaskMessage(
                id=req.task_id,
                project_id="",
                title=f"A2A task: {req.task_id}",
                prompt=req.prompt,
            )
            result = await self._executor.execute(task)

            # Publish completion back to Go.
            state = "completed" if result.status.value == "completed" else "failed"
            complete = A2ATaskCompleteMessage(
                task_id=req.task_id,
                state=state,
                error=result.error or "",
            )
            if self._js is not None:
                await self._js.publish(
                    SUBJECT_A2A_TASK_COMPLETE,
                    complete.model_dump_json().encode(),
                )

            await msg.ack()
            log.info("A2A task completed", state=state)

        except Exception as exc:
            logger.exception("failed to process A2A task", error=str(exc))
            # Publish failure completion so Go side knows.
            try:
                req = A2ATaskCreatedMessage.model_validate_json(msg.data)
                if self._js is not None:
                    fail = A2ATaskCompleteMessage(
                        task_id=req.task_id,
                        state="failed",
                        error=str(exc),
                    )
                    await self._js.publish(
                        SUBJECT_A2A_TASK_COMPLETE,
                        fail.model_dump_json().encode(),
                    )
            except Exception as exc:
                logger.debug("failed to publish A2A error result", error=str(exc))
            await msg.ack()

    async def _handle_a2a_task_cancel(self, msg: nats.aio.msg.Msg) -> None:
        """Handle an A2A task cancellation request."""
        try:
            data = json.loads(msg.data)
            task_id = data.get("task_id", "")
            log = logger.bind(task_id=task_id)
            log.info("received A2A task cancel")

            # For now, acknowledge the cancel — the runtime cancel mechanism
            # handles stopping the actual execution via is_cancelled flag.
            # Future: look up the running task and trigger cancellation.

            await msg.ack()
            log.info("A2A task cancel acknowledged")

        except Exception as exc:
            logger.exception("failed to process A2A task cancel", error=str(exc))
            await msg.ack()
