"""Built-in tool: find files matching a glob pattern."""

from __future__ import annotations

import logging
from pathlib import Path
from typing import Any

from codeforge.constants import MAX_TOOL_RESULTS
from codeforge.tools._base import ToolDefinition, ToolExample, ToolExecutor, ToolResult

logger = logging.getLogger(__name__)

MAX_RESULTS = MAX_TOOL_RESULTS

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
    when_to_use="Use to discover files by name or extension. Helpful for finding project structure or locating specific file types.",
    output_format="Newline-separated list of relative file paths. Returns 'no matches found' if empty.",
    common_mistakes=[
        "Forgetting '**/' prefix for recursive search — '*.py' only matches root, '**/*.py' matches all directories",
        "Using regex syntax instead of glob syntax — use * and ** not .* or .+",
    ],
    examples=[
        ToolExample(
            description="Find all Python files in the project",
            tool_call_json='{"pattern": "**/*.py"}',
            expected_result="src/main.py\\nsrc/utils.py\\ntests/test_main.py",
        ),
        ToolExample(
            description="Find configuration files",
            tool_call_json='{"pattern": "**/*.{yaml,yml,toml}"}',
            expected_result="config.yaml\\npyproject.toml",
        ),
    ],
)


class GlobFilesTool(ToolExecutor):
    """Find files matching a glob pattern."""

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        pattern = arguments.get("pattern", "")
        workspace = Path(workspace_path).resolve()

        # Block patterns with '..' components to prevent path traversal.
        if ".." in pattern.split("/"):
            return ToolResult(output="", error="path traversal blocked: '..' not allowed in glob pattern", success=False)

        try:
            matches = sorted(workspace.glob(pattern))
        except ValueError as exc:
            return ToolResult(output="", error=str(exc), success=False)

        # Filter to only files within the workspace and compute relative paths.
        rel_paths: list[str] = []
        for m in matches:
            resolved = m.resolve()
            if not resolved.is_file():
                continue
            if not str(resolved).startswith(str(workspace)):
                continue
            rel_paths.append(str(resolved.relative_to(workspace)))

        if not rel_paths:
            return ToolResult(output="no matches found")

        truncated = len(rel_paths) > MAX_RESULTS
        rel_paths = rel_paths[:MAX_RESULTS]

        output = "\n".join(rel_paths)
        if truncated:
            output += f"\n\n... truncated to {MAX_RESULTS} results"

        return ToolResult(output=output)
