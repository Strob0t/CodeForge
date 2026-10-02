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
from codeforge.tool_identity import system_identity, use_identity
from codeforge.tool_process import start_tool_process, tool_isolation

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
        identity = system_identity() if isolation.config.required else None
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
