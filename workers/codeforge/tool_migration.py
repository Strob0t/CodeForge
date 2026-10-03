"""Migrating workspace trees from before the upgrade (KI-96 D9).

Before the upgrade every tenant's tools ran as the shared tool user 10002 in
the workspace group 10010: its files are in every tenant tree, and a legacy
tool could hard-link a file of another tenant's tree into its own or plant
ACL entries for UIDs tenants get later (E9). A tenant's tool UID gets a path
into its tree only once the tree is migrated, at the tenant's first work
item, with no capability beyond the worker's (SETUID, SETGID, KILL): every
change is made by the entry's owner.

1. As 10002 (``tool_walk.py legacy-open``): its entries get ``g:10010``, so
   the worker can walk the whole tree.
2. As the worker: a census of the hard links inside the tree, and
3. a private copy of every inode with links outside it, renamed over the
   name: the other tree's file is no longer reachable from this one.
4. As 10002 (``tool_walk.py legacy-exact``): exact ACLs for the tenant's
   UID on its entries (planted entries go); an inode still linked outside
   is skipped and logged.
5. As the worker: the same on its own entries; last, the tenant directory
   gets the tenant's ACLs (2770, ``u:T:--x``, the default ACL).
6. A stamp ``<root>/.codeforge/tenants/<tenant>`` = ``1 <T> <dev> <ino>``
   records it; a stamp with another UID or inode is stale and the tree is
   migrated again (self-healing; adopted workspaces the worker owns get
   ``.codeforge/adopted/<sha256 of the path>``).

A migration needs the tenant's exclusive lock (``.codeforge/locks/<tenant>``,
flock): every work item holds the shared lock while it runs, in every worker
on the volume, so no tool process of the tenant runs meanwhile. Waiting is
bounded (LOCK_WAIT_SECONDS). A process of 10002 or of the tenant's UID in
this container refuses the migration.

A worker that rolled back to the KI-71 image walks the whole root at its
start and opens the worker's state to the workspace group. The root stamp
(``.codeforge-workspace-sharing``: "3") and the state directory's own
check detect that at the next start: the state goes aside
(``.codeforge.rollback-<time>-<random>``) and every tenant migrates again.
"""

from __future__ import annotations

import asyncio
import contextlib
import errno
import fcntl
import hashlib
import logging
import os
import stat
import time
from typing import TYPE_CHECKING

from codeforge import posix_acl, tool_reaper, tool_state, tool_walk
from codeforge.tool_identity import (
    LEGACY_TOOL_UID,
    WORKSPACE_GID,
    ToolIdentity,
    ToolIsolationError,
    in_tenant_area,
    new_work_id,
)

if TYPE_CHECKING:
    from collections.abc import Callable

logger = logging.getLogger(__name__)

ROOT_STAMP = ".codeforge-workspace-sharing"
ROOT_STAMP_VERSION = "3"
STAMP_VERSION = "1"
TENANTS = "tenants"
ADOPTED = "adopted"
LOCK_WAIT_SECONDS = 600.0
_LOCK_POLL_SECONDS = 0.2
_STAMP_MAX_BYTES = 64
_FILE_FLAGS = os.O_NOFOLLOW | os.O_CLOEXEC


# ---------------------------------------------------------------------------
# Stamps
# ---------------------------------------------------------------------------


def _stamp_text(uid: int, info: os.stat_result) -> str:
    return f"{STAMP_VERSION} {uid} {info.st_dev} {info.st_ino}\n"


def adopted_key(path: str) -> str:
    return hashlib.sha256(os.path.normpath(path).encode()).hexdigest()


def stamp_ok(root: str, kind: str, name: str, uid: int, info: os.stat_result) -> bool:
    """Whether the stamp of *name* records tool UID *uid* and the directory *info* (dev, inode)."""
    try:
        with tool_state.state_dir(root, kind) as fd:
            data = tool_state.read_small(fd, name, _STAMP_MAX_BYTES)
    except (ToolIsolationError, OSError) as exc:
        logger.warning("ignoring the migration stamp %s/%s: %s", kind, name, exc)
        return False
    return data is not None and data.decode(errors="replace") == _stamp_text(uid, info)


