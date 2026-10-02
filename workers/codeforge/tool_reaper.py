"""Finding and stopping a tool UID's processes (KI-96 D9, D10).

A background process an agent starts (a dev server, a watcher, a setsid
daemon) outlives its tool call and its work item. When a tenant has no work
left in this worker, every process of its tool UID in this container is
killed. The kill goes through a pidfd: the PID is opened first and its UID
read again, so a PID reused between the scan and the kill (by a process of
another user) gets no signal, and the signal reaches exactly the process
that was checked. The worker holds CAP_KILL for this.

Killed orphans the worker inherited (it is PID 1 of its container) are
reaped; zombies of other parents are not waited for.
"""

from __future__ import annotations

import contextlib
import logging
import os
import signal
import time

from codeforge.tool_identity import TOOL_UID_MAX, TOOL_UID_MIN

logger = logging.getLogger(__name__)

_POLL_SECONDS = 0.05


def _status(proc: str, pid: int) -> dict[str, str]:
    fields: dict[str, str] = {}
    try:
        with open(f"{proc}/{pid}/status") as status:
            for line in status:
                name, sep, value = line.partition(":")
                if sep:
                    fields[name] = value.strip()
    except OSError:
        return {}
    return fields


def _uids(fields: dict[str, str]) -> list[int]:
    return [int(value) for value in fields.get("Uid", "").split()]


def processes_of(uids: set[int], proc: str = "/proc") -> dict[int, int]:
    """PID -> UID of every process in this container that runs as one of *uids* (any of its four UIDs)."""
    found: dict[int, int] = {}
    for name in os.listdir(proc):
        if not name.isdigit():
            continue
        matching = [uid for uid in _uids(_status(proc, int(name))) if uid in uids]
        if matching:
            found[int(name)] = matching[0]
    return found


def running_processes_of(uids: set[int], proc: str = "/proc") -> dict[int, int]:
    """processes_of without zombies (exited processes their parent has not collected yet)."""
    return {
        pid: uid
        for pid, uid in processes_of(uids, proc).items()
        if not _status(proc, pid).get("State", "").startswith("Z")
    }


def _alive(uid: int, proc: str) -> list[int]:
    return list(running_processes_of({uid}, proc))


def _close(fd: int) -> None:
    os.close(fd)


def _reap_zombie(pid: int) -> None:
    """Collect a killed orphan the worker inherited (PID 1 of the container)."""
    with contextlib.suppress(ChildProcessError, OSError):
        os.waitpid(pid, os.WNOHANG)


def _kill(pid: int, uid: int, proc: str) -> bool:
    try:
        fd = os.pidfd_open(pid)
    except (ProcessLookupError, OSError):
        return False  # exited meanwhile
    try:
        # The pidfd names one process now: kill it only if it still is the tenant's.
        if uid not in _uids(_status(proc, pid)):
            return False
        try:
            signal.pidfd_send_signal(fd, signal.SIGKILL)
        except ProcessLookupError:
            return False
        return True
    finally:
        _close(fd)


def reap(uid: int, *, proc: str = "/proc", timeout: float = 5.0) -> int:
    """Kill every process of tool UID *uid* in this container; how many were killed.

    Waits (bounded by *timeout*, then logs) until none is left alive.
    """
    if not TOOL_UID_MIN <= uid <= TOOL_UID_MAX:
        raise ValueError(f"{uid} is not a tool uid")
    killed = [pid for pid in _alive(uid, proc) if _kill(pid, uid, proc)]
    deadline = time.monotonic() + timeout
    while True:
        for pid in killed:
            if _status(proc, pid).get("PPid") == str(os.getpid()):
                _reap_zombie(pid)
        left = _alive(uid, proc)
        if not left:
            break
        if time.monotonic() >= deadline:
            logger.warning("processes of tool uid %d are still running after the kill: %s", uid, left[:10])
            break
        time.sleep(_POLL_SECONDS)
    if killed:
        logger.info("stopped %d leftover processes of tool uid %d", len(killed), uid)
    return len(killed)
