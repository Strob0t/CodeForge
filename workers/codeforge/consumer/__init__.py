"""NATS JetStream consumer for receiving tasks from Go Core.

The TaskConsumer is composed from handler mixins — each mixin owns a
related group of NATS message handlers.  The ``main()`` entry point
at the bottom starts the consumer.
"""

from __future__ import annotations

import asyncio
import functools
import os
import signal
import sys
import threading
import traceback
from typing import TYPE_CHECKING

import nats
import nats.js.client
import nats.js.errors
import structlog

from codeforge.config import DEV_LITELLM_MASTER_KEY, WorkerSettings, get_settings
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
from codeforge.consumer._memory import MemoryHandlerMixin
from codeforge.consumer._prompt_evolution import PromptEvolutionHandlerMixin
from codeforge.consumer._quality_gate import QualityGateHandlerMixin
from codeforge.consumer._repomap import RepoMapHandlerMixin
from codeforge.consumer._retrieval import RetrievalHandlerMixin
from codeforge.consumer._runs import RunHandlerMixin
from codeforge.consumer._subjects import (
    INBOX_PREFIX,
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
    SUBJECT_CONVERSATION_TEST_REQUEST,
    SUBJECT_EVAL_GEMMAS_REQUEST,
    SUBJECT_GRAPH_BUILD_REQUEST,
    SUBJECT_GRAPH_SEARCH_REQUEST,
    SUBJECT_MEMORY_RECALL,
    SUBJECT_MEMORY_STORE,
    SUBJECT_PROMPT_EVOLUTION_PROMOTED,
    SUBJECT_PROMPT_EVOLUTION_REFLECT,
    SUBJECT_PROMPT_EVOLUTION_REVERTED,
    SUBJECT_QG_REQUEST,
    SUBJECT_REPOMAP_REQUEST,
    SUBJECT_RETRIEVAL_INDEX_REQUEST,
    SUBJECT_RETRIEVAL_SEARCH_REQUEST,
    SUBJECT_RUN_START,
    SUBJECT_SHARED_UPDATED,
    SUBJECT_SUBAGENT_SEARCH_REQUEST,
    SUBJECT_WORKSPACE_DELETE_REQUEST,
    consumer_name,
)
from codeforge.consumer._tasks import TaskHandlerMixin
from codeforge.consumer._workspace_delete import WorkspaceDeleteHandlerMixin
from codeforge.consumer._workspace_test import WorkspaceTestHandlerMixin
from codeforge.executor import AgentExecutor
from codeforge.graphrag import CodeGraphBuilder, GraphSearcher
from codeforge.health import start_health_server
from codeforge.llm import LiteLLMClient
from codeforge.logger import redact_url, setup_logging, stop_logging
from codeforge.notifications import NotificationHub
from codeforge.qualitygate import QualityGateExecutor
from codeforge.repomap import RepoMapGenerator
from codeforge.retrieval import HybridRetriever, RetrievalSubAgent
from codeforge.secrets import SECRETS_DIR, lock_secrets_dir
from codeforge.tool_process import (
    IsolationConfig,
    IsolationStatus,
    configure_tool_isolation,
    tool_isolation,
)
from codeforge.tracing import tracing_manager
from codeforge.tracing.propagation import TracingJetStreamContext

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable, Coroutine

    from nats.aio.client import Client as NATSClient
    from nats.js.client import JetStreamContext

logger = structlog.get_logger()

# Consumer error backoff config (from centralized WorkerSettings)
_MAX_CONSECUTIVE_ERRORS = get_settings().consumer_max_errors
_BACKOFF_MULTIPLIER = get_settings().consumer_backoff_multiplier
_BACKOFF_MAX = get_settings().consumer_backoff_max

# A pull request to a durable that no longer exists has no responders, which
# nats-py raises as ServiceUnavailableError (status 503). A pending pull whose
# durable is deleted gets "409 Consumer Deleted", which nats-py reports as a
# timeout; the next fetch then fails with the 503.
_CONSUMER_GONE_ERRORS = (nats.js.errors.ServiceUnavailableError,)

