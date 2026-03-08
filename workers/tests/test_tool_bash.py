"""Comprehensive tests for the BashTool."""

from __future__ import annotations

from typing import TYPE_CHECKING

from codeforge.tools.bash import DEFINITION, BashTool, _truncate

if TYPE_CHECKING:
    from pathlib import Path


# ---------------------------------------------------------------------------
# Definition
# ---------------------------------------------------------------------------


class TestBashDefinition:
    """Tests for the DEFINITION constant."""

    def test_name(self) -> None:
        assert DEFINITION.name == "bash"

    def test_has_description(self) -> None:
        assert DEFINITION.description

    def test_command_is_required(self) -> None:
        assert "command" in DEFINITION.parameters.get("required", [])

    def test_has_timeout_param(self) -> None:
        props = DEFINITION.parameters.get("properties", {})
        assert "timeout" in props

    def test_has_examples(self) -> None:
        assert len(DEFINITION.examples) > 0


# ---------------------------------------------------------------------------
# Basic execution
# ---------------------------------------------------------------------------


class TestBashBasicExecution:
    """Tests for basic command execution."""

    async def test_simple_echo(self, tmp_path: Path) -> None:
        tool = BashTool()
        result = await tool.execute({"command": "echo hello"}, str(tmp_path))
        assert result.success is True
        assert result.error == ""
        assert "hello" in result.output

    async def test_command_output_stripped_newline(self, tmp_path: Path) -> None:
        tool = BashTool()
        result = await tool.execute({"command": "echo -n exact"}, str(tmp_path))
        assert result.success is True
        assert "exact" in result.output

    async def test_multiline_output(self, tmp_path: Path) -> None:
        tool = BashTool()
        result = await tool.execute(
            {"command": "echo 'line1'; echo 'line2'; echo 'line3'"},
            str(tmp_path),
        )
        assert result.success is True
        assert "line1" in result.output
        assert "line2" in result.output
        assert "line3" in result.output

    async def test_runs_in_workspace_directory(self, tmp_path: Path) -> None:
        tool = BashTool()
        result = await tool.execute({"command": "pwd"}, str(tmp_path))
        assert result.success is True
        assert str(tmp_path) in result.output

    async def test_can_access_workspace_files(self, tmp_path: Path) -> None:
        (tmp_path / "test.txt").write_text("file content")
        tool = BashTool()
        result = await tool.execute({"command": "cat test.txt"}, str(tmp_path))
        assert result.success is True
        assert "file content" in result.output

    async def test_pipe_commands(self, tmp_path: Path) -> None:
        tool = BashTool()
        result = await tool.execute(
            {"command": "echo 'apple\nbanana\ncherry' | grep banana"},
            str(tmp_path),
        )
        assert result.success is True
        assert "banana" in result.output

    async def test_environment_variables(self, tmp_path: Path) -> None:
        tool = BashTool()
        result = await tool.execute(
            {"command": "export MY_VAR=test123 && echo $MY_VAR"},
            str(tmp_path),
        )
        assert result.success is True
        assert "test123" in result.output


# ---------------------------------------------------------------------------
# Exit codes and stderr
# ---------------------------------------------------------------------------


class TestBashExitCodes:
    """Tests for non-zero exit codes and stderr handling."""

    async def test_nonzero_exit_code(self, tmp_path: Path) -> None:
        tool = BashTool()
        result = await tool.execute({"command": "exit 1"}, str(tmp_path))
        assert result.success is False
        assert "exit code 1" in result.error

    async def test_exit_code_42(self, tmp_path: Path) -> None:
        tool = BashTool()
        result = await tool.execute({"command": "exit 42"}, str(tmp_path))
        assert result.success is False
        assert "exit code 42" in result.error

    async def test_stderr_included_in_output(self, tmp_path: Path) -> None:
        tool = BashTool()
        result = await tool.execute(
            {"command": "echo 'stdout msg' && echo 'stderr msg' >&2 && exit 1"},
            str(tmp_path),
        )
        assert result.success is False
        assert "stdout msg" in result.output
        assert "stderr msg" in result.output
        assert "--- stderr ---" in result.output

    async def test_stderr_only_no_stdout(self, tmp_path: Path) -> None:
        tool = BashTool()
        result = await tool.execute(
            {"command": "echo 'error' >&2 && exit 1"},
            str(tmp_path),
        )
        assert result.success is False
        assert "error" in result.output

    async def test_command_not_found(self, tmp_path: Path) -> None:
        tool = BashTool()
        result = await tool.execute(
            {"command": "nonexistent_command_xyz_123"},
            str(tmp_path),
        )
        assert result.success is False
        assert "exit code" in result.error

    async def test_success_with_stderr_output(self, tmp_path: Path) -> None:
        """Commands that write to stderr but exit 0 should still be success."""
        tool = BashTool()
        result = await tool.execute(
            {"command": "echo 'ok' && echo 'warning' >&2"},
            str(tmp_path),
        )
        assert result.success is True
        assert "ok" in result.output
        assert "warning" in result.output


