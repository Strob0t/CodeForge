"""Removing a deleted project's workspace as the tenant's tool UID (KI-96 D11).

Under the tenant directories' default ACLs a tool can create entries
(``mkdtemp``, ``mkdir -m 0700``, a stripped ACL) that neither the Go Core
nor the worker may remove: only the tool UID. With workspace.tool_acls:
required the Go Core deletes the project row, records the deletion and
publishes workspace.delete.request; the worker then

1. enters the tenant's tool identity (the accept checks; the tenant's
   shared lock, so its tree is migrated first if it still is from before
   the upgrade),
2. runs the full sharing pass and removes the workspace's contents as the
   tool UID, confined by Landlock to the workspace,
3. removes its own entries the tool UID cannot reach: the Go Core (the
   same UID as the worker) keeps some private, such as the patches of
   ``.git/codeforge/patches`` (0700, the patch 0600: under the default ACL
   the mask leaves the tool UID nothing),
4. removes the empty directory relative to the tenant directory's
   descriptor (never by a path a tool could change) and has the tenant's
   HOME cache removed at its idle (build caches can hold the project's
   data).

Delivered at least once: a workspace that is already gone counts as removed.
"""

from __future__ import annotations

import asyncio
import contextlib
import errno
import os
import stat
from dataclasses import dataclass

from codeforge import tool_state
from codeforge.tool_identity import ToolIsolationError, mark_cache_for_removal, tool_tenant
from codeforge.tool_process import remove_as_tool, share_tool_files, tool_isolation

_DIR_FLAGS = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC
# How many entries that could not be removed an error names.
_MAX_LEFT = 10


def _project_name(root: str, tenant_id: str, workspace: str) -> str:
    """The workspace's directory name in ``<root>/<tenant_id>``; anything else is refused."""
    if not os.path.isabs(workspace):
        raise ToolIsolationError(f"workspace {workspace!r} is not an absolute path")
    path = os.path.normpath(workspace)
    parent, name = os.path.split(path)
    if (
        parent != f"{os.path.normpath(root)}/{tenant_id}"
        or not tool_state.is_name(name)
        or not tool_state.is_name(tenant_id)
    ):
        raise ToolIsolationError(f"workspace {workspace} is not a project directory of tenant {tenant_id}: refused")
    return name


def remove_own_entries(parent_fd: int, name: str) -> list[str]:
    """Remove the worker's own entries below the directory *name* in *parent_fd*, and every directory
    that is empty then (not *name* itself); what is left, at most _MAX_LEFT entries.

    Runs after the tool UID's removal: what is left is out of the tool
    UID's reach. Every directory is opened relative to its parent's
    descriptor without following a symlink, on the same file system, and
    must be the listed one; every entry is removed by name in its directory,
    a symlink as itself. Entries of other users stay (their removal as the
    tool UID failed).
    """
    left: list[str] = []
    top = tool_state.open_dir_at(parent_fd, name)
    stack: list[_Dir] = []
    try:
        dev = os.fstat(top).st_dev
        stack.append(_Dir(top, name, os.listdir(top)))
        while stack:
            current = stack[-1]
            if current.names:
                child = _remove_next(current, dev, left)
                if child is not None:
                    stack.append(child)
                continue
            stack.pop()
            os.close(current.fd)
            if stack:
                _remove_emptied(stack[-1].fd, current.path, left)
    finally:
        for directory in stack:
            os.close(directory.fd)
    return left


@dataclass
class _Dir:
    """A directory of remove_own_entries' walk: its descriptor, its path for messages, what is still to do."""

    fd: int
    path: str
    names: list[str]


def _note(left: list[str], entry: str) -> None:
    if len(left) < _MAX_LEFT:
        left.append(entry)


