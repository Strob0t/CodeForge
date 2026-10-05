"""Conversation run handler mixin."""

from __future__ import annotations

import asyncio
import contextlib
import json
import os
import uuid
from typing import TYPE_CHECKING, ClassVar, Protocol

import structlog

from codeforge.consumer._cancel_registry import conversation_key, run_key
from codeforge.consumer._conversation_experience import answer_from_experience, remember_answer
from codeforge.consumer._conversation_prompt_builder import build_system_prompt
from codeforge.consumer._conversation_routing import resolve_model_and_fallbacks
from codeforge.consumer._conversation_skill_integration import (
    register_handoff_tool,
    register_propose_goal_tool,
    register_propose_roadmap_tool,
    wire_skill_tools,
)
from codeforge.consumer._delivery import stream_sequence
from codeforge.consumer._subjects import SUBJECT_CONVERSATION_RUN_COMPLETE
from codeforge.loop_config import build_loop_config, resolve_model_capability
from codeforge.model_resolver import NoModelAvailableError
from codeforge.models import AgentLoopResult, ConversationRunCompleteMessage, ConversationRunStartMessage
from codeforge.nats_publish import publish_with_retry
from codeforge.provider_keys import fallbacks_for_key, model_provider
from codeforge.runtime import RuntimeClient, heartbeat_interval
from codeforge.tool_identity import ToolIsolationError, tool_tenant
from codeforge.tools.capability import CapabilityLevel
from codeforge.tools.text_protocol import HISTORY_RESERVE_TOKENS
from codeforge.workspace_fs import WorkspaceRoot

if TYPE_CHECKING:
    import nats.aio.msg

    from codeforge.consumer._in_flight import AcceptedWork
    from codeforge.mcp_models import MCPTool
    from codeforge.mcp_workbench import McpWorkbench
    from codeforge.models import ContextEntry

logger = structlog.get_logger()


class RoutingResult(Protocol):
    """Protocol for the routing result object passed to conversation handlers."""

    model: str
    temperature: float
    tags: list[str]
    routing_layer: str
    complexity_tier: str
    task_type: str


class ToolRegistryLike(Protocol):
    """Protocol for the tool registry passed to conversation handlers."""

    tool_names: list[str]

    def merge_mcp_tools(self, workbench: object) -> None: ...


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


# Dependency manifests are small; a larger one is not read.
_MAX_MANIFEST_BYTES = 1024 * 1024


def _scan_file_for_keys(
    root: WorkspaceRoot,
    name: str,
    mapping: dict[str, str],
    existing: set[str],
    *,
    parse_json: bool = False,
) -> list[str]:
    """Scan a workspace file for known dependency keys and return matched framework names.

    The file is read through the workspace helper (KI-95): a symlink that
    leaves the workspace, a FIFO or an oversized file is not read.
    """
    try:
        raw = root.read_text(name, max_bytes=_MAX_MANIFEST_BYTES, errors="replace")
    except OSError:
        return []

    hits: list[str] = []
    if parse_json:
        try:
            data = json.loads(raw)
        except (json.JSONDecodeError, ValueError):
            return []
        if not isinstance(data, dict):
            return []
        all_deps: dict[str, object] = {}
        for key in ("dependencies", "devDependencies"):
            deps = data.get(key)
            if isinstance(deps, dict):
                all_deps.update(deps)
        for pkg, framework in mapping.items():
            if pkg in all_deps and framework not in existing:
                hits.append(framework)
    else:
        content = raw.lower()
        for pkg, framework in mapping.items():
            if pkg in content and framework not in existing:
                hits.append(framework)
    return hits


