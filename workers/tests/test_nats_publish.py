"""Tests for publishing completions that must not be lost (ADR-016)."""

from __future__ import annotations

import pytest

from codeforge import nats_publish
from codeforge.nats_publish import PUBLISH_ATTEMPTS, publish_with_retry
from tests.jetstream_fakes import RecordingJetStream


@pytest.fixture
def delays(monkeypatch: pytest.MonkeyPatch) -> list[float]:
    """Record the backoff delays instead of sleeping."""
    recorded: list[float] = []

    async def fake_sleep(delay: float) -> None:
        recorded.append(delay)

    monkeypatch.setattr(nats_publish.asyncio, "sleep", fake_sleep)
    return recorded


async def test_publishes_once_when_the_first_attempt_succeeds(delays: list[float]) -> None:
    js = RecordingJetStream()

    await publish_with_retry(js, "runs.complete", b"{}", headers={"Nats-Msg-Id": "id-1"})  # type: ignore[arg-type]

    assert js.published == [("runs.complete", b"{}")]
    assert js.published_headers == [{"Nats-Msg-Id": "id-1"}]
    assert delays == []


async def test_retries_a_transient_failure_with_the_same_headers(delays: list[float]) -> None:
    """The same Nats-Msg-Id on every attempt lets JetStream drop a retry whose first attempt was stored."""
    js = RecordingJetStream(failing_times={"runs.complete": 1})

    await publish_with_retry(js, "runs.complete", b"{}", headers={"Nats-Msg-Id": "id-1"})  # type: ignore[arg-type]

    assert js.attempts == ["runs.complete", "runs.complete"]
    assert js.attempt_headers == [{"Nats-Msg-Id": "id-1"}, {"Nats-Msg-Id": "id-1"}]
    assert js.published == [("runs.complete", b"{}")]
    assert len(delays) == 1


async def test_generates_one_message_id_for_all_attempts(delays: list[float]) -> None:
    js = RecordingJetStream(failing_times={"tasks.result": 1})

    await publish_with_retry(js, "tasks.result", b"{}")  # type: ignore[arg-type]
    await publish_with_retry(js, "tasks.result", b"{}")  # type: ignore[arg-type]

    ids = [(headers or {}).get("Nats-Msg-Id") for headers in js.attempt_headers]
    assert all(ids), "every attempt carries a message ID"
    assert ids[0] == ids[1], "a retry repeats the message ID"
    assert ids[1] != ids[2], "a new publish gets a new message ID"


async def test_keeps_other_headers(delays: list[float]) -> None:
    js = RecordingJetStream()

    await publish_with_retry(js, "tasks.result", b"{}", headers={"X-Request-ID": "req-1"})  # type: ignore[arg-type]

    headers = js.published_headers[0] or {}
    assert headers["X-Request-ID"] == "req-1"
    assert headers["Nats-Msg-Id"]


async def test_backoff_doubles_between_attempts(delays: list[float]) -> None:
    js = RecordingJetStream(failing_times={"tasks.result": PUBLISH_ATTEMPTS - 1})

    await publish_with_retry(js, "tasks.result", b"{}")  # type: ignore[arg-type]

    assert len(js.published) == 1
    assert delays == [nats_publish.PUBLISH_BACKOFF_SECONDS * 2**i for i in range(PUBLISH_ATTEMPTS - 1)]


async def test_raises_the_last_error_after_the_last_attempt(delays: list[float]) -> None:
    js = RecordingJetStream(failing={"tasks.result"})

    with pytest.raises(ConnectionError):
        await publish_with_retry(js, "tasks.result", b"{}")  # type: ignore[arg-type]

    assert js.attempts == ["tasks.result"] * PUBLISH_ATTEMPTS
    assert len(delays) == PUBLISH_ATTEMPTS - 1, "no sleep after the last attempt"
