"""Test that workspace path resolution works with both absolute and relative paths."""

from pathlib import Path

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
