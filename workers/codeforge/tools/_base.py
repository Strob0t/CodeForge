"""Base types for the tool framework."""

from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Protocol


@dataclass(frozen=True)
class ToolDefinition:
    """Declarative description of a tool (name, description, JSON Schema parameters)."""

    name: str
    description: str
    parameters: dict[str, Any] = field(default_factory=dict)


@dataclass
class ToolResult:
    """Result returned by a tool execution."""

    output: str
    error: str = ""
    success: bool = True


class ToolExecutor(Protocol):
    """Interface that all tool implementations satisfy."""

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult: ...


def resolve_safe_path(
    workspace_path: str,
    relative_path: str,
    *,
    must_exist: bool = False,
    must_be_file: bool = False,
    must_be_dir: bool = False,
) -> tuple[Path, ToolResult | None]:
    """Resolve *relative_path* under *workspace_path* and validate constraints.

    Returns ``(resolved_path, None)`` on success or ``(Path(), error_result)``
    when a constraint is violated.
    """
    workspace = Path(workspace_path).resolve()
    target = (workspace / relative_path).resolve()

    if not str(target).startswith(str(workspace)):
        return Path(), ToolResult(output="", error="path traversal blocked", success=False)

    if must_exist and not target.exists():
        return Path(), ToolResult(output="", error=f"not found: {relative_path}", success=False)

    if must_be_file and not target.is_file():
        return Path(), ToolResult(output="", error=f"file not found: {relative_path}", success=False)

    if must_be_dir and not target.is_dir():
        return Path(), ToolResult(output="", error=f"not a directory: {relative_path}", success=False)

    return target, None
