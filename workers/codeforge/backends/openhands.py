"""OpenHands backend executor — stub."""

from __future__ import annotations

from codeforge.backends._base import BackendInfo, StubBackendExecutor
from codeforge.config import resolve_backend_path


class OpenHandsExecutor(StubBackendExecutor):
    """Stub executor for the OpenHands backend."""

    def __init__(self, url: str | None = None) -> None:
        self._url = resolve_backend_path(url, "CODEFORGE_OPENHANDS_URL", "http://localhost:3000")

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
