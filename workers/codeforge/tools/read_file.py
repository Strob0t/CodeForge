"""Built-in tool: read file contents with optional offset and limit."""

from __future__ import annotations

import logging
from typing import Any

from codeforge.tools._base import ToolDefinition, ToolExample, ToolExecutor, ToolResult, resolve_safe_path
from codeforge.tools._error_handler import catch_os_error

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
    when_to_use="Use to inspect file contents before editing, understand code structure, or verify changes.",
    output_format="Numbered lines: '   1\\tline content'. Use offset/limit for large files.",
    common_mistakes=[
        "Using absolute paths instead of workspace-relative paths",
        "Reading entire large files when only a section is needed — use offset and limit",
    ],
    examples=[
        ToolExample(
            description="Read the first 20 lines of a Python file",
            tool_call_json='{"file_path": "src/main.py", "limit": 20}',
            expected_result="     1\\timport os\\n     2\\timport sys\\n...",
        ),
        ToolExample(
            description="Read lines 50-70 of a file",
            tool_call_json='{"file_path": "src/main.py", "offset": 50, "limit": 20}',
            expected_result="    50\\tdef process():\\n    51\\t    ...",
        ),
    ],
)


class ReadFileTool(ToolExecutor):
    """Read a file's contents with optional line offset and limit."""

    @catch_os_error
    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        rel = arguments.get("file_path", "")
        target, err = resolve_safe_path(workspace_path, rel, must_be_file=True)
        if err is not None:
            return err

        text = target.read_text(encoding="utf-8", errors="replace")

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
