"""Goose backend executor — stub."""

from __future__ import annotations

from codeforge.backends._base import BackendInfo, StubBackendExecutor
from codeforge.config import resolve_backend_path


class GooseExecutor(StubBackendExecutor):
    """Stub executor for the Goose backend."""

    def __init__(self, cli_path: str | None = None) -> None:
        self._cli_path = resolve_backend_path(cli_path, "CODEFORGE_GOOSE_PATH", "goose")

    @property
    def info(self) -> BackendInfo:
        return BackendInfo(
            name="goose",
            display_name="Goose",
            cli_command=self._cli_path,
            requires_docker=False,
            capabilities=["code-edit", "mcp-native"],
        )
