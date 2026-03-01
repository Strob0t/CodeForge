"""Built-in tool: search file contents with regex."""

from __future__ import annotations

import asyncio
import logging
from typing import Any

from codeforge.constants import MAX_SEARCH_MATCHES
from codeforge.tools._base import ToolDefinition, ToolExample, ToolExecutor, ToolResult

logger = logging.getLogger(__name__)

MAX_MATCHES = MAX_SEARCH_MATCHES

DEFINITION = ToolDefinition(
    name="search_files",
    description="Search file contents using a regex pattern. Returns matching lines with file paths and line numbers.",
    parameters={
        "type": "object",
        "properties": {
            "pattern": {
                "type": "string",
                "description": "Regular expression pattern to search for.",
            },
            "path": {
                "type": "string",
                "description": "Subdirectory to search in (relative to workspace). Defaults to entire workspace.",
            },
            "include": {
                "type": "string",
                "description": "Glob pattern to filter files (e.g. '*.py').",
            },
        },
        "required": ["pattern"],
    },
    when_to_use="Use to find where a function, variable, string, or pattern is used across the codebase.",
    output_format="Lines formatted as 'filepath:line_number:matching_line'. Returns 'no matches found' if nothing matches.",
    common_mistakes=[
        "Using overly broad patterns that match too many files — use 'include' to filter by file type",
        "Not escaping regex special characters (dots, brackets, etc.)",
        "Searching entire workspace when you know the subdirectory — use 'path' to narrow scope",
    ],
    examples=[
        ToolExample(
            description="Find all usages of a function in Python files",
            tool_call_json='{"pattern": "def process_data", "include": "*.py"}',
            expected_result="src/utils.py:42:def process_data(items: list) -> dict:",
        ),
        ToolExample(
            description="Search for TODO comments in a specific directory",
            tool_call_json='{"pattern": "TODO|FIXME", "path": "src", "include": "*.py"}',
            expected_result="src/main.py:10:# TODO: add error handling",
        ),
    ],
)


class SearchFilesTool(ToolExecutor):
    """Search file contents with grep."""

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        pattern = arguments.get("pattern", "")
        sub_path = arguments.get("path", ".")
        include = arguments.get("include", "")

        cmd = ["grep", "-rn", "--color=never"]
        if include:
            cmd.extend([f"--include={include}"])
        cmd.extend(["-m", str(MAX_MATCHES), "--", pattern, sub_path])

        try:
            proc = await asyncio.create_subprocess_exec(
                *cmd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
                cwd=workspace_path,
            )
            stdout, stderr = await asyncio.wait_for(proc.communicate(), timeout=30)
        except TimeoutError:
            return ToolResult(output="", error="search timed out", success=False)
        except OSError as exc:
            return ToolResult(output="", error=str(exc), success=False)

        output = stdout.decode("utf-8", errors="replace").strip()

        # grep returns exit 1 when no matches (not an error)
        if proc.returncode == 1 and not output:
            return ToolResult(output="no matches found")

        if proc.returncode not in (0, 1):
            err = stderr.decode("utf-8", errors="replace").strip()
            return ToolResult(output="", error=err or f"grep exit code {proc.returncode}", success=False)

        # Limit output lines
        lines = output.splitlines()
        if len(lines) > MAX_MATCHES:
            lines = lines[:MAX_MATCHES]
            output = "\n".join(lines) + f"\n\n... truncated to {MAX_MATCHES} matches"
        else:
            output = "\n".join(lines)

        return ToolResult(output=output)
