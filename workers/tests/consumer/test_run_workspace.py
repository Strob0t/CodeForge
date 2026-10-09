"""A run needs its workspace directory on the worker, as a conversation turn does (KI-193).

Without it the file tools would work in the worker's own directory; the
run is failed before any tool, MCP server or backend starts.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, ClassVar
from unittest.mock import AsyncMock

import pytest
import structlog

import codeforge.consumer._runs as runs_module
from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._runs import RunHandlerMixin
from codeforge.models import RunStartMessage

if TYPE_CHECKING:
    from pathlib import Path


class _Runtime:
    instances: ClassVar[list[_Runtime]] = []

    def __init__(self, **kwargs: object) -> None:
        self.run_id = kwargs["run_id"]
        self.completed_with: list[tuple[str, str]] = []
        self.completed = False
        _Runtime.instances.append(self)

    async def complete_run(self, status: str, error: str = "", **_: object) -> None:
        self.completed_with.append((status, error))
        self.completed = True

    async def start_cancel_listener(self, extra_subjects: list[str] | None = None, after: int | None = None) -> None:
        return None

    async def start_heartbeat(self, interval: float = 30.0) -> None:
        return None

    async def close(self) -> None:
        return None


def _handler() -> RunHandlerMixin:
    handler = type("Handler", (RunHandlerMixin, ConsumerBaseMixin), {})()
    handler._js = AsyncMock()  # type: ignore[attr-defined]
    handler._executor = AsyncMock()  # type: ignore[attr-defined]
    return handler


def _start(workspace: str) -> RunStartMessage:
    return RunStartMessage(
        run_id="run-1", task_id="task-1", project_id="p1", agent_id="a1", prompt="do it", workspace_path=workspace
    )


@pytest.mark.parametrize("workspace", ["", "   ", "missing", "file.txt"])
async def test_a_run_without_a_workspace_directory_fails_before_anything_runs(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch, workspace: str
) -> None:
    (tmp_path / "file.txt").write_text("not a directory\n")
    path = str(tmp_path / workspace) if workspace.strip() else workspace
    monkeypatch.setattr(runs_module, "RuntimeClient", _Runtime)
    _Runtime.instances = []
    handler = _handler()

    await handler._do_run_start(_start(path), structlog.get_logger())

    handler._executor.execute_with_runtime.assert_not_awaited()  # type: ignore[attr-defined]
    [(status, error)] = _Runtime.instances[0].completed_with
    assert status == "failed"
    assert "no usable workspace" in error


async def test_a_run_with_its_workspace_directory_runs(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(runs_module, "RuntimeClient", _Runtime)
    _Runtime.instances = []
    handler = _handler()

    await handler._do_run_start(_start(str(tmp_path)), structlog.get_logger())

    handler._executor.execute_with_runtime.assert_awaited_once()  # type: ignore[attr-defined]
    assert _Runtime.instances[0].completed_with == []
