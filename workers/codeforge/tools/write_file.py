"""Built-in tool: write content to a file."""

from __future__ import annotations

import logging
from typing import Any

from codeforge.tools._base import ToolDefinition, ToolExecutor, ToolResult, resolve_safe_path

logger = logging.getLogger(__name__)

DEFINITION = ToolDefinition(
    name="write_file",
    description="Write content to a file. Creates parent directories if needed.",
    parameters={
        "type": "object",
        "properties": {
            "file_path": {
                "type": "string",
                "description": "Path to the file to write (relative to workspace).",
            },
            "content": {
                "type": "string",
                "description": "Content to write to the file.",
            },
        },
        "required": ["file_path", "content"],
    },
)


class WriteFileTool(ToolExecutor):
    """Write content to a file, creating parent directories as needed."""

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        rel = arguments.get("file_path", "")
        target, err = resolve_safe_path(workspace_path, rel)
        if err is not None:
            return err

        content = arguments.get("content", "")

        try:
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(content, encoding="utf-8")
        except OSError as exc:
            return ToolResult(output="", error=str(exc), success=False)

        return ToolResult(output=f"wrote {len(content)} bytes to {rel}")
