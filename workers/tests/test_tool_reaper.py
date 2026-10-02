"""Stopping a tenant's leftover tool processes when the tenant goes idle (KI-96 D10).

A background process an agent starts (a dev server, a watcher, a setsid
daemon) outlives its tool call. When the tenant has no work left in the
worker, every process of the tenant's tool UID is killed through a pidfd:
the PID is opened first and its UID read again, so a PID reused between the
scan and the kill gets no signal. These tests run on a fake /proc and fake
pidfd calls: no real process is ever signalled.
"""

from __future__ import annotations

import signal
from typing import TYPE_CHECKING

import pytest

from codeforge import tool_reaper

if TYPE_CHECKING:
    from pathlib import Path

UID = 20011


def _process(proc: Path, pid: int, uid: int, state: str = "S (sleeping)", ppid: int = 1) -> None:
    (proc / str(pid)).mkdir(exist_ok=True)
    (proc / str(pid) / "status").write_text(
        f"Name:\tx\nState:\t{state}\nPPid:\t{ppid}\nUid:\t{uid}\t{uid}\t{uid}\t{uid}\n"
    )


class _FakePidfd:
    """os.pidfd_open and signal.pidfd_send_signal on the fake /proc."""

    def __init__(self, proc: Path) -> None:
        self.proc = proc
        self.opened: list[int] = []
        self.signalled: list[tuple[int, int]] = []
        self.closed: list[int] = []
        self.on_open: dict[int, object] = {}  # pid -> what happens between the scan and the open

    def pidfd_open(self, pid: int, _flags: int = 0) -> int:
        action = self.on_open.get(pid)
        if callable(action):
            action()
        if not (self.proc / str(pid)).exists():
            raise ProcessLookupError(pid)
        self.opened.append(pid)
        return 1000 + pid

    def send(self, fd: int, sig: int) -> None:
        pid = fd - 1000
        self.signalled.append((pid, sig))
        # The process dies.
        for entry in (self.proc / str(pid)).iterdir():
            entry.unlink()
        (self.proc / str(pid)).rmdir()

    def close(self, fd: int) -> None:
        self.closed.append(fd)


@pytest.fixture
def fake(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> _FakePidfd:
    fake = _FakePidfd(tmp_path)
    monkeypatch.setattr(tool_reaper.os, "pidfd_open", fake.pidfd_open)
    monkeypatch.setattr(tool_reaper.signal, "pidfd_send_signal", fake.send)
    monkeypatch.setattr(tool_reaper, "_close", fake.close)
    monkeypatch.setattr(tool_reaper, "_reap_zombie", lambda _pid: None)
    return fake


def test_processes_of_reads_proc(tmp_path: Path) -> None:
    for pid, uid in ((101, 10002), (102, 20009), (103, 10001), (104, 20009)):
        _process(tmp_path, pid, uid)
    (tmp_path / "self").mkdir()
    (tmp_path / "105").mkdir()  # exited meanwhile: no status
    assert tool_reaper.processes_of({10002, 20009}, proc=str(tmp_path)) == {101: 10002, 102: 20009, 104: 20009}


def test_the_tenants_processes_are_killed_through_pidfds(tmp_path: Path, fake: _FakePidfd) -> None:
    _process(tmp_path, 201, UID)
    _process(tmp_path, 202, UID)
    _process(tmp_path, 203, 20012)  # another tenant's
    _process(tmp_path, 204, 10001)  # the worker

    assert tool_reaper.reap(UID, proc=str(tmp_path), timeout=1.0) == 2

    assert sorted(fake.signalled) == [(201, signal.SIGKILL), (202, signal.SIGKILL)]
    assert sorted(fake.closed) == [1201, 1202]
    assert (tmp_path / "203").exists()
    assert (tmp_path / "204").exists()


def test_a_process_that_exits_before_the_open_gets_no_signal(tmp_path: Path, fake: _FakePidfd) -> None:
    _process(tmp_path, 301, UID)

    def exit_now() -> None:
        (tmp_path / "301" / "status").unlink()
        (tmp_path / "301").rmdir()

    fake.on_open[301] = exit_now
    assert tool_reaper.reap(UID, proc=str(tmp_path), timeout=1.0) == 0
    assert fake.signalled == []


def test_a_pid_reused_by_another_user_gets_no_signal(tmp_path: Path, fake: _FakePidfd) -> None:
    """Between the scan and the open the process exits and its PID goes to a process of another
    UID: the pidfd names that new process, whose status no longer shows the tenant's UID."""
    _process(tmp_path, 401, UID)
    fake.on_open[401] = lambda: _process(tmp_path, 401, 10001)
    assert tool_reaper.reap(UID, proc=str(tmp_path), timeout=1.0) == 0
    assert fake.signalled == []
    assert fake.closed == [1401]


def test_zombies_are_not_waited_for(tmp_path: Path, fake: _FakePidfd) -> None:
    _process(tmp_path, 501, UID, state="Z (zombie)")
    _process(tmp_path, 502, UID)
    assert tool_reaper.reap(UID, proc=str(tmp_path), timeout=0.5) == 1
    assert fake.signalled == [(502, signal.SIGKILL)]
    assert tool_reaper.running_processes_of({UID}, proc=str(tmp_path)) == {}
    assert tool_reaper.processes_of({UID}, proc=str(tmp_path)) == {501: UID}


def test_only_tool_uids_are_reaped() -> None:
    for uid in (0, 10001, 10002, 19999, 30000):
        with pytest.raises(ValueError, match="tool uid"):
            tool_reaper.reap(uid, proc="/nonexistent", timeout=0.1)
