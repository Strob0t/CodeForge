"""A stop request is never lost, whenever it arrives (review of KI-34).

SIGTERM can arrive while start() is still connecting or subscribing. The
request is sticky: start() shuts down instead of starting its loops, and a
later signal still works.
"""

from __future__ import annotations

import asyncio
import signal
import threading
from typing import TYPE_CHECKING, ClassVar
from unittest.mock import AsyncMock, MagicMock

import nats.errors
import pytest

import codeforge.consumer as consumer_module
from codeforge.consumer import TaskConsumer

if TYPE_CHECKING:
    from collections.abc import Callable


def _nats_client() -> MagicMock:
    nc = MagicMock()
    nc.is_connected = True
    nc.jetstream = MagicMock()
    nc.drain = AsyncMock()
    nc.close = AsyncMock()
    return nc


def _idle_subscription() -> MagicMock:
    """A pull subscription without messages; fetch() yields to the event loop like the real one."""

    async def fetch(**_kwargs: object) -> list[object]:
        await asyncio.sleep(0.01)
        raise nats.errors.TimeoutError

    sub = MagicMock()
    sub.fetch = fetch
    sub.unsubscribe = AsyncMock()
    return sub


async def _idle_ensure(_js: object, _name: str, _subject: str) -> MagicMock:
    return _idle_subscription()


class _SlowConnect:
    """nats.connect that blocks until released."""

    def __init__(self, nc: MagicMock) -> None:
        self.nc = nc
        self.called = asyncio.Event()
        self.release = asyncio.Event()

    async def __call__(self, _url: str) -> MagicMock:
        self.called.set()
        await self.release.wait()
        return self.nc


@pytest.fixture
def js() -> AsyncMock:
    js = AsyncMock()
    js.find_stream_name_by_subject = AsyncMock(return_value="CODEFORGE")
    return js


@pytest.fixture
def consumer(monkeypatch: pytest.MonkeyPatch, js: AsyncMock) -> TaskConsumer:
    monkeypatch.setattr("codeforge.consumer.TracingJetStreamContext", lambda _nc: js)
    return TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")


async def test_stop_while_connecting_is_not_lost(consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch) -> None:
    nc = _nats_client()
    connect = _SlowConnect(nc)
    ensure = AsyncMock(side_effect=_idle_ensure)
    monkeypatch.setattr("codeforge.consumer.nats.connect", connect)
    monkeypatch.setattr("codeforge.consumer.ensure_durable", ensure)

    start = asyncio.create_task(consumer.start())
    await connect.called.wait()
    consumer.request_stop()
    await consumer.stop()  # nothing to drain yet
    connect.release.set()
    await asyncio.wait_for(start, timeout=5)

    ensure.assert_not_awaited()
    assert consumer._loop_tasks == []
    assert consumer.ready is False
    nc.close.assert_awaited_once()  # the connection made after stop() is closed


async def test_stop_while_subscribing_starts_no_loops(consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch) -> None:
    nc = _nats_client()
    monkeypatch.setattr("codeforge.consumer.nats.connect", AsyncMock(return_value=nc))
    subscribing = asyncio.Event()
    release = asyncio.Event()

    async def slow_ensure(_js: object, _name: str, _subject: str) -> MagicMock:
        subscribing.set()
        await release.wait()
        return _idle_subscription()

    monkeypatch.setattr("codeforge.consumer.ensure_durable", slow_ensure)

    start = asyncio.create_task(consumer.start())
    await subscribing.wait()
    consumer.request_stop()
    await consumer.stop()
    release.set()
    await asyncio.wait_for(start, timeout=5)

    assert consumer._loop_tasks == []
    assert consumer.ready is False
    nc.drain.assert_awaited_once()


async def test_setup_error_caused_by_the_stop_is_not_a_crash(
    consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch
) -> None:
    """stop() drains the connection under a subscribing start(); its failure is the stop, not an error."""
    nc = _nats_client()
    monkeypatch.setattr("codeforge.consumer.nats.connect", AsyncMock(return_value=nc))
    subscribing = asyncio.Event()
    release = asyncio.Event()

    async def failing_ensure(_js: object, _name: str, _subject: str) -> str:
        subscribing.set()
        await release.wait()
        raise ConnectionError("nats: connection closed")

    monkeypatch.setattr("codeforge.consumer.ensure_durable", failing_ensure)

    start = asyncio.create_task(consumer.start())
    await subscribing.wait()
    consumer.request_stop()
    release.set()
    await asyncio.wait_for(start, timeout=5)  # returns, does not raise
    assert consumer._loop_tasks == []


