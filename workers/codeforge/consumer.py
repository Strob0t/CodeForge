"""NATS JetStream consumer for receiving tasks from Go Core."""

from __future__ import annotations

import asyncio
import os
import signal
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable

import nats
import structlog

from codeforge.config import WorkerSettings
from codeforge.executor import AgentExecutor
from codeforge.graphrag import CodeGraphBuilder, GraphSearcher
from codeforge.llm import LiteLLMClient
from codeforge.logger import setup_logging, stop_logging
from codeforge.models import (
    GraphBuildRequest,
    GraphBuildResult,
    GraphSearchRequest,
    GraphSearchResult,
    QualityGateRequest,
    QualityGateResult,
    RepoMapRequest,
    RepoMapResult,
    RetrievalIndexRequest,
    RetrievalIndexResult,
    RetrievalSearchRequest,
    RetrievalSearchResult,
    RunStartMessage,
    SubAgentSearchRequest,
    SubAgentSearchResult,
    TaskMessage,
    TaskResult,
)
from codeforge.qualitygate import QualityGateExecutor
from codeforge.repomap import RepoMapGenerator
from codeforge.retrieval import HybridRetriever, RetrievalSubAgent
from codeforge.runtime import RuntimeClient

if TYPE_CHECKING:
    from nats.aio.client import Client as NATSClient
    from nats.js.client import JetStreamContext

