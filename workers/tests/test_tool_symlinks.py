"""File tools never follow a symlink out of the workspace and never block on a FIFO (KI-95).

The tools run in the worker's own process, with rights the tool user (who
writes the workspace) does not have: a symlink an agent planted must not make
them read or write files outside the workspace.
"""

from __future__ import annotations

import os
import threading
from typing import TYPE_CHECKING

import pytest

from codeforge.tools.edit_file import EditFileTool
from codeforge.tools.glob_files import GlobFilesTool
from codeforge.tools.list_directory import ListDirectoryTool
from codeforge.tools.read_file import ReadFileTool
from codeforge.tools.search_files import SearchFilesTool
from codeforge.tools.write_file import WriteFileTool

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable
    from pathlib import Path

    from codeforge.tools._base import ToolResult

OUTSIDE_TEXT = "outside-only content"


@pytest.fixture
def ws(tmp_path: Path) -> Path:
    """A workspace with in-workspace and outside symlinks, next to an outside directory."""
    workspace = tmp_path / "ws"
    outside = tmp_path / "out"
    (workspace / "src" / "sub").mkdir(parents=True)
    outside.mkdir()
    (outside / "secret.py").write_text(OUTSIDE_TEXT)
    (workspace / "src" / "a.py").write_text("inside = 1\n")
    (workspace / "src" / "sub" / "b.py").write_text("deep = 2\n")
    (workspace / "leak.py").symlink_to("../out/secret.py")
    (workspace / "leak_abs.py").symlink_to(outside / "secret.py")
    (workspace / "outdir").symlink_to("../out")
    (workspace / "alias.py").symlink_to("src/a.py")
    (workspace / "srclink").symlink_to("src")
    return workspace


def _outside(ws: Path) -> Path:
    return ws.parent / "out"


def _run_without_blocking(call: Callable[[], Awaitable[ToolResult]], seconds: float = 10.0) -> ToolResult:
    """Run a tool call on its own event loop in a thread; fail when it blocks (a FIFO opened blocking)."""
    import asyncio

    results: list[ToolResult] = []
    thread = threading.Thread(target=lambda: results.append(asyncio.run(call())), daemon=True)
    thread.start()
    thread.join(seconds)
    assert not thread.is_alive(), "the tool blocked"
    return results[0]


class TestReadFile:
    @pytest.mark.parametrize("rel", ["leak.py", "leak_abs.py", "outdir/secret.py", "srclink/../../out/secret.py"])
    async def test_outside_refused(self, ws: Path, rel: str) -> None:
        result = await ReadFileTool().execute({"file_path": rel}, str(ws))
        assert not result.success
        assert "leaves the workspace" in result.error
        assert OUTSIDE_TEXT not in result.output

    async def test_inside_symlinks_followed(self, ws: Path) -> None:
        for rel in ("alias.py", "srclink/a.py"):
            result = await ReadFileTool().execute({"file_path": rel}, str(ws))
            assert result.success, result.error
            assert "inside = 1" in result.output

    def test_fifo_does_not_block(self, ws: Path) -> None:
        os.mkfifo(ws / "pipe.py")
        result = _run_without_blocking(lambda: ReadFileTool().execute({"file_path": "pipe.py"}, str(ws)))
        assert not result.success
        assert "not a regular file" in result.error

    async def test_size_cap(self, ws: Path, monkeypatch: pytest.MonkeyPatch) -> None:
        import codeforge.tools.read_file as read_file

        monkeypatch.setattr(read_file, "MAX_WORKSPACE_FILE_BYTES", 4)
        result = await ReadFileTool().execute({"file_path": "src/a.py"}, str(ws))
        assert not result.success
        assert "file too large" in result.error


class TestWriteFile:
    @pytest.mark.parametrize("rel", ["leak.py", "leak_abs.py", "outdir/secret.py", "outdir/new.py", "outdir/x/y.py"])
    async def test_outside_refused(self, ws: Path, rel: str) -> None:
        result = await WriteFileTool().execute({"file_path": rel, "content": "pwned"}, str(ws))
        assert not result.success
        assert "leaves the workspace" in result.error
        assert (_outside(ws) / "secret.py").read_text() == OUTSIDE_TEXT
        assert sorted(p.name for p in _outside(ws).iterdir()) == ["secret.py"]

    async def test_inside_symlink_writes_its_target(self, ws: Path) -> None:
        result = await WriteFileTool().execute({"file_path": "alias.py", "content": "changed = 1\n"}, str(ws))
        assert result.success, result.error
        assert (ws / "src" / "a.py").read_text() == "changed = 1\n"
        assert (ws / "alias.py").is_symlink()
        assert result.diff is not None
        assert result.diff["hunks"][0]["old_content"] == "inside = 1\n"

    def test_fifo_does_not_block(self, ws: Path) -> None:
        os.mkfifo(ws / "pipe.py")
        result = _run_without_blocking(
            lambda: WriteFileTool().execute({"file_path": "pipe.py", "content": "x"}, str(ws))
        )
        assert not result.success
        assert "not a regular file" in result.error


