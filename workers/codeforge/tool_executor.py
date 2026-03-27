"""Extracted tool execution logic from AgentLoopExecutor.

Handles single tool call execution with policy checks, error handling,
OTEL tracing, and trajectory event publishing.
"""

from __future__ import annotations

import logging
import time
from datetime import UTC, datetime
from typing import TYPE_CHECKING

from opentelemetry import trace
from opentelemetry.trace import StatusCode

from codeforge.json_utils import safe_json_loads
from codeforge.loop_helpers import (
    ToolErrorTracker,
    build_correction_hint,
    build_tool_result_message,
    build_tool_result_text,
)
from codeforge.tracing import metrics as otel_metrics

if TYPE_CHECKING:
    from codeforge.agent_loop import _LoopState
    from codeforge.llm import ToolCallPart
    from codeforge.models import ConversationMessagePayload
    from codeforge.quality_tracking import IterationQualityTracker
    from codeforge.runtime import RuntimeClient
    from codeforge.tools import ToolRegistry

logger = logging.getLogger(__name__)


def _payload_to_dict(msg: ConversationMessagePayload) -> dict[str, object]:
    """Convert a ConversationMessagePayload to a dict for the messages list."""
    from codeforge.loop_helpers import payload_to_dict

    return payload_to_dict(msg)


class ToolExecutor:
    """Executes individual tool calls with policy, tracing, and error handling.

    Extracted from AgentLoopExecutor to reduce its size and isolate
    tool execution concerns from LLM iteration logic.
    """

    def __init__(
        self,
        registry: ToolRegistry,
        runtime: RuntimeClient,
        workspace_path: str,
    ) -> None:
        self._registry = registry
        self._runtime = runtime
        self._workspace = workspace_path

    async def execute(
        self,
        tc: ToolCallPart,
        messages: list[dict[str, object]],
        state: _LoopState,
        *,
        quality_tracker: IterationQualityTracker | None = None,
        error_tracker: ToolErrorTracker | None = None,
    ) -> None:
        """Execute a single tool call with policy check and error handling."""
        arguments: dict = safe_json_loads(tc.arguments, {}) if tc.arguments else {}
        decision = await self._runtime.request_tool_call(
            tool=tc.name, command=tc.arguments[:200] if tc.arguments else ""
        )

        if decision.decision != "allow":
            result_text = f"Permission denied: {decision.reason}"
            self.append_result(tc, result_text, messages, state)
            await self._runtime.report_tool_result(
                call_id=decision.call_id, tool=tc.name, success=False, error=result_text
            )
            await self._publish_trajectory_event(tc, result_text, False, 0, state.step_count)
            return

        tracer = trace.get_tracer("codeforge")
        tool_start = time.monotonic()
        with tracer.start_as_current_span(f"tool.execute:{tc.name}", attributes={"tool.name": tc.name}) as tool_span:
            try:
                result = await self._registry.execute(tc.name, arguments, self._workspace)
            except Exception as exc:
                tool_span.set_status(StatusCode.ERROR, str(exc))
                tool_span.record_exception(exc)
                logger.exception("tool %s execution error", tc.name)
                result_text = f"Error executing {tc.name}: {exc}"
                correction = build_correction_hint(tc.name, str(exc))
                if correction:
                    result_text = f"{result_text}\n\n{correction}"
                self.append_result(tc, result_text, messages, state)
                await self._runtime.report_tool_result(
                    call_id=decision.call_id, tool=tc.name, success=False, error=result_text
                )
                elapsed_ms = (time.monotonic() - tool_start) * 1000
                await self._publish_trajectory_event(tc, result_text, False, elapsed_ms, state.step_count)
                return

            result_text = build_tool_result_text(result, tc.name, error_tracker)
            if not result.success:
                tool_span.set_status(StatusCode.ERROR, result.error or "Tool returned an error")
            self.append_result(tc, result_text, messages, state)
            await self._runtime.report_tool_result(
                call_id=decision.call_id,
                tool=tc.name,
                success=result.success,
                output=result.output[:500] if result.output else "",
                error=result.error,
                diff=result.diff,
            )
            elapsed_ms = (time.monotonic() - tool_start) * 1000
            otel_metrics.tool_duration.record(elapsed_ms / 1000)
            await self._publish_trajectory_event(tc, result_text, result.success, elapsed_ms, state.step_count)

            qt = quality_tracker or (state.quality_tracker if state else None)
            if qt is not None:
                qt.record(tool_success=result.success, output_length=len(result_text))

            if result.success and tc.name in ("edit_file", "write_file"):
                try:
                    await self._runtime.publish_trajectory_event(
                        {
                            "event_type": "agent.action_suggestion",
                            "label": "Run tests",
                            "action": "send_message",
                            "value": "Run the test suite to verify the changes",
                        }
                    )
                except (ConnectionError, TimeoutError, OSError) as exc:
                    logger.debug("failed to publish action_suggestion event: %s", exc)

    @staticmethod
    def append_result(
        tc: ToolCallPart,
        content: str,
        messages: list[dict[str, object]],
        state: _LoopState,
    ) -> None:
        """Build and append a tool result message to state and messages."""
        msg = build_tool_result_message(tc, content)
        state.tool_messages.append(msg)
        messages.append(_payload_to_dict(msg))

    async def _publish_trajectory_event(
        self,
        tc: ToolCallPart,
        result_text: str,
        success: bool,
        elapsed_ms: float,
        step: int,
    ) -> None:
        """Publish a trajectory event for a tool call (fire-and-forget)."""
        try:
            await self._runtime.publish_trajectory_event(
                {
                    "event_type": "agent.tool_called",
                    "tool_name": tc.name,
                    "input": (tc.arguments or "")[:500],
                    "output": result_text[:500],
                    "success": success,
                    "duration_ms": round(elapsed_ms, 1) if elapsed_ms else 0,
                    "step": step,
                    "timestamp": datetime.now(UTC).isoformat(),
                }
            )
        except (ConnectionError, TimeoutError, OSError) as exc:
            logger.debug("failed to publish tool_called trajectory event: %s", exc)