def _replace_file(dir_fd: int, name: str, data: bytes) -> None:
    """Write *name* in *dir_fd* atomically: a new file renamed over it.

    The temporary name is random: worker replicas write the same stamps.
    """
    tmp = tool_state.temporary_name(f".{name}.") + ".tmp"
    if not tool_state.write_new(dir_fd, tmp, data):
        raise ToolIsolationError(f"cannot write {name}: {tmp} exists")
    try:
        os.rename(tmp, name, src_dir_fd=dir_fd, dst_dir_fd=dir_fd)
    except BaseException:
        with contextlib.suppress(OSError):
            os.unlink(tmp, dir_fd=dir_fd)
        raise


def write_stamp(root: str, kind: str, name: str, uid: int, info: os.stat_result) -> None:
    with tool_state.state_dir(root, kind) as fd:
        _replace_file(fd, name, _stamp_text(uid, info).encode())


def write_root_stamp(root: str) -> None:
    fd = tool_state.open_root(root)
    try:
        _replace_file(fd, ROOT_STAMP, (ROOT_STAMP_VERSION + "\n").encode())
    finally:
        os.close(fd)


def _root_stamp(root_fd: int) -> str:
    try:
        data = tool_state.read_small(root_fd, ROOT_STAMP, _STAMP_MAX_BYTES)
    except (ToolIsolationError, OSError):
        return ""
    return "" if data is None else data.decode(errors="replace").strip()


def detect_rollback(root: str) -> bool:
    """Move the worker's state aside when an older worker ran on the volume; True if it did.

    An older worker (KI-71) wrote root stamp "2" and opened the state
    directory to the workspace group, where legacy tools could change it.
    Then the state (UID bindings, migration stamps) is not trusted: it moves
    to ``.codeforge.rollback-<time>-<random>`` and every tenant migrates again.
    """
    root_fd = tool_state.open_root(root)
    try:
        state = f"{root}/{tool_state.STATE_DIR}"
        try:
            fd = tool_state.open_dir_at(root_fd, tool_state.STATE_DIR)
        except FileNotFoundError:
            problem = None
        except OSError as exc:
            problem = f"{state} is not the worker's directory ({exc.strerror})"
        else:
            try:
                problem = tool_state.private_dir_problem(fd, state)
            finally:
                os.close(fd)
            if not problem and _root_stamp(root_fd) != ROOT_STAMP_VERSION:
                problem = f"the root stamp is {_root_stamp(root_fd)!r}, not {ROOT_STAMP_VERSION!r}"
        if problem:
            # Random as well as dated: worker replicas that start together may both get here.
            aside = tool_state.temporary_name(f"{tool_state.STATE_DIR}.rollback-{int(time.time())}-")
            try:
                os.rename(tool_state.STATE_DIR, aside, src_dir_fd=root_fd, dst_dir_fd=root_fd)
                moved_to = f"{root}/{aside}"
            except FileNotFoundError:
                moved_to = "a directory another worker chose"
            logger.warning(
                "an older worker ran on the workspaces volume (%s): its state moved to %s; every tenant's "
                "workspaces are migrated again at their next work item",
                problem,
                moved_to,
            )
        _replace_file(root_fd, ROOT_STAMP, (ROOT_STAMP_VERSION + "\n").encode())
        return bool(problem)
    finally:
        os.close(root_fd)


# ---------------------------------------------------------------------------
# Locks
# ---------------------------------------------------------------------------


