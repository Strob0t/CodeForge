"""Backend router: dispatches task execution to the correct backend executor."""

from __future__ import annotations

import logging
from typing import Any

from codeforge.backends._base import BackendExecutor, ConfigField, OutputCallback, TaskResult

logger = logging.getLogger(__name__)


def validate_config(
    config: dict[str, Any],
    schema: tuple[ConfigField, ...],
    backend_name: str,
) -> list[str]:
    """Validate config dict against schema. Returns list of error strings.

    - Checks required fields are present
    - Checks types match (isinstance check on values)
    - Logs unknown keys at warning level
    - Returns list of validation errors (empty = valid)
    """
    if not schema:
        return []

    errors: list[str] = []
    known_keys = {f.key for f in schema}

    for key in config:
        if key not in known_keys:
            logger.warning("Backend '%s': unknown config key '%s'", backend_name, key)

    for field in schema:
        if field.key not in config:
            if field.required:
                errors.append(f"Required config key '{field.key}' is missing for backend '{backend_name}'")
            continue
        value = config[field.key]
        if not isinstance(value, field.type):
            errors.append(
                f"Config key '{field.key}' for backend '{backend_name}' "
                f"expected {field.type.__name__}, got {type(value).__name__}"
            )

    return errors


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
        required_capabilities: list[str] | None = None,
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

        # Capability enforcement (A4)
        if required_capabilities:
            provided = set(executor.info.capabilities)
            missing = [cap for cap in required_capabilities if cap not in provided]
            if missing:
                logger.warning(
                    "Backend '%s' missing capabilities: %s",
                    backend_name,
                    missing,
                )
                return TaskResult(
                    status="failed",
                    error=(f"Backend '{backend_name}' missing required capabilities: {', '.join(missing)}"),
                )

        # Config validation (A2) — advisory, logs warnings but does not block
        if config and executor.info.config_schema:
            warnings = validate_config(config, executor.info.config_schema, backend_name)
            for warning in warnings:
                logger.warning(warning)

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
