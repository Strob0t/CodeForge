"""Tests for Claude Code CLI availability detection.

Routing offers claudecode/* only for a CLI that passes the executor's
capability check (the flags the policy enforcement needs), not merely one
that answers --version.
"""

from __future__ import annotations

from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, patch

import pytest

from codeforge.claude_code_executor import ClaudeCodeCLIError
from codeforge.config import get_settings

if TYPE_CHECKING:
    from collections.abc import Iterator


@pytest.fixture(autouse=True)
def _reset_cache(monkeypatch: pytest.MonkeyPatch) -> Iterator[None]:
    """Reset the availability cache and the cached settings between tests.

    get_settings() is lru_cached, so each test must rebuild it from its patched
    env; a local codeforge.yaml must not leak in either.
    """
    from codeforge import claude_code_availability as mod

    mod._claude_code_available = None
    mod._claude_code_check_time = 0.0
    monkeypatch.setattr("codeforge.config.load_yaml_config", dict)
    get_settings.cache_clear()
    yield
    get_settings.cache_clear()


def _resolve(result: str | Exception) -> AsyncMock:
    if isinstance(result, Exception):
        return AsyncMock(side_effect=result)
    return AsyncMock(return_value=result)


class TestIsClaudeCodeAvailable:
    async def test_disabled_returns_false(self) -> None:
        from codeforge.claude_code_availability import is_claude_code_available

        resolve = _resolve("/usr/bin/claude")
        with (
            patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_ENABLED": "false"}),
            patch("codeforge.claude_code_availability.resolve_cli", resolve),
        ):
            assert await is_claude_code_available() is False
        resolve.assert_not_awaited()

    async def test_missing_env_returns_false(self) -> None:
        from codeforge.claude_code_availability import is_claude_code_available

        with patch.dict("os.environ", {}, clear=True):
            assert await is_claude_code_available() is False

    async def test_supported_cli_is_available(self) -> None:
        from codeforge.claude_code_availability import is_claude_code_available

        resolve = _resolve("/usr/bin/claude")
        with (
            patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_ENABLED": "true", "CODEFORGE_CLAUDECODE_PATH": "claude"}),
            patch("codeforge.claude_code_availability.resolve_cli", resolve),
        ):
            assert await is_claude_code_available() is True
        resolve.assert_awaited_once_with("claude")

    @pytest.mark.parametrize(
        "error",
        [
            ClaudeCodeCLIError("Claude Code CLI 'claude' not found"),
            ClaudeCodeCLIError("Claude Code CLI '/usr/bin/claude' is not supported: it lacks --tools"),
        ],
    )
    async def test_missing_or_unsupported_cli_is_not_available(self, error: Exception) -> None:
        from codeforge.claude_code_availability import is_claude_code_available

        with (
            patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_ENABLED": "true"}),
            patch("codeforge.claude_code_availability.resolve_cli", _resolve(error)),
        ):
            assert await is_claude_code_available() is False

    async def test_caches_result(self) -> None:
        from codeforge.claude_code_availability import is_claude_code_available

        resolve = _resolve("/usr/bin/claude")
        with (
            patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_ENABLED": "true"}),
            patch("codeforge.claude_code_availability.resolve_cli", resolve),
        ):
            await is_claude_code_available()
            await is_claude_code_available()
        assert resolve.await_count == 1

    async def test_cache_expires(self) -> None:
        import time

        from codeforge import claude_code_availability as mod
        from codeforge.claude_code_availability import is_claude_code_available

        resolve = _resolve("/usr/bin/claude")
        with (
            patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_ENABLED": "true"}),
            patch("codeforge.claude_code_availability.resolve_cli", resolve),
        ):
            await is_claude_code_available()
            mod._claude_code_check_time = time.monotonic() - 400.0
            await is_claude_code_available()
        assert resolve.await_count == 2