class TenantLock:
    """The cross-worker lock of one tenant's work (flock on ``.codeforge/locks/<tenant>``).

    Each work item opens its own descriptor: flock locks of separate
    descriptors exclude each other also within one process.
    """

    def __init__(self, root: str, tenant_id: str) -> None:
        if not tool_state.is_name(tenant_id):
            raise ToolIsolationError(f"{tenant_id!r} is not a tenant ID")
        with tool_state.state_dir(root, "locks") as dir_fd:
            try:
                fd = os.open(tenant_id, os.O_RDWR | os.O_CREAT | os.O_NONBLOCK | _FILE_FLAGS, 0o600, dir_fd=dir_fd)
            except OSError as exc:
                raise ToolIsolationError(f"the lock of tenant {tenant_id} cannot be opened: {exc.strerror}") from exc
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != tool_state.worker_uid():
            os.close(fd)
            raise ToolIsolationError(f"the lock of tenant {tenant_id} is not the worker's file")
        self.fd = fd
        self.tenant_id = tenant_id

    def _try(self, operation: int) -> bool:
        try:
            fcntl.flock(self.fd, operation | fcntl.LOCK_NB)
        except BlockingIOError:
            return False
        return True

    def try_shared(self) -> bool:
        return self._try(fcntl.LOCK_SH)

    def try_exclusive(self) -> bool:
        return self._try(fcntl.LOCK_EX)

    def downgrade(self) -> None:
        fcntl.flock(self.fd, fcntl.LOCK_SH)

    def release(self) -> None:
        fcntl.flock(self.fd, fcntl.LOCK_UN)

    def close(self) -> None:
        with contextlib.suppress(OSError):
            os.close(self.fd)


async def acquire(lock: TenantLock, *, needs_migration: Callable[[], bool], migrate: Callable[[], None]) -> None:
    """Hold *lock* shared for a work item; migrate first, exclusively, when the tree needs it.

    A work item that finds the tree migrated by another one meanwhile only
    shares the lock. Waiting is bounded by LOCK_WAIT_SECONDS. The migration
    runs in a thread: the event loop, heartbeats and /health go on.
    """
    deadline = time.monotonic() + LOCK_WAIT_SECONDS
    while True:
        if needs_migration():
            if lock.try_exclusive():
                try:
                    if needs_migration():
                        await asyncio.to_thread(migrate)
                except BaseException:
                    lock.release()
                    raise
                lock.downgrade()
                return
        elif lock.try_shared():
            if not needs_migration():
                return
            lock.release()
            continue
        if time.monotonic() >= deadline:
            raise ToolIsolationError(
                f"tenant workspace migration is waiting for the tenant's other work (tenant {lock.tenant_id}, "
                f"{LOCK_WAIT_SECONDS:.0f} s): retry once it ended"
            )
        await asyncio.sleep(_LOCK_POLL_SECONDS)


# ---------------------------------------------------------------------------
# The worker's steps
# ---------------------------------------------------------------------------


def _copy_over(dir_fd: int, name: str, listed: os.stat_result) -> None:
    """Replace *name* by a copy of its content (a new inode of the worker's), never through a symlink."""
    src = os.open(name, os.O_RDONLY | os.O_NONBLOCK | _FILE_FLAGS, dir_fd=dir_fd)
    tmp = tool_state.temporary_name(f".cf-unshare-{listed.st_ino}-")
    created = False
    try:
        info = os.fstat(src)
        if (info.st_dev, info.st_ino) != (listed.st_dev, listed.st_ino) or not stat.S_ISREG(info.st_mode):
            raise OSError(errno.ESTALE, "replaced meanwhile")
        mode = stat.S_IMODE(info.st_mode) & 0o777
        dst = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_EXCL | _FILE_FLAGS, 0o600, dir_fd=dir_fd)
        created = True
        try:
            while chunk := os.read(src, 1 << 20):
                os.write(dst, chunk)
            os.fchmod(dst, mode)
        finally:
            os.close(dst)
        os.rename(tmp, name, src_dir_fd=dir_fd, dst_dir_fd=dir_fd)
    except BaseException:
        if created:  # never an entry this copy did not make
            with contextlib.suppress(OSError):
                os.unlink(tmp, dir_fd=dir_fd)
        raise
    finally:
        os.close(src)


