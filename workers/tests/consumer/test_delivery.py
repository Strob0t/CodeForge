"""Tests for the worker's JetStream delivery semantics (KI-18, KI-19, ADR-016).

Messages are real ``nats.aio.msg.Msg`` objects whose client records the
settlement (ack, nak, term, in-progress), see ``tests.jetstream_fakes``.
"""

from __future__ import annotations

import asyncio
from types import SimpleNamespace
from unittest.mock import AsyncMock

import nats.js.errors
import pytest
from nats.js.api import AckPolicy, ConsumerConfig, DeliverPolicy
from pydantic import BaseModel

from codeforge.consumer import TaskConsumer
from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._delivery import (
    delivery_attempt,
    ensure_durable,
    is_last_attempt,
    keep_in_progress,
)
from codeforge.nats_subjects import ACK_WAIT_SECONDS, MAX_DELIVER, NAK_DELAY_SECONDS, STREAM_NAME
from tests.jetstream_fakes import RecordingJetStream, jetstream_msg

# ---------------------------------------------------------------------------
# Durable consumers (KI-18)
# ---------------------------------------------------------------------------


def _admin_js(existing: ConsumerConfig | None = None, ack_floor_seq: int = 0) -> AsyncMock:
    """A JetStream context whose consumer_info returns *existing* (or raises NotFound)."""
    js = AsyncMock()
    if existing is None:
        js.consumer_info.side_effect = nats.js.errors.NotFoundError()
    else:
        js.consumer_info.return_value = SimpleNamespace(
            config=existing,
            ack_floor=SimpleNamespace(stream_seq=ack_floor_seq),
        )
    return js


def _added_config(js: AsyncMock) -> ConsumerConfig:
    js.add_consumer.assert_awaited_once()
    assert js.add_consumer.await_args.args == (STREAM_NAME,)
    return js.add_consumer.await_args.kwargs["config"]


