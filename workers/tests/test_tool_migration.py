"""Migrating workspace trees from before the upgrade (KI-96 D9).

Before the upgrade every tenant's tools ran as the shared tool user 10002 in
the workspace group: its files are in every tenant tree, a legacy tool could
hard-link a file of another tenant's tree into its own and plant ACL entries
for UIDs a tenant would get later (E9). At a tenant's first work item the
worker migrates the tenant's tree, without any new capability: 10002 opens
its entries to the workspace group, the worker copies inodes with links
outside the tree, 10002 and the worker set exact ACLs on their own entries,
and only then does the tenant directory get the tenant's ACLs and a stamp.

Here the walker's steps run in-process as the test's UID (which owns the
test files); the steps as 10002 and the tenant UID run in
test_tool_isolation_integration.py.
"""

from __future__ import annotations

import asyncio
import errno
import fcntl
import json
import os
import signal
import stat
import subprocess
import threading
from typing import TYPE_CHECKING

import pytest

from codeforge import posix_acl, tool_migration, tool_process, tool_reaper, tool_state, tool_walk
from codeforge.posix_acl import Entry
from codeforge.tool_identity import ToolIsolationError
from tests.failing_writes import FailingWrites

if TYPE_CHECKING:
    from pathlib import Path

UID = os.getuid()
TOOL_UID = 20009
GROUP = posix_acl.WORKSPACE_GID


@pytest.fixture(autouse=True)
def _needs_acls(tmp_path: Path) -> None:
    try:
        posix_acl.set_acl(str(tmp_path), posix_acl.DEFAULT, posix_acl.tenant_default(TOOL_UID))
        posix_acl.remove_acl(str(tmp_path), posix_acl.DEFAULT)
    except OSError as exc:
        if exc.errno == errno.EOPNOTSUPP:
            pytest.skip("no POSIX ACLs on the test file system")
        raise


def _exact(directory: bool, owner: int, executable: bool = False) -> list[Entry]:
    x = 1 if directory or executable else 0
    return [
        Entry(posix_acl.USER_OBJ, owner),
        Entry(posix_acl.USER, 6 | x, TOOL_UID),
        Entry(posix_acl.GROUP_OBJ, 6 | x),
        Entry(posix_acl.GROUP, 6 | x, GROUP),
        Entry(posix_acl.MASK, 6 | x),
        Entry(posix_acl.OTHER, 0),
    ]


# ---------------------------------------------------------------------------
# The walker's migration modes
# ---------------------------------------------------------------------------


def test_legacy_open_gives_the_workspace_group_every_own_entry(tmp_path: Path) -> None:
    tree = tmp_path / "tenant"
    (tree / "private").mkdir(parents=True)
    (tree / "private").chmod(0o700)
    (tree / "private" / "f").write_text("x")
    (tree / "private" / "f").chmod(0o600)

    tool_walk.legacy_open(str(tree))

    for path, need in ((tree / "private", 7), (tree / "private" / "f", 6)):
        acl = posix_acl.get_acl(str(path), posix_acl.ACCESS) or []
        assert Entry(posix_acl.GROUP, need, GROUP) in acl
        assert next(e.perm for e in acl if e.tag == posix_acl.MASK) & need == need
    # No default ACL yet: the tenant UID gets nothing before the last step.
    assert posix_acl.get_acl(str(tree / "private"), posix_acl.DEFAULT) is None


