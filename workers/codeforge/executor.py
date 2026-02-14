"""Agent executor stub for processing tasks."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from codeforge.models import TaskMessage, TaskResult, TaskStatus

if TYPE_CHECKING:
    from codeforge.llm import LiteLLMClient

logger = logging.getLogger(__name__)


class AgentExecutor:
    """Stub executor that receives a task, calls LLM, and returns a result."""

    def __init__(self, llm: LiteLLMClient) -> None:
        self._llm = llm

    async def execute(self, task: TaskMessage) -> TaskResult:
        """Execute a task by sending the prompt to the LLM."""
        logger.info("executing task %s: %s", task.id, task.title)

        try:
            response = await self._llm.completion(
                prompt=task.prompt,
                system=f"You are working on task: {task.title}",
            )

            return TaskResult(
                task_id=task.id,
                status=TaskStatus.COMPLETED,
                output=response.content,
                tokens_in=response.tokens_in,
                tokens_out=response.tokens_out,
            )
        except Exception as exc:
            logger.exception("task %s failed", task.id)
            return TaskResult(
                task_id=task.id,
                status=TaskStatus.FAILED,
                error=str(exc),
            )
