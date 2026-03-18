"""Tests for Claude Code CLI availability detection."""

from __future__ import annotations

from unittest.mock import AsyncMock, patch

import pytest


@pytest.fixture(autouse=True)
def _reset_cache() -> None:
    """Reset module-level cache between tests."""
    from codeforge import claude_code_availability as mod

    mod._claude_code_available = None
    mod._claude_code_check_time = 0.0


class TestIsClaudeCodeAvailable:
    @pytest.mark.asyncio
    async def test_disabled_returns_false(self) -> None:
        from codeforge.claude_code_availability import is_claude_code_available

        with patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_ENABLED": "false"}):
            assert await is_claude_code_available() is False

    @pytest.mark.asyncio
    async def test_missing_env_returns_false(self) -> None:
        from codeforge.claude_code_availability import is_claude_code_available

        with patch.dict("os.environ", {}, clear=True):
            assert await is_claude_code_available() is False

    @pytest.mark.asyncio
    async def test_enabled_cli_found(self) -> None:
        from codeforge.claude_code_availability import is_claude_code_available

        mock_proc = AsyncMock()
        mock_proc.returncode = 0
        mock_proc.wait = AsyncMock()
        with (
            patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_ENABLED": "true"}),
            patch("asyncio.create_subprocess_exec", return_value=mock_proc),
        ):
            assert await is_claude_code_available() is True

    @pytest.mark.asyncio
    async def test_enabled_cli_not_found(self) -> None:
        from codeforge.claude_code_availability import is_claude_code_available

        with (
            patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_ENABLED": "true"}),
            patch("asyncio.create_subprocess_exec", side_effect=OSError("not found")),
        ):
            assert await is_claude_code_available() is False

    @pytest.mark.asyncio
    async def test_caches_result(self) -> None:
        from codeforge.claude_code_availability import is_claude_code_available

        mock_proc = AsyncMock()
        mock_proc.returncode = 0
        mock_proc.wait = AsyncMock()
        with (
            patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_ENABLED": "true"}),
            patch("asyncio.create_subprocess_exec", return_value=mock_proc) as mock_exec,
        ):
            await is_claude_code_available()
            await is_claude_code_available()
            assert mock_exec.call_count == 1

    @pytest.mark.asyncio
    async def test_cache_expires(self) -> None:
        import time

        from codeforge import claude_code_availability as mod
        from codeforge.claude_code_availability import is_claude_code_available

        mock_proc = AsyncMock()
        mock_proc.returncode = 0
        mock_proc.wait = AsyncMock()
        with (
            patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_ENABLED": "true"}),
            patch("asyncio.create_subprocess_exec", return_value=mock_proc) as mock_exec,
        ):
            await is_claude_code_available()
            mod._claude_code_check_time = time.monotonic() - 400.0
            await is_claude_code_available()
            assert mock_exec.call_count == 2

    @pytest.mark.asyncio
    async def test_timeout_returns_false(self) -> None:
        from codeforge.claude_code_availability import is_claude_code_available

        with (
            patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_ENABLED": "true"}),
            patch("asyncio.create_subprocess_exec", side_effect=TimeoutError()),
        ):
            assert await is_claude_code_available() is False
