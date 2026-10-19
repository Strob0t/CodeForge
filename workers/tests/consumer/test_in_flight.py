"""Tests for the registry of accepted at-most-once work (ADR-016)."""

from __future__ import annotations

import asyncio
import time
from typing import TYPE_CHECKING

from codeforge.consumer._in_flight import InFlightWork

if TYPE_CHECKING:
    import pytest


class _Reporter:
    def __init__(self, error: Exception | None = None, hang: bool = False) -> None:
        self.reasons: list[str] = []
        self._error = error
        self._hang = hang

    async def __call__(self, reason: str) -> None:
        self.reasons.append(reason)
        if self._hang:
            await asyncio.Event().wait()
        if self._error is not None:
            raise self._error


async def test_unfinished_work_is_reported_with_the_reason() -> None:
    in_flight = InFlightWork()
    reporter = _Reporter()
    in_flight.accept("run r1", reporter)

    await in_flight.abort([], grace=0.0, reason="worker stopped")

    assert reporter.reasons == ["worker stopped"]


async def test_completed_work_is_not_reported() -> None:
    in_flight = InFlightWork()
    reporter = _Reporter()
    work = in_flight.accept("run r1", reporter)
    work.completed = True

    await in_flight.abort([], grace=0.0, reason="worker stopped")

    assert reporter.reasons == []


async def test_released_work_is_not_reported() -> None:
    in_flight = InFlightWork()
    reporter = _Reporter()
    with in_flight.track("run r1", reporter):
        pass

    await in_flight.abort([], grace=0.0, reason="worker stopped")

    assert reporter.reasons == []


async def test_a_failing_report_does_not_stop_the_others() -> None:
    in_flight = InFlightWork()
    broken = _Reporter(error=ConnectionError("nats down"))
    healthy = _Reporter()
    in_flight.accept("run r1", broken)
    in_flight.accept("run r2", healthy)

    await in_flight.abort([], grace=0.0, reason="worker stopped")

    assert broken.reasons == ["worker stopped"]
    assert healthy.reasons == ["worker stopped"]


async def test_reporting_is_bounded(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr("codeforge.consumer._in_flight.REPORT_TIMEOUT_SECONDS", 0.05)
    in_flight = InFlightWork()
    in_flight.accept("run r1", _Reporter(hang=True))

    started = time.monotonic()
    await asyncio.wait_for(in_flight.abort([], grace=0.0, reason="worker stopped"), timeout=2)

    assert time.monotonic() - started < 1


async def test_grace_period_ends_when_the_work_is_done() -> None:
    in_flight = InFlightWork()
    reporter = _Reporter()
    work = in_flight.accept("run r1", reporter)

    async def finish() -> None:
        await asyncio.sleep(0.02)
        in_flight.release(work)

    finisher = asyncio.create_task(finish())
    started = time.monotonic()
    await in_flight.abort([], grace=5.0, reason="worker stopped")
    await finisher

    assert time.monotonic() - started < 2
    assert reporter.reasons == []


async def test_tasks_and_background_tasks_are_cancelled() -> None:
    in_flight = InFlightWork()
    loop_task = asyncio.create_task(asyncio.Event().wait())
    background = in_flight.start_background(asyncio.Event().wait(), name="benchmark-b1")

    await in_flight.abort([loop_task], grace=0.0, reason="worker stopped")

    assert loop_task.cancelled()
    assert background.cancelled()
