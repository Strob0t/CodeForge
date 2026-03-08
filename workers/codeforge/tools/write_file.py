"""Built-in tool: write content to a file."""

from __future__ import annotations

import logging
from typing import Any

from codeforge.tools._base import ToolDefinition, ToolExample, ToolExecutor, ToolResult, resolve_safe_path
from codeforge.tools._error_handler import catch_os_error

logger = logging.getLogger(__name__)

DEFINITION = ToolDefinition(
    name="write_file",
    description="Write content to a file. Creates parent directories if needed. Overwrites existing content entirely.",
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
    when_to_use="Use to create new files or completely replace file content. For partial changes, use edit_file instead.",
    output_format="Confirmation message: 'wrote N bytes to path'.",
    common_mistakes=[
        "Using write_file to make small changes — use edit_file for partial modifications",
        "Forgetting that write_file overwrites the entire file",
        "Not including the full desired content (write_file replaces everything)",
    ],
    examples=[
        ToolExample(
            description="Create a new Python module",
            tool_call_json='{"file_path": "src/utils.py", "content": "def add(a: int, b: int) -> int:\\n    return a + b\\n"}',
            expected_result="wrote 42 bytes to src/utils.py",
        ),
    ],
)


class WriteFileTool(ToolExecutor):
    """Write content to a file, creating parent directories as needed."""

    @catch_os_error
    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        rel = arguments.get("file_path", "")
        target, err = resolve_safe_path(workspace_path, rel)
        if err is not None:
            return err

        content = arguments.get("content", "")

        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content, encoding="utf-8")

        return ToolResult(output=f"wrote {len(content)} bytes to {rel}")
