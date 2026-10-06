"""Subprocess helpers shared across backend executors."""

from __future__ import annotations

import asyncio
import contextlib
import logging
import os
import shutil
import signal

from codeforge.constants import CLI_CHECK_TIMEOUT_SECONDS
from codeforge.subprocess_env import tool_env
from codeforge.tool_identity import system_work, use_identity
from codeforge.tool_process import start_tool_process, start_tool_shell, tool_isolation

logger = logging.getLogger(__name__)


def _signal_group(pgid: object, sig: signal.Signals) -> None:
    # Only a child's own group: never init's (1), never the worker's own, and
    # nothing that is not a real PID (a mocked process would be "1").
    if type(pgid) is not int or pgid <= 1 or pgid == os.getpgrp():
        logger.error("refusing to signal process group %r", pgid)
        return
    # ESRCH: the group is gone; EPERM: its members are exiting (zombies).
    with contextlib.suppress(ProcessLookupError, PermissionError):
        os.killpg(pgid, sig)


def kill_process_group(proc: asyncio.subprocess.Process) -> None:
    """SIGKILL the process group of *proc* (started with ``start_new_session=True``) at once.

    Synchronous: a cancelled caller runs it before it awaits anything, so
    nothing the process started survives the cancel (KI-194). Only while
    *proc* was not reaped: the group's ID is its PID, which may name another
    process group once it is free again (as tool_process._kill_group).
    """
    if proc.returncode is None:
        _signal_group(proc.pid, signal.SIGKILL)


# How much a read of a command's output asks for at once.
_READ_CHUNK = 64 * 1024


class _CappedOutput:
    """Output that keeps its first and last *limit*/2 bytes (the summary of a test run is at its end);
    what lies between is dropped, with a note, as it arrives."""

    def __init__(self, limit: int) -> None:
        self._half = limit // 2
        self._head = bytearray()
        self._tail = bytearray()
        self._total = 0

    def extend(self, chunk: bytes) -> None:
        self._total += len(chunk)
        if len(self._head) < self._half:
            room = self._half - len(self._head)
            self._head += chunk[:room]
            chunk = chunk[room:]
        self._tail += chunk
        del self._tail[: max(0, len(self._tail) - self._half)]

    def value(self) -> bytes:
        dropped = self._total - len(self._head) - len(self._tail)
        if dropped <= 0:
            return bytes(self._head + self._tail)
        return bytes(self._head) + f"\n\n... {dropped} bytes of output truncated ...\n\n".encode() + bytes(self._tail)


async def _collect(stream: asyncio.StreamReader | None, sink: bytearray | _CappedOutput) -> None:
    """Read *stream* to its end into *sink*; what arrived stays there when the read is cancelled."""
    if stream is None:
        return
    while chunk := await stream.read(_READ_CHUNK):
        sink.extend(chunk)


async def _wait_in_group(
    proc: asyncio.subprocess.Process,
    timeout: float,
    *sinks: tuple[asyncio.StreamReader | None, bytearray | _CappedOutput],
) -> None:
    """Read the output of *proc* into the sinks and wait for it, for at most *timeout* seconds.

    When it does not end in time (TimeoutError), when the caller is
    cancelled (a Stop, a run's wall clock, a worker shutdown) and when the
    wait fails, the whole group is killed: neither the command nor anything
    it started keeps running or holds its pipes (KI-194). A command that
    ended while what it started in the background still holds its pipes is
    not a timeout: the read stops with what arrived, and the job keeps
    running (KI-194 review).
    """
    try:
        async with asyncio.timeout(timeout):
            await asyncio.gather(*(_collect(stream, sink) for stream, sink in sinks))
            await proc.wait()
    except TimeoutError:
        if proc.returncode is not None:
            return
        kill_process_group(proc)
        await proc.wait()
        raise
    except BaseException:
        kill_process_group(proc)
        raise


