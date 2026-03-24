"""Base types for the backend executor framework."""

from __future__ import annotations

import json
from abc import ABC, abstractmethod
from collections.abc import Callable, Coroutine
from dataclasses import dataclass, field
from typing import Any, Protocol

from codeforge.subprocess_utils import check_cli_available


def parse_extra_args(config: dict[str, Any] | None) -> list[str]:
    """Extract extra CLI arguments from config.

    Supports:
    - config["extra_args"] as list[str]: used directly
    - config["extra_args"] as str: parsed as JSON list
    - Missing or None: returns empty list
    """
    if config is None:
        return []
    raw = config.get("extra_args")
    if raw is None:
        return []
    if isinstance(raw, list):
        return raw
    if isinstance(raw, str):
        return json.loads(raw)
    return []


@dataclass(frozen=True)
class ConfigField:
    """Describes a single configuration key for a backend."""

    key: str
    type: type
    default: object = None
    description: str = ""
    required: bool = False


@dataclass(frozen=True)
class BackendInfo:
    """Metadata about a backend executor."""

    name: str
    display_name: str
    cli_command: str
    requires_docker: bool = False
    capabilities: list[str] = field(default_factory=list)
    config_schema: tuple[ConfigField, ...] = ()


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


class StubBackendExecutor(ABC):
    """Template base class for backends not yet implemented in CodeForge.

    Not currently subclassed by any production backend. Provides sensible
    defaults for ``execute()`` (error message) and ``cancel()`` (no-op)
    so new backend stubs can be added with minimal boilerplate.
    """

    @property
    @abstractmethod
    def info(self) -> BackendInfo: ...

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

    async def cancel(self, task_id: str) -> None:  # noqa: B027
        pass
