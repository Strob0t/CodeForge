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
from codeforge.models import TaskMessage, TaskResult

if TYPE_CHECKING:
    from nats.aio.client import Client as NATSClient
    from nats.js.client import JetStreamContext

STREAM_NAME = "CODEFORGE"
SUBJECT_AGENT = "tasks.agent.*"
SUBJECT_RESULT = "tasks.result"
SUBJECT_OUTPUT = "tasks.output"
HEADER_REQUEST_ID = "X-Request-ID"

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

    async def start(self) -> None:
        """Connect to NATS and subscribe to the task assignment subject."""
        self._nc = await nats.connect(self.nats_url)
        self._js = self._nc.jetstream()
        self._running = True

        logger.info("connected to NATS", url=self.nats_url)

        # Subscribe to agent task dispatches (tasks.agent.aider, tasks.agent.openhands, etc.)
        sub = await self._js.subscribe(
            SUBJECT_AGENT,
            stream=STREAM_NAME,
            manual_ack=True,
        )

        logger.info("subscribed", subject=SUBJECT_AGENT)

        # Message processing loop
        while self._running:
            try:
                msg = await asyncio.wait_for(sub.next_msg(), timeout=1.0)
            except TimeoutError:
                continue
            except Exception:
                if self._running:
                    logger.exception("error receiving message")
                break

            await self._handle_message(msg)

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
            log.exception("failed to process message")
            await msg.nak()

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
