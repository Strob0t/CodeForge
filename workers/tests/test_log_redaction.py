"""URL userinfo (passwords, tokens) never reaches the worker logs."""

from __future__ import annotations

import json
import logging
import time

import pytest

from codeforge.logger import redact_url, setup_logging, stop_logging

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

    async def fake_connect(_url: str, **_options: object) -> _FakeNC:
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


@pytest.mark.usefixtures("_logging_restored")
def test_structlog_output_is_redacted(capsys: pytest.CaptureFixture[str]) -> None:
    """Message, attributes and positional arguments of structlog lines (Go-schema formatter, KI-35)."""
    import structlog

    setup_logging(service="test-worker", level="info")
    structlog.get_logger("s").warning(
        "connect to %s failed", "nats://u:pw-1@nats:4222", dsn="postgresql://cf:pw-2@postgres:5432/cf"
    )
    stop_logging()
    line = json.loads(capsys.readouterr().out)
    assert line["msg"] == "connect to nats://[REDACTED]@nats:4222 failed"
    assert line["dsn"] == "postgresql://[REDACTED]@postgres:5432/cf"


@pytest.mark.usefixtures("_logging_restored")
@pytest.mark.parametrize("source", ["structlog", "stdlib"])
def test_exception_text_is_redacted(capsys: pytest.CaptureFixture[str], source: str) -> None:
    import structlog

    setup_logging(service="test-worker", level="info")
    try:
        raise ConnectionError("dial postgresql://cf:pw-456@postgres:5432/cf refused")
    except ConnectionError:
        if source == "structlog":
            structlog.get_logger("s").exception("database down")
        else:
            logging.getLogger("psycopg").exception("database down")
    stop_logging()
    out = capsys.readouterr().out
    assert "pw-456" not in out
    assert "postgresql://[REDACTED]@postgres:5432/cf" in json.loads(out)["exception"]


class _Endpoint:
    """A value JSON cannot serialize; the renderer writes its repr()."""

    def __repr__(self) -> str:
        return "Endpoint(https://svc:pw-repr@api.example.com)"


def _one_line(capsys: pytest.CaptureFixture[str]) -> tuple[str, dict[str, object]]:
    stop_logging()
    out = capsys.readouterr().out
    lines = out.splitlines()
    assert len(lines) == 1, out
    return out, json.loads(lines[0])


@pytest.mark.usefixtures("_logging_restored")
def test_nested_structlog_values_are_redacted(capsys: pytest.CaptureFixture[str]) -> None:
    """Every value of the line, at any depth, as the whole rendered line was redacted before KI-35."""
    import structlog

    setup_logging(service="test-worker", level="info")
    structlog.get_logger("s").info(
        "x",
        extra_url={"u": "https://user:pw-dict@host", "deeper": {"dsn": "postgresql://cf:pw-deep@db/cf"}},
        servers=["nats://u:pw-list@a:4222", "nats://b:4222"],
        pair=("https://t:pw-tuple@host", 1),
        endpoint=_Endpoint(),
        error=ConnectionError("dial nats://u:pw-exc@nats:4222 refused"),
    )
    out, line = _one_line(capsys)
    for secret in ("pw-dict", "pw-deep", "pw-list", "pw-tuple", "pw-repr", "pw-exc", "user:", "svc:"):
        assert secret not in out, f"{secret} leaked: {out}"
    assert line["extra_url"] == {"u": "https://[REDACTED]@host", "deeper": {"dsn": "postgresql://[REDACTED]@db/cf"}}
    assert line["servers"] == ["nats://[REDACTED]@a:4222", "nats://b:4222"]
    assert line["pair"] == ["https://[REDACTED]@host", 1]


@pytest.mark.usefixtures("_logging_restored")
def test_stdlib_record_with_arguments_is_redacted(capsys: pytest.CaptureFixture[str]) -> None:
    setup_logging(service="test-worker", level="info")
    logging.getLogger("nats.aio.client").error("connect to %s failed (%d)", "nats://u:p-123@nats:4222", 3)
    out, line = _one_line(capsys)
    assert "p-123" not in out
    assert line["msg"] == "connect to nats://[REDACTED]@nats:4222 failed (3)"


@pytest.mark.usefixtures("_logging_restored")
def test_stdlib_exception_with_arguments_is_redacted_once(capsys: pytest.CaptureFixture[str]) -> None:
    setup_logging(service="test-worker", level="info")
    try:
        raise ConnectionError("dial postgresql://cf:pw-789@postgres:5432/cf refused")
    except ConnectionError:
        logging.getLogger("psycopg").exception("pool for %s failed", "postgresql://cf:pw-789@postgres:5432/cf")
    out, line = _one_line(capsys)
    assert "pw-789" not in out
    assert line["msg"] == "pool for postgresql://[REDACTED]@postgres:5432/cf failed"
    assert str(line["exception"]).count("Traceback (most recent call last)") == 1


@pytest.mark.usefixtures("_logging_restored")
def test_bad_format_arguments_do_not_break_the_caller(capsys: pytest.CaptureFixture[str]) -> None:
    """A malformed stdlib record is left to logging's own error handling, as before."""
    setup_logging(service="test-worker", level="info")
    logging.getLogger("lib").info("%d items", "not-a-number")  # must not raise
    logging.getLogger("lib").info("next line")
    stop_logging()
    lines = capsys.readouterr().out.splitlines()
    assert json.loads(lines[-1])["msg"] == "next line"


@pytest.mark.usefixtures("_logging_restored")
@pytest.mark.parametrize(
    "value",
    [
        'quoted "https://u:pw-q@host" value',
        "back\\slash https://u:pw\\x@host",
        "line\nbreak https://u:pw-nl@host\nend",
        "unicode \u00e9 https://u:pw-\u00e9@host",
        "tab\thttps://u:pw-tab@host",
    ],
)
def test_redacted_line_stays_valid_json(capsys: pytest.CaptureFixture[str], value: str) -> None:
    import structlog

    setup_logging(service="test-worker", level="info")
    structlog.get_logger("s").info("tricky", value=value)
    out, line = _one_line(capsys)  # parses
    assert "pw" not in out
    assert "[REDACTED]" in str(line["value"])


def test_redacting_rendered_json_never_breaks_it() -> None:
    """Redaction runs on the rendered line: it must keep any value valid JSON (seeded random values)."""
    import random

    from codeforge.logger import _render_redacted_json

    alphabet = [
        '"',
        "\\",
        "\n",
        "\t",
        "@",
        ":",
        "/",
        "://",
        "https://",
        "u:p@",
        chr(0xE9),
        chr(0x2028),
        "x",
        " ",
        "'",
        "]",
    ]
    rng = random.Random(4711)  # noqa: S311 - deterministic test data
    for _ in range(3000):
        value = "".join(rng.choice(alphabet) for _ in range(rng.randint(1, 24)))
        nested = {"v": value, "list": [value, {"k": value}]}
        rendered = _render_redacted_json(None, "info", nested)
        parsed = json.loads(rendered)  # raises if redaction cut an escape sequence
        assert set(parsed) == {"v", "list"}
