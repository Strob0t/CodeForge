"""Runtime client for the step-by-step execution protocol (Phase 4B).

This module handles the conversational NATS protocol between Python workers
and the Go control plane. Instead of fire-and-forget task execution, each
tool call is individually approved by the control plane's policy engine.
"""

from __future__ import annotations

import asyncio
import contextlib
import json
import time
import uuid
from datetime import UTC, datetime
from typing import TYPE_CHECKING

import structlog
from nats.js.api import ConsumerConfig, DeliverPolicy

from codeforge.constants import NATS_RESPONSE_TIMEOUT_SECONDS
from codeforge.metrics import ExecutionMetrics
from codeforge.models import RunCompleteMessage, ToolCallDecision

if TYPE_CHECKING:
    from nats.js.client import JetStreamContext

    from codeforge.models import TerminationConfig

# NATS subjects for the run protocol
SUBJECT_TOOLCALL_REQUEST = "runs.toolcall.request"
SUBJECT_TOOLCALL_RESPONSE = "runs.toolcall.response"
SUBJECT_TOOLCALL_RESULT = "runs.toolcall.result"
SUBJECT_RUN_COMPLETE = "runs.complete"
SUBJECT_RUN_OUTPUT = "runs.output"
SUBJECT_RUN_CANCEL = "runs.cancel"
SUBJECT_CONVERSATION_RUN_CANCEL = "conversation.run.cancel"
SUBJECT_RUN_HEARTBEAT = "runs.heartbeat"

RESPONSE_TIMEOUT_SECONDS = NATS_RESPONSE_TIMEOUT_SECONDS

logger = structlog.get_logger()


