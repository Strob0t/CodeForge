"""Structured JSON logging for Python workers.

Log schema aligns with Go Core:
  {time, level, service, msg, request_id, task_id}
"""

from __future__ import annotations

import logging
import queue
import sys
from logging.handlers import QueueHandler, QueueListener

import structlog

_listener: QueueListener | None = None


def setup_logging(service: str = "codeforge-worker", level: str = "info") -> None:
    """Configure structlog with async JSON output matching the Go Core schema.

    Must be called once at application startup before any logging.
    Uses QueueHandler + QueueListener for non-blocking async log output.
    """
    log_level = getattr(logging, level.upper(), logging.INFO)

    # Async logging via QueueHandler + QueueListener
    log_queue: queue.Queue[logging.LogRecord] = queue.Queue(maxsize=10_000)
    stream_handler = logging.StreamHandler(sys.stdout)
    stream_handler.setLevel(log_level)

    global _listener
    _listener = QueueListener(log_queue, stream_handler, respect_handler_level=True)
    _listener.start()

    # Root logger uses QueueHandler
    root = logging.getLogger()
    root.handlers.clear()
    root.addHandler(QueueHandler(log_queue))
    root.setLevel(log_level)

    structlog.configure(
        processors=[
            structlog.contextvars.merge_contextvars,
            structlog.stdlib.filter_by_level,
            structlog.stdlib.add_logger_name,
            structlog.stdlib.add_log_level,
            structlog.processors.TimeStamper(fmt="iso"),
            structlog.processors.StackInfoRenderer(),
            structlog.processors.format_exc_info,
            structlog.processors.UnicodeDecoder(),
            _add_service(service),
            structlog.processors.JSONRenderer(),
        ],
        wrapper_class=structlog.stdlib.BoundLogger,
        context_class=dict,
        logger_factory=structlog.stdlib.LoggerFactory(),
        cache_logger_on_first_use=True,
    )


def stop_logging() -> None:
    """Flush and stop the async log listener."""
    global _listener
    if _listener is not None:
        _listener.stop()
        _listener = None


def _add_service(service: str) -> structlog.types.Processor:
    """Return a processor that adds the service name to every log entry."""

    def processor(
        _logger: structlog.types.WrappedLogger,
        _method_name: str,
        event_dict: structlog.types.EventDict,
    ) -> structlog.types.EventDict:
        event_dict["service"] = service
        return event_dict

    return processor
