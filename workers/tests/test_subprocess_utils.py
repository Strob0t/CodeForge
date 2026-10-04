"""Tests for subprocess utility helpers."""

from __future__ import annotations

import asyncio
import os
import signal
from unittest.mock import AsyncMock, MagicMock, call

import pytest

from codeforge.subprocess_utils import terminate_process_group


class TestTerminateProcessGroup:
    """Tests for terminate_process_group() (KI-22); os.killpg is always patched here."""

    @staticmethod
    def _proc(pid: object) -> AsyncMock:
        proc = AsyncMock(spec=asyncio.subprocess.Process)
        proc.pid = pid
        proc.wait = AsyncMock(return_value=0)
        return proc

    @pytest.mark.asyncio
    async def test_terms_then_kills_the_group(self, monkeypatch: pytest.MonkeyPatch) -> None:
        killpg = MagicMock()
        monkeypatch.setattr("codeforge.subprocess_utils.os.killpg", killpg)

        await terminate_process_group(self._proc(424242), grace_period=0.01)

        assert killpg.call_args_list == [call(424242, signal.SIGTERM), call(424242, signal.SIGKILL)]

    @pytest.mark.asyncio
    async def test_kills_the_group_only_after_the_grace_period(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """A process that ignores SIGTERM gets SIGKILL once the grace period is over."""
        signals: list[tuple[int, float]] = []
        loop = asyncio.get_running_loop()
        monkeypatch.setattr(
            "codeforge.subprocess_utils.os.killpg", lambda _pgid, sig: signals.append((sig, loop.time()))
        )
        proc = self._proc(424242)
        waits = 0

        async def wait() -> int:
            nonlocal waits
            waits += 1
            if waits == 1:
                await asyncio.sleep(10)  # ignores SIGTERM
            return -9

        proc.wait = AsyncMock(side_effect=wait)

        await terminate_process_group(proc, grace_period=0.05)

        assert [sig for sig, _ in signals] == [signal.SIGTERM, signal.SIGKILL]
        assert signals[1][1] - signals[0][1] >= 0.05

    @pytest.mark.asyncio
    async def test_group_already_gone_is_not_an_error(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setattr("codeforge.subprocess_utils.os.killpg", MagicMock(side_effect=ProcessLookupError))

        await terminate_process_group(self._proc(424242), grace_period=0.01)

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        "pid",
        [MagicMock(), 1, 0, -1, None, "123"],
        ids=["mocked process", "init", "zero", "negative", "none", "string"],
    )
    async def test_never_signals_anything_but_a_child_group(self, monkeypatch: pytest.MonkeyPatch, pid: object) -> None:
        """A mocked process has pid MagicMock(), which os.killpg reads as 1: that would kill init's group."""
        killpg = MagicMock()
        monkeypatch.setattr("codeforge.subprocess_utils.os.killpg", killpg)

        await terminate_process_group(self._proc(pid), grace_period=0.01)

        killpg.assert_not_called()

    @pytest.mark.asyncio
    async def test_never_signals_the_workers_own_group(self, monkeypatch: pytest.MonkeyPatch) -> None:
        killpg = MagicMock()
        monkeypatch.setattr("codeforge.subprocess_utils.os.killpg", killpg)

        await terminate_process_group(self._proc(os.getpgrp()), grace_period=0.01)

        killpg.assert_not_called()
