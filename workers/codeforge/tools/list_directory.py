"""Built-in tool: list directory contents."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING, Any

from codeforge.constants import MAX_DIR_ENTRIES, MAX_LIST_DEPTH
from codeforge.tools._base import (
    ToolDefinition,
    ToolExample,
    ToolExecutor,
    ToolResult,
    failed,
    open_tool_workspace,
    tool_path,
)
from codeforge.tools._error_handler import catch_os_error

if TYPE_CHECKING:
    from codeforge.workspace_fs import WorkspaceRoot

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
    when_to_use="Use to explore directory structure. Start with the root to understand project layout, then drill into subdirectories.",
    output_format="Lines prefixed with [DIR] or [FILE] followed by relative path. Directories sorted first.",
    common_mistakes=[
        "Using recursive on large directories — start with non-recursive to get an overview first",
        "Passing a file path instead of a directory path",
    ],
    examples=[
        ToolExample(
            description="List project root",
            tool_call_json='{"path": "."}',
            expected_result="[DIR]  src\\n[DIR]  tests\\n[FILE] README.md\\n[FILE] pyproject.toml",
        ),
        ToolExample(
            description="Recursively list a small directory",
            tool_call_json='{"path": "src", "recursive": true}',
            expected_result="[DIR]  src/utils\\n[FILE] src/utils/helpers.py\\n[FILE] src/main.py",
        ),
    ],
)


def _list_entries(root: WorkspaceRoot, rel_dir: str, recursive: bool, depth: int = 0) -> list[str]:
    """Collect directory entries with prefix markers.

    A symlink is listed as [DIR] when it resolves to a directory inside the
    workspace, else as [FILE]; the listing never descends into a symlink.
    """
    entries: list[str] = []

    try:
        children = sorted(root.list_dir(rel_dir), key=lambda e: (not e.is_dir, e.name))
    except OSError:
        return entries

    for child in children:
        if len(entries) >= MAX_ENTRIES:
            break
        rel = child.name if rel_dir == "." else f"{rel_dir}/{child.name}"
        if child.is_dir:
            entries.append(f"[DIR]  {rel}")
            if recursive and not child.is_symlink and depth < MAX_DEPTH and len(entries) < MAX_ENTRIES:
                entries.extend(_list_entries(root, rel, recursive, depth + 1))
        else:
            entries.append(f"[FILE] {rel}")

    return entries


class ListDirectoryTool(ToolExecutor):
    """List directory contents."""

    @catch_os_error
    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        rel = arguments.get("path", ".")
        recursive = arguments.get("recursive", False)

        with open_tool_workspace(workspace_path) as root:
            try:
                start = root.resolve(tool_path(workspace_path, rel))
            except FileNotFoundError:
                return failed(f"not a directory: {rel}")
            if not root.is_dir(start):
                return failed(f"not a directory: {rel}")
            entries = _list_entries(root, start, recursive)

        if not entries:
            return ToolResult(output="(empty directory)")

        truncated = len(entries) > MAX_ENTRIES
        entries = entries[:MAX_ENTRIES]
        output = "\n".join(entries)
        if truncated:
            output += f"\n\n... truncated to {MAX_ENTRIES} entries"

        return ToolResult(output=output)
