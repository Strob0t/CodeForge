"""NATS JetStream consumer for receiving tasks from Go Core.

The TaskConsumer is composed from handler mixins — each mixin owns a
related group of NATS message handlers.  The ``main()`` entry point
at the bottom starts the consumer.
"""

from __future__ import annotations

import asyncio
import functools
import signal
from pathlib import Path
from typing import TYPE_CHECKING

import nats
import nats.errors
import nats.js.client
import nats.js.errors
import structlog

from codeforge.config import WorkerSettings, get_settings
from codeforge.consumer._a2a import A2AHandlerMixin
from codeforge.consumer._backend_health import BackendHealthHandlerMixin
from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._benchmark import BenchmarkHandlerMixin
from codeforge.consumer._compact import CompactHandlerMixin
from codeforge.consumer._context import ContextHandlerMixin
from codeforge.consumer._context_events import ContextEventsHandlerMixin
from codeforge.consumer._conversation import ConversationHandlerMixin
from codeforge.consumer._delivery import PROGRESS_INTERVAL_SECONDS, ensure_durable, keep_in_progress
from codeforge.consumer._graph import GraphHandlerMixin
from codeforge.consumer._handoff import HandoffHandlerMixin
from codeforge.consumer._memory import MemoryHandlerMixin
from codeforge.consumer._prompt_evolution import PromptEvolutionHandlerMixin
from codeforge.consumer._quality_gate import QualityGateHandlerMixin
from codeforge.consumer._repomap import RepoMapHandlerMixin
from codeforge.consumer._retrieval import RetrievalHandlerMixin
from codeforge.consumer._review import ReviewHandlerMixin
from codeforge.consumer._runs import RunHandlerMixin
from codeforge.consumer._subjects import (
    STREAM_NAME,
    STREAM_SUBJECTS,
    SUBJECT_A2A_TASK_CANCEL,
    SUBJECT_A2A_TASK_CREATED,
    SUBJECT_AGENT,
    SUBJECT_BACKEND_HEALTH_REQUEST,
    SUBJECT_BENCHMARK_RUN_REQUEST,
    SUBJECT_CONTEXT_RERANK_REQUEST,
    SUBJECT_CONVERSATION_COMPACT_REQUEST,
    SUBJECT_CONVERSATION_RUN_START,
    SUBJECT_EVAL_GEMMAS_REQUEST,
    SUBJECT_GRAPH_BUILD_REQUEST,
    SUBJECT_GRAPH_SEARCH_REQUEST,
    SUBJECT_HANDOFF_REQUEST,
    SUBJECT_MEMORY_RECALL,
    SUBJECT_MEMORY_STORE,
    SUBJECT_PROMPT_EVOLUTION_PROMOTED,
    SUBJECT_PROMPT_EVOLUTION_REFLECT,
    SUBJECT_PROMPT_EVOLUTION_REVERTED,
    SUBJECT_QG_REQUEST,
    SUBJECT_REPOMAP_REQUEST,
    SUBJECT_RETRIEVAL_INDEX_REQUEST,
    SUBJECT_RETRIEVAL_SEARCH_REQUEST,
    SUBJECT_REVIEW_TRIGGER_REQUEST,
    SUBJECT_RUN_START,
    SUBJECT_SHARED_UPDATED,
    SUBJECT_SUBAGENT_SEARCH_REQUEST,
    consumer_name,
)
from codeforge.consumer._tasks import TaskHandlerMixin
from codeforge.executor import AgentExecutor
from codeforge.graphrag import CodeGraphBuilder, GraphSearcher
from codeforge.llm import LiteLLMClient
from codeforge.logger import setup_logging, stop_logging
from codeforge.qualitygate import QualityGateExecutor
from codeforge.repomap import RepoMapGenerator
from codeforge.retrieval import HybridRetriever, RetrievalSubAgent
from codeforge.tracing import tracing_manager

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable

    from nats.aio.client import Client as NATSClient
    from nats.js.client import JetStreamContext

logger = structlog.get_logger()

# Sentinel file touched when NATS connection is active and message loops
# are running.  The worker healthcheck script checks for this file.
_HEALTHY_SENTINEL = Path("/tmp/codeforge-worker-healthy")  # noqa: S108

# Consumer error backoff config (from centralized WorkerSettings)
_MAX_CONSECUTIVE_ERRORS = get_settings().consumer_max_errors
_BACKOFF_MULTIPLIER = get_settings().consumer_backoff_multiplier
_BACKOFF_MAX = get_settings().consumer_backoff_max

# Errors of a fetch on a durable that no longer exists ("no responders" for the
# pull request, or consumer not found).
_CONSUMER_GONE_ERRORS = (nats.js.errors.NotFoundError, nats.js.errors.ServiceUnavailableError)


