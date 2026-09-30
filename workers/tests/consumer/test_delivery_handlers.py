"""Delivery semantics of the worker's message handlers (KI-18, KI-19, ADR-016).

Every handler dead-letters and terminates an invalid payload, at-least-once
handlers retry a failure until the last JetStream delivery, and runs are acked
on accept and release their per-run NATS subscriptions when they end.
Messages are real ``nats.aio.msg.Msg`` objects, see ``tests.jetstream_fakes``.
"""

from __future__ import annotations

import asyncio
import json
import threading
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.config import get_settings
from codeforge.consumer import TaskConsumer
from codeforge.models import ConversationRunStartMessage, RunStartMessage, TaskMessage, TerminationConfig
from codeforge.runtime import RuntimeClient
from tests.jetstream_fakes import RecordingJetStream, jetstream_msg

if TYPE_CHECKING:
    from pathlib import Path

MAX_DELIVER = 4  # JetStream delivery limit: first delivery plus 3 retries


@pytest.fixture
def consumer() -> TaskConsumer:
    worker = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    worker._js = RecordingJetStream()  # type: ignore[assignment]
    return worker


def _published(worker: TaskConsumer) -> list[tuple[str, bytes]]:
    return worker._js.published  # type: ignore[union-attr]


# Every subscription of TaskConsumer.start with its subject.
HANDLERS = [
    ("_handle_message", "tasks.agent.aider"),
    ("_handle_run_start", "runs.start"),
    ("_handle_quality_gate", "runs.qualitygate.request"),
    ("_handle_repomap", "repomap.generate.request"),
    ("_handle_retrieval_index", "retrieval.index.request"),
    ("_handle_retrieval_search", "retrieval.search.request"),
    ("_handle_subagent_search", "retrieval.subagent.request"),
    ("_handle_graph_build", "graph.build.request"),
    ("_handle_graph_search", "graph.search.request"),
    ("_handle_context_rerank", "context.rerank.request"),
    ("_handle_conversation_run", "conversation.run.start"),
    ("_handle_conversation_compact", "conversation.compact.request"),
    ("_handle_benchmark_run", "benchmark.run.request"),
    ("_handle_gemmas_eval", "evaluation.gemmas.request"),
    ("_handle_memory_store", "memory.store"),
    ("_handle_memory_recall", "memory.recall"),
    ("_handle_handoff_request", "handoff.request"),
    ("_handle_a2a_task_created", "a2a.task.created"),
    ("_handle_a2a_task_cancel", "a2a.task.cancel"),
    ("_handle_backend_health", "backends.health.request"),
    ("_handle_review_trigger", "review.trigger.request"),
    ("_handle_prompt_evolution_reflect", "prompt.evolution.reflect"),
    ("_handle_prompt_promoted", "prompt.evolution.promoted"),
    ("_handle_prompt_reverted", "prompt.evolution.reverted"),
    ("_handle_shared_context_updated", "context.shared.updated"),
]


@pytest.mark.parametrize(("handler_name", "subject"), HANDLERS)
@pytest.mark.parametrize("payload", [b"not json", b"[1, 2]"])
async def test_invalid_payload_is_dead_lettered_and_terminated(
    consumer: TaskConsumer,
    monkeypatch: pytest.MonkeyPatch,
    handler_name: str,
    subject: str,
    payload: bytes,
) -> None:
    """An invalid payload goes to {subject}.dlq at once and is never NAK'd or redelivered."""
    monkeypatch.setenv("APP_ENV", "development")  # benchmark runs are only accepted in dev mode
    get_settings.cache_clear()
    monkeypatch.setattr("codeforge.consumer._benchmark._wait_for_litellm", AsyncMock(return_value=True))
    msg, client = jetstream_msg(payload, subject=subject)

    await getattr(consumer, handler_name)(msg)

    assert _published(consumer) == [(f"{subject}.dlq", payload)]
    assert client.settlements() == ["term"]


def test_handler_table_covers_every_subscription() -> None:
    """HANDLERS must list every handler that TaskConsumer.start subscribes."""
    import inspect

    import codeforge.consumer as consumer_module

    source = inspect.getsource(consumer_module.TaskConsumer.start)
    subscribed = {name for name, _ in HANDLERS if f"self.{name})" in source}
    assert subscribed == {name for name, _ in HANDLERS}
    assert source.count("self._handle") == len(HANDLERS)