class TestEnsureDurable:
    async def test_first_creation_starts_at_new_messages(self) -> None:
        js = _admin_js(existing=None)

        await ensure_durable(js, "codeforge-py-runs-start", "runs.start")

        cfg = _added_config(js)
        assert cfg.durable_name == "codeforge-py-runs-start"
        assert cfg.name == "codeforge-py-runs-start"
        assert cfg.filter_subject == "runs.start"
        assert cfg.deliver_policy == DeliverPolicy.NEW
        assert cfg.ack_policy == AckPolicy.EXPLICIT
        assert cfg.ack_wait == ACK_WAIT_SECONDS
        assert cfg.max_deliver == MAX_DELIVER
        assert cfg.inactive_threshold is None, "a durable must not expire while workers are down"
        js.delete_consumer.assert_not_awaited()
        js.pull_subscribe_bind.assert_awaited_once_with(consumer="codeforge-py-runs-start", stream=STREAM_NAME)

    async def test_reattach_keeps_position_and_updates_settings(self) -> None:
        """A durable from an earlier release (deliver all, server defaults) is updated, not recreated."""
        legacy = ConsumerConfig(
            name="codeforge-py-runs-start",
            durable_name="codeforge-py-runs-start",
            filter_subject="runs.start",
            deliver_policy=DeliverPolicy.ALL,
            ack_wait=30.0,
            max_deliver=-1,
        )
        js = _admin_js(existing=legacy)

        await ensure_durable(js, "codeforge-py-runs-start", "runs.start")

        cfg = _added_config(js)
        assert cfg.deliver_policy == DeliverPolicy.ALL, "start policy cannot be changed in place"
        assert cfg.ack_wait == ACK_WAIT_SECONDS
        assert cfg.max_deliver == MAX_DELIVER
        js.delete_consumer.assert_not_awaited()

    async def test_reattach_keeps_explicit_start_sequence(self) -> None:
        existing = ConsumerConfig(
            durable_name="codeforge-py-runs-start",
            deliver_policy=DeliverPolicy.BY_START_SEQUENCE,
            opt_start_seq=42,
        )
        js = _admin_js(existing=existing)

        await ensure_durable(js, "codeforge-py-runs-start", "runs.start")

        cfg = _added_config(js)
        assert cfg.deliver_policy == DeliverPolicy.BY_START_SEQUENCE
        assert cfg.opt_start_seq == 42

    async def test_reattach_is_idempotent(self) -> None:
        js = _admin_js(existing=None)
        await ensure_durable(js, "codeforge-py-runs-start", "runs.start")
        created = _added_config(js)

        js2 = _admin_js(existing=created)
        await ensure_durable(js2, "codeforge-py-runs-start", "runs.start")

        assert _added_config(js2) == created
        js2.delete_consumer.assert_not_awaited()

    async def test_push_consumer_is_replaced_after_its_ack_floor(self) -> None:
        """A push consumer cannot become a pull consumer: it is recreated without replaying."""
        push = ConsumerConfig(durable_name="codeforge-py-runs-start", deliver_subject="_INBOX.x")
        js = _admin_js(existing=push, ack_floor_seq=41)

        await ensure_durable(js, "codeforge-py-runs-start", "runs.start")

        js.delete_consumer.assert_awaited_once_with(STREAM_NAME, "codeforge-py-runs-start")
        cfg = _added_config(js)
        assert cfg.deliver_subject is None
        assert cfg.deliver_policy == DeliverPolicy.BY_START_SEQUENCE
        assert cfg.opt_start_seq == 42

    async def test_push_consumer_without_acks_is_replaced_with_new_policy(self) -> None:
        push = ConsumerConfig(durable_name="codeforge-py-runs-start", deliver_subject="_INBOX.x")
        js = _admin_js(existing=push, ack_floor_seq=0)

        await ensure_durable(js, "codeforge-py-runs-start", "runs.start")

        cfg = _added_config(js)
        assert cfg.deliver_policy == DeliverPolicy.NEW
        assert cfg.opt_start_seq is None

    async def test_consumer_start_uses_ensure_durable_for_every_subject(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """TaskConsumer.start binds every subscription through ensure_durable (no delete/replay path)."""
        consumer = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
        js = AsyncMock()
        nc = AsyncMock()
        nc.jetstream = lambda: js
        bound: dict[str, str] = {}

        async def fake_ensure(_js: object, name: str, subject: str) -> str:
            bound[subject] = name
            return name

        async def fake_loop(*_args: object) -> None:
            return None

        monkeypatch.setattr("codeforge.consumer.nats.connect", AsyncMock(return_value=nc))
        monkeypatch.setattr("codeforge.consumer.ensure_durable", fake_ensure)
        monkeypatch.setattr(consumer, "_message_loop", fake_loop)
        monkeypatch.setattr("codeforge.consumer._HEALTHY_SENTINEL", SimpleNamespace(touch=lambda: None))
        await consumer.start()

        assert bound["runs.start"] == "codeforge-py-runs-start"
        assert bound["conversation.run.start"] == "codeforge-py-conversation-run-start"
        js.delete_consumer.assert_not_awaited()
        js.pull_subscribe.assert_not_awaited()


# ---------------------------------------------------------------------------
# Delivery count
# ---------------------------------------------------------------------------


class TestDeliveryAttempt:
    @pytest.mark.parametrize(("num_delivered", "last"), [(1, False), (MAX_DELIVER - 1, False), (MAX_DELIVER, True)])
    def test_reads_jetstream_delivery_count(self, num_delivered: int, last: bool) -> None:
        msg, _ = jetstream_msg(b"{}", num_delivered=num_delivered)
        assert delivery_attempt(msg) == num_delivered
        assert is_last_attempt(msg) is last

    def test_message_without_jetstream_metadata_is_a_first_delivery(self) -> None:
        msg, _ = jetstream_msg(b"{}")
        msg.reply = ""
        assert delivery_attempt(msg) == 1
        assert is_last_attempt(msg) is False


# ---------------------------------------------------------------------------
# In-progress heartbeat (KI-18)
# ---------------------------------------------------------------------------


class TestKeepInProgress:
    async def test_reports_progress_while_the_block_runs(self) -> None:
        msg, client = jetstream_msg(b"{}")

        async with keep_in_progress(msg, interval=0.02):
            await asyncio.sleep(0.11)
        count = client.settlements().count("progress")
        await asyncio.sleep(0.06)

        assert count >= 3
        assert client.settlements().count("progress") == count, "no progress after the block exits"

    async def test_stops_once_the_message_is_settled(self) -> None:
        msg, client = jetstream_msg(b"{}")

        async with keep_in_progress(msg, interval=0.02):
            await msg.ack()
            await asyncio.sleep(0.08)

        assert client.settlements() == ["ack"]

    async def test_failed_progress_ack_does_not_break_the_handler(self) -> None:
        msg, client = jetstream_msg(b"{}")

        async def broken_publish(*_args: object, **_kwargs: object) -> None:
            raise ConnectionError("connection lost")

        client.publish = broken_publish  # type: ignore[method-assign]
        async with keep_in_progress(msg, interval=0.01):
            await asyncio.sleep(0.05)


class TestMessageLoopHeartbeat:
    async def test_loop_keeps_slow_handlers_in_progress(self) -> None:
        consumer = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
        consumer._running = True
        consumer._progress_interval = 0.02
        msg, client = jetstream_msg(b"{}")

        sub = AsyncMock()
        calls = 0

        async def fetch(**_kwargs: object) -> list[object]:
            nonlocal calls
            calls += 1
            if calls == 1:
                return [msg]
            consumer._running = False
            raise TimeoutError

        sub.fetch = fetch

        async def slow_handler(m: object) -> None:
            await asyncio.sleep(0.1)
            await msg.ack()

        await consumer._message_loop(sub, slow_handler, "test.request")

        settled = client.settlements()
        assert settled[-1] == "ack"
        assert settled.count("progress") >= 2


# ---------------------------------------------------------------------------
# Generic request handling: retries, dead letters, dedup (KI-19)
# ---------------------------------------------------------------------------


class _Request(BaseModel):
    job_id: str
    size: int = 0


class _Result(BaseModel):
    job_id: str
    ok: bool = True


class _Worker(ConsumerBaseMixin):
    def __init__(self, js: RecordingJetStream) -> None:
        self._js = js  # type: ignore[assignment]
        self.calls: list[str] = []
        self.fail_times = 0

    async def handle(self, msg: object, *, ack_on_accept: bool = False) -> None:
        options = {"ack_on_accept": True} if ack_on_accept else {}
        await self._handle_request(
            msg,  # type: ignore[arg-type]
            request_model=_Request,
            dedup_key=lambda r: f"job-{r.job_id}",
            handler=self._do,
            result_subject="test.result",
            **options,
        )

    async def _do(self, request: _Request, _log: object) -> _Result:
        self.calls.append(request.job_id)
        if self.fail_times > 0:
            self.fail_times -= 1
            raise RuntimeError("transient failure")
        return _Result(job_id=request.job_id)


VALID = b'{"job_id": "j1", "size": 3}'
NAK_DELAYED = f"nak({NAK_DELAY_SECONDS:g}s)"


class TestHandleRequest:
    async def test_success_publishes_result_and_acks(self) -> None:
        js = RecordingJetStream()
        worker = _Worker(js)
        msg, client = jetstream_msg(VALID)

        await worker.handle(msg)

        assert js.subjects() == ["test.result"]
        assert client.settlements() == ["ack"]

    @pytest.mark.parametrize(
        "payload",
        [
            b"not json",
            b"",
            b"[]",
            b"null",
            b'{"size": 3}',  # missing required field
            b'{"job_id": "j1", "size": "three"}',  # wrong type
        ],
    )
    async def test_invalid_payload_is_dead_lettered_and_terminated(self, payload: bytes) -> None:
        js = RecordingJetStream()
        worker = _Worker(js)
        msg, client = jetstream_msg(payload, subject="test.request")

        await worker.handle(msg)

        assert js.published == [("test.request.dlq", payload)]
        assert client.settlements() == ["term"], "invalid payloads must never be NAK'd"
        assert worker.calls == []

    async def test_invalid_payload_is_kept_when_dlq_publish_fails(self) -> None:
        js = RecordingJetStream(failing={"test.request.dlq"})
        worker = _Worker(js)
        msg, client = jetstream_msg(b"not json", subject="test.request")

        await worker.handle(msg)

        assert client.settlements() == [NAK_DELAYED]

    async def test_failure_before_last_attempt_is_retried_with_delay(self) -> None:
        js = RecordingJetStream()
        worker = _Worker(js)
        worker.fail_times = 1
        msg, client = jetstream_msg(VALID, num_delivered=1)

        await worker.handle(msg)

        assert client.settlements() == [NAK_DELAYED]
        assert js.published == []

    async def test_redelivery_of_a_failed_request_is_not_a_duplicate(self) -> None:
        js = RecordingJetStream()
        worker = _Worker(js)
        worker.fail_times = 1

        first, first_client = jetstream_msg(VALID, num_delivered=1)
        await worker.handle(first)
        second, second_client = jetstream_msg(VALID, num_delivered=2)
        await worker.handle(second)

        assert worker.calls == ["j1", "j1"], "the redelivered request must be processed again"
        assert first_client.settlements() == [NAK_DELAYED]
        assert second_client.settlements() == ["ack"]
        assert js.subjects() == ["test.result"]

    async def test_completed_request_is_a_duplicate(self) -> None:
        js = RecordingJetStream()
        worker = _Worker(js)
        first, _ = jetstream_msg(VALID)
        await worker.handle(first)
        again, again_client = jetstream_msg(VALID, num_delivered=2)

        await worker.handle(again)

        assert worker.calls == ["j1"]
        assert again_client.settlements() == ["ack"]

    async def test_failure_on_last_attempt_is_dead_lettered_and_acked(self) -> None:
        js = RecordingJetStream()
        worker = _Worker(js)
        worker.fail_times = 1
        msg, client = jetstream_msg(VALID, subject="test.request", num_delivered=MAX_DELIVER)

        await worker.handle(msg)

        assert js.published == [("test.request.dlq", VALID)]
        assert client.settlements() == ["ack"]

    async def test_last_attempt_is_not_acked_when_dlq_publish_fails(self) -> None:
        js = RecordingJetStream(failing={"test.request.dlq"})
        worker = _Worker(js)
        worker.fail_times = 1
        msg, client = jetstream_msg(VALID, subject="test.request", num_delivered=MAX_DELIVER)

        await worker.handle(msg)

        assert client.settlements() == [NAK_DELAYED], "never ack a message that has no dead-letter copy"

    async def test_dead_lettered_request_can_be_processed_again(self) -> None:
        js = RecordingJetStream()
        worker = _Worker(js)
        worker.fail_times = 1
        last, _ = jetstream_msg(VALID, num_delivered=MAX_DELIVER)
        await worker.handle(last)
        replayed, replayed_client = jetstream_msg(VALID, num_delivered=1)

        await worker.handle(replayed)

        assert worker.calls == ["j1", "j1"]
        assert replayed_client.settlements() == ["ack"]

    async def test_ack_on_accept_acks_before_the_handler_runs(self) -> None:
        js = RecordingJetStream()
        worker = _Worker(js)
        msg, client = jetstream_msg(VALID)
        acked_when_called: list[bool] = []

        async def observe(request: _Request, _log: object) -> None:
            acked_when_called.append(msg.is_acked)

        worker._do = observe  # type: ignore[method-assign]
        await worker.handle(msg, ack_on_accept=True)

        assert acked_when_called == [True]
        assert client.settlements() == ["ack"]

    async def test_ack_on_accept_failure_is_not_retried(self) -> None:
        """At-most-once: a failed run is not redelivered or dead-lettered."""
        js = RecordingJetStream()
        worker = _Worker(js)
        worker.fail_times = 1
        msg, client = jetstream_msg(VALID)

        await worker.handle(msg, ack_on_accept=True)

        assert client.settlements() == ["ack"]
        assert js.published == []


class TestMoveToDlq:
    async def test_copies_headers_to_the_dead_letter(self) -> None:
        js = AsyncMock()
        worker = _Worker(js)  # type: ignore[arg-type]
        msg, client = jetstream_msg(b"payload", subject="tasks.agent.aider", headers={"X-Request-ID": "req-1"})

        await worker._move_to_dlq(msg)

        js.publish.assert_awaited_once_with("tasks.agent.aider.dlq", b"payload", headers={"X-Request-ID": "req-1"})
        assert client.settlements() == ["ack"]

    async def test_terminate_settles_with_term(self) -> None:
        worker = _Worker(RecordingJetStream())
        msg, client = jetstream_msg(b"payload", subject="tasks.agent.aider")

        await worker._move_to_dlq(msg, terminate=True)

        assert client.settlements() == ["term"]
