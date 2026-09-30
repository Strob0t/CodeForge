"""Tests for async logging setup and the Go slog log schema (KI-35)."""

from __future__ import annotations

import calendar
import json
import logging
import os
import re
import subprocess
import sys
import threading
import time
from logging.handlers import QueueHandler
from pathlib import Path
from typing import TYPE_CHECKING

import pytest
import structlog

from codeforge.logger import setup_logging, stop_logging

if TYPE_CHECKING:
    from collections.abc import Iterator

# Go's slog JSON handler: RFC 3339 with exactly three fractional digits.
GO_TIME = re.compile(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$")
WORKERS_DIR = Path(__file__).resolve().parents[1]


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
    structlog.contextvars.clear_contextvars()


def _lines(capsys: pytest.CaptureFixture[str]) -> list[dict[str, object]]:
    """Stop the listener (flushes the queue) and parse every stdout line as one JSON object."""
    stop_logging()
    captured = capsys.readouterr()
    assert captured.err == "", "all log output goes to stdout"
    lines = captured.out.splitlines()
    assert lines, "expected log output"
    return [json.loads(line) for line in lines]


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


# ---------------------------------------------------------------------------
# Go slog schema (KI-35)
# ---------------------------------------------------------------------------


def test_structlog_line_uses_the_go_schema(capsys: pytest.CaptureFixture[str]) -> None:
    setup_logging(service="test-worker", level="info")
    structlog.get_logger("codeforge.test").info("run started", run_id="r1", attempt=2)

    [line] = _lines(capsys)
    assert list(line)[:4] == ["time", "level", "msg", "service"], "Go slog order: time, level, msg, then attrs"
    assert GO_TIME.match(str(line["time"])), line["time"]
    assert line["level"] == "INFO"
    assert line["msg"] == "run started"
    assert line["service"] == "test-worker"
    assert line["logger"] == "codeforge.test"
    assert line["run_id"] == "r1"
    assert line["attempt"] == 2
    assert "event" not in line
    assert "timestamp" not in line


def test_stdlib_line_uses_the_go_schema(capsys: pytest.CaptureFixture[str]) -> None:
    """Libraries (httpx, nats, litellm) log through stdlib: same schema, same stream."""
    setup_logging(service="test-worker", level="info")
    logging.getLogger("httpx").info('HTTP Request: %s "%s"', "POST", "HTTP/1.1 200 OK")

    [line] = _lines(capsys)
    assert list(line)[:4] == ["time", "level", "msg", "service"]
    assert GO_TIME.match(str(line["time"])), line["time"]
    assert line["level"] == "INFO"
    assert line["msg"] == 'HTTP Request: POST "HTTP/1.1 200 OK"'
    assert line["service"] == "test-worker"
    assert line["logger"] == "httpx"


LEVELS = [
    ("debug", "DEBUG"),
    ("info", "INFO"),
    ("warning", "WARN"),
    ("error", "ERROR"),
    ("critical", "ERROR"),
]


@pytest.mark.parametrize(("method", "expected"), LEVELS)
def test_levels_are_go_level_names(capsys: pytest.CaptureFixture[str], method: str, expected: str) -> None:
    setup_logging(service="test-worker", level="debug")
    getattr(structlog.get_logger("s"), method)("from structlog")
    getattr(logging.getLogger("l"), method)("from stdlib")

    assert [(line["msg"], line["level"]) for line in _lines(capsys)] == [
        ("from structlog", expected),
        ("from stdlib", expected),
    ]


def test_level_filter_applies_to_both(capsys: pytest.CaptureFixture[str]) -> None:
    setup_logging(service="test-worker", level="warning")
    structlog.get_logger("s").info("hidden")
    logging.getLogger("l").info("hidden")
    structlog.get_logger("s").warning("shown")
    logging.getLogger("l").warning("shown too")

    assert [line["msg"] for line in _lines(capsys)] == ["shown", "shown too"]


@pytest.mark.parametrize("source", ["structlog", "stdlib"])
def test_exception_stays_on_one_line(capsys: pytest.CaptureFixture[str], source: str) -> None:
    """A traceback is a field of the JSON object, never extra lines on the stream."""
    setup_logging(service="test-worker", level="info")
    try:
        raise ValueError("line one\nline two")
    except ValueError:
        if source == "structlog":
            structlog.get_logger("s").exception("handler failed")
        else:
            logging.getLogger("l").exception("handler failed")

    [line] = _lines(capsys)
    assert line["level"] == "ERROR"
    assert line["msg"] == "handler failed"
    assert "Traceback (most recent call last)" in str(line["exception"])
    assert "ValueError: line one\nline two" in str(line["exception"])


def test_multiline_message_stays_on_one_line(capsys: pytest.CaptureFixture[str]) -> None:
    setup_logging(service="test-worker", level="info")
    logging.getLogger("l").info("first\nsecond")
    structlog.get_logger("s").info("third\nfourth")

    assert [line["msg"] for line in _lines(capsys)] == ["first\nsecond", "third\nfourth"]


def test_structlog_positional_arguments_are_formatted(capsys: pytest.CaptureFixture[str]) -> None:
    setup_logging(service="test-worker", level="info")
    structlog.get_logger("s").info("loaded %d skills from %s", 3, "db")

    [line] = _lines(capsys)
    assert line["msg"] == "loaded 3 skills from db"
    assert "positional_args" not in line


def test_context_variables_reach_structlog_and_stdlib_lines(capsys: pytest.CaptureFixture[str]) -> None:
    setup_logging(service="test-worker", level="info")
    structlog.contextvars.bind_contextvars(request_id="req-1")
    structlog.get_logger("s").info("from structlog")
    logging.getLogger("httpx").info("from stdlib")

    assert [line.get("request_id") for line in _lines(capsys)] == ["req-1", "req-1"]


def test_non_json_values_are_rendered(capsys: pytest.CaptureFixture[str]) -> None:
    setup_logging(service="test-worker", level="info")
    structlog.get_logger("s").info("odd values", data=b"bytes", items={"a"}, text="\udcff")

    [line] = _lines(capsys)
    assert line["msg"] == "odd values"
    assert line["data"] == "bytes"
    assert "a" in str(line["items"])


@pytest.mark.parametrize(
    ("created", "expected"),
    [
        (1_700_000_000.123456, "2023-11-14T22:13:20.123Z"),
        (1_700_000_000.0, "2023-11-14T22:13:20.000Z"),
        (1_700_000_000.9999, "2023-11-14T22:13:20.999Z"),
    ],
)
def test_the_time_is_the_records_creation_time_in_go_format(
    capsys: pytest.CaptureFixture[str], created: float, expected: str
) -> None:
    """The listener thread writes later; the time is the logging call's, truncated to milliseconds like slog."""
    setup_logging(service="test-worker", level="info")
    record = logging.LogRecord("l", logging.INFO, __file__, 1, "at a fixed time", None, None)
    record.created = created
    logging.getLogger().handle(record)

    [line] = _lines(capsys)
    assert line["time"] == expected


def test_the_time_is_current(capsys: pytest.CaptureFixture[str]) -> None:
    setup_logging(service="test-worker", level="info")
    before = time.time()
    logging.getLogger("l").info("now")
    [line] = _lines(capsys)
    stamp = calendar.timegm(time.strptime(str(line["time"])[:19], "%Y-%m-%dT%H:%M:%S"))
    assert int(before) <= stamp <= time.time()


def test_importing_the_worker_writes_nothing(tmp_path: Path) -> None:
    """Modules log at import (settings, tracing) before setup_logging(); nothing may bypass the JSON schema."""
    env = {k: v for k, v in os.environ.items() if k not in ("LITELLM_MASTER_KEY", "CODEFORGE_CONFIG_FILE")}
    env["PYTHONPATH"] = str(WORKERS_DIR)
    env["CODEFORGE_OTEL_ENABLED"] = "false"
    result = subprocess.run(  # noqa: S603 - fixed interpreter and arguments
        [sys.executable, "-c", "import codeforge.consumer"],
        cwd=tmp_path,
        env=env,
        capture_output=True,
        text=True,
        timeout=120,
        check=True,
    )
    assert result.stdout == ""
    assert result.stderr == ""


async def test_main_logs_the_startup_state_in_the_go_schema(
    capsys: pytest.CaptureFixture[str], monkeypatch: pytest.MonkeyPatch
) -> None:
    """What import-time code used to print is logged by main() once the formatter is set up."""
    import codeforge.consumer as consumer_module

    class _Consumer:
        failed = False

        def __init__(self, **_kwargs: object) -> None:
            pass

        async def start(self) -> None:
            return None

        async def stop(self) -> None:
            return None

    monkeypatch.setattr(consumer_module, "TaskConsumer", _Consumer)
    monkeypatch.delenv("LITELLM_MASTER_KEY", raising=False)
    monkeypatch.setattr("codeforge.secrets.get_secret", lambda _name: "")
    monkeypatch.setenv("CODEFORGE_OTEL_ENABLED", "false")
    monkeypatch.setenv("CODEFORGE_WORKER_HEALTH_PORT", "0")
    await consumer_module.main()

    lines = _lines(capsys)
    by_msg = {str(line["msg"]): line for line in lines}
    assert by_msg["using the development LiteLLM master key - set LITELLM_MASTER_KEY for production"]["level"] == "WARN"
    assert by_msg["otel tracing and metrics disabled"]["level"] == "INFO"


def test_logging_from_threads_keeps_lines_whole(capsys: pytest.CaptureFixture[str]) -> None:
    setup_logging(service="test-worker", level="info")

    def work(n: int) -> None:
        for i in range(50):
            logging.getLogger("t").info("thread %d line %d", n, i)
            structlog.get_logger("t").info("structlog line", thread=n, i=i)

    threads = [threading.Thread(target=work, args=(n,)) for n in range(4)]
    for t in threads:
        t.start()
    for t in threads:
        t.join()

    assert len(_lines(capsys)) == 4 * 50 * 2
