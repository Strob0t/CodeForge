"""The sharing pass (KI-96 D8): what a tenant's tools create stays reachable for the workspace group.

The pass runs as the entries' owner (the tenant's tool UID) through the
launcher. Here it runs in-process: the test files belong to the test's own
UID, which plays the tenant. Under default ACLs a tool can still lock the
Go Core and the worker (group 10010) out: an owner-only mode masks the
``g:10010`` entry, ``setfacl`` strips it, a directory loses its default ACL
(E1, E17). The pass repairs exactly that, on its own entries only, never
through a symlink, and writes only what differs.
"""

from __future__ import annotations

import errno
import json
import os
import stat
import subprocess
import sys
import time
from dataclasses import asdict
from typing import TYPE_CHECKING

import pytest

from codeforge import posix_acl, tool_walk
from codeforge.posix_acl import Entry
from tests.isolation_requirements import require

if TYPE_CHECKING:
    from pathlib import Path

UID = os.getuid()
GROUP = posix_acl.WORKSPACE_GID


@pytest.fixture
def workspace(tmp_path: Path) -> Path:
    ws = tmp_path / "ws"
    ws.mkdir()
    try:
        posix_acl.set_acl(str(ws), posix_acl.DEFAULT, posix_acl.tenant_default(UID))
    except OSError as exc:
        if exc.errno == errno.EOPNOTSUPP:
            pytest.skip("no POSIX ACLs on the test file system")
        raise
    posix_acl.set_acl(str(ws), posix_acl.ACCESS, posix_acl.tenant_default(UID))
    return ws


def _access(path: Path) -> list[Entry]:
    return posix_acl.get_acl(str(path), posix_acl.ACCESS) or []


def _effective_group(path: Path) -> int:
    """What g:10010 may do on *path* (its entry masked)."""
    acl = _access(path)
    named = next((e.perm for e in acl if e.tag == posix_acl.GROUP and e.id == GROUP), 0)
    mask = next((e.perm for e in acl if e.tag == posix_acl.MASK), 7)
    return named & mask


def test_owner_only_modes_are_opened_to_the_workspace_group(workspace: Path) -> None:
    private_dir = workspace / "private"
    private_dir.mkdir(mode=0o700)  # mask --- under the default ACL
    private_file = private_dir / "secret"
    private_file.write_text("x")
    private_file.chmod(0o600)
    script = workspace / "run.sh"
    script.write_text("#!/bin/sh\n")
    script.chmod(0o700)
    assert _effective_group(private_dir) == 0

    report = tool_walk.share(str(workspace))

    assert _effective_group(private_dir) == 7
    assert _effective_group(private_file) == 6
    assert _effective_group(script) == 7  # the owner may execute it: so may the group
    assert posix_acl.equal(posix_acl.get_acl(str(private_dir), posix_acl.DEFAULT), posix_acl.tenant_default(UID))
    assert report.changed == 3
    assert report.errors == []
    # The owner's own bits stay as the tool set them.
    assert stat.S_IMODE(private_file.stat().st_mode) & 0o700 == 0o600


def test_a_stripped_or_masked_group_entry_is_restored(workspace: Path) -> None:
    require(os.geteuid() == 0, "the test needs root to give entries other owners")
    stripped = workspace / "stripped"
    stripped.write_text("x")
    posix_acl.remove_acl(str(stripped), posix_acl.ACCESS)  # setfacl -b
    stripped.chmod(0o600)
    masked = workspace / "masked"
    masked.write_text("x")
    posix_acl.set_acl(
        str(masked),
        posix_acl.ACCESS,
        [Entry(posix_acl.USER_OBJ, 6), Entry(posix_acl.GROUP_OBJ, 0), Entry(posix_acl.GROUP, 6, GROUP),
         Entry(posix_acl.MASK, 0), Entry(posix_acl.OTHER, 0)],
    )  # fmt: skip
    other_group = workspace / "other-group"
    other_group.write_text("x")
    os.chown(other_group, -1, 12345)  # chgrp: the named entry still grants
    other_group.chmod(0o600)

    tool_walk.share(str(workspace))

    for path in (stripped, masked, other_group):
        assert _effective_group(path) == 6, path
        assert stat.S_IMODE(path.stat().st_mode) & 0o007 == 0, "others stay out"


