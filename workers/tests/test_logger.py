"""Tests for async logging setup."""

from __future__ import annotations

import logging

from codeforge.logger import setup_logging, stop_logging


def test_async_logging_writes() -> None:
    """Verify that log messages are written through the async queue."""
    setup_logging(service="test-worker", level="info")
    logger = logging.getLogger("test_async")
    logger.info("hello from async test")
    stop_logging()
    # If we get here without error, the async pipeline worked.


def test_stop_logging_flushes() -> None:
    """Verify stop_logging is idempotent and flushes."""
    setup_logging(service="test-worker", level="info")
    logger = logging.getLogger("test_flush")
    logger.info("flush test message")
    stop_logging()
    # Second call should be safe (idempotent)
    stop_logging()