def _detect_frameworks(workspace_path: str) -> list[str]:
    """Detect frameworks from workspace dependency files."""
    if not workspace_path:
        return []
    try:
        root = WorkspaceRoot(workspace_path)
    except OSError:
        return []

    frameworks: list[str] = []
    seen: set[str] = set()

    with root:
        for name, mapping, use_json in [
            ("package.json", _JS_FRAMEWORK_MAP, True),
            ("requirements.txt", _PY_FRAMEWORK_MAP, False),
            ("pyproject.toml", _PY_FRAMEWORK_MAP, False),
            ("go.mod", _GO_MODULE_MAP, False),
        ]:
            hits = _scan_file_for_keys(root, name, mapping, seen, parse_json=use_json)
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

    async def _build_conversation_messages(
        self,
        run_msg: ConversationRunStartMessage,
        runtime: RuntimeClient,
        registry: ToolRegistryLike,
        log: structlog.stdlib.BoundLogger,
        *,
        model: str,
    ) -> list[dict[str, str]]:
        """Build the message list from system prompt, history, context, and session info.

        *model* is the model the run calls (resolved by routing or the default
        model when the run start names none).
        """
        from codeforge.history import ConversationHistoryManager, HistoryConfig

        # The capability selects the tool guide and the context limit sizes
        # the history (and picks a compact guide for small-context models).
        capability = await resolve_model_capability(self._llm, model)

        system_prompt, loaded_skills = await build_system_prompt(
            run_msg,
            registry,
            log,
            self._db_url,
            self._llm,
            capability=capability,
        )

        wire_skill_tools(registry, loaded_skills, run_msg.project_id, log, self._db_url, tenant_id=run_msg.tenant_id)
        register_handoff_tool(
            registry,
            run_msg.run_id,
            self._js,
            tenant_id=run_msg.tenant_id,
            project_id=run_msg.project_id,
            approval_timeout_seconds=run_msg.approval_timeout_seconds,
        )
        # The planning tools only in planning turns: in an implementation
        # turn (the auto-agent's feature turns) a weak model called them
        # instead of writing code (KI-153).
        if not run_msg.implementation_turn:
            register_propose_goal_tool(registry, runtime)
            register_propose_roadmap_tool(registry, runtime)
        # spawn_subagent is not registered until Go starts sub-agents and
        # returns their results (KI-25, see register_spawn_subagent_tool).

        if run_msg.summarize_threshold > 0 and len(run_msg.messages) > run_msg.summarize_threshold:
            from codeforge.history import ConversationSummarizer

            summarizer = ConversationSummarizer(llm=self._llm, threshold=run_msg.summarize_threshold)
            run_msg.messages = await summarizer.summarize_if_needed(run_msg.messages)

        # The text tool protocol adds its prompt section to every request of a
        # pure-completion model (S9-C); the history leaves room for it.
        reserve = HISTORY_RESERVE_TOKENS if capability.level == CapabilityLevel.PURE_COMPLETION else 0
        history_cfg = HistoryConfig(max_context_tokens=capability.context_limit - reserve)
        if run_msg.tool_output_max_chars > 0:
            history_cfg.tool_output_max_chars = run_msg.tool_output_max_chars

        history_mgr = ConversationHistoryManager(history_cfg)
        messages = history_mgr.build_messages(
            system_prompt=system_prompt,
            history=run_msg.messages,
            context_entries=run_msg.context,
        )

        self._inject_session_context(messages, run_msg, log)
        return messages

    async def _resolve_routing_and_fallbacks(
        self,
        run_msg: ConversationRunStartMessage,
        user_prompt: str,
        log: structlog.stdlib.BoundLogger,
    ) -> tuple[str, RoutingResult, list[str]]:
        """Resolve the primary model via routing and build fallback chain.

        Returns (primary_model, routing_result, fallback_models).
        """
        return await resolve_model_and_fallbacks(
            self._litellm_url,
            self._litellm_key,
            prompt=user_prompt,
            scenario=run_msg.mode.llm_scenario if run_msg.mode else "",
            explicit_model=run_msg.model,
            max_cost=run_msg.termination.max_cost,
            log=log,
        )

    async def _handle_conversation_run(self, msg: nats.aio.msg.Msg) -> None:
        """Process a conversation run: agentic loop with tool calling.

        Acked on accept (at-most-once): the run changes the workspace and must
        not be executed a second time by another worker. Failures are reported
        to the Go Core as a failed completion instead of being retried.
        """
        run_msg = await self._parse_request(msg, ConversationRunStartMessage)
        if run_msg is None:
            return
        run_id = run_msg.run_id
        log = logger.bind(run_id=run_id, conversation_id=run_msg.conversation_id, session_id=run_msg.session_id)

        if run_id in self._active_runs:
            log.warning("duplicate conversation run start, skipping")
            await msg.ack()
            return

        # A conversation run stopped while its start waited in NATS is not
        # executed; a later turn (published after the stop) runs (KI-65
        # follow-up). Its cancelled completion ends the Go turn (S2-G fix, 2);
        # if it cannot be published, the start is retried (dead-lettered on
        # its last delivery, which Go ends).
        start = stream_sequence(msg)
        if start is not None and self._cancels.cancelled_any(
            [conversation_key(run_msg.conversation_id), run_key(run_id)], start
        ):
            log.info("conversation run stopped while it waited for a worker, skipping")
            try:
                await self._publish_skipped_completion(run_msg)
            except Exception as exc:
                log.exception("could not report the skipped conversation run", error=str(exc))
                await self._retry_or_dead_letter(msg)
                return
            await msg.ack()
            return

        if self._js is None:
            log.error("JetStream not available")
            await msg.nak()
            return

        self._active_runs.add(run_id)
        if not await self._accept(msg):
            self._active_runs.discard(run_id)
            return

        async def report_failure(reason: str) -> None:
            await self._publish_failed_completion(run_msg, reason)

        try:
            with self._in_flight.track(f"conversation run {run_id}", report_failure) as work:
                try:
                    await self._run_conversation(run_msg, log, work, start)
                except Exception as exc:
                    # Intentional catch-all: outermost handler safety net. A run
                    # whose completion was already published is not failed again.
                    logger.exception("failed to process conversation run", error=str(exc))
                    if not work.completed:
                        # A refused tool identity names its reason (tenant, tool UID,
                        # remedy), a missing model what to configure (KI-125).
                        named = isinstance(exc, (ToolIsolationError, NoModelAvailableError))
                        reason = str(exc) if named else "internal worker error"
                        await self._publish_failed_completion(run_msg, reason)
        finally:
            self._active_runs.discard(run_id)

    async def _run_conversation(
        self,
        run_msg: ConversationRunStartMessage,
        log: structlog.stdlib.BoundLogger,
        work: AcceptedWork,
        start: int | None = None,
    ) -> None:
        """Execute an accepted conversation run and publish its completion (then *work* is completed).

        *start* is the stream sequence of the run's start message: the cancel
        listener sees every cancel published after it (S2-G fix, f2).
        """
        from codeforge.mcp_workbench import McpWorkbench
        from codeforge.tools import ToolRegistry, build_default_registry

        log.info("received conversation run start")
        runtime = RuntimeClient(
            js=self._js,
            run_id=run_msg.run_id,
            task_id=run_msg.run_id,
            project_id=run_msg.project_id,
            termination=run_msg.termination,
            tenant_id=run_msg.tenant_id,
            mode_id=run_msg.mode.id if run_msg.mode else "",
            turn_id=run_msg.turn_id,
            approval_timeout_seconds=run_msg.approval_timeout_seconds,
            notifications=self._notifications,
        )
        workbench: McpWorkbench | None = None
        identity = contextlib.AsyncExitStack()
        try:
            await runtime.start_cancel_listener(extra_subjects=["conversation.run.cancel"], after=start)
            await runtime.start_heartbeat(heartbeat_interval(run_msg.heartbeat_seconds))
            # The turn's tool processes and MCP stdio servers run as its tenant's tool UID (KI-96).
            await identity.enter_async_context(
                tool_tenant(run_msg.tenant_id, run_msg.tool_uid, run_msg.workspace_path or None)
            )

            registry: ToolRegistry = build_default_registry()
            if run_msg.mode:
                registry.restrict_to_mode(run_msg.mode.tools, run_msg.mode.denied_tools)

            if run_msg.mcp_servers:
                workbench = McpWorkbench()
                await workbench.connect_servers(run_msg.mcp_servers)
                await workbench.discover_tools()
                registry.merge_mcp_tools(workbench)
                log.info("mcp tools merged", count=len(workbench.get_tools_for_llm()))

            await self._maybe_prefetch_docs(workbench, run_msg, log)

            user_prompt = ""
            for m in run_msg.messages:
                if m.role == "user" and m.content:
                    user_prompt = m.content
                    break

            # The model first: the prompt and history are sized for its
            # capability, also when the Go Core sent none (KI-125).
            primary_model, routing, fallback_models = await self._resolve_routing_and_fallbacks(
                run_msg,
                user_prompt,
                log,
            )

            messages = await self._build_conversation_messages(run_msg, runtime, registry, log, model=primary_model)

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
            work.completed = True
            log.info(
                "conversation run complete",
                steps=result.step_count,
                cost=result.total_cost,
                error=result.error or None,
            )
        finally:
            await runtime.close()
            if workbench is not None:
                await workbench.disconnect_all()
            # Leaving the identity shares what MCP servers and tool processes created (KI-71 review).
            await identity.aclose()

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
            turn_id=run_msg.turn_id,
        )
        stamped = self._stamp_trust(complete_msg.model_dump())
        # One message ID for all attempts: the Go Core deduplicates completions
        # by Nats-Msg-Id only (a retry must not store the assistant message twice).
        await publish_with_retry(
            self._js,
            SUBJECT_CONVERSATION_RUN_COMPLETE,
            json.dumps(stamped).encode(),
            headers={"Nats-Msg-Id": f"conv-complete-{uuid.uuid4()}"},
        )

    async def _publish_skipped_completion(self, run_msg: ConversationRunStartMessage) -> None:
        """Publish the cancelled completion of a conversation run whose start is skipped; raises on failure."""
        if self._js is None:
            err_msg = "JetStream not available for the skipped conversation run's completion"
            raise RuntimeError(err_msg)
        skipped = ConversationRunCompleteMessage(
            run_id=run_msg.run_id,
            conversation_id=run_msg.conversation_id,
            session_id=run_msg.session_id,
            status="cancelled",
            error="conversation run cancelled before a worker started it",
            tenant_id=run_msg.tenant_id,
            turn_id=run_msg.turn_id,
        )
        # One message ID per turn: a retried start publishes it again, and the
        # Go Core deduplicates completions by Nats-Msg-Id.
        await publish_with_retry(
            self._js,
            SUBJECT_CONVERSATION_RUN_COMPLETE,
            skipped.model_dump_json().encode(),
            headers={"Nats-Msg-Id": f"conv-skipped-{run_msg.run_id}-{run_msg.turn_id}"},
        )

    async def _publish_failed_completion(self, run_msg: ConversationRunStartMessage, error: str) -> None:
        """Last-resort failed completion of an accepted run whose own completion was not published."""
        if self._js is None:
            logger.error("JetStream not available, conversation run not failed", run_id=run_msg.run_id)
            return
        error_complete = ConversationRunCompleteMessage(
            run_id=run_msg.run_id,
            conversation_id=run_msg.conversation_id,
            session_id=run_msg.session_id,
            status="failed",
            error=error,
            tenant_id=run_msg.tenant_id,
            turn_id=run_msg.turn_id,
        )
        try:
            await publish_with_retry(
                self._js,
                SUBJECT_CONVERSATION_RUN_COMPLETE,
                error_complete.model_dump_json().encode(),
                headers={"Nats-Msg-Id": f"conv-error-{uuid.uuid4()}"},
            )
        except Exception as exc:  # Intentional catch-all: last-resort error notification
            logger.exception("failed to publish conversation error result", run_id=run_msg.run_id, error=str(exc))

    async def _execute_conversation_run(
        self,
        run_msg: ConversationRunStartMessage,
        messages: list[dict],
        primary_model: str,
        routing: RoutingResult,
        runtime: RuntimeClient,
        registry: ToolRegistryLike,
        fallback_models: list[str],
    ) -> AgentLoopResult:
        """Dispatch to simple chat, Claude Code, or LiteLLM agentic loop."""
        if run_msg.provider_api_key:
            # The user's own key belongs to the provider of the run's model
            # (the Go Core resolved it for that model) and is sent with every
            # call: fall back only to models of that provider.
            same_provider = fallbacks_for_key(run_msg.model, fallback_models)
            if skipped := [m for m in fallback_models if m not in same_provider]:
                logger.warning(
                    "fallback models of another provider skipped: the run uses the user's own key",
                    run_id=run_msg.run_id,
                    provider=model_provider(run_msg.model),
                    skipped=skipped,
                )
            fallback_models = same_provider
        if not run_msg.agentic:
            pool = getattr(self, "_experience_pool", None)
            cached = await answer_from_experience(pool, run_msg, runtime, primary_model)
            if cached is not None:
                return cached
            result = await self._run_simple_chat(
                run_msg,
                messages,
                primary_model,
                routing,
                runtime,
                fallback_models=fallback_models,
            )
            await remember_answer(pool, run_msg, result)
            return result

        if primary_model.startswith("claudecode/"):
            from codeforge.claude_code_executor import ClaudeCodeExecutor, get_default_max_turns

            cc_executor = ClaudeCodeExecutor(workspace_path=run_msg.workspace_path, runtime=runtime)
            result = await cc_executor.run(
                messages=messages,
                model=primary_model,
                max_turns=run_msg.termination.max_steps or get_default_max_turns(),
                system_prompt=run_msg.system_prompt,
            )
            # Re-run the turn on another model only when Claude Code applied
            # nothing and was not stopped; otherwise its error says why.
            if result.error and fallback_models and result.metadata.get("fallback_safe") is True:
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
        routing: RoutingResult,
        runtime: RuntimeClient,
        registry: ToolRegistryLike,
        fallback_models: list[str],
    ) -> AgentLoopResult:
        """Run the LiteLLM-based agentic loop with optional multi-rollout."""
        from codeforge.agent_loop import AgentLoopExecutor, ConversationRolloutExecutor

        executor = AgentLoopExecutor(
            llm=self._llm,
            tool_registry=registry,
            runtime=runtime,
            workspace_path=run_msg.workspace_path,
        )
        capability = await resolve_model_capability(self._llm, primary_model)
        loop_cfg, complexity_hint = build_loop_config(
            primary_model=primary_model,
            capability_level=capability.level,
            routing=routing,
            tool_names=registry.tool_names,
            fallback_models=fallback_models,
            user_prompt=next((m.content for m in run_msg.messages if m.role == "user" and m.content), ""),
            max_steps=run_msg.termination.max_steps,
            max_cost=run_msg.termination.max_cost,
            mode_tools=frozenset(run_msg.mode.tools) if run_msg.mode and run_msg.mode.tools else frozenset(),
            provider_api_key=run_msg.provider_api_key,
            plan_act_enabled=run_msg.plan_act_enabled,
            tool_output_max_chars=run_msg.tool_output_max_chars,
            implementation_turn=run_msg.implementation_turn,
        )
        if complexity_hint:
            messages.append({"role": "system", "content": complexity_hint})

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
        routing: RoutingResult,
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
