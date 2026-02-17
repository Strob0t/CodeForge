"""NATS JetStream consumer for receiving tasks from Go Core."""

from __future__ import annotations

import asyncio
import signal
from typing import TYPE_CHECKING

import nats
import structlog

from codeforge.config import WorkerSettings
from codeforge.executor import AgentExecutor
from codeforge.llm import LiteLLMClient
from codeforge.logger import setup_logging
from codeforge.models import QualityGateRequest, QualityGateResult, RunStartMessage, TaskMessage, TaskResult
from codeforge.qualitygate import QualityGateExecutor
from codeforge.runtime import RuntimeClient

if TYPE_CHECKING:
    from nats.aio.client import Client as NATSClient
    from nats.js.client import JetStreamContext

STREAM_NAME = "CODEFORGE"
SUBJECT_AGENT = "tasks.agent.*"
SUBJECT_RESULT = "tasks.result"
SUBJECT_OUTPUT = "tasks.output"
SUBJECT_RUN_START = "runs.start"
SUBJECT_QG_REQUEST = "runs.qualitygate.request"
SUBJECT_QG_RESULT = "runs.qualitygate.result"
HEADER_REQUEST_ID = "X-Request-ID"
HEADER_RETRY_COUNT = "Retry-Count"
MAX_RETRIES = 3

logger = structlog.get_logger()


