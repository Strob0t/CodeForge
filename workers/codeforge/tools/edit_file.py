"""Built-in tool: edit a file by replacing an exact text match."""

from __future__ import annotations

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
from codeforge.tools._lint import post_write_check

if TYPE_CHECKING:
    from codeforge.workspace_fs import WorkspaceRoot

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
    when_to_use="Use to make targeted changes to existing files. Always read_file first to get the exact text to match.",
    output_format="Confirmation: 'replaced N line(s) with M line(s) in path'.",
    common_mistakes=[
        "old_text does not match exactly — copy text from read_file output, including whitespace and indentation",
        "old_text appears multiple times — include more surrounding context to make it unique",
        "Editing without reading the file first — always read_file before edit_file",
    ],
    examples=[
        ToolExample(
            description="Change a function return value",
            tool_call_json='{"file_path": "src/main.py", "old_text": "    return 0", "new_text": "    return 1"}',
            expected_result="replaced 1 line(s) with 1 line(s) in src/main.py",
        ),
        ToolExample(
            description="Add an import at the top of a file",
            tool_call_json='{"file_path": "src/main.py", "old_text": "import os", "new_text": "import os\\nimport sys"}',
            expected_result="replaced 1 line(s) with 2 line(s) in src/main.py",
        ),
    ],
)


def _normalized(text: str) -> str:
    """*text* with universal newlines and no trailing whitespace on any line."""
    return "\n".join(line.rstrip() for line in text.replace("\r\n", "\n").replace("\r", "\n").split("\n"))


def _normalized_span(content: str, old_text: str) -> tuple[int, int] | int:
    """Where *old_text* occurs in *content* once line endings and trailing whitespace are normalized.

    Returns the (start, end) offsets in *content*, or the number of matches
    when there is not exactly one. A match that ends at the end of a line
    takes that line's trailing whitespace along, never its line break.
    """
    norm_chars: list[str] = []
    starts: list[int] = []  # content offset of each normalized character
    ends: list[int] = []  # content offset just past it
    pos = 0
    segments = content.split("\n")
    for index, segment in enumerate(segments):
        has_break = index < len(segments) - 1
        body = segment[:-1] if has_break and segment.endswith("\r") else segment
        kept = body.rstrip()
        for k, char in enumerate(kept):
            norm_chars.append(char)
            starts.append(pos + k)
            ends.append(pos + k + 1)
        if kept:
            ends[-1] = pos + len(body)  # a match that ends here takes the trailing whitespace along
        if has_break:
            norm_chars.append("\n")
            starts.append(pos + len(body))
            ends.append(pos + len(segment) + 1)
        pos += len(segment) + 1
    norm = "".join(norm_chars)
    needle = _normalized(old_text)
    count = norm.count(needle) if needle else 0
    if count != 1:
        return count
    first = norm.index(needle)
    last = first + len(needle) - 1
    return starts[first], ends[last]


class EditFileTool(ToolExecutor):
    """Replace a unique text snippet in a file."""

    @catch_os_error
    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        rel = arguments.get("file_path", "")
        old_text = str(arguments.get("old_text", ""))
        new_text = str(arguments.get("new_text", ""))
        with open_tool_workspace(workspace_path) as root:
            return self._edit(root, tool_path(workspace_path, rel), rel, old_text, new_text)

    def _edit(self, root: WorkspaceRoot, path: str, rel: str, old_text: str, new_text: str) -> ToolResult:
        try:
            content = root.read_text(path, max_bytes=MAX_WORKSPACE_FILE_BYTES)
        except FileNotFoundError:
            return failed(f"file not found: {rel}")

        note = ""
        count = content.count(old_text) if old_text else 0
        if count > 1:
            return failed(f"old_text found {count} times (must be unique)")
        if count == 1:
            start = content.index(old_text)
            end = start + len(old_text)
        else:
            # Models often get line endings (a CRLF file read back as LF) and
            # trailing whitespace wrong: match with both normalized and write
            # the replacement with the file's line endings.
            span = _normalized_span(content, old_text)
            if isinstance(span, int):
                if span > 1:
                    return failed(f"old_text found {span} times (must be unique)")
                return failed(
                    "old_text not found in file. "
                    "Hint: Use read_file to copy the exact text including whitespace and indentation."
                )
            start, end = span
            newline = "\r\n" if "\r\n" in content else "\n"
            new_text = new_text.replace("\r\n", "\n").replace("\n", newline)
            note = " (line endings and trailing whitespace normalized)"

        old_actual = content[start:end]
        updated = content[:start] + new_text + content[end:]
        root.write_text(path, updated)

        old_lines = old_actual.count("\n") + 1
        new_lines = new_text.count("\n") + 1
        start_line = content[:start].count("\n") + 1

        diff_data = {
            "path": rel,
            "hunks": [
                {
                    "old_start": start_line,
                    "old_lines": old_lines,
                    "new_start": start_line,
                    "new_lines": new_lines,
                    "old_content": old_actual,
                    "new_content": new_text,
                }
            ],
        }

        output_msg = f"replaced {old_lines} line(s) with {new_lines} line(s) in {rel}{note}"
        lint_warning = post_write_check(rel, updated)
        if lint_warning:
            output_msg += f"\n\nSyntax warning: {lint_warning}\nPlease review and fix the syntax error."

        return ToolResult(
            output=output_msg,
            diff=diff_data,
        )