def _remove_next(current: _Dir, dev: int, left: list[str]) -> _Dir | None:
    """Remove the next entry of *current* if it is the worker's; a directory is returned to walk into."""
    name = current.names.pop()
    path = f"{current.path}/{name}"
    try:
        info = os.stat(name, dir_fd=current.fd, follow_symlinks=False)
    except FileNotFoundError:
        return None
    if stat.S_ISDIR(info.st_mode):
        fd = _open_listed_dir(current.fd, name, info, dev)
        if fd is None:
            _note(left, f"{path} (replaced, or on another file system)")
            return None
        try:
            return _Dir(fd, path, os.listdir(fd))
        except OSError as exc:
            os.close(fd)
            _note(left, f"{path} ({exc.strerror})")
            return None
    if info.st_uid != tool_state.worker_uid():
        _note(left, f"{path} (uid {info.st_uid})")
        return None
    try:
        os.unlink(name, dir_fd=current.fd)
    except OSError as exc:
        if exc.errno != errno.ENOENT:
            _note(left, f"{path} ({exc.strerror})")
    return None


def _remove_emptied(parent_fd: int, path: str, left: list[str]) -> None:
    """Remove the walked directory *path* from its parent once it is empty (of any owner)."""
    try:
        os.rmdir(os.path.basename(path), dir_fd=parent_fd)
    except OSError as exc:
        if exc.errno != errno.ENOTEMPTY:  # what is inside was noted already
            _note(left, f"{path} ({exc.strerror})")


def _open_listed_dir(dir_fd: int, name: str, listed: os.stat_result, dev: int) -> int | None:
    """A descriptor of the directory *name* if it is still the listed one, on the file system *dev*."""
    if listed.st_dev != dev:
        return None
    try:
        fd = os.open(name, _DIR_FLAGS, dir_fd=dir_fd)
    except OSError:
        return None
    info = os.fstat(fd)
    if (info.st_dev, info.st_ino) != (listed.st_dev, listed.st_ino):
        os.close(fd)
        return None
    return fd


async def delete_workspace(tenant_id: str, tool_uid: int, workspace: str) -> None:
    """Remove the project workspace *workspace* of *tenant_id* as its tool UID *tool_uid*.

    Raises ToolIsolationError when the request is refused, OSError when the
    workspace could not be removed completely.
    """
    config = tool_isolation().config
    if not config.required:
        raise ToolIsolationError(
            "tool isolation is off: the Go Core removes workspaces itself (workspace.tool_acls=off)"
        )
    name = _project_name(config.workspace_root, tenant_id, workspace)
    root_fd = tool_state.open_root(config.workspace_root)
    try:
        try:
            tenant_fd = tool_state.open_dir_at(root_fd, tenant_id)
        except FileNotFoundError:
            return  # the tenant directory is gone, and the workspace with it
    finally:
        os.close(root_fd)
    try:
        try:
            info = os.stat(name, dir_fd=tenant_fd, follow_symlinks=False)
        except FileNotFoundError:
            return  # removed by an earlier delivery
        if not stat.S_ISDIR(info.st_mode):
            os.unlink(name, dir_fd=tenant_fd)
            return
        path = os.path.normpath(workspace)
        async with tool_tenant(tenant_id, tool_uid, path) as identity:
            await share_tool_files(path, identity)
            removed = await remove_as_tool([path], identity, confine=path, contents_only=True)  # type: ignore[arg-type]
            mark_cache_for_removal(tenant_id)
        left = await asyncio.to_thread(remove_own_entries, tenant_fd, name)
        try:
            os.rmdir(name, dir_fd=tenant_fd)
        except OSError as exc:
            if exc.errno != errno.ENOTEMPTY:
                raise
            failed = "" if removed else f"the removal as tool uid {tool_uid} failed; "
            raise OSError(
                errno.ENOTEMPTY,
                f"workspace {workspace} was not removed completely: {failed}left: {', '.join(left) or 'unknown'}",
            ) from exc
    finally:
        with contextlib.suppress(OSError):
            os.close(tenant_fd)