def test_exact_sets_exact_acls_and_removes_planted_entries(tmp_path: Path) -> None:
    tree = tmp_path / "tenant"
    project = tree / "p1"
    project.mkdir(parents=True)
    project.chmod(0o755)
    planted = project / "planted"
    planted.write_text("x")
    # A legacy tool planted an entry for the UID another tenant gets later (E9).
    posix_acl.set_acl(
        str(planted),
        posix_acl.ACCESS,
        [Entry(posix_acl.USER_OBJ, 6), Entry(posix_acl.USER, 7, 20010), Entry(posix_acl.GROUP_OBJ, 4),
         Entry(posix_acl.MASK, 7), Entry(posix_acl.OTHER, 4)],
    )  # fmt: skip
    posix_acl.set_acl(str(project), posix_acl.DEFAULT, posix_acl.tenant_default(20010))
    script = project / "run.sh"
    script.write_text("#!/bin/sh\n")
    script.chmod(0o755)

    report = tool_walk.exact(str(tree), TOOL_UID)

    assert report.errors == []
    assert posix_acl.equal(posix_acl.get_acl(str(planted), posix_acl.ACCESS), _exact(False, 6))
    assert posix_acl.equal(posix_acl.get_acl(str(script), posix_acl.ACCESS), _exact(False, 7, executable=True))
    assert posix_acl.equal(posix_acl.get_acl(str(project), posix_acl.ACCESS), _exact(True, 7))
    assert posix_acl.equal(posix_acl.get_acl(str(project), posix_acl.DEFAULT), posix_acl.tenant_default(TOOL_UID))
    assert tool_walk.exact(str(tree), TOOL_UID).changed == 0


def test_exact_skips_an_inode_linked_outside_the_tree(tmp_path: Path) -> None:
    tree = tmp_path / "tenant"
    tree.mkdir()
    other = tmp_path / "other-tenant"
    other.mkdir()
    outside = other / "config"
    outside.write_text("theirs")
    outside.chmod(0o640)
    os.link(outside, tree / "linked")
    (tree / "twice").write_text("x")
    os.link(tree / "twice", tree / "twice-again")

    report = tool_walk.exact(str(tree), TOOL_UID)

    # Rewriting it would change the other tenant's file (revision 1's denial of service).
    assert posix_acl.get_acl(str(outside), posix_acl.ACCESS) is None
    assert stat.S_IMODE(outside.stat().st_mode) == 0o640
    assert report.linked_outside == 1
    # Links that all lie inside the tree are this tenant's alone.
    assert posix_acl.equal(posix_acl.get_acl(str(tree / "twice"), posix_acl.ACCESS), _exact(False, 6))


def test_census_counts_links_inside_the_tree(tmp_path: Path) -> None:
    tree = tmp_path / "t"
    (tree / "d").mkdir(parents=True)
    (tree / "a").write_text("x")
    os.link(tree / "a", tree / "d" / "b")
    (tmp_path / "out").write_text("y")
    os.link(tmp_path / "out", tree / "c")
    counts, report = tool_walk.census(str(tree))
    a, c = (tree / "a").stat(), (tree / "c").stat()
    assert report.unentered == 0
    assert counts[(a.st_dev, a.st_ino)] == 2
    assert counts[(c.st_dev, c.st_ino)] == 1


def test_a_directory_swapped_for_a_symlink_during_the_walk_changes_nothing_outside(tmp_path: Path) -> None:
    """Race: while the walk runs, a thread swaps a directory of the tree for a symlink into another
    tenant's tree. Every change goes through a descriptor of the listed inode: the other tree stays
    exactly as it was."""
    tree = tmp_path / "tenant"
    tree.mkdir()
    other = tmp_path / "other"
    (other / "secret").mkdir(parents=True)
    (other / "secret" / "f").write_text("theirs")
    before = {p: (p.stat().st_mode, posix_acl.get_acl(str(p), posix_acl.ACCESS)) for p in other.rglob("*")}
    swap = tree / "swap"
    stop = threading.Event()

    def swapper() -> None:
        while not stop.is_set():
            try:
                swap.mkdir()
                (swap / "x").write_text("x")
                (swap / "x").unlink()
                swap.rmdir()
                swap.symlink_to(other / "secret")
                swap.unlink()
            except OSError:
                pass

    thread = threading.Thread(target=swapper)
    thread.start()
    try:
        for _ in range(200):
            tool_walk.exact(str(tree), TOOL_UID)
            tool_walk.legacy_open(str(tree))
            tool_walk.share(str(tree))
    finally:
        stop.set()
        thread.join()
    after = {p: (p.stat().st_mode, posix_acl.get_acl(str(p), posix_acl.ACCESS)) for p in other.rglob("*")}
    assert after == before
    assert posix_acl.get_acl(str(other / "secret"), posix_acl.DEFAULT) is None


