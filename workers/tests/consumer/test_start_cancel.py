"""A run or conversation run stopped while its start waits in NATS is not executed later (S2 follow-up to KI-65).

runs.cancel and conversation.run.cancel reached only the listeners of runs
that already started. Go ends the stopped run at once; a worker that fetched
the start later ran it anyway: LLM calls, workspace edits, tool calls the Go
Core then denies. Every worker records these cancels with their stream
sequence; a start published before a cancel of its run is acked and
skipped, a start published after it (the conversation's next turn) runs.

S2-G fix, 2: a skipped start reports a cancelled completion, so the Go Core
ends the run even when no stop of its own ended it, and only a cancel of
the run (runs.cancel, conversation.run.cancel) skips a start: tasks.cancel
never ended the Go run, which then hung.
"""

from __future__ import annotations

import asyncio
import json
from unittest.mock import AsyncMock, MagicMock

import pytest
from nats.js.api import DeliverPolicy

from codeforge.consumer import TaskConsumer
from codeforge.consumer._cancel_registry import conversation_key, run_key, task_key
from codeforge.models import ConversationRunStartMessage, RunStartMessage
from tests.jetstream_fakes import RecordingJetStream, jetstream_msg


@pytest.fixture
def consumer() -> TaskConsumer:
    worker = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    worker._js = RecordingJetStream()  # type: ignore[assignment]
    worker._notifications = worker._js
    return worker


def _published_subjects(worker: TaskConsumer) -> list[str]:
    return [subject for subject, _ in worker._js.published]  # type: ignore[union-attr]


def _completions(worker: TaskConsumer, subject: str) -> list[dict[str, object]]:
    return [json.loads(data) for s, data in worker._js.published if s == subject]  # type: ignore[union-attr]


def _run_start(run_id: str = "run-q", seq: int = 10) -> tuple[object, object]:
    payload = RunStartMessage(
        run_id=run_id, task_id="task-q", project_id="p1", tenant_id="tenant-q", agent_id="a1", prompt="fix it"
    )
    return jetstream_msg(payload.model_dump_json().encode(), subject="runs.start", stream_seq=seq)


def _conversation_start(conversation_id: str = "conv-q", seq: int = 10) -> tuple[object, object]:
    payload = ConversationRunStartMessage(
        run_id=conversation_id,
        conversation_id=conversation_id,
        project_id="p1",
        messages=[],
        system_prompt="",
        model="test-model",
        turn_id="turn-1",
        tenant_id="tenant-q",
    )
    return jetstream_msg(payload.model_dump_json().encode(), subject="conversation.run.start", stream_seq=seq)


