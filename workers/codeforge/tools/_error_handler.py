"""Shared error handling decorator for file tools."""

from __future__ import annotations

import functools
from typing import Any

from codeforge.tools._base import ToolResult


def catch_os_error(
    func: Any,
) -> Any:
    """Decorator: catches OSError and returns a failed ToolResult."""

    @functools.wraps(func)
    async def wrapper(self: Any, arguments: dict, workspace_path: str) -> ToolResult:
        try:
            return await func(self, arguments, workspace_path)
        except OSError as exc:
            return ToolResult(output="", error=str(exc), success=False)

    return wrapper
