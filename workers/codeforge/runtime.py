"""Runtime client for the step-by-step execution protocol (Phase 4B).

This module handles the conversational NATS protocol between Python workers
and the Go control plane. Instead of fire-and-forget task execution, each
tool call is individually approved by the control plane's policy engine.
"""

from __future__ import annotations

import asyncio
import contextlib
import json
import uuid
from typing import TYPE_CHECKING

import structlog

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
SUBJECT_RUN_HEARTBEAT = "runs.heartbeat"

RESPONSE_TIMEOUT_SECONDS = 30

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
        self._step_count = 0
        self._total_cost = 0.0
        self._total_tokens_in = 0
        self._total_tokens_out = 0
        self._model = ""
        self._cancelled = False
        self._cancel_sub: object | None = None
        self._heartbeat_task: asyncio.Task[None] | None = None
        self._log = logger.bind(run_id=run_id, task_id=task_id)

    async def start_cancel_listener(self) -> None:
        """Subscribe to cancellation messages for this run."""
        sub = await self._js.subscribe(SUBJECT_RUN_CANCEL)
        self._cancel_sub = sub

        async def _listen() -> None:
            while not self._cancelled:
                try:
                    msg = await sub.next_msg(timeout=1.0)
                    data = json.loads(msg.data)
                    if data.get("run_id") == self.run_id:
                        self._cancelled = True
                        self._log.info("run cancelled by control plane")
                except TimeoutError:
                    continue
                except Exception:
                    break

        self._cancel_task = asyncio.create_task(_listen())

    async def start_heartbeat(self, interval: float = 30.0) -> None:
        """Start periodic heartbeat to the control plane."""

        async def _beat() -> None:
            while not self._cancelled:
                payload = {
                    "run_id": self.run_id,
                    "timestamp": asyncio.get_event_loop().time(),
                }
                try:
                    await self._js.publish(
                        SUBJECT_RUN_HEARTBEAT,
                        json.dumps(payload).encode(),
                    )
                except Exception:
                    self._log.warning("heartbeat publish failed")
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
        return self._step_count

    @property
    def total_cost(self) -> float:
        """Accumulated cost of this run."""
        return self._total_cost

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

        self._log.debug("requesting tool call", tool=tool, call_id=call_id)
        await self._js.publish(
            SUBJECT_TOOLCALL_REQUEST,
            json.dumps(request).encode(),
        )

        # Wait for response (subscribe and filter by call_id)
        sub = await self._js.subscribe(SUBJECT_TOOLCALL_RESPONSE)
        try:
            deadline = asyncio.get_event_loop().time() + RESPONSE_TIMEOUT_SECONDS
            while True:
                remaining = deadline - asyncio.get_event_loop().time()
                if remaining <= 0:
                    self._log.warning("tool call response timed out", call_id=call_id)
                    return ToolCallDecision(
                        call_id=call_id,
                        decision="deny",
                        reason="response timeout",
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
                    return ToolCallDecision(
                        call_id=call_id,
                        decision=data.get("decision", "deny"),
                        reason=data.get("reason", ""),
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
        self._step_count += 1
        self._total_cost += cost_usd
        self._total_tokens_in += tokens_in
        self._total_tokens_out += tokens_out
        if model:
            self._model = model

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
            cost_usd=self._total_cost,
            step_count=self._step_count,
            tokens_in=self._total_tokens_in,
            tokens_out=self._total_tokens_out,
            model=self._model,
        )
        await self._js.publish(
            SUBJECT_RUN_COMPLETE,
            msg.model_dump_json().encode(),
        )
        self._log.info(
            "run completed",
            status=status,
            steps=self._step_count,
            cost=self._total_cost,
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
