"""Built-in tool: write content to a file."""

from __future__ import annotations

import contextlib
import logging
from typing import Any

from codeforge.constants import MAX_WORKSPACE_FILE_BYTES
from codeforge.tools._base import ToolDefinition, ToolExample, ToolExecutor, ToolResult, open_tool_workspace, tool_path
from codeforge.tools._error_handler import catch_os_error
from codeforge.tools._lint import post_write_check
from codeforge.workspace_fs import FileTooLargeError, NotRegularFileError

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
        content = arguments.get("content", "")

        with open_tool_workspace(workspace_path) as root:
            path = tool_path(workspace_path, rel)
            # Snapshot before write for diff (none for a new, special or very large file;
            # the write reports a special file).
            old_content = ""
            with contextlib.suppress(FileNotFoundError, NotRegularFileError, FileTooLargeError):
                old_content = root.read_text(path, max_bytes=MAX_WORKSPACE_FILE_BYTES, errors="replace")
            root.write_text(path, content, make_parents=True)

        diff_data = {
            "path": rel,
            "hunks": [
                {
                    "old_start": 1,
                    "old_lines": old_content.count("\n") + 1 if old_content else 0,
                    "new_start": 1,
                    "new_lines": content.count("\n") + 1 if content else 0,
                    "old_content": old_content,
                    "new_content": content,
                }
            ],
        }

        output_msg = f"wrote {len(content)} bytes to {rel}"
        lint_warning = post_write_check(rel, content)
        if lint_warning:
            output_msg += f"\n\nSyntax warning: {lint_warning}\nPlease review and fix the syntax error."

        return ToolResult(
            output=output_msg,
            diff=diff_data,
        )
