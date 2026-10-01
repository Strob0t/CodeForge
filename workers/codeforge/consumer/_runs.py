"""Run start handler mixin."""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._cancel_registry import run_key
from codeforge.consumer._delivery import stream_sequence
from codeforge.consumer._subjects import SUBJECT_RUN_COMPLETE, SUBJECT_TASK_CANCEL
from codeforge.models import RunCompleteMessage, RunStartMessage, TaskMessage
from codeforge.nats_publish import publish_with_retry
from codeforge.runtime import RuntimeClient, heartbeat_interval

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()

# Tools always run as local processes of this worker; there is no container
# isolation yet (KI-13), so only mount runs may execute. "" is the legacy mount default.
_EXECUTABLE_EXEC_MODES = frozenset({"", "mount"})


class RunHandlerMixin:
    """Handles runs.start messages — runtime protocol execution."""

    async def _handle_run_start(self, msg: nats.aio.msg.Msg) -> None:
        """Process a run start message: parse, create RuntimeClient, execute with runtime.

        Acked on accept (at-most-once): a run changes the workspace and must not
        be executed a second time by another worker. If this worker dies, the Go
        Core's run timeout fails the run.
        """
        # A run stopped while its start waited in NATS is not executed (KI-65
        # follow-up); its cancelled completion ends the Go run. Only a cancel
        # of the run skips it: a tasks.cancel stops a backend task and never
        # ends a run on the task (S2-G fix, 2).
        start = stream_sequence(msg)
        await self._handle_request(
            msg=msg,
            request_model=RunStartMessage,
            dedup_key=lambda r: f"run-{r.run_id}",
            handler=self._do_run_start,
            result_subject=None,
            log_context=lambda r: {"run_id": r.run_id, "task_id": r.task_id},
            ack_on_accept=True,
            cancelled=lambda r: start is not None and self._cancels.cancelled(run_key(r.run_id), start),
            report_skipped=self._report_skipped_run,
        )

    async def _report_skipped_run(self, run_msg: RunStartMessage, log: structlog.BoundLogger) -> None:
        """Publish the cancelled completion of a run whose start is skipped, so the Go Core ends it."""
        if self._js is None:
            err_msg = "JetStream not available for the skipped run's completion"
            raise RuntimeError(err_msg)
        completion = RunCompleteMessage(
            run_id=run_msg.run_id,
            task_id=run_msg.task_id,
            tenant_id=run_msg.tenant_id,
            project_id=run_msg.project_id,
            status="cancelled",
            error="run cancelled before a worker started it",
        )
        await publish_with_retry(self._js, SUBJECT_RUN_COMPLETE, completion.model_dump_json().encode())
        log.info("skipped run reported as cancelled")

    async def _do_run_start(self, run_msg: RunStartMessage, log: structlog.BoundLogger) -> None:
        """Business logic for run start execution."""
        log.info("received run start", prompt=run_msg.prompt[:80])

        if self._js is None:
            log.error("JetStream not available")
            err_msg = "JetStream not available for run start"
            raise RuntimeError(err_msg)

        runtime = RuntimeClient(
            js=self._js,
            run_id=run_msg.run_id,
            task_id=run_msg.task_id,
            project_id=run_msg.project_id,
            termination=run_msg.termination,
            tenant_id=run_msg.tenant_id,
            mode_id=run_msg.mode.id,
            approval_timeout_seconds=run_msg.approval_timeout_seconds,
        )

        # Go rejects these runs at start; this catches run starts that bypass it
        # (messages queued before an upgrade) and fails them visibly.
        if run_msg.exec_mode not in _EXECUTABLE_EXEC_MODES:
            error = (
                f"{run_msg.exec_mode!r} execution mode is not available yet: tools would run without isolation (KI-13)"
            )
            log.error("run rejected", exec_mode=run_msg.exec_mode, error=error)
            await runtime.complete_run(status="failed", error=error)
            return

        with self._in_flight.track(
            f"run {run_msg.run_id}", lambda reason: self._report_run_failure(runtime, reason, log)
        ):
            try:
                await runtime.start_cancel_listener(extra_subjects=[SUBJECT_TASK_CANCEL])
                # Agent-loop runs take long; runtime.close() stops the heartbeat.
                await runtime.start_heartbeat(heartbeat_interval(run_msg.heartbeat_seconds))
                task = self._build_run_task(run_msg, log)
                await self._executor.execute_with_runtime(
                    task, runtime, mode=run_msg.mode, mcp_servers=run_msg.mcp_servers
                )
            except Exception as exc:
                # The run was acked on accept and is never redelivered: end it as
                # failed instead of leaving it running until the Go run timeout.
                log.exception("run failed", error=str(exc))
                await self._report_run_failure(runtime, str(exc), log)
            finally:
                await runtime.close()
        log.info(
            "run processing complete",
            mode_id=run_msg.mode.id if run_msg.mode else None,
        )

    @staticmethod
    async def _report_run_failure(runtime: RuntimeClient, error: str, log: structlog.BoundLogger) -> None:
        """Publish a failed completion unless the run already reported its outcome."""
        if runtime.completed:
            return
        try:
            await runtime.complete_run(status="failed", error=error)
        except Exception as exc:
            log.exception("failed to report run failure", error=str(exc))

    @staticmethod
    def _build_run_task(run_msg: RunStartMessage, log: structlog.BoundLogger) -> TaskMessage:
        """Build the executor task, with pre-packed context and microagent prompts in the prompt."""
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
            log.info(
                "microagent prompts injected",
                count=len(run_msg.microagent_prompts),
            )

        return TaskMessage(
            id=run_msg.task_id,
            project_id=run_msg.project_id,
            tenant_id=run_msg.tenant_id,
            agent_id=run_msg.agent_id,
            title=run_msg.prompt[:80],
            prompt=enriched_prompt,
            backend=run_msg.backend,
            workspace_path=run_msg.workspace_path,
            config=run_msg.config,
        )
