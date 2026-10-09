"""Shared Pydantic validators for Go nil-coercion and common patterns."""

from __future__ import annotations

from codeforge.constants import MAX_TOOL_OUTPUT_MAX_CHARS


def coerce_none_to_list[T](v: list[T] | None) -> list[T]:
    """Go marshals nil slices as JSON null; coerce to empty list."""
    return v if v is not None else []


def clamp_top_k(v: int, *, min_val: int = 1, max_val: int = 500) -> int:
    """Clamp top_k to a valid range for retrieval queries."""
    return max(min_val, min(v, max_val))


def clamp_tool_output_max_chars(v: int) -> int:
    """Clamp agent.tool_output_max_chars to 0 (the worker's default) .. MAX_TOOL_OUTPUT_MAX_CHARS."""
    return max(0, min(v, MAX_TOOL_OUTPUT_MAX_CHARS))
