"""Base types for the backend executor framework."""

from __future__ import annotations

from collections.abc import Callable, Coroutine
from dataclasses import dataclass, field
from typing import Any, Protocol

from codeforge.subprocess_utils import check_cli_available


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


class StubBackendExecutor:
    """Base class for backend executors that are not yet implemented.

    Provides default implementations for ``check_available()`` (CLI probe),
    ``execute()`` (returns "not yet implemented"), and ``cancel()`` (no-op).
    Subclasses only need to define ``info`` and ``__init__``.
    """

    @property
    def info(self) -> BackendInfo:
        raise NotImplementedError

    async def check_available(self) -> bool:
        return await check_cli_available(self.info.cli_command)

    async def execute(
        self,
        task_id: str,
        prompt: str,
        workspace_path: str,
        config: dict[str, Any] | None = None,
        on_output: OutputCallback | None = None,
    ) -> TaskResult:
        return TaskResult(
            status="failed",
            error=(
                f"Backend '{self.info.display_name}' is not yet implemented in CodeForge. "
                f"See docs/features/04-agent-orchestration.md"
            ),
        )

    async def cancel(self, task_id: str) -> None:
        pass