STREAM_NAME = "CODEFORGE"
STREAM_SUBJECTS = [
    "tasks.>",
    "agents.>",
    "runs.>",
    "context.>",
    "repomap.>",
    "retrieval.>",
    "graph.>",
]
SUBJECT_AGENT = "tasks.agent.*"
SUBJECT_RESULT = "tasks.result"
SUBJECT_OUTPUT = "tasks.output"
SUBJECT_RUN_START = "runs.start"
SUBJECT_QG_REQUEST = "runs.qualitygate.request"
SUBJECT_QG_RESULT = "runs.qualitygate.result"
SUBJECT_REPOMAP_REQUEST = "repomap.generate.request"
SUBJECT_REPOMAP_RESULT = "repomap.generate.result"
SUBJECT_RETRIEVAL_INDEX_REQUEST = "retrieval.index.request"
SUBJECT_RETRIEVAL_INDEX_RESULT = "retrieval.index.result"
SUBJECT_RETRIEVAL_SEARCH_REQUEST = "retrieval.search.request"
SUBJECT_RETRIEVAL_SEARCH_RESULT = "retrieval.search.result"
SUBJECT_SUBAGENT_SEARCH_REQUEST = "retrieval.subagent.request"
SUBJECT_SUBAGENT_SEARCH_RESULT = "retrieval.subagent.result"
SUBJECT_GRAPH_BUILD_REQUEST = "graph.build.request"
SUBJECT_GRAPH_BUILD_RESULT = "graph.build.result"
SUBJECT_GRAPH_SEARCH_REQUEST = "graph.search.request"
SUBJECT_GRAPH_SEARCH_RESULT = "graph.search.result"
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

        # Ensure the stream exists (idempotent — matches Go Core's CreateOrUpdateStream).
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

    async def _handle_repomap(self, msg: nats.aio.msg.Msg) -> None:
        """Process a repo map request: generate map and publish result."""
        try:
            request = RepoMapRequest.model_validate_json(msg.data)
            log = logger.bind(project_id=request.project_id)
            log.info("received repomap request", workspace=request.workspace_path)

            self._repomap_generator._token_budget = request.token_budget
            result: RepoMapResult = await self._repomap_generator.generate(
                workspace_path=request.workspace_path,
                active_files=request.active_files,
            )
            result = result.model_copy(update={"project_id": request.project_id})

            if self._js is not None:
                await self._js.publish(
                    SUBJECT_REPOMAP_RESULT,
                    result.model_dump_json().encode(),
                )

            await msg.ack()
            log.info(
                "repomap generated",
                files=result.file_count,
                symbols=result.symbol_count,
                tokens=result.token_count,
            )

        except Exception:
            logger.exception("failed to process repomap request")
            await msg.nak()

    async def _handle_retrieval_index(self, msg: nats.aio.msg.Msg) -> None:
        """Process a retrieval index request: build index and publish result."""
        try:
            request = RetrievalIndexRequest.model_validate_json(msg.data)
            log = logger.bind(project_id=request.project_id)
            log.info("received retrieval index request", workspace=request.workspace_path)

            status = await self._retriever.build_index(
                project_id=request.project_id,
                workspace_path=request.workspace_path,
                embedding_model=request.embedding_model,
                file_extensions=request.file_extensions or None,
            )

            result = RetrievalIndexResult(
                project_id=status.project_id,
                status=status.status,
                file_count=status.file_count,
                chunk_count=status.chunk_count,
                embedding_model=status.embedding_model,
                error=status.error,
            )

            if self._js is not None:
                await self._js.publish(
                    SUBJECT_RETRIEVAL_INDEX_RESULT,
                    result.model_dump_json().encode(),
                )

            await msg.ack()
            log.info(
                "retrieval index built",
                status=result.status,
                files=result.file_count,
                chunks=result.chunk_count,
            )

        except Exception:
            logger.exception("failed to process retrieval index request")
            await msg.nak()

    async def _handle_retrieval_search(self, msg: nats.aio.msg.Msg) -> None:
        """Process a retrieval search request: search index and publish result."""
        try:
            request = RetrievalSearchRequest.model_validate_json(msg.data)
            log = logger.bind(project_id=request.project_id, request_id=request.request_id)
            log.info("received retrieval search request", query=request.query[:80])

            hits = await self._retriever.search(
                project_id=request.project_id,
                query=request.query,
                top_k=request.top_k,
                bm25_weight=request.bm25_weight,
                semantic_weight=request.semantic_weight,
            )

            result = RetrievalSearchResult(
                project_id=request.project_id,
                query=request.query,
                request_id=request.request_id,
                results=hits,
            )

            if self._js is not None:
                await self._js.publish(
                    SUBJECT_RETRIEVAL_SEARCH_RESULT,
                    result.model_dump_json().encode(),
                )

            await msg.ack()
            log.info("retrieval search completed", hits=len(hits))

        except Exception:
            logger.exception("failed to process retrieval search request")
            await self._publish_error_result(
                msg,
                RetrievalSearchRequest,
                RetrievalSearchResult,
                SUBJECT_RETRIEVAL_SEARCH_RESULT,
            )

    async def _handle_subagent_search(self, msg: nats.aio.msg.Msg) -> None:
        """Process a sub-agent search request: expand, search, dedup, rerank, publish."""
        try:
            request = SubAgentSearchRequest.model_validate_json(msg.data)
            log = logger.bind(project_id=request.project_id, request_id=request.request_id)
            log.info("received subagent search request", query=request.query[:80])

            hits, expanded_queries, total_candidates = await self._subagent.search(
                project_id=request.project_id,
                query=request.query,
                top_k=request.top_k,
                max_queries=request.max_queries,
                model=request.model,
                rerank=request.rerank,
            )

            result = SubAgentSearchResult(
                project_id=request.project_id,
                query=request.query,
                request_id=request.request_id,
                results=hits,
                expanded_queries=expanded_queries,
                total_candidates=total_candidates,
            )

            if self._js is not None:
                await self._js.publish(
                    SUBJECT_SUBAGENT_SEARCH_RESULT,
                    result.model_dump_json().encode(),
                )

            await msg.ack()
            log.info(
                "subagent search completed",
                hits=len(hits),
                queries=len(expanded_queries),
                candidates=total_candidates,
            )

        except Exception:
            logger.exception("failed to process subagent search request")
            await self._publish_error_result(
                msg,
                SubAgentSearchRequest,
                SubAgentSearchResult,
                SUBJECT_SUBAGENT_SEARCH_RESULT,
            )

    async def _handle_graph_build(self, msg: nats.aio.msg.Msg) -> None:
        """Process a graph build request: build code graph and publish result."""
        try:
            request = GraphBuildRequest.model_validate_json(msg.data)
            log = logger.bind(project_id=request.project_id)
            log.info("received graph build request", workspace=request.workspace_path)

            result: GraphBuildResult = await self._graph_builder.build_graph(
                project_id=request.project_id,
                workspace_path=request.workspace_path,
                db_url=self._db_url,
            )

            if self._js is not None:
                await self._js.publish(
                    SUBJECT_GRAPH_BUILD_RESULT,
                    result.model_dump_json().encode(),
                )

            await msg.ack()
            log.info(
                "graph build completed",
                status=result.status,
                nodes=result.node_count,
                edges=result.edge_count,
            )

        except Exception:
            logger.exception("failed to process graph build request")
            await msg.nak()

    async def _handle_graph_search(self, msg: nats.aio.msg.Msg) -> None:
        """Process a graph search request: search graph and publish result."""
        try:
            request = GraphSearchRequest.model_validate_json(msg.data)
            log = logger.bind(project_id=request.project_id, request_id=request.request_id)
            log.info("received graph search request", seeds=request.seed_symbols)

            hits = await self._graph_searcher.search(
                project_id=request.project_id,
                seed_symbols=request.seed_symbols,
                max_hops=request.max_hops,
                top_k=request.top_k,
                db_url=self._db_url,
            )

            result = GraphSearchResult(
                project_id=request.project_id,
                request_id=request.request_id,
                results=hits,
            )

            if self._js is not None:
                await self._js.publish(
                    SUBJECT_GRAPH_SEARCH_RESULT,
                    result.model_dump_json().encode(),
                )

            await msg.ack()
            log.info("graph search completed", hits=len(hits))

        except Exception:
            logger.exception("failed to process graph search request")
            await self._publish_graph_search_error(msg)

    async def _publish_graph_search_error(self, msg: nats.aio.msg.Msg) -> None:
        """Publish an error result for graph search so the Go waiter gets a response."""
        try:
            req = GraphSearchRequest.model_validate_json(msg.data)
            error_result = GraphSearchResult(
                project_id=req.project_id,
                request_id=req.request_id,
                error="internal worker error",
            )
            if self._js is not None:
                await self._js.publish(
                    SUBJECT_GRAPH_SEARCH_RESULT,
                    error_result.model_dump_json().encode(),
                )
        except Exception:
            logger.exception("failed to publish graph search error result")
        await msg.nak()

    async def _publish_error_result(
        self,
        msg: nats.aio.msg.Msg,
        request_model: type,
        result_model: type,
        subject: str,
    ) -> None:
        """Publish an error result so the Go waiter gets an immediate response, then nak."""
        try:
            req = request_model.model_validate_json(msg.data)
            error_result = result_model(
                project_id=req.project_id,
                query=req.query,
                request_id=req.request_id,
                error="internal worker error",
            )
            if self._js is not None:
                await self._js.publish(subject, error_result.model_dump_json().encode())
        except Exception:
            logger.exception("failed to publish error result", subject=subject)
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

        await self._js.publish(SUBJECT_OUTPUT, payload.encode(), headers=headers or None)

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
