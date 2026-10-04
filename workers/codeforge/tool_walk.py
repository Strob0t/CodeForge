"""Owner-run ACL walks over workspace trees (KI-71 review, KI-96 D8, D9).

The Go Core and the worker (group 10010) read, checkpoint, deliver and delete
what a tenant's tools create. Under the tenant directories' default ACLs a
new entry gets ``g:10010``, but a tool can still lock them out: an
owner-only mode (``mkdtemp``, ``mkdir -m 0700``, ``chmod 600``) masks the
entry, ``setfacl`` strips it, a directory loses its default ACL.

Every walk changes only entries of the UID that runs it (the owner may set
ACLs on its own entries, nobody else may), so the worker starts it as that
owner through the launcher (``<python> -I -S tool_walk.py <mode> ...``):

- ``share <root> [--since <epoch>]``, as the tenant's tool UID after its tool
  calls and work items: its files and directories get ``g:10010`` with
  ``rw`` (``rwx`` for directories and for files the owner may execute) in
  the access ACL and the mask, directories the tenant's default ACL
  (``u::rwx,u:T:rwx,g::rwx,g:10010:rwx,m::rwx,o::---``) and their owner
  search access (so the walk reaches what is inside). With ``--since`` only
  entries changed since then, minus one second, are checked; creating,
  ``chmod``, ``chgrp`` and ``setfacl`` all set the change time.
- ``legacy-open <root>``, as the retired shared tool user 10002 in the
  migration of a tree from before the upgrade (D9): the same access grant
  for the workspace group, no default ACL, so the worker can walk the tree.
- ``legacy-exact <root> <tool uid>``, as 10002 later in that migration: its
  entries get exactly ``u::<owner>,u:T:rwX,g::rwX,g:10010:rwX,m::rwX,o::---``
  (planted entries go) and directories the tenant's default ACL. A regular
  file with links outside the tree is skipped: its inode also lives in
  another tree. The worker runs the same walk in-process on its own entries.

A walk writes an ACL only where it differs. It never follows a symlink and
never leaves the file system: every entry is listed relative to its
directory's descriptor, opened with ``O_PATH | O_NOFOLLOW`` and changed only
when the descriptor is the listed inode (``/proc/self/fd/<n>`` names exactly
that inode, so an entry of mode 0000 is reached too). Entries of other
owners are counted and left alone.

Standard library only (plus the sibling posix_acl.py, loaded by path): the
launcher runs it with ``-I -S``. Prints a JSON report; exits 1 when an entry
could not be checked, 2 when *root* cannot be walked.
"""

from __future__ import annotations

import json
import os
import stat
import sys
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Callable
    from types import ModuleType


def _sibling(name: str) -> ModuleType:
    """A module next to this file, loaded by path (the launcher runs the walker without package paths)."""
    import importlib.util

    spec = importlib.util.spec_from_file_location(f"cf_{name}", Path(__file__).with_name(f"{name}.py"))
    if spec is None or spec.loader is None:
        raise ImportError(name)
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


if __package__:
    from codeforge import posix_acl
else:  # pragma: no cover - exercised through the launcher and test_the_walker_runs_as_a_script
    posix_acl = _sibling("posix_acl")

_MAX_ERRORS = 20
_DIR_FLAGS = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC
_PATH_FLAGS = os.O_PATH | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC
# Entries changed this long before --since are still checked (time stamps of
# file systems and clocks are not exact).
_SINCE_SLACK_SECONDS = 1.0
_USER_OBJ = (posix_acl.USER_OBJ, posix_acl.UNDEFINED_ID)
_GROUP_OBJ = (posix_acl.GROUP_OBJ, posix_acl.UNDEFINED_ID)
_MASK = (posix_acl.MASK, posix_acl.UNDEFINED_ID)
_WORKSPACE_GROUP = (posix_acl.GROUP, posix_acl.WORKSPACE_GID)

