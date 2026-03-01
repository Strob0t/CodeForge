"""OpenCode backend executor — stub."""

from __future__ import annotations

from codeforge.backends._base import BackendInfo, StubBackendExecutor
from codeforge.config import resolve_backend_path


class OpenCodeExecutor(StubBackendExecutor):
    """Stub executor for the OpenCode backend."""

    def __init__(self, cli_path: str | None = None) -> None:
        self._cli_path = resolve_backend_path(cli_path, "CODEFORGE_OPENCODE_PATH", "opencode")

    @property
    def info(self) -> BackendInfo:
        return BackendInfo(
            name="opencode",
            display_name="OpenCode",
            cli_command=self._cli_path,
            capabilities=["code-edit", "lsp"],
        )