# After a message loop gave up, accepted at-most-once work may finish within
# this time; what is still running then is cancelled and reported as failed.
_GIVE_UP_GRACE_SECONDS = 30.0

# On SIGTERM accepted at-most-once work may finish within this time before it
# is cancelled and reported as failed (KI-65). The whole shutdown (this grace,
# the cancellation and the reports in _in_flight, the drain) must fit into the
# worker's stop_grace_period in docker-compose.prod.yml, after which Docker
# kills the process and the completions are lost.
_SHUTDOWN_GRACE_SECONDS = 5.0
_DRAIN_TIMEOUT_SECONDS = 10.0

# The Go Core creates and configures the CODEFORGE stream (limits, retention,
# dedup window); a worker that starts first waits this long for it.
_STREAM_WAIT_SECONDS = 120.0
_STREAM_POLL_SECONDS = 2.0


class TaskConsumer(
    ConsumerBaseMixin,
    TaskHandlerMixin,
    RunHandlerMixin,
    QualityGateHandlerMixin,
    WorkspaceTestHandlerMixin,
    WorkspaceDeleteHandlerMixin,
    RepoMapHandlerMixin,
    RetrievalHandlerMixin,
    GraphHandlerMixin,
    ConversationHandlerMixin,
    CompactHandlerMixin,
    ContextHandlerMixin,
    ContextEventsHandlerMixin,
    BenchmarkHandlerMixin,
    MemoryHandlerMixin,
    A2AHandlerMixin,
    BackendHealthHandlerMixin,
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
        self._notifications: NotificationHub | None = None
        self._running = False
        # Sticky: once set, start() shuts down instead of starting its loops.
        self._stop_requested = False
        # Set when a message loop could not recover; main() then exits non-zero.
        self.failed = False
        self._loop_tasks: list[asyncio.Task[None]] = []
        # Fails the accepted work and stops the loops after a loop gave up.
        self._abort_task: asyncio.Task[None] | None = None
        self._llm = LiteLLMClient(base_url=litellm_url, api_key=litellm_key)
        settings = get_settings()
        self._db_url = settings.database_url
        self._knowledge_content_root = settings.knowledge_content_root

        from codeforge.memory.experience import ExperiencePool

        # Off unless experience.enabled: only first-turn simple chats use it.
        self._experience_pool: ExperiencePool | None = None
        if settings.experience_enabled:
            self._experience_pool = ExperiencePool(
                db_url=self._db_url,
                llm=self._llm,
                confidence_threshold=settings.experience_confidence_threshold,
                max_entries=settings.experience_max_entries,
            )
        self._executor = AgentExecutor(llm=self._llm, litellm_url=litellm_url, litellm_key=litellm_key)

        from codeforge.backends import build_default_router

        self._backend_router = build_default_router()
        self._gate_executor = QualityGateExecutor()
        self._repomap_generator = RepoMapGenerator()
        self._retriever = HybridRetriever(litellm_url=litellm_url, litellm_key=litellm_key)
        self._subagent = RetrievalSubAgent(retriever=self._retriever, llm=self._llm)
        self._graph_builder = CodeGraphBuilder()
        self._graph_searcher = GraphSearcher()

    @property
    def ready(self) -> bool:
        """Whether the worker consumes its subjects: running, connected to NATS, every loop alive, its
        notifications complete (consumers restored, nothing missed left to read back), not given up.

        Read by the health server thread (GET /health/ready).
        """
        loops = list(self._loop_tasks)
        return (
            self._running
            and not self.failed
            and self._nc is not None
            and self._nc.is_connected
            and bool(loops)
            and not any(task.done() for task in loops)
            and self._notifications is not None
            and self._notifications.ready
        )

    def request_stop(self) -> None:
        """Ask the worker to stop (signal handler; stop() does the shutdown).

        The request is sticky: a start() that is still connecting or subscribing
        returns without starting its loops, whenever the request arrived.
        """
        self._stop_requested = True
        self._running = False

    async def start(self) -> None:
        """Connect to NATS and subscribe to task and run subjects.

        Returns without consuming if a stop was requested meanwhile (see
        request_stop); otherwise returns once every message loop ended.
        """
        # Reconnect until the worker stops: nats-py's default of 60 attempts
        # (about 2 minutes) left the worker without NATS for good after a
        # longer outage (KI-213).
        self._nc = await nats.connect(
            self.nats_url,
            inbox_prefix=INBOX_PREFIX,
            reconnected_cb=self._restore_notifications,
            max_reconnect_attempts=-1,
        )
        if self._stop_requested:
            # stop() may have run while connecting, with no connection to drain.
            logger.info("stop requested while connecting, the consumer does not start")
            if self._nc.is_connected:
                await self._nc.close()
            return
        js = self._js = TracingJetStreamContext(self._nc)
        self._running = True

        logger.info("connected to NATS", url=redact_url(self.nats_url))

        try:
            await self._wait_for_stream(js)
            # Before any work is fetched: runs and tasks listen for cancels
            # and tool-call decisions through it.
            hub = NotificationHub(self._nc, js)
            await hub.start()
            self._notifications = hub
            loops = await self._subscribe_all(js)
        except Exception:
            if self._stop_requested:
                # stop() drained the connection under the subscribing start().
                logger.info("stop requested while subscribing, the consumer does not start")
                return
            raise
        if self._stop_requested:
            logger.info("stop requested while subscribing, the consumer does not start")
            return
        await self._start_cancel_registry()

        for subject, run_loop in loops:
            task = asyncio.create_task(run_loop(), name=f"message-loop {subject}")
            task.add_done_callback(functools.partial(self._loop_ended, subject))
            self._loop_tasks.append(task)

        # From here on GET /health/ready answers 200 (see ready).
        logger.info("worker ready", subscriptions=len(self._loop_tasks))

        await asyncio.wait(self._loop_tasks)
        if self._abort_task is not None:
            await self._abort_task

    async def _subscribe_all(
        self, js: JetStreamContext
    ) -> list[tuple[str, Callable[[], Coroutine[object, object, None]]]]:
        """Ensure every durable (the stream exists); return the message loops (not started yet)."""

        subscriptions: list[tuple[str, Callable[[nats.aio.msg.Msg], Awaitable[None]]]] = [
            (SUBJECT_AGENT, self._handle_message),
            (SUBJECT_RUN_START, self._handle_run_start),
            (SUBJECT_QG_REQUEST, self._handle_quality_gate),
            (SUBJECT_CONVERSATION_TEST_REQUEST, self._handle_workspace_test),
            (SUBJECT_WORKSPACE_DELETE_REQUEST, self._handle_workspace_delete),
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
            (SUBJECT_A2A_TASK_CREATED, self._handle_a2a_task_created),
            (SUBJECT_A2A_TASK_CANCEL, self._handle_a2a_task_cancel),
            (SUBJECT_BACKEND_HEALTH_REQUEST, self._handle_backend_health),
            (SUBJECT_PROMPT_EVOLUTION_REFLECT, self._handle_prompt_evolution_reflect),
            (SUBJECT_PROMPT_EVOLUTION_PROMOTED, self._handle_prompt_promoted),
            (SUBJECT_PROMPT_EVOLUTION_REVERTED, self._handle_prompt_reverted),
            (SUBJECT_SHARED_UPDATED, self._handle_shared_context_updated),
        ]

        loops: list[tuple[str, Callable[[], Coroutine[object, object, None]]]] = []
        for subject, handler in subscriptions:
            name = consumer_name(subject)
            sub = await ensure_durable(js, name, subject)
            logger.info("subscribed", subject=subject, durable=name)
            reattach = functools.partial(ensure_durable, js, name, subject)
            loops.append((subject, functools.partial(self._message_loop, sub, handler, subject, reattach=reattach)))
        return loops

    async def _wait_for_stream(self, js: JetStreamContext) -> None:
        """Wait until the Go Core has created the CODEFORGE stream (KI-67).

        The worker does not create it: a stream with default settings (no size
        limit, no dedup window) would stay in place with the wrong
        configuration. Raises RuntimeError if the stream does not appear in
        time; the container's restart policy then starts the worker again.
        """
        loop = asyncio.get_running_loop()
        deadline = loop.time() + _STREAM_WAIT_SECONDS
        while True:
            try:
                await js.find_stream_name_by_subject(STREAM_SUBJECTS[0])
                return
            except nats.js.errors.NotFoundError:
                if self._stop_requested or loop.time() >= deadline:
                    msg = f"JetStream stream {STREAM_NAME} not found: the Go Core creates it, start the Go Core first"
                    raise RuntimeError(msg) from None
                logger.info("waiting for the Go Core to create the JetStream stream", stream=STREAM_NAME)
                await asyncio.sleep(_STREAM_POLL_SECONDS)

    def _loop_ended(self, subject: str, task: asyncio.Task[None]) -> None:
        """A loop that ended with an exception leaves its subject unconsumed: stop the worker."""
        if task.cancelled() or task.exception() is None:
            return
        logger.error("message loop crashed", subject=subject, error=str(task.exception()))
        self._give_up(subject)

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
                # nats.errors.TimeoutError is a TimeoutError: the server answered
                # that there is nothing to fetch, the consumer is healthy.
                consecutive_errors = 0
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
                    fresh = await self._reattach(sub, reattach, label)
                    if fresh is not None:
                        sub = fresh
                        consecutive_errors = 0
                continue

            for msg in msgs:
                import time as _time

                from opentelemetry import context as otel_context

                from codeforge.tenant_context import bind_tenant, reset_tenant, tenant_of
                from codeforge.tracing import metrics as otel_metrics
                from codeforge.tracing.propagation import extract_trace_context

                # Extract W3C trace context from NATS headers for distributed tracing.
                raw_headers: dict[str, str] = {}
                if msg.headers:
                    for k, v in msg.headers.items():
                        raw_headers[k] = v[0] if isinstance(v, list) else v
                _, token = extract_trace_context(raw_headers)
                # The message is handled in the tenant of its header, which
                # everything published meanwhile carries back (KI-64).
                tenant_token = bind_tenant(tenant_of(raw_headers))
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
                    reset_tenant(tenant_token)
                    otel_context.detach(token)

    @staticmethod
    async def _reattach(
        old: nats.js.client.JetStreamContext.PullSubscription,
        reattach: Callable[[], Awaitable[nats.js.client.JetStreamContext.PullSubscription]],
        label: str,
    ) -> nats.js.client.JetStreamContext.PullSubscription | None:
        """Ensure the durable again (it was deleted) and bind a new subscription to it; None if that failed."""
        try:
            fresh = await reattach()
        except Exception as exc:
            logger.warning("re-attaching the durable consumer failed", subject=label, error=str(exc))
            return None
        try:
            await old.unsubscribe()
        except Exception as exc:
            logger.debug("unsubscribing the old pull subscription failed", subject=label, error=str(exc))
        logger.info("durable consumer re-attached", subject=label)
        return fresh

    def _give_up(self, subject: str) -> None:
        """Stop the worker after a message loop could not recover.

        Every loop ends, /health/ready fails at once (see ready) and main() exits
        with status 1, so the container's restart policy starts a fresh worker
        instead of a "healthy" process that no longer consumes *subject*. Accepted
        at-most-once work gets a bounded grace period; what is still running
        then is cancelled and reported as failed (ADR-016), so the exit is not
        delayed by a long run and the Go Core does not wait for its timeout.
        """
        logger.error("message loop cannot recover, stopping the worker", subject=subject)
        self.failed = True
        self._running = False
        self._abort_accepted_work(
            _GIVE_UP_GRACE_SECONDS,
            f"worker stopped before the work finished: message loop for {subject} could not recover",
        )

    def _abort_accepted_work(self, grace: float, reason: str) -> asyncio.Task[None]:
        """Start aborting the worker's work once (see InFlightWork.abort); the first reason wins."""
        if self._abort_task is None:
            self._abort_task = asyncio.create_task(
                self._in_flight.abort(self._loop_tasks, grace, reason), name="abort accepted work"
            )
        return self._abort_task

    async def _restore_notifications(self) -> None:
        """After a reconnect: the hub recreates notification consumers the server lost and
        reads back what this worker missed, retrying until it succeeded (not ready meanwhile).
        """
        if self._notifications is not None:
            self._notifications.reconnected()

    async def stop(self) -> None:
        """Gracefully shut down: fail unfinished accepted work, drain with timeout and close.

        Safe to call more than once. Accepted at-most-once work is never
        redelivered (ADR-016): what does not finish within the shutdown grace
        is cancelled and reported as failed while NATS is still connected, so
        the Go Core learns its outcome instead of waiting for its watchdog.
        """
        self.request_stop()  # /health/ready fails from now on; a start() still setting up stops
        logger.info("stopping consumer")

        await self._abort_accepted_work(
            _SHUTDOWN_GRACE_SECONDS, "worker stopped before the work finished: the worker is shutting down"
        )

        await self._llm.close()
        await self._retriever.close()

        # start() closes a connection it makes after this point itself.
        if self._nc is not None and self._nc.is_connected:
            try:
                await asyncio.wait_for(self._nc.drain(), timeout=_DRAIN_TIMEOUT_SECONDS)
            except TimeoutError:
                logger.warning("NATS drain timed out, closing connection", timeout=_DRAIN_TIMEOUT_SECONDS)
                await self._nc.close()
            except Exception as exc:
                logger.warning("NATS drain failed", error=str(exc))
        if self._notifications is not None:
            self._notifications.close()

        # The final OTLP export blocks; keep the event loop responsive meanwhile.
        await asyncio.to_thread(tracing_manager.shutdown)
        logger.info("consumer stopped")


# The worker's own umask with isolation: what it creates stays writable for
# the workspace group (the Go Core); below a tenant directory's default ACL
# the kernel ignores it.
WORKER_UMASK = 0o002


def setup_tool_isolation(settings: WorkerSettings) -> IsolationStatus:
    """Check how agent tool processes run (KI-71, KI-96) and log it; call once every secret was read.

    With isolation required the check prepares and verifies the volumes (the
    workspace root 2771, the worker's state directory, the tool HOME base,
    POSIX ACLs) and probes a tool process; the worker creates files with
    umask 002 and locks its secrets directory.
    """
    config = IsolationConfig.from_settings(settings)
    if config.required:
        os.umask(WORKER_UMASK)
    status = configure_tool_isolation(config)
    if not config.required:
        logger.info("tool isolation off: agent tool processes run as the worker user", worker_uid=os.getuid())
        return status
    if status.ready:
        logger.info(
            "tool isolation required: every tenant's tool processes run as the tenant's tool UID",
            worker_uid=os.getuid(),
            workspace_root=config.workspace_root,
            home_base=config.home_base,
            tool_path=config.tool_path,
            landlock=config.landlock,
            landlock_abi=status.landlock_abi,
            scoped=status.landlock_abi >= 6,
        )
        _log_landlock_gaps(config, status)
    else:
        logger.error(
            "tool isolation required but not available: every tool call fails and the worker is not ready",
            reason=status.reason,
        )
    if lock_secrets_dir():
        logger.info("secrets directory locked after reading the secrets", path=str(SECRETS_DIR))
    return status


def _log_landlock_gaps(config: IsolationConfig, status: IsolationStatus) -> None:
    """What the kernel's Landlock ABI leaves open, once at startup."""
    if not config.confined:
        logger.warning(
            "Landlock is off (CODEFORGE_TOOL_LANDLOCK=off): tool command lines and /proc are readable across "
            "tenants; for development only"
        )
        return
    if status.landlock_abi < 3:
        logger.warning(
            "Landlock ABI below 3 does not handle truncate: tools can truncate files of their tenant's other "
            "projects (other tenants stay separated by file permissions)",
            landlock_abi=status.landlock_abi,
        )
    if status.landlock_abi < 6:
        logger.warning(
            "Landlock ABI below 6 has no scopes: abstract unix sockets and signals between a tenant's runs are "
            "not separated",
            landlock_abi=status.landlock_abi,
        )


def isolation_problem() -> str:
    """Why tool processes cannot start ("" when they can or isolation is off): the worker is then not ready."""
    status = tool_isolation()
    if not status.config.required or status.ready:
        return ""
    return f"tool isolation not ready: {status.reason}"


async def main() -> None:
    """Entry point for running the consumer."""
    from codeforge.secrets import get_secret

    settings = get_settings()
    setup_logging(service=settings.log_service, level=settings.log_level)

    # Docker Secrets override: prefer /run/secrets/* files, fall back to env/config.
    litellm_key = get_secret("LITELLM_MASTER_KEY") or settings.litellm_api_key
    if litellm_key == DEV_LITELLM_MASTER_KEY:
        logger.warning("using the development LiteLLM master key - set LITELLM_MASTER_KEY for production")
    tracing_manager.log_status()
    # After every secret was read: it locks the secrets directory.
    await asyncio.to_thread(setup_tool_isolation, settings)

    consumer = TaskConsumer(
        nats_url=settings.nats_url,
        litellm_url=settings.litellm_url,
        litellm_key=litellm_key,
    )

    # A worker without its health endpoint would be restarted as unhealthy:
    # fail at once, before connecting to NATS.
    started = threading.Event()
    try:
        health = start_health_server(
            settings.health_port,
            # A worker whose tool processes cannot be isolated is not ready (KI-96 D12).
            lambda: consumer.ready and not isolation_problem(),
            # Not ready before the consumer starts: starting.
            describe=lambda: isolation_problem() or ("not ready" if started.is_set() else "starting"),
        )
    except (OSError, OverflowError) as exc:
        logger.error("health server failed to start", port=settings.health_port, error=str(exc))
        stop_logging()
        raise SystemExit(1) from exc
    logger.info("health server listening", port=health.server_address[1], ready_path="/health/ready")

    stopping: list[asyncio.Task[None]] = []

    def request_stop() -> None:
        """Signal handler. The request is sticky (see TaskConsumer.request_stop); a
        signal after a finished stop runs stop() again, so no signal is ignored.
        """
        consumer.request_stop()
        if not stopping or stopping[-1].done():
            stopping.append(asyncio.create_task(consumer.stop(), name="stop consumer"))

    loop = asyncio.get_running_loop()
    signals = (signal.SIGINT, signal.SIGTERM)
    for sig in signals:
        loop.add_signal_handler(sig, request_stop)

    crashed = False
    try:
        # start() returns once every loop ended (after a stop request or a
        # give-up), or at once when a stop was requested during its setup.
        try:
            started.set()
            await consumer.start()
        except Exception as exc:  # e.g. NATS unreachable at startup
            crashed = True
            logger.exception("worker stopped by an error", error=str(exc))
        if not stopping:
            request_stop()
        # Awaited, not left to asyncio.run() to cancel: the shutdown drains
        # NATS and flushes the tracing queue. A signal meanwhile may add a stop.
        while not all(task.done() for task in stopping):
            await asyncio.gather(*stopping)
    finally:
        for sig in signals:
            loop.remove_signal_handler(sig)
        await asyncio.to_thread(health.shutdown)
        health.server_close()
        stop_logging()
    if crashed or consumer.failed:
        raise SystemExit(1)


def run() -> None:
    """Run the worker (``python -m codeforge.consumer``) and exit once main() shut it down.

    Threads that still run CPU-bound work (repo map, retrieval index, code
    graph) cannot be cancelled. asyncio.run() would wait for them (the
    executor shutdown, then the interpreter's thread join) and delay the exit
    after a give-up or SIGTERM for as long as the indexing runs (KI-67). The
    process exits without them: their messages are at-least-once and are
    redelivered.
    """
    loop = asyncio.new_event_loop()
    code = 0
    try:
        loop.run_until_complete(main())
    except SystemExit as exc:
        code = exc.code if isinstance(exc.code, int) else 1
    except BaseException:
        traceback.print_exc()
        code = 1
    finally:
        try:
            loop.run_until_complete(loop.shutdown_asyncgens())
        finally:
            loop.close()  # shuts the default executor down without waiting for its threads
    sys.stdout.flush()
    sys.stderr.flush()
    os._exit(code)


if __name__ == "__main__":
    run()


__all__ = ["TaskConsumer", "main", "run"]
