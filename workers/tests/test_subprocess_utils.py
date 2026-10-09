"""Tests for subprocess utility helpers."""

from __future__ import annotations

import asyncio
import os
import signal
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock, call

import pytest

from codeforge.subprocess_utils import (
    communicate_in_group,
    kill_process_group,
    run_tool_shell,
    terminate_process_group,
)
from tests.processes import alive, gone

if TYPE_CHECKING:
    from pathlib import Path


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


class TestKillOnlyAChildThatWasNotReaped:
    """KI-194 review: the group's ID is the leader's PID; once the leader was reaped the PID may name
    another process group (the worker starts many). Kill only while returncode is None, as
    tool_process._kill_group does."""

    @staticmethod
    def _proc(returncode: int | None) -> MagicMock:
        proc = MagicMock(spec=asyncio.subprocess.Process)
        proc.pid = 424242
        proc.returncode = returncode
        return proc

    def test_a_running_child_is_killed_with_its_group(self, monkeypatch: pytest.MonkeyPatch) -> None:
        killpg = MagicMock()
        monkeypatch.setattr("codeforge.subprocess_utils.os.killpg", killpg)

        kill_process_group(self._proc(None))

        assert killpg.call_args_list == [call(424242, signal.SIGKILL)]

    @pytest.mark.parametrize("returncode", [0, 1, -9])
    def test_a_reaped_child_is_never_signalled(self, monkeypatch: pytest.MonkeyPatch, returncode: int) -> None:
        killpg = MagicMock()
        monkeypatch.setattr("codeforge.subprocess_utils.os.killpg", killpg)

        kill_process_group(self._proc(returncode))

        killpg.assert_not_called()


# A command that ends at once and leaves a background job holding its output pipes.
_LEAVES_A_JOB = "echo done; sleep 300 & echo $! > bg.pid"


class TestAFinishedCommandIsNotATimeout:
    """KI-194 review: a command that ended while what it started still held its pipes was reported
    as timed out (and its group killed after its PID was free)."""

    @pytest.mark.asyncio
    async def test_communicate_returns_the_output_of_a_finished_command(self, tmp_path: Path) -> None:
        proc = await asyncio.create_subprocess_exec(
            "bash",
            "-c",
            _LEAVES_A_JOB,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            cwd=tmp_path,
            start_new_session=True,
        )
        try:
            stdout, stderr = await communicate_in_group(proc, 1)

            assert (stdout, stderr, proc.returncode) == (b"done\n", b"", 0)
            assert alive(int((tmp_path / "bg.pid").read_text())), "a finished command keeps its background jobs"
        finally:
            os.kill(int((tmp_path / "bg.pid").read_text()), signal.SIGKILL)

    @pytest.mark.asyncio
    async def test_run_tool_shell_returns_the_output_of_a_finished_command(self, tmp_path: Path) -> None:
        try:
            assert await run_tool_shell(_LEAVES_A_JOB, cwd=str(tmp_path), timeout=1, max_output=1000) == (0, "done\n")
        finally:
            os.kill(int((tmp_path / "bg.pid").read_text()), signal.SIGKILL)

    @pytest.mark.asyncio
    async def test_a_command_still_running_times_out_with_its_group(self, tmp_path: Path) -> None:
        with pytest.raises(TimeoutError):
            await run_tool_shell("sleep 300 & echo $! > bg.pid; wait", cwd=str(tmp_path), timeout=1, max_output=1000)

        assert await gone(int((tmp_path / "bg.pid").read_text()))