Inode = tuple[int, int]


@dataclass
class Report:
    checked: int = 0
    changed: int = 0
    foreign: int = 0
    skipped: int = 0
    linked_outside: int = 0
    errors: list[str] = field(default_factory=list)

    def error(self, message: str) -> None:
        if len(self.errors) < _MAX_ERRORS:
            self.errors.append(message)


def open_root(path: str) -> int:
    """A descriptor of the directory *path*, opened one component at a time without following a symlink.

    The levels above it are opened with ``O_PATH``: the tool UID may only
    search its tenant directory, not list it.
    """
    if not os.path.isabs(path):
        raise OSError(f"{path!r} is not an absolute path")
    names = [part for part in path.split("/") if part]
    fd = os.open("/", _DIR_FLAGS if not names else _PATH_FLAGS)
    try:
        for index, name in enumerate(names):
            if name == "..":
                raise OSError(f"{path!r} contains '..'")
            flags = _DIR_FLAGS if index == len(names) - 1 else _PATH_FLAGS
            try:
                next_fd = os.open(name, flags, dir_fd=fd)
            except OSError as exc:
                if stat.S_ISLNK(os.stat(name, dir_fd=fd, follow_symlinks=False).st_mode):
                    raise OSError(f"{path}: {name} is a symlink: refused") from exc
                raise
            os.close(fd)
            fd = next_fd
    except BaseException:
        os.close(fd)
        raise
    return fd


def open_checked(dir_fd: int, name: str, listed: os.stat_result) -> int | None:
    """An ``O_PATH`` descriptor of *name* if it is still the listed inode (type, owner); else None."""
    try:
        fd = os.open(name, os.O_PATH | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=dir_fd)
    except OSError:
        return None
    info = os.fstat(fd)
    if (
        (info.st_dev, info.st_ino) != (listed.st_dev, listed.st_ino)
        or stat.S_IFMT(info.st_mode) != stat.S_IFMT(listed.st_mode)
        or info.st_uid != listed.st_uid
    ):
        os.close(fd)
        return None
    return fd


def _open_subdir(dir_fd: int, name: str, listed: os.stat_result) -> int | None:
    try:
        fd = os.open(name, _DIR_FLAGS, dir_fd=dir_fd)
    except OSError:
        return None
    info = os.fstat(fd)
    if (info.st_dev, info.st_ino) != (listed.st_dev, listed.st_ino):
        os.close(fd)
        return None
    return fd


def walk(
    root: str,
    visit: Callable[[int, str, os.stat_result, Report], None],
    report: Report,
    *,
    include_root: bool = True,
) -> Report:
    """Call *visit* for *root* (unless excluded) and every directory and regular file below it on its file system.

    A directory is visited before it is entered (the visit may give its
    owner search access). Symlinks, special files and other file systems
    are counted as skipped.
    """
    root_fd = open_root(root)
    root_info = os.fstat(root_fd)
    if include_root:
        parent_fd = os.open("..", _PATH_FLAGS, dir_fd=root_fd)
        try:
            visit(parent_fd, os.path.basename(os.path.normpath(root)) or ".", root_info, report)
        finally:
            os.close(parent_fd)
    stack: list[tuple[int, str]] = [(root_fd, root)]
    while stack:
        dir_fd, path = stack.pop()
        try:
            names = os.listdir(dir_fd)
        except OSError as exc:
            report.error(f"{path}: {exc.strerror}")
            names = []
        for name in names:
            try:
                info = os.stat(name, dir_fd=dir_fd, follow_symlinks=False)
            except OSError:
                continue  # removed meanwhile
            if info.st_dev != root_info.st_dev or not (stat.S_ISDIR(info.st_mode) or stat.S_ISREG(info.st_mode)):
                report.skipped += 1
                continue
            visit(dir_fd, name, info, report)
            if stat.S_ISDIR(info.st_mode):
                sub = _open_subdir(dir_fd, name, info)
                if sub is None:
                    report.error(f"{path}/{name}: cannot be entered")
                else:
                    stack.append((sub, f"{path}/{name}"))
        os.close(dir_fd)
    return report


