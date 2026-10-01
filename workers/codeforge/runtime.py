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
from nats.js.api import AckPolicy, ConsumerConfig, DeliverPolicy

from codeforge.constants import APPROVAL_RESPONSE_MARGIN_SECONDS, DEFAULT_APPROVAL_TIMEOUT_SECONDS
from codeforge.metrics import ExecutionMetrics
from codeforge.models import RunCompleteMessage, ToolCallDecision
from codeforge.nats_publish import publish_with_retry
from codeforge.nats_subjects import (
    SUBJECT_AGENT_OUTPUT,
    SUBJECT_RUN_CANCEL,
    SUBJECT_RUN_COMPLETE,
    SUBJECT_RUN_HEARTBEAT,
    SUBJECT_RUN_OUTPUT,
    SUBJECT_TOOLCALL_REQUEST,
    SUBJECT_TOOLCALL_RESPONSE,
    SUBJECT_TOOLCALL_RESULT,
    SUBJECT_TRAJECTORY_EVENT,
)

if TYPE_CHECKING:
    from collections.abc import AsyncIterator, Awaitable, Callable

    from nats.js.client import JetStreamContext

    from codeforge.models import TerminationConfig

# Maximum length (characters) of the arguments preview sent with a tool call.
ARGUMENTS_PREVIEW_MAX_CHARS = 1000

# How often a worker reports work it executes as alive (the Go Core's
# runtime.heartbeat_interval). The Go Core ends accepted work whose heartbeats
# stop for runtime.heartbeat_timeout (KI-65).
HEARTBEAT_INTERVAL_SECONDS = 30.0


def heartbeat_interval(heartbeat_seconds: int) -> float:
    """Seconds between two heartbeats of work whose start message sent *heartbeat_seconds* (0 = default)."""
    return float(heartbeat_seconds) if heartbeat_seconds > 0 else HEARTBEAT_INTERVAL_SECONDS


logger = structlog.get_logger()


async def send_heartbeats(
    js: JetStreamContext,
    subject: str,
    payload: dict[str, str],
    interval: float,
    until: Callable[[], bool] = lambda: False,
    on_beat: Callable[[], Awaitable[None]] | None = None,
) -> None:
    """Publish *payload* with the current time on *subject* every *interval* seconds.

    Runs until cancelled or until *until()* is true; *on_beat* runs with
    every heartbeat (e.g. an in-progress ack of the message being handled).
    A failed publish or *on_beat* is logged; the next heartbeat follows.
    """
    while not until():
        beat = {**payload, "timestamp": datetime.now(UTC).isoformat()}
        try:
            await js.publish(subject, json.dumps(beat).encode())
        except Exception as exc:
            logger.warning("heartbeat publish failed", subject=subject, error=str(exc), **payload)
        if on_beat is not None:
            try:
                await on_beat()
            except Exception as exc:
                logger.warning("heartbeat callback failed", subject=subject, error=str(exc), **payload)
        await asyncio.sleep(interval)


@contextlib.asynccontextmanager
async def heartbeats(
    js: JetStreamContext,
    subject: str,
    payload: dict[str, str],
    interval: float,
    on_beat: Callable[[], Awaitable[None]] | None = None,
) -> AsyncIterator[None]:
    """Send heartbeats (see send_heartbeats) while the block runs."""
    task = asyncio.create_task(
        send_heartbeats(js, subject, payload, interval, on_beat=on_beat), name=f"heartbeat {subject}"
    )
    try:
        yield
    finally:
        task.cancel()
        # asyncio.wait: the heartbeat's own cancellation does not end the
        # caller, a cancellation of the caller still does.
        await asyncio.wait({task})