# ---------------------------------------------------------------------------
# Unsharing hard links (the worker's step)
# ---------------------------------------------------------------------------


def test_unshare_copies_inodes_linked_from_outside(tmp_path: Path) -> None:
    tree = tmp_path / "tenant"
    (tree / "p").mkdir(parents=True)
    theirs = tmp_path / "other" / ".git" / "config"
    theirs.parent.mkdir(parents=True)
    theirs.write_text("[core]\n")
    theirs.chmod(0o664)
    os.link(theirs, tree / "p" / "planted")
    (tree / "p" / "own").write_text("mine")
    os.link(tree / "p" / "own", tree / "p" / "own-too")
    original = theirs.stat()

    copied = tool_migration.unshare_links(str(tree))

    assert copied == 1
    planted = (tree / "p" / "planted").stat()
    assert (planted.st_dev, planted.st_ino) != (original.st_dev, original.st_ino)
    assert (tree / "p" / "planted").read_text() == "[core]\n"
    assert stat.S_IMODE(planted.st_mode) & 0o700 == 0o600
    assert theirs.stat().st_nlink == 1
    # Writing the tenant's copy leaves the other tenant's file alone (E9).
    (tree / "p" / "planted").write_text("changed")
    assert theirs.read_text() == "[core]\n"
    # Links inside the tree stay links.
    assert (tree / "p" / "own").stat().st_nlink == 2
    assert not [p for p in (tree / "p").iterdir() if p.name.startswith(".cf-unshare")]


