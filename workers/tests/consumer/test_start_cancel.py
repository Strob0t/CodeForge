"""A run or conversation run stopped while its start waits in NATS is not executed later (S2 follow-up to KI-65).

runs.cancel and conversation.run.cancel reached only the listeners of runs
that already started. Go ends the stopped run at once; a worker that fetched
the start later ran it anyway: LLM calls, workspace edits, tool calls the Go
Core then denies. Every worker records these cancels with their stream
sequence; a start published before a cancel of its run is acked and
skipped, a start published after it (the conversation's next turn) runs.
"""

from __future__ import annotations

import asyncio
import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.consumer import TaskConsumer
from codeforge.consumer._cancel_registry import conversation_key, run_key, task_key
from codeforge.models import ConversationRunStartMessage, RunStartMessage
from tests.jetstream_fakes import RecordingJetStream, jetstream_msg


@pytest.fixture
def consumer() -> TaskConsumer:
    worker = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    worker._js = RecordingJetStream()  # type: ignore[assignment]
    return worker


def _published_subjects(worker: TaskConsumer) -> list[str]:
    return [subject for subject, _ in worker._js.published]  # type: ignore[union-attr]


def _run_start(run_id: str = "run-q", seq: int = 10) -> tuple[object, object]:
    payload = RunStartMessage(run_id=run_id, task_id="task-q", project_id="p1", agent_id="a1", prompt="fix it")
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
    )
    return jetstream_msg(payload.model_dump_json().encode(), subject="conversation.run.start", stream_seq=seq)


class TestRunStart:
    @pytest.mark.parametrize("cancel_key", [run_key("run-q"), task_key("task-q")])
    async def test_a_run_stopped_while_queued_is_not_executed(self, consumer: TaskConsumer, cancel_key: str) -> None:
        consumer._executor = MagicMock()
        consumer._executor.execute_with_runtime = AsyncMock()
        consumer._cancels.record(cancel_key, 20)
        msg, client = _run_start(seq=10)

        await consumer._handle_run_start(msg)  # type: ignore[arg-type]

        consumer._executor.execute_with_runtime.assert_not_awaited()
        assert client.settlements() == ["ack"]  # type: ignore[attr-defined]
        assert "runs.complete" not in _published_subjects(consumer), "Go ended the stopped run itself"

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
        assert "conversation.run.complete" not in _published_subjects(consumer)

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
