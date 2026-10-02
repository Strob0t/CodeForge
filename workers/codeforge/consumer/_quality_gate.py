"""Quality gate handler mixin."""

from __future__ import annotations

import contextlib
from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._subjects import SUBJECT_QG_RESULT, SUBJECT_RUN_HEARTBEAT
from codeforge.models import QualityGateRequest, QualityGateResult
from codeforge.runtime import heartbeats
from codeforge.tool_identity import ToolIsolationError, tool_tenant

if TYPE_CHECKING:
    from collections.abc import AsyncIterator

    import nats.aio.msg
    import nats.js.client

logger = structlog.get_logger()

# Heartbeat interval when the request does not set one (an older Go Core).
DEFAULT_GATE_HEARTBEAT_SECONDS: float = 30.0

# Phase of the heartbeats sent while a gate runs (Go: HeartbeatPhaseQualityGate).
HEARTBEAT_PHASE_QUALITY_GATE = "quality_gate"


def gate_heartbeat_interval(request: QualityGateRequest) -> float:
    """Seconds between two heartbeats of the request's gate."""
    return float(request.heartbeat_seconds) if request.heartbeat_seconds > 0 else DEFAULT_GATE_HEARTBEAT_SECONDS


@contextlib.asynccontextmanager
async def gate_heartbeat(
    js: nats.js.client.JetStreamContext | None,
    msg: nats.aio.msg.Msg,
    request: QualityGateRequest,
    log: structlog.BoundLogger,
) -> AsyncIterator[None]:
    """Report the gate as running while the block runs.

    A runs.heartbeat (phase quality_gate, the run's tenant) right away and then
    every interval tells the Go Core's watchdog the gate is alive, from the
    moment it starts; an in-progress ack keeps JetStream from redelivering the
    request to another worker while the gate runs (the gate's own timeouts
    bound it, so this never keeps a hung handler alive).
    """
    if js is None:
        log.warning("JetStream not available, quality gate runs without heartbeats")
        yield
        return

    async def keep_in_progress() -> None:
        if not msg.is_acked:
            await msg.in_progress()

    payload = {"run_id": request.run_id, "tenant_id": request.tenant_id, "phase": HEARTBEAT_PHASE_QUALITY_GATE}
    async with heartbeats(
        js, SUBJECT_RUN_HEARTBEAT, payload, gate_heartbeat_interval(request), on_beat=keep_in_progress
    ):
        yield


class QualityGateHandlerMixin:
    """Handles runs.qualitygate.request messages."""

    async def _handle_quality_gate(self, msg: nats.aio.msg.Msg) -> None:
        """Process a quality gate request: run tests/lint and publish result."""

        async def run_gate(request: QualityGateRequest, log: structlog.BoundLogger) -> QualityGateResult:
            async with gate_heartbeat(self._js, msg, request, log):
                try:
                    # The gate commands run as the run's tenant's tool UID (KI-96).
                    async with tool_tenant(request.tenant_id, request.tool_uid, request.workspace_path):
                        return await self._do_quality_gate(request, log)
                except ToolIsolationError as exc:
                    # A gate that could not run: no verdict, the reason (no rollback).
                    log.error("quality gate refused", error=str(exc))
                    return QualityGateResult(run_id=request.run_id, tenant_id=request.tenant_id, error=str(exc))

        await self._handle_request(
            msg=msg,
            request_model=QualityGateRequest,
            dedup_key=lambda r: f"qgate-{r.run_id}",
            handler=run_gate,
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
            error=result.error,
        )
        return result
