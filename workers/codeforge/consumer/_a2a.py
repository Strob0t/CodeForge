"""A2A (Agent-to-Agent) task handler mixin."""

from __future__ import annotations

import json
from typing import TYPE_CHECKING

import structlog

from codeforge.a2a_protocol import A2ATaskState
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

            # Publish "working" state transition before execution.
            if self._js is not None:
                working_msg = A2ATaskCompleteMessage(
                    task_id=req.task_id,
                    tenant_id=req.tenant_id,
                    state=A2ATaskState.WORKING,
                )
                await self._js.publish(
                    SUBJECT_A2A_TASK_COMPLETE,
                    working_msg.model_dump_json().encode(),
                )

            # Execute via the simple executor path.
            from codeforge.models import TaskMessage

            task = TaskMessage(
                id=req.task_id,
                project_id="",
                title=f"A2A task: {req.task_id}",
                prompt=req.prompt,
            )
            result = await self._executor.execute(task)

            # Publish completion back to Go using validated enum state.
            state = A2ATaskState.COMPLETED if result.status.value == "completed" else A2ATaskState.FAILED
            complete = A2ATaskCompleteMessage(
                task_id=req.task_id,
                tenant_id=req.tenant_id,
                state=state,
                error=result.error or "",
            )
            if self._js is not None:
                stamped = self._stamp_trust(complete.model_dump())
                await self._js.publish(
                    SUBJECT_A2A_TASK_COMPLETE,
                    json.dumps(stamped).encode(),
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
                        tenant_id=req.tenant_id,
                        state=A2ATaskState.FAILED,
                        error=str(exc),
                    )
                    stamped = self._stamp_trust(fail.model_dump())
                    await self._js.publish(
                        SUBJECT_A2A_TASK_COMPLETE,
                        json.dumps(stamped).encode(),
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
