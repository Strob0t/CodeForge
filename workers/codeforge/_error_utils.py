"""Shared error handling utilities for non-critical operations."""

from __future__ import annotations

import contextlib
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    import logging
    from collections.abc import Generator


@contextlib.contextmanager
def best_effort(
    logger: logging.Logger,
    operation: str,
    *,
    level: str = "warning",
) -> Generator[None, None, None]:
    """Suppress and log any exception from a non-critical operation.

    Only use when failure is truly acceptable (telemetry, caching, prefetch).
    For recoverable errors, catch specific exception types instead.

    Example::

        with best_effort(logger, "experience cache store"):
            await pool.store(key, value)
    """
    try:
        yield
    except Exception as exc:
        log_fn = getattr(logger, level)
        log_fn("%s failed (non-fatal): %s", operation, exc)