def unshare_links(tree: str) -> int:
    """Copy every regular file of *tree* whose inode has links outside it (D9 step 3); how many were copied."""
    inside = tool_walk.census(tree)
    copied = 0

    def visit(dir_fd: int, name: str, info: os.stat_result, report: tool_walk.Report) -> None:
        nonlocal copied
        key = (info.st_dev, info.st_ino)
        if not stat.S_ISREG(info.st_mode) or info.st_nlink <= inside.get(key, 1):
            return
        try:
            _copy_over(dir_fd, name, info)
        except OSError as exc:
            report.error(f"{name}: {exc.strerror or exc}")
            return
        inside[key] = inside.get(key, 1) - 1
        copied += 1

    report = tool_walk.walk(tree, visit, tool_walk.Report())
    if report.errors:
        logger.warning("could not unshare every hard link in %s: %s", tree, "; ".join(report.errors))
    return copied


def legacy_identity(tenant_id: str, tree: str) -> ToolIdentity:
    """The retired shared tool user, in the workspace group: it changes its own entries in *tree*."""
    return ToolIdentity(
        tenant_id=tenant_id,
        uid=LEGACY_TOOL_UID,
        home="",
        work_id=new_work_id(),
        workspace=tree,
        groups=(WORKSPACE_GID,),
    )


def _run_legacy_walk(tenant_id: str, tree: str, args: list[str]) -> dict[str, object]:
    from codeforge import tool_process

    done = tool_process.run_walker(
        legacy_identity(tenant_id, tree), args, timeout=tool_process.MIGRATION_WALK_TIMEOUT_SECONDS
    )
    # 1: some entries could not be checked (logged below); anything else, a killed walk (it timed
    # out) included, left the tree half done.
    if done.returncode not in (0, 1):
        raise ToolIsolationError(
            f"the migration walk {args[0]} of {tree} failed (exit {done.returncode}): {done.stderr.strip()[-500:]}"
        )
    if done.returncode:
        logger.warning("the migration walk %s of %s left entries: %s", args[0], tree, done.stdout.strip()[-1000:])
    return {"walk": args[0], "exit": done.returncode, "report": done.stdout.strip()}


def migrate_tree(tree: str, tenant_id: str, uid: int, *, include_root: bool) -> None:
    """Steps 1-5 on *tree* for tool UID *uid*; the caller sets the top directory and the stamp."""
    started = time.monotonic()
    # The tenant's exclusive lock is held: no worker runs its work, what is
    # left of its processes here are leftovers (D10).
    tool_reaper.reap(uid)
    running = tool_reaper.running_processes_of({LEGACY_TOOL_UID, uid})
    if running:
        raise ToolIsolationError(
            f"the workspaces of tenant {tenant_id} cannot be migrated while processes of uid "
            f"{sorted(set(running.values()))} run (pids {sorted(running)[:10]}): stop the old worker first"
        )
    opened = _run_legacy_walk(tenant_id, tree, ["legacy-open", tree])
    copied = unshare_links(tree)
    exact_legacy = _run_legacy_walk(tenant_id, tree, ["legacy-exact", tree, str(uid)])
    own = tool_walk.exact(tree, uid, include_root=include_root)
    if own.errors:
        logger.warning("could not set the ACLs of every worker entry in %s: %s", tree, "; ".join(own.errors))
    logger.info(
        "migrated the workspaces of tenant %s for tool uid %d in %.1f s: %s; %d hard links copied; %s; "
        "worker entries %d checked, %d changed, %d still linked outside",
        tenant_id,
        uid,
        time.monotonic() - started,
        opened["report"],
        copied,
        exact_legacy["report"],
        own.checked,
        own.changed,
        own.linked_outside,
    )


