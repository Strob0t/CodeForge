"""Structured JSON logging for Python workers.

Log schema aligns with Go Core:
  {time, level, service, msg, request_id, task_id}
"""

from __future__ import annotations

import logging
import queue
import re
import sys
from logging.handlers import QueueHandler, QueueListener

import structlog

_listener: QueueListener | None = None

_SCHEME_CHARS = frozenset("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+-.")
# Where the text that can hold "userinfo@host" ends.
_AUTHORITY_END = re.compile(r"""[\s"'`()\[\]{}<>]|://""")


def redact_url(text: str) -> str:
    """Replace the userinfo (user, password or token) of every URL in *text*.

    After each "scheme://" the userinfo runs to the last "@" before the next
    whitespace, quote, bracket or "://", so passwords containing "@", ":" or an
    unencoded "/", URLs inside quotes or brackets and server lists are covered,
    whatever follows the host. An "@" later in a path or query is treated as
    the end of the userinfo (errs on the side of redacting). Linear time.
    Mirrors secrets.RedactURL in the Go core.
    """
    parts: list[str] = []
    copied = 0
    pos = 0
    while (sep := text.find("://", pos)) >= 0:
        start = sep + 3
        pos = start
        if sep == 0 or text[sep - 1] not in _SCHEME_CHARS:
            continue
        match = _AUTHORITY_END.search(text, start)
        end = match.start() if match else len(text)
        pos = end
        at = text.rfind("@", start, end)
        if at < 0:
            continue
        parts.append(text[copied:start])
        parts.append("[REDACTED]")
        copied = at
    if not parts:
        return text
    parts.append(text[copied:])
    return "".join(parts)


class RedactURLFilter(logging.Filter):
    """Redact URL userinfo in stdlib log records (nats, httpx, litellm, ...).

    Records from ``logging.getLogger`` loggers never pass the structlog
    processors, so the root handler renders and redacts their message and
    exception text here.
    """

    def filter(self, record: logging.LogRecord) -> bool:
        try:
            message = record.getMessage()
        except (TypeError, ValueError):
            return True  # leave a malformed record to logging's own error handling
        record.msg = redact_url(message)
        record.args = None
        if record.exc_info and not record.exc_text:
            record.exc_text = logging.Formatter().formatException(record.exc_info)
        if record.exc_text:
            record.exc_text = redact_url(record.exc_text)
        return True


def redact_urls_processor(
    _logger: structlog.types.WrappedLogger,
    _method_name: str,
    event_dict: structlog.types.EventDict,
) -> structlog.types.EventDict:
    """structlog processor: redact URL userinfo in every string value."""
    for key, value in event_dict.items():
        if isinstance(value, str):
            event_dict[key] = redact_url(value)
    return event_dict


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
    queue_handler = QueueHandler(log_queue)
    queue_handler.addFilter(RedactURLFilter())
    root.addHandler(queue_handler)
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
            redact_urls_processor,
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
