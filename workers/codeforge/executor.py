"""Agent executor for processing tasks."""

from __future__ import annotations

import logging
import os
from typing import TYPE_CHECKING

from codeforge.agent_loop import DEFAULT_MAX_ITERATIONS, AgentLoopExecutor, LoopConfig
from codeforge.llm import resolve_model_with_routing
from codeforge.mcp_workbench import McpWorkbench
from codeforge.models import ModeConfig, TaskMessage, TaskResult, TaskStatus
from codeforge.pricing import resolve_cost
from codeforge.tools import build_default_registry
from codeforge.tools.capability import CapabilityLevel, classify_model
from codeforge.tracing import tracing_manager

if TYPE_CHECKING:
    from codeforge.llm import LiteLLMClient
    from codeforge.mcp_models import MCPServerDef
    from codeforge.runtime import RuntimeClient

logger = logging.getLogger(__name__)

_tracer = tracing_manager.get_tracer()


class AgentExecutor:
    """Executes tasks: runs in the agent loop, fire-and-forget and A2A tasks as one completion."""

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

    @_tracer.trace_agent("executor-a2a")
    async def execute_a2a_task(
        self,
        *,
        task_id: str,
        skill_id: str,
        prompt: str,
    ) -> TaskResult:
        """Execute an inbound A2A task.

        Unlike the generic ``execute()``, this method builds a system prompt
        that incorporates the A2A skill context so the LLM understands the
        request originates from an external agent.
        """
        logger.info("executing A2A task %s (skill=%s)", task_id, skill_id)

        skill_context = f" (skill: {skill_id})" if skill_id else ""
        system = f"You are an AI agent executing an A2A task{skill_context}. Respond concisely."

        try:
            response = await self._llm.completion(
                prompt=prompt,
                system=system,
            )

            cost = resolve_cost(
                response.cost_usd,
                response.model,
                response.tokens_in,
                response.tokens_out,
            )
            return TaskResult(
                task_id=task_id,
                status=TaskStatus.COMPLETED,
                output=response.content,
                tokens_in=response.tokens_in,
                tokens_out=response.tokens_out,
                cost_usd=cost,
            )
        except Exception as exc:
            logger.exception("A2A task %s failed", task_id)
            return TaskResult(
                task_id=task_id,
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
        """Execute a run in the agent loop and publish its completion.

        The LLM calls tools until it is done; the Go control plane approves
        every LLM and tool call before it runs (runs.toolcall.request), and
        the tools work in the project workspace named by the run start.
        """
        logger.info("executing task %s with runtime protocol: %s", task.id, task.title)

        workspace = task.workspace_path.strip()
        if not workspace or not os.path.isdir(workspace):
            # Without it the tools would work in the worker's own directory.
            error = f"run has no usable workspace ({task.workspace_path!r} is not a directory on this worker)"
            logger.error("run %s rejected: %s", runtime.run_id, error)
            await runtime.complete_run(status="failed", error=error)
            return

        await runtime.send_output(f"Starting task: {task.title}")

        system_prompt = mode.prompt_prefix if mode and mode.prompt_prefix else f"You are working on task: {task.title}"
        scenario_tag = mode.llm_scenario if mode and mode.llm_scenario else "default"
        routing = resolve_model_with_routing(prompt=task.prompt, scenario=scenario_tag)
        model = routing.model or task.config.get("model", "")
        logger.info(
            "llm_routing_decision run_id=%s mode=%s routed_model=%s temperature=%.2f",
            runtime.run_id,
            mode.id if mode else "",
            model or "(tag-based)",
            routing.temperature,
        )

        workbench: McpWorkbench | None = None
        try:
            registry = build_default_registry()
            if mcp_servers:
                workbench = McpWorkbench()
                await workbench.connect_servers(mcp_servers)
                await workbench.discover_tools()
                registry.merge_mcp_tools(workbench)

            # No experience pool: a cached answer would report the run done
            # without changing the workspace.
            loop = AgentLoopExecutor(llm=self._llm, tool_registry=registry, runtime=runtime, workspace_path=workspace)
            config = LoopConfig(
                max_iterations=runtime.termination.max_steps or DEFAULT_MAX_ITERATIONS,
                max_cost=runtime.termination.max_cost,
                model=model,
                temperature=routing.temperature,
                tags=routing.tags,
                capability_level=str(classify_model(model)) if model else str(CapabilityLevel.FULL),
                mode_tools=frozenset(mode.tools) if mode else frozenset(),
            )
            result = await loop.run(
                [{"role": "system", "content": system_prompt}, {"role": "user", "content": task.prompt}],
                config,
            )

            if runtime.is_cancelled:
                await runtime.complete_run(status="cancelled", error="cancelled by user")
            elif result.error:
                await runtime.complete_run(status="failed", output=result.final_content, error=result.error)
            else:
                await runtime.complete_run(status="completed", output=result.final_content)
        except Exception as exc:
            logger.exception("task %s failed in runtime mode", task.id)
            await runtime.complete_run(
                status="failed",
                error=str(exc),
            )
        finally:
            if workbench is not None:
                await workbench.disconnect_all()