async def communicate_in_group(proc: asyncio.subprocess.Process, timeout: float) -> tuple[bytes, bytes]:
    """``proc.communicate()`` for at most *timeout* seconds; *proc* runs in a process group of its own.

    Raises TimeoutError when the command does not end in time; see _wait_in_group.
    """
    stdout, stderr = bytearray(), bytearray()
    await _wait_in_group(proc, timeout, (proc.stdout, stdout), (proc.stderr, stderr))
    return bytes(stdout), bytes(stderr)


async def run_tool_shell(command: str, *, cwd: str | None, timeout: float, max_output: int) -> tuple[int, str]:
    """Run a shell *command* for an agent, stdout and stderr together; (exit code, output).

    It runs in a process group of its own for at most *timeout* seconds
    (then TimeoutError). On a timeout, a cancel or an error the whole group
    is killed (see _wait_in_group). The output keeps its first and last
    *max_output*/2 bytes.
    """
    proc = await start_tool_shell(
        command,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.STDOUT,
        cwd=cwd,
        env=tool_env(),
        start_new_session=True,
    )
    output = _CappedOutput(max_output)
    await _wait_in_group(proc, timeout, (proc.stdout, output))
    return proc.returncode or 0, output.value().decode("utf-8", errors="replace")


async def terminate_process_group(
    proc: asyncio.subprocess.Process,
    grace_period: float = 5.0,
) -> None:
    """Stop a process started with ``start_new_session=True`` and everything it started.

    SIGTERM to its process group -> wait(grace) for the leader -> SIGKILL to
    what is left of the group -> wait(). Agent CLIs start shell commands and
    servers of their own; stopping only the CLI leaves those running.
    """
    _signal_group(proc.pid, signal.SIGTERM)
    with contextlib.suppress(TimeoutError):
        await asyncio.wait_for(proc.wait(), timeout=grace_period)
    # The group ID stays reserved while members live, even after the leader
    # was reaped, so this reaches only the leader's descendants.
    _signal_group(proc.pid, signal.SIGKILL)
    await proc.wait()


async def check_cli_available(
    cli_path: str,
    timeout: int = CLI_CHECK_TIMEOUT_SECONDS,
) -> bool:
    """Return True if *cli_path* is reachable (via ``shutil.which`` or ``--version``).

    With tool isolation the CLI is looked up on the tool PATH (the PATH tool
    processes get; a CLI only in the worker's venv is reported missing
    instead of failing at run time) and ``--version`` runs as the system tool
    user: the check belongs to no tenant (KI-96).
    """
    isolation = tool_isolation()
    tool_path = isolation.config.tool_path if isolation.config.required else None
    if shutil.which(cli_path, path=tool_path) is not None:
        return True
    try:
        async with system_work() as identity:
            with use_identity(identity):
                env = tool_env()
            proc = await start_tool_process(
                cli_path,
                "--version",
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
                env=env,
                identity=identity,
            )
            await asyncio.wait_for(proc.communicate(), timeout=timeout)
            return proc.returncode == 0
    except (OSError, TimeoutError):
        return False


async def run_subprocess(
    args: list[str],
    *,
    cwd: str | None = None,
    timeout: int = 120,
    merge_stderr: bool = False,
) -> tuple[int, str, str]:
    """Run a subprocess with a scrubbed environment (``tool_env``) and return
    ``(returncode, stdout, stderr)``.

    When *merge_stderr* is True, stderr is redirected to stdout and the
    returned ``stderr`` string is empty.
    """
    stderr_target = asyncio.subprocess.STDOUT if merge_stderr else asyncio.subprocess.PIPE

    proc = await start_tool_process(
        *args,
        stdout=asyncio.subprocess.PIPE,
        stderr=stderr_target,
        cwd=cwd or None,
        env=tool_env(),
    )

    stdout_bytes, stderr_bytes = await asyncio.wait_for(proc.communicate(), timeout=timeout)
    stdout = (stdout_bytes or b"").decode("utf-8", errors="replace")
    stderr = (stderr_bytes or b"").decode("utf-8", errors="replace")
    return proc.returncode or 0, stdout, stderr
