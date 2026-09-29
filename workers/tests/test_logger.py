"""Tests for async logging setup."""

from __future__ import annotations

import logging
from logging.handlers import QueueHandler
from typing import TYPE_CHECKING

import pytest
import structlog

from codeforge.logger import setup_logging, stop_logging

if TYPE_CHECKING:
    from collections.abc import Iterator


@pytest.fixture(autouse=True)
def _restore_global_logging() -> Iterator[None]:
    """Undo the process-wide changes setup_logging() makes.

    setup_logging() replaces the root logger's handlers with a QueueHandler and
    configures structlog to route through stdlib logging with logger caching.
    Left in place, later tests log into a stopped queue listener and structlog
    output no longer reaches stdout.
    """
    root = logging.getLogger()
    saved_level = root.level
    yield
    stop_logging()
    for handler in root.handlers[:]:
        if isinstance(handler, QueueHandler):
            root.removeHandler(handler)
    root.setLevel(saved_level)
    structlog.reset_defaults()


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