# ---------------------------------------------------------------------------
# tasks.agent.* (at-most-once: backend tasks change the workspace)
# ---------------------------------------------------------------------------


def _task_payload(task_id: str = "task-1") -> bytes:
    return TaskMessage(id=task_id, project_id="p1", title="t", prompt="do it").model_dump_json().encode()


def _task_results(worker: TaskConsumer) -> list[dict[str, object]]:
    return [json.loads(data) for subject, data in _published(worker) if subject == "tasks.result"]


class TestTasks:
    async def test_acked_on_accept_before_execution(self, consumer: TaskConsumer) -> None:
        from codeforge.backends._base import TaskResult as BackendTaskResult

        msg, client = jetstream_msg(_task_payload(), subject="tasks.agent.aider")
        acked_during_run: list[bool] = []

        async def execute(**_kwargs: object) -> BackendTaskResult:
            acked_during_run.append(msg.is_acked)
            return BackendTaskResult(status="completed", output="ok")

        consumer._backend_router = MagicMock()
        consumer._backend_router.execute = execute

        await consumer._handle_message(msg)

        assert acked_during_run == [True]
        assert client.settlements() == ["ack(sync)"]
        assert [r["status"] for r in _task_results(consumer)] == ["completed"]

    async def test_failure_reports_a_failed_result_and_is_not_retried(self, consumer: TaskConsumer) -> None:
        """A half-applied backend task (Aider, OpenHands, ...) must not run again on its workspace."""
        consumer._backend_router = MagicMock()
        consumer._backend_router.execute = AsyncMock(side_effect=RuntimeError("backend down"))

        first, first_client = jetstream_msg(_task_payload(), subject="tasks.agent.aider", num_delivered=1)
        await consumer._handle_message(first)
        redelivered, redelivered_client = jetstream_msg(_task_payload(), subject="tasks.agent.aider", num_delivered=2)
        await consumer._handle_message(redelivered)

        assert consumer._backend_router.execute.await_count == 1
        assert first_client.settlements() == ["ack(sync)"]
        assert redelivered_client.settlements() == ["ack"], "the redelivery of an accepted task is a duplicate"
        results = _task_results(consumer)
        assert [(r["task_id"], r["status"]) for r in results] == [("task-1", "failed")]
        assert "backend down" in str(results[0]["error"])
        assert not any(subject.endswith(".dlq") for subject, _ in _published(consumer))

    async def test_unconfirmed_accept_does_not_start_the_task(self, consumer: TaskConsumer) -> None:
        consumer._backend_router = MagicMock()
        consumer._backend_router.execute = AsyncMock()
        msg, client = jetstream_msg(_task_payload(), subject="tasks.agent.aider")
        client.fail_requests = True

        await consumer._handle_message(msg)

        consumer._backend_router.execute.assert_not_awaited()
        assert client.settlements() == []
        assert _published(consumer) == []


# ---------------------------------------------------------------------------
# runs.start (at-most-once, ack on accept)
# ---------------------------------------------------------------------------


def _run_start_payload(run_id: str = "run-1") -> bytes:
    return (
        RunStartMessage(run_id=run_id, task_id="task-1", project_id="p1", agent_id="a1", prompt="fix it")
        .model_dump_json()
        .encode()
    )


