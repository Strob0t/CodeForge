"""Tests for the Aider backend executor."""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.backends.aider import AiderExecutor


@pytest.fixture
def executor() -> AiderExecutor:
    return AiderExecutor(cli_path="/usr/bin/aider")


class TestCheckAvailable:
    """Tests for check_available()."""

    @pytest.mark.asyncio
    async def test_available_via_shutil_which(self, executor: AiderExecutor) -> None:
        with patch("codeforge.subprocess_utils.shutil.which", return_value="/usr/bin/aider"):
            assert await executor.check_available() is True

    @pytest.mark.asyncio
    async def test_available_via_subprocess_fallback(self, executor: AiderExecutor) -> None:
        mock_proc = AsyncMock()
        mock_proc.returncode = 0
        mock_proc.communicate = AsyncMock(return_value=(b"aider 0.50.0", b""))

        with (
            patch("codeforge.subprocess_utils.shutil.which", return_value=None),
            patch("asyncio.create_subprocess_exec", return_value=mock_proc),
        ):
            assert await executor.check_available() is True

    @pytest.mark.asyncio
    async def test_not_available(self, executor: AiderExecutor) -> None:
        with (
            patch("codeforge.subprocess_utils.shutil.which", return_value=None),
            patch("asyncio.create_subprocess_exec", side_effect=OSError("not found")),
        ):
            assert await executor.check_available() is False


class TestExecute:
    """Tests for execute()."""

    @pytest.mark.asyncio
    async def test_successful_execution(self, executor: AiderExecutor) -> None:
        mock_stdout = AsyncMock()
        mock_stdout.readline = AsyncMock(side_effect=[b"Processing...\n", b"Done.\n", b""])

        mock_proc = AsyncMock()
        mock_proc.stdout = mock_stdout
        mock_proc.returncode = 0
        mock_proc.wait = AsyncMock()

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc):
            result = await executor.execute("t1", "fix the bug", "/workspace")

        assert result.status == "completed"
        assert "Processing..." in result.output
        assert "Done." in result.output

    @pytest.mark.asyncio
    async def test_failed_execution(self, executor: AiderExecutor) -> None:
        mock_stdout = AsyncMock()
        mock_stdout.readline = AsyncMock(side_effect=[b"Error: model not found\n", b""])

        mock_proc = AsyncMock()
        mock_proc.stdout = mock_stdout
        mock_proc.returncode = 1
        mock_proc.wait = AsyncMock()

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc):
            result = await executor.execute("t1", "fix the bug", "/workspace")

        assert result.status == "failed"
        assert "exited with code 1" in result.error

    @pytest.mark.asyncio
    async def test_timeout(self, executor: AiderExecutor) -> None:
        mock_stdout = AsyncMock()
        mock_stdout.readline = AsyncMock(side_effect=TimeoutError)

        mock_proc = AsyncMock()
        mock_proc.stdout = mock_stdout
        mock_proc.returncode = None
        mock_proc.terminate = MagicMock()
        mock_proc.wait = AsyncMock()

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc):
            result = await executor.execute("t1", "fix the bug", "/workspace", config={"timeout": 5})

        assert result.status == "failed"
        assert "timed out" in result.error
        mock_proc.terminate.assert_called_once()

    @pytest.mark.asyncio
    async def test_os_error_starting_process(self, executor: AiderExecutor) -> None:
        with patch(
            "asyncio.create_subprocess_exec",
            side_effect=OSError("Permission denied"),
        ):
            result = await executor.execute("t1", "fix the bug", "/workspace")

        assert result.status == "failed"
        assert "Failed to start aider" in result.error

    @pytest.mark.asyncio
    async def test_output_callback_invoked(self, executor: AiderExecutor) -> None:
        mock_stdout = AsyncMock()
        mock_stdout.readline = AsyncMock(side_effect=[b"line1\n", b"line2\n", b""])

        mock_proc = AsyncMock()
        mock_proc.stdout = mock_stdout
        mock_proc.returncode = 0
        mock_proc.wait = AsyncMock()

        callback = AsyncMock()

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc):
            await executor.execute("t1", "fix the bug", "/workspace", on_output=callback)

        assert callback.call_count == 2
        callback.assert_any_call("line1")
        callback.assert_any_call("line2")

    @pytest.mark.asyncio
    async def test_model_and_api_base_passed_to_cmd(self, executor: AiderExecutor) -> None:
        mock_stdout = AsyncMock()
        mock_stdout.readline = AsyncMock(side_effect=[b""])

        mock_proc = AsyncMock()
        mock_proc.stdout = mock_stdout
        mock_proc.returncode = 0
        mock_proc.wait = AsyncMock()

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc) as mock_exec:
            await executor.execute(
                "t1",
                "prompt",
                "/workspace",
                config={"model": "gpt-4", "openai_api_base": "http://llm:4000"},
            )

        cmd_args = mock_exec.call_args[0]
        assert "--model" in cmd_args
        assert "gpt-4" in cmd_args
        assert "--openai-api-base" in cmd_args
        assert "http://llm:4000" in cmd_args


