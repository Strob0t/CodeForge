"""Built-in tool: read file contents with optional offset and limit."""

from __future__ import annotations

import io
import logging
from typing import TYPE_CHECKING, Any

from codeforge.constants import MAX_WORKSPACE_FILE_BYTES
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
    from collections.abc import Iterable

logger = logging.getLogger(__name__)

# The most read_file returns at once; the file itself may be larger (offset/limit reach any line).
MAX_OUTPUT_BYTES = MAX_WORKSPACE_FILE_BYTES

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
        offset = max(arguments.get("offset", 1), 1)
        limit = arguments.get("limit")
        with open_tool_workspace(workspace_path) as root:
            try:
                raw = root.open_binary(tool_path(workspace_path, rel))
            except FileNotFoundError:
                return failed(f"file not found: {rel}")
        # The file is streamed: only the returned lines count against the cap,
        # so offset/limit reach any line of a large file.
        with io.TextIOWrapper(raw, encoding="utf-8", errors="replace", newline=None) as text:
            return ToolResult(output=_numbered_lines(text, offset, limit))


def _numbered_lines(text: Iterable[str], offset: int, limit: int | None) -> str:
    """Lines offset .. offset + limit - 1 (1-based) of *text*, numbered, at most MAX_OUTPUT_BYTES."""
    out: list[str] = []
    size = 0
    for number, line in enumerate(text, start=1):
        if number < offset:
            continue
        if limit is not None and number >= offset + limit:
            break
        entry = f"{number:>6}\t{line}"
        if not entry.endswith("\n"):
            entry += "\n"
        size += len(entry.encode())
        if size > MAX_OUTPUT_BYTES:
            out.append(f"... truncated at {MAX_OUTPUT_BYTES} bytes; read on with offset={number}\n")
            break
        out.append(entry)
    return "".join(out)