# ---------------------------------------------------------------------------
# Timeout
# ---------------------------------------------------------------------------


class TestBashTimeout:
    """Tests for timeout enforcement."""

    async def test_timeout_kills_long_command(self, tmp_path: Path) -> None:
        tool = BashTool()
        result = await tool.execute(
            {"command": "sleep 60", "timeout": 1},
            str(tmp_path),
        )
        assert result.success is False
        assert "timed out" in result.error
        assert "1s" in result.error

    async def test_default_timeout_is_120(self, tmp_path: Path) -> None:
        """Fast command should succeed within default timeout."""
        tool = BashTool()
        result = await tool.execute({"command": "echo fast"}, str(tmp_path))
        assert result.success is True

    async def test_custom_timeout_sufficient(self, tmp_path: Path) -> None:
        tool = BashTool()
        result = await tool.execute(
            {"command": "sleep 0.1 && echo done", "timeout": 10},
            str(tmp_path),
        )
        assert result.success is True
        assert "done" in result.output


# ---------------------------------------------------------------------------
# Truncation
# ---------------------------------------------------------------------------


class TestBashTruncation:
    """Tests for the _truncate helper function."""

    def test_short_text_unchanged(self) -> None:
        text = "short output"
        assert _truncate(text) == text

    def test_exact_limit_unchanged(self) -> None:
        from codeforge.tools.bash import MAX_OUTPUT

        text = "x" * MAX_OUTPUT
        assert _truncate(text) == text

    def test_over_limit_truncated(self) -> None:
        from codeforge.tools.bash import HALF_OUTPUT, MAX_OUTPUT

        text = "x" * (MAX_OUTPUT + 1000)
        result = _truncate(text)
        assert "... truncated ..." in result
        assert len(result) < len(text)
        # Should preserve head and tail
        assert result.startswith("x" * HALF_OUTPUT)
        assert result.endswith("x" * HALF_OUTPUT)


# ---------------------------------------------------------------------------
# Edge cases
# ---------------------------------------------------------------------------


class TestBashEdgeCases:
    """Tests for edge cases."""

    async def test_empty_command(self, tmp_path: Path) -> None:
        tool = BashTool()
        result = await tool.execute({"command": ""}, str(tmp_path))
        # Empty command is valid bash (does nothing, exit 0)
        assert result.success is True

    async def test_command_with_special_characters(self, tmp_path: Path) -> None:
        tool = BashTool()
        result = await tool.execute(
            {"command": "echo 'hello \"world\"'"},
            str(tmp_path),
        )
        assert result.success is True
        assert 'hello "world"' in result.output

    async def test_binary_output_handled(self, tmp_path: Path) -> None:
        tool = BashTool()
        # printf some non-UTF8 bytes
        result = await tool.execute(
            {"command": "printf '\\x80\\x81\\x82'"},
            str(tmp_path),
        )
        # Should not crash -- uses errors="replace"
        assert result.success is True

    async def test_creates_file_in_workspace(self, tmp_path: Path) -> None:
        tool = BashTool()
        result = await tool.execute(
            {"command": "echo 'created' > output.txt"},
            str(tmp_path),
        )
        assert result.success is True
        assert (tmp_path / "output.txt").exists()
        assert "created" in (tmp_path / "output.txt").read_text()
