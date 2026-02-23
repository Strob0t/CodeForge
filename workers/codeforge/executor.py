"""Agent executor stub for processing tasks."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from codeforge.llm import resolve_scenario
from codeforge.models import ModeConfig, TaskMessage, TaskResult, TaskStatus
from codeforge.pricing import resolve_cost

if TYPE_CHECKING:
    from codeforge.llm import LiteLLMClient
    from codeforge.runtime import RuntimeClient

logger = logging.getLogger(__name__)


class AgentExecutor:
    """Stub executor that receives a task, calls LLM, and returns a result."""

    def __init__(self, llm: LiteLLMClient) -> None:
        self._llm = llm

    async def execute(self, task: TaskMessage) -> TaskResult:
        """Execute a task by sending the prompt to the LLM (fire-and-forget path)."""
        logger.info("executing task %s: %s", task.id, task.title)

        try:
            response = await self._llm.completion(
                prompt=task.prompt,
                system=f"You are working on task: {task.title}",
            )

            cost = resolve_cost(
                response.cost_usd,
                response.model,
                response.tokens_in,
                response.tokens_out,
            )
            return TaskResult(
                task_id=task.id,
                status=TaskStatus.COMPLETED,
                output=response.content,
                tokens_in=response.tokens_in,
                tokens_out=response.tokens_out,
                cost_usd=cost,
            )
        except Exception as exc:
            logger.exception("task %s failed", task.id)
            return TaskResult(
                task_id=task.id,
                status=TaskStatus.FAILED,
                error=str(exc),
            )

    async def execute_with_runtime(
        self,
        task: TaskMessage,
        runtime: RuntimeClient,
        mode: ModeConfig | None = None,
    ) -> None:
        """Execute a task using the step-by-step runtime protocol.

        Instead of fire-and-forget, each tool call is individually approved
        by the control plane before execution.
        """
        logger.info("executing task %s with runtime protocol: %s", task.id, task.title)
        await runtime.send_output(f"Starting task: {task.title}")

        # Build system prompt from mode or fallback to generic prompt
        system_prompt = mode.prompt_prefix if mode and mode.prompt_prefix else f"You are working on task: {task.title}"

        # Resolve scenario for model routing and temperature
        scenario_tag = mode.llm_scenario if mode and mode.llm_scenario else "default"
        scenario_cfg = resolve_scenario(scenario_tag)
        logger.info(
            "llm_routing_decision run_id=%s mode=%s scenario=%s temperature=%.2f",
            runtime.run_id,
            mode.id if mode else "",
            scenario_cfg.tag,
            scenario_cfg.temperature,
        )

        try:
            # Request permission for LLM call
            decision = await runtime.request_tool_call(
                tool="LLM",
                command="completion",
            )

            if decision.decision != "allow":
                logger.warning(
                    "LLM call denied by policy: %s",
                    decision.reason,
                )
                await runtime.complete_run(
                    status="failed",
                    error=f"LLM call denied: {decision.reason}",
                )
                return

            # Execute the LLM call with scenario-based routing
            response = await self._llm.completion(
                prompt=task.prompt,
                system=system_prompt,
                temperature=scenario_cfg.temperature,
                tags=[scenario_cfg.tag],
            )

            # Report result with real cost and tokens
            cost = resolve_cost(
                response.cost_usd,
                response.model,
                response.tokens_in,
                response.tokens_out,
            )
            await runtime.report_tool_result(
                call_id=decision.call_id,
                tool="LLM",
                success=True,
                output=response.content[:200],
                cost_usd=cost,
                tokens_in=response.tokens_in,
                tokens_out=response.tokens_out,
                model=response.model,
            )

            if runtime.is_cancelled:
                await runtime.complete_run(status="cancelled", error="cancelled by user")
                return

            await runtime.complete_run(
                status="completed",
                output=response.content,
            )

        except Exception as exc:
            logger.exception("task %s failed in runtime mode", task.id)
            await runtime.complete_run(
                status="failed",
                error=str(exc),
            )