class TestRunStart:
    async def test_acked_on_accept_before_execution(self, consumer: TaskConsumer) -> None:
        msg, client = jetstream_msg(_run_start_payload(), subject="runs.start")
        acked_during_run: list[bool] = []

        async def execute(*_args: object, **_kwargs: object) -> None:
            acked_during_run.append(msg.is_acked)

        consumer._executor = MagicMock()
        consumer._executor.execute_with_runtime = execute

        await consumer._handle_run_start(msg)

        assert acked_during_run == [True]
        assert client.settlements() == ["ack(sync)"]

    async def test_failed_run_is_not_redelivered(self, consumer: TaskConsumer) -> None:
        msg, client = jetstream_msg(_run_start_payload(), subject="runs.start")
        consumer._executor = MagicMock()
        consumer._executor.execute_with_runtime = AsyncMock(side_effect=RuntimeError("boom"))

        await consumer._handle_run_start(msg)

        assert client.settlements() == ["ack(sync)"]
        assert not any(subject.endswith(".dlq") for subject, _ in _published(consumer))

    async def test_failure_after_accept_reports_a_failed_run(self, consumer: TaskConsumer) -> None:
        """An exception escaping the run must end it as failed, not leave it running until a timeout."""
        msg, _ = jetstream_msg(_run_start_payload("run-err"), subject="runs.start")
        consumer._executor = MagicMock()
        consumer._executor.execute_with_runtime = AsyncMock(side_effect=RuntimeError("boom"))

        await consumer._handle_run_start(msg)

        completions = [json.loads(data) for subject, data in _published(consumer) if subject == "runs.complete"]
        assert [(c["run_id"], c["status"]) for c in completions] == [("run-err", "failed")]
        assert "boom" in completions[0]["error"]

    async def test_cancel_listener_failure_reports_a_failed_run(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        msg, _ = jetstream_msg(_run_start_payload("run-sub"), subject="runs.start")
        monkeypatch.setattr(consumer._js, "subscribe", AsyncMock(side_effect=ConnectionError("nats down")))
        consumer._executor = MagicMock()
        consumer._executor.execute_with_runtime = AsyncMock()

        await consumer._handle_run_start(msg)

        consumer._executor.execute_with_runtime.assert_not_awaited()
        completions = [json.loads(data) for subject, data in _published(consumer) if subject == "runs.complete"]
        assert [(c["run_id"], c["status"]) for c in completions] == [("run-sub", "failed")]

    async def test_completed_run_is_not_reported_twice(self, consumer: TaskConsumer) -> None:
        msg, _ = jetstream_msg(_run_start_payload("run-done"), subject="runs.start")

        async def complete_then_fail(_task: object, runtime: RuntimeClient, **_kwargs: object) -> None:
            await runtime.complete_run(status="completed", output="done")
            raise RuntimeError("cleanup failed")

        consumer._executor = MagicMock()
        consumer._executor.execute_with_runtime = complete_then_fail

        await consumer._handle_run_start(msg)

        completions = [json.loads(data) for subject, data in _published(consumer) if subject == "runs.complete"]
        assert [(c["run_id"], c["status"]) for c in completions] == [("run-done", "completed")]

    async def test_unconfirmed_accept_does_not_start_the_run(self, consumer: TaskConsumer) -> None:
        msg, client = jetstream_msg(_run_start_payload(), subject="runs.start")
        client.fail_requests = True
        consumer._executor = MagicMock()
        consumer._executor.execute_with_runtime = AsyncMock()

        await consumer._handle_run_start(msg)

        consumer._executor.execute_with_runtime.assert_not_awaited()
        assert client.settlements() == []
        assert _published(consumer) == []

    @pytest.mark.parametrize("fails", [False, True])
    async def test_cancel_listeners_are_released_when_the_run_ends(self, consumer: TaskConsumer, fails: bool) -> None:
        msg, _ = jetstream_msg(_run_start_payload(), subject="runs.start")
        consumer._executor = MagicMock()
        consumer._executor.execute_with_runtime = AsyncMock(side_effect=RuntimeError("boom") if fails else None)

        await consumer._handle_run_start(msg)

        subs = consumer._js.subscriptions  # type: ignore[union-attr]
        assert [s.subject for s in subs] == ["runs.cancel", "tasks.cancel"]
        assert all(s.unsubscribed for s in subs)


# ---------------------------------------------------------------------------
# conversation.run.start (at-most-once, ack on accept)
# ---------------------------------------------------------------------------


def _conversation_payload(run_id: str = "conv-1") -> bytes:
    return (
        ConversationRunStartMessage(
            run_id=run_id, conversation_id=run_id, project_id="p1", messages=[], system_prompt="s", model="m"
        )
        .model_dump_json()
        .encode()
    )


class TestConversationRun:
    async def test_acked_on_accept_before_execution(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        msg, client = jetstream_msg(_conversation_payload(), subject="conversation.run.start")
        acked_during_run: list[bool] = []

        async def run(*_args: object) -> None:
            acked_during_run.append(msg.is_acked)

        monkeypatch.setattr(consumer, "_run_conversation", run)
        await consumer._handle_conversation_run(msg)

        assert acked_during_run == [True]
        assert client.settlements() == ["ack(sync)"]

    async def test_unconfirmed_accept_does_not_start_the_run(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        msg, client = jetstream_msg(_conversation_payload(), subject="conversation.run.start")
        client.fail_requests = True
        run = AsyncMock()
        monkeypatch.setattr(consumer, "_run_conversation", run)

        await consumer._handle_conversation_run(msg)

        run.assert_not_awaited()
        assert client.settlements() == []
        assert _published(consumer) == []
        assert "conv-1" not in consumer._active_runs, "the redelivery must be accepted"

    async def test_failure_publishes_failed_completion_and_is_not_retried(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        msg, client = jetstream_msg(_conversation_payload("conv-err"), subject="conversation.run.start")
        monkeypatch.setattr(consumer, "_run_conversation", AsyncMock(side_effect=RuntimeError("loop crashed")))

        await consumer._handle_conversation_run(msg)

        completions = [
            json.loads(data) for subject, data in _published(consumer) if subject == "conversation.run.complete"
        ]
        assert [(c["run_id"], c["status"]) for c in completions] == [("conv-err", "failed")]
        assert client.settlements() == ["ack(sync)"]

    async def test_runtime_is_closed_when_the_run_fails(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        msg, _ = jetstream_msg(_conversation_payload(), subject="conversation.run.start")
        monkeypatch.setattr("codeforge.tools.build_default_registry", MagicMock)
        monkeypatch.setattr(consumer, "_build_conversation_messages", AsyncMock(side_effect=RuntimeError("no model")))

        await consumer._handle_conversation_run(msg)

        subs = consumer._js.subscriptions  # type: ignore[union-attr]
        assert [s.subject for s in subs] == ["runs.cancel", "conversation.run.cancel"]
        assert all(s.unsubscribed for s in subs)


# ---------------------------------------------------------------------------
# RuntimeClient per-run subscriptions (KI-18)
# ---------------------------------------------------------------------------


class TestRuntimeClose:
    async def test_close_releases_cancel_listeners_and_heartbeat(self) -> None:
        js = RecordingJetStream()
        runtime = RuntimeClient(js=js, run_id="r1", task_id="t1", project_id="p1", termination=TerminationConfig())  # type: ignore[arg-type]
        await runtime.start_cancel_listener(extra_subjects=["conversation.run.cancel"])
        await runtime.start_heartbeat(interval=0.01)
        await asyncio.sleep(0.03)

        await runtime.close()
        beats = sum(1 for subject, _ in js.published if subject == "runs.heartbeat")
        await asyncio.sleep(0.05)

        assert [s.subject for s in js.subscriptions] == ["runs.cancel", "conversation.run.cancel"]
        assert all(s.unsubscribed for s in js.subscriptions)
        assert sum(1 for subject, _ in js.published if subject == "runs.heartbeat") == beats
        await runtime.close()  # idempotent


# ---------------------------------------------------------------------------
# conversation.compact.request (settled exactly once)
# ---------------------------------------------------------------------------


class TestCompact:
    async def test_missing_conversation_id_is_dead_lettered(self, consumer: TaskConsumer) -> None:
        payload = b'{"tenant_id": "t1"}'
        msg, client = jetstream_msg(payload, subject="conversation.compact.request")

        await consumer._handle_conversation_compact(msg)

        assert _published(consumer) == [("conversation.compact.request.dlq", payload)]
        assert client.settlements() == ["term"]

    async def test_nothing_to_compact_is_acked_once(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        msg, client = jetstream_msg(b'{"conversation_id": "c1"}', subject="conversation.compact.request")
        monkeypatch.setattr(consumer, "_fetch_conversation_messages", AsyncMock(return_value=[]))

        await consumer._handle_conversation_compact(msg)

        assert client.settlements() == ["ack"]


# ---------------------------------------------------------------------------
# Message loop resilience
# ---------------------------------------------------------------------------


async def test_message_loop_survives_a_handler_error(consumer: TaskConsumer) -> None:
    consumer._running = True
    first, _ = jetstream_msg(b"{}")
    second, _ = jetstream_msg(b"{}")
    batches: list[list[object]] = [[first], [second]]
    handled: list[object] = []

    async def fetch(**_kwargs: object) -> list[object]:
        if batches:
            return batches.pop(0)
        consumer._running = False
        raise TimeoutError

    async def handler(msg: object) -> None:
        handled.append(msg)
        if msg is first:
            raise RuntimeError("unexpected")

    sub = MagicMock()
    sub.fetch = fetch
    await consumer._message_loop(sub, handler, "test.request")

    assert handled == [first, second]


# ---------------------------------------------------------------------------
# benchmark.run.request (at-most-once, runs in the background)
# ---------------------------------------------------------------------------


def _benchmark_payload() -> bytes:
    from codeforge.models import BenchmarkRunRequest

    return BenchmarkRunRequest(run_id="bench-1", dataset_path="/data/b.yaml", model="m").model_dump_json().encode()


class TestBenchmarkRun:
    @pytest.fixture(autouse=True)
    def _dev_mode(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("APP_ENV", "development")
        get_settings.cache_clear()
        monkeypatch.setattr("codeforge.consumer._benchmark._wait_for_litellm", AsyncMock(return_value=True))
        monkeypatch.setattr("codeforge.consumer._benchmark._validate_model_exists", AsyncMock())

    async def test_acked_on_accept_before_the_run_starts(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        msg, client = jetstream_msg(_benchmark_payload(), subject="benchmark.run.request")
        started = asyncio.Event()
        acked_at_start: list[bool] = []

        async def execute(*_args: object) -> None:
            acked_at_start.append(msg.is_acked)
            started.set()

        monkeypatch.setattr(consumer, "_execute_benchmark_run", execute)
        await consumer._handle_benchmark_run(msg)
        await asyncio.wait_for(started.wait(), timeout=1)

        assert acked_at_start == [True]
        assert client.settlements() == ["ack(sync)"]

    async def test_unconfirmed_accept_does_not_start_the_run(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        msg, client = jetstream_msg(_benchmark_payload(), subject="benchmark.run.request")
        client.fail_requests = True
        execute = AsyncMock()
        monkeypatch.setattr(consumer, "_execute_benchmark_run", execute)

        await consumer._handle_benchmark_run(msg)
        await asyncio.sleep(0.02)

        execute.assert_not_awaited()
        assert client.settlements() == []


# ---------------------------------------------------------------------------
# Request/response handlers: an error result settles the request
# ---------------------------------------------------------------------------


def _fail_rerank(worker: TaskConsumer, monkeypatch: pytest.MonkeyPatch) -> AsyncMock:
    rerank = AsyncMock(side_effect=RuntimeError("llm down"))

    class _FailingReranker:
        def __init__(self, **_kwargs: object) -> None:
            self.rerank = rerank

    monkeypatch.setattr("codeforge.consumer._context.ContextReranker", _FailingReranker)
    return rerank


def _fail_retrieval_search(worker: TaskConsumer, monkeypatch: pytest.MonkeyPatch) -> AsyncMock:
    search = AsyncMock(side_effect=RuntimeError("llm down"))
    monkeypatch.setattr(worker._retriever, "search", search)
    return search


def _fail_subagent_search(worker: TaskConsumer, monkeypatch: pytest.MonkeyPatch) -> AsyncMock:
    search = AsyncMock(side_effect=RuntimeError("llm down"))
    monkeypatch.setattr(worker._subagent, "search", search)
    return search


def _fail_graph_search(worker: TaskConsumer, monkeypatch: pytest.MonkeyPatch) -> AsyncMock:
    search = AsyncMock(side_effect=RuntimeError("db down"))
    monkeypatch.setattr(worker._graph_searcher, "search", search)
    return search


ERROR_RESULT_HANDLERS = [
    (
        "_handle_context_rerank",
        "context.rerank.request",
        "context.rerank.result",
        b'{"request_id": "q1", "project_id": "p1", "query": "auth", "entries": []}',
        _fail_rerank,
    ),
    (
        "_handle_retrieval_search",
        "retrieval.search.request",
        "retrieval.search.result",
        b'{"project_id": "p1", "query": "auth", "request_id": "q2"}',
        _fail_retrieval_search,
    ),
    (
        "_handle_subagent_search",
        "retrieval.subagent.request",
        "retrieval.subagent.result",
        b'{"project_id": "p1", "query": "auth", "request_id": "q3"}',
        _fail_subagent_search,
    ),
    (
        "_handle_graph_search",
        "graph.search.request",
        "graph.search.result",
        b'{"project_id": "p1", "request_id": "q4", "seed_symbols": ["main"]}',
        _fail_graph_search,
    ),
]


@pytest.mark.parametrize(
    ("handler_name", "subject", "result_subject", "payload", "make_failing"), ERROR_RESULT_HANDLERS
)
async def test_error_result_settles_the_request(
    consumer: TaskConsumer,
    monkeypatch: pytest.MonkeyPatch,
    handler_name: str,
    subject: str,
    result_subject: str,
    payload: bytes,
    make_failing: object,
) -> None:
    """Once the Go waiter got an error result, repeating the (LLM) work would only cost money."""
    failing = make_failing(consumer, monkeypatch)  # type: ignore[operator]
    msg, client = jetstream_msg(payload, subject=subject)

    await getattr(consumer, handler_name)(msg)

    results = [json.loads(data) for published, data in _published(consumer) if published == result_subject]
    assert len(results) == 1
    assert results[0]["error"]
    assert client.settlements() == ["ack"]
    assert failing.await_count == 1


# ---------------------------------------------------------------------------
# Per-run cancel listeners
# ---------------------------------------------------------------------------


async def _wait_until_cancelled(runtime: RuntimeClient) -> bool:
    for _ in range(100):
        if runtime.is_cancelled:
            return True
        await asyncio.sleep(0.01)
    return runtime.is_cancelled


class TestRuntimeCancelListener:
    async def test_malformed_cancel_messages_do_not_stop_the_listener(self) -> None:
        """One bad message on runs.cancel must not disable cancellation for every active run."""
        js = RecordingJetStream()
        runtime = RuntimeClient(js=js, run_id="r1", task_id="t1", project_id="p1", termination=TerminationConfig())  # type: ignore[arg-type]
        await runtime.start_cancel_listener()
        sub = js.subscriptions[0]

        for bad in (b"null", b"[]", b"not json", b"\xff\xfe", b'"r1"', b'{"run_id": "other"}'):
            sub.deliver(bad)
        await asyncio.sleep(0.1)
        assert not runtime.is_cancelled

        sub.deliver(b'{"run_id": "r1"}')
        assert await _wait_until_cancelled(runtime)
        await runtime.close()

    async def test_empty_ids_never_match(self) -> None:
        """A run without a task ID must not be cancelled by a cancel message for 'no task'."""
        js = RecordingJetStream()
        runtime = RuntimeClient(js=js, run_id="r1", task_id="", project_id="p1", termination=TerminationConfig())  # type: ignore[arg-type]
        await runtime.start_cancel_listener(extra_subjects=["tasks.cancel"])

        js.subscriptions[1].deliver(b'{"task_id": ""}')
        js.subscriptions[0].deliver(b'{"run_id": "", "task_id": ""}')
        await asyncio.sleep(0.1)

        assert not runtime.is_cancelled
        await runtime.close()


# ---------------------------------------------------------------------------
# CPU-bound indexing must not block the event loop (heartbeats, other loops)
# ---------------------------------------------------------------------------


def _record_thread(threads: list[bool], result: object) -> object:
    def record(*_args: object, **_kwargs: object) -> object:
        threads.append(threading.current_thread() is threading.main_thread())
        return result

    return record


class TestIndexingRunsOffTheEventLoop:
    async def test_repo_map_is_generated_in_a_worker_thread(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        from codeforge.repomap import RepoMapGenerator

        (tmp_path / "a.py").write_text("def f():\n    return 1\n")
        generator = RepoMapGenerator()
        on_main_thread: list[bool] = []
        monkeypatch.setattr(generator, "_collect_files", _record_thread(on_main_thread, []))

        await generator.generate(str(tmp_path))

        assert on_main_thread == [False]

    async def test_retrieval_chunking_runs_in_a_worker_thread(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        from codeforge.retrieval import HybridRetriever

        retriever = HybridRetriever()
        on_main_thread: list[bool] = []
        monkeypatch.setattr(retriever._chunker, "chunk_workspace_by_file", _record_thread(on_main_thread, {}))

        status = await retriever.build_index(project_id="p1", workspace_path=str(tmp_path))

        assert status.status == "empty"
        assert on_main_thread == [False]

    async def test_graph_extraction_runs_in_a_worker_thread(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        from codeforge.graphrag import CodeGraphBuilder

        builder = CodeGraphBuilder()
        on_main_thread: list[bool] = []
        monkeypatch.setattr(builder, "_collect_files", _record_thread(on_main_thread, []))

        result = await builder.build_graph(project_id="p1", workspace_path=str(tmp_path), db_url="postgresql://unused")

        assert result.status == "ready"
        assert on_main_thread == [False]