def test_a_stripped_default_acl_is_restored(workspace: Path) -> None:
    sub = workspace / "sub"
    sub.mkdir()
    posix_acl.remove_acl(str(sub), posix_acl.DEFAULT)

    tool_walk.share(str(workspace))

    assert posix_acl.equal(posix_acl.get_acl(str(sub), posix_acl.DEFAULT), posix_acl.tenant_default(UID))
    # What the tool creates there inherits g:10010 again.
    (sub / "new").write_text("x")
    assert _effective_group(sub / "new") == 6


def test_entries_with_mode_0000_are_shared_and_walked(workspace: Path) -> None:
    locked = workspace / "locked"
    locked.mkdir()
    inner = locked / "inner"
    inner.write_text("x")
    inner.chmod(0o000)
    locked.chmod(0o000)

    tool_walk.share(str(workspace))

    assert _effective_group(locked) == 7
    assert _effective_group(inner) == 6


def test_symlinks_are_never_followed(workspace: Path, tmp_path: Path) -> None:
    outside = tmp_path / "outside"
    outside.write_text("x")
    outside.chmod(0o600)
    outside_dir = tmp_path / "outside-dir"
    outside_dir.mkdir(mode=0o700)
    (workspace / "link").symlink_to(outside)
    (workspace / "dirlink").symlink_to(outside_dir)

    report = tool_walk.share(str(workspace))

    assert posix_acl.get_acl(str(outside), posix_acl.ACCESS) is None
    assert stat.S_IMODE(outside.stat().st_mode) == 0o600
    assert posix_acl.get_acl(str(outside_dir), posix_acl.DEFAULT) is None
    assert report.errors == []


def test_a_symlinked_root_is_refused(workspace: Path, tmp_path: Path) -> None:
    link = tmp_path / "ws-link"
    link.symlink_to(workspace)
    with pytest.raises(OSError, match="symlink"):
        tool_walk.share(str(link))


def test_entries_of_other_owners_are_left_alone(workspace: Path) -> None:
    require(os.geteuid() == 0, "the test needs root to give entries other owners")
    foreign = workspace / "foreign"
    foreign.write_text("x")
    os.chown(foreign, 12345, -1)
    foreign.chmod(0o600)
    posix_acl.remove_acl(str(foreign), posix_acl.ACCESS)

    report = tool_walk.share(str(workspace))

    assert posix_acl.get_acl(str(foreign), posix_acl.ACCESS) is None
    assert report.foreign == 1


def test_a_second_pass_writes_nothing(workspace: Path) -> None:
    (workspace / "a").mkdir(mode=0o700)
    (workspace / "a" / "b").write_text("x")
    assert tool_walk.share(str(workspace)).changed > 0
    assert tool_walk.share(str(workspace)).changed == 0


def test_the_changed_mode_skips_entries_not_touched_since(workspace: Path) -> None:
    old = workspace / "old"
    old.write_text("x")
    old.chmod(0o600)
    since = time.time() + 5  # nothing changed after this

    report = tool_walk.share(str(workspace), since=since)

    assert report.changed == 0
    assert _effective_group(old) == 0
    assert tool_walk.share(str(workspace), since=time.time() - 60).changed == 1
    assert _effective_group(old) == 6


def test_a_replaced_entry_is_skipped(workspace: Path, tmp_path: Path) -> None:
    """Between the listing and the open the tool swaps a directory for a symlink: the
    descriptor's inode differs from the listed one, nothing is changed through it."""
    victim = workspace / "victim"
    victim.mkdir()
    listed = os.stat(victim, follow_symlinks=False)
    victim.rmdir()
    elsewhere = tmp_path / "elsewhere"
    elsewhere.mkdir(mode=0o700)
    victim.symlink_to(elsewhere)

    fd = os.open(workspace, os.O_RDONLY | os.O_DIRECTORY)
    try:
        assert tool_walk.open_checked(fd, "victim", listed) is None
    finally:
        os.close(fd)
    assert posix_acl.get_acl(str(elsewhere), posix_acl.DEFAULT) is None