def notification_consumer(after: int | None = None) -> ConsumerConfig:
    """Settings of the ephemeral consumers a run or task listens on (cancel messages, tool-call responses).

    They see new messages only - with *after*, every message published after
    that stream sequence (e.g. the work's own start message), so none
    published while the listener subscribes is missed - and are never acked:
    with explicit acks JetStream would redeliver every message after the ack
    wait and stop delivering once MaxAckPending messages were outstanding.
    """
    if after is not None:
        return ConsumerConfig(
            deliver_policy=DeliverPolicy.BY_START_SEQUENCE, opt_start_seq=after + 1, ack_policy=AckPolicy.NONE
        )
    return ConsumerConfig(deliver_policy=DeliverPolicy.NEW, ack_policy=AckPolicy.NONE)


def cancel_ids(data: object) -> tuple[str, str] | None:
    """(run_id, task_id) of a cancel message, "" for an absent ID; None if the message is malformed."""
    try:
        payload = json.loads(data)  # type: ignore[arg-type]
    except (ValueError, TypeError) as exc:  # invalid JSON or UTF-8, or no bytes at all
        logger.warning("ignoring malformed cancel message", error=str(exc))
        return None
    if not isinstance(payload, dict):
        logger.warning("ignoring malformed cancel message", payload_type=type(payload).__name__)
        return None
    run_id = payload.get("run_id")
    task_id = payload.get("task_id")
    return (run_id if isinstance(run_id, str) else "", task_id if isinstance(task_id, str) else "")


async def listen_for_cancel(
    sub: JetStreamContext.PushSubscription,
    matches: Callable[[str, str], bool],
    on_cancel: Callable[[], None],
    *,
    until: Callable[[], bool],
) -> None:
    """Call *on_cancel* once a cancel message on *sub* matches(run_id, task_id).

    Cancel subjects are shared by every run and task, so a malformed message is
    skipped, not fatal. Returns after the cancel, once *until()* is true, or
    when the subscription is closed.
    """
    while not until():
        try:
            msg = await sub.next_msg(timeout=1.0)
        except TimeoutError:
            continue
        except Exception as exc:
            logger.debug("cancel listener stopped", error=str(exc))
            return
        ids = cancel_ids(msg.data)
        if ids is not None and matches(*ids):
            on_cancel()
            return
        # Let the run go on even if unmatched messages arrive back to back.
        await asyncio.sleep(0)


def policy_response_timeout(approval_timeout_seconds: float) -> float:
    """How long to wait for the Go Core's decision on a tool call.

    A call the policy resolves to "ask" is answered only once a human decided
    or Go's approval timeout expired, so the wait outlasts that timeout
    (KI-21). A timeout <= 0 (none sent) means the Go default.
    """
    approval = approval_timeout_seconds if approval_timeout_seconds > 0 else DEFAULT_APPROVAL_TIMEOUT_SECONDS
    return approval + APPROVAL_RESPONSE_MARGIN_SECONDS


