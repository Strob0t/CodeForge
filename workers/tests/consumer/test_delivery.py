"""Tests for the worker's JetStream delivery semantics (KI-18, KI-19, ADR-016).

Messages are real ``nats.aio.msg.Msg`` objects whose client records the
settlement (ack, nak, term, in-progress), see ``tests.jetstream_fakes``.
"""

from __future__ import annotations

import asyncio
import time
from types import SimpleNamespace
from typing import ClassVar
from unittest.mock import AsyncMock, MagicMock

import nats.errors
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
from codeforge.nats_publish import PUBLISH_ATTEMPTS
from codeforge.nats_subjects import ACCEPT_ATTEMPTS, ACK_WAIT_SECONDS, MAX_DELIVER, NAK_DELAY_SECONDS, STREAM_NAME
from tests.jetstream_fakes import RecordingClient, RecordingJetStream, jetstream_msg

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
        jetstream_clients: list[object] = []

        def tracing_jetstream(client: object) -> AsyncMock:
            jetstream_clients.append(client)
            return js

        monkeypatch.setattr("codeforge.consumer.TracingJetStreamContext", tracing_jetstream)
        bound: dict[str, str] = {}

        async def fake_ensure(_js: object, name: str, subject: str) -> str:
            bound[subject] = name
            return name

        reattachers: dict[str, object] = {}

        async def fake_loop(_sub: object, _handler: object, label: str, reattach: object = None) -> None:
            reattachers[label] = reattach

        monkeypatch.setattr("codeforge.consumer.nats.connect", AsyncMock(return_value=nc))
        monkeypatch.setattr("codeforge.consumer.ensure_durable", fake_ensure)
        monkeypatch.setattr(consumer, "_message_loop", fake_loop)
        await consumer.start()

        assert jetstream_clients == [nc], "every publish must carry the trace context (KI-36)"
        assert bound["runs.start"] == "codeforge-py-runs-start"
        assert bound["conversation.run.start"] == "codeforge-py-conversation-run-start"
        js.delete_consumer.assert_not_awaited()
        js.pull_subscribe.assert_not_awaited()
        assert set(reattachers) == set(bound), "every loop can re-attach its durable"
        assert all(callable(r) for r in reattachers.values())


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


async def _wait_for_progress(client: RecordingClient, count: int) -> None:
    """Wait until *count* in-progress acks were sent; the deadline only bounds a broken implementation."""
    deadline = time.monotonic() + 5.0
    while client.settlements().count("progress") < count:
        if time.monotonic() > deadline:
            pytest.fail(f"in-progress acks: {client.settlements().count('progress')}, want at least {count}")
        await asyncio.sleep(0.001)


class TestKeepInProgress:
    async def test_reports_progress_while_the_block_runs(self) -> None:
        msg, client = jetstream_msg(b"{}")

        async with keep_in_progress(msg, interval=0.005):
            await _wait_for_progress(client, 3)
        count = client.settlements().count("progress")
        await asyncio.sleep(0.04)  # eight intervals

        assert client.settlements().count("progress") == count, "no progress after the block exits"

    async def test_stops_once_the_message_is_settled(self) -> None:
        msg, client = jetstream_msg(b"{}")

        async with keep_in_progress(msg, interval=0.005):
            await msg.ack()
            await asyncio.sleep(0.04)  # eight intervals

        assert client.settlements() == ["ack"]

    async def test_stops_at_the_limit(self) -> None:
        """A hung handler is reported in progress only up to the limit, then JetStream redelivers it.

        The limit is measured with an injected clock, so the test does not depend on scheduling.
        """
        now = [0.0]
        msg, client = jetstream_msg(b"{}")

        async with keep_in_progress(msg, interval=0.005, limit=60.0, clock=lambda: now[0]):
            await _wait_for_progress(client, 2)
            now[0] = 60.5
            count = client.settlements().count("progress")
            await asyncio.sleep(0.04)  # eight intervals
            after = client.settlements().count("progress")

        assert after == count

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
            await _wait_for_progress(client, 2)
            await msg.ack()

        await consumer._message_loop(sub, slow_handler, "test.request")

        settled = client.settlements()
        assert settled[-1] == "ack"
        assert settled.count("progress") >= 2