def test_the_walker_runs_as_a_script(workspace: Path) -> None:
    """The launcher starts it as ``<python> -I -S tool_walk.py share <root>``: no site, no paths."""
    (workspace / "p").mkdir(mode=0o700)
    done = subprocess.run(  # noqa: S603 - the walker itself
        [sys.executable, "-I", "-S", tool_walk.__file__, "share", str(workspace), "--since", "0"],
        capture_output=True,
        text=True,
        check=False,
    )
    assert done.returncode == 0, done.stderr
    assert '"changed": 1' in done.stdout
    assert _effective_group(workspace / "p") == 7


# ---------------------------------------------------------------------------
# A bounded number of descriptors (KI-223, R8-8)
# ---------------------------------------------------------------------------

# Runs the walk in a process of its own with RLIMIT_NOFILE at the descriptors
# it has open plus argv[2]; prints what it saw. The walk opened a descriptor
# for every subdirectory of a directory before descending: a node_modules
# with more than about 1000 packages hit EMFILE under Docker's usual soft
# limit of 1024, and those subtrees were skipped.
_LIMITED_WALK = """
import json, os, resource, sys
from codeforge import tool_walk
headroom = int(sys.argv[2])
soft = len(os.listdir("/proc/self/fd")) + headroom
resource.setrlimit(resource.RLIMIT_NOFILE, (soft, resource.getrlimit(resource.RLIMIT_NOFILE)[1]))
seen = []
report = tool_walk.walk(sys.argv[1], lambda d, n, i, r: seen.append(n), tool_walk.Report())
print(json.dumps({"visited": len(seen), "errors": report.errors}))
"""


def _limited_walk(root: Path, headroom: int) -> dict[str, object]:
    done = subprocess.run(  # noqa: S603 - this interpreter
        [sys.executable, "-c", _LIMITED_WALK, str(root), str(headroom)],
        capture_output=True,
        text=True,
        check=False,
        cwd=os.path.dirname(os.path.dirname(os.path.abspath(tool_walk.__file__))),
    )
    assert done.returncode == 0, done.stderr
    return json.loads(done.stdout)


def test_a_wide_tree_is_walked_under_a_low_descriptor_limit(tmp_path: Path) -> None:
    modules = tmp_path / "node_modules"
    modules.mkdir()
    for i in range(1500):
        (modules / f"pkg{i}" / "lib").mkdir(parents=True)

    result = _limited_walk(tmp_path, 48)

    assert result["errors"] == []
    assert result["visited"] == 1 + 1 + 1500 * 2  # the root, node_modules, each package and its lib


def test_a_deep_tree_is_walked_under_a_low_descriptor_limit(tmp_path: Path) -> None:
    """Deeper than the descriptors the walk holds: the deep levels are reopened from the root."""
    depth = 200
    path = tmp_path
    for level in range(depth):
        path = path / f"d{level}"
        path.mkdir()
        (path / "f").write_text("x")
    (tmp_path / "d0" / "side").mkdir()

    result = _limited_walk(tmp_path, 48)

    assert result["errors"] == []
    assert result["visited"] == 1 + depth * 2 + 1


