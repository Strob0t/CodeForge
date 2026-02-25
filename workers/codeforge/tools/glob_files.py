"""Built-in tool: find files matching a glob pattern."""

from __future__ import annotations

import logging
from pathlib import Path
from typing import Any

from codeforge.tools._base import ToolDefinition, ToolExecutor, ToolResult

logger = logging.getLogger(__name__)

MAX_RESULTS = 500

DEFINITION = ToolDefinition(
    name="glob_files",
    description="Find files matching a glob pattern. Returns a sorted list of relative file paths.",
    parameters={
        "type": "object",
        "properties": {
            "pattern": {
                "type": "string",
                "description": "Glob pattern (e.g. '**/*.py', 'src/**/*.ts').",
            },
        },
        "required": ["pattern"],
    },
)


class GlobFilesTool(ToolExecutor):
    """Find files matching a glob pattern."""

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        pattern = arguments.get("pattern", "")
        workspace = Path(workspace_path).resolve()

        try:
            matches = sorted(workspace.glob(pattern))
        except ValueError as exc:
            return ToolResult(output="", error=str(exc), success=False)

        # Filter to only files and compute relative paths
        rel_paths = [str(m.relative_to(workspace)) for m in matches if m.is_file()]

        if not rel_paths:
            return ToolResult(output="no matches found")

        truncated = len(rel_paths) > MAX_RESULTS
        rel_paths = rel_paths[:MAX_RESULTS]

        output = "\n".join(rel_paths)
        if truncated:
            output += f"\n\n... truncated to {MAX_RESULTS} results"

        return ToolResult(output=output)
