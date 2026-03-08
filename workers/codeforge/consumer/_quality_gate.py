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
        await self._handle_request(
            msg=msg,
            request_model=QualityGateRequest,
            dedup_key=lambda r: f"qgate-{r.run_id}",
            handler=self._do_quality_gate,
            result_subject=SUBJECT_QG_RESULT,
            log_context=lambda r: {"run_id": r.run_id},
        )

    async def _do_quality_gate(self, request: QualityGateRequest, log: structlog.BoundLogger) -> QualityGateResult:
        """Business logic for quality gate execution."""
        log.info("received quality gate request")
        result: QualityGateResult = await self._gate_executor.execute(request)
        log.info(
            "quality gate completed",
            tests_passed=result.tests_passed,
            lint_passed=result.lint_passed,
        )
        return result
