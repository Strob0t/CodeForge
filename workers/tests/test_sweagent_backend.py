"""Tests for SWE-agent backend executor."""

from __future__ import annotations

from unittest.mock import AsyncMock, patch

import pytest

from codeforge.backends.sweagent import SweagentExecutor


class TestSweagentInfo:
    """Tests for SweagentExecutor metadata."""

    def test_info_name(self) -> None:
        executor = SweagentExecutor(cli_path="/usr/bin/sweagent")
        assert executor.info.name == "sweagent"

    def test_info_display_name(self) -> None:
        executor = SweagentExecutor(cli_path="/usr/bin/sweagent")
        assert executor.info.display_name == "SWE-agent"

    def test_info_capabilities(self) -> None:
        executor = SweagentExecutor(cli_path="/usr/bin/sweagent")
        caps = executor.info.capabilities
        assert "code-edit" in caps
        assert "sandbox" in caps

    def test_info_requires_docker(self) -> None:
        executor = SweagentExecutor(cli_path="/usr/bin/sweagent")
        assert executor.info.requires_docker is True

    def test_config_schema_has_model(self) -> None:
        executor = SweagentExecutor(cli_path="/usr/bin/sweagent")
        keys = [f.key for f in executor.info.config_schema]
        assert "model" in keys
        assert "timeout" in keys


class TestSweagentExecute:
    """Tests for SweagentExecutor.execute()."""

    @pytest.fixture
    def executor(self) -> SweagentExecutor:
        return SweagentExecutor(cli_path="/usr/bin/sweagent")

    @pytest.mark.asyncio
    async def test_successful_execution(self, executor: SweagentExecutor) -> None:
        """Subprocess returns 0 -> completed status."""
        mock_proc = AsyncMock()
        mock_proc.returncode = 0
        mock_proc.stdout = AsyncMock()
        mock_proc.stdout.readline = AsyncMock(side_effect=[b"Thinking...\n", b"Done.\n", b""])
        mock_proc.wait = AsyncMock()

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc):
            result = await executor.execute(
                task_id="t1",
                prompt="Fix the bug",
                workspace_path="/tmp/repo",
            )

        assert result.status == "completed"
        assert "Done." in result.output

    @pytest.mark.asyncio
    async def test_failed_execution(self, executor: SweagentExecutor) -> None:
        """Subprocess returns non-zero -> failed status."""
        mock_proc = AsyncMock()
        mock_proc.returncode = 1
        mock_proc.stdout = AsyncMock()
        mock_proc.stdout.readline = AsyncMock(side_effect=[b"Error: model not found\n", b""])
        mock_proc.wait = AsyncMock()

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc):
            result = await executor.execute(
                task_id="t2",
                prompt="Fix something",
                workspace_path="/tmp/repo",
            )

        assert result.status == "failed"
        assert "exited with code 1" in result.error

    @pytest.mark.asyncio
    async def test_oserror_on_start(self, executor: SweagentExecutor) -> None:
        """OSError when starting subprocess -> failed with message."""
        with patch(
            "asyncio.create_subprocess_exec",
            side_effect=OSError("sweagent not found"),
        ):
            result = await executor.execute(
                task_id="t3",
                prompt="Fix",
                workspace_path="/tmp/repo",
            )

        assert result.status == "failed"
        assert "sweagent not found" in result.error

    @pytest.mark.asyncio
    async def test_timeout_terminates_process(self, executor: SweagentExecutor) -> None:
        """Timeout triggers graceful termination."""
        mock_proc = AsyncMock()
        mock_proc.returncode = None
        mock_proc.stdout = AsyncMock()
        mock_proc.stdout.readline = AsyncMock(side_effect=TimeoutError)
        mock_proc.terminate = AsyncMock()
        mock_proc.wait = AsyncMock()
        mock_proc.kill = AsyncMock()

        with (
            patch("asyncio.create_subprocess_exec", return_value=mock_proc),
            patch("codeforge.backends.sweagent.graceful_terminate", new_callable=AsyncMock) as mock_term,
        ):
            result = await executor.execute(
                task_id="t4",
                prompt="Long task",
                workspace_path="/tmp/repo",
                config={"timeout": 1},
            )

        assert result.status == "failed"
        assert "timed out" in result.error
        mock_term.assert_awaited_once()

    @pytest.mark.asyncio
    async def test_streaming_output_callback(self, executor: SweagentExecutor) -> None:
        """Output lines are streamed to the on_output callback."""
        mock_proc = AsyncMock()
        mock_proc.returncode = 0
        mock_proc.stdout = AsyncMock()
        mock_proc.stdout.readline = AsyncMock(side_effect=[b"Line 1\n", b"Line 2\n", b""])
        mock_proc.wait = AsyncMock()

        lines: list[str] = []

        async def capture(line: str) -> None:
            lines.append(line)

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc):
            await executor.execute(
                task_id="t5",
                prompt="Test",
                workspace_path="/tmp/repo",
                on_output=capture,
            )

        assert lines == ["Line 1", "Line 2"]

    @pytest.mark.asyncio
    async def test_model_config_passed_to_cli(self, executor: SweagentExecutor) -> None:
        """Model name from config is passed as CLI argument."""
        mock_proc = AsyncMock()
        mock_proc.returncode = 0
        mock_proc.stdout = AsyncMock()
        mock_proc.stdout.readline = AsyncMock(side_effect=[b""])
        mock_proc.wait = AsyncMock()

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc) as mock_exec:
            await executor.execute(
                task_id="t6",
                prompt="Fix bug",
                workspace_path="/tmp/repo",
                config={"model": "claude-sonnet-4-6"},
            )

        cmd = mock_exec.call_args[0]
        assert "--agent.model.name" in cmd
        assert "claude-sonnet-4-6" in cmd


class TestSweagentCancel:
    """Tests for SweagentExecutor.cancel()."""

    @pytest.mark.asyncio
    async def test_cancel_running_task(self) -> None:
        """Cancel terminates the running subprocess."""
        executor = SweagentExecutor(cli_path="/usr/bin/sweagent")
        mock_proc = AsyncMock()
        mock_proc.returncode = None
        executor._processes["t1"] = mock_proc

        with patch("codeforge.backends.sweagent.graceful_terminate", new_callable=AsyncMock) as mock_term:
            await executor.cancel("t1")

        mock_term.assert_awaited_once_with(mock_proc)

    @pytest.mark.asyncio
    async def test_cancel_unknown_task(self) -> None:
        """Cancel on unknown task_id is a no-op."""
        executor = SweagentExecutor(cli_path="/usr/bin/sweagent")
        await executor.cancel("nonexistent")  # should not raise


class TestSweagentCheckAvailable:
    """Tests for SweagentExecutor.check_available()."""

    @pytest.mark.asyncio
    async def test_available_when_cli_found(self) -> None:
        executor = SweagentExecutor(cli_path="/usr/bin/sweagent")
        with patch("codeforge.backends.sweagent.check_cli_available", return_value=True):
            assert await executor.check_available() is True

    @pytest.mark.asyncio
    async def test_unavailable_when_cli_missing(self) -> None:
        executor = SweagentExecutor(cli_path="/usr/bin/sweagent")
        with patch("codeforge.backends.sweagent.check_cli_available", return_value=False):
            assert await executor.check_available() is False
