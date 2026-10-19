"""Claude Code CLI availability detection with caching."""

from __future__ import annotations

import asyncio
import logging
import time

from codeforge.claude_code_executor import ClaudeCodeCLIError, resolve_cli
from codeforge.config import get_settings

logger = logging.getLogger(__name__)

_cache_lock = asyncio.Lock()
_claude_code_available: bool | None = None
_claude_code_check_time: float = 0.0
_CACHE_TTL = 300.0


async def is_claude_code_available() -> bool:
    """Check if Claude Code is enabled and its CLI passes the executor's capability check.

    A CLI that lacks an option the policy enforcement needs is not offered to
    routing: every run on it would fail. Cached for 5 minutes behind
    asyncio.Lock. Returns False immediately if CODEFORGE_CLAUDECODE_ENABLED != 'true'.
    """
    global _claude_code_available, _claude_code_check_time

    settings = get_settings()
    if not settings.claudecode_enabled:
        return False

    async with _cache_lock:
        now = time.monotonic()
        if _claude_code_available is not None and (now - _claude_code_check_time) < _CACHE_TTL:
            return _claude_code_available

        try:
            await resolve_cli(settings.claudecode_path)
            _claude_code_available = True
        except ClaudeCodeCLIError as exc:
            logger.warning("Claude Code is enabled but not available: %s", exc)
            _claude_code_available = False

        _claude_code_check_time = now
        return _claude_code_available