class TaskConsumer:
    """Consumes task messages from NATS JetStream and dispatches them to the executor."""

    def __init__(
        self,
        nats_url: str = "nats://localhost:4222",
        litellm_url: str = "http://localhost:4000",
        litellm_key: str = "",
    ) -> None:
        self.nats_url = nats_url
        self._nc: NATSClient | None = None
        self._js: JetStreamContext | None = None
        self._running = False
        self._llm = LiteLLMClient(base_url=litellm_url, api_key=litellm_key)
        self._executor = AgentExecutor(llm=self._llm)
        self._gate_executor = QualityGateExecutor()

    async def start(self) -> None:
        """Connect to NATS and subscribe to task and run subjects."""
        self._nc = await nats.connect(self.nats_url)
        self._js = self._nc.jetstream()
        self._running = True

        logger.info("connected to NATS", url=self.nats_url)

        # Subscribe to agent task dispatches (tasks.agent.aider, tasks.agent.openhands, etc.)
        task_sub = await self._js.subscribe(
            SUBJECT_AGENT,
            stream=STREAM_NAME,
            manual_ack=True,
        )
        logger.info("subscribed", subject=SUBJECT_AGENT)

        # Subscribe to run start messages (step-by-step protocol)
        run_sub = await self._js.subscribe(
            SUBJECT_RUN_START,
            stream=STREAM_NAME,
            manual_ack=True,
        )
        logger.info("subscribed", subject=SUBJECT_RUN_START)

        # Subscribe to quality gate requests
        qg_sub = await self._js.subscribe(
            SUBJECT_QG_REQUEST,
            stream=STREAM_NAME,
            manual_ack=True,
        )
        logger.info("subscribed", subject=SUBJECT_QG_REQUEST)

        # Process all subscriptions concurrently
        await asyncio.gather(
            self._process_task_messages(task_sub),
            self._process_run_messages(run_sub),
            self._process_quality_gate_messages(qg_sub),
        )

    async def _process_task_messages(self, sub: object) -> None:
        """Message processing loop for legacy fire-and-forget tasks."""
        while self._running:
            try:
                msg = await asyncio.wait_for(sub.next_msg(), timeout=1.0)  # type: ignore[union-attr]
            except TimeoutError:
                continue
            except Exception:
                if self._running:
                    logger.exception("error receiving task message")
                break

            await self._handle_message(msg)

    async def _process_run_messages(self, sub: object) -> None:
        """Message processing loop for step-by-step run protocol."""
        while self._running:
            try:
                msg = await asyncio.wait_for(sub.next_msg(), timeout=1.0)  # type: ignore[union-attr]
            except TimeoutError:
                continue
            except Exception:
                if self._running:
                    logger.exception("error receiving run message")
                break

            await self._handle_run_start(msg)

    async def _handle_message(self, msg: nats.aio.msg.Msg) -> None:
        """Process a single task message: parse, execute, ack/nack."""
        # Extract request ID from NATS headers for log correlation
        request_id = ""
        if msg.headers and HEADER_REQUEST_ID in msg.headers:
            request_id = msg.headers[HEADER_REQUEST_ID]

        log = logger.bind(request_id=request_id) if request_id else logger

        try:
            task = TaskMessage.model_validate_json(msg.data)
            log = log.bind(task_id=task.id)
            log.info("received task", title=task.title)

            # Send running status update
            await self._publish_output(task.id, f"Starting task: {task.title}", "stdout", request_id)

            result: TaskResult = await self._executor.execute(task)

            # Publish result back
            if self._js is not None:
                await self._js.publish(SUBJECT_RESULT, result.model_dump_json().encode())

            await msg.ack()
            log.info("task completed", status=result.status)

        except Exception:
            retries = self._retry_count(msg)
            log.exception("failed to process message", retry=retries)

            if retries >= MAX_RETRIES:
                log.warning("max retries reached, moving to DLQ", retry=retries)
                await self._move_to_dlq(msg)
            else:
                await msg.nak()

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

            # Convert to TaskMessage for executor compatibility
            task = TaskMessage(
                id=run_msg.task_id,
                project_id=run_msg.project_id,
                title=run_msg.prompt[:80],
                prompt=enriched_prompt,
                config=run_msg.config,
            )

            await self._executor.execute_with_runtime(task, runtime)
            await msg.ack()
            log.info("run processing complete")

        except Exception:
            logger.exception("failed to process run start message")
            await msg.nak()

    async def _process_quality_gate_messages(self, sub: object) -> None:
        """Message processing loop for quality gate requests."""
        while self._running:
            try:
                msg = await asyncio.wait_for(sub.next_msg(), timeout=1.0)  # type: ignore[union-attr]
            except TimeoutError:
                continue
            except Exception:
                if self._running:
                    logger.exception("error receiving quality gate message")
                break

            await self._handle_quality_gate(msg)

    async def _handle_quality_gate(self, msg: nats.aio.msg.Msg) -> None:
        """Process a quality gate request: run tests/lint and publish result."""
        try:
            request = QualityGateRequest.model_validate_json(msg.data)
            log = logger.bind(run_id=request.run_id)
            log.info("received quality gate request")

            result: QualityGateResult = await self._gate_executor.execute(request)

            if self._js is not None:
                await self._js.publish(
                    SUBJECT_QG_RESULT,
                    result.model_dump_json().encode(),
                )

            await msg.ack()
            log.info("quality gate completed", tests_passed=result.tests_passed, lint_passed=result.lint_passed)

        except Exception:
            logger.exception("failed to process quality gate request")
            await msg.nak()

    @staticmethod
    def _retry_count(msg: nats.aio.msg.Msg) -> int:
        """Extract the Retry-Count header value, defaulting to 0."""
        if msg.headers and HEADER_RETRY_COUNT in msg.headers:
            try:
                return int(msg.headers[HEADER_RETRY_COUNT])
            except (ValueError, TypeError):
                return 0
        return 0

    async def _move_to_dlq(self, msg: nats.aio.msg.Msg) -> None:
        """Publish message to DLQ subject and ack the original."""
        if self._js is None:
            return
        dlq_subject = msg.subject + ".dlq"
        headers = dict(msg.headers) if msg.headers else {}
        try:
            await self._js.publish(dlq_subject, msg.data, headers=headers or None)
            logger.warning("message moved to DLQ", dlq_subject=dlq_subject)
        except Exception:
            logger.exception("failed to publish to DLQ", dlq_subject=dlq_subject)
        await msg.ack()

    async def _publish_output(self, task_id: str, line: str, stream: str = "stdout", request_id: str = "") -> None:
        """Publish a streaming output line for a task."""
        if self._js is None:
            return
        import json

        payload = json.dumps({"task_id": task_id, "line": line, "stream": stream})

        headers = {}
        if request_id:
            headers[HEADER_REQUEST_ID] = request_id

        await self._js.publish(SUBJECT_OUTPUT, payload.encode(), headers=headers if headers else None)

    async def stop(self) -> None:
        """Gracefully shut down: drain with timeout and close."""
        self._running = False
        logger.info("stopping consumer")

        await self._llm.close()

        if self._nc is not None and self._nc.is_connected:
            try:
                await asyncio.wait_for(self._nc.drain(), timeout=10.0)
            except TimeoutError:
                logger.warning("NATS drain timed out after 10s, closing connection")
                await self._nc.close()

        logger.info("consumer stopped")


async def main() -> None:
    """Entry point for running the consumer."""
    settings = WorkerSettings()
    setup_logging(service=settings.log_service, level=settings.log_level)

    consumer = TaskConsumer(
        nats_url=settings.nats_url,
        litellm_url=settings.litellm_url,
        litellm_key=settings.litellm_api_key,
    )

    loop = asyncio.get_running_loop()
    for sig in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(sig, lambda: asyncio.create_task(consumer.stop()))

    await consumer.start()


if __name__ == "__main__":
    asyncio.run(main())
