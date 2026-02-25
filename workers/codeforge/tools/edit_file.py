"""Built-in tool: edit a file by replacing an exact text match."""

from __future__ import annotations

import logging
from pathlib import Path
from typing import Any

from codeforge.tools._base import ToolDefinition, ToolExecutor, ToolResult

logger = logging.getLogger(__name__)

DEFINITION = ToolDefinition(
    name="edit_file",
    description="Edit a file by replacing an exact occurrence of old_text with new_text. The old_text must appear exactly once in the file.",
    parameters={
        "type": "object",
        "properties": {
            "file_path": {
                "type": "string",
                "description": "Path to the file to edit (relative to workspace).",
            },
            "old_text": {
                "type": "string",
                "description": "Exact text to find and replace (must occur exactly once).",
            },
            "new_text": {
                "type": "string",
                "description": "Replacement text.",
            },
        },
        "required": ["file_path", "old_text", "new_text"],
    },
)


class EditFileTool(ToolExecutor):
    """Replace a unique text snippet in a file."""

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        workspace = Path(workspace_path).resolve()
        rel = arguments.get("file_path", "")
        target = (workspace / rel).resolve()

        if not str(target).startswith(str(workspace)):
            return ToolResult(output="", error="path traversal blocked", success=False)

        if not target.is_file():
            return ToolResult(output="", error=f"file not found: {rel}", success=False)

        old_text = arguments.get("old_text", "")
        new_text = arguments.get("new_text", "")

        try:
            content = target.read_text(encoding="utf-8")
        except OSError as exc:
            return ToolResult(output="", error=str(exc), success=False)

        count = content.count(old_text)
        if count == 0:
            return ToolResult(output="", error="old_text not found in file", success=False)
        if count > 1:
            return ToolResult(output="", error=f"old_text found {count} times (must be unique)", success=False)

        updated = content.replace(old_text, new_text, 1)

        try:
            target.write_text(updated, encoding="utf-8")
        except OSError as exc:
            return ToolResult(output="", error=str(exc), success=False)

        old_lines = old_text.count("\n") + 1
        new_lines = new_text.count("\n") + 1
        return ToolResult(output=f"replaced {old_lines} line(s) with {new_lines} line(s) in {rel}")
