"""Path handling of the file tools: workspace-relative and absolute paths, .git blocked (KI-77).

The tools resolve every path through codeforge.workspace_fs (KI-95); these
tests check the rules the former resolve_safe_path enforced, through the tools.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import pytest

from codeforge.tools.read_file import ReadFileTool
from codeforge.tools.write_file import WriteFileTool

if TYPE_CHECKING:
    from pathlib import Path


class TestWorkspacePathResolution:
    """Relative paths and absolute paths into the workspace both work."""

    async def test_relative_path(self, tmp_path: Path) -> None:
        (tmp_path / "hello.py").write_text("print('hello')")
        result = await ReadFileTool().execute({"file_path": "hello.py"}, str(tmp_path))
        assert result.success, result.error
        assert "print('hello')" in result.output

    async def test_absolute_path_inside_the_workspace(self, tmp_path: Path) -> None:
        """Go Core sends absolute workspace paths and models often pass absolute file paths."""
        (tmp_path / "hello.py").write_text("print('hello')")
        result = await ReadFileTool().execute({"file_path": str(tmp_path / "hello.py")}, str(tmp_path))
        assert result.success, result.error
        assert "print('hello')" in result.output

    async def test_absolute_path_outside_the_workspace(self, tmp_path: Path) -> None:
        ws = tmp_path / "ws"
        ws.mkdir()
        (tmp_path / "secret.txt").write_text("secret")
        result = await ReadFileTool().execute({"file_path": str(tmp_path / "secret.txt")}, str(ws))
        assert not result.success
        assert "leaves the workspace" in result.error


class TestGitMetadataBlocked:
    """File tools never reach .git (KI-77): its config and hooks make git run programs."""

    @pytest.mark.parametrize(
        "relative",
        [
            ".git/config",
            ".git",
            ".git/hooks/pre-commit",
            "sub/.git/config",
            ".GIT/config",
            "./.git/info/attributes",
            "a/../.git/config",
        ],
    )
    async def test_git_paths_refused(self, tmp_path: Path, relative: str) -> None:
        (tmp_path / ".git" / "hooks").mkdir(parents=True)
        (tmp_path / ".git" / "config").write_text("[core]\n")
        (tmp_path / "sub" / ".git").mkdir(parents=True)
        for tool, arguments in (
            (ReadFileTool(), {"file_path": relative}),
            (WriteFileTool(), {"file_path": relative, "content": "x"}),
        ):
            result = await tool.execute(arguments, str(tmp_path))
            assert result.success is False
            assert "access to .git is blocked" in result.error
        assert (tmp_path / ".git" / "config").read_text() == "[core]\n"

    async def test_symlink_into_git_refused(self, tmp_path: Path) -> None:
        (tmp_path / ".git").mkdir()
        (tmp_path / ".git" / "config").write_text("[core]\n")
        (tmp_path / "innocent").symlink_to(tmp_path / ".git")
        (tmp_path / "cfg").symlink_to(".git/config")
        for relative in ("innocent/config", "cfg"):
            result = await WriteFileTool().execute({"file_path": relative, "content": "x"}, str(tmp_path))
            assert result.success is False
        assert (tmp_path / ".git" / "config").read_text() == "[core]\n"

    @pytest.mark.parametrize("relative", [".gitignore", ".github/workflows/ci.yml", "src/.gitkeep", "git/config"])
    async def test_similar_names_allowed(self, tmp_path: Path, relative: str) -> None:
        result = await WriteFileTool().execute({"file_path": relative, "content": "x"}, str(tmp_path))
        assert result.success, result.error
        assert (tmp_path / relative).read_text() == "x"
