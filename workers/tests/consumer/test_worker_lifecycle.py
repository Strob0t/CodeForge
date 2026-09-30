"""Worker lifecycle gaps (KI-67).

The Go Core owns the CODEFORGE stream's configuration (limits, retention,
dedup window): a worker that starts first waits for the stream instead of
creating it with default settings. A worker whose shutdown is over exits at
once, even while worker threads still run CPU-bound indexing.
"""

from __future__ import annotations

import asyncio
import threading
import time
from unittest.mock import AsyncMock, MagicMock

import nats.errors
import nats.js.errors
import pytest

import codeforge.consumer as consumer_module
from codeforge.consumer import TaskConsumer
from tests.jetstream_fakes import FakeSubscription


def _nats_client() -> MagicMock:
    nc = MagicMock()
    nc.is_connected = True
    nc.drain = AsyncMock()
    nc.close = AsyncMock()
    return nc


def _idle_subscription() -> MagicMock:
    async def fetch(**_kwargs: object) -> list[object]:
        await asyncio.sleep(0.01)
        raise nats.errors.TimeoutError

    sub = MagicMock()
    sub.fetch = fetch
    sub.unsubscribe = AsyncMock()
    return sub


@pytest.fixture
def js() -> AsyncMock:
    js = AsyncMock()

    async def subscribe(subject: str, config: object = None) -> FakeSubscription:
        # A subscription that waits for messages like a real one: a listener
        # loop on an AsyncMock that returns at once would never yield.
        return FakeSubscription(subject, config)

    js.subscribe = subscribe
    return js


@pytest.fixture
def consumer(monkeypatch: pytest.MonkeyPatch, js: AsyncMock) -> TaskConsumer:
    monkeypatch.setattr("codeforge.consumer.TracingJetStreamContext", lambda _nc: js)
    monkeypatch.setattr("codeforge.consumer.nats.connect", AsyncMock(return_value=_nats_client()))
    monkeypatch.setattr("codeforge.consumer._STREAM_POLL_SECONDS", 0.01)
    return TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")


async def test_a_missing_stream_is_not_created_by_the_worker(
    consumer: TaskConsumer, js: AsyncMock, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr("codeforge.consumer._STREAM_WAIT_SECONDS", 0.05)
    js.find_stream_name_by_subject = AsyncMock(side_effect=nats.js.errors.NotFoundError())
    ensure = AsyncMock()
    monkeypatch.setattr("codeforge.consumer.ensure_durable", ensure)

    with pytest.raises(RuntimeError, match="CODEFORGE"):
        await asyncio.wait_for(consumer.start(), timeout=5)

    js.add_stream.assert_not_called()
    ensure.assert_not_awaited()
    assert js.find_stream_name_by_subject.await_count > 1, "the worker waits for the Go Core"


async def test_the_worker_starts_once_the_go_core_created_the_stream(
    consumer: TaskConsumer, js: AsyncMock, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr("codeforge.consumer._STREAM_WAIT_SECONDS", 5.0)
    js.find_stream_name_by_subject = AsyncMock(
        side_effect=[nats.js.errors.NotFoundError(), nats.js.errors.NotFoundError(), "CODEFORGE"]
    )
    subscribed = asyncio.Event()

    async def ensure(_js: object, _name: str, _subject: str) -> MagicMock:
        subscribed.set()
        return _idle_subscription()

    monkeypatch.setattr("codeforge.consumer.ensure_durable", ensure)

    start = asyncio.create_task(consumer.start())
    await asyncio.wait_for(subscribed.wait(), timeout=5)
    await consumer.stop()
    await asyncio.wait_for(start, timeout=5)

    js.add_stream.assert_not_called()


async def test_a_stop_while_waiting_for_the_stream_is_not_a_crash(
    consumer: TaskConsumer, js: AsyncMock, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr("codeforge.consumer._STREAM_WAIT_SECONDS", 30.0)
    js.find_stream_name_by_subject = AsyncMock(side_effect=nats.js.errors.NotFoundError())

    start = asyncio.create_task(consumer.start())
    await asyncio.sleep(0.05)
    await consumer.stop()
    await asyncio.wait_for(start, timeout=5)  # returns, does not raise


def test_run_exits_without_waiting_for_worker_threads(monkeypatch: pytest.MonkeyPatch) -> None:
    """A thread still indexing (repo map, retrieval, graph) must not hold up the exit after the shutdown."""
    release = threading.Event()

    async def main() -> None:
        # Stands in for main() after a give-up: indexing still runs in a worker thread.
        asyncio.get_running_loop().run_in_executor(None, release.wait, 10.0)
        await asyncio.sleep(0.01)
        raise SystemExit(1)

    exits: list[int] = []

    class _ExitedError(Exception):
        pass

    def fake_exit(code: int) -> None:
        exits.append(code)
        raise _ExitedError

    monkeypatch.setattr(consumer_module, "main", main)
    monkeypatch.setattr(consumer_module.os, "_exit", fake_exit)

    started = time.monotonic()
    with pytest.raises(_ExitedError):
        consumer_module.run()
    elapsed = time.monotonic() - started
    release.set()

    assert exits == [1]
    assert elapsed < 5.0, "the exit waited for the indexing thread"
