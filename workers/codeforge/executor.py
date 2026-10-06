"""Agent executor for processing tasks."""

from __future__ import annotations

import logging
import os
from typing import TYPE_CHECKING

import structlog

from codeforge.agent_loop import AgentLoopExecutor
from codeforge.loop_config import build_loop_config, resolve_model_capability
from codeforge.mcp_workbench import McpWorkbench
from codeforge.models import ModeConfig, TaskMessage, TaskResult, TaskStatus
from codeforge.pricing import resolve_cost
from codeforge.tools import build_default_registry
from codeforge.tracing import tracing_manager

if TYPE_CHECKING:
    from codeforge.llm import LiteLLMClient
    from codeforge.mcp_models import MCPServerDef
    from codeforge.runtime import RuntimeClient

logger = logging.getLogger(__name__)

_tracer = tracing_manager.get_tracer()

_CLAUDE_CODE_PREFIX = "claudecode/"


def _litellm_models(primary: str, fallbacks: list[str]) -> tuple[str, list[str]]:
    """Drop Claude Code models from a run's model choice.

    The run loop calls models through LiteLLM with a policy decision per tool
    call; Claude Code runs its own tools outside that policy (KI-72).
    """
    usable = [m for m in fallbacks if not m.startswith(_CLAUDE_CODE_PREFIX)]
    if primary.startswith(_CLAUDE_CODE_PREFIX):
        primary = usable.pop(0) if usable else ""
    return primary, usable


class AgentExecutor:
    """Executes tasks: runs in the agent loop, fire-and-forget and A2A tasks as one completion."""

    def __init__(self, llm: LiteLLMClient, litellm_url: str = "", litellm_key: str = "") -> None:
        self._llm = llm
        # Model routing and discovery of a run (HybridRouter, fallback chain).
        self._litellm_url = litellm_url
        self._litellm_key = litellm_key

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
        tool_output_max_chars: int = 0,
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

        # The run path shares the conversation path's routing, loop setup and
        # tool guide; imported here because the consumer package imports this
        # module.
        from codeforge.consumer._conversation_prompt_builder import inject_tool_guide
        from codeforge.consumer._conversation_routing import resolve_model_and_fallbacks

        log = structlog.get_logger().bind(run_id=runtime.run_id, task_id=task.id)
        workbench: McpWorkbench | None = None
        try:
            # No skill tools: search_skills would find nothing and create_skill
            # would not save without the conversation path's skill wiring.
            registry = build_default_registry(skill_tools=False)
            if mode:
                registry.restrict_to_mode(mode.tools, mode.denied_tools)
            if mcp_servers:
                workbench = McpWorkbench()
                await workbench.connect_servers(mcp_servers)
                await workbench.discover_tools()
                registry.merge_mcp_tools(workbench)

            scenario = mode.llm_scenario if mode and mode.llm_scenario else "default"
            primary_model, routing, fallback_models = await resolve_model_and_fallbacks(
                self._litellm_url,
                self._litellm_key,
                prompt=task.prompt,
                scenario=scenario,
                explicit_model=task.config.get("model", ""),
                max_cost=runtime.termination.max_cost,
                log=log,
            )
            primary_model, fallback_models = _litellm_models(primary_model, fallback_models)

            base_prompt = (
                mode.prompt_prefix if mode and mode.prompt_prefix else f"You are working on task: {task.title}"
            )
            capability = await resolve_model_capability(self._llm, primary_model)
            system_prompt = inject_tool_guide(
                base_prompt, registry, capability.level, log, context_limit=capability.context_limit
            )

            config, complexity_hint = build_loop_config(
                primary_model=primary_model,
                capability_level=capability.level,
                routing=routing,
                tool_names=registry.tool_names,
                fallback_models=fallback_models,
                user_prompt=task.prompt,
                max_steps=runtime.termination.max_steps,
                max_cost=runtime.termination.max_cost,
                mode_tools=frozenset(mode.tools) if mode else frozenset(),
                tool_output_max_chars=tool_output_max_chars,
                implementation_turn=True,  # a run implements its task (KI-153)
                context_window=capability.context_window,
            )
            messages: list[dict[str, object]] = [
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": task.prompt},
            ]
            if complexity_hint:
                messages.append({"role": "system", "content": complexity_hint})

            # No experience pool: a cached answer would report the run done
            # without changing the workspace.
            loop = AgentLoopExecutor(llm=self._llm, tool_registry=registry, runtime=runtime, workspace_path=workspace)
            result = await loop.run(messages, config)

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
