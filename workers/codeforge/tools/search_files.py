"""Built-in tool: search file contents with regex."""

from __future__ import annotations

import asyncio
import logging
import re
from typing import Any

from codeforge.constants import MAX_SEARCH_MATCHES
from codeforge.tools._base import ToolDefinition, ToolExample, ToolExecutor, ToolResult, resolve_safe_path

logger = logging.getLogger(__name__)

MAX_MATCHES = MAX_SEARCH_MATCHES

DEFINITION = ToolDefinition(
    name="search_files",
    description="Search file contents using a pattern. Returns matching lines with file paths and line numbers.",
    parameters={
        "type": "object",
        "properties": {
            "pattern": {
                "type": "string",
                "description": "Pattern to search for (fixed string by default, regex if 'regex' is true).",
            },
            "path": {
                "type": "string",
                "description": "Subdirectory to search in (relative to workspace). Defaults to entire workspace.",
            },
            "include": {
                "type": "string",
                "description": "Glob pattern to filter files (e.g. '*.py').",
            },
            "regex": {
                "type": "boolean",
                "description": "If true, treat pattern as a regular expression. Defaults to false (fixed-string match).",
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
            tool_call_json='{"pattern": "TODO|FIXME", "path": "src", "include": "*.py", "regex": true}',
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
        use_regex = arguments.get("regex", False)

        # Validate path to prevent traversal outside workspace.
        safe_path, path_err = resolve_safe_path(
            workspace_path,
            sub_path,
            must_exist=True,
            must_be_dir=True,
        )
        if path_err is not None:
            return path_err

        # Validate regex pattern when regex mode is enabled.
        if use_regex:
            try:
                re.compile(pattern)
            except re.error as regex_exc:
                return ToolResult(output="", error=f"invalid regex pattern: {regex_exc}", success=False)

        cmd = ["grep", "-rn", "--color=never"]
        if use_regex:
            cmd.append("-E")
        else:
            cmd.append("-F")
        if include:
            cmd.append(f"--include={include}")
        cmd.extend(["-m", str(MAX_MATCHES), "--", pattern, str(safe_path)])

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
        except OSError as os_exc:
            return ToolResult(output="", error=str(os_exc), success=False)

        output = stdout.decode("utf-8", errors="replace").strip()

        # grep returns exit 1 when no matches (not an error)
        if proc.returncode == 1 and not output:
            return ToolResult(output="no matches found")

        if proc.returncode not in (0, 1):
            grep_err = stderr.decode("utf-8", errors="replace").strip()
            return ToolResult(output="", error=grep_err or f"grep exit code {proc.returncode}", success=False)

        # Limit output lines
        lines = output.splitlines()
        if len(lines) > MAX_MATCHES:
            lines = lines[:MAX_MATCHES]
            output = "\n".join(lines) + f"\n\n... truncated to {MAX_MATCHES} matches"
        else:
            output = "\n".join(lines)

        return ToolResult(output=output)