async def test_otel_shutdown_runs_off_the_event_loop(consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch) -> None:
    """The final OTLP export blocks; it must not stall the event loop (drain, other stop work)."""
    threads: list[threading.Thread] = []

    class _Tracing:
        def shutdown(self) -> None:
            threads.append(threading.current_thread())

    monkeypatch.setattr("codeforge.consumer.tracing_manager", _Tracing())
    await consumer.stop()
    assert len(threads) == 1
    assert threads[0] is not threading.main_thread()


async def test_a_failing_drain_does_not_break_stop(consumer: TaskConsumer) -> None:
    nc = _nats_client()
    nc.drain = AsyncMock(side_effect=ConnectionError("nats: connection closed"))
    consumer._nc = nc
    await consumer.stop()  # does not raise


# ---------------------------------------------------------------------------
# main(): real signals
# ---------------------------------------------------------------------------


class _Signals:
    """Records the handlers main() installs on the event loop instead of installing them.

    Real signals would kill the test process whenever no handler is installed.
    """

    def __init__(self) -> None:
        self.handlers: dict[int, Callable[[], object]] = {}
        self.removed: list[int] = []

    def add(self, sig: int, callback: Callable[[], object]) -> None:
        self.handlers[sig] = callback

    def remove(self, sig: int) -> bool:
        self.removed.append(sig)
        return self.handlers.pop(sig, None) is not None

    def send(self, sig: int) -> None:
        self.handlers[sig]()


@pytest.fixture
async def signals(monkeypatch: pytest.MonkeyPatch) -> _Signals:
    recorder = _Signals()
    loop = asyncio.get_running_loop()
    monkeypatch.setattr(loop, "add_signal_handler", recorder.add)
    monkeypatch.setattr(loop, "remove_signal_handler", recorder.remove)
    return recorder


@pytest.fixture
def quiet_main(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(consumer_module, "setup_logging", lambda **_kwargs: None)
    monkeypatch.setenv("CODEFORGE_WORKER_HEALTH_PORT", "0")


@pytest.mark.usefixtures("quiet_main")
async def test_sigterm_while_connecting_stops_the_worker(
    monkeypatch: pytest.MonkeyPatch, js: AsyncMock, signals: _Signals
) -> None:
    nc = _nats_client()
    connect = _SlowConnect(nc)
    ensure = AsyncMock(side_effect=_idle_ensure)
    monkeypatch.setattr("codeforge.consumer.nats.connect", connect)
    monkeypatch.setattr("codeforge.consumer.ensure_durable", ensure)
    monkeypatch.setattr("codeforge.consumer.TracingJetStreamContext", lambda _nc: js)

    main = asyncio.create_task(consumer_module.main())
    await asyncio.wait_for(connect.called.wait(), timeout=10)
    signals.send(signal.SIGTERM)
    await asyncio.sleep(0.05)  # the stop runs while there is no connection yet
    connect.release.set()
    await asyncio.wait_for(main, timeout=10)  # exits normally (status 0), no loops ran

    ensure.assert_not_awaited()
    nc.close.assert_awaited_once()
    assert sorted(signals.removed) == sorted([signal.SIGINT, signal.SIGTERM]), "handlers are removed on exit"


class _StubbornConsumer:
    """Keeps running until stop() was called twice (e.g. a stop that could not end start())."""

    instances: ClassVar[list[_StubbornConsumer]] = []

    def __init__(self, **_kwargs: object) -> None:
        self.started = asyncio.Event()
        self.stopped_once = asyncio.Event()
        self.released = asyncio.Event()
        self.stop_calls = 0
        self.requests = 0
        self.ready = False
        self.failed = False
        _StubbornConsumer.instances.append(self)

    def request_stop(self) -> None:
        self.requests += 1

    async def start(self) -> None:
        self.started.set()
        await self.released.wait()

    async def stop(self) -> None:
        self.stop_calls += 1
        self.stopped_once.set()
        if self.stop_calls == 2:
            self.released.set()


@pytest.mark.usefixtures("quiet_main")
async def test_a_second_signal_still_works(monkeypatch: pytest.MonkeyPatch, signals: _Signals) -> None:
    _StubbornConsumer.instances = []
    monkeypatch.setattr(consumer_module, "TaskConsumer", _StubbornConsumer)

    main = asyncio.create_task(consumer_module.main())
    while not _StubbornConsumer.instances:
        await asyncio.sleep(0.01)
    worker = _StubbornConsumer.instances[0]
    await asyncio.wait_for(worker.started.wait(), timeout=10)

    signals.send(signal.SIGTERM)
    await asyncio.wait_for(worker.stopped_once.wait(), timeout=5)
    await asyncio.sleep(0.05)
    assert not main.done()
    signals.send(signal.SIGINT)
    await asyncio.wait_for(main, timeout=10)

    assert worker.stop_calls == 2
    assert worker.requests == 2
