"""Claude Code CLI availability detection with caching."""

from __future__ import annotations

import asyncio
import time

from codeforge.config import get_settings

_cache_lock = asyncio.Lock()
_claude_code_available: bool | None = None
_claude_code_check_time: float = 0.0
_CACHE_TTL = 300.0


async def is_claude_code_available() -> bool:
    """Check if Claude Code CLI is installed and the feature is enabled.

    Cached for 5 minutes behind asyncio.Lock.
    Returns False immediately if CODEFORGE_CLAUDECODE_ENABLED != 'true'.
    """
    global _claude_code_available, _claude_code_check_time

    settings = get_settings()
    if not settings.claudecode_enabled:
        return False

    async with _cache_lock:
        now = time.monotonic()
        if _claude_code_available is not None and (now - _claude_code_check_time) < _CACHE_TTL:
            return _claude_code_available

        cli_path = settings.claudecode_path
        try:
            proc = await asyncio.create_subprocess_exec(
                cli_path,
                "--version",
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
            )
            await asyncio.wait_for(proc.wait(), timeout=5.0)
            _claude_code_available = proc.returncode == 0
        except (OSError, TimeoutError):
            _claude_code_available = False

        _claude_code_check_time = now
        return _claude_code_available
