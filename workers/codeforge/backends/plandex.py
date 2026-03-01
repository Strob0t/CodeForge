"""Plandex backend executor — stub."""

from __future__ import annotations

from codeforge.backends._base import BackendInfo, StubBackendExecutor
from codeforge.config import resolve_backend_path


class PlandexExecutor(StubBackendExecutor):
    """Stub executor for the Plandex backend."""

    def __init__(self, cli_path: str | None = None) -> None:
        self._cli_path = resolve_backend_path(cli_path, "CODEFORGE_PLANDEX_PATH", "plandex")

    @property
    def info(self) -> BackendInfo:
        return BackendInfo(
            name="plandex",
            display_name="Plandex",
            cli_command=self._cli_path,
            capabilities=["code-edit", "planning", "multi-file"],
        )
