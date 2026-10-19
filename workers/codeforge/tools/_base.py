"""Base types for the tool framework."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Protocol

from codeforge.workspace_fs import WorkspaceRoot, workspace_relative


@dataclass(frozen=True)
class ToolExample:
    """Example invocation of a tool for weaker model guidance."""

    description: str
    tool_call_json: str
    expected_result: str


@dataclass(frozen=True)
class ToolDefinition:
    """Declarative description of a tool (name, description, JSON Schema parameters).

    Extended metadata fields (when_to_use, output_format, common_mistakes,
    examples) are used by the adaptive tool guide to help weaker models.
    """

    name: str
    description: str
    parameters: dict[str, Any] = field(default_factory=dict)
    when_to_use: str = ""
    output_format: str = ""
    common_mistakes: list[str] = field(default_factory=list)
    examples: list[ToolExample] = field(default_factory=list)


@dataclass
class ToolResult:
    """Result returned by a tool execution."""

    output: str
    error: str = ""
    success: bool = True
    diff: dict[str, Any] | None = None


class ToolExecutor(Protocol):
    """Interface that all tool implementations satisfy."""

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult: ...


# Git metadata is off limits for file tools (KI-77): the Go Core runs git in
# the workspace, and .git/config, hooks and info/attributes can make git run
# programs. The workspace helper refuses a .git component wherever it comes
# from (the path or a symlink target), in any case.
_BLOCKED_NAMES = frozenset({".git"})


def open_tool_workspace(workspace_path: str) -> WorkspaceRoot:
    """The workspace of a file tool: every path resolves inside it (KI-95) and .git is blocked.

    The tools read and write workspace files only through it
    (codeforge.workspace_fs): symlinks are followed only while they stay
    inside the workspace, and only regular files are read or written.
    """
    return WorkspaceRoot(workspace_path, blocked_names=_BLOCKED_NAMES)


def tool_path(workspace_path: str, path: str) -> str:
    """The workspace-relative form of a path a model passed; absolute paths into the workspace are accepted."""
    return workspace_relative(workspace_path, path)


def failed(error: str) -> ToolResult:
    """A failed tool result with *error*."""
    return ToolResult(output="", error=error, success=False)