class RuntimeClient:
    """Handles the run protocol: request permission, report results, complete run.

    The RuntimeClient communicates with the Go control plane over NATS,
    requesting permission for each tool call and reporting results back.
    """

    def __init__(
        self,
        js: JetStreamContext,
        run_id: str,
        task_id: str,
        project_id: str,
        termination: TerminationConfig,
    ) -> None:
        self._js = js
        self.run_id = run_id
        self.task_id = task_id
        self.project_id = project_id
        self.termination = termination
        self._metrics = ExecutionMetrics()
        self._cancelled = False
        self._cancel_sub: object | None = None
        self._heartbeat_task: asyncio.Task[None] | None = None
        self._log = logger.bind(run_id=run_id, task_id=task_id)

    async def start_cancel_listener(self, extra_subjects: list[str] | None = None) -> None:
        """Subscribe to cancellation messages for this run.

        Listens on the default runs.cancel subject plus any extra subjects
        (e.g. conversation.run.cancel for conversation runs).
        """
        subjects = [SUBJECT_RUN_CANCEL] + (extra_subjects or [])
        _new_only = ConsumerConfig(deliver_policy=DeliverPolicy.NEW)
        subs = [await self._js.subscribe(s, config=_new_only) for s in subjects]
        self._cancel_sub = subs  # type: ignore[assignment]

        async def _listen_sub(sub: object) -> None:
            while not self._cancelled:
                try:
                    msg = await sub.next_msg(timeout=1.0)  # type: ignore[attr-defined]
                    data = json.loads(msg.data)
                    if data.get("run_id") == self.run_id:
                        self._cancelled = True
                        self._log.info("run cancelled by control plane")
                except TimeoutError:
                    continue
                except Exception as exc:
                    logger.debug("cancel listener error", error=str(exc))
                    break

        for sub in subs:
            asyncio.create_task(_listen_sub(sub))  # noqa: RUF006

    async def start_heartbeat(self, interval: float = 30.0) -> None:
        """Start periodic heartbeat to the control plane."""

        async def _beat() -> None:
            while not self._cancelled:
                payload = {
                    "run_id": self.run_id,
                    "timestamp": datetime.now(UTC).isoformat(),
                }
                try:
                    await self._js.publish(
                        SUBJECT_RUN_HEARTBEAT,
                        json.dumps(payload).encode(),
                    )
                except Exception as exc:
                    self._log.warning("heartbeat publish failed", error=str(exc))
                await asyncio.sleep(interval)

        self._heartbeat_task = asyncio.create_task(_beat())

    async def stop_heartbeat(self) -> None:
        """Stop the heartbeat ticker."""
        if self._heartbeat_task and not self._heartbeat_task.done():
            self._heartbeat_task.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await self._heartbeat_task
            self._heartbeat_task = None

    @property
    def is_cancelled(self) -> bool:
        """Whether this run has been cancelled."""
        return self._cancelled

    @property
    def step_count(self) -> int:
        """Number of tool calls executed so far."""
        return self._metrics.step_count

    @property
    def total_cost(self) -> float:
        """Accumulated cost of this run."""
        return self._metrics.total_cost

    async def request_tool_call(
        self,
        tool: str,
        command: str = "",
        path: str = "",
    ) -> ToolCallDecision:
        """Request permission from the control plane to execute a tool call.

        Publishes a request to NATS, then waits for the response.
        Returns the decision (allow/deny/ask).
        """
        if self._cancelled:
            return ToolCallDecision(
                call_id="",
                decision="deny",
                reason="run cancelled",
            )

        call_id = str(uuid.uuid4())
        request = {
            "run_id": self.run_id,
            "call_id": call_id,
            "tool": tool,
            "command": command,
            "path": path,
        }

        start_time = time.monotonic()
        self._log.debug("requesting tool call", tool=tool, call_id=call_id)

        # Subscribe BEFORE publishing to avoid a race condition where Go
        # responds before the subscription is established.
        # Use DeliverNew to skip old messages in the stream — we only care
        # about the response to the request we are about to publish.
        sub = await self._js.subscribe(
            SUBJECT_TOOLCALL_RESPONSE,
            config=ConsumerConfig(deliver_policy=DeliverPolicy.NEW),
        )
        try:
            try:
                await self._js.publish(
                    SUBJECT_TOOLCALL_REQUEST,
                    json.dumps(request).encode(),
                )
            except Exception as pub_err:
                elapsed_ms = (time.monotonic() - start_time) * 1000
                self._log.error(
                    "NATS publish failed for tool call request",
                    call_id=call_id,
                    tool=tool,
                    elapsed_ms=round(elapsed_ms, 1),
                    error=str(pub_err),
                )
                return ToolCallDecision(
                    call_id=call_id,
                    decision="deny",
                    reason=f"NATS publish failed: {pub_err}",
                )

            publish_ms = (time.monotonic() - start_time) * 1000
            self._log.debug(
                "tool call request published",
                call_id=call_id,
                tool=tool,
                publish_ms=round(publish_ms, 1),
            )

            deadline = asyncio.get_event_loop().time() + RESPONSE_TIMEOUT_SECONDS
            while True:
                remaining = deadline - asyncio.get_event_loop().time()
                if remaining <= 0:
                    elapsed_ms = (time.monotonic() - start_time) * 1000
                    self._log.warning(
                        "NATS response timeout waiting for policy decision from Go control plane",
                        call_id=call_id,
                        tool=tool,
                        elapsed_ms=round(elapsed_ms, 1),
                        timeout_seconds=RESPONSE_TIMEOUT_SECONDS,
                    )
                    return ToolCallDecision(
                        call_id=call_id,
                        decision="deny",
                        reason=f"NATS response timeout after {RESPONSE_TIMEOUT_SECONDS}s "
                        f"waiting for policy decision (not an LLM timeout)",
                    )

                try:
                    msg = await sub.next_msg(timeout=remaining)
                except TimeoutError:
                    # next_msg() raises nats.errors.TimeoutError (subclass of
                    # TimeoutError) when no message arrives before the timeout.
                    # Retry until the overall deadline expires.
                    continue

                data = json.loads(msg.data)
                if data.get("call_id") == call_id:
                    elapsed_ms = (time.monotonic() - start_time) * 1000
                    decision = data.get("decision", "deny")
                    reason = data.get("reason", "")

                    log_method = self._log.debug if decision == "allow" else self._log.info
                    log_method(
                        "tool call decision received",
                        call_id=call_id,
                        tool=tool,
                        decision=decision,
                        reason=reason,
                        elapsed_ms=round(elapsed_ms, 1),
                    )

                    return ToolCallDecision(
                        call_id=call_id,
                        decision=decision,
                        reason=reason,
                    )
        finally:
            await sub.unsubscribe()

    async def report_tool_result(
        self,
        call_id: str,
        tool: str,
        success: bool,
        output: str = "",
        error: str = "",
        cost_usd: float = 0.0,
        tokens_in: int = 0,
        tokens_out: int = 0,
        model: str = "",
    ) -> None:
        """Report the outcome of an executed tool call back to the control plane."""
        self._metrics.step_count += 1
        self._metrics.record(cost=cost_usd, tokens_in=tokens_in, tokens_out=tokens_out, model=model)

        result = {
            "run_id": self.run_id,
            "call_id": call_id,
            "tool": tool,
            "success": success,
            "output": output,
            "error": error,
            "cost_usd": cost_usd,
            "tokens_in": tokens_in,
            "tokens_out": tokens_out,
            "model": model,
        }
        await self._js.publish(
            SUBJECT_TOOLCALL_RESULT,
            json.dumps(result).encode(),
        )

    async def complete_run(
        self,
        status: str = "completed",
        output: str = "",
        error: str = "",
    ) -> None:
        """Signal that the run has finished."""
        await self.stop_heartbeat()
        msg = RunCompleteMessage(
            run_id=self.run_id,
            task_id=self.task_id,
            project_id=self.project_id,
            status=status,
            output=output,
            error=error,
            cost_usd=self._metrics.total_cost,
            step_count=self._metrics.step_count,
            tokens_in=self._metrics.total_tokens_in,
            tokens_out=self._metrics.total_tokens_out,
            model=self._metrics.model,
        )
        await self._js.publish(
            SUBJECT_RUN_COMPLETE,
            msg.model_dump_json().encode(),
        )
        self._log.info(
            "run completed",
            status=status,
            steps=self._metrics.step_count,
            cost=self._metrics.total_cost,
        )

    async def send_output(self, line: str, stream: str = "stdout") -> None:
        """Send a streaming output line to the control plane."""
        payload = {
            "run_id": self.run_id,
            "task_id": self.task_id,
            "line": line,
            "stream": stream,
        }
        await self._js.publish(
            SUBJECT_RUN_OUTPUT,
            json.dumps(payload).encode(),
        )
