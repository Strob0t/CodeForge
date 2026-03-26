"""Conversation run handler mixin."""

from __future__ import annotations

import asyncio
import json
import os
import uuid
from typing import TYPE_CHECKING, ClassVar

import structlog

from codeforge.consumer._conversation_prompt_builder import build_system_prompt
from codeforge.consumer._conversation_routing import (
    build_fallback_chain,
    get_available_models,
    get_hybrid_router,
)
from codeforge.consumer._conversation_skill_integration import (
    register_handoff_tool,
    register_propose_goal_tool,
    wire_skill_tools,
)
from codeforge.consumer._subjects import SUBJECT_CONVERSATION_RUN_COMPLETE
from codeforge.models import AgentLoopResult, ConversationRunCompleteMessage, ConversationRunStartMessage
from codeforge.runtime import RuntimeClient

if TYPE_CHECKING:
    import nats.aio.msg

    from codeforge.mcp_models import MCPTool
    from codeforge.mcp_workbench import McpWorkbench
    from codeforge.models import ContextEntry

logger = structlog.get_logger()

# --- Framework detection helpers for proactive docs prefetch ---

_JS_FRAMEWORK_MAP: dict[str, str] = {
    "solid-js": "solidjs",
    "react": "react",
    "vue": "vue",
    "next": "nextjs",
    "svelte": "svelte",
    "angular": "angular",
    "express": "express",
    "fastify": "fastify",
    "tailwindcss": "tailwindcss",
}

_PY_FRAMEWORK_MAP: dict[str, str] = {
    "fastapi": "fastapi",
    "flask": "flask",
    "django": "django",
    "starlette": "starlette",
    "pydantic": "pydantic",
    "sqlalchemy": "sqlalchemy",
    "pytest": "pytest",
}

_GO_MODULE_MAP: dict[str, str] = {
    "chi": "chi",
    "gin": "gin",
    "echo": "echo",
    "fiber": "fiber",
}


def _scan_file_for_keys(
    filepath: str,
    mapping: dict[str, str],
    existing: set[str],
    *,
    parse_json: bool = False,
) -> list[str]:
    """Scan a file for known dependency keys and return matched framework names."""
    if not os.path.isfile(filepath):
        return []
    try:
        with open(filepath) as f:
            raw = f.read()
    except OSError:
        return []

    hits: list[str] = []
    if parse_json:
        try:
            data = json.loads(raw)
        except (json.JSONDecodeError, ValueError):
            return []
        all_deps: dict[str, str] = {}
        all_deps.update(data.get("dependencies", {}))
        all_deps.update(data.get("devDependencies", {}))
        for pkg, name in mapping.items():
            if pkg in all_deps and name not in existing:
                hits.append(name)
    else:
        content = raw.lower()
        for pkg, name in mapping.items():
            if pkg in content and name not in existing:
                hits.append(name)
    return hits


def _detect_frameworks(workspace_path: str) -> list[str]:
    """Detect frameworks from workspace dependency files."""
    if not workspace_path or not os.path.isdir(workspace_path):
        return []

    frameworks: list[str] = []
    seen: set[str] = set()

    for filepath, mapping, use_json in [
        (os.path.join(workspace_path, "package.json"), _JS_FRAMEWORK_MAP, True),
        (os.path.join(workspace_path, "requirements.txt"), _PY_FRAMEWORK_MAP, False),
        (os.path.join(workspace_path, "pyproject.toml"), _PY_FRAMEWORK_MAP, False),
        (os.path.join(workspace_path, "go.mod"), _GO_MODULE_MAP, False),
    ]:
        hits = _scan_file_for_keys(filepath, mapping, seen, parse_json=use_json)
        frameworks.extend(hits)
        seen.update(hits)

    return frameworks[:5]


def _find_search_docs_tool(workbench: McpWorkbench) -> MCPTool | None:
    """Find the search_docs tool in the workbench's discovered tools."""
    for tool in workbench._tools:
        if tool.name == "search_docs":
            return tool
    return None