def test_the_copy_leaves_an_entry_of_the_same_temporary_name_alone(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """KI-96 review: a temporary name made of the inode and the PID (1 in every container) could be
    taken; the copy then failed and its cleanup removed the other entry."""
    monkeypatch.setattr(tool_migration.os, "getpid", lambda: 1)
    tree = tmp_path / "tenant"
    tree.mkdir()
    theirs = tmp_path / "other"
    theirs.write_text("theirs")
    os.link(theirs, tree / "planted")
    taken = tree / f".cf-unshare-{theirs.stat().st_ino}-1"
    taken.write_text("not the copy's")

    assert tool_migration.unshare_links(str(tree)) == 1
    assert taken.read_text() == "not the copy's"
    assert theirs.stat().st_nlink == 1


# ---------------------------------------------------------------------------
# Stamps, locks, processes, rollback
# ---------------------------------------------------------------------------


@pytest.fixture
def root(tmp_path: Path) -> Path:
    path = tmp_path / "workspaces"
    path.mkdir()
    path.chmod(0o2771)
    tool_state.ensure_state_dirs(str(path))
    return path


def test_a_tenant_stamp_names_the_uid_and_the_directory(root: Path) -> None:
    tenant = root / "tenant-a"
    tenant.mkdir()
    info = tenant.stat()
    assert not tool_migration.stamp_ok(str(root), "tenants", "tenant-a", TOOL_UID, info)
    tool_migration.write_stamp(str(root), "tenants", "tenant-a", TOOL_UID, info)
    assert tool_migration.stamp_ok(str(root), "tenants", "tenant-a", TOOL_UID, info)
    # Another UID or a replaced directory makes it stale.
    assert not tool_migration.stamp_ok(str(root), "tenants", "tenant-a", TOOL_UID + 1, info)
    tenant.rename(root / "moved")
    (root / "tenant-a").mkdir()
    assert not tool_migration.stamp_ok(str(root), "tenants", "tenant-a", TOOL_UID, (root / "tenant-a").stat())


@pytest.mark.parametrize("fail_after", [0, 1])
def test_a_stamp_that_cannot_be_written_leaves_no_temporary_file(
    fail_after: int, root: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """KI-96 review: a write that failed after the O_EXCL create (ENOSPC, EIO) left the random
    .<name>.<hex>.tmp file in the state directory."""
    tenant = root / "tenant-a"
    tenant.mkdir()
    FailingWrites(root, max_bytes=4, fail_after=fail_after).install(monkeypatch)
    with pytest.raises(OSError, match="No space"):
        tool_migration.write_stamp(str(root), "tenants", "tenant-a", TOOL_UID, tenant.stat())
    assert list((root / ".codeforge" / "tenants").iterdir()) == []


def test_an_unshared_copy_written_short_is_complete(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    tree = tmp_path / "tenant"
    tree.mkdir()
    theirs = tmp_path / "other"
    theirs.write_bytes(b"0123456789" * 10)
    os.link(theirs, tree / "planted")
    FailingWrites(tree, max_bytes=7).install(monkeypatch)

    assert tool_migration.unshare_links(str(tree)) == 1
    assert (tree / "planted").read_bytes() == b"0123456789" * 10
    assert theirs.stat().st_nlink == 1


@pytest.mark.parametrize("forgery", ["symlink", "fifo", "oversize", "foreign"])
def test_a_forged_stamp_is_ignored(forgery: str, root: Path, tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    tenant = root / "tenant-a"
    tenant.mkdir()
    info = tenant.stat()
    stamp = root / ".codeforge" / "tenants" / "tenant-a"
    content = f"1 {TOOL_UID} {info.st_dev} {info.st_ino}\n"
    if forgery == "symlink":
        (tmp_path / "forged").write_text(content)
        stamp.symlink_to(tmp_path / "forged")
    elif forgery == "fifo":
        os.mkfifo(stamp)
    elif forgery == "oversize":
        stamp.write_text(content + " " * 100)
    else:
        stamp.write_text(content)
        monkeypatch.setattr(tool_state, "worker_uid", lambda: UID + 1)
    assert not tool_migration.stamp_ok(str(root), "tenants", "tenant-a", TOOL_UID, info)


def test_the_tenant_lock_excludes_across_descriptors(root: Path) -> None:
    first = tool_migration.TenantLock(str(root), "tenant-a")
    second = tool_migration.TenantLock(str(root), "tenant-a")
    try:
        assert first.try_shared()
        assert second.try_shared()
        assert not second.try_exclusive()
        first.release()
        assert second.try_exclusive()
        assert not first.try_shared()
        second.downgrade()
        assert first.try_shared()
    finally:
        first.close()
        second.close()
    lock_file = root / ".codeforge" / "locks" / "tenant-a"
    assert stat.S_IMODE(lock_file.stat().st_mode) == 0o600


def test_a_planted_lock_file_is_refused(root: Path, tmp_path: Path) -> None:
    (tmp_path / "target").write_text("")
    (root / ".codeforge" / "locks" / "tenant-a").symlink_to(tmp_path / "target")
    with pytest.raises(ToolIsolationError):
        tool_migration.TenantLock(str(root), "tenant-a")


def test_the_current_process_is_found() -> None:
    assert tool_reaper.processes_of({UID})[os.getpid()] == UID


def test_two_workers_that_detect_a_rollback_at_once_both_go_on(root: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """KI-96 review: replicas start together. The second one must neither fail on the name the first
    one used in the same second, nor because the first one already moved the state aside."""
    (root / tool_migration.ROOT_STAMP).write_text("2\n")
    monkeypatch.setattr(tool_migration.time, "time", lambda: 1700000000.0)
    taken = root / ".codeforge.rollback-1700000000"
    taken.mkdir()
    (taken / "kept").write_text("the first worker's\n")
    assert tool_migration.detect_rollback(str(root))
    assert (taken / "kept").read_text() == "the first worker's\n"
    assert len([p for p in root.iterdir() if p.name.startswith(".codeforge.rollback-")]) == 2

    tool_state.ensure_state_dirs(str(root))
    (root / tool_migration.ROOT_STAMP).write_text("2\n")
    real_rename = os.rename

    def first_worker_was_faster(src: str, dst: str, **kwargs: int) -> None:
        if src == tool_state.STATE_DIR:
            real_rename(root / src, root / ".codeforge.rollback-other")
        real_rename(src, dst, **kwargs)

    monkeypatch.setattr(tool_migration.os, "rename", first_worker_was_faster)
    assert tool_migration.detect_rollback(str(root))
    assert (root / tool_migration.ROOT_STAMP).read_text().strip() == tool_migration.ROOT_STAMP_VERSION


@pytest.mark.parametrize("write", ["root stamp", "rollback check", "tenant stamp"])
def test_a_stamp_leaves_another_workers_temporary_file_alone(
    write: str, root: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """KI-96 review: every worker replica is PID 1 of its container. A temporary name made of the PID
    let one replica unlink another's file between its write and its rename (rename ENOENT), or find
    it there ("exists"), at startup: the replica stayed not ready."""
    tool_migration.write_root_stamp(str(root))  # this worker's own earlier start
    monkeypatch.setattr(tool_migration.os, "getpid", lambda: 1)
    tenant = root / "tenant-a"
    tenant.mkdir()
    if write == "tenant stamp":
        directory, name = root / ".codeforge" / "tenants", "tenant-a"
    else:
        directory, name = root, tool_migration.ROOT_STAMP
    others = directory / f".{name}.1.tmp"
    others.write_text("another worker's\n")
    if write == "root stamp":
        tool_migration.write_root_stamp(str(root))
    elif write == "rollback check":
        assert not tool_migration.detect_rollback(str(root))
    else:
        tool_migration.write_stamp(str(root), "tenants", "tenant-a", TOOL_UID, tenant.stat())
    assert others.read_text() == "another worker's\n"
    assert sorted(p.name for p in directory.iterdir() if p.name.endswith(".tmp")) == [others.name]


@pytest.mark.parametrize("tamper", ["group-bits", "old-stamp", "acl"])
def test_a_rollback_moves_the_state_aside(tamper: str, root: Path) -> None:
    tool_migration.write_root_stamp(str(root))
    tool_state.bind_tool_uid(str(root), TOOL_UID, "tenant-a")
    state = root / ".codeforge"
    if tamper == "group-bits":  # the old worker's walk opened it to the workspace group
        state.chmod(0o2770)
    elif tamper == "old-stamp":
        stamp = root / tool_migration.ROOT_STAMP
        stamp.unlink()
        stamp.write_text("2\n")
    else:
        posix_acl.set_acl(str(state), posix_acl.ACCESS, posix_acl.home_access(TOOL_UID))

    assert tool_migration.detect_rollback(str(root))

    aside = [p.name for p in root.iterdir() if p.name.startswith(".codeforge.rollback-")]
    assert len(aside) == 1
    assert not state.exists()
    tool_state.ensure_state_dirs(str(root))
    assert tool_state.bound_tenant(str(root), TOOL_UID) is None
    assert (root / tool_migration.ROOT_STAMP).read_text().strip() == tool_migration.ROOT_STAMP_VERSION


def test_no_rollback_on_a_fresh_or_current_root(root: Path) -> None:
    (root / tool_migration.ROOT_STAMP).write_text("2\n")  # the KI-71 worker's, before the upgrade
    (root / ".codeforge").rename(root / "gone")
    assert not tool_migration.detect_rollback(str(root))
    assert (root / tool_migration.ROOT_STAMP).read_text().strip() == "3"
    tool_state.ensure_state_dirs(str(root))
    assert not tool_migration.detect_rollback(str(root))


async def test_waiting_for_the_tenants_other_work_is_bounded(root: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(tool_migration, "LOCK_WAIT_SECONDS", 0.3)
    holder = tool_migration.TenantLock(str(root), "tenant-a")
    assert holder.try_shared()
    waiting = tool_migration.TenantLock(str(root), "tenant-a")
    try:
        with pytest.raises(ToolIsolationError, match="waiting for the tenant's other work"):
            await tool_migration.acquire(waiting, needs_migration=lambda: True, migrate=lambda: None)
    finally:
        holder.close()
        waiting.close()


async def test_a_migration_another_work_item_finished_is_not_repeated(root: Path) -> None:
    migrated: list[int] = []
    done = asyncio.Event()

    def needs() -> bool:
        return not migrated

    def migrate() -> None:
        migrated.append(1)

    first = tool_migration.TenantLock(str(root), "tenant-a")
    second = tool_migration.TenantLock(str(root), "tenant-a")
    try:
        await tool_migration.acquire(first, needs_migration=needs, migrate=migrate)
        done.set()
        # The second work item finds the tree migrated and shares the lock.
        await tool_migration.acquire(second, needs_migration=needs, migrate=migrate)
        assert migrated == [1]
        assert fcntl.flock(second.fd, fcntl.LOCK_SH | fcntl.LOCK_NB) is None
    finally:
        first.close()
        second.close()


# ---------------------------------------------------------------------------
# The owner-run walks end in bounded time (KI-96 review)
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("returncode", [-signal.SIGKILL, -signal.SIGTERM, 2, 3])
def test_a_killed_or_failed_migration_walk_fails_the_migration(
    returncode: int, monkeypatch: pytest.MonkeyPatch
) -> None:
    """A walk that was killed (it timed out) left the tree half done: the migration must not go on."""
    calls: list[float | None] = []

    def walker(_identity: object, args: list[str], *, timeout: float | None = None) -> subprocess.CompletedProcess[str]:
        calls.append(timeout)
        return subprocess.CompletedProcess(args, returncode, "", "timed out after 3600 s: killed")

    monkeypatch.setattr(tool_process, "run_walker", walker)
    with pytest.raises(ToolIsolationError, match="legacy-open"):
        tool_migration._run_legacy_walk("tenant-a", "/w/tenant-a", ["legacy-open", "/w/tenant-a"])
    # A whole tenant tree: the long bound, not the per-call helpers' one.
    assert calls == [tool_process.MIGRATION_WALK_TIMEOUT_SECONDS]


@pytest.mark.parametrize("returncode", [0, 1])
def test_a_walk_that_ran_through_lets_the_migration_go_on(returncode: int, monkeypatch: pytest.MonkeyPatch) -> None:
    """Exit 1: some entries could not be checked (logged); the walk itself ran through."""

    def walker(_identity: object, args: list[str], *, timeout: float | None = None) -> subprocess.CompletedProcess[str]:
        return subprocess.CompletedProcess(args, returncode, '{"checked": 1}', "")

    monkeypatch.setattr(tool_process, "run_walker", walker)
    done = tool_migration._run_legacy_walk("tenant-a", "/w/tenant-a", ["legacy-open", "/w/tenant-a"])
    assert done.exit == returncode


# ---------------------------------------------------------------------------
# A walk that skipped subtrees fails the migration (KI-223, R8-8)
# ---------------------------------------------------------------------------
# Entries in a skipped subtree keep their legacy ACLs and planted entries; a
# tree stamped migrated anyway is never migrated again.


def test_a_legacy_walk_that_skipped_subtrees_fails_the_migration(monkeypatch: pytest.MonkeyPatch) -> None:
    report = '{"checked": 5, "unentered": 2, "errors": ["/w/tenant-a/node_modules/pkg7: cannot be entered"]}'

    def walker(_identity: object, args: list[str], *, timeout: float | None = None) -> subprocess.CompletedProcess[str]:
        return subprocess.CompletedProcess(args, 1, report, "")

    monkeypatch.setattr(tool_process, "run_walker", walker)
    with pytest.raises(ToolIsolationError, match=r"legacy-exact.*2 subtrees.*pkg7"):
        tool_migration._run_legacy_walk("tenant-a", "/w/tenant-a", ["legacy-exact", "/w/tenant-a", "20009"])


def test_a_legacy_walk_report_that_cannot_be_read_fails_the_migration(monkeypatch: pytest.MonkeyPatch) -> None:
    def walker(_identity: object, args: list[str], *, timeout: float | None = None) -> subprocess.CompletedProcess[str]:
        return subprocess.CompletedProcess(args, 1, "not json", "")

    monkeypatch.setattr(tool_process, "run_walker", walker)
    with pytest.raises(ToolIsolationError, match="legacy-open"):
        tool_migration._run_legacy_walk("tenant-a", "/w/tenant-a", ["legacy-open", "/w/tenant-a"])


def _refuse_entering(monkeypatch: pytest.MonkeyPatch, refused: str) -> None:
    real_open = tool_walk._open_subdir

    def open_subdir(dir_fd: int, name: str, listed: os.stat_result) -> int | None:
        return None if name == refused else real_open(dir_fd, name, listed)

    monkeypatch.setattr(tool_walk, "_open_subdir", open_subdir)


def test_unsharing_fails_when_a_subtree_was_skipped(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    tree = tmp_path / "tenant"
    (tree / "skipped" / "inner").mkdir(parents=True)
    _refuse_entering(monkeypatch, "skipped")

    with pytest.raises(ToolIsolationError, match="skipped"):
        tool_migration.unshare_links(str(tree))


def test_unsharing_fails_when_the_census_missed_a_subtree(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """A census that missed a subtree takes links into it for links outside the tree (KI-223 review)."""
    tree = tmp_path / "tenant"
    tree.mkdir()
    missed = tool_walk.Report(unentered=1, errors=[f"{tree}/hidden: cannot be entered"])
    monkeypatch.setattr(tool_walk, "census", lambda _root: ({}, missed))

    with pytest.raises(ToolIsolationError, match=r"census.*hidden"):
        tool_migration.unshare_links(str(tree))


def test_the_worker_walk_fails_the_migration_when_a_subtree_was_skipped(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    tree = tmp_path / "tenant"
    (tree / "p" / "skipped").mkdir(parents=True)

    def walker(_identity: object, args: list[str], *, timeout: float | None = None) -> subprocess.CompletedProcess[str]:
        return subprocess.CompletedProcess(args, 0, '{"checked": 1, "unentered": 0, "errors": []}', "")

    monkeypatch.setattr(tool_process, "run_walker", walker)
    monkeypatch.setattr(tool_reaper, "reap", lambda _uid: 0)
    monkeypatch.setattr(tool_reaper, "running_processes_of", lambda _uids: {})
    monkeypatch.setattr(tool_migration, "unshare_links", lambda _tree: 0)
    _refuse_entering(monkeypatch, "skipped")

    with pytest.raises(ToolIsolationError, match="skipped"):
        tool_migration.migrate_tree(str(tree), "tenant-a", TOOL_UID, include_root=False)


# ---------------------------------------------------------------------------
# Directories of other owners the legacy walks could not list (KI-223 review)
# ---------------------------------------------------------------------------
# The Go Core makes .git/codeforge/patches 0700: under the tenant default ACL
# 10002 cannot list it, and the migration failed for good. Only its owner
# could have locked 10002 out; the worker (the Go Core's UID) checks it.


def _reporting_walker(report: dict[str, object]) -> object:
    def walker(_identity: object, args: list[str], *, timeout: float | None = None) -> subprocess.CompletedProcess[str]:
        return subprocess.CompletedProcess(args, 0, json.dumps(report), "")

    return walker


def test_a_legacy_walk_lists_the_directories_of_other_owners_it_skipped(monkeypatch: pytest.MonkeyPatch) -> None:
    skipped = "/w/tenant-a/p/.git/codeforge/patches"
    monkeypatch.setattr(tool_process, "run_walker", _reporting_walker({"unentered": 0, "foreign_unentered": [skipped]}))

    done = tool_migration._run_legacy_walk("tenant-a", "/w/tenant-a", ["legacy-open", "/w/tenant-a"])

    assert done.foreign_unentered == (skipped,)


@pytest.mark.parametrize("listed", ["/w/tenant-a/x", [1], None])
def test_an_unreadable_list_of_skipped_directories_fails_the_migration(
    listed: object, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr(tool_process, "run_walker", _reporting_walker({"unentered": 0, "foreign_unentered": listed}))

    with pytest.raises(ToolIsolationError, match="legacy-open"):
        tool_migration._run_legacy_walk("tenant-a", "/w/tenant-a", ["legacy-open", "/w/tenant-a"])


def _migrate_skipping(tree: Path, skipped: list[str], monkeypatch: pytest.MonkeyPatch) -> None:
    """migrate_tree with legacy walks that could not list *skipped*."""
    monkeypatch.setattr(tool_process, "run_walker", _reporting_walker({"unentered": 0, "foreign_unentered": skipped}))
    monkeypatch.setattr(tool_reaper, "reap", lambda _uid: 0)
    monkeypatch.setattr(tool_reaper, "running_processes_of", lambda _uids: {})
    monkeypatch.setattr(tool_migration, "unshare_links", lambda _tree: 0)
    tool_migration.migrate_tree(str(tree), "tenant-a", TOOL_UID, include_root=False)


def _patches(tmp_path: Path) -> tuple[Path, Path]:
    tree = tmp_path / "tenant"
    patches = tree / "p" / ".git" / "codeforge" / "patches"
    patches.mkdir(parents=True, mode=0o700)
    (patches / "run-1.patch").write_text("diff\n")
    return tree, patches


def test_a_directory_of_the_go_core_the_legacy_walks_skipped_is_checked_by_the_worker(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    tree, patches = _patches(tmp_path)

    _migrate_skipping(tree, [str(patches)], monkeypatch)  # nothing of 10002 there: the migration goes on


def test_an_entry_of_10002_in_a_skipped_directory_fails_the_migration(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """10002's walks did not reach it: its ACL (planted entries included) is the legacy one."""
    tree, patches = _patches(tmp_path)
    monkeypatch.setattr(tool_migration, "LEGACY_TOOL_UID", UID)  # the test's files play 10002's

    with pytest.raises(ToolIsolationError, match=r"patches.*run-1\.patch"):
        _migrate_skipping(tree, [str(patches)], monkeypatch)


def test_a_skipped_directory_removed_meanwhile_is_nothing_to_check(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    tree, patches = _patches(tmp_path)

    _migrate_skipping(tree, [f"{patches}-gone"], monkeypatch)


@pytest.mark.parametrize("relative", ["../other", "p/./x", "p//x", ""])
def test_a_skipped_directory_outside_the_tree_fails_the_migration(
    relative: str, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    tree, _ = _patches(tmp_path)
    (tmp_path / "other").mkdir()

    with pytest.raises(ToolIsolationError, match="not inside"):
        _migrate_skipping(tree, [f"{tree}/{relative}"], monkeypatch)


def test_a_directory_the_worker_cannot_enter_fails_its_walk_whoever_owns_it(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """The worker's own walks check everything: nobody checks what they skip."""
    tree, _ = _patches(tmp_path)
    skipped = tool_walk.Report(foreign_unentered=[f"{tree}/p/private"])
    monkeypatch.setattr(tool_walk, "exact", lambda *_args, **_kwargs: skipped)

    with pytest.raises(ToolIsolationError, match="private"):
        _migrate_skipping(tree, [], monkeypatch)
