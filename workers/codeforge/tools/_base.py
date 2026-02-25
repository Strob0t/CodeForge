"""Base types for the tool framework."""

from __future__ import annotations

from dataclasses import dataclass, field
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
