"""Agent executor for processing tasks."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from codeforge.mcp_workbench import McpWorkbench
from codeforge.models import ModeConfig, TaskMessage, TaskResult, TaskStatus
from codeforge.pricing import resolve_cost
from codeforge.tracing import tracing_manager

if TYPE_CHECKING:
    from codeforge.llm import LiteLLMClient
    from codeforge.mcp_models import MCPServerDef
    from codeforge.memory.experience import ExperiencePool
    from codeforge.runtime import RuntimeClient

logger = logging.getLogger(__name__)

_tracer = tracing_manager.get_tracer()


class AgentExecutor:
    """Executor that receives a task, calls LLM, and returns a result."""

    def __init__(self, llm: LiteLLMClient, experience_pool: ExperiencePool | None = None) -> None:
        self._llm = llm
        self._experience_pool = experience_pool

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

    async def _check_experience_cache(self, task: TaskMessage, runtime: RuntimeClient) -> bool:
        """Check if a cached experience exists. Returns True if cache hit was used."""
        if not (self._experience_pool and task.project_id and task.prompt):
            return False
        try:
            cached = await self._experience_pool.lookup(task.prompt, task.project_id)
            if cached:
                logger.info(
                    "experience cache hit run_id=%s entry_id=%s similarity=%.3f",
                    runtime.run_id,
                    cached["id"],
                    cached["similarity"],
                )
                await runtime.send_output(f"Using cached result (similarity: {cached['similarity']:.2f})")
                await runtime.complete_run(status="completed", output=cached["result_output"])
                return True
        except Exception as exc:
            logger.warning("experience pool lookup failed, continuing: %s", exc)
        return False

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

        # Resolve scenario for model routing and temperature.
        from codeforge.llm import resolve_model_with_routing

        scenario_tag = mode.llm_scenario if mode and mode.llm_scenario else "default"
        routing = resolve_model_with_routing(
            prompt=task.prompt,
            scenario=scenario_tag,
        )
        logger.info(
            "llm_routing_decision run_id=%s mode=%s routed_model=%s temperature=%.2f",
            runtime.run_id,
            mode.id if mode else "",
            routing.model or "(tag-based)",
            routing.temperature,
        )

        workbench: McpWorkbench | None = None
        try:
            # Check experience pool for cached result
            if await self._check_experience_cache(task, runtime):
                return

            # Set up MCP workbench if servers are configured
            if mcp_servers:
                workbench = McpWorkbench()
                await workbench.connect_servers(mcp_servers)
                mcp_tools = await workbench.discover_tools()
                if mcp_tools:
                    tool_names = [f"{t.server_id}/{t.name}" for t in mcp_tools]
                    logger.info("discovered %d MCP tools: %s", len(mcp_tools), tool_names)
                    await runtime.send_output(f"MCP: discovered {len(mcp_tools)} tools")
                    # Include MCP tool descriptions in system prompt so the LLM
                    # is aware of available tools even in single-shot mode.
                    tool_descs = "\n".join(
                        f"- {t.server_id}/{t.name}: {t.description}" for t in mcp_tools if t.description
                    )
                    if tool_descs:
                        system_prompt = f"{system_prompt}\n\nAvailable MCP tools:\n{tool_descs}"

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

            # Execute the LLM call with routing decision.
            model = routing.model or task.config.get("model", "")
            response = await self._llm.completion(
                prompt=task.prompt,
                system=system_prompt,
                temperature=routing.temperature,
                tags=routing.tags or None,
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

            # Store successful result in experience pool
            if self._experience_pool and task.project_id and task.prompt:
                try:
                    await self._experience_pool.store(
                        task_desc=task.prompt,
                        project_id=task.project_id,
                        result_output=response.content,
                        result_cost=cost,
                        result_status="completed",
                        run_id=runtime.run_id,
                    )
                except Exception as exc:
                    logger.warning("experience pool store failed: %s", exc)

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
