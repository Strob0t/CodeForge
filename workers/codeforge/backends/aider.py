"""Aider backend executor — real subprocess wrapper."""

from __future__ import annotations

import asyncio
import logging
import os
import shutil
from typing import Any

from codeforge.backends._base import BackendInfo, OutputCallback, TaskResult

logger = logging.getLogger(__name__)

_DEFAULT_TIMEOUT = 600  # 10 minutes


class AiderExecutor:
    """Execute tasks using the Aider CLI."""

    def __init__(self, cli_path: str | None = None) -> None:
        self._cli_path = cli_path or os.environ.get("CODEFORGE_AIDER_PATH", "aider")
        self._processes: dict[str, asyncio.subprocess.Process] = {}

    @property
    def info(self) -> BackendInfo:
        return BackendInfo(
            name="aider",
            display_name="Aider",
            cli_command=self._cli_path,
            capabilities=["code-edit", "git-commit", "multi-file"],
        )

    async def check_available(self) -> bool:
        """Check if aider CLI is installed and reachable."""
        path = shutil.which(self._cli_path)
        if path is not None:
            return True
        try:
            proc = await asyncio.create_subprocess_exec(
                self._cli_path,
                "--version",
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
            )
            await asyncio.wait_for(proc.communicate(), timeout=10)
            return proc.returncode == 0
        except (OSError, TimeoutError):
            return False

    async def execute(
        self,
        task_id: str,
        prompt: str,
        workspace_path: str,
        config: dict[str, Any] | None = None,
        on_output: OutputCallback | None = None,
    ) -> TaskResult:
        """Run aider with the given prompt in the workspace directory."""
        config = config or {}
        timeout = config.get("timeout", _DEFAULT_TIMEOUT)

        cmd = [self._cli_path, "--yes-always", "--no-auto-commits", "--message", prompt]

        model = config.get("model")
        if model:
            cmd.extend(["--model", model])

        api_base = config.get("openai_api_base")
        if api_base:
            cmd.extend(["--openai-api-base", api_base])

        logger.info("aider exec task=%s cmd=%s cwd=%s", task_id, cmd[:4], workspace_path)

        try:
            proc = await asyncio.create_subprocess_exec(
                *cmd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.STDOUT,
                cwd=workspace_path or None,
            )
        except OSError as exc:
            return TaskResult(status="failed", error=f"Failed to start aider: {exc}")

        self._processes[task_id] = proc
        output_lines: list[str] = []

        try:
            stdout = proc.stdout
            if stdout is None:
                return TaskResult(status="failed", error="Failed to capture aider stdout")
            while True:
                try:
                    line_bytes = await asyncio.wait_for(stdout.readline(), timeout=timeout)
                except TimeoutError:
                    proc.terminate()
                    await proc.wait()
                    return TaskResult(
                        status="failed",
                        output="\n".join(output_lines),
                        error=f"Aider timed out after {timeout}s",
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
                error=f"Aider exited with code {proc.returncode}",
            )
        finally:
            self._processes.pop(task_id, None)

    async def cancel(self, task_id: str) -> None:
        """Terminate the running aider process."""
        proc = self._processes.get(task_id)
        if proc is not None and proc.returncode is None:
            proc.terminate()
            logger.info("aider process terminated for task %s", task_id)
