"""Quality gate handler mixin."""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._subjects import SUBJECT_QG_RESULT
from codeforge.models import QualityGateRequest, QualityGateResult

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()


class QualityGateHandlerMixin:
    """Handles runs.qualitygate.request messages."""

    async def _handle_quality_gate(self, msg: nats.aio.msg.Msg) -> None:
        """Process a quality gate request: run tests/lint and publish result."""
        try:
            request = QualityGateRequest.model_validate_json(msg.data)
            log = logger.bind(run_id=request.run_id)

            if self._is_duplicate(f"qgate-{request.run_id}"):
                log.warning("duplicate quality gate request, skipping")
                await msg.ack()
                return

            log.info("received quality gate request")

            result: QualityGateResult = await self._gate_executor.execute(request)

            if self._js is not None:
                await self._js.publish(
                    SUBJECT_QG_RESULT,
                    result.model_dump_json().encode(),
                )

            await msg.ack()
            log.info("quality gate completed", tests_passed=result.tests_passed, lint_passed=result.lint_passed)

        except Exception as exc:
            logger.exception("failed to process quality gate request", error=str(exc))
            await msg.nak()