def arguments_preview(arguments: dict[str, object]) -> str:
    """Render tool call arguments as truncated JSON for the human approver.

    Display only: the Go policy layer never evaluates the preview.
    """
    text = json.dumps(arguments, ensure_ascii=False, sort_keys=True, default=str)
    if len(text) <= ARGUMENTS_PREVIEW_MAX_CHARS:
        return text
    return text[: ARGUMENTS_PREVIEW_MAX_CHARS - 3] + "..."


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
        tenant_id: str = "",
        mode_id: str = "",
        turn_id: str = "",
        approval_timeout_seconds: float = 0,
    ) -> None:
        self._js = js
        self.run_id = run_id
        self.task_id = task_id
        self.project_id = project_id
        # Echoed on every message to the control plane: the Go core scopes store
        # writes and WebSocket events to it and drops events without a tenant.
        self.tenant_id = tenant_id
        self.termination = termination
        # Agent mode the run was started with; the Go policy layer enforces
        # its tool lists on every tool call.
        self.mode_id = mode_id
        # Turn of a conversation run: conversation runs reuse the conversation
        # ID as run ID, so Go tells the calls of a stopped run from the calls
        # of the next run of the same conversation by it.
        self.turn_id = turn_id
        # Go's HITL approval timeout, sent with the run start: a decision is
        # awaited longer than that.
        self.policy_wait_seconds = policy_response_timeout(approval_timeout_seconds)
        self._metrics = ExecutionMetrics()
        self._cancelled = False
        self._completed = False
        self._cancel_subs: list[JetStreamContext.PushSubscription] = []
        self._cancel_tasks: list[asyncio.Task[None]] = []
        self._heartbeat_task: asyncio.Task[None] | None = None
        self._log = logger.bind(run_id=run_id, task_id=task_id)

    async def start_cancel_listener(self, extra_subjects: list[str] | None = None) -> None:
        """Subscribe to cancellation messages for this run.

        Listens on the default runs.cancel subject plus any extra subjects
        (e.g. conversation.run.cancel for conversation runs). The subscriptions
        belong to this run: ``close()`` must be called when the run ends.
        """
        subjects = [SUBJECT_RUN_CANCEL] + (extra_subjects or [])
        for subject in subjects:
            sub = await self._js.subscribe(subject, config=notification_consumer())
            self._cancel_subs.append(sub)
            listener = listen_for_cancel(sub, self._names_this_run, self._mark_cancelled, until=self._is_cancelled)
            self._cancel_tasks.append(asyncio.create_task(listener))

    def _names_this_run(self, run_id: str, task_id: str) -> bool:
        # Empty IDs never match: a run without a task ID is not cancelled by a
        # cancel for "no task".
        return (bool(run_id) and run_id == self.run_id) or (bool(task_id) and task_id == self.task_id)

    def _mark_cancelled(self) -> None:
        self._cancelled = True
        self._log.info("run cancelled by control plane")

    def _is_cancelled(self) -> bool:
        return self._cancelled

    async def stop_cancel_listener(self) -> None:
        """Stop the listener tasks and unsubscribe this run's cancel subscriptions."""
        tasks, self._cancel_tasks = self._cancel_tasks, []
        for task in tasks:
            task.cancel()
        for task in tasks:
            with contextlib.suppress(asyncio.CancelledError):
                await task
        subs, self._cancel_subs = self._cancel_subs, []
        for sub in subs:
            try:
                await sub.unsubscribe()
            except Exception as exc:
                self._log.warning("cancel listener unsubscribe failed", error=str(exc))

    async def close(self) -> None:
        """Release everything this run holds on NATS: heartbeat and cancel listeners."""
        await self.stop_heartbeat()
        await self.stop_cancel_listener()

    async def start_heartbeat(self, interval: float = HEARTBEAT_INTERVAL_SECONDS) -> None:
        """Start the periodic heartbeat to the control plane (stops with close() or a cancel).

        It names the run's tenant, and a conversation run's turn: the Go Core
        records it for the conversation's active turn only (KI-65).
        """
        payload = {"run_id": self.run_id, "tenant_id": self.tenant_id}
        if self.turn_id:
            payload["turn_id"] = self.turn_id
        self._heartbeat_task = asyncio.create_task(
            send_heartbeats(self._js, SUBJECT_RUN_HEARTBEAT, payload, interval, until=self._is_cancelled)
        )

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
    def completed(self) -> bool:
        """Whether this run's completion has been published to the control plane."""
        return self._completed

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
        arguments_preview: str = "",
    ) -> ToolCallDecision:
        """Request permission from the control plane to execute a tool call.

        Publishes a request to NATS, then waits for the response.
        Returns the decision (allow/deny/ask). ``arguments_preview`` is shown
        to a human approver only; the policy evaluates tool, command and path.
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
            "tenant_id": self.tenant_id,
            "tool": tool,
            "command": command,
            "path": path,
            "mode_id": self.mode_id,
            "arguments_preview": arguments_preview,
            "turn_id": self.turn_id,
        }

        start_time = time.monotonic()
        self._log.debug("requesting tool call", tool=tool, call_id=call_id)

        # Subscribe BEFORE publishing to avoid a race condition where Go
        # responds before the subscription is established. Only new messages
        # matter: the response to the request we are about to publish.
        sub = await self._js.subscribe(SUBJECT_TOOLCALL_RESPONSE, config=notification_consumer())
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

            deadline = asyncio.get_event_loop().time() + self.policy_wait_seconds
            while True:
                remaining = deadline - asyncio.get_event_loop().time()
                if remaining <= 0:
                    elapsed_ms = (time.monotonic() - start_time) * 1000
                    self._log.warning(
                        "NATS response timeout waiting for policy decision from Go control plane",
                        call_id=call_id,
                        tool=tool,
                        elapsed_ms=round(elapsed_ms, 1),
                        timeout_seconds=self.policy_wait_seconds,
                    )
                    return ToolCallDecision(
                        call_id=call_id,
                        decision="deny",
                        reason=f"NATS response timeout after {self.policy_wait_seconds:g}s "
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
        diff: dict[str, object] | None = None,
    ) -> None:
        """Report the outcome of an executed tool call back to the control plane."""
        self._metrics.step_count += 1
        self._metrics.record(cost=cost_usd, tokens_in=tokens_in, tokens_out=tokens_out, model=model)

        result: dict[str, object] = {
            "run_id": self.run_id,
            "call_id": call_id,
            "tenant_id": self.tenant_id,
            "tool": tool,
            "success": success,
            "output": output,
            "error": error,
            "cost_usd": cost_usd,
            "tokens_in": tokens_in,
            "tokens_out": tokens_out,
            "model": model,
        }
        if diff is not None:
            result["diff"] = diff
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
            tenant_id=self.tenant_id,
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
        # The run was acked on accept and is never redelivered: its completion
        # must not be lost to a transient publish failure (ADR-016).
        await publish_with_retry(self._js, SUBJECT_RUN_COMPLETE, msg.model_dump_json().encode())
        self._completed = True
        self._log.info(
            "run completed",
            status=status,
            steps=self._metrics.step_count,
            cost=self._metrics.total_cost,
        )

    async def send_output(self, line: str, stream: str = "stdout") -> None:
        """Send a streaming output line to the control plane.

        Also publishes to ``agents.output`` so the Go AgentService can
        broadcast the line to WebSocket clients.
        """
        payload = {
            "run_id": self.run_id,
            "task_id": self.task_id,
            "tenant_id": self.tenant_id,
            "line": line,
            "stream": stream,
        }
        await self._js.publish(
            SUBJECT_RUN_OUTPUT,
            json.dumps(payload).encode(),
        )
        # Mirror to agents.output for WS broadcast (best-effort).
        await self.publish_agent_output(line, stream)

    async def publish_trajectory_event(self, event: dict[str, object]) -> None:
        """Publish a trajectory event for recording and UI display."""
        event["run_id"] = self.run_id
        event["project_id"] = self.project_id
        event["tenant_id"] = self.tenant_id
        try:
            await self._js.publish(
                SUBJECT_TRAJECTORY_EVENT,
                json.dumps(event, default=str).encode(),
            )
        except Exception as exc:
            self._log.debug("trajectory event publish failed", error=str(exc))

    async def publish_agent_output(self, line: str, stream: str = "stdout") -> None:
        """Publish a line of agent output for Go WS broadcast.

        Unlike ``send_output`` (which publishes to ``runs.output`` for the run
        protocol), this method publishes to ``agents.output`` so the Go
        ``AgentService.StartAgentOutputSubscriber`` can broadcast the line to
        WebSocket clients.
        """
        payload = {
            "task_id": self.task_id,
            "tenant_id": self.tenant_id,
            "line": line,
            "stream": stream,
            "timestamp": datetime.now(UTC).isoformat(),
        }
        try:
            await self._js.publish(
                SUBJECT_AGENT_OUTPUT,
                json.dumps(payload).encode(),
            )
        except Exception as exc:
            self._log.debug("agent output publish failed", error=str(exc))
