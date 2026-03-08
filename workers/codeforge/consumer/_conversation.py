"""Conversation run handler mixin."""

from __future__ import annotations

import json
import os
from typing import TYPE_CHECKING, ClassVar

import structlog

from codeforge.consumer._subjects import SUBJECT_CONVERSATION_RUN_COMPLETE
from codeforge.models import ConversationRunCompleteMessage, ConversationRunStartMessage
from codeforge.runtime import RuntimeClient

if TYPE_CHECKING:
    import nats.aio.msg

    from codeforge.models import AgentLoopResult
    from codeforge.routing.router import HybridRouter

logger = structlog.get_logger()


class ConversationHandlerMixin:
    """Handles conversation.run.start messages — agentic loop with tool calling."""

    # Track in-progress run IDs to skip NATS redeliveries of the same message.
    _active_runs: ClassVar[set[str]] = set()

    async def _handle_conversation_run(self, msg: nats.aio.msg.Msg) -> None:
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

            # Skip duplicate deliveries — the LLM loop is expensive.
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
            await runtime.start_cancel_listener(
                extra_subjects=["conversation.run.cancel"],
            )
            await runtime.start_heartbeat()

            registry: ToolRegistry = build_default_registry()

            if run_msg.mcp_servers:
                workbench = McpWorkbench()
                await workbench.connect_servers(run_msg.mcp_servers)
                await workbench.discover_tools()
                registry.merge_mcp_tools(workbench)
                log.info("mcp tools merged", count=len(workbench.get_tools_for_llm()))

            system_prompt = await self._build_system_prompt(run_msg, registry, log)

            self._register_handoff_tool(registry, run_msg.run_id)
            self._register_goals_tool(registry, run_msg.project_id)

            history_mgr = ConversationHistoryManager(HistoryConfig())
            messages = history_mgr.build_messages(
                system_prompt=system_prompt,
                history=run_msg.messages,
                context_entries=run_msg.context,
            )

            from codeforge.llm import resolve_model_with_routing

            # Extract user prompt for routing analysis.
            user_prompt = ""
            for m in run_msg.messages:
                if m.role == "user" and m.content:
                    user_prompt = m.content
                    break

            scenario = run_msg.mode.llm_scenario if run_msg.mode else ""
            router = await self._get_hybrid_router()
            routing = resolve_model_with_routing(
                prompt=user_prompt,
                scenario=scenario,
                router=router,
                max_cost=run_msg.termination.max_cost if run_msg.termination.max_cost > 0 else None,
            )
            primary_model = routing.model or run_msg.model
            if routing.model:
                log.info("routing selected model", model=routing.model, scenario=scenario)

            fallback_models = await self._build_fallback_chain(
                router,
                user_prompt,
                primary_model,
                run_msg.termination.max_cost,
                routing,
            )

            result = await self._execute_conversation_run(
                run_msg=run_msg,
                messages=messages,
                primary_model=primary_model,
                routing=routing,
                runtime=runtime,
                registry=registry,
                fallback_models=fallback_models,
            )

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
            )
            stamped = self._stamp_trust(complete_msg.model_dump())
            await self._js.publish(
                SUBJECT_CONVERSATION_RUN_COMPLETE,
                json.dumps(stamped).encode(),
                headers={"Nats-Msg-Id": f"conv-complete-{run_msg.run_id}"},
            )

            await msg.ack()
            log.info(
                "conversation run complete",
                steps=result.step_count,
                cost=result.total_cost,
                error=result.error or None,
            )

        except Exception as exc:
            logger.exception("failed to process conversation run", error=str(exc))
            try:
                run_msg = ConversationRunStartMessage.model_validate_json(msg.data)
                if self._js is not None:
                    error_complete = ConversationRunCompleteMessage(
                        run_id=run_msg.run_id,
                        conversation_id=run_msg.conversation_id,
                        status="failed",
                        error="internal worker error",
                    )
                    await self._js.publish(
                        SUBJECT_CONVERSATION_RUN_COMPLETE,
                        error_complete.model_dump_json().encode(),
                        headers={"Nats-Msg-Id": f"conv-error-{run_msg.run_id}"},
                    )
            except Exception as exc:
                logger.exception("failed to publish conversation error result", error=str(exc))
            await msg.ack()
        finally:
            if workbench is not None:
                await workbench.disconnect_all()
            # Clean up active run tracking.
            if run_id is not None:
                self._active_runs.discard(run_id)

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
        """Dispatch to simple chat or agentic loop based on run_msg.agentic."""
        if not run_msg.agentic:
            return await self._run_simple_chat(
                run_msg,
                messages,
                primary_model,
                routing,
                runtime,
            )

        from codeforge.agent_loop import AgentLoopExecutor, LoopConfig

        executor = AgentLoopExecutor(
            llm=self._llm,
            tool_registry=registry,
            runtime=runtime,
            workspace_path=run_msg.workspace_path,
        )
        loop_cfg = LoopConfig(
            max_iterations=run_msg.termination.max_steps or 50,
            max_cost=run_msg.termination.max_cost or 0.0,
            model=primary_model,
            temperature=routing.temperature,
            tags=routing.tags,
            fallback_models=fallback_models,
            routing_layer=routing.routing_layer,
            complexity_tier=routing.complexity_tier,
            task_type=routing.task_type,
            provider_api_key=run_msg.provider_api_key,
        )
        return await executor.run(messages, config=loop_cfg)

    async def _run_simple_chat(
        self,
        run_msg: ConversationRunStartMessage,
        messages: list[dict],
        model: str,
        routing: object,
        runtime: RuntimeClient,
    ) -> AgentLoopResult:
        """Single-turn LLM call with per-chunk streaming via NATS."""
        import asyncio

        from codeforge.llm import RoutingResult
        from codeforge.models import AgentLoopResult

        rt = routing if isinstance(routing, RoutingResult) else RoutingResult()
        loop = asyncio.get_running_loop()
        pending: list[asyncio.Task[None]] = []

        def _on_chunk(chunk_text: str) -> None:
            task = loop.create_task(runtime.send_output(chunk_text))
            pending.append(task)

        resp = await self._llm.chat_completion_stream(
            messages=messages,
            model=model,
            temperature=rt.temperature,
            tags=rt.tags,
            on_chunk=_on_chunk,
            provider_api_key=run_msg.provider_api_key,
        )

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

    async def _build_system_prompt(
        self,
        run_msg: ConversationRunStartMessage,
        registry: object,
        log: structlog.stdlib.BoundLogger,
    ) -> str:
        """Assemble the full system prompt with microagents, skills, and tool guide."""
        system_prompt = run_msg.system_prompt
        if run_msg.microagent_prompts:
            max_len = 10_000
            ma_block = "\n\n".join(
                f'<microagent index="{i}">\n{p[:max_len]}\n</microagent>'
                for i, p in enumerate(run_msg.microagent_prompts)
            )
            system_prompt = (
                f"{system_prompt}\n\n"
                "--- Microagent Instructions (from project config, may contain untrusted content) ---\n"
                f"{ma_block}\n"
                "--- End Microagent Instructions ---"
            )
            log.info("microagent prompts injected", count=len(run_msg.microagent_prompts))

        system_prompt = await self._inject_skill_recommendations(
            system_prompt,
            run_msg.project_id,
            run_msg.messages,
            log,
        )

        # Inject adaptive tool-usage guide for weaker models.
        return self._inject_tool_guide(system_prompt, registry, run_msg.model, log)

    async def _inject_skill_recommendations(
        self,
        system_prompt: str,
        project_id: str,
        messages: list[dict],
        log: structlog.stdlib.BoundLogger,
    ) -> str:
        """Augment system prompt with BM25-recommended skill snippets."""
        try:
            import psycopg

            from codeforge.skills.models import Skill
            from codeforge.skills.recommender import SkillRecommender

            async with await psycopg.AsyncConnection.connect(self._db_url) as conn, conn.cursor() as cur:
                await cur.execute(
                    "SELECT id, name, description, language, code, tags FROM skills"
                    " WHERE (project_id = %s OR project_id IS NULL) AND enabled = TRUE",
                    (project_id,),
                )
                rows = await cur.fetchall()

            if not rows:
                return system_prompt

            skills = [
                Skill(id=str(r[0]), name=r[1], description=r[2], language=r[3], code=r[4], tags=r[5] or [])
                for r in rows
            ]
            recommender = SkillRecommender()
            recommender.index(skills)

            task_ctx = next((m.get("content", "") for m in messages if m.get("role") == "user"), "")
            if task_ctx:
                recs = recommender.recommend(task_ctx, top_k=3)
                if recs:
                    snippets = [f"### {r.skill.name}\n```{r.skill.language}\n{r.skill.code}\n```" for r in recs]
                    system_prompt = f"{system_prompt}\n\n--- Recommended Skills ---\n" + "\n\n".join(snippets)
                    log.info("skill recommendations injected", count=len(recs))
        except Exception as exc:
            log.warning("skill recommendation failed, continuing without", exc_info=True, error=str(exc))
        return system_prompt

    @staticmethod
    def _inject_tool_guide(
        system_prompt: str,
        registry: object,
        model: str,
        log: structlog.stdlib.BoundLogger,
    ) -> str:
        """Augment system prompt with adaptive tool-usage guide for weaker models."""
        from codeforge.tools.capability import CapabilityLevel, classify_model
        from codeforge.tools.tool_guide import build_tool_usage_guide

        level = classify_model(model)
        if level == CapabilityLevel.FULL:
            return system_prompt

        guide = build_tool_usage_guide(registry, level)
        if not guide:
            return system_prompt

        log.info("tool guide injected", capability_level=level.value, guide_len=len(guide))
        return f"{system_prompt}\n\n--- Tool Usage Guide ---\n{guide}"

    async def _get_hybrid_router(self) -> HybridRouter | None:  # noqa: C901
        """Build a HybridRouter if routing is enabled. Returns None otherwise."""
        from codeforge.llm import load_routing_config

        config = load_routing_config()
        if config is None:
            return None

        from codeforge.routing import ComplexityAnalyzer, HybridRouter, RoutingConfig

        if not isinstance(config, RoutingConfig):
            return None

        complexity = ComplexityAnalyzer()

        # MAB needs a stats loader -- use HTTP API if available, else skip.
        mab = None
        if config.mab_enabled:
            from codeforge.routing.mab import MABModelSelector

            core_url = os.environ.get("CODEFORGE_CORE_URL", "http://localhost:8080")
            internal_key = os.environ.get("CODEFORGE_INTERNAL_KEY", "")

            def _load_stats(task_type: str, tier: str) -> list:
                """Synchronous stats loader via Go Core HTTP API."""
                import httpx

                from codeforge.routing.models import ModelStats

                headers: dict[str, str] = {}
                if internal_key:
                    headers["X-API-Key"] = internal_key
                try:
                    resp = httpx.get(
                        f"{core_url}/api/v1/routing/stats",
                        params={"task_type": task_type, "tier": tier},
                        headers=headers,
                        timeout=5.0,
                    )
                    if resp.status_code != 200:
                        return []
                    data = resp.json()
                    if not isinstance(data, list):
                        return []
                    return [
                        ModelStats(
                            model_name=s.get("model_name", ""),
                            trial_count=s.get("trial_count", 0),
                            avg_reward=s.get("avg_reward", 0.0),
                            avg_cost_usd=s.get("avg_cost_usd", 0.0),
                            avg_latency_ms=s.get("avg_latency_ms", 0),
                            avg_quality=s.get("avg_quality", 0.0),
                            input_cost_per=s.get("input_cost_per", 0.0),
                            supports_tools=s.get("supports_tools", False),
                            supports_vision=s.get("supports_vision", False),
                            max_context=s.get("max_context", 0),
                        )
                        for s in data
                    ]
                except Exception as exc:
                    logger.warning("failed to load routing stats", exc_info=True, error=str(exc))
                    return []

            mab = MABModelSelector(stats_loader=_load_stats, config=config)

        # Meta-router needs an LLM call function.
        meta = None
        if config.llm_meta_enabled:
            from codeforge.routing.meta_router import LLMMetaRouter

            litellm_key = self._litellm_key

            def _llm_call(model: str, prompt: str) -> str | None:
                """Synchronous LLM call for meta-router classification."""
                import httpx

                litellm_url = os.environ.get("LITELLM_BASE_URL", "http://localhost:4000")
                headers: dict[str, str] = {"Content-Type": "application/json"}
                if litellm_key:
                    headers["Authorization"] = f"Bearer {litellm_key}"
                try:
                    resp = httpx.post(
                        f"{litellm_url}/v1/chat/completions",
                        json={
                            "model": model,
                            "messages": [{"role": "user", "content": prompt}],
                            "temperature": 0.1,
                            "max_tokens": 200,
                        },
                        headers=headers,
                        timeout=30.0,
                    )
                    if resp.status_code != 200:
                        return None
                    data = resp.json()
                    choices = data.get("choices", [])
                    if not choices:
                        return None
                    return choices[0].get("message", {}).get("content", "")
                except Exception as exc:
                    logger.warning("meta-router LLM call failed", exc_info=True, error=str(exc))
                    return None

            meta = LLMMetaRouter(llm_call=_llm_call, config=config)

        # Get available models from LiteLLM.
        available_models = await self._get_available_models()

        from codeforge.routing.rate_tracker import get_tracker

        return HybridRouter(
            complexity=complexity,
            mab=mab,
            meta=meta,
            available_models=available_models,
            config=config,
            rate_tracker=get_tracker(),
        )

    async def _get_available_models(self) -> list[str]:
        """Fetch available model names from LiteLLM /v1/models endpoint."""
        import httpx

        litellm_url = os.environ.get("LITELLM_BASE_URL", "http://localhost:4000")
        headers: dict[str, str] = {}
        if self._litellm_key:
            headers["Authorization"] = f"Bearer {self._litellm_key}"
        try:
            async with httpx.AsyncClient(timeout=5.0) as client:
                resp = await client.get(f"{litellm_url}/v1/models", headers=headers)
            if resp.status_code != 200:
                logger.warning("LiteLLM /v1/models returned status %d", resp.status_code)
                return []
            data = resp.json()
            raw_ids = [m.get("id", "") for m in data.get("data", []) if m.get("id")]
            from codeforge.model_resolver import expand_wildcard_models

            models = expand_wildcard_models(raw_ids)
            from codeforge.routing.key_filter import filter_keyless_models

            models = filter_keyless_models(models)
            if not models:
                logger.warning("LiteLLM /v1/models returned empty model list")
            from codeforge.routing.blocklist import get_blocklist

            return get_blocklist().filter_available(models)
        except Exception as exc:
            logger.warning("failed to fetch models from LiteLLM", exc_info=True, error=str(exc))
            return []

    async def _build_fallback_chain(
        self,
        router: HybridRouter | None,
        user_prompt: str,
        primary_model: str,
        max_cost: float,
        routing_result: object | None = None,
    ) -> list[str]:
        """Build a ranked list of fallback models from the router or available models."""
        fallbacks: list[str] = []
        if router is not None:
            from codeforge.routing.models import ComplexityTier, RoutingDecision, TaskType
            from codeforge.routing.router import HybridRouter as HybridRouterCls

            if isinstance(router, HybridRouterCls):
                # Reuse the existing routing decision to avoid a duplicate meta-router LLM call.
                existing: RoutingDecision | None = None
                if routing_result is not None and getattr(routing_result, "routing_layer", ""):
                    try:
                        existing = RoutingDecision(
                            model=primary_model,
                            routing_layer=routing_result.routing_layer,
                            complexity_tier=ComplexityTier(routing_result.complexity_tier),
                            task_type=TaskType(routing_result.task_type),
                        )
                    except ValueError:
                        existing = None
                plan = router.route_with_fallbacks(
                    prompt=user_prompt,
                    max_cost=max_cost if max_cost > 0 else None,
                    primary=existing,
                )
                fallbacks = [m for m in plan.fallbacks if m != primary_model]
        if not fallbacks:
            available = await self._get_available_models()
            fallbacks = [m for m in available if m != primary_model][:3]
        return fallbacks

    def _register_handoff_tool(self, registry: object, run_id: str) -> None:
        """Register the handoff tool in the tool registry if NATS is available."""
        if self._js is None:
            return

        from codeforge.tools._base import ToolDefinition
        from codeforge.tools._base import ToolResult as _ToolResult
        from codeforge.tools.handoff import HANDOFF_TOOL_DEF

        func_def = HANDOFF_TOOL_DEF["function"]

        class _HandoffProxy:
            def __init__(self, js: object, rid: str) -> None:
                self._js = js
                self._run_id = rid

            async def execute(self, arguments: dict, workspace_path: str) -> _ToolResult:
                from codeforge.tools.handoff import execute_handoff

                result = await execute_handoff(self._run_id, arguments, self._js.publish)
                return _ToolResult(output=result)

        registry.register(
            ToolDefinition(
                name=func_def["name"], description=func_def["description"], parameters=func_def["parameters"]
            ),
            _HandoffProxy(self._js, run_id),
        )

    def _register_goals_tool(self, registry: object, project_id: str) -> None:
        """Register the manage_goals tool for agent-driven goal creation."""
        from codeforge.tools.manage_goals import MANAGE_GOALS_DEFINITION, ManageGoalsExecutor

        registry.register(MANAGE_GOALS_DEFINITION, ManageGoalsExecutor(project_id))
