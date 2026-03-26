"""Template Method base class for CLI-based backend executors.

All five CLI backends (Aider, Goose, OpenCode, Plandex, SWE-agent) share
identical subprocess management: start process, stream stdout line-by-line,
handle timeout/cancel, collect output, return TaskResult. The only
differences are (a) the BackendInfo metadata and (b) how the command list
is built from the prompt and config.

Subclasses implement ``info`` and ``_build_command`` (~30 lines each).
Everything else lives here.
"""

from __future__ import annotations

import asyncio
import logging
import os
from abc import ABC, abstractmethod
from typing import TypedDict

from codeforge.backends._base import BackendInfo, OutputCallback, TaskResult
from codeforge.config import resolve_backend_path
from codeforge.constants import DEFAULT_BACKEND_TIMEOUT_SECONDS
from codeforge.subprocess_utils import check_cli_available, graceful_terminate

logger = logging.getLogger(__name__)

_DEFAULT_TIMEOUT = DEFAULT_BACKEND_TIMEOUT_SECONDS


class ExecutorConfig(TypedDict, total=False):
    """Typed configuration dict accepted by all CLI backend executors."""

    timeout: int
    model: str
    extra_args: list[str]
    extra_env: dict[str, str]
    working_dir_override: str


class CLIBackendExecutor(ABC):
    """Template Method base for CLI backends.

    Subclasses must implement:
    - ``info`` property returning ``BackendInfo``
    - ``_build_command(prompt, config)`` returning the CLI argument list
    """

    def __init__(self, cli_path: str | None, env_var: str, default_cmd: str) -> None:
        self._cli_path = resolve_backend_path(cli_path, env_var, default_cmd)
        self._processes: dict[str, asyncio.subprocess.Process] = {}

    @property
    @abstractmethod
    def info(self) -> BackendInfo: ...

    @abstractmethod
    def _build_command(self, prompt: str, config: ExecutorConfig) -> list[str]:
        """Build the full CLI argument list for execution.

        Implementations should use ``parse_extra_args(config)`` to append
        user-supplied extra arguments.
        """
        ...

    async def check_available(self) -> bool:
        """Check if the CLI tool is installed and reachable."""
        return await check_cli_available(self._cli_path)

    async def execute(
        self,
        task_id: str,
        prompt: str,
        workspace_path: str,
        config: ExecutorConfig | None = None,
        on_output: OutputCallback | None = None,
    ) -> TaskResult:
        """Run the CLI tool with the given prompt in the workspace directory."""
        effective_config: ExecutorConfig = config or {}  # type: ignore[assignment]
        timeout = effective_config.get("timeout", _DEFAULT_TIMEOUT)
        cwd = effective_config.get("working_dir_override") or workspace_path
        extra_env: dict[str, str] = effective_config.get("extra_env") or {}

        cmd = self._build_command(prompt, effective_config)
        name = self.info.name

        logger.info("%s exec task=%s cmd=%s cwd=%s", name, task_id, cmd[:4], cwd)

        merged_env = {**os.environ, **extra_env} if extra_env else None

        try:
            proc = await asyncio.create_subprocess_exec(
                *cmd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.STDOUT,
                cwd=cwd or None,
                env=merged_env,
            )
        except OSError as exc:
            return TaskResult(status="failed", error=f"Failed to start {name}: {exc}")

        self._processes[task_id] = proc
        output_lines: list[str] = []

        try:
            stdout = proc.stdout
            if stdout is None:
                return TaskResult(status="failed", error=f"Failed to capture {name} stdout")
            while True:
                try:
                    line_bytes = await asyncio.wait_for(stdout.readline(), timeout=timeout)
                except TimeoutError:
                    await graceful_terminate(proc)
                    return TaskResult(
                        status="failed",
                        output="\n".join(output_lines),
                        error=f"{self.info.display_name} timed out after {timeout}s",
                    )

                if not line_bytes:
                    break

                line = line_bytes.decode("utf-8", errors="replace").rstrip("\n")
                output_lines.append(line)

                if on_output is not None:
                    await on_output(line)

            await proc.wait()

            output = "\n".join(output_lines)
            if proc.returncode == 0:
                return TaskResult(status="completed", output=output)
            return TaskResult(
                status="failed",
                output=output,
                error=f"{self.info.display_name} exited with code {proc.returncode}",
            )
        finally:
            self._processes.pop(task_id, None)

    async def cancel(self, task_id: str) -> None:
        """Terminate the running CLI process."""
        proc = self._processes.get(task_id)
        if proc is not None and proc.returncode is None:
            await graceful_terminate(proc)
            logger.info("%s process terminated for task %s", self.info.name, task_id)
