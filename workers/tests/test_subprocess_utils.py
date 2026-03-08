"""Tests for subprocess utility helpers."""

from __future__ import annotations

import asyncio
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.subprocess_utils import graceful_terminate


class TestGracefulTerminate:
    """Tests for graceful_terminate()."""

    @pytest.mark.asyncio
    async def test_sigterm_succeeds_within_grace_period(self) -> None:
        """Process exits cleanly after SIGTERM within the grace period."""
        proc = AsyncMock(spec=asyncio.subprocess.Process)
        proc.terminate = MagicMock()
        proc.kill = MagicMock()
        proc.wait = AsyncMock(return_value=0)

        await graceful_terminate(proc, grace_period=5.0)

        proc.terminate.assert_called_once()
        proc.wait.assert_awaited_once()
        proc.kill.assert_not_called()

    @pytest.mark.asyncio
    async def test_sigterm_timeout_falls_back_to_sigkill(self) -> None:
        """When SIGTERM times out, SIGKILL is sent."""
        proc = AsyncMock(spec=asyncio.subprocess.Process)
        proc.terminate = MagicMock()
        proc.kill = MagicMock()

        # First wait (grace period) times out, second wait (after kill) succeeds.
        call_count = 0

        async def wait_side_effect() -> int:
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                # Simulate the wait_for timeout by never completing.
                await asyncio.sleep(100)
            return -9

        proc.wait = AsyncMock(side_effect=wait_side_effect)

        await graceful_terminate(proc, grace_period=0.05)

        proc.terminate.assert_called_once()
        proc.kill.assert_called_once()
        assert proc.wait.await_count == 2

    @pytest.mark.asyncio
    async def test_process_already_exited(self) -> None:
        """No error when process has already exited (OSError on terminate)."""
        proc = AsyncMock(spec=asyncio.subprocess.Process)
        proc.terminate = MagicMock(side_effect=OSError("No such process"))
        proc.kill = MagicMock()
        proc.wait = AsyncMock(return_value=0)

        # Should not raise
        await graceful_terminate(proc)

        proc.terminate.assert_called_once()
        proc.kill.assert_not_called()

    @pytest.mark.asyncio
    async def test_custom_grace_period_honored(self) -> None:
        """Custom grace_period is passed to wait_for timeout."""
        proc = AsyncMock(spec=asyncio.subprocess.Process)
        proc.terminate = MagicMock()
        proc.kill = MagicMock()

        # wait() never returns to force a timeout
        async def slow_wait() -> int:
            await asyncio.sleep(100)
            return 0

        proc.wait = AsyncMock(side_effect=slow_wait)

        # With a very short grace, it should fall through to kill quickly
        await graceful_terminate(proc, grace_period=0.01)

        proc.terminate.assert_called_once()
        proc.kill.assert_called_once()

    @pytest.mark.asyncio
    async def test_oserror_on_kill_suppressed(self) -> None:
        """OSError on kill (already dead) is suppressed."""
        proc = AsyncMock(spec=asyncio.subprocess.Process)
        proc.terminate = MagicMock()
        proc.kill = MagicMock(side_effect=OSError("No such process"))

        # wait times out on SIGTERM
        call_count = 0

        async def wait_side_effect() -> int:
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                await asyncio.sleep(100)
            return -9

        proc.wait = AsyncMock(side_effect=wait_side_effect)

        # Should not raise despite kill() raising OSError
        await graceful_terminate(proc, grace_period=0.01)

        proc.terminate.assert_called_once()
        proc.kill.assert_called_once()
