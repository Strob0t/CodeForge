"""Conversation run handler mixin."""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._subjects import SUBJECT_CONVERSATION_RUN_COMPLETE
from codeforge.models import ConversationRunCompleteMessage, ConversationRunStartMessage
from codeforge.runtime import RuntimeClient

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()


class ConversationHandlerMixin:
    """Handles conversation.run.start messages — agentic loop with tool calling."""

    async def _handle_conversation_run(self, msg: nats.aio.msg.Msg) -> None:
        """Process a conversation run: agentic loop with tool calling."""
        from codeforge.agent_loop import AgentLoopExecutor, LoopConfig
        from codeforge.history import ConversationHistoryManager, HistoryConfig
        from codeforge.mcp_workbench import McpWorkbench
        from codeforge.tools import ToolRegistry, build_default_registry

        workbench: McpWorkbench | None = None
        try:
            run_msg = ConversationRunStartMessage.model_validate_json(msg.data)
            log = logger.bind(run_id=run_msg.run_id, conversation_id=run_msg.conversation_id)
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

            system_prompt = run_msg.system_prompt
            if run_msg.microagent_prompts:
                ma_block = "\n\n".join(run_msg.microagent_prompts)
                system_prompt = f"{system_prompt}\n\n--- Microagent Instructions ---\n{ma_block}"
                log.info("microagent prompts injected", count=len(run_msg.microagent_prompts))

            system_prompt = await self._inject_skill_recommendations(
                system_prompt,
                run_msg.project_id,
                run_msg.messages,
                log,
            )
            self._register_handoff_tool(registry, run_msg.run_id)

            history_mgr = ConversationHistoryManager(
                HistoryConfig(
                    max_context_tokens=128_000,
                )
            )
            messages = history_mgr.build_messages(
                system_prompt=system_prompt,
                history=run_msg.messages,
                context_entries=run_msg.context,
            )

            from codeforge.llm import resolve_scenario

            scenario_tags: list[str] = []
            if run_msg.mode and run_msg.mode.llm_scenario:
                scenario_cfg = resolve_scenario(run_msg.mode.llm_scenario)
                if scenario_cfg.tag:
                    scenario_tags = [scenario_cfg.tag]
                    log.info("scenario resolved", scenario=run_msg.mode.llm_scenario, tag=scenario_cfg.tag)

            executor = AgentLoopExecutor(
                llm=self._llm,
                tool_registry=registry,
                runtime=runtime,
                workspace_path=run_msg.workspace_path,
            )
            loop_cfg = LoopConfig(
                max_iterations=run_msg.termination.max_steps or 50,
                max_cost=run_msg.termination.max_cost or 0.0,
                model=run_msg.model,
                tags=scenario_tags,
            )
            result = await executor.run(messages, config=loop_cfg)

            complete_msg = ConversationRunCompleteMessage(
                run_id=run_msg.run_id,
                conversation_id=run_msg.conversation_id,
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
            await self._js.publish(
                SUBJECT_CONVERSATION_RUN_COMPLETE,
                complete_msg.model_dump_json().encode(),
            )

            await msg.ack()
            log.info(
                "conversation run complete",
                steps=result.step_count,
                cost=result.total_cost,
                error=result.error or None,
            )

        except Exception:
            logger.exception("failed to process conversation run")
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
                    )
            except Exception:
                logger.exception("failed to publish conversation error result")
            await msg.ack()
        finally:
            if workbench is not None:
                await workbench.disconnect_all()

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
        except Exception:
            log.warning("skill recommendation failed, continuing without", exc_info=True)
        return system_prompt

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
