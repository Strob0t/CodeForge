"""JSON helpers for safe parsing across the worker codebase."""

from __future__ import annotations

import json


def safe_json_loads[T](raw: str | bytes, default: T) -> dict | list | T:
    """Parse JSON, returning *default* on any decode error."""
    try:
        return json.loads(raw)
    except (json.JSONDecodeError, TypeError, ValueError):
        return default