class TestExtraArgs:
    """Tests for extra_args config passthrough (A3)."""

    @pytest.mark.asyncio
    async def test_extra_args_list_appended(self, executor: AiderExecutor) -> None:
        """extra_args as list is appended to the command."""
        mock_stdout = AsyncMock()
        mock_stdout.readline = AsyncMock(side_effect=[b""])

        mock_proc = AsyncMock()
        mock_proc.stdout = mock_stdout
        mock_proc.returncode = 0
        mock_proc.wait = AsyncMock()

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc) as mock_exec:
            await executor.execute(
                "t1",
                "prompt",
                "/workspace",
                config={"extra_args": ["--verbose", "--dark-mode"]},
            )

        cmd_args = mock_exec.call_args[0]
        assert "--verbose" in cmd_args
        assert "--dark-mode" in cmd_args

    @pytest.mark.asyncio
    async def test_extra_args_json_string_parsed(self, executor: AiderExecutor) -> None:
        """extra_args as JSON string is parsed and appended."""
        mock_stdout = AsyncMock()
        mock_stdout.readline = AsyncMock(side_effect=[b""])

        mock_proc = AsyncMock()
        mock_proc.stdout = mock_stdout
        mock_proc.returncode = 0
        mock_proc.wait = AsyncMock()

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc) as mock_exec:
            await executor.execute(
                "t1",
                "prompt",
                "/workspace",
                config={"extra_args": '["--verbose", "--dark-mode"]'},
            )

        cmd_args = mock_exec.call_args[0]
        assert "--verbose" in cmd_args
        assert "--dark-mode" in cmd_args

    @pytest.mark.asyncio
    async def test_no_extra_args_no_change(self, executor: AiderExecutor) -> None:
        """Without extra_args, command is unchanged."""
        mock_stdout = AsyncMock()
        mock_stdout.readline = AsyncMock(side_effect=[b""])

        mock_proc = AsyncMock()
        mock_proc.stdout = mock_stdout
        mock_proc.returncode = 0
        mock_proc.wait = AsyncMock()

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc) as mock_exec:
            await executor.execute("t1", "prompt", "/workspace")

        cmd_args = mock_exec.call_args[0]
        # Base command: aider --yes-always --no-auto-commits --message prompt
        assert cmd_args == ("/usr/bin/aider", "--yes-always", "--no-auto-commits", "--message", "prompt")

    @pytest.mark.asyncio
    async def test_empty_list_no_change(self, executor: AiderExecutor) -> None:
        """Empty extra_args list has no effect."""
        mock_stdout = AsyncMock()
        mock_stdout.readline = AsyncMock(side_effect=[b""])

        mock_proc = AsyncMock()
        mock_proc.stdout = mock_stdout
        mock_proc.returncode = 0
        mock_proc.wait = AsyncMock()

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc) as mock_exec:
            await executor.execute(
                "t1",
                "prompt",
                "/workspace",
                config={"extra_args": []},
            )

        cmd_args = mock_exec.call_args[0]
        assert cmd_args == ("/usr/bin/aider", "--yes-always", "--no-auto-commits", "--message", "prompt")


class TestCancel:
    """Tests for cancel()."""

    @pytest.mark.asyncio
    async def test_cancel_terminates_process(self, executor: AiderExecutor) -> None:
        mock_proc = AsyncMock()
        mock_proc.returncode = None
        mock_proc.terminate = MagicMock()
        mock_proc.wait = AsyncMock(return_value=0)

        executor._processes["t1"] = mock_proc

        await executor.cancel("t1")
        mock_proc.terminate.assert_called_once()

    @pytest.mark.asyncio
    async def test_cancel_noop_for_unknown_task(self, executor: AiderExecutor) -> None:
        # Should not raise
        await executor.cancel("no-such-task")

    @pytest.mark.asyncio
    async def test_cancel_noop_for_already_finished(self, executor: AiderExecutor) -> None:
        mock_proc = MagicMock()
        mock_proc.returncode = 0  # Already finished
        mock_proc.terminate = MagicMock()

        executor._processes["t1"] = mock_proc

        await executor.cancel("t1")
        mock_proc.terminate.assert_not_called()
