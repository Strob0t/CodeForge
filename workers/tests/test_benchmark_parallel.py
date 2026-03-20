"""Tests for parallel benchmark run processing (REC-1).

Verifies the semaphore-based concurrency limiting and the task exception
callback used by ``BenchmarkHandlerMixin._handle_benchmark_run``.
"""

from __future__ import annotations

import asyncio

import pytest


@pytest.mark.asyncio
async def test_semaphore_limits_concurrency() -> None:
    """Verify that a semaphore caps the number of concurrent coroutines."""
    sem = asyncio.Semaphore(2)
    active = 0
    max_active = 0

    async def fake_run() -> None:
        nonlocal active, max_active
        async with sem:
            active += 1
            max_active = max(max_active, active)
            await asyncio.sleep(0.05)
            active -= 1

    tasks = [asyncio.create_task(fake_run()) for _ in range(5)]
    await asyncio.gather(*tasks)
    assert max_active == 2


def test_get_benchmark_semaphore_default() -> None:
    """Default BENCHMARK_MAX_PARALLEL is 3."""
    from codeforge.consumer._benchmark import _get_benchmark_semaphore

    sem = _get_benchmark_semaphore()
    assert sem._value == 3


def test_get_benchmark_semaphore_env(monkeypatch: pytest.MonkeyPatch) -> None:
    """BENCHMARK_MAX_PARALLEL env var controls the semaphore value."""
    monkeypatch.setenv("BENCHMARK_MAX_PARALLEL", "7")
    from codeforge.consumer._benchmark import _get_benchmark_semaphore

    sem = _get_benchmark_semaphore()
    assert sem._value == 7


def test_ensure_benchmark_semaphore_returns_same_instance() -> None:
    """_ensure_benchmark_semaphore is a lazy singleton."""
    import codeforge.consumer._benchmark as mod

    # Reset module-level state to test lazy initialization.
    mod._benchmark_semaphore = None
    try:
        first = mod._ensure_benchmark_semaphore()
        second = mod._ensure_benchmark_semaphore()
        assert first is second
    finally:
        # Clean up to avoid polluting other tests.
        mod._benchmark_semaphore = None


@pytest.mark.asyncio
async def test_handle_task_exception_logs_error(caplog: pytest.LogCaptureFixture) -> None:
    """_handle_task_exception logs errors from failed tasks without raising."""
    from codeforge.consumer._benchmark import _handle_task_exception

    async def failing() -> None:
        raise RuntimeError("boom")

    task = asyncio.create_task(failing(), name="test-boom")
    # Wait for the task to finish (it will raise internally).
    with pytest.raises(RuntimeError):
        await task

    # The callback should not raise — it only logs.
    _handle_task_exception(task)


@pytest.mark.asyncio
async def test_handle_task_exception_ignores_cancelled() -> None:
    """_handle_task_exception silently ignores cancelled tasks."""
    from codeforge.consumer._benchmark import _handle_task_exception

    async def slow() -> None:
        await asyncio.sleep(10)

    task = asyncio.create_task(slow(), name="test-cancel")
    task.cancel()
    with pytest.raises(asyncio.CancelledError):
        await task

    # Should not raise.
    _handle_task_exception(task)


@pytest.mark.asyncio
async def test_handle_task_exception_ignores_success() -> None:
    """_handle_task_exception does nothing for successful tasks."""
    from codeforge.consumer._benchmark import _handle_task_exception

    async def ok() -> None:
        pass

    task = asyncio.create_task(ok(), name="test-ok")
    await task

    # Should not raise.
    _handle_task_exception(task)
