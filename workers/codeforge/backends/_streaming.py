"""Streaming subprocess runner for backend executors."""

from __future__ import annotations

import asyncio
import logging
import os
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from codeforge.backends._base import OutputCallback

logger = logging.getLogger(__name__)


async def run_streaming_subprocess(
    cmd: list[str],
    cwd: str,
    on_output: OutputCallback | None = None,
    timeout: float = 3600.0,
    env: dict[str, str] | None = None,
) -> tuple[int, str, str]:
    """Run a subprocess, streaming stdout/stderr line-by-line via callback.

    Returns (exit_code, stdout_text, stderr_text).
    """
    merged_env = {**os.environ, **(env or {})}

    proc = await asyncio.create_subprocess_exec(
        *cmd,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
        cwd=cwd,
        env=merged_env,
    )

    stdout_lines: list[str] = []
    stderr_lines: list[str] = []

    async def _read_stream(stream: asyncio.StreamReader, lines: list[str]) -> None:
        while True:
            line_bytes = await stream.readline()
            if not line_bytes:
                break
            line = line_bytes.decode("utf-8", errors="replace").rstrip("\n")
            lines.append(line)
            if on_output:
                try:
                    await on_output(line)
                except Exception as exc:
                    logger.debug("on_output callback error: %s", exc)

    try:
        await asyncio.wait_for(
            asyncio.gather(
                _read_stream(proc.stdout, stdout_lines),  # type: ignore[arg-type]
                _read_stream(proc.stderr, stderr_lines),  # type: ignore[arg-type]
            ),
            timeout=timeout,
        )
        await proc.wait()
    except TimeoutError:
        proc.kill()
        await proc.wait()
        return -1, "\n".join(stdout_lines), "Process timed out"

    return proc.returncode or 0, "\n".join(stdout_lines), "\n".join(stderr_lines)