class TestMessageLoopConsumerLifecycle:
    """The loop re-attaches to a deleted durable and fails the worker when it cannot recover (KI-67)."""

    @pytest.fixture
    def consumer(self, monkeypatch: pytest.MonkeyPatch) -> TaskConsumer:
        monkeypatch.setattr("codeforge.consumer._BACKOFF_MULTIPLIER", 0.0)
        worker = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
        worker._running = True
        return worker

    async def test_reattaches_a_deleted_durable(self, consumer: TaskConsumer) -> None:
        """A fetch on a deleted durable fails with 'no responders'; the loop ensures it again."""
        msg, _ = jetstream_msg(b"{}")
        gone = MagicMock()
        gone.fetch = AsyncMock(side_effect=nats.js.errors.ServiceUnavailableError())
        gone.unsubscribe = AsyncMock()
        batches: list[list[object]] = [[msg]]

        async def fetch(**_kwargs: object) -> list[object]:
            if batches:
                return batches.pop(0)
            consumer._running = False
            raise TimeoutError

        fresh = MagicMock()
        fresh.fetch = fetch
        reattach = AsyncMock(return_value=fresh)
        handled: list[object] = []

        async def handler(m: object) -> None:
            handled.append(m)

        await consumer._message_loop(gone, handler, "test.request", reattach=reattach)

        reattach.assert_awaited_once()
        gone.unsubscribe.assert_awaited_once()
        assert handled == [msg]
        assert consumer.failed is False

    async def test_gives_up_and_fails_the_worker(self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch) -> None:
        """After too many errors the worker is marked unhealthy and all loops stop, so it exits and restarts."""
        monkeypatch.setattr("codeforge.consumer._MAX_CONSECUTIVE_ERRORS", 3)
        consumer._nc = MagicMock(is_connected=True)
        loop = asyncio.get_running_loop()
        consumer._loop_tasks = [loop.create_task(asyncio.sleep(3600))]
        assert consumer.ready is True
        sub = MagicMock()
        sub.fetch = AsyncMock(side_effect=ConnectionError("connection lost"))

        async def handler(_m: object) -> None:
            raise AssertionError("no message expected")

        await consumer._message_loop(sub, handler, "test.request")
        assert consumer.ready is False, "/health/ready fails as soon as the worker gives up (KI-34)"
        await consumer._abort_task

        assert sub.fetch.await_count == 3
        assert consumer.failed is True
        assert consumer._running is False
        assert consumer.ready is False

    async def test_idle_fetches_reset_the_error_count(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """An answered fetch without messages is a healthy idle: separate error episodes do not add up."""
        monkeypatch.setattr("codeforge.consumer._MAX_CONSECUTIVE_ERRORS", 3)
        idle = nats.errors.TimeoutError()
        script: list[BaseException] = [ConnectionError("blip"), ConnectionError("blip"), idle] * 3

        async def fetch(**_kwargs: object) -> list[object]:
            if not script:
                consumer._running = False
                raise idle
            raise script.pop(0)

        sub = MagicMock()
        sub.fetch = fetch

        async def handler(_m: object) -> None:
            raise AssertionError("no message expected")

        await consumer._message_loop(sub, handler, "test.request")

        assert consumer.failed is False

    async def test_reattach_resets_the_error_count(
        self, consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """A durable that was deleted and re-attached successfully is healthy again."""
        monkeypatch.setattr("codeforge.consumer._MAX_CONSECUTIVE_ERRORS", 2)
        deletions = 3

        async def fetch(**_kwargs: object) -> list[object]:
            nonlocal deletions
            if deletions:
                deletions -= 1
                raise nats.js.errors.ServiceUnavailableError()
            consumer._running = False
            raise nats.errors.TimeoutError

        def subscription() -> MagicMock:
            sub = MagicMock()
            sub.fetch = fetch
            sub.unsubscribe = AsyncMock()
            return sub

        reattach = AsyncMock(side_effect=lambda: subscription())

        async def handler(_m: object) -> None:
            raise AssertionError("no message expected")

        await consumer._message_loop(subscription(), handler, "test.request", reattach=reattach)

        assert reattach.await_count == 3
        assert consumer.failed is False

    async def test_main_exits_non_zero_when_the_worker_failed(self, monkeypatch: pytest.MonkeyPatch) -> None:
        import codeforge.consumer as consumer_module

        class _FailedConsumer:
            instances: ClassVar[list[_FailedConsumer]] = []

            def __init__(self, **_kwargs: object) -> None:
                self.failed = False
                self.stopped = False
                _FailedConsumer.instances.append(self)

            async def start(self) -> None:
                self.failed = True

            async def stop(self) -> None:
                self.stopped = True

        monkeypatch.setattr(consumer_module, "TaskConsumer", _FailedConsumer)
        monkeypatch.setattr(consumer_module, "setup_logging", lambda **_kwargs: None)
        monkeypatch.setenv("CODEFORGE_WORKER_HEALTH_PORT", "0")  # any free port for the health server

        with pytest.raises(SystemExit) as exc_info:
            await consumer_module.main()

        assert exc_info.value.code == 1
        assert _FailedConsumer.instances[0].stopped is True


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
        assert client.settlements() == ["ack(sync)"], "accepting a run needs a confirmed (double) ack"

    async def test_ack_on_accept_failure_is_not_retried(self) -> None:
        """At-most-once: a failed run is not redelivered or dead-lettered."""
        js = RecordingJetStream()
        worker = _Worker(js)
        worker.fail_times = 1
        msg, client = jetstream_msg(VALID)

        await worker.handle(msg, ack_on_accept=True)

        assert client.settlements() == ["ack(sync)"]
        assert js.published == []

    async def test_unconfirmed_ack_is_retried(self) -> None:
        """Repeating the double ack is idempotent: the server confirms an ack it already applied."""
        js = RecordingJetStream()
        worker = _Worker(js)
        msg, client = jetstream_msg(VALID)
        client.failing_requests = ACCEPT_ATTEMPTS - 1

        await worker.handle(msg, ack_on_accept=True)

        assert worker.calls == ["j1"]
        assert client.request_count == ACCEPT_ATTEMPTS
        assert client.settlements() == ["ack(sync)"]

    async def test_unconfirmed_accept_does_not_start_the_work(self) -> None:
        """If the ack on accept is never confirmed the work is not started and the message is released.

        The NAK hands a message whose ack never arrived to the next worker at
        once; the server ignores it for a message whose ack did arrive.
        """
        js = RecordingJetStream()
        worker = _Worker(js)
        msg, client = jetstream_msg(VALID)
        client.fail_requests = True

        await worker.handle(msg, ack_on_accept=True)

        assert worker.calls == []
        assert client.request_count == ACCEPT_ATTEMPTS
        assert client.settlements() == ["nak"]
        assert js.published == []

        redelivered, redelivered_client = jetstream_msg(VALID, num_delivered=2)
        await worker.handle(redelivered, ack_on_accept=True)

        assert worker.calls == ["j1"], "the redelivery must not be skipped as a duplicate"
        assert redelivered_client.settlements() == ["ack(sync)"]


class TestMoveToDlq:
    async def test_copies_headers_to_the_dead_letter(self) -> None:
        js = AsyncMock()
        worker = _Worker(js)  # type: ignore[arg-type]
        msg, client = jetstream_msg(b"payload", subject="tasks.agent.aider", headers={"X-Request-ID": "req-1"})

        await worker._move_to_dlq(msg)

        js.publish.assert_awaited_once_with("tasks.agent.aider.dlq", b"payload", headers={"X-Request-ID": "req-1"})
        assert client.settlements() == ["ack"]

    async def test_drops_publish_control_headers(self) -> None:
        """The original Nats-Msg-Id would make JetStream discard the copy as a duplicate."""
        js = RecordingJetStream()
        worker = _Worker(js)
        headers = {"Nats-Msg-Id": "orig-1", "Nats-Expected-Stream": "OTHER", "X-Request-ID": "req-1"}
        msg, client = jetstream_msg(b"payload", subject="tasks.agent.aider", headers=headers)

        await worker._move_to_dlq(msg, terminate=True)

        assert js.published == [("tasks.agent.aider.dlq", b"payload")]
        assert js.published_headers == [{"X-Request-ID": "req-1", "X-Original-Msg-Id": "orig-1"}]
        assert msg.headers == headers, "the original message's headers must stay untouched"
        assert client.settlements() == ["term"]

    @pytest.mark.parametrize("terminate", [False, True])
    async def test_duplicate_publish_ack_keeps_the_message(self, terminate: bool) -> None:
        """A PubAck with duplicate=True stored nothing: never settle the original as dead-lettered."""
        js = RecordingJetStream(duplicates={"tasks.agent.aider.dlq"})
        worker = _Worker(js)
        msg, client = jetstream_msg(b"payload", subject="tasks.agent.aider", headers={"Nats-Msg-Id": "orig-1"})

        await worker._move_to_dlq(msg, terminate=terminate)

        assert client.settlements() == [NAK_DELAYED]

    async def test_terminate_settles_with_term(self) -> None:
        worker = _Worker(RecordingJetStream())
        msg, client = jetstream_msg(b"payload", subject="tasks.agent.aider")

        await worker._move_to_dlq(msg, terminate=True)

        assert client.settlements() == ["term"]


# ---------------------------------------------------------------------------
# Results and completions that must not be lost (ADR-016)
# ---------------------------------------------------------------------------


class TestPublishResult:
    @pytest.fixture(autouse=True)
    def _no_backoff(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setattr("codeforge.nats_publish.PUBLISH_BACKOFF_SECONDS", 0.0)

    async def test_transient_failure_is_retried(self) -> None:
        js = RecordingJetStream(failing_times={"test.result": 1})
        worker = _Worker(js)

        await worker._publish_result(_Result(job_id="j1", ok=False), "test.result")

        assert js.attempts == ["test.result", "test.result"]
        assert js.published == [("test.result", _Result(job_id="j1", ok=False).model_dump_json().encode())]

    async def test_final_failure_is_logged_not_raised(self, monkeypatch: pytest.MonkeyPatch) -> None:
        js = RecordingJetStream(failing={"test.result"})
        worker = _Worker(js)
        log = MagicMock()
        monkeypatch.setattr("codeforge.consumer._base.logger", log)

        await worker._publish_result(_Result(job_id="j1"), "test.result")

        assert js.attempts == ["test.result"] * PUBLISH_ATTEMPTS
        log.exception.assert_called_once()
        assert log.exception.call_args.kwargs["subject"] == "test.result"