async def _prefetch_docs(
    workbench: McpWorkbench,
    workspace_path: str,
    user_message: str,
    log: structlog.stdlib.BoundLogger,
) -> list[ContextEntry]:
    """Pre-fetch documentation from docs-mcp-server for detected frameworks."""
    from codeforge.models import ContextEntry

    if not workbench or not user_message:
        return []

    search_tool = _find_search_docs_tool(workbench)
    if search_tool is None:
        return []

    frameworks = _detect_frameworks(workspace_path)
    if not frameworks:
        return []

    entries: list[ContextEntry] = []
    for framework in frameworks[:3]:
        try:
            result = await workbench.call_tool(
                search_tool.server_id,
                "search_docs",
                {"library": framework, "query": user_message, "limit": 3},
            )
            if result and result.output and len(result.output) > 50:
                entries.append(
                    ContextEntry(
                        kind="knowledge",
                        path=f"docs/{framework}",
                        content=result.output[:2000],
                        tokens=len(result.output) // 4,
                        priority=80,
                    )
                )
                log.info("prefetched docs", framework=framework, chars=len(result.output))
        except Exception as exc:
            log.debug("docs prefetch failed", framework=framework, error=str(exc))

    return entries


# Context token limits per capability level (M3).
_CONTEXT_LIMITS: dict[str, int] = {
    "full": 120_000,
    "api_with_tools": 32_000,
    "pure_completion": 16_000,
}


