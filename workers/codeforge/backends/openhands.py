"""OpenHands backend executor — stub."""

from __future__ import annotations

import os
from typing import Any

from codeforge.backends._base import BackendInfo, OutputCallback, TaskResult


class OpenHandsExecutor:
    """Stub executor for the OpenHands backend."""

    def __init__(self, url: str | None = None) -> None:
        self._url = url or os.environ.get("CODEFORGE_OPENHANDS_URL", "http://localhost:3000")

    @property
    def info(self) -> BackendInfo:
        return BackendInfo(
            name="openhands",
            display_name="OpenHands",
            cli_command=self._url,
            requires_docker=True,
            capabilities=["code-edit", "browser", "sandbox"],
        )

    async def check_available(self) -> bool:
        """Check if the OpenHands HTTP API is reachable."""
        try:
            import httpx

            async with httpx.AsyncClient(timeout=5.0) as client:
                resp = await client.get(f"{self._url}/api/health")
                return resp.status_code == 200
        except Exception:
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
                "OpenHands support is planned — see docs/features/04-agent-orchestration.md"
            ),
        )

    async def cancel(self, task_id: str) -> None:
        pass  # No-op for stub
