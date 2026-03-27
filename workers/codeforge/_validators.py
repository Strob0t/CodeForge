"""Shared Pydantic validators for Go nil-coercion and common patterns."""

from __future__ import annotations


def coerce_none_to_list[T](v: list[T] | None) -> list[T]:
    """Go marshals nil slices as JSON null; coerce to empty list."""
    return v if v is not None else []


def clamp_top_k(v: int, *, min_val: int = 1, max_val: int = 500) -> int:
    """Clamp top_k to a valid range for retrieval queries."""
    return max(min_val, min(v, max_val))