class ConversationHandlerMixin:
    """Handles conversation.run.start messages -- agentic loop with tool calling."""

    _active_runs: ClassVar[set[str]] = set()

    _SESSION_CONTEXT_NOTES: ClassVar[dict[str, str]] = {
        "resume": "This conversation is being resumed from a previous session. Continue where you left off.",
        "fork": "This conversation was forked from a previous point. The message history above represents the state at the fork point. Continue from here.",
        "rewind": "This conversation was rewound to an earlier state. Some later messages have been removed. Continue from this point.",
    }

    @staticmethod
    def _inject_session_context(
        messages: list[dict[str, str]],
        run_msg: ConversationRunStartMessage,
        log: structlog.stdlib.BoundLogger,
    ) -> None:
        """Append a system note when the session is a resume/fork/rewind."""
        if not run_msg.session_meta or not run_msg.session_meta.operation:
            return
        op = run_msg.session_meta.operation
        note = ConversationHandlerMixin._SESSION_CONTEXT_NOTES.get(op)
        if note:
            messages.append({"role": "system", "content": note})
            log.info("injected session context note", operation=op)

    async def _handle_conversation_run(self, msg: nats.aio.msg.Msg) -> None:  # noqa: C901
        """Process a conversation run: agentic loop with tool calling."""
        from codeforge.history import ConversationHistoryManager, HistoryConfig
        from codeforge.mcp_workbench import McpWorkbench
        from codeforge.tools import ToolRegistry, build_default_registry

        workbench: McpWorkbench | None = None
        run_id: str | None = None
        try:
            run_msg = ConversationRunStartMessage.model_validate_json(msg.data)
            run_id = run_msg.run_id
            log = logger.bind(run_id=run_id, conversation_id=run_msg.conversation_id, session_id=run_msg.session_id)

            if run_id in self._active_runs:
                log.warning("duplicate conversation run start, skipping")
                await msg.ack()
                return
            self._active_runs.add(run_id)

            log.info("received conversation run start")

            if self._js is None:
                log.error("JetStream not available")
                await msg.nak()
                return

            runtime = RuntimeClient(
                js=self._js,
                run_id=run_msg.run_id,
                task_id=run_msg.run_id,
                project_id=run_msg.project_id,
                termination=run_msg.termination,
            )
            await runtime.start_cancel_listener(extra_subjects=["conversation.run.cancel"])
            await runtime.start_heartbeat()

            registry: ToolRegistry = build_default_registry()

            if run_msg.mcp_servers:
                workbench = McpWorkbench()
                await workbench.connect_servers(run_msg.mcp_servers)
                await workbench.discover_tools()
                registry.merge_mcp_tools(workbench)
                log.info("mcp tools merged", count=len(workbench.get_tools_for_llm()))

            await self._maybe_prefetch_docs(workbench, run_msg, log)

            system_prompt, loaded_skills = await build_system_prompt(run_msg, registry, log, self._db_url, self._llm)

            wire_skill_tools(registry, loaded_skills, run_msg.project_id, log, self._db_url)
            register_handoff_tool(registry, run_msg.run_id, self._js)
            register_propose_goal_tool(registry, runtime)

            if run_msg.summarize_threshold > 0 and len(run_msg.messages) > run_msg.summarize_threshold:
                from codeforge.history import ConversationSummarizer

                summarizer = ConversationSummarizer(llm=self._llm, threshold=run_msg.summarize_threshold)
                run_msg.messages = await summarizer.summarize_if_needed(run_msg.messages)

            from codeforge.tools.capability import classify_model

            _cap_level = classify_model(run_msg.model)
            _context_cap = _CONTEXT_LIMITS.get(_cap_level, 120_000)
            history_cfg = HistoryConfig(max_context_tokens=_context_cap)
            log.info("context limit set", capability_level=_cap_level.value, max_tokens=_context_cap)

            history_mgr = ConversationHistoryManager(history_cfg)
            messages = history_mgr.build_messages(
                system_prompt=system_prompt,
                history=run_msg.messages,
                context_entries=run_msg.context,
            )

            self._inject_session_context(messages, run_msg, log)

            from codeforge.llm import resolve_model_with_routing

            user_prompt = ""
            for m in run_msg.messages:
                if m.role == "user" and m.content:
                    user_prompt = m.content
                    break

            scenario = run_msg.mode.llm_scenario if run_msg.mode else ""
            router = await get_hybrid_router(self._litellm_url, self._litellm_key)
            routing = await asyncio.to_thread(
                resolve_model_with_routing,
                prompt=user_prompt,
                scenario=scenario,
                router=router,
                max_cost=run_msg.termination.max_cost if run_msg.termination.max_cost > 0 else None,
            )
            primary_model = run_msg.model or routing.model
            if run_msg.model and routing.model and routing.model != run_msg.model:
                log.info("explicit model overrides routing", explicit=run_msg.model, routed=routing.model)
            elif not run_msg.model and routing.model:
                log.info("routing selected model", model=routing.model, scenario=scenario)

            fallback_models = await build_fallback_chain(
                router,
                user_prompt,
                primary_model,
                run_msg.termination.max_cost,
                routing,
                lambda: get_available_models(self._litellm_url, self._litellm_key),
            )

            timeout = int(os.getenv("CODEFORGE_CONVERSATION_TIMEOUT", "3600"))
            try:
                result = await asyncio.wait_for(
                    self._execute_conversation_run(
                        run_msg=run_msg,
                        messages=messages,
                        primary_model=primary_model,
                        routing=routing,
                        runtime=runtime,
                        registry=registry,
                        fallback_models=fallback_models,
                    ),
                    timeout=timeout,
                )
            except TimeoutError:
                logger.warning("conversation timed out", conversation_id=run_msg.conversation_id, timeout=timeout)
                result = AgentLoopResult(output="", tool_calls=[], cost=0.0, error="Wall-clock timeout exceeded")

            await self._publish_completion(run_msg, result)

            await msg.ack()
            log.info(
                "conversation run complete",
                steps=result.step_count,
                cost=result.total_cost,
                error=result.error or None,
            )

        except Exception as exc:
            logger.exception("failed to process conversation run", error=str(exc))
            await self._publish_error_result(msg)
            await msg.ack()
        finally:
            if workbench is not None:
                await workbench.disconnect_all()
            if run_id is not None:
                self._active_runs.discard(run_id)

    async def _publish_completion(
        self,
        run_msg: ConversationRunStartMessage,
        result: AgentLoopResult,
    ) -> None:
        """Publish a conversation run completion message to NATS."""
        complete_msg = ConversationRunCompleteMessage(
            run_id=run_msg.run_id,
            conversation_id=run_msg.conversation_id,
            session_id=run_msg.session_id,
            assistant_content=result.final_content,
            tool_messages=result.tool_messages,
            status="failed" if result.error else "completed",
            error=result.error,
            cost_usd=result.total_cost,
            tokens_in=result.total_tokens_in,
            tokens_out=result.total_tokens_out,
            step_count=result.step_count,
            model=result.model,
            tenant_id=run_msg.tenant_id,
        )
        stamped = self._stamp_trust(complete_msg.model_dump())
        await self._js.publish(
            SUBJECT_CONVERSATION_RUN_COMPLETE,
            json.dumps(stamped).encode(),
            headers={"Nats-Msg-Id": f"conv-complete-{uuid.uuid4()}"},
        )

    async def _publish_error_result(self, msg: nats.aio.msg.Msg) -> None:
        """Best-effort publish of an error completion when the main handler fails."""
        try:
            run_msg = ConversationRunStartMessage.model_validate_json(msg.data)
            if self._js is not None:
                error_complete = ConversationRunCompleteMessage(
                    run_id=run_msg.run_id,
                    conversation_id=run_msg.conversation_id,
                    session_id=run_msg.session_id,
                    status="failed",
                    error="internal worker error",
                    tenant_id=run_msg.tenant_id,
                )
                await self._js.publish(
                    SUBJECT_CONVERSATION_RUN_COMPLETE,
                    error_complete.model_dump_json().encode(),
                    headers={"Nats-Msg-Id": f"conv-error-{uuid.uuid4()}"},
                )
        except Exception as exc:
            logger.exception("failed to publish conversation error result", error=str(exc))

    async def _execute_conversation_run(
        self,
        run_msg: ConversationRunStartMessage,
        messages: list[dict],
        primary_model: str,
        routing: object,
        runtime: RuntimeClient,
        registry: object,
        fallback_models: list[str],
    ) -> AgentLoopResult:
        """Dispatch to simple chat, Claude Code, or LiteLLM agentic loop."""
        if not run_msg.agentic:
            return await self._run_simple_chat(
                run_msg,
                messages,
                primary_model,
                routing,
                runtime,
                fallback_models=fallback_models,
            )

        if primary_model.startswith("claudecode/"):
            from codeforge.claude_code_executor import ClaudeCodeExecutor, get_default_max_turns

            cc_executor = ClaudeCodeExecutor(workspace_path=run_msg.workspace_path, runtime=runtime)
            result = await cc_executor.run(
                messages=messages,
                model=primary_model,
                max_turns=run_msg.termination.max_steps or get_default_max_turns(),
                system_prompt=run_msg.system_prompt,
            )
            if result.error and fallback_models:
                next_model = fallback_models[0]
                remaining = fallback_models[1:]
                await runtime.send_output(f"\n[Claude Code unavailable. Switching to {next_model}]\n")
                return await self._execute_litellm_loop(
                    run_msg,
                    messages,
                    next_model,
                    routing,
                    runtime,
                    registry,
                    remaining,
                )
            return result

        return await self._execute_litellm_loop(
            run_msg,
            messages,
            primary_model,
            routing,
            runtime,
            registry,
            fallback_models,
        )

    async def _execute_litellm_loop(
        self,
        run_msg: ConversationRunStartMessage,
        messages: list[dict],
        primary_model: str,
        routing: object,
        runtime: RuntimeClient,
        registry: object,
        fallback_models: list[str],
    ) -> AgentLoopResult:
        """Run the LiteLLM-based agentic loop with optional multi-rollout."""
        from codeforge.agent_loop import AgentLoopExecutor, ConversationRolloutExecutor, LoopConfig
        from codeforge.tools.capability import classify_model
        from codeforge.tools.tool_router import ToolRouter

        executor = AgentLoopExecutor(
            llm=self._llm,
            tool_registry=registry,
            runtime=runtime,
            workspace_path=run_msg.workspace_path,
            experience_pool=getattr(self, "_experience_pool", None),
        )
        mode_tools = frozenset(run_msg.mode.tools) if run_msg.mode and run_msg.mode.tools else frozenset()
        capability_level = classify_model(primary_model)

        user_msg = next((m.content for m in run_msg.messages if m.role == "user" and m.content), "")
        tool_router = ToolRouter(all_tool_names=registry.tool_names)
        selected_tools = tool_router.select(user_msg) if user_msg else None
        if selected_tools is not None:
            logger.info("tool router selected", count=len(selected_tools), tools=selected_tools)

        _is_local = primary_model.startswith(("lm_studio/", "ollama/"))
        _temperature = 0.7 if _is_local else routing.temperature
        _top_p: float | None = 0.8 if _is_local else None
        _extra_body: dict[str, object] | None = {"top_k": 20, "repetition_penalty": 1.05} if _is_local else None

        loop_cfg = LoopConfig(
            max_iterations=run_msg.termination.max_steps or 50,
            max_cost=run_msg.termination.max_cost or 0.0,
            model=primary_model,
            temperature=_temperature,
            tags=routing.tags,
            fallback_models=fallback_models,
            routing_layer=routing.routing_layer,
            complexity_tier=routing.complexity_tier,
            task_type=routing.task_type,
            provider_api_key=run_msg.provider_api_key,
            plan_act_enabled=run_msg.plan_act_enabled,
            extra_plan_tools=mode_tools,
            routing_metadata=getattr(routing, "routing_metadata", None),
            capability_level=str(capability_level),
            mode_tools=mode_tools,
            top_p=_top_p,
            extra_body=_extra_body,
            selected_tools=selected_tools,
        )

        rollout_count = max(1, min(run_msg.rollout_count, 8))
        if rollout_count > 1:
            rollout_exec = ConversationRolloutExecutor(
                agent_loop_executor=executor,
                rollout_count=rollout_count,
                workspace_path=run_msg.workspace_path,
                runtime=runtime,
            )
            return await rollout_exec.execute(messages, config=loop_cfg)

        return await executor.run(messages, config=loop_cfg)

    async def _run_simple_chat(
        self,
        run_msg: ConversationRunStartMessage,
        messages: list[dict],
        model: str,
        routing: object,
        runtime: RuntimeClient,
        fallback_models: list[str] | None = None,
    ) -> AgentLoopResult:
        """Single-turn LLM call with per-chunk streaming via NATS."""
        import asyncio

        from codeforge.llm import LLMError, RoutingResult, classify_error_type, is_fallback_eligible
        from codeforge.models import AgentLoopResult
        from codeforge.routing.blocklist import get_blocklist
        from codeforge.routing.rate_tracker import get_tracker

        rt = routing if isinstance(routing, RoutingResult) else RoutingResult()

        models_to_try = [model] + (fallback_models or [])
        tracker = get_tracker()
        failed: set[str] = set()
        last_error: str = ""

        for current_model in models_to_try:
            if current_model in failed:
                continue
            provider = current_model.split("/", 1)[0] if "/" in current_model else ""
            if provider and tracker.is_exhausted(provider):
                continue

            loop = asyncio.get_running_loop()
            pending: list[asyncio.Task[None]] = []

            def _on_chunk(chunk_text: str, _pending: list = pending, _loop: asyncio.AbstractEventLoop = loop) -> None:
                task = _loop.create_task(runtime.send_output(chunk_text))
                _pending.append(task)

            try:
                resp = await self._llm.chat_completion_stream(
                    messages=messages,
                    model=current_model,
                    temperature=rt.temperature,
                    tags=rt.tags,
                    on_chunk=_on_chunk,
                    provider_api_key=run_msg.provider_api_key,
                )
            except LLMError as exc:
                failed.add(current_model)
                last_error = str(exc)
                error_type = classify_error_type(exc)
                if error_type:
                    tracker.record_error(provider or current_model, error_type=error_type)
                if exc.status_code in (401, 403):
                    get_blocklist().block_auth(current_model, reason=f"HTTP {exc.status_code}")
                if not is_fallback_eligible(exc) or current_model == models_to_try[-1]:
                    break
                notice = f"\n[Model {current_model} unavailable ({exc.status_code}). Switching to next model]\n"
                await runtime.send_output(notice)
                logger.warning("simple_chat fallback: %s failed (%d)", current_model, exc.status_code)
                continue

            if pending:
                await asyncio.gather(*pending, return_exceptions=True)

            return AgentLoopResult(
                final_content=resp.content,
                total_cost=resp.cost_usd,
                total_tokens_in=resp.tokens_in,
                total_tokens_out=resp.tokens_out,
                step_count=1,
                model=resp.model,
            )

        return AgentLoopResult(
            final_content="",
            step_count=0,
            model=model,
            error=f"All models failed. Last error: {last_error}",
        )

    @staticmethod
    async def _maybe_prefetch_docs(
        workbench: McpWorkbench | None,
        run_msg: ConversationRunStartMessage,
        log: structlog.stdlib.BoundLogger,
    ) -> None:
        """Prefetch docs from MCP workbench and append to run_msg.context."""
        if workbench is None:
            return
        user_message = next(
            (m.content for m in run_msg.messages if m.role == "user" and m.content),
            "",
        )
        prefetched = await _prefetch_docs(
            workbench=workbench,
            workspace_path=run_msg.workspace_path,
            user_message=user_message,
            log=log,
        )
        if prefetched:
            run_msg.context.extend(prefetched)
            log.info("docs prefetch injected", count=len(prefetched))
