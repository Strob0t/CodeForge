"""Delivery semantics of the worker's message handlers (KI-18, KI-19, ADR-016).

Every handler dead-letters and terminates an invalid payload, at-least-once
handlers retry a failure until the last JetStream delivery, and runs are acked
on accept and release their per-run NATS subscriptions when they end.
Messages are real ``nats.aio.msg.Msg`` objects, see ``tests.jetstream_fakes``.
"""

from __future__ import annotations

import asyncio
import json
import os
import threading
import time
from types import SimpleNamespace
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock

import nats.errors
import pytest
from nats.js.api import AckPolicy, DeliverPolicy

from codeforge.config import get_settings
from codeforge.consumer import TaskConsumer
from codeforge.models import (
    AgentLoopResult,
    ConversationRunStartMessage,
    RunStartMessage,
    TaskMessage,
    TerminationConfig,
)
from codeforge.runtime import RuntimeClient
from tests.jetstream_fakes import RecordingJetStream, jetstream_msg, patch_notification_hub

if TYPE_CHECKING:
    from pathlib import Path

MAX_DELIVER = 4  # JetStream delivery limit: first delivery plus 3 retries


@pytest.fixture
def consumer() -> TaskConsumer:
    worker = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    worker._js = RecordingJetStream()  # type: ignore[assignment]
    worker._notifications = worker._js
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
    ("_handle_workspace_test", "conversation.test.request"),
    ("_handle_workspace_delete", "workspace.delete.request"),
    ("_handle_benchmark_run", "benchmark.run.request"),
    ("_handle_gemmas_eval", "evaluation.gemmas.request"),
    ("_handle_memory_store", "memory.store"),
    ("_handle_memory_recall", "memory.recall"),
    ("_handle_a2a_task_created", "a2a.task.created"),
    ("_handle_a2a_task_cancel", "a2a.task.cancel"),
    ("_handle_backend_health", "backends.health.request"),
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
    """HANDLERS must list every handler that TaskConsumer._subscribe_all subscribes."""
    import inspect

    import codeforge.consumer as consumer_module

    source = inspect.getsource(consumer_module.TaskConsumer._subscribe_all)
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
        assert client.settlements() == ["nak"], "an unconfirmed accept releases the message"
        assert _published(consumer) == []

    @pytest.mark.parametrize("fails", [False, True])
    async def test_heartbeats_while_the_task_runs(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch, fails: bool
    ) -> None:
        """The Go Core fails a task whose heartbeats stop (KI-65): a running task sends them, an ended one not."""
        from codeforge.backends._base import TaskResult as BackendTaskResult

        monkeypatch.setattr("codeforge.consumer._tasks.heartbeat_interval", lambda _seconds: 0.01)
        payload = TaskMessage(
            id="task-beat", project_id="p1", tenant_id="tenant-1", title="t", prompt="do it", dispatch_id="d-1"
        )
        msg, _ = jetstream_msg(payload.model_dump_json().encode(), subject="tasks.agent.aider")

        def beats() -> list[dict[str, object]]:
            return [json.loads(d) for s, d in _published(consumer) if s == "tasks.heartbeat"]

        async def execute(**_kwargs: object) -> BackendTaskResult:
            await asyncio.sleep(0.05)
            if fails:
                raise RuntimeError("backend down")
            return BackendTaskResult(status="completed", output="ok")

        consumer._backend_router = MagicMock()
        consumer._backend_router.execute = execute

        await consumer._handle_message(msg)
        sent = beats()
        await asyncio.sleep(0.05)

        assert len(sent) >= 2, "heartbeats are sent while the task runs"
        # The heartbeats name the dispatch (S2-F review, F7).
        assert {(b["task_id"], b["tenant_id"], b["dispatch_id"]) for b in sent} == {("task-beat", "tenant-1", "d-1")}
        assert all(isinstance(b["timestamp"], str) for b in sent)
        assert beats() == sent, "the heartbeat stops when the task ends"

    @pytest.mark.parametrize("outcome", ["completed", "failed", "cancelled"])
    async def test_result_names_the_dispatch(self, consumer: TaskConsumer, outcome: str) -> None:
        """S2-G fix, 6: a late result of an ended dispatch overwrote the task's next dispatch.

        Every result echoes the dispatch it reports, so the Go Core ignores a
        result of a dispatch that is no longer the task's current one.
        """
        from codeforge.backends._base import TaskResult as BackendTaskResult

        payload = TaskMessage(
            id="task-d", project_id="p1", tenant_id="tenant-1", title="t", prompt="do it", dispatch_id="d-7"
        )
        msg, _ = jetstream_msg(payload.model_dump_json().encode(), subject="tasks.agent.aider")
        consumer._backend_router = MagicMock()
        if outcome == "completed":
            consumer._backend_router.execute = AsyncMock(
                return_value=BackendTaskResult(status="completed", output="ok")
            )
        elif outcome == "failed":
            consumer._backend_router.execute = AsyncMock(side_effect=RuntimeError("backend down"))
        else:
            consumer._run_backend = AsyncMock(return_value=None)  # type: ignore[method-assign]

        await consumer._handle_message(msg)

        results = _task_results(consumer)
        assert [(r["task_id"], r["status"], r["dispatch_id"]) for r in results] == [("task-d", outcome, "d-7")]

    @pytest.mark.parametrize(("heartbeat_seconds", "interval"), [(7, 7.0), (0, 30.0)])
    async def test_heartbeat_interval_comes_from_the_task(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch, heartbeat_seconds: int, interval: float
    ) -> None:
        """S2-F review, F11: the worker beats at the Go Core's runtime.heartbeat_interval (30 s by default)."""
        import contextlib

        from codeforge.backends._base import TaskResult as BackendTaskResult

        intervals: list[float] = []

        @contextlib.asynccontextmanager
        async def recording_heartbeats(_js: object, _subject: str, _payload: object, every: float):  # type: ignore[no-untyped-def]
            intervals.append(every)
            yield

        monkeypatch.setattr("codeforge.consumer._tasks.heartbeats", recording_heartbeats)
        payload = TaskMessage(
            id="task-interval", project_id="p1", title="t", prompt="do it", heartbeat_seconds=heartbeat_seconds
        )
        msg, _ = jetstream_msg(payload.model_dump_json().encode(), subject="tasks.agent.aider")

        async def execute(**_kwargs: object) -> BackendTaskResult:
            return BackendTaskResult(status="completed", output="ok")

        consumer._backend_router = MagicMock()
        consumer._backend_router.execute = execute

        await consumer._handle_message(msg)

        assert intervals == [interval]

    async def test_lost_result_publish_is_retried(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """The task is never redelivered: its result is the only way the Go Core learns the outcome."""
        from codeforge.backends._base import TaskResult as BackendTaskResult

        monkeypatch.setattr("codeforge.nats_publish.PUBLISH_BACKOFF_SECONDS", 0.0)
        consumer._js = RecordingJetStream(failing_times={"tasks.result": 2})  # type: ignore[assignment]
        consumer._notifications = consumer._js
        consumer._backend_router = MagicMock()
        consumer._backend_router.execute = AsyncMock(return_value=BackendTaskResult(status="completed", output="ok"))
        msg, _ = jetstream_msg(_task_payload("task-retry"), subject="tasks.agent.aider")

        await consumer._handle_message(msg)

        assert [(r["task_id"], r["status"]) for r in _task_results(consumer)] == [("task-retry", "completed")]


# ---------------------------------------------------------------------------
# runs.start (at-most-once, ack on accept)
# ---------------------------------------------------------------------------


def _run_start_payload(run_id: str = "run-1") -> bytes:
    return (
        RunStartMessage(
            run_id=run_id,
            task_id="task-1",
            project_id="p1",
            agent_id="a1",
            prompt="fix it",
            workspace_path=os.path.dirname(os.path.abspath(__file__)),
        )
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
        assert client.settlements() == ["nak"], "an unconfirmed accept releases the message"
        assert _published(consumer) == []

    async def test_lost_completion_publish_is_retried(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setattr("codeforge.nats_publish.PUBLISH_BACKOFF_SECONDS", 0.0)
        consumer._js = RecordingJetStream(failing_times={"runs.complete": 1})  # type: ignore[assignment]
        consumer._notifications = consumer._js
        msg, _ = jetstream_msg(_run_start_payload("run-retry"), subject="runs.start")

        async def complete(_task: object, runtime: RuntimeClient, **_kwargs: object) -> None:
            await runtime.complete_run(status="completed", output="done")

        consumer._executor = MagicMock()
        consumer._executor.execute_with_runtime = complete

        await consumer._handle_run_start(msg)

        completions = [json.loads(data) for subject, data in _published(consumer) if subject == "runs.complete"]
        assert [(c["run_id"], c["status"]) for c in completions] == [("run-retry", "completed")]

    @pytest.mark.parametrize("fails", [False, True])
    async def test_cancel_listeners_are_released_when_the_run_ends(self, consumer: TaskConsumer, fails: bool) -> None:
        msg, _ = jetstream_msg(_run_start_payload(), subject="runs.start")
        consumer._executor = MagicMock()
        consumer._executor.execute_with_runtime = AsyncMock(side_effect=RuntimeError("boom") if fails else None)

        await consumer._handle_run_start(msg)

        subs = consumer._js.subscriptions  # type: ignore[union-attr]
        assert [s.subject for s in subs] == ["runs.cancel", "tasks.cancel"]
        assert all(s.unsubscribed for s in subs)

    @pytest.mark.parametrize("fails", [False, True])
    async def test_heartbeats_while_the_run_runs(self, consumer: TaskConsumer, fails: bool) -> None:
        """Agent-loop runs are long: the Go Core sees them alive, and the heartbeat ends with the run."""
        msg, _ = jetstream_msg(_run_start_payload("run-beat"), subject="runs.start")
        seen: dict[str, object] = {}

        async def execute(_task: object, runtime: RuntimeClient, **_kwargs: object) -> None:
            await asyncio.sleep(0.05)
            seen["runtime"] = runtime
            seen["beats"] = [json.loads(d)["run_id"] for s, d in _published(consumer) if s == "runs.heartbeat"]
            if fails:
                raise RuntimeError("boom")

        consumer._executor = MagicMock()
        consumer._executor.execute_with_runtime = execute

        await consumer._handle_run_start(msg)

        assert seen["beats"] == ["run-beat"], "a heartbeat is sent while the run runs"
        runtime = seen["runtime"]
        assert isinstance(runtime, RuntimeClient)
        assert runtime._heartbeat_task is None, "the heartbeat stops when the run ends"


# ---------------------------------------------------------------------------
# conversation.run.start (at-most-once, ack on accept)
# ---------------------------------------------------------------------------


def _conversation_payload(run_id: str = "conv-1") -> bytes:
    # An existing directory: an agentic run needs one (KI-193); no tool runs here.
    workspace = os.path.dirname(os.path.abspath(__file__))
    return (
        ConversationRunStartMessage(
            run_id=run_id,
            conversation_id=run_id,
            project_id="p1",
            messages=[],
            system_prompt="s",
            model="m",
            workspace_path=workspace,
        )
        .model_dump_json()
        .encode()
    )


def _patch_conversation_pipeline(consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch) -> None:
    """Replace the run's collaborators (LLM, routing, tools); keep the handler's own completion logic."""
    monkeypatch.setattr("codeforge.tools.build_default_registry", MagicMock)
    monkeypatch.setattr(consumer, "_maybe_prefetch_docs", AsyncMock())
    monkeypatch.setattr(consumer, "_build_conversation_messages", AsyncMock(return_value=[]))
    monkeypatch.setattr(consumer, "_resolve_routing_and_fallbacks", AsyncMock(return_value=("m", MagicMock(), [])))
    monkeypatch.setattr(
        consumer, "_execute_conversation_run", AsyncMock(return_value=AgentLoopResult(final_content="done"))
    )


def _conversation_completions(worker: TaskConsumer) -> list[dict[str, object]]:
    return [json.loads(data) for subject, data in _published(worker) if subject == "conversation.run.complete"]


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
        assert client.settlements() == ["nak"], "an unconfirmed accept releases the message"
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

    async def test_failure_after_the_completion_is_not_reported_twice(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """A run whose completion was published is over: a later cleanup error must not fail it again."""
        _patch_conversation_pipeline(consumer, monkeypatch)
        original_stop = RuntimeClient.stop_cancel_listener

        async def stop_then_fail(runtime: RuntimeClient) -> None:
            await original_stop(runtime)
            raise RuntimeError("unsubscribe failed")

        monkeypatch.setattr(RuntimeClient, "stop_cancel_listener", stop_then_fail)
        msg, _ = jetstream_msg(_conversation_payload("conv-done"), subject="conversation.run.start")

        await consumer._handle_conversation_run(msg)

        completions = _conversation_completions(consumer)
        assert [(c["run_id"], c["status"]) for c in completions] == [("conv-done", "completed")]

    async def test_lost_completion_publish_is_retried_with_one_message_id(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """The Go Core deduplicates completions by Nats-Msg-Id only, so a retry must repeat it."""
        monkeypatch.setattr("codeforge.nats_publish.PUBLISH_BACKOFF_SECONDS", 0.0)
        js = RecordingJetStream(failing_times={"conversation.run.complete": 1})
        consumer._js = js  # type: ignore[assignment]
        consumer._notifications = consumer._js
        _patch_conversation_pipeline(consumer, monkeypatch)
        msg, _ = jetstream_msg(_conversation_payload("conv-retry"), subject="conversation.run.start")

        await consumer._handle_conversation_run(msg)

        completions = _conversation_completions(consumer)
        assert [(c["run_id"], c["status"]) for c in completions] == [("conv-retry", "completed")]
        ids = [
            (headers or {}).get("Nats-Msg-Id")
            for subject, headers in zip(js.attempts, js.attempt_headers, strict=True)
            if subject == "conversation.run.complete"
        ]
        assert len(ids) == 2
        assert ids[0]
        assert ids[0] == ids[1]

    async def test_runtime_is_closed_when_the_run_fails(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        msg, _ = jetstream_msg(_conversation_payload(), subject="conversation.run.start")
        monkeypatch.setattr("codeforge.tools.build_default_registry", MagicMock)
        monkeypatch.setattr(consumer, "_resolve_routing_and_fallbacks", AsyncMock(side_effect=RuntimeError("no model")))

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
        runtime = RuntimeClient(
            js=js, notifications=js, run_id="r1", task_id="t1", project_id="p1", termination=TerminationConfig()
        )  # type: ignore[arg-type]
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

    @pytest.mark.parametrize(
        ("turn_id", "want"),
        [
            ("turn-1", {"run_id": "conv-1", "tenant_id": "tenant-1", "turn_id": "turn-1"}),
            ("", {"run_id": "conv-1", "tenant_id": "tenant-1"}),
        ],
    )
    async def test_heartbeat_names_the_tenant_and_the_turn(self, turn_id: str, want: dict[str, str]) -> None:
        """Go records a heartbeat in the run's tenant, and a conversation run's for its turn only (KI-65)."""
        js = RecordingJetStream()
        runtime = RuntimeClient(
            js=js,  # type: ignore[arg-type]
            notifications=js,  # type: ignore[arg-type]
            run_id="conv-1",
            task_id="",
            project_id="p1",
            termination=TerminationConfig(),
            tenant_id="tenant-1",
            turn_id=turn_id,
        )
        await runtime.start_heartbeat(interval=0.01)
        await asyncio.sleep(0.02)
        await runtime.close()

        beats = [json.loads(data) for subject, data in js.published if subject == "runs.heartbeat"]
        assert beats
        for beat in beats:
            assert isinstance(beat.pop("timestamp"), str)
            assert beat == want


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


def _benchmark_payload(tenant_id: str = "") -> bytes:
    from codeforge.models import BenchmarkRunRequest

    return (
        BenchmarkRunRequest(run_id="bench-1", tenant_id=tenant_id, dataset_path="/data/b.yaml", model="m")
        .model_dump_json()
        .encode()
    )


def _benchmark_results(worker: TaskConsumer) -> list[dict[str, object]]:
    return [json.loads(data) for subject, data in _published(worker) if subject == "benchmark.run.result"]


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
        assert client.settlements() == ["nak"], "an unconfirmed accept releases the message"

    async def test_litellm_unavailable_fails_the_requested_run(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """The failed result must name the run and its tenant, or the Go Core cannot end the run."""
        monkeypatch.setattr("codeforge.consumer._benchmark._wait_for_litellm", AsyncMock(return_value=False))
        msg, client = jetstream_msg(_benchmark_payload(tenant_id="t1"), subject="benchmark.run.request")

        await consumer._handle_benchmark_run(msg)

        results = _benchmark_results(consumer)
        assert [(r["run_id"], r["tenant_id"], r["status"]) for r in results] == [("bench-1", "t1", "failed")]
        assert "LiteLLM" in str(results[0]["error"])
        assert client.settlements() == ["ack"]

    async def test_lost_failure_result_is_retried(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        import structlog

        from codeforge.models import BenchmarkRunRequest

        monkeypatch.setattr("codeforge.nats_publish.PUBLISH_BACKOFF_SECONDS", 0.0)
        consumer._js = RecordingJetStream(failing_times={"benchmark.run.result": 1})  # type: ignore[assignment]
        consumer._notifications = consumer._js
        monkeypatch.setattr(
            "codeforge.consumer._benchmark._build_evaluators", MagicMock(side_effect=RuntimeError("no evaluator"))
        )
        request = BenchmarkRunRequest.model_validate_json(_benchmark_payload(tenant_id="t1"))

        await consumer._execute_benchmark_run(request, structlog.get_logger())

        results = _benchmark_results(consumer)
        assert [(r["run_id"], r["tenant_id"], r["status"]) for r in results] == [("bench-1", "t1", "failed")]


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
        runtime = RuntimeClient(
            js=js, notifications=js, run_id="r1", task_id="t1", project_id="p1", termination=TerminationConfig()
        )  # type: ignore[arg-type]
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
        runtime = RuntimeClient(
            js=js, notifications=js, run_id="r1", task_id="", project_id="p1", termination=TerminationConfig()
        )  # type: ignore[arg-type]
        await runtime.start_cancel_listener(extra_subjects=["tasks.cancel"])

        js.subscriptions[1].deliver(b'{"task_id": ""}')
        js.subscriptions[0].deliver(b'{"run_id": "", "task_id": ""}')
        await asyncio.sleep(0.1)

        assert not runtime.is_cancelled
        await runtime.close()

    async def test_cancel_subscriptions_need_no_acks(self) -> None:
        """The listener never acks: with explicit acks JetStream would redeliver every cancel
        message and stop delivering once MaxAckPending messages are outstanding."""
        js = RecordingJetStream()
        runtime = RuntimeClient(
            js=js, notifications=js, run_id="r1", task_id="t1", project_id="p1", termination=TerminationConfig()
        )  # type: ignore[arg-type]
        await runtime.start_cancel_listener(extra_subjects=["conversation.run.cancel"])
        configs = [sub.config for sub in js.subscriptions]
        await runtime.close()

        assert [(c.ack_policy, c.deliver_policy) for c in configs] == [(AckPolicy.NONE, DeliverPolicy.NEW)] * 2

    async def test_tool_call_response_subscription_needs_no_acks(self) -> None:
        js = RecordingJetStream()
        runtime = RuntimeClient(
            js=js, notifications=js, run_id="r1", task_id="t1", project_id="p1", termination=TerminationConfig()
        )  # type: ignore[arg-type]

        request = asyncio.create_task(runtime.request_tool_call("read_file", path="a.py"))
        for _ in range(500):
            if js.published:
                break
            await asyncio.sleep(0.001)
        call_id = json.loads(js.published[0][1])["call_id"]
        js.subscriptions[0].deliver(json.dumps({"call_id": call_id, "decision": "allow"}).encode())
        decision = await asyncio.wait_for(request, timeout=2)

        assert decision.decision == "allow"
        config = js.subscriptions[0].config
        assert (config.ack_policy, config.deliver_policy) == (AckPolicy.NONE, DeliverPolicy.NEW)
        assert js.subscriptions[0].unsubscribed


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

    async def test_retrieval_index_is_built_in_a_worker_thread(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """BM25 indexing and embedding decoding scale with the repository; full and incremental builds."""
        import httpx

        from codeforge import retrieval

        (tmp_path / "a.py").write_text("def alpha():\n    return 1\n")
        retriever = retrieval.HybridRetriever()

        async def post(_url: str, json: dict[str, object]) -> httpx.Response:
            vectors = [{"index": i, "embedding": [0.1, 0.2, 0.3]} for i in range(len(json["input"]))]  # type: ignore[arg-type]
            return httpx.Response(200, json={"data": vectors}, request=httpx.Request("POST", "http://litellm"))

        monkeypatch.setattr(retriever, "_get_client", lambda: SimpleNamespace(post=post))
        tokenized_on_main: list[bool] = []
        decoded_on_main: list[bool] = []
        real_tokenize = retrieval.bm25s.tokenize
        real_decode = retrieval._decode_embeddings

        def tokenize(*args: object, **kwargs: object) -> object:
            tokenized_on_main.append(threading.current_thread() is threading.main_thread())
            return real_tokenize(*args, **kwargs)

        def decode(*args: object) -> object:
            decoded_on_main.append(threading.current_thread() is threading.main_thread())
            return real_decode(*args)

        monkeypatch.setattr(retrieval.bm25s, "tokenize", tokenize)
        monkeypatch.setattr(retrieval, "_decode_embeddings", decode)

        full = await retriever.build_index(project_id="p1", workspace_path=str(tmp_path))
        (tmp_path / "b.py").write_text("def beta():\n    return 2\n")
        incremental = await retriever.build_index(project_id="p1", workspace_path=str(tmp_path))

        assert (full.status, incremental.status, incremental.incremental) == ("ready", "ready", True)
        assert tokenized_on_main == [False, False]
        assert decoded_on_main == [False, False]

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


# ---------------------------------------------------------------------------
# A worker that gives up exits promptly and fails its accepted work (ADR-016)
# ---------------------------------------------------------------------------

_FAILING_SUBJECT = "graph.search.request"


class _Work:
    """Stands in for accepted work: runs until it is cancelled, or for *seconds* and then returns *returns*."""

    def __init__(self, seconds: float | None = None, returns: object = None) -> None:
        self.started = asyncio.Event()
        self.cancelled = False
        self._seconds = seconds
        self._returns = returns

    async def __call__(self, *_args: object, **_kwargs: object) -> object:
        self.started.set()
        try:
            if self._seconds is None:
                await asyncio.Event().wait()
            else:
                await asyncio.sleep(self._seconds)
        except asyncio.CancelledError:
            self.cancelled = True
            raise
        return self._returns


def _install_task_work(consumer: TaskConsumer, _monkeypatch: pytest.MonkeyPatch, work: _Work) -> None:
    consumer._backend_router = MagicMock()
    consumer._backend_router.execute = work


def _install_run_work(consumer: TaskConsumer, _monkeypatch: pytest.MonkeyPatch, work: _Work) -> None:
    consumer._executor = MagicMock()
    consumer._executor.execute_with_runtime = work


def _install_conversation_work(consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch, work: _Work) -> None:
    monkeypatch.setattr(consumer, "_run_conversation", work)


def _install_benchmark_work(consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch, work: _Work) -> None:
    monkeypatch.setenv("APP_ENV", "development")
    get_settings.cache_clear()
    monkeypatch.setattr("codeforge.consumer._benchmark._wait_for_litellm", AsyncMock(return_value=True))
    monkeypatch.setattr("codeforge.consumer._benchmark._validate_model_exists", AsyncMock())
    monkeypatch.setattr(consumer, "_execute_benchmark_run", work)


async def _start_until_given_up(
    consumer: TaskConsumer,
    monkeypatch: pytest.MonkeyPatch,
    subscription: str,
    msg: object,
    work: _Work,
    grace: float,
) -> float:
    """Run TaskConsumer.start: *msg* is accepted on *subscription*, then another loop fails for good.

    Returns how long start() took.
    """
    monkeypatch.setattr("codeforge.consumer._MAX_CONSECUTIVE_ERRORS", 2)
    monkeypatch.setattr("codeforge.consumer._BACKOFF_MULTIPLIER", 0.0)
    monkeypatch.setattr("codeforge.consumer._GIVE_UP_GRACE_SECONDS", grace)
    monkeypatch.setattr("codeforge.nats_publish.PUBLISH_BACKOFF_SECONDS", 0.0)
    js = consumer._js
    js.find_stream_name_by_subject = AsyncMock(return_value="CODEFORGE")  # type: ignore[union-attr]
    monkeypatch.setattr("codeforge.consumer.TracingJetStreamContext", lambda _nc: js)
    patch_notification_hub(monkeypatch, js)
    monkeypatch.setattr("codeforge.consumer.nats.connect", AsyncMock(return_value=MagicMock()))
    pending = [msg]

    async def ensure(_js: object, _name: str, subject: str) -> MagicMock:
        async def fetch(**_kwargs: object) -> list[object]:
            if subject == subscription and pending:
                return [pending.pop()]
            if subject == _FAILING_SUBJECT:
                await work.started.wait()
                raise ConnectionError("connection lost")
            await asyncio.sleep(0.01)
            raise nats.errors.TimeoutError

        sub = MagicMock()
        sub.fetch = fetch
        sub.unsubscribe = AsyncMock()
        return sub

    monkeypatch.setattr("codeforge.consumer.ensure_durable", ensure)
    started_at = time.monotonic()
    await asyncio.wait_for(consumer.start(), timeout=10)
    return time.monotonic() - started_at


ACCEPTED_WORK = [
    pytest.param(
        "tasks.agent.*",
        "tasks.agent.aider",
        _task_payload("task-hang"),
        _install_task_work,
        "tasks.result",
        "task_id",
        "task-hang",
        id="tasks.agent",
    ),
    pytest.param(
        "runs.start",
        "runs.start",
        _run_start_payload("run-hang"),
        _install_run_work,
        "runs.complete",
        "run_id",
        "run-hang",
        id="runs.start",
    ),
    pytest.param(
        "conversation.run.start",
        "conversation.run.start",
        _conversation_payload("conv-hang"),
        _install_conversation_work,
        "conversation.run.complete",
        "run_id",
        "conv-hang",
        id="conversation.run.start",
    ),
    pytest.param(
        "benchmark.run.request",
        "benchmark.run.request",
        _benchmark_payload(),
        _install_benchmark_work,
        "benchmark.run.result",
        "run_id",
        "bench-1",
        id="benchmark.run.request",
    ),
]


class TestGiveUpFailsAcceptedWork:
    @pytest.mark.parametrize(
        ("subscription", "subject", "payload", "install", "result_subject", "id_field", "work_id"), ACCEPTED_WORK
    )
    async def test_unfinished_work_is_failed_and_the_worker_exits(
        self,
        consumer: TaskConsumer,
        monkeypatch: pytest.MonkeyPatch,
        subscription: str,
        subject: str,
        payload: bytes,
        install: object,
        result_subject: str,
        id_field: str,
        work_id: str,
    ) -> None:
        """Accepted work is never redelivered: without a failed completion the Go Core waits for a timeout."""
        work = _Work()
        install(consumer, monkeypatch, work)  # type: ignore[operator]
        msg, client = jetstream_msg(payload, subject=subject)

        await _start_until_given_up(consumer, monkeypatch, subscription, msg, work, grace=0.05)

        assert consumer.failed is True
        assert consumer.ready is False
        assert work.cancelled, "the work must not outlive the worker"
        results = [json.loads(data) for published, data in _published(consumer) if published == result_subject]
        assert [(r[id_field], r["status"]) for r in results] == [(work_id, "failed")]
        assert "worker stopped" in str(results[0]["error"])
        assert client.settlements()[0] == "ack(sync)"

    async def test_work_that_finishes_within_the_grace_period_is_not_failed(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        from codeforge.backends._base import TaskResult as BackendTaskResult

        work = _Work(seconds=0.05, returns=BackendTaskResult(status="completed", output="ok"))
        _install_task_work(consumer, monkeypatch, work)
        msg, _ = jetstream_msg(_task_payload("task-quick"), subject="tasks.agent.aider")

        elapsed = await _start_until_given_up(consumer, monkeypatch, "tasks.agent.*", msg, work, grace=5.0)

        assert consumer.failed is True
        assert not work.cancelled
        assert [(r["task_id"], r["status"]) for r in _task_results(consumer)] == [("task-quick", "completed")]
        assert elapsed < 5.0, "the worker exits as soon as its accepted work is done, not after the whole grace period"


# ---------------------------------------------------------------------------
# SIGTERM fails accepted work before NATS is drained (KI-65, ADR-016)
# ---------------------------------------------------------------------------


async def _start_until_stopped(
    consumer: TaskConsumer,
    monkeypatch: pytest.MonkeyPatch,
    subscription: str,
    msg: object,
    work: _Work,
    grace: float,
) -> list[int]:
    """Run TaskConsumer.start with *msg* accepted on *subscription* and stop the worker while it runs.

    Returns how many messages were published when the connection was drained.
    """
    monkeypatch.setattr("codeforge.consumer._SHUTDOWN_GRACE_SECONDS", grace)
    monkeypatch.setattr("codeforge.nats_publish.PUBLISH_BACKOFF_SECONDS", 0.0)
    js = consumer._js
    js.find_stream_name_by_subject = AsyncMock(return_value="CODEFORGE")  # type: ignore[union-attr]
    monkeypatch.setattr("codeforge.consumer.TracingJetStreamContext", lambda _nc: js)
    patch_notification_hub(monkeypatch, js)
    published_at_drain: list[int] = []

    async def drain() -> None:
        published_at_drain.append(len(_published(consumer)))

    nc = MagicMock()
    nc.is_connected = True
    nc.drain = drain
    nc.close = AsyncMock()
    monkeypatch.setattr("codeforge.consumer.nats.connect", AsyncMock(return_value=nc))
    pending = [msg]

    async def ensure(_js: object, _name: str, subject: str) -> MagicMock:
        async def fetch(**_kwargs: object) -> list[object]:
            if subject == subscription and pending:
                return [pending.pop()]
            await asyncio.sleep(0.01)
            raise nats.errors.TimeoutError

        sub = MagicMock()
        sub.fetch = fetch
        sub.unsubscribe = AsyncMock()
        return sub

    monkeypatch.setattr("codeforge.consumer.ensure_durable", ensure)
    started = asyncio.create_task(consumer.start())
    await asyncio.wait_for(work.started.wait(), timeout=10)
    await asyncio.wait_for(consumer.stop(), timeout=10)
    await asyncio.wait_for(started, timeout=10)
    return published_at_drain


class TestShutdownFailsAcceptedWork:
    @pytest.mark.parametrize(
        ("subscription", "subject", "payload", "install", "result_subject", "id_field", "work_id"), ACCEPTED_WORK
    )
    async def test_unfinished_work_is_failed_before_the_drain(
        self,
        consumer: TaskConsumer,
        monkeypatch: pytest.MonkeyPatch,
        subscription: str,
        subject: str,
        payload: bytes,
        install: object,
        result_subject: str,
        id_field: str,
        work_id: str,
    ) -> None:
        """Docker stops the worker with SIGTERM: accepted work is never redelivered, so it is failed while NATS is up."""
        work = _Work()
        install(consumer, monkeypatch, work)  # type: ignore[operator]
        msg, _ = jetstream_msg(payload, subject=subject)

        published_at_drain = await _start_until_stopped(consumer, monkeypatch, subscription, msg, work, grace=0.05)

        assert work.cancelled, "the work must not outlive the worker"
        results = [json.loads(data) for published, data in _published(consumer) if published == result_subject]
        assert [(r[id_field], r["status"]) for r in results] == [(work_id, "failed")]
        assert "shutting down" in str(results[0]["error"])
        assert published_at_drain == [len(_published(consumer))], "the failed completion is published before the drain"
        assert consumer.failed is False, "a requested stop is not a failure"

    async def test_work_that_finishes_within_the_grace_period_is_not_failed(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        from codeforge.backends._base import TaskResult as BackendTaskResult

        work = _Work(seconds=0.05, returns=BackendTaskResult(status="completed", output="ok"))
        _install_task_work(consumer, monkeypatch, work)
        msg, _ = jetstream_msg(_task_payload("task-quick"), subject="tasks.agent.aider")

        await _start_until_stopped(consumer, monkeypatch, "tasks.agent.*", msg, work, grace=5.0)

        assert not work.cancelled
        assert [(r["task_id"], r["status"]) for r in _task_results(consumer)] == [("task-quick", "completed")]

    def test_the_shutdown_fits_the_container_stop_grace_period(self) -> None:
        """Docker kills the worker after stop_grace_period: grace, cancel, reports and drain must fit in it."""
        import re
        from pathlib import Path

        import codeforge.consumer as consumer_module
        from codeforge.consumer import _in_flight

        compose = (Path(__file__).resolve().parents[3] / "docker-compose.prod.yml").read_text()
        worker = compose.split("\n  worker:\n", 1)[1].split("\n\n", 1)[0]
        match = re.search(r"stop_grace_period: (\d+)s", worker)
        assert match, "the worker service sets stop_grace_period"
        worst_case = (
            consumer_module._SHUTDOWN_GRACE_SECONDS
            + _in_flight.CANCEL_WAIT_SECONDS
            + _in_flight.REPORT_TIMEOUT_SECONDS
            + consumer_module._DRAIN_TIMEOUT_SECONDS
        )
        assert worst_case < int(match.group(1))


# ---------------------------------------------------------------------------
# Deduplication by request (KI-66)
# ---------------------------------------------------------------------------
