"""URL userinfo (passwords, tokens) never reaches the worker logs."""

from __future__ import annotations

import logging
import time

import pytest

from codeforge.logger import RedactURLFilter, redact_url, redact_urls_processor, setup_logging, stop_logging

REDACT_CASES = [
    ("nats://user:s3cret@nats:4222", "nats://[REDACTED]@nats:4222"),
    ("nats://t0ken@nats:4222", "nats://[REDACTED]@nats:4222"),
    (
        "postgresql://codeforge:pw@postgres:5432/codeforge?sslmode=require",
        "postgresql://[REDACTED]@postgres:5432/codeforge?sslmode=require",
    ),
    ("postgres://u:p@ss:w@rd@db:5432/x", "postgres://[REDACTED]@db:5432/x"),
    ("postgres://u:p%2Fw%40d@db/x", "postgres://[REDACTED]@db/x"),
    ("nats://u:ab/cd+ef==@nats:4222", "nats://[REDACTED]@nats:4222"),
    ("postgres://u:p@[::1]:5432/db", "postgres://[REDACTED]@[::1]:5432/db"),
    ("nats://u:p@a:4222,nats://u:p@b:4222", "nats://[REDACTED]@a:4222,nats://[REDACTED]@b:4222"),
    ("dial nats://u:p@a:4222 failed", "dial nats://[REDACTED]@a:4222 failed"),
    ("server nats://u:p@nats", "server nats://[REDACTED]@nats"),
    ("nats://nats:4222", "nats://nats:4222"),
    ('parse "nats://u:ab/cd+ef==@nats:4222": invalid port', 'parse "nats://[REDACTED]@nats:4222": invalid port'),
    ("dsn='postgres://u:p@db/x'", "dsn='postgres://[REDACTED]@db/x'"),
    ("(see nats://u:p@nats:4222)", "(see nats://[REDACTED]@nats:4222)"),
    ("cannot reach nats://u:p@nats:4222.", "cannot reach nats://[REDACTED]@nats:4222."),
    ("a=nats://u:p@nats:4222;b=1", "a=nats://[REDACTED]@nats:4222;b=1"),
    ("[nats://u:p@nats:4222]", "[nats://[REDACTED]@nats:4222]"),
    ("<nats://u:p@nats:4222>", "<nats://[REDACTED]@nats:4222>"),
    ("nats://u:p@nats_server:4222", "nats://[REDACTED]@nats_server:4222"),
    ("nats://[REDACTED]@nats:4222", "nats://[REDACTED]@nats:4222"),
    # Errs on the side of redacting: an "@" in path or query ends the userinfo.
    ("https://host/path@v1", "https://[REDACTED]@v1"),
    ("https://host/x?mail=a@b.c", "https://[REDACTED]@b.c"),
    ("text ://u:p@host", "text ://u:p@host"),
    ("", ""),
    ("just text", "just text"),
]

# Inputs that made a backtracking implementation quadratic.
PERF_INPUTS = {
    "scheme characters": "a" * 100_000,
    "scheme characters with separator": "a" * 100_000 + "://",
    "many separators": "a://" * 25_000,
    "long authority without at": "x://" + "b" * 100_000,
    "many at signs": "x://" + "@" * 100_000,
    "many urls": "n://u:p@h " * 10_000,
}


@pytest.mark.parametrize(("raw", "expected"), REDACT_CASES)
def test_redact_url(raw: str, expected: str) -> None:
    assert redact_url(raw) == expected


@pytest.mark.parametrize("name", sorted(PERF_INPUTS))
def test_redact_url_is_linear(name: str) -> None:
    text = PERF_INPUTS[name]
    start = time.perf_counter()
    redact_url(text)
    elapsed = time.perf_counter() - start
    assert elapsed < 0.05, f"{name}: {elapsed * 1000:.1f} ms for {len(text)} chars"


def test_processor_redacts_every_string_value() -> None:
    event = {
        "event": "connecting to nats://u:p1@nats:4222",
        "url": "postgresql://cf:p2@postgres:5432/cf",
        "attempt": 3,
        "nested": "unchanged",
    }
    out = redact_urls_processor(None, "info", event)
    assert out["event"] == "connecting to nats://[REDACTED]@nats:4222"
    assert out["url"] == "postgresql://[REDACTED]@postgres:5432/cf"
    assert out["attempt"] == 3
    assert out["nested"] == "unchanged"


async def test_consumer_logs_nats_url_without_credentials(monkeypatch: pytest.MonkeyPatch) -> None:
    """The consumer's 'connected to NATS' line must not carry the password."""
    import codeforge.consumer as consumer_mod

    logged: list[dict[str, object]] = []

    class _StopAfterLogError(Exception):
        pass

    class _FakeLogger:
        def info(self, event: str, **kw: object) -> None:
            logged.append({"event": event, **kw})
            if event == "connected to NATS":
                raise _StopAfterLogError

    class _FakeNC:
        def jetstream(self) -> object:
            return object()

    async def fake_connect(_url: str) -> _FakeNC:
        return _FakeNC()

    monkeypatch.setattr(consumer_mod, "logger", _FakeLogger())
    monkeypatch.setattr(consumer_mod.nats, "connect", fake_connect)

    consumer = consumer_mod.TaskConsumer(nats_url="nats://worker:pw-123@nats:4222")
    with pytest.raises(_StopAfterLogError):
        await consumer.start()

    line = next(entry for entry in logged if entry["event"] == "connected to NATS")
    assert line["url"] == "nats://[REDACTED]@nats:4222"


# ---------------------------------------------------------------------------
# stdlib logging (nats, httpx, litellm and plain logging.getLogger modules)
# ---------------------------------------------------------------------------


def _record(msg: str, args: tuple[object, ...] = (), exc: BaseException | None = None) -> logging.LogRecord:
    exc_info = (type(exc), exc, exc.__traceback__) if exc is not None else None
    return logging.LogRecord("nats.aio.client", logging.ERROR, __file__, 1, msg, args, exc_info)


def test_filter_redacts_message_and_args() -> None:
    record = _record("connect to %s failed (%d)", ("nats://u:p-123@nats:4222", 3))
    assert RedactURLFilter().filter(record)
    assert record.getMessage() == "connect to nats://[REDACTED]@nats:4222 failed (3)"


def test_filter_redacts_exception_text() -> None:
    try:
        raise ConnectionError("dial postgresql://cf:pw-456@postgres:5432/cf refused")
    except ConnectionError as exc:
        record = _record("database down", exc=exc)
    RedactURLFilter().filter(record)
    rendered = logging.Formatter().format(record)
    assert "pw-456" not in rendered
    assert "postgresql://[REDACTED]@postgres:5432/cf" in rendered


def test_filter_keeps_records_with_bad_format_args() -> None:
    record = _record("%d items", ("not-a-number",))
    assert RedactURLFilter().filter(record)


@pytest.fixture
def _logging_restored() -> object:
    root = logging.getLogger()
    saved_handlers = root.handlers[:]
    saved_level = root.level
    yield None
    stop_logging()
    root.handlers[:] = saved_handlers
    root.setLevel(saved_level)
    import structlog

    structlog.reset_defaults()


@pytest.mark.usefixtures("_logging_restored")
def test_stdlib_logger_output_is_redacted(capsys: pytest.CaptureFixture[str]) -> None:
    setup_logging(service="test-worker", level="info")
    logging.getLogger("httpx").warning("request to %s failed", "https://tok-789@api.example.com/v1")
    stop_logging()
    out = capsys.readouterr().out
    assert "tok-789" not in out
    assert "https://[REDACTED]@api.example.com/v1" in out
