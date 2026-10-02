"""The sharing pass: a tenant's tool files stay reachable for the workspace group (KI-71 review, KI-96 D8).

The Go Core and the worker (group 10010) read, checkpoint, deliver and delete
what a tenant's tools create. Under the tenant directories' default ACLs a
new entry gets ``g:10010``, but a tool can still lock them out: an
owner-only mode (``mkdtemp``, ``mkdir -m 0700``, ``chmod 600``) masks the
entry, ``setfacl`` strips it, a directory loses its default ACL.

The worker starts this walker as the entries' owner, the tenant's tool UID,
through the launcher (``<python> -I -S tool_walk.py share <root>``). For
every regular file and directory of its own below *root* it makes sure that

- the access ACL has ``g:10010`` with ``rw`` (``rwx`` for directories and
  for files the owner may execute) and the mask lets that through;
- a directory has the tenant's default ACL
  (``u::rwx,u:T:rwx,g::rwx,g:10010:rwx,m::rwx,o::---``) and its owner can
  search it (so the walk reaches what is inside);

and writes an ACL only where it differs. It never follows a symlink and never
leaves the file system: every entry is listed relative to its directory's
descriptor, opened with ``O_PATH | O_NOFOLLOW`` and changed only when the
descriptor is the listed inode (``/proc/self/fd/<n>`` names exactly that
inode, so an entry of mode 0000 is reached too). Entries of other owners are
counted and left alone.

With ``--since <epoch>`` (after one tool call) only entries changed since
then, minus one second, are checked; creating, ``chmod``, ``chgrp`` and
``setfacl`` all set the change time.

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


@dataclass
class Report:
    checked: int = 0
    changed: int = 0
    foreign: int = 0
    skipped: int = 0
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
    user_key = (posix_acl.USER_OBJ, posix_acl.UNDEFINED_ID)
    if directory:
        perms[user_key] |= 5  # the owner (and so the walk) can list and enter it
        need = 7
    else:
        need = 6 | (perms[user_key] & 1)
    group_key = (posix_acl.GROUP, posix_acl.WORKSPACE_GID)
    perms[group_key] = perms.get(group_key, 0) | need
    mask_key = (posix_acl.MASK, posix_acl.UNDEFINED_ID)
    # A minimal ACL has no mask: the group class so far was the owning group alone.
    mask = perms.get(mask_key, perms[(posix_acl.GROUP_OBJ, posix_acl.UNDEFINED_ID)])
    perms[mask_key] = mask | need
    return [posix_acl.Entry(tag, perm, ident) for (tag, ident), perm in perms.items()]


def share_entry(fd: int, info: os.stat_result, uid: int) -> bool:
    """Give the workspace group access to the entry behind the ``O_PATH`` descriptor *fd*; True if changed."""
    target = f"/proc/self/fd/{fd}"
    directory = stat.S_ISDIR(info.st_mode)
    current = posix_acl.get_acl(target, posix_acl.ACCESS)
    wanted = wanted_access(current or _minimal(stat.S_IMODE(info.st_mode)), directory=directory)
    changed = False
    if not posix_acl.equal(current, wanted):
        posix_acl.set_acl(target, posix_acl.ACCESS, wanted)
        changed = True
    if directory:
        default = posix_acl.tenant_default(uid)
        if not posix_acl.equal(posix_acl.get_acl(target, posix_acl.DEFAULT), default):
            posix_acl.set_acl(target, posix_acl.DEFAULT, default)
            changed = True
    return changed


def _check(dir_fd: int, name: str, info: os.stat_result, uid: int, since: float | None, report: Report) -> None:
    if info.st_uid != uid:
        report.foreign += 1
        return
    if since is not None and info.st_ctime < since - _SINCE_SLACK_SECONDS:
        return
    fd = open_checked(dir_fd, name, info)
    if fd is None:
        report.skipped += 1
        return
    try:
        report.checked += 1
        if share_entry(fd, info, uid):
            report.changed += 1
    except OSError as exc:
        report.error(f"{name}: {exc.strerror or exc}")
    finally:
        os.close(fd)


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


def share(root: str, *, since: float | None = None, uid: int | None = None) -> Report:
    """Share the entries of *uid* (default: this process's UID) below and including *root*."""
    uid = os.getuid() if uid is None else uid
    report = Report()
    root_fd = open_root(root)
    root_info = os.fstat(root_fd)
    parent_fd = os.open("..", _PATH_FLAGS, dir_fd=root_fd)
    try:
        _check(parent_fd, os.path.basename(os.path.normpath(root)) or ".", root_info, uid, since, report)
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
            _check(dir_fd, name, info, uid, since, report)
            if stat.S_ISDIR(info.st_mode):
                sub = _open_subdir(dir_fd, name, info)
                if sub is None:
                    report.error(f"{path}/{name}: cannot be entered")
                else:
                    stack.append((sub, f"{path}/{name}"))
        os.close(dir_fd)
    return report


def main(argv: list[str]) -> int:
    if len(argv) not in (2, 4) or argv[0] != "share" or (len(argv) == 4 and argv[2] != "--since"):
        sys.stderr.write("usage: tool_walk.py share <root> [--since <epoch seconds>]\n")
        return 2
    since = float(argv[3]) if len(argv) == 4 else None
    try:
        report = share(argv[1], since=since)
    except OSError as exc:
        sys.stderr.write(f"tool_walk: {exc}\n")
        return 2
    sys.stdout.write(json.dumps(asdict(report)) + "\n")
    return 1 if report.errors else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