def _tenant_dir_fd(root: str, tenant_id: str) -> int:
    root_fd = tool_state.open_root(root)
    try:
        return tool_state.open_dir_at(root_fd, tenant_id)
    finally:
        os.close(root_fd)


def needs_migration(root: str, identity: ToolIdentity) -> bool:
    """Whether the work item's tree must be migrated first (a refused tenant directory raises)."""
    workspace = identity.workspace
    if not workspace:
        return False
    if in_tenant_area(root, workspace):
        check = tool_state.check_tenant_dir(root, identity.tenant_id, identity.uid)
        if check.state is tool_state.TenantDir.REFUSED:
            raise ToolIsolationError(f"tool work of tenant {identity.tenant_id} refused: {check.reason}")
        if check.state is tool_state.TenantDir.MIGRATE:
            return True
        fd = _tenant_dir_fd(root, identity.tenant_id)
        try:
            info = os.fstat(fd)
        finally:
            os.close(fd)
        return not stamp_ok(root, TENANTS, identity.tenant_id, identity.uid, info)
    fd = os.open(workspace, os.O_RDONLY | os.O_DIRECTORY | _FILE_FLAGS)
    try:
        info = os.fstat(fd)
    finally:
        os.close(fd)
    if info.st_uid != tool_state.worker_uid():
        return False  # an operator's directory: their setfacl -R prepared it
    return not stamp_ok(root, ADOPTED, adopted_key(workspace), identity.uid, info)


def migrate(root: str, identity: ToolIdentity) -> None:
    """Migrate the work item's tree (the tenant directory, or an adopted workspace the worker owns)."""
    workspace = identity.workspace or ""
    tenant_id, uid = identity.tenant_id, identity.uid
    if in_tenant_area(root, workspace):
        tree = f"{os.path.normpath(root)}/{tenant_id}"
        migrate_tree(tree, tenant_id, uid, include_root=False)
        fd = _tenant_dir_fd(root, tenant_id)
        try:
            info = os.fstat(fd)
            if info.st_uid != tool_state.worker_uid():
                raise ToolIsolationError(f"the tenant directory of tenant {tenant_id} is not the worker's")
            # Last: only now does the tenant's UID get a path into the tree.
            os.fchmod(fd, tool_state.TENANT_DIR_MODE)
            posix_acl.set_acl(fd, posix_acl.ACCESS, posix_acl.tenant_access(uid))
            posix_acl.set_acl(fd, posix_acl.DEFAULT, posix_acl.tenant_default(uid))
            write_stamp(root, TENANTS, tenant_id, uid, os.fstat(fd))
        finally:
            os.close(fd)
        return
    migrate_tree(workspace, tenant_id, uid, include_root=True)
    fd = os.open(workspace, os.O_RDONLY | os.O_DIRECTORY | _FILE_FLAGS)
    try:
        write_stamp(root, ADOPTED, adopted_key(workspace), uid, os.fstat(fd))
    finally:
        os.close(fd)


def refused_tenant_dirs(root: str) -> list[str]:
    """Why tenant directories under *root* are refused (another owner, a symlink): logged at startup."""
    root_fd = tool_state.open_root(root)
    problems: list[str] = []
    try:
        for name in os.listdir(root_fd):
            if name.startswith("."):
                continue
            info = os.stat(name, dir_fd=root_fd, follow_symlinks=False)
            if stat.S_ISLNK(info.st_mode) or not stat.S_ISDIR(info.st_mode):
                problems.append(f"{root}/{name} is not a directory (a symlink?): its tenant's tool work is refused")
            elif info.st_uid != tool_state.worker_uid():
                problems.append(
                    f"{root}/{name} belongs to uid {info.st_uid}, not the worker: its tenant's tool work is refused "
                    f"(an operator fixes the directory's owner with a one-off root container: "
                    f"chown 10001:10010 {root}/{name})"
                )
    finally:
        os.close(root_fd)
    return problems
