"""NATS JetStream consumer for receiving tasks from Go Core."""

import asyncio
import logging
import signal

logger = logging.getLogger(__name__)


class TaskConsumer:
    """Consumes task messages from NATS JetStream and dispatches to agent backends."""

    def __init__(self, nats_url: str = "nats://localhost:4222") -> None:
        self.nats_url = nats_url
        self._running = False

    async def start(self) -> None:
        """Connect to NATS and start consuming messages."""
        self._running = True
        logger.info("task consumer started (nats_url=%s)", self.nats_url)

        # Placeholder: NATS connection will be established in Phase 1
        while self._running:
            await asyncio.sleep(1)

    async def stop(self) -> None:
        """Gracefully stop the consumer."""
        self._running = False
        logger.info("task consumer stopped")


async def main() -> None:
    """Entry point for the worker process."""
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )

    consumer = TaskConsumer()

    loop = asyncio.get_running_loop()
    for sig in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(sig, lambda: asyncio.create_task(consumer.stop()))

    await consumer.start()


if __name__ == "__main__":
    asyncio.run(main())
