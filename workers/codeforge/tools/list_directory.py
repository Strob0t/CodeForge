"""Built-in tool: list directory contents."""

from __future__ import annotations

import logging
from pathlib import Path
from typing import Any

from codeforge.constants import MAX_DIR_ENTRIES, MAX_LIST_DEPTH
from codeforge.tools._base import ToolDefinition, ToolExecutor, ToolResult, resolve_safe_path

logger = logging.getLogger(__name__)

MAX_ENTRIES = MAX_DIR_ENTRIES
MAX_DEPTH = MAX_LIST_DEPTH

DEFINITION = ToolDefinition(
    name="list_directory",
    description="List contents of a directory with [DIR] and [FILE] prefixes.",
    parameters={
        "type": "object",
        "properties": {
            "path": {
                "type": "string",
                "description": "Directory path relative to workspace (defaults to '.').",
            },
            "recursive": {
                "type": "boolean",
                "description": "List recursively up to depth 3 (default false).",
            },
        },
    },
)


def _list_entries(base: Path, workspace: Path, recursive: bool, depth: int = 0) -> list[str]:
    """Collect directory entries with prefix markers."""
    entries: list[str] = []

    try:
        children = sorted(base.iterdir(), key=lambda p: (not p.is_dir(), p.name))
    except OSError:
        return entries

    for child in children:
        if len(entries) >= MAX_ENTRIES:
            break
        rel = str(child.relative_to(workspace))
        if child.is_dir():
            entries.append(f"[DIR]  {rel}")
            if recursive and depth < MAX_DEPTH and len(entries) < MAX_ENTRIES:
                entries.extend(_list_entries(child, workspace, recursive, depth + 1))
        else:
            entries.append(f"[FILE] {rel}")

    return entries


class ListDirectoryTool(ToolExecutor):
    """List directory contents."""

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        rel = arguments.get("path", ".")
        recursive = arguments.get("recursive", False)

        target, err = resolve_safe_path(workspace_path, rel, must_be_dir=True)
        if err is not None:
            return err
        workspace = Path(workspace_path).resolve()

        entries = _list_entries(target, workspace, recursive)

        if not entries:
            return ToolResult(output="(empty directory)")

        truncated = len(entries) > MAX_ENTRIES
        entries = entries[:MAX_ENTRIES]
        output = "\n".join(entries)
        if truncated:
            output += f"\n\n... truncated to {MAX_ENTRIES} entries"

        return ToolResult(output=output)
