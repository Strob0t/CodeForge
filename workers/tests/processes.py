"""Watching the real processes a test starts (KI-194): which still run after a cancel or a timeout."""

from __future__ import annotations

import asyncio
from pathlib import Path

# A shell command that starts a background child and then waits: it writes the
# shell's PID to sh.pid and the child's to bg.pid in its working directory.
SPAWN = "sleep 300 & echo $! > bg.pid; echo $$ > sh.pid; wait"


def alive(pid: int) -> bool:
    """Whether *pid* runs (a zombie that nobody reaped yet counts as gone)."""
    try:
        stat = Path(f"/proc/{pid}/stat").read_text()
    except (FileNotFoundError, ProcessLookupError):
        return False
    return stat.rsplit(")", 1)[1].split()[0] not in ("Z", "X")


async def gone(pid: int, timeout: float = 5.0) -> bool:
    """Whether *pid* has ended within *timeout* seconds."""
    loop = asyncio.get_running_loop()
    deadline = loop.time() + timeout
    while loop.time() < deadline:
        if not alive(pid):
            return True
        await asyncio.sleep(0.02)
    return not alive(pid)


async def spawned(directory: Path, timeout: float = 10.0) -> tuple[int, int]:
    """The (shell, background child) PIDs SPAWN wrote to *directory*."""
    loop = asyncio.get_running_loop()
    deadline = loop.time() + timeout
    while loop.time() < deadline:
        texts = [
            (directory / name).read_text().strip() if (directory / name).exists() else ""
            for name in ("sh.pid", "bg.pid")
        ]
        if all(text.isdigit() for text in texts):
            return int(texts[0]), int(texts[1])
        await asyncio.sleep(0.02)
    msg = f"the command wrote no PIDs to {directory}"
    raise AssertionError(msg)
