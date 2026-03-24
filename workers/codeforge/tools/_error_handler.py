"""Shared error handling decorator for file tools."""

from __future__ import annotations

import functools
from collections.abc import Awaitable, Callable

from codeforge.tools._base import ToolResult

# FIX-089: Specific callable type for async tool methods.
# Matches the (self, arguments, workspace_path) -> ToolResult signature.
ToolMethod = Callable[[object, dict[str, object], str], Awaitable[ToolResult]]


def catch_os_error(
    func: ToolMethod,
) -> ToolMethod:
    """Decorator: catches OSError and returns a failed ToolResult."""

    @functools.wraps(func)
    async def wrapper(self: object, arguments: dict[str, object], workspace_path: str) -> ToolResult:
        try:
            return await func(self, arguments, workspace_path)
        except OSError as exc:
            return ToolResult(output="", error=str(exc), success=False)

    return wrapper
