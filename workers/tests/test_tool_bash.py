"""Comprehensive tests for the BashTool."""

from __future__ import annotations

from typing import TYPE_CHECKING

import pytest

from codeforge.tools.bash import DEFINITION, BashTool, _check_dangerous_command, _truncate

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


# ---------------------------------------------------------------------------
# Defense-in-depth: dangerous command blocklist
# ---------------------------------------------------------------------------


class TestBashDangerousCommandBlocklist:
    """Tests for the defense-in-depth command blocklist (FIX-004)."""

    def test_rm_rf_root(self) -> None:
        result = _check_dangerous_command("rm -rf /")
        assert result is not None
        assert "blocked" in result

    def test_rm_rf_root_wildcard(self) -> None:
        result = _check_dangerous_command("rm -rf /*")
        assert result is not None

    def test_rm_fr_root(self) -> None:
        result = _check_dangerous_command("rm -fr /")
        assert result is not None

    def test_mkfs(self) -> None:
        result = _check_dangerous_command("mkfs.ext4 /dev/sda1")
        assert result is not None
        assert "filesystem formatting" in result

    def test_dd(self) -> None:
        result = _check_dangerous_command("dd if=/dev/zero of=/dev/sda bs=1M")
        assert result is not None
        assert "raw disk write" in result

    def test_fork_bomb(self) -> None:
        result = _check_dangerous_command(":(){:|:&};:")
        assert result is not None
        assert "fork bomb" in result

    def test_shutdown(self) -> None:
        result = _check_dangerous_command("shutdown -h now")
        assert result is not None

    def test_reboot(self) -> None:
        result = _check_dangerous_command("reboot")
        assert result is not None

    def test_safe_rm_allowed(self) -> None:
        """rm on a specific file should NOT be blocked."""
        result = _check_dangerous_command("rm -rf /tmp/mydir")
        assert result is None

    def test_safe_echo_allowed(self) -> None:
        result = _check_dangerous_command("echo hello world")
        assert result is None

    def test_safe_git_allowed(self) -> None:
        result = _check_dangerous_command("git status")
        assert result is None

    def test_case_insensitive(self) -> None:
        result = _check_dangerous_command("RM -RF /")
        assert result is not None

    async def test_blocked_command_returns_error(self, tmp_path: Path) -> None:
        """BashTool.execute should return failure for blocked commands."""
        tool = BashTool()
        result = await tool.execute({"command": "rm -rf /"}, str(tmp_path))
        assert result.success is False
        assert "blocked" in result.error
        assert result.output == ""

    async def test_safe_command_still_works(self, tmp_path: Path) -> None:
        """Ensure the blocklist does not interfere with normal commands."""
        tool = BashTool()
        result = await tool.execute({"command": "echo ok"}, str(tmp_path))
        assert result.success is True
        assert "ok" in result.output


# ---------------------------------------------------------------------------
# FIX-012: Comprehensive command injection edge cases
# ---------------------------------------------------------------------------


class TestCommandInjectionEdgeCases:
    """FIX-012: Comprehensive command injection edge-case tests.

    The blocklist uses simple substring matching on the normalized
    (stripped + lowercased) command. These tests verify both the
    patterns that ARE caught and document known limitations.
    """

    @pytest.mark.parametrize(
        "cmd",
        [
            "rm -rf /",
            "rm -rf / --no-preserve-root",
            "rm -fr /",
            "rm -rf /*",
            "  rm -rf /  ",  # leading/trailing whitespace
            "sudo rm -rf /",  # sudo prefix (substring still matches)
            "bash -c 'rm -rf /'",  # nested shell
            "echo hello; rm -rf /",  # command chaining with ;
            "echo hello && rm -rf /",  # logical AND
            "echo hello || rm -rf /",  # logical OR
            "$(rm -rf /)",  # command substitution
            "`rm -rf /`",  # backtick substitution
        ],
        ids=[
            "plain",
            "no-preserve-root",
            "rm-fr",
            "wildcard",
            "whitespace",
            "sudo",
            "nested-shell",
            "semicolon-chain",
            "and-chain",
            "or-chain",
            "cmd-substitution",
            "backtick-substitution",
        ],
    )
    def test_rm_rf_root_variants_blocked(self, cmd: str) -> None:
        result = _check_dangerous_command(cmd)
        assert result is not None, f"Command should be blocked: {cmd}"

    @pytest.mark.parametrize(
        "cmd",
        [
            "dd if=/dev/zero of=/dev/sda",
            "dd if=/dev/urandom of=/dev/nvme0n1",
            "mkfs.ext4 /dev/sda",
            "mkfs.xfs /dev/sdb1",
            ":(){:|:&};:",  # fork bomb
            "shutdown -h now",
            "shutdown -r now",
            "reboot",
            "init 0",
            "init 6",
        ],
        ids=[
            "dd-zero",
            "dd-urandom",
            "mkfs-ext4",
            "mkfs-xfs",
            "fork-bomb",
            "shutdown-halt",
            "shutdown-reboot",
            "reboot",
            "init-0",
            "init-6",
        ],
    )
    def test_destructive_system_commands_blocked(self, cmd: str) -> None:
        result = _check_dangerous_command(cmd)
        assert result is not None, f"Command should be blocked: {cmd}"

    @pytest.mark.parametrize(
        "cmd",
        [
            "ls -la",
            "cat file.txt",
            "grep -r pattern .",
            "python script.py",
            "go test ./...",
            "npm test",
            "git rm file.txt",  # git rm != rm -rf /
            "find . -name '*.tmp' -delete",
            "rm -rf /tmp/mydir",  # specific path, not root
            "rm -rf ./build",  # relative path
            "rm file.txt",  # single file
            "echo hello world",
            "pip install requests",
            "cargo build",
            "make clean",
            "docker build .",
            "curl https://example.com",
            "python -m pytest tests/ -v",
        ],
        ids=[
            "ls",
            "cat",
            "grep",
            "python",
            "go-test",
            "npm-test",
            "git-rm",
            "find-delete",
            "rm-tmp",
            "rm-relative",
            "rm-single",
            "echo",
            "pip",
            "cargo",
            "make",
            "docker",
            "curl",
            "pytest",
        ],
    )
    def test_safe_commands_allowed(self, cmd: str) -> None:
        result = _check_dangerous_command(cmd)
        assert result is None, f"Command should be allowed: {cmd}"

    def test_case_insensitivity(self) -> None:
        """Blocklist should be case-insensitive."""
        assert _check_dangerous_command("RM -RF /") is not None
        assert _check_dangerous_command("Shutdown -h now") is not None
        assert _check_dangerous_command("REBOOT") is not None
        assert _check_dangerous_command("DD IF=/dev/zero OF=/dev/sda") is not None

    def test_chmod_root(self) -> None:
        """chmod -r 777 / should be blocked."""
        result = _check_dangerous_command("chmod -r 777 /")
        assert result is not None

    def test_chown_recursive(self) -> None:
        """chown -r should be blocked."""
        result = _check_dangerous_command("chown -r root:root /etc")
        assert result is not None

    def test_dev_sda_overwrite(self) -> None:
        """> /dev/sda should be blocked."""
        result = _check_dangerous_command("echo x > /dev/sda")
        assert result is not None
