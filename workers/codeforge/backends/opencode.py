"""OpenCode backend executor — subprocess wrapper."""

from __future__ import annotations

import asyncio
import logging
import os
from typing import Any

from codeforge.backends._base import BackendInfo, ConfigField, OutputCallback, TaskResult, parse_extra_args
from codeforge.config import resolve_backend_path
from codeforge.constants import DEFAULT_BACKEND_TIMEOUT_SECONDS
from codeforge.subprocess_utils import check_cli_available, graceful_terminate

logger = logging.getLogger(__name__)

_DEFAULT_TIMEOUT = DEFAULT_BACKEND_TIMEOUT_SECONDS


class OpenCodeExecutor:
    """Execute tasks using the OpenCode CLI."""

    def __init__(self, cli_path: str | None = None) -> None:
        self._cli_path = resolve_backend_path(cli_path, "CODEFORGE_OPENCODE_PATH", "opencode")
        self._processes: dict[str, asyncio.subprocess.Process] = {}

    @property
    def info(self) -> BackendInfo:
        return BackendInfo(
            name="opencode",
            display_name="OpenCode",
            cli_command=self._cli_path,
            capabilities=["code-edit", "lsp"],
            config_schema=(
                ConfigField(key="model", type=str, description="LLM model name"),
                ConfigField(key="timeout", type=int, default=_DEFAULT_TIMEOUT, description="Timeout in seconds"),
                ConfigField(key="extra_args", type=list, description="Extra CLI arguments"),
                ConfigField(key="extra_env", type=dict, description="Extra environment variables for subprocess"),
                ConfigField(key="working_dir_override", type=str, description="Override workspace path"),
            ),
        )

    async def check_available(self) -> bool:
        return await check_cli_available(self._cli_path)

    async def execute(
        self,
        task_id: str,
        prompt: str,
        workspace_path: str,
        config: dict[str, Any] | None = None,
        on_output: OutputCallback | None = None,
    ) -> TaskResult:
        """Run opencode with the given prompt in the workspace directory."""
        config = config or {}
        timeout = config.get("timeout", _DEFAULT_TIMEOUT)
        cwd = config.get("working_dir_override") or workspace_path
        extra_env: dict[str, str] = config.get("extra_env") or {}

        # opencode run --prompt "<prompt>" runs a single-shot task.
        cmd = [self._cli_path, "run", "--prompt", prompt]

        model = config.get("model")
        if model:
            cmd.extend(["--model", model])

        cmd.extend(parse_extra_args(config))

        logger.info("opencode exec task=%s cmd=%s cwd=%s", task_id, cmd[:4], cwd)

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
            return TaskResult(status="failed", error=f"Failed to start opencode: {exc}")

        self._processes[task_id] = proc
        output_lines: list[str] = []

        try:
            stdout = proc.stdout
            if stdout is None:
                return TaskResult(status="failed", error="Failed to capture opencode stdout")
            while True:
                try:
                    line_bytes = await asyncio.wait_for(stdout.readline(), timeout=timeout)
                except TimeoutError:
                    await graceful_terminate(proc)
                    return TaskResult(
                        status="failed",
                        output="\n".join(output_lines),
                        error=f"OpenCode timed out after {timeout}s",
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
                error=f"OpenCode exited with code {proc.returncode}",
            )
        finally:
            self._processes.pop(task_id, None)

    async def cancel(self, task_id: str) -> None:
        proc = self._processes.get(task_id)
        if proc is not None and proc.returncode is None:
            await graceful_terminate(proc)
            logger.info("opencode process terminated for task %s", task_id)
