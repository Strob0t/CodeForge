"""NATS JetStream consumer for receiving tasks from Go Core."""

from __future__ import annotations

import asyncio
import logging
import os
import signal
from typing import TYPE_CHECKING

import nats

from codeforge.executor import AgentExecutor
from codeforge.llm import LiteLLMClient
from codeforge.models import TaskMessage, TaskResult

if TYPE_CHECKING:
    from nats.aio.client import Client as NATSClient
    from nats.js.client import JetStreamContext

logger = logging.getLogger(__name__)

STREAM_NAME = "CODEFORGE"
SUBJECT_ASSIGNED = "tasks.agent.assigned"
SUBJECT_RESULT = "tasks.result"


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

        logger.info("connected to NATS at %s", self.nats_url)

        # Subscribe to task assignments
        sub = await self._js.subscribe(
            SUBJECT_ASSIGNED,
            stream=STREAM_NAME,
            manual_ack=True,
        )

        logger.info("subscribed to %s", SUBJECT_ASSIGNED)

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
        try:
            task = TaskMessage.model_validate_json(msg.data)
            logger.info("received task %s: %s", task.id, task.title)

            result: TaskResult = await self._executor.execute(task)

            # Publish result back
            if self._js is not None:
                await self._js.publish(SUBJECT_RESULT, result.model_dump_json().encode())

            await msg.ack()
            logger.info("task %s completed with status %s", task.id, result.status)

        except Exception:
            logger.exception("failed to process message")
            await msg.nak()

    async def stop(self) -> None:
        """Gracefully shut down: drain and close."""
        self._running = False
        logger.info("stopping consumer...")

        await self._llm.close()

        if self._nc is not None and self._nc.is_connected:
            await self._nc.drain()

        logger.info("consumer stopped")


async def main() -> None:
    """Entry point for running the consumer."""
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )

    consumer = TaskConsumer(
        nats_url=os.environ.get("NATS_URL", "nats://localhost:4222"),
        litellm_url=os.environ.get("LITELLM_URL", "http://localhost:4000"),
        litellm_key=os.environ.get("LITELLM_MASTER_KEY", ""),
    )

    loop = asyncio.get_running_loop()
    for sig in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(sig, lambda: asyncio.create_task(consumer.stop()))

    await consumer.start()


if __name__ == "__main__":
    asyncio.run(main())
