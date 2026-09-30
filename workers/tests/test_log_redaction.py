"""URL userinfo (passwords, tokens) never reaches the worker logs."""

from __future__ import annotations

import pytest

from codeforge.logger import redact_url, redact_urls_processor

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
    ("https://host/x?mail=a@b.c", "https://host/x?mail=a@b.c"),
    ("", ""),
    ("just text", "just text"),
]


@pytest.mark.parametrize(("raw", "expected"), REDACT_CASES)
def test_redact_url(raw: str, expected: str) -> None:
    assert redact_url(raw) == expected


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