def _minimal(mode: int) -> list[posix_acl.Entry]:
    """The ACL the mode bits stand for when an entry has none."""
    return [
        posix_acl.Entry(posix_acl.USER_OBJ, mode >> 6 & 7),
        posix_acl.Entry(posix_acl.GROUP_OBJ, mode >> 3 & 7),
        posix_acl.Entry(posix_acl.OTHER, mode & 7),
    ]


def wanted_access(current: list[posix_acl.Entry], *, directory: bool) -> list[posix_acl.Entry]:
    """*current* with ``g:10010`` (and the mask) granting what the workspace group needs."""
    perms = {(e.tag, e.id): e.perm for e in current}
    if directory:
        perms[_USER_OBJ] |= 5  # the owner (and so the walk) can list and enter it
        need = 7
    else:
        need = 6 | (perms[_USER_OBJ] & 1)
    perms[_WORKSPACE_GROUP] = perms.get(_WORKSPACE_GROUP, 0) | need
    # A minimal ACL has no mask: the group class so far was the owning group alone.
    perms[_MASK] = perms.get(_MASK, perms[_GROUP_OBJ]) | need
    return [posix_acl.Entry(tag, perm, ident) for (tag, ident), perm in perms.items()]


def exact_access(mode: int, tool_uid: int, *, directory: bool) -> list[posix_acl.Entry]:
    """The exact access ACL of an entry in a tenant's tree (``X``: directories and files their owner may execute)."""
    rw_x = 7 if directory or mode & 0o100 else 6  # the owner's own execute bit: planted entries do not count
    owner = mode >> 6 & 7 | (5 if directory else 0)
    return [
        posix_acl.Entry(posix_acl.USER_OBJ, owner),
        posix_acl.Entry(posix_acl.USER, rw_x, tool_uid),
        posix_acl.Entry(posix_acl.GROUP_OBJ, rw_x),
        posix_acl.Entry(posix_acl.GROUP, rw_x, posix_acl.WORKSPACE_GID),
        posix_acl.Entry(posix_acl.MASK, rw_x),
        posix_acl.Entry(posix_acl.OTHER, 0),
    ]


def _set_if_different(target: str, name: str, wanted: list[posix_acl.Entry]) -> bool:
    if posix_acl.equal(posix_acl.get_acl(target, name), wanted):
        return False
    posix_acl.set_acl(target, name, wanted)
    return True


def _owned(
    dir_fd: int,
    name: str,
    info: os.stat_result,
    uid: int,
    report: Report,
    change: Callable[[str, os.stat_result], bool],
) -> None:
    """Run *change* on ``/proc/self/fd/<n>`` of *name* when it is *uid*'s and still the listed inode."""
    if info.st_uid != uid:
        report.foreign += 1
        return
    fd = open_checked(dir_fd, name, info)
    if fd is None:
        report.skipped += 1
        return
    try:
        report.checked += 1
        if change(f"/proc/self/fd/{fd}", info):
            report.changed += 1
    except OSError as exc:
        report.error(f"{name}: {exc.strerror or exc}")
    finally:
        os.close(fd)


def share(root: str, *, since: float | None = None, uid: int | None = None) -> Report:
    """Share the entries of *uid* (default: this process's UID) below and including *root*."""
    owner = os.getuid() if uid is None else uid

    def change(target: str, info: os.stat_result) -> bool:
        directory = stat.S_ISDIR(info.st_mode)
        current = posix_acl.get_acl(target, posix_acl.ACCESS)
        changed = _set_if_different(
            target, posix_acl.ACCESS, wanted_access(current or _minimal(info.st_mode), directory=directory)
        )
        if directory:
            changed |= _set_if_different(target, posix_acl.DEFAULT, posix_acl.tenant_default(owner))
        return changed

    def visit(dir_fd: int, name: str, info: os.stat_result, report: Report) -> None:
        if since is not None and info.st_uid == owner and info.st_ctime < since - _SINCE_SLACK_SECONDS:
            return
        _owned(dir_fd, name, info, owner, report, change)

    return walk(root, visit, Report())


