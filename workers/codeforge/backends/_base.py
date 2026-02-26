"""Base types for the backend executor framework."""

from __future__ import annotations

from collections.abc import Callable, Coroutine
from dataclasses import dataclass, field
from typing import Any, Protocol


@dataclass(frozen=True)
class BackendInfo:
    """Metadata about a backend executor."""

    name: str
    display_name: str
    cli_command: str
    requires_docker: bool = False
    capabilities: list[str] = field(default_factory=list)


@dataclass
class TaskResult:
    """Result returned by a backend execution."""

    status: str  # "completed" or "failed"
    output: str = ""
    error: str = ""


# Callback type for streaming output lines.
OutputCallback = Callable[[str], Coroutine[Any, Any, None]]


class BackendExecutor(Protocol):
    """Interface that all backend executors satisfy."""

    @property
    def info(self) -> BackendInfo: ...

    async def check_available(self) -> bool: ...

    async def execute(
        self,
        task_id: str,
        prompt: str,
        workspace_path: str,
        config: dict[str, Any] | None = None,
        on_output: OutputCallback | None = None,
    ) -> TaskResult: ...

    async def cancel(self, task_id: str) -> None: ...
