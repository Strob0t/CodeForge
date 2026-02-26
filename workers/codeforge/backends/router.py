"""Backend router: dispatches task execution to the correct backend executor."""

from __future__ import annotations

import logging
from typing import Any

from codeforge.backends._base import BackendExecutor, OutputCallback, TaskResult

logger = logging.getLogger(__name__)


class BackendRouter:
    """Routes task execution to registered backend executors."""

    def __init__(self) -> None:
        self._executors: dict[str, BackendExecutor] = {}
        self._active: dict[str, str] = {}  # task_id -> backend_name

    def register(self, executor: BackendExecutor) -> None:
        """Register a backend executor."""
        self._executors[executor.info.name] = executor

    def get(self, name: str) -> BackendExecutor | None:
        """Get executor by backend name."""
        return self._executors.get(name)

    def available_backends(self) -> list[str]:
        """Return list of registered backend names."""
        return list(self._executors.keys())

    async def execute(
        self,
        backend_name: str,
        task_id: str,
        prompt: str,
        workspace_path: str,
        config: dict[str, Any] | None = None,
        on_output: OutputCallback | None = None,
    ) -> TaskResult:
        """Route execution to the named backend."""
        executor = self._executors.get(backend_name)
        if executor is None:
            available = ", ".join(sorted(self._executors.keys())) or "none"
            return TaskResult(
                status="failed",
                error=f"Unknown backend '{backend_name}'. Available: {available}",
            )

        if not await executor.check_available():
            info = executor.info
            return TaskResult(
                status="failed",
                error=(f"Backend '{info.display_name}' is not available. Install it with: {info.cli_command} --help"),
            )

        self._active[task_id] = backend_name
        try:
            return await executor.execute(
                task_id=task_id,
                prompt=prompt,
                workspace_path=workspace_path,
                config=config,
                on_output=on_output,
            )
        finally:
            self._active.pop(task_id, None)

    async def cancel(self, task_id: str) -> None:
        """Cancel a running task by routing to the correct backend."""
        backend_name = self._active.get(task_id)
        if backend_name is None:
            logger.warning("cancel requested for unknown task %s", task_id)
            return
        executor = self._executors.get(backend_name)
        if executor is not None:
            await executor.cancel(task_id)
