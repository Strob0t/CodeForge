"""Other in-process workspace readers and writers stay inside the workspace (KI-95).

Framework detection for the docs prefetch, the benchmark runner's snapshots
and harness files, and the filesystem-state evaluator read or write files an
agent could have replaced by a symlink or a FIFO.
"""

from __future__ import annotations

import json
import os
import threading
from typing import TYPE_CHECKING

import pytest

from codeforge.consumer._conversation import _detect_frameworks
from codeforge.evaluation.evaluators.filesystem_state import FilesystemStateEvaluator
from codeforge.evaluation.providers.base import ExecutionResult, TaskSpec
from codeforge.evaluation.runners.agent import _prepare_test_files, _setup_workspace, _snapshot_files
from codeforge.workspace_fs import PathLeavesWorkspaceError

if TYPE_CHECKING:
    from collections.abc import Callable
    from pathlib import Path

OUTSIDE_TEXT = "outside-only"


@pytest.fixture
def ws(tmp_path: Path) -> Path:
    workspace = tmp_path / "ws"
    outside = tmp_path / "out"
    workspace.mkdir()
    outside.mkdir()
    (outside / "secret.txt").write_text(OUTSIDE_TEXT)
    return workspace


def _without_blocking[T](fn: Callable[[], T], seconds: float = 10.0) -> T:
    result: list[T] = []
    thread = threading.Thread(target=lambda: result.append(fn()), daemon=True)
    thread.start()
    thread.join(seconds)
    assert not thread.is_alive(), "the reader blocked"
    return result[0]


class TestFrameworkDetection:
    def test_manifest_symlinked_outside_is_not_read(self, ws: Path, tmp_path: Path) -> None:
        (tmp_path / "out" / "package.json").write_text(json.dumps({"dependencies": {"react": "18"}}))
        (ws / "package.json").symlink_to("../out/package.json")
        assert _detect_frameworks(str(ws)) == []

    def test_manifest_fifo_does_not_block(self, ws: Path) -> None:
        os.mkfifo(ws / "package.json")
        os.mkfifo(ws / "go.mod")
        (ws / "requirements.txt").write_text("django\n")
        assert _without_blocking(lambda: _detect_frameworks(str(ws))) == ["django"]

    def test_manifest_inside_symlink_is_read(self, ws: Path) -> None:
        (ws / "deps.txt").write_text("fastapi\n")
        (ws / "requirements.txt").symlink_to("deps.txt")
        assert _detect_frameworks(str(ws)) == ["fastapi"]

    def test_odd_package_json_is_ignored(self, ws: Path) -> None:
        (ws / "package.json").write_text('["not", "an", "object"]')
        assert _detect_frameworks(str(ws)) == []


class TestBenchmarkRunner:
    def test_snapshot_skips_symlinks_out_and_fifos(self, ws: Path) -> None:
        (ws / "main.py").write_text("x = 1\n")
        (ws / "leak.txt").symlink_to("../out/secret.txt")
        (ws / "outdir").symlink_to("../out")
        (ws / "alias.py").symlink_to("main.py")
        os.mkfifo(ws / "pipe")
        snapshot = _without_blocking(lambda: _snapshot_files(ws))
        assert snapshot == {"main.py": "x = 1\n", "alias.py": "x = 1\n"}

    def test_setup_refuses_paths_that_leave_the_workspace(self, tmp_path: Path) -> None:
        task = TaskSpec(id="t", name="t", input="", initial_files={"../escaped.py": "x"})
        with pytest.raises(PathLeavesWorkspaceError):
            _setup_workspace(task, str(tmp_path))
        assert not (tmp_path / "escaped.py").exists()

    def test_harness_replaces_a_planted_symlink(self, ws: Path, tmp_path: Path) -> None:
        (ws / "solution.py").symlink_to("../out/secret.txt")
        os.mkfifo(ws / "test_patch.diff")
        task = TaskSpec(
            id="t", name="t", input="", metadata={"test_harness": "{SOLUTION}\nassert True\n", "test_patch": "diff"}
        )
        _without_blocking(lambda: _prepare_test_files(task, ws, "def f(): pass"))
        assert (tmp_path / "out" / "secret.txt").read_text() == OUTSIDE_TEXT
        assert not (ws / "solution.py").is_symlink()
        assert (ws / "solution.py").read_text().startswith("def f(): pass")
        assert (ws / "test_patch.diff").read_text() == "diff"


class TestFilesystemStateEvaluator:
    async def test_symlink_outside_is_not_read(self, ws: Path) -> None:
        (ws / "result.txt").symlink_to("../out/secret.txt")
        task = TaskSpec(
            id="t",
            name="t",
            input="",
            metadata={"expected_files": json.dumps({"result.txt": OUTSIDE_TEXT})},
        )
        dims = await FilesystemStateEvaluator(str(ws)).evaluate(task, ExecutionResult(actual_output=""))
        assert dims[0].score == 0.0
        assert "missing: result.txt" in dims[0].details["failures"]

    async def test_fifo_does_not_block(self, ws: Path) -> None:
        os.mkfifo(ws / "result.txt")
        task = TaskSpec(id="t", name="t", input="", metadata={"expected_files": json.dumps({"result.txt": "x"})})
        dims = await FilesystemStateEvaluator(str(ws)).evaluate(task, ExecutionResult(actual_output=""))
        assert dims[0].score == 0.0

    async def test_symlink_outside_counts_as_existing(self, ws: Path) -> None:
        (ws / "gone.txt").symlink_to("../out/secret.txt")
        task = TaskSpec(id="t", name="t", input="", metadata={"expected_missing": json.dumps(["gone.txt", "absent"])})
        dims = await FilesystemStateEvaluator(str(ws)).evaluate(task, ExecutionResult(actual_output=""))
        assert dims[0].details["passed"] == "1"
        assert "should not exist: gone.txt" in dims[0].details["failures"]
