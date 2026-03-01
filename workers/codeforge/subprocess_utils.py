"""Subprocess helpers shared across backend executors."""

from __future__ import annotations

import asyncio
import logging
import shutil

from codeforge.constants import CLI_CHECK_TIMEOUT_SECONDS

logger = logging.getLogger(__name__)


async def check_cli_available(
    cli_path: str,
    timeout: int = CLI_CHECK_TIMEOUT_SECONDS,
) -> bool:
    """Return True if *cli_path* is reachable (via ``shutil.which`` or ``--version``)."""
    if shutil.which(cli_path) is not None:
        return True
    try:
        proc = await asyncio.create_subprocess_exec(
            cli_path,
            "--version",
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
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
    """Run a subprocess and return ``(returncode, stdout, stderr)``.

    When *merge_stderr* is True, stderr is redirected to stdout and the
    returned ``stderr`` string is empty.
    """
    stderr_target = asyncio.subprocess.STDOUT if merge_stderr else asyncio.subprocess.PIPE

    proc = await asyncio.create_subprocess_exec(
        *args,
        stdout=asyncio.subprocess.PIPE,
        stderr=stderr_target,
        cwd=cwd or None,
    )

    stdout_bytes, stderr_bytes = await asyncio.wait_for(proc.communicate(), timeout=timeout)
    stdout = (stdout_bytes or b"").decode("utf-8", errors="replace")
    stderr = (stderr_bytes or b"").decode("utf-8", errors="replace")
    return proc.returncode or 0, stdout, stderr
