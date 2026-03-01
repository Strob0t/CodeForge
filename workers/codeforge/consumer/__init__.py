"""NATS JetStream consumer for receiving tasks from Go Core.

The TaskConsumer is composed from handler mixins — each mixin owns a
related group of NATS message handlers.  The ``main()`` entry point
at the bottom starts the consumer.
"""

from __future__ import annotations

import asyncio
import os
import signal
from typing import TYPE_CHECKING

import nats
import structlog

from codeforge.config import WorkerSettings
from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._benchmark import BenchmarkHandlerMixin
from codeforge.consumer._conversation import ConversationHandlerMixin
from codeforge.consumer._graph import GraphHandlerMixin
from codeforge.consumer._handoff import HandoffHandlerMixin
from codeforge.consumer._memory import MemoryHandlerMixin
from codeforge.consumer._quality_gate import QualityGateHandlerMixin
from codeforge.consumer._repomap import RepoMapHandlerMixin
from codeforge.consumer._retrieval import RetrievalHandlerMixin
from codeforge.consumer._runs import RunHandlerMixin
from codeforge.consumer._subjects import (
    STREAM_NAME,
    STREAM_SUBJECTS,
    SUBJECT_AGENT,
    SUBJECT_BENCHMARK_RUN_REQUEST,
    SUBJECT_CONVERSATION_RUN_START,
    SUBJECT_EVAL_GEMMAS_REQUEST,
    SUBJECT_GRAPH_BUILD_REQUEST,
    SUBJECT_GRAPH_SEARCH_REQUEST,
    SUBJECT_HANDOFF_REQUEST,
    SUBJECT_MEMORY_RECALL,
    SUBJECT_MEMORY_STORE,
    SUBJECT_QG_REQUEST,
    SUBJECT_REPOMAP_REQUEST,
    SUBJECT_RETRIEVAL_INDEX_REQUEST,
    SUBJECT_RETRIEVAL_SEARCH_REQUEST,
    SUBJECT_RUN_START,
    SUBJECT_SUBAGENT_SEARCH_REQUEST,
)
from codeforge.consumer._tasks import TaskHandlerMixin
from codeforge.executor import AgentExecutor
from codeforge.graphrag import CodeGraphBuilder, GraphSearcher
from codeforge.llm import LiteLLMClient
from codeforge.logger import setup_logging, stop_logging
from codeforge.qualitygate import QualityGateExecutor
from codeforge.repomap import RepoMapGenerator
from codeforge.retrieval import HybridRetriever, RetrievalSubAgent

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable

    from nats.aio.client import Client as NATSClient
    from nats.js.client import JetStreamContext

logger = structlog.get_logger()


class TaskConsumer(
    ConsumerBaseMixin,
    TaskHandlerMixin,
    RunHandlerMixin,
    QualityGateHandlerMixin,
    RepoMapHandlerMixin,
    RetrievalHandlerMixin,
    GraphHandlerMixin,
    ConversationHandlerMixin,
    BenchmarkHandlerMixin,
    MemoryHandlerMixin,
    HandoffHandlerMixin,
):
    """Consumes task messages from NATS JetStream and dispatches them to the executor."""

    def __init__(
        self,
        nats_url: str = "nats://localhost:4222",
        litellm_url: str = "http://localhost:4000",
        litellm_key: str = "",
    ) -> None:
        self.nats_url = nats_url
        self._litellm_url = litellm_url
        self._litellm_key = litellm_key
        self._nc: NATSClient | None = None
        self._js: JetStreamContext | None = None
        self._running = False
        self._llm = LiteLLMClient(base_url=litellm_url, api_key=litellm_key)
        self._executor = AgentExecutor(llm=self._llm)

        from codeforge.backends import build_default_router

        self._backend_router = build_default_router()
        self._gate_executor = QualityGateExecutor()
        self._repomap_generator = RepoMapGenerator()
        self._retriever = HybridRetriever(litellm_url=litellm_url, litellm_key=litellm_key)
        self._subagent = RetrievalSubAgent(retriever=self._retriever, llm=self._llm)
        self._graph_builder = CodeGraphBuilder()
        self._graph_searcher = GraphSearcher()
        self._db_url = os.environ.get(
            "DATABASE_URL",
            "postgresql://codeforge:codeforge_dev@localhost:5432/codeforge",
        )

    async def start(self) -> None:
        """Connect to NATS and subscribe to task and run subjects."""
        self._nc = await nats.connect(self.nats_url)
        self._js = self._nc.jetstream()
        self._running = True

        logger.info("connected to NATS", url=self.nats_url)

        try:
            await self._js.find_stream_name_by_subject(STREAM_SUBJECTS[0])
        except nats.js.errors.NotFoundError:
            await self._js.add_stream(
                name=STREAM_NAME,
                subjects=STREAM_SUBJECTS,
            )
            logger.info("created JetStream stream", stream=STREAM_NAME)

        subscriptions: list[tuple[str, Callable[[nats.aio.msg.Msg], Awaitable[None]]]] = [
            (SUBJECT_AGENT, self._handle_message),
            (SUBJECT_RUN_START, self._handle_run_start),
            (SUBJECT_QG_REQUEST, self._handle_quality_gate),
            (SUBJECT_REPOMAP_REQUEST, self._handle_repomap),
            (SUBJECT_RETRIEVAL_INDEX_REQUEST, self._handle_retrieval_index),
            (SUBJECT_RETRIEVAL_SEARCH_REQUEST, self._handle_retrieval_search),
            (SUBJECT_SUBAGENT_SEARCH_REQUEST, self._handle_subagent_search),
            (SUBJECT_GRAPH_BUILD_REQUEST, self._handle_graph_build),
            (SUBJECT_GRAPH_SEARCH_REQUEST, self._handle_graph_search),
            (SUBJECT_CONVERSATION_RUN_START, self._handle_conversation_run),
            (SUBJECT_BENCHMARK_RUN_REQUEST, self._handle_benchmark_run),
            (SUBJECT_EVAL_GEMMAS_REQUEST, self._handle_gemmas_eval),
            (SUBJECT_MEMORY_STORE, self._handle_memory_store),
            (SUBJECT_MEMORY_RECALL, self._handle_memory_recall),
            (SUBJECT_HANDOFF_REQUEST, self._handle_handoff_request),
        ]

        loops = []
        for subject, handler in subscriptions:
            sub = await self._js.subscribe(subject, stream=STREAM_NAME, manual_ack=True)
            logger.info("subscribed", subject=subject)
            loops.append(self._message_loop(sub, handler, subject))

        await asyncio.gather(*loops)

    async def _message_loop(
        self,
        sub: object,
        handler: Callable[[nats.aio.msg.Msg], Awaitable[None]],
        label: str,
    ) -> None:
        """Generic message processing loop shared by all subscriptions."""
        while self._running:
            try:
                msg = await asyncio.wait_for(sub.next_msg(), timeout=1.0)  # type: ignore[union-attr]
            except TimeoutError:
                continue
            except Exception:
                if self._running:
                    logger.exception("error receiving message", subject=label)
                break

            await handler(msg)

    async def stop(self) -> None:
        """Gracefully shut down: drain with timeout and close."""
        self._running = False
        logger.info("stopping consumer")

        await self._llm.close()
        await self._retriever.close()

        if self._nc is not None and self._nc.is_connected:
            try:
                await asyncio.wait_for(self._nc.drain(), timeout=10.0)
            except TimeoutError:
                logger.warning("NATS drain timed out after 10s, closing connection")
                await self._nc.close()

        logger.info("consumer stopped")
        stop_logging()


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


__all__ = ["TaskConsumer", "main"]
