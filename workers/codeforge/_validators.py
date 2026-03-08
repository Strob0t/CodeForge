"""Shared Pydantic validators for Go nil-coercion and common patterns."""

from __future__ import annotations


def coerce_none_to_list[T](v: list[T] | None) -> list[T]:
    """Go marshals nil slices as JSON null; coerce to empty list."""
    return v if v is not None else []
