"""Built-in tool: find files matching a glob pattern."""

from __future__ import annotations

import fnmatch
import itertools
import logging
import os
import stat
from typing import Any

from codeforge.constants import MAX_TOOL_RESULTS
from codeforge.tools._base import ToolDefinition, ToolExample, ToolExecutor, ToolResult, failed, tool_path
from codeforge.tools._error_handler import catch_os_error
from codeforge.workspace_fs import WorkspaceRoot

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


def _has_magic(part: str) -> bool:
    return any(char in part for char in "*?[")


def _matches(pattern: list[str], parts: list[str]) -> bool:
    """Whether the path components match the pattern components (pathlib semantics).

    "**" stands for any number of directories (never the file itself); other
    components match one name with fnmatch rules, case-sensitive, dot files
    included.
    """
    if not pattern:
        return not parts
    head = pattern[0]
    if head == "**":
        return any(_matches(pattern[1:], parts[skip:]) for skip in range(len(parts)))
    return bool(parts) and fnmatch.fnmatchcase(parts[0], head) and _matches(pattern[1:], parts[1:])


def _below(base: str, dirpath: str) -> list[str]:
    """The components of *dirpath* below *base* (both workspace-relative)."""
    if dirpath == base:
        return []
    return (dirpath if base == "." else dirpath[len(base) + 1 :]).split("/")


def _glob(root: WorkspaceRoot, pattern: list[str]) -> list[str]:
    """Workspace-relative paths of the regular files that match *pattern*.

    The walk starts at the pattern's literal directory prefix and follows
    relative directory symlinks inside the workspace (reported under the
    symlink's path; never back into a directory on the walk's path, at most
    MAX_SYMLINKS per path); a symlinked file matches when it resolves to a
    regular file inside the workspace.
    """
    literal = list(itertools.takewhile(lambda part: not _has_magic(part), pattern[:-1]))
    rest = pattern[len(literal) :]
    base = "/".join(literal) or "."
    if not root.is_dir(base):
        return []
    # Without "**" a match is at most len(rest) - 1 directories below the base.
    max_depth = None if "**" in rest else len(rest) - 1
    found: list[str] = []
    for dirpath, dirnames, filenames, dir_fd in root.walk(base, follow_dir_symlinks=True):
        sub = _below(base, dirpath)
        if max_depth is not None and len(sub) >= max_depth:
            dirnames[:] = []
        for name in filenames:
            if not _matches(rest, [*sub, name]):
                continue
            rel = name if dirpath == "." else f"{dirpath}/{name}"
            try:
                mode = os.stat(name, dir_fd=dir_fd, follow_symlinks=False).st_mode
            except OSError:
                continue
            if stat.S_ISREG(mode) or (stat.S_ISLNK(mode) and root.is_file(rel)):
                found.append(rel)
    return sorted(found, key=lambda rel: rel.split("/"))


class GlobFilesTool(ToolExecutor):
    """Find files matching a glob pattern."""

    @catch_os_error
    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        pattern = tool_path(workspace_path, arguments.get("pattern", ""))

        # Block patterns with '..' components to prevent path traversal.
        if ".." in pattern.split("/"):
            return failed("path leaves the workspace: '..' is not allowed in a glob pattern")
        if pattern.startswith("/"):
            return failed(f"path leaves the workspace: {pattern}")
        parts = [part for part in pattern.split("/") if part not in ("", ".")]
        if not parts:
            return failed(f"Unacceptable pattern: {pattern!r}")

        with WorkspaceRoot(workspace_path) as root:
            rel_paths = _glob(root, parts)

        if not rel_paths:
            return ToolResult(output="no matches found")

        truncated = len(rel_paths) > MAX_RESULTS
        rel_paths = rel_paths[:MAX_RESULTS]

        output = "\n".join(rel_paths)
        if truncated:
            output += f"\n\n... truncated to {MAX_RESULTS} results"

        return ToolResult(output=output)
