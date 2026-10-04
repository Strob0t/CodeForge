"""Structured JSON logging for Python workers.

Every line on stdout is one JSON object in the Go core's slog schema:
  {"time": "2026-09-30T12:00:00.123Z", "level": "INFO", "msg": "...", "service": "codeforge-worker",
   "logger": "codeforge.consumer", ...attributes}
structlog loggers and stdlib loggers (httpx, nats, litellm, ...) share one
formatter, so both produce the same schema on the same stream, and URL
userinfo (passwords, tokens) is redacted in every rendered line.
"""

from __future__ import annotations

import logging
import queue
import re
import sys
from datetime import UTC, datetime
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


_render_json = structlog.processors.JSONRenderer()


def _render_redacted_json(
    logger: structlog.types.WrappedLogger,
    method_name: str,
    event_dict: structlog.types.EventDict,
) -> str:
    """Last formatter step: render the line as JSON and redact URL userinfo in all of it.

    Covers every value at any depth (dicts, lists, values rendered with
    repr(), exception text) of structlog and stdlib records alike. JSON
    escaping never cuts through a redacted span: it runs from "://" to an
    "@", neither of which is part of an escape sequence, and ends before the
    closing quote of its string.
    """
    return redact_url(_render_json(logger, method_name, event_dict))


def setup_logging(service: str = "codeforge-worker", level: str = "info") -> None:
    """Route structlog and stdlib logging through one Go-schema JSON formatter on stdout.

    Must be called once at application startup before any logging.
    Records are formatted in the thread that logs (so time and context
    variables are the call's) and written by a QueueListener thread, so
    writing to stdout never blocks the event loop.
    """
    log_level = getattr(logging, level.upper(), logging.INFO)

    formatter = structlog.stdlib.ProcessorFormatter(
        foreign_pre_chain=[structlog.contextvars.merge_contextvars],
        processors=[
            structlog.processors.format_exc_info,
            _go_schema(service),
            _render_redacted_json,
        ],
    )

    # Async logging via QueueHandler + QueueListener
    log_queue: queue.Queue[logging.LogRecord] = queue.Queue(maxsize=10_000)
    stream_handler = logging.StreamHandler(sys.stdout)
    stream_handler.setLevel(log_level)

    global _listener
    _listener = QueueListener(log_queue, stream_handler, respect_handler_level=True)
    _listener.start()

    # Root logger uses QueueHandler; its prepare() renders the record with
    # the formatter before queueing it, the listener writes the line as is.
    root = logging.getLogger()
    root.handlers.clear()
    queue_handler = QueueHandler(log_queue)
    queue_handler.setFormatter(formatter)
    root.addHandler(queue_handler)
    root.setLevel(log_level)

    structlog.configure(
        processors=[
            structlog.contextvars.merge_contextvars,
            structlog.stdlib.filter_by_level,
            structlog.stdlib.PositionalArgumentsFormatter(),
            structlog.processors.StackInfoRenderer(),
            structlog.processors.UnicodeDecoder(),
            structlog.stdlib.ProcessorFormatter.wrap_for_formatter,
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


def _go_level(levelno: int) -> str:
    """Go slog level name: DEBUG, INFO, WARN or ERROR (CRITICAL is ERROR, slog has no higher level)."""
    if levelno >= logging.ERROR:
        return "ERROR"
    if levelno >= logging.WARNING:
        return "WARN"
    if levelno >= logging.INFO:
        return "INFO"
    return "DEBUG"


def _go_time(created: float) -> str:
    """RFC 3339 in UTC with milliseconds, as Go's slog JSON handler writes it."""
    stamp = datetime.fromtimestamp(created, tz=UTC)
    return f"{stamp:%Y-%m-%dT%H:%M:%S}.{stamp.microsecond // 1000:03d}Z"


# Keys the schema sets itself; an attribute of the same name is dropped.
_SCHEMA_KEYS = frozenset({"time", "level", "msg", "service", "logger", "event", "timestamp"})


def _go_schema(service: str) -> structlog.types.Processor:
    """Return the processor that turns an event dict into the Go slog schema.

    Time, level and logger name come from the log record (``_record``, which
    ProcessorFormatter sets for structlog and stdlib records alike); the other
    entries follow as attributes, without the formatter's ``_`` bookkeeping keys.
    """

    def processor(
        _logger: structlog.types.WrappedLogger,
        _method_name: str,
        event_dict: structlog.types.EventDict,
    ) -> structlog.types.EventDict:
        record: logging.LogRecord = event_dict["_record"]
        attributes = {
            key: value for key, value in event_dict.items() if key not in _SCHEMA_KEYS and not key.startswith("_")
        }
        return {
            "time": _go_time(record.created),
            "level": _go_level(record.levelno),
            "msg": event_dict.get("event", ""),
            "service": service,
            "logger": record.name,
            **attributes,
        }

    return processor