class TestRunStart:
    async def test_a_run_stopped_while_queued_is_not_executed(self, consumer: TaskConsumer) -> None:
        consumer._executor = MagicMock()
        consumer._executor.execute_with_runtime = AsyncMock()
        consumer._cancels.record(run_key("run-q"), 20)
        msg, client = _run_start(seq=10)

        await consumer._handle_run_start(msg)  # type: ignore[arg-type]

        consumer._executor.execute_with_runtime.assert_not_awaited()
        assert client.settlements() == ["ack"]  # type: ignore[attr-defined]
        completions = _completions(consumer, "runs.complete")
        assert [(c["run_id"], c["task_id"], c["tenant_id"], c["status"]) for c in completions] == [
            ("run-q", "task-q", "tenant-q", "cancelled")
        ], "the skipped start ends the Go run"

    async def test_a_task_cancel_does_not_skip_a_run(self, consumer: TaskConsumer) -> None:
        """tasks.cancel stops a backend task; it never ended a run that runs on the task."""
        consumer._executor = MagicMock()
        consumer._executor.execute_with_runtime = AsyncMock()
        consumer._cancels.record(task_key("task-q"), 20)
        msg, client = _run_start(seq=10)

        await consumer._handle_run_start(msg)  # type: ignore[arg-type]

        consumer._executor.execute_with_runtime.assert_awaited_once()
        assert client.settlements() == ["ack(sync)"]  # type: ignore[attr-defined]

    async def test_a_skipped_start_whose_completion_fails_is_retried(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """The start is released for a retry (dead-lettered on its last delivery, which Go ends), never lost."""
        monkeypatch.setattr("codeforge.nats_publish.PUBLISH_BACKOFF_SECONDS", 0.0)
        consumer._js = RecordingJetStream(failing={"runs.complete"})  # type: ignore[assignment]
        consumer._notifications = consumer._js
        consumer._executor = MagicMock()
        consumer._executor.execute_with_runtime = AsyncMock()
        consumer._cancels.record(run_key("run-q"), 20)
        msg, client = _run_start(seq=10)

        await consumer._handle_run_start(msg)  # type: ignore[arg-type]

        consumer._executor.execute_with_runtime.assert_not_awaited()
        assert [s for s in client.settlements() if s.startswith("nak")], "the start is released for a retry"  # type: ignore[attr-defined]
        redelivered, redelivered_client = _run_start(seq=10)
        consumer._js = RecordingJetStream()  # type: ignore[assignment]
        consumer._notifications = consumer._js
        await consumer._handle_run_start(redelivered)  # type: ignore[arg-type]
        assert redelivered_client.settlements() == ["ack"]  # type: ignore[attr-defined]
        assert [c["status"] for c in _completions(consumer, "runs.complete")] == ["cancelled"]

    async def test_a_start_published_after_the_cancel_runs(self, consumer: TaskConsumer) -> None:
        consumer._executor = MagicMock()
        consumer._executor.execute_with_runtime = AsyncMock()
        consumer._cancels.record(run_key("run-q"), 20)
        msg, client = _run_start(seq=30)

        await consumer._handle_run_start(msg)  # type: ignore[arg-type]

        consumer._executor.execute_with_runtime.assert_awaited_once()
        assert client.settlements() == ["ack(sync)"]  # type: ignore[attr-defined]


class TestConversationRunStart:
    @pytest.mark.parametrize("cancel_key", [conversation_key("conv-q"), run_key("conv-q")])
    async def test_a_conversation_run_stopped_while_queued_is_not_executed(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch, cancel_key: str
    ) -> None:
        run = AsyncMock()
        monkeypatch.setattr(consumer, "_run_conversation", run)
        consumer._cancels.record(cancel_key, 20)
        msg, client = _conversation_start(seq=10)

        await consumer._handle_conversation_run(msg)  # type: ignore[arg-type]

        run.assert_not_awaited()
        assert client.settlements() == ["ack"]  # type: ignore[attr-defined]
        completions = _completions(consumer, "conversation.run.complete")
        assert [(c["run_id"], c["turn_id"], c["tenant_id"], c["status"]) for c in completions] == [
            ("conv-q", "turn-1", "tenant-q", "cancelled")
        ], "the skipped start ends the Go turn"

    async def test_a_skipped_conversation_start_whose_completion_fails_is_retried(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setattr("codeforge.nats_publish.PUBLISH_BACKOFF_SECONDS", 0.0)
        consumer._js = RecordingJetStream(failing={"conversation.run.complete"})  # type: ignore[assignment]
        consumer._notifications = consumer._js
        run = AsyncMock()
        monkeypatch.setattr(consumer, "_run_conversation", run)
        consumer._cancels.record(run_key("conv-q"), 20)
        msg, client = _conversation_start(seq=10)

        await consumer._handle_conversation_run(msg)  # type: ignore[arg-type]

        run.assert_not_awaited()
        assert [s for s in client.settlements() if s.startswith("nak")], "the start is released for a retry"  # type: ignore[attr-defined]

    async def test_the_next_turn_after_a_stop_runs(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        run = AsyncMock()
        monkeypatch.setattr(consumer, "_run_conversation", run)
        consumer._cancels.record(conversation_key("conv-q"), 20)
        msg, client = _conversation_start(seq=30)

        await consumer._handle_conversation_run(msg)  # type: ignore[arg-type]

        run.assert_awaited_once()
        assert client.settlements() == ["ack(sync)"]  # type: ignore[attr-defined]


async def test_the_worker_records_run_and_conversation_cancels(consumer: TaskConsumer) -> None:
    await consumer._start_cancel_registry()
    subs = {s.subject: s for s in consumer._js.subscriptions}  # type: ignore[union-attr]
    assert set(subs) == {"tasks.cancel", "runs.cancel", "conversation.run.cancel"}

    subs["runs.cancel"].deliver(json.dumps({"run_id": "run-7"}).encode(), stream_seq=5)
    subs["conversation.run.cancel"].deliver(json.dumps({"run_id": "conv-7"}).encode(), stream_seq=6)
    subs["tasks.cancel"].deliver(json.dumps({"task_id": "task-7"}).encode(), stream_seq=7)
    await asyncio.sleep(0.1)

    assert consumer._cancels.cancelled(run_key("run-7"), 4)
    assert consumer._cancels.cancelled(conversation_key("conv-7"), 5)
    assert consumer._cancels.cancelled(task_key("task-7"), 6)
    assert not consumer._cancels.cancelled(run_key("conv-7"), 5), "a conversation cancel is no run cancel"

    await consumer._in_flight.abort([], 0, "test over")
    assert all(s.unsubscribed for s in subs.values())


class TestCancelListenersReplayFromTheStart:
    """S2-G fix, f2: a cancel published while a run's listener subscribed was lost.

    The run's and the conversation run's cancel listeners saw new messages
    only; a cancel published after the cancel registry checked the start
    and before the listener subscribed reached neither. Like a task's
    (S2-F review, F12), they replay every cancel published after the run's
    start message.
    """

    @staticmethod
    def _listener_configs(worker: TaskConsumer) -> dict[str, object]:
        return {s.subject: s.config for s in worker._js.subscriptions}  # type: ignore[union-attr]

    async def test_a_run_listens_from_its_start_on(self, consumer: TaskConsumer) -> None:
        consumer._executor = MagicMock()
        consumer._executor.execute_with_runtime = AsyncMock()
        msg, _ = _run_start(seq=10)

        await consumer._handle_run_start(msg)  # type: ignore[arg-type]

        configs = self._listener_configs(consumer)
        for subject in ("runs.cancel", "tasks.cancel"):
            assert configs[subject].deliver_policy == DeliverPolicy.BY_START_SEQUENCE, subject  # type: ignore[attr-defined]
            assert configs[subject].opt_start_seq == 11, subject  # type: ignore[attr-defined]

    async def test_a_conversation_run_listens_from_its_start_on(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setattr(
            consumer, "_resolve_routing_and_fallbacks", AsyncMock(side_effect=RuntimeError("stop after the listener"))
        )
        msg, _ = _conversation_start(seq=10)

        await consumer._handle_conversation_run(msg)  # type: ignore[arg-type]

        configs = self._listener_configs(consumer)
        for subject in ("runs.cancel", "conversation.run.cancel"):
            assert configs[subject].deliver_policy == DeliverPolicy.BY_START_SEQUENCE, subject  # type: ignore[attr-defined]
            assert configs[subject].opt_start_seq == 11, subject  # type: ignore[attr-defined]
