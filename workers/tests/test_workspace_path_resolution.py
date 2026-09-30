"""Test that workspace path resolution works with both absolute and relative paths."""

from pathlib import Path

import pytest

from codeforge.tools._base import resolve_safe_path


class TestWorkspacePathResolution:
    """Verify resolve_safe_path handles workspace paths correctly."""

    def test_absolute_path_resolves_correctly(self, tmp_path: Path) -> None:
        """Absolute workspace path should resolve files inside it."""
        test_file = tmp_path / "hello.py"
        test_file.write_text("print('hello')")

        resolved, err = resolve_safe_path(str(tmp_path), "hello.py", must_be_file=True)
        assert err is None
        assert resolved == test_file

    def test_absolute_workspace_path_from_nats_resolves_correctly(self, tmp_path: Path) -> None:
        """After fix: Go Core sends absolute paths, so resolution is correct."""
        test_file = tmp_path / "hello.py"
        test_file.write_text("print('hello')")
        resolved, err = resolve_safe_path(str(tmp_path), "hello.py", must_be_file=True)
        assert err is None
        assert resolved == test_file
        assert resolved.is_absolute()


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
    def test_git_paths_refused(self, tmp_path: Path, relative: str) -> None:
        (tmp_path / ".git" / "hooks").mkdir(parents=True)
        (tmp_path / "sub" / ".git").mkdir(parents=True)
        resolved, err = resolve_safe_path(str(tmp_path), relative)
        assert err is not None
        assert err.success is False
        assert ".git" in (err.error or "")
        assert resolved == Path()

    def test_symlink_into_git_refused(self, tmp_path: Path) -> None:
        (tmp_path / ".git").mkdir()
        (tmp_path / "innocent").symlink_to(tmp_path / ".git")
        _, err = resolve_safe_path(str(tmp_path), "innocent/config")
        assert err is not None

    @pytest.mark.parametrize("relative", [".gitignore", ".github/workflows/ci.yml", "src/.gitkeep", "git/config"])
    def test_similar_names_allowed(self, tmp_path: Path, relative: str) -> None:
        _, err = resolve_safe_path(str(tmp_path), relative)
        assert err is None
