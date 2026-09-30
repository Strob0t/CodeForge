"""Delivery semantics of the worker's message handlers (KI-18, KI-19, ADR-016).

Every handler dead-letters and terminates an invalid payload, at-least-once
handlers retry a failure until the last JetStream delivery, and runs are acked
on accept and release their per-run NATS subscriptions when they end.
Messages are real ``nats.aio.msg.Msg`` objects, see ``tests.jetstream_fakes``.
"""

from __future__ import annotations

import asyncio
import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.config import get_settings
from codeforge.consumer import TaskConsumer
from codeforge.models import ConversationRunStartMessage, RunStartMessage, TaskMessage, TerminationConfig
from codeforge.runtime import RuntimeClient
from tests.jetstream_fakes import RecordingJetStream, jetstream_msg

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
# tasks.agent.* (at-least-once with retries)
# ---------------------------------------------------------------------------


def _task_payload(task_id: str = "task-1") -> bytes:
    return TaskMessage(id=task_id, project_id="p1", title="t", prompt="do it").model_dump_json().encode()


class TestTaskRetries:
    async def test_failure_is_retried_and_redelivery_is_processed_again(self, consumer: TaskConsumer) -> None:
        from codeforge.backends._base import TaskResult as BackendTaskResult

        consumer._backend_router = MagicMock()
        consumer._backend_router.execute = AsyncMock(
            side_effect=[RuntimeError("backend down"), BackendTaskResult(status="completed", output="ok")]
        )

        first, first_client = jetstream_msg(_task_payload(), subject="tasks.agent.aider", num_delivered=1)
        await consumer._handle_message(first)
        second, second_client = jetstream_msg(_task_payload(), subject="tasks.agent.aider", num_delivered=2)
        await consumer._handle_message(second)

        assert first_client.settlements() == ["nak(2s)"]
        assert second_client.settlements() == ["ack"], "a failed task must not be skipped as a duplicate"
        assert consumer._backend_router.execute.await_count == 2

    async def test_last_attempt_is_dead_lettered(self, consumer: TaskConsumer) -> None:
        consumer._backend_router = MagicMock()
        consumer._backend_router.execute = AsyncMock(side_effect=RuntimeError("backend down"))
        payload = _task_payload()
        msg, client = jetstream_msg(payload, subject="tasks.agent.aider", num_delivered=MAX_DELIVER)

        await consumer._handle_message(msg)

        assert ("tasks.agent.aider.dlq", payload) in _published(consumer)
        assert client.settlements() == ["ack"]


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
        assert client.settlements() == ["ack"]

    async def test_failed_run_is_not_redelivered(self, consumer: TaskConsumer) -> None:
        msg, client = jetstream_msg(_run_start_payload(), subject="runs.start")
        consumer._executor = MagicMock()
        consumer._executor.execute_with_runtime = AsyncMock(side_effect=RuntimeError("boom"))

        await consumer._handle_run_start(msg)

        assert client.settlements() == ["ack"]
        assert not any(subject.endswith(".dlq") for subject, _ in _published(consumer))

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
        assert client.settlements() == ["ack"]

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
        assert client.settlements() == ["ack"]

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
