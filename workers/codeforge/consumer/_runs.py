"""Run start handler mixin."""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from codeforge.models import RunStartMessage, TaskMessage
from codeforge.runtime import RuntimeClient

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()


class RunHandlerMixin:
    """Handles runs.start messages — runtime protocol execution."""

    async def _handle_run_start(self, msg: nats.aio.msg.Msg) -> None:
        """Process a run start message: parse, create RuntimeClient, execute with runtime."""
        try:
            run_msg = RunStartMessage.model_validate_json(msg.data)
            log = logger.bind(run_id=run_msg.run_id, task_id=run_msg.task_id)
            log.info("received run start", prompt=run_msg.prompt[:80])

            if self._js is None:
                log.error("JetStream not available")
                await msg.nak()
                return

            runtime = RuntimeClient(
                js=self._js,
                run_id=run_msg.run_id,
                task_id=run_msg.task_id,
                project_id=run_msg.project_id,
                termination=run_msg.termination,
            )
            await runtime.start_cancel_listener()

            # Enrich prompt with pre-packed context entries (Phase 5D)
            enriched_prompt = run_msg.prompt
            if run_msg.context:
                context_section = "\n\n--- Relevant Context ---\n"
                for entry in run_msg.context:
                    context_section += f"\n### {entry.kind}: {entry.path}\n{entry.content}\n"
                enriched_prompt = run_msg.prompt + context_section
                log.info("context injected", entries=len(run_msg.context))

            # Inject matched microagent prompts (Phase 22C)
            if run_msg.microagent_prompts:
                ma_block = "\n\n".join(run_msg.microagent_prompts)
                enriched_prompt = f"{enriched_prompt}\n\n--- Microagent Instructions ---\n{ma_block}"
                log.info("microagent prompts injected", count=len(run_msg.microagent_prompts))

            task = TaskMessage(
                id=run_msg.task_id,
                project_id=run_msg.project_id,
                title=run_msg.prompt[:80],
                prompt=enriched_prompt,
                config=run_msg.config,
            )

            await self._executor.execute_with_runtime(task, runtime, mode=run_msg.mode)
            await msg.ack()
            log.info("run processing complete", mode_id=run_msg.mode.id)

        except Exception:
            logger.exception("failed to process run start message")
            await msg.nak()