class TestEditFile:
    @pytest.mark.parametrize("rel", ["leak.py", "outdir/secret.py"])
    async def test_outside_refused(self, ws: Path, rel: str) -> None:
        arguments = {"file_path": rel, "old_text": "outside", "new_text": "pwned"}
        result = await EditFileTool().execute(arguments, str(ws))
        assert not result.success
        assert "leaves the workspace" in result.error
        assert (_outside(ws) / "secret.py").read_text() == OUTSIDE_TEXT

    async def test_inside_symlink_edits_its_target(self, ws: Path) -> None:
        arguments = {"file_path": "srclink/a.py", "old_text": "inside = 1", "new_text": "inside = 3"}
        result = await EditFileTool().execute(arguments, str(ws))
        assert result.success, result.error
        assert (ws / "src" / "a.py").read_text() == "inside = 3\n"

    def test_fifo_does_not_block(self, ws: Path) -> None:
        os.mkfifo(ws / "pipe.py")
        arguments = {"file_path": "pipe.py", "old_text": "a", "new_text": "b"}
        result = _run_without_blocking(lambda: EditFileTool().execute(arguments, str(ws)))
        assert not result.success
        assert "not a regular file" in result.error


class TestListDirectory:
    async def test_symlinks_listed_never_descended(self, ws: Path) -> None:
        os.mkfifo(ws / "pipe")
        result = await ListDirectoryTool().execute({"path": ".", "recursive": True}, str(ws))
        assert result.success, result.error
        lines = result.output.splitlines()
        assert "[DIR]  srclink" in lines  # resolves to a directory inside
        assert "[FILE] outdir" in lines  # leaves the workspace: not a directory of it
        assert "[FILE] pipe" in lines
        assert "[FILE] src/sub/b.py" in lines
        assert not [line for line in lines if line.startswith(("[DIR]  srclink/", "[FILE] srclink/", "[FILE] outdir/"))]
        assert "secret" not in result.output

    async def test_listing_outside_refused(self, ws: Path) -> None:
        result = await ListDirectoryTool().execute({"path": "outdir"}, str(ws))
        assert not result.success
        assert "leaves the workspace" in result.error

    async def test_listing_inside_symlink(self, ws: Path) -> None:
        result = await ListDirectoryTool().execute({"path": "srclink"}, str(ws))
        assert result.success, result.error
        assert result.output.splitlines() == ["[DIR]  src/sub", "[FILE] src/a.py"]


class TestGlobFiles:
    async def test_outside_never_matched(self, ws: Path) -> None:
        os.mkfifo(ws / "pipe.py")
        for pattern in ("**/*.py", "*/*.py", "*.py", "outdir/*.py", "outdir/**/*.py"):
            result = await GlobFilesTool().execute({"pattern": pattern}, str(ws))
            assert result.success, result.error
            assert "secret" not in result.output, pattern
            assert "leak" not in result.output, pattern
            assert "pipe" not in result.output, pattern

    async def test_symlinked_file_inside_matched(self, ws: Path) -> None:
        result = await GlobFilesTool().execute({"pattern": "*.py"}, str(ws))
        assert result.output.splitlines() == ["alias.py"]

    async def test_symlinked_directories_not_descended(self, ws: Path) -> None:
        result = await GlobFilesTool().execute({"pattern": "**/*.py"}, str(ws))
        assert result.output.splitlines() == ["alias.py", "src/a.py", "src/sub/b.py"]

    async def test_literal_prefix_through_inside_symlink(self, ws: Path) -> None:
        result = await GlobFilesTool().execute({"pattern": "srclink/*.py"}, str(ws))
        assert result.output.splitlines() == ["src/a.py"]

    async def test_absolute_pattern_outside_refused(self, ws: Path) -> None:
        result = await GlobFilesTool().execute({"pattern": f"{_outside(ws)}/*.py"}, str(ws))
        assert not result.success
        assert "leaves the workspace" in result.error


class TestSearchFiles:
    @pytest.mark.parametrize("path", ["outdir", "srclink/../../out"])
    async def test_outside_directory_refused(self, ws: Path, path: str) -> None:
        result = await SearchFilesTool().execute({"pattern": "outside", "path": path}, str(ws))
        assert not result.success
        assert "leaves the workspace" in result.error

    async def test_inside_symlinked_directory(self, ws: Path) -> None:
        result = await SearchFilesTool().execute({"pattern": "inside", "path": "srclink"}, str(ws))
        assert result.success, result.error
        assert "a.py" in result.output