def test_a_deep_directory_replaced_meanwhile_is_not_entered(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """A level the walk reopens from the root must still be the listed inode."""
    monkeypatch.setattr(tool_walk, "_MAX_HELD_DIRS", 2)
    path = tmp_path
    for level in range(5):
        path = path / f"d{level}"
        path.mkdir()
    deep = tmp_path / "d0" / "d1" / "d2" / "d3"
    (deep / "a").mkdir()
    (deep / "b").mkdir()
    swapped: list[str] = []

    def visit(_dir_fd: int, name: str, _info: os.stat_result, _report: tool_walk.Report) -> None:
        if name == "a" and not swapped:
            # The walk listed d3 and holds no descriptor for it: replace it before b is entered.
            moved = tmp_path / "moved"
            deep.rename(moved)
            deep.mkdir()
            (deep / "b").mkdir()
            swapped.append(name)

    report = tool_walk.walk(str(tmp_path), visit, tool_walk.Report())

    assert swapped
    assert report.unentered >= 1
    assert any("d3" in error for error in report.errors)


def test_a_subtree_that_cannot_be_entered_is_counted(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    (tmp_path / "ok" / "inner").mkdir(parents=True)
    (tmp_path / "locked" / "inner").mkdir(parents=True)
    real_open = tool_walk._open_subdir

    def refuse_locked(dir_fd: int, name: str, listed: os.stat_result) -> int | None:
        return None if name == "locked" else real_open(dir_fd, name, listed)

    monkeypatch.setattr(tool_walk, "_open_subdir", refuse_locked)
    seen: list[str] = []

    report = tool_walk.walk(str(tmp_path), lambda _d, n, _i, _r: seen.append(n), tool_walk.Report())

    assert report.unentered == 1
    assert sorted(seen) == sorted([tmp_path.name, "ok", "inner", "locked"])
    assert any(error.endswith("locked: cannot be entered") for error in report.errors)


def test_the_count_of_skipped_subtrees_is_in_the_report(tmp_path: Path) -> None:
    """The migration reads it from the walker's JSON report."""
    assert "unentered" in json.loads(
        json.dumps(asdict(tool_walk.walk(str(tmp_path), lambda *_: None, tool_walk.Report())))
    )


# ---------------------------------------------------------------------------
# What the walk could not check is reported, never hidden (KI-223 review)
# ---------------------------------------------------------------------------


def _failing_lstat(monkeypatch: pytest.MonkeyPatch, name: str, error: OSError) -> None:
    real = tool_walk._lstat_at

    def lstat_at(dir_fd: int, entry: str) -> os.stat_result:
        if entry == name:
            raise error
        return real(dir_fd, entry)

    monkeypatch.setattr(tool_walk, "_lstat_at", lstat_at)


@pytest.mark.parametrize(
    "error",
    [
        pytest.param(OSError(errno.EIO, "Input/output error"), id="EIO"),
        pytest.param(OSError(errno.ELOOP, "Too many levels of symbolic links"), id="ELOOP"),
    ],
)
def test_an_entry_that_cannot_be_examined_is_not_walked(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch, error: OSError
) -> None:
    """Only an entry removed meanwhile may be passed over silently: an EIO or EACCES hid a subtree."""
    (tmp_path / "broken" / "inner").mkdir(parents=True)
    _failing_lstat(monkeypatch, "broken", error)

    report = tool_walk.walk(str(tmp_path), lambda *_: None, tool_walk.Report())

    assert report.unentered == 1
    assert any("broken" in message for message in report.errors)


def test_an_entry_removed_meanwhile_is_passed_over(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    (tmp_path / "gone").mkdir()
    _failing_lstat(monkeypatch, "gone", FileNotFoundError(errno.ENOENT, "No such file or directory"))

    report = tool_walk.walk(str(tmp_path), lambda *_: None, tool_walk.Report())

    assert report.unentered == 0
    assert report.errors == []


def test_exact_fails_when_its_census_missed_a_subtree(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """A census that missed a subtree takes the links into it for links outside the tree: exact()
    would leave those files' legacy ACLs (and planted entries) as they are."""
    (tmp_path / "f").write_text("x")
    missed = tool_walk.Report(unentered=1, errors=[f"{tmp_path}/hidden: cannot be entered"])
    monkeypatch.setattr(tool_walk, "census", lambda _root: ({}, missed))

    report = tool_walk.exact(str(tmp_path), 20009)

    assert report.unentered == 1
    assert any("hidden" in message for message in report.errors)
