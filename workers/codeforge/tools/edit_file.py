"""Built-in tool: edit a file by replacing an exact text match."""

from __future__ import annotations

import logging
from typing import Any

from codeforge.tools._base import ToolDefinition, ToolExample, ToolExecutor, ToolResult, resolve_safe_path
from codeforge.tools._error_handler import catch_os_error
from codeforge.tools._lint import post_write_check

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


class EditFileTool(ToolExecutor):
    """Replace a unique text snippet in a file."""

    @catch_os_error
    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        rel = arguments.get("file_path", "")
        target, err = resolve_safe_path(workspace_path, rel, must_be_file=True)
        if err is not None:
            return err

        old_text = arguments.get("old_text", "")
        new_text = arguments.get("new_text", "")

        content = target.read_text(encoding="utf-8")

        count = content.count(old_text)
        if count == 0:
            # Fallback: try matching after normalizing line endings and
            # trailing whitespace per line, which LLMs often get wrong.
            old_norm = "\n".join(line.rstrip() for line in old_text.splitlines())
            content_norm = "\n".join(line.rstrip() for line in content.splitlines())
            count = content_norm.count(old_norm)
            if count == 1:
                # Rebuild old_text from the actual content to preserve
                # original whitespace in the replacement.
                start_idx = content_norm.index(old_norm)
                actual_old = content[start_idx : start_idx + len(old_text)]
                # Verify lengths align (they may differ if trailing ws was stripped).
                # Use line-by-line reconstruction for safety.
                orig_lines = content.splitlines(keepends=True)
                norm_pos = 0
                actual_start = None
                actual_end = None
                for i, line in enumerate(orig_lines):
                    line_norm = line.rstrip() + "\n" if line.endswith("\n") else line.rstrip()
                    if norm_pos <= start_idx < norm_pos + len(line_norm) and actual_start is None:
                        actual_start = sum(len(l) for l in orig_lines[:i]) + (start_idx - norm_pos)
                    norm_pos += len(line_norm)
                    if norm_pos >= start_idx + len(old_norm) and actual_end is None:
                        actual_end = sum(len(l) for l in orig_lines[: i + 1])
                if actual_start is not None and actual_end is not None:
                    actual_old = content[actual_start:actual_end]
                    updated = content[:actual_start] + new_text + content[actual_end:]
                    target.write_text(updated, encoding="utf-8")
                    old_lines = actual_old.count("\n") + 1
                    new_lines = new_text.count("\n") + 1
                    start_line = content[:actual_start].count("\n") + 1
                    return ToolResult(
                        output=f"edited {rel} (fuzzy match: trailing whitespace normalized)",
                        error=None,
                        success=True,
                    )
            return ToolResult(
                output="",
                error=(
                    "old_text not found in file. "
                    "Hint: Use read_file to copy the exact text including whitespace and indentation."
                ),
                success=False,
            )
        if count > 1:
            return ToolResult(output="", error=f"old_text found {count} times (must be unique)", success=False)

        updated = content.replace(old_text, new_text, 1)
        target.write_text(updated, encoding="utf-8")

        old_lines = old_text.count("\n") + 1
        new_lines = new_text.count("\n") + 1
        start_line = content[: content.index(old_text)].count("\n") + 1

        diff_data = {
            "path": rel,
            "hunks": [
                {
                    "old_start": start_line,
                    "old_lines": old_lines,
                    "new_start": start_line,
                    "new_lines": new_lines,
                    "old_content": old_text,
                    "new_content": new_text,
                }
            ],
        }

        output_msg = f"replaced {old_lines} line(s) with {new_lines} line(s) in {rel}"
        lint_warning = post_write_check(rel, updated)
        if lint_warning:
            output_msg += f"\n\nSyntax warning: {lint_warning}\nPlease review and fix the syntax error."

        return ToolResult(
            output=output_msg,
            diff=diff_data,
        )
