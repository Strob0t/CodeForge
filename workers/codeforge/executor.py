"""Agent executor for processing tasks."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from codeforge.llm import resolve_scenario
from codeforge.mcp_workbench import McpWorkbench
from codeforge.models import ModeConfig, TaskMessage, TaskResult, TaskStatus
from codeforge.pricing import resolve_cost
from codeforge.tracing.setup import TracingManager

if TYPE_CHECKING:
    from codeforge.llm import LiteLLMClient
    from codeforge.mcp_models import MCPServerDef
    from codeforge.runtime import RuntimeClient

logger = logging.getLogger(__name__)

_tracing = TracingManager()
_tracer = _tracing.get_tracer()


class AgentExecutor:
    """Executor that receives a task, calls LLM, and returns a result."""

    def __init__(self, llm: LiteLLMClient) -> None:
        self._llm = llm

    @_tracer.trace_agent("executor")
    async def execute(self, task: TaskMessage) -> TaskResult:
        """Execute a task by sending the prompt to the LLM (fire-and-forget path)."""
        logger.info("executing task %s: %s", task.id, task.title)

        model = task.config.get("model", "")
        try:
            response = await self._llm.completion(
                prompt=task.prompt,
                system=f"You are working on task: {task.title}",
                **({"model": model} if model else {}),
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

    @_tracer.trace_agent("executor")
    async def execute_with_runtime(
        self,
        task: TaskMessage,
        runtime: RuntimeClient,
        mode: ModeConfig | None = None,
        mcp_servers: list[MCPServerDef] | None = None,
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

        workbench: McpWorkbench | None = None
        try:
            # Set up MCP workbench if servers are configured
            if mcp_servers:
                workbench = McpWorkbench()
                await workbench.connect_servers(mcp_servers)
                mcp_tools = await workbench.discover_tools()
                if mcp_tools:
                    tool_names = [f"{t.server_id}/{t.name}" for t in mcp_tools]
                    logger.info("discovered %d MCP tools: %s", len(mcp_tools), tool_names)
                    await runtime.send_output(f"MCP: discovered {len(mcp_tools)} tools")

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
            model = task.config.get("model", "")
            response = await self._llm.completion(
                prompt=task.prompt,
                system=system_prompt,
                temperature=scenario_cfg.temperature,
                tags=[scenario_cfg.tag],
                **({"model": model} if model else {}),
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
        finally:
            if workbench is not None:
                await workbench.disconnect_all()
