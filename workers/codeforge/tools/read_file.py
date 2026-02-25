"""Built-in tool: read file contents with optional offset and limit."""

from __future__ import annotations

import logging
from pathlib import Path
from typing import Any

from codeforge.tools._base import ToolDefinition, ToolExecutor, ToolResult

logger = logging.getLogger(__name__)

DEFINITION = ToolDefinition(
    name="read_file",
    description="Read the contents of a file. Returns lines with line numbers.",
    parameters={
        "type": "object",
        "properties": {
            "file_path": {
                "type": "string",
                "description": "Path to the file to read (relative to workspace).",
            },
            "offset": {
                "type": "integer",
                "description": "Line number to start reading from (1-based). Defaults to 1.",
            },
            "limit": {
                "type": "integer",
                "description": "Maximum number of lines to return. Defaults to all.",
            },
        },
        "required": ["file_path"],
    },
)


class ReadFileTool(ToolExecutor):
    """Read a file's contents with optional line offset and limit."""

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        workspace = Path(workspace_path).resolve()
        rel = arguments.get("file_path", "")
        target = (workspace / rel).resolve()

        if not str(target).startswith(str(workspace)):
            return ToolResult(output="", error="path traversal blocked", success=False)

        if not target.is_file():
            return ToolResult(output="", error=f"file not found: {rel}", success=False)

        try:
            text = target.read_text(encoding="utf-8", errors="replace")
        except OSError as exc:
            return ToolResult(output="", error=str(exc), success=False)

        lines = text.splitlines(keepends=True)
        offset = max(arguments.get("offset", 1), 1)
        limit = arguments.get("limit")

        start = offset - 1
        end = start + limit if limit is not None else len(lines)
        selected = lines[start:end]

        numbered = "".join(f"{start + i + 1:>6}\t{line}" for i, line in enumerate(selected))
        if numbered and not numbered.endswith("\n"):
            numbered += "\n"

        return ToolResult(output=numbered)
