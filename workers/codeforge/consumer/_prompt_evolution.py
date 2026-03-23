"""Prompt evolution handler mixin for reflection, mutation, and event awareness."""

from __future__ import annotations

import json
import uuid
from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._subjects import (
    SUBJECT_PROMPT_EVOLUTION_MUTATE_COMPLETE,
    SUBJECT_PROMPT_EVOLUTION_REFLECT_COMPLETE,
)
from codeforge.evaluation.prompt_mutator import mutate_prompt
from codeforge.evaluation.prompt_optimizer import (
    TacticalFix,
    reflect_on_failures,
)
from codeforge.models import (
    PromptEvolutionMutateComplete,
    PromptEvolutionReflectComplete,
    PromptEvolutionReflectRequest,
    PromptEvolutionTacticalFix,
)

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()


class PromptEvolutionHandlerMixin:
    """Handles prompt.evolution.reflect NATS messages."""

    async def _handle_prompt_evolution_reflect(self, msg: nats.aio.msg.Msg) -> None:
        await self._handle_request(  # type: ignore[attr-defined]
            msg=msg,
            request_model=PromptEvolutionReflectRequest,
            dedup_key=lambda r: f"prompt-evo-{r.tenant_id}-{r.mode_id}-{r.model_family}",
            handler=self._do_prompt_evolution_reflect,
            log_context=lambda r: {
                "tenant_id": str(r.tenant_id),
                "mode_id": r.mode_id,
                "model_family": r.model_family,
                "failure_count": len(r.failures),
            },
        )

    async def _do_prompt_evolution_reflect(
        self,
        request: PromptEvolutionReflectRequest,
        log: structlog.BoundLogger,
    ) -> None:
        js = self._js  # type: ignore[attr-defined]
        llm = self._llm  # type: ignore[attr-defined]

        log.info("prompt evolution reflect started")

        try:
            report = await reflect_on_failures(
                failures=request.failures,
                current_prompt=request.current_prompt,
                mode_id=request.mode_id,
                model_family=request.model_family,
                llm_client=llm,
            )

            result = PromptEvolutionReflectComplete(
                tenant_id=request.tenant_id,
                mode_id=request.mode_id,
                model_family=request.model_family,
                tactical_fixes=[
                    PromptEvolutionTacticalFix(
                        task_id=fix.task_id,
                        failure_description=fix.failure_description,
                        root_cause=fix.root_cause,
                        proposed_addition=fix.proposed_addition,
                        confidence=fix.confidence,
                    )
                    for fix in report.tactical_fixes
                ],
                strategic_principles=report.strategic_principles,
            )

            # Trigger mutation inline after reflection.
            tactical_fixes = [
                TacticalFix(
                    task_id=fix.task_id,
                    failure_description=fix.failure_description,
                    root_cause=fix.root_cause,
                    proposed_addition=fix.proposed_addition,
                    confidence=fix.confidence,
                )
                for fix in result.tactical_fixes
            ]

            if tactical_fixes or result.strategic_principles:
                variant = await mutate_prompt(
                    current_content=request.current_prompt,
                    tactical_fixes=tactical_fixes,
                    strategic_principles=result.strategic_principles,
                    mode_id=request.mode_id,
                    llm_client=llm,
                )

                mutate_result = PromptEvolutionMutateComplete(
                    tenant_id=request.tenant_id,
                    mode_id=request.mode_id,
                    model_family=request.model_family,
                    variant_content=variant.content,
                    version=variant.version,
                    parent_id=variant.parent_id,
                    mutation_source=variant.mutation_source,
                    validation_passed=variant.validation_passed,
                )

                if js is not None:
                    await js.publish(
                        SUBJECT_PROMPT_EVOLUTION_MUTATE_COMPLETE,
                        mutate_result.model_dump_json().encode(),
                        headers={"Nats-Msg-Id": str(uuid.uuid4())},
                    )

            log.info("prompt evolution reflect completed", fixes=len(result.tactical_fixes))

        except Exception as exc:
            log.exception("prompt evolution reflect failed", error=str(exc))
            result = PromptEvolutionReflectComplete(
                tenant_id=request.tenant_id,
                mode_id=request.mode_id,
                model_family=request.model_family,
                error=str(exc),
            )

        if js is not None:
            await js.publish(
                SUBJECT_PROMPT_EVOLUTION_REFLECT_COMPLETE,
                result.model_dump_json().encode(),
                headers={"Nats-Msg-Id": str(uuid.uuid4())},
            )

    async def _handle_prompt_promoted(self, msg: nats.aio.msg.Msg) -> None:
        """Handle prompt.evolution.promoted event.

        Awareness event: logs that a variant was promoted so Python workers
        know to clear any cached prompt variants.
        """
        try:
            data = json.loads(msg.data)
            logger.info(
                "prompt variant promoted, clearing cached prompts",
                mode_id=data.get("mode_id", ""),
                variant_id=data.get("variant_id", ""),
                new_version=data.get("new_version"),
            )
        except (json.JSONDecodeError, Exception) as exc:
            logger.warning("failed to parse prompt promoted event", error=str(exc))
        await msg.ack()

    async def _handle_prompt_reverted(self, msg: nats.aio.msg.Msg) -> None:
        """Handle prompt.evolution.reverted event.

        Awareness event: logs that a variant was reverted so Python workers
        know to clear any cached prompt variants.
        """
        try:
            data = json.loads(msg.data)
            logger.info(
                "prompt variant reverted, clearing cached prompts",
                mode_id=data.get("mode_id", ""),
                variant_id=data.get("variant_id", ""),
                new_version=data.get("new_version"),
            )
        except (json.JSONDecodeError, Exception) as exc:
            logger.warning("failed to parse prompt reverted event", error=str(exc))
        await msg.ack()
