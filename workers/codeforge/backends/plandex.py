"""Plandex backend executor — stub."""

from __future__ import annotations

import asyncio
import os
import shutil
from typing import Any

from codeforge.backends._base import BackendInfo, OutputCallback, TaskResult


class PlandexExecutor:
    """Stub executor for the Plandex backend."""

    def __init__(self, cli_path: str | None = None) -> None:
        self._cli_path = cli_path or os.environ.get("CODEFORGE_PLANDEX_PATH", "plandex")

    @property
    def info(self) -> BackendInfo:
        return BackendInfo(
            name="plandex",
            display_name="Plandex",
            cli_command=self._cli_path,
            capabilities=["code-edit", "planning", "multi-file"],
        )

    async def check_available(self) -> bool:
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
        return TaskResult(
            status="failed",
            error=(
                f"Backend '{self.info.display_name}' is not yet implemented in CodeForge. "
                "Plandex support is planned — see docs/features/04-agent-orchestration.md"
            ),
        )

    async def cancel(self, task_id: str) -> None:
        pass  # No-op for stub
