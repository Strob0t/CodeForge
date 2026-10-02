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
3. removes the empty directory relative to the tenant directory's
   descriptor (never by a path a tool could change) and has the tenant's
   HOME cache removed at its idle (build caches can hold the project's
   data).

Delivered at least once: a workspace that is already gone counts as removed.
"""

from __future__ import annotations

import contextlib
import os
import stat

from codeforge import tool_state
from codeforge.tool_identity import ToolIsolationError, mark_cache_for_removal, tool_tenant
from codeforge.tool_process import remove_as_tool, share_tool_files, tool_isolation


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
            await remove_as_tool([path], identity, confine=path, contents_only=True)  # type: ignore[arg-type]
            mark_cache_for_removal(tenant_id)
        # Only the worker's own empty directory is left (ENOTEMPTY if the tool could not remove everything).
        os.rmdir(name, dir_fd=tenant_fd)
    finally:
        with contextlib.suppress(OSError):
            os.close(tenant_fd)