def legacy_open(root: str, *, uid: int | None = None) -> Report:
    """Give the workspace group access to every entry of *uid* below *root*; no default ACL (D9 step 1)."""
    owner = os.getuid() if uid is None else uid

    def change(target: str, info: os.stat_result) -> bool:
        current = posix_acl.get_acl(target, posix_acl.ACCESS)
        wanted = wanted_access(current or _minimal(info.st_mode), directory=stat.S_ISDIR(info.st_mode))
        return _set_if_different(target, posix_acl.ACCESS, wanted)

    return walk(root, lambda d, n, i, r: _owned(d, n, i, owner, r, change), Report())


def census(root: str) -> dict[Inode, int]:
    """How many names inside *root* each regular file with more than one link has."""
    counts: dict[Inode, int] = {}

    def visit(_dir_fd: int, _name: str, info: os.stat_result, _report: Report) -> None:
        if stat.S_ISREG(info.st_mode) and info.st_nlink > 1:
            key = (info.st_dev, info.st_ino)
            counts[key] = counts.get(key, 0) + 1

    walk(root, visit, Report())
    return counts


def exact(root: str, tool_uid: int, *, uid: int | None = None, include_root: bool = True) -> Report:
    """Exact ACLs for tool UID *tool_uid* on every entry of *uid* below *root* (D9 steps 4 and 5).

    A regular file with links outside the tree keeps its ACL: rewriting it
    would change the inode another tree shares (it is counted). Without
    *include_root* the top directory is left to the caller (a tenant
    directory gets the tenant's ACLs, not a project's).
    """
    owner = os.getuid() if uid is None else uid
    inside = census(root)

    def change(target: str, info: os.stat_result) -> bool:
        directory = stat.S_ISDIR(info.st_mode)
        changed = _set_if_different(
            target, posix_acl.ACCESS, exact_access(stat.S_IMODE(info.st_mode), tool_uid, directory=directory)
        )
        if directory:
            changed |= _set_if_different(target, posix_acl.DEFAULT, posix_acl.tenant_default(tool_uid))
        return changed

    def visit(dir_fd: int, name: str, info: os.stat_result, report: Report) -> None:
        if stat.S_ISREG(info.st_mode) and info.st_nlink > inside.get((info.st_dev, info.st_ino), 1):
            if info.st_uid == owner:
                report.linked_outside += 1
            return
        _owned(dir_fd, name, info, owner, report, change)

    return walk(root, visit, Report(), include_root=include_root)


_USAGE = (
    "usage: tool_walk.py share <root> [--since <epoch seconds>]\n"
    "       tool_walk.py legacy-open <root>\n"
    "       tool_walk.py legacy-exact <root> <tool uid>\n"
)


def _run(argv: list[str]) -> Report | None:
    match argv:
        case ["share", root]:
            return share(root)
        case ["share", root, "--since", since]:
            return share(root, since=float(since))
        case ["legacy-open", root]:
            return legacy_open(root)
        case ["legacy-exact", root, tool_uid] if tool_uid.isdigit():
            return exact(root, int(tool_uid))
        case _:
            return None


def main(argv: list[str]) -> int:
    try:
        report = _run(argv)
    except (OSError, ValueError) as exc:
        sys.stderr.write(f"tool_walk: {exc}\n")
        return 2
    if report is None:
        sys.stderr.write(_USAGE)
        return 2
    sys.stdout.write(json.dumps(asdict(report)) + "\n")
    return 1 if report.errors else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