class TaskConsumer(
    ConsumerBaseMixin,
    TaskHandlerMixin,
    RunHandlerMixin,
    QualityGateHandlerMixin,
    RepoMapHandlerMixin,
    RetrievalHandlerMixin,
    GraphHandlerMixin,
    ConversationHandlerMixin,
    CompactHandlerMixin,
    ContextHandlerMixin,
    ContextEventsHandlerMixin,
    BenchmarkHandlerMixin,
    MemoryHandlerMixin,
    HandoffHandlerMixin,
    A2AHandlerMixin,
    BackendHealthHandlerMixin,
    ReviewHandlerMixin,
    PromptEvolutionHandlerMixin,
):
    """Consumes task messages from NATS JetStream and dispatches them to the executor."""

    # How often a message that is still being handled is reported in progress.
    _progress_interval: float = PROGRESS_INTERVAL_SECONDS

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
        # Set when a message loop could not recover; main() then exits non-zero.
        self.failed = False
        self._llm = LiteLLMClient(base_url=litellm_url, api_key=litellm_key)
        self._db_url = get_settings().database_url

        from codeforge.memory.experience import ExperiencePool

        self._experience_pool = ExperiencePool(db_url=self._db_url, llm=self._llm)
        self._executor = AgentExecutor(llm=self._llm, experience_pool=self._experience_pool)

        from codeforge.backends import build_default_router

        self._backend_router = build_default_router()
        self._gate_executor = QualityGateExecutor()
        self._repomap_generator = RepoMapGenerator()
        self._retriever = HybridRetriever(litellm_url=litellm_url, litellm_key=litellm_key)
        self._subagent = RetrievalSubAgent(retriever=self._retriever, llm=self._llm)
        self._graph_builder = CodeGraphBuilder()
        self._graph_searcher = GraphSearcher()

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
            (SUBJECT_CONTEXT_RERANK_REQUEST, self._handle_context_rerank),
            (SUBJECT_CONVERSATION_RUN_START, self._handle_conversation_run),
            (SUBJECT_CONVERSATION_COMPACT_REQUEST, self._handle_conversation_compact),
            (SUBJECT_BENCHMARK_RUN_REQUEST, self._handle_benchmark_run),
            (SUBJECT_EVAL_GEMMAS_REQUEST, self._handle_gemmas_eval),
            (SUBJECT_MEMORY_STORE, self._handle_memory_store),
            (SUBJECT_MEMORY_RECALL, self._handle_memory_recall),
            (SUBJECT_HANDOFF_REQUEST, self._handle_handoff_request),
            (SUBJECT_A2A_TASK_CREATED, self._handle_a2a_task_created),
            (SUBJECT_A2A_TASK_CANCEL, self._handle_a2a_task_cancel),
            (SUBJECT_BACKEND_HEALTH_REQUEST, self._handle_backend_health),
            (SUBJECT_REVIEW_TRIGGER_REQUEST, self._handle_review_trigger),
            (SUBJECT_PROMPT_EVOLUTION_REFLECT, self._handle_prompt_evolution_reflect),
            (SUBJECT_PROMPT_EVOLUTION_PROMOTED, self._handle_prompt_promoted),
            (SUBJECT_PROMPT_EVOLUTION_REVERTED, self._handle_prompt_reverted),
            (SUBJECT_SHARED_UPDATED, self._handle_shared_context_updated),
        ]

        loops = []
        for subject, handler in subscriptions:
            name = consumer_name(subject)
            sub = await ensure_durable(self._js, name, subject)
            logger.info("subscribed", subject=subject, durable=name)
            reattach = functools.partial(ensure_durable, self._js, name, subject)
            loops.append(self._message_loop(sub, handler, subject, reattach=reattach))

        # Signal to the Docker healthcheck that the worker is connected and
        # all subscriptions are active.
        _HEALTHY_SENTINEL.touch()
        logger.info("healthcheck sentinel created", path=str(_HEALTHY_SENTINEL))

        await asyncio.gather(*loops)

    async def _message_loop(
        self,
        sub: nats.js.client.JetStreamContext.PullSubscription,
        handler: Callable[[nats.aio.msg.Msg], Awaitable[None]],
        label: str,
        reattach: Callable[[], Awaitable[nats.js.client.JetStreamContext.PullSubscription]] | None = None,
    ) -> None:
        """Generic message processing loop shared by all subscriptions.

        A durable deleted while the worker runs is ensured again through
        *reattach*. A loop that cannot recover stops the whole worker (see
        ``_give_up``) instead of leaving a subject without a consumer.
        """
        consecutive_errors = 0
        max_consecutive_errors = _MAX_CONSECUTIVE_ERRORS
        while self._running:
            try:
                msgs = await sub.fetch(batch=1, timeout=1)
                consecutive_errors = 0
            except TimeoutError:
                continue
            except nats.errors.TimeoutError:
                continue
            except Exception as exc:
                if not self._running:
                    break
                consecutive_errors += 1
                logger.exception(
                    "error receiving message",
                    subject=label,
                    error=str(exc),
                    consecutive_errors=consecutive_errors,
                )
                if consecutive_errors >= max_consecutive_errors:
                    self._give_up(label)
                    break
                await asyncio.sleep(min(consecutive_errors * _BACKOFF_MULTIPLIER, _BACKOFF_MAX))
                if reattach is not None and isinstance(exc, _CONSUMER_GONE_ERRORS):
                    sub = await self._reattach(sub, reattach, label)
                continue

            for msg in msgs:
                import time as _time

                from opentelemetry import context as otel_context

                from codeforge.tracing import metrics as otel_metrics
                from codeforge.tracing.propagation import extract_trace_context

                # Extract W3C trace context from NATS headers for distributed tracing.
                raw_headers: dict[str, str] = {}
                if msg.headers:
                    for k, v in msg.headers.items():
                        raw_headers[k] = v[0] if isinstance(v, list) else v
                _, token = extract_trace_context(raw_headers)
                msg_start = _time.monotonic()
                try:
                    # Handlers may run longer than the ack wait; keep JetStream
                    # from redelivering the message to another worker meanwhile.
                    async with keep_in_progress(msg, self._progress_interval):
                        await handler(msg)
                except Exception as exc:
                    # Handlers settle their messages themselves; an escaping error
                    # must not end the loop. The unsettled message is redelivered
                    # after the ack wait, at most MAX_DELIVER times in total.
                    logger.exception("unhandled error in message handler", subject=label, error=str(exc))
                finally:
                    otel_metrics.nats_processing.record(_time.monotonic() - msg_start)
                    otel_context.detach(token)

    @staticmethod
    async def _reattach(
        old: nats.js.client.JetStreamContext.PullSubscription,
        reattach: Callable[[], Awaitable[nats.js.client.JetStreamContext.PullSubscription]],
        label: str,
    ) -> nats.js.client.JetStreamContext.PullSubscription:
        """Ensure the durable again (it was deleted) and bind a new subscription to it."""
        try:
            fresh = await reattach()
        except Exception as exc:
            logger.warning("re-attaching the durable consumer failed", subject=label, error=str(exc))
            return old
        try:
            await old.unsubscribe()
        except Exception as exc:
            logger.debug("unsubscribing the old pull subscription failed", subject=label, error=str(exc))
        logger.info("durable consumer re-attached", subject=label)
        return fresh

    def _give_up(self, subject: str) -> None:
        """Stop the worker after a message loop could not recover.

        Every loop ends, the health sentinel is removed and main() exits with
        status 1, so the container's restart policy starts a fresh worker instead
        of a "healthy" process that no longer consumes *subject*.
        """
        logger.error("message loop cannot recover, stopping the worker", subject=subject)
        self.failed = True
        self._running = False
        _HEALTHY_SENTINEL.unlink(missing_ok=True)

    async def stop(self) -> None:
        """Gracefully shut down: drain with timeout and close."""
        self._running = False
        logger.info("stopping consumer")

        # Remove healthcheck sentinel so Docker marks the container unhealthy
        # during shutdown.
        _HEALTHY_SENTINEL.unlink(missing_ok=True)

        await self._llm.close()
        await self._retriever.close()

        if self._nc is not None and self._nc.is_connected:
            try:
                await asyncio.wait_for(self._nc.drain(), timeout=10.0)
            except TimeoutError:
                logger.warning("NATS drain timed out after 10s, closing connection")
                await self._nc.close()

        tracing_manager.shutdown()
        logger.info("consumer stopped")
        stop_logging()


async def main() -> None:
    """Entry point for running the consumer."""
    from codeforge.secrets import get_secret

    settings = WorkerSettings()
    setup_logging(service=settings.log_service, level=settings.log_level)

    # Docker Secrets override: prefer /run/secrets/* files, fall back to env/config.
    litellm_key = get_secret("LITELLM_MASTER_KEY") or settings.litellm_api_key

    consumer = TaskConsumer(
        nats_url=settings.nats_url,
        litellm_url=settings.litellm_url,
        litellm_key=litellm_key,
    )

    loop = asyncio.get_running_loop()
    # FIX-091: Bind consumer via default argument so the closure captures
    # the current value, not the variable (prevents stale reference if
    # the function is ever refactored to reassign consumer).
    for sig in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(sig, lambda c=consumer: asyncio.create_task(c.stop()))

    await consumer.start()
    if consumer.failed:
        await consumer.stop()
        raise SystemExit(1)


if __name__ == "__main__":
    asyncio.run(main())


__all__ = ["TaskConsumer", "main"]
