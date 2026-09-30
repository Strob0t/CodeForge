"""Fakes for JetStream messages used by consumer tests.

``jetstream_msg`` builds a real ``nats.aio.msg.Msg`` whose client records what
the message was settled with, so ack/nak/term/in-progress behave exactly as
in production and tests can assert the settlement and the delivery count.
"""

from __future__ import annotations

import asyncio
import json

from nats.aio.msg import Msg
from nats.js.api import PubAck


class RecordingClient:
    """Stands in for the NATS client that a Msg uses to send its ack replies.

    ``ack_sync`` (a request to the reply subject) is recorded as ``ack(sync)``;
    with ``fail_requests`` set every one times out like an unconfirmed double
    ack, with ``failing_requests`` = n only the next n do.
    """

    def __init__(self) -> None:
        self.replies: list[bytes] = []
        self.fail_requests = False
        self.failing_requests = 0
        self.request_count = 0

    async def publish(
        self,
        subject: str,
        payload: bytes = b"",
        reply: str = "",
        headers: dict[str, str] | None = None,
    ) -> None:
        self.replies.append(payload)

    async def request(
        self,
        subject: str,
        payload: bytes = b"",
        timeout: float = 0.5,
        old_style: bool = False,
        headers: dict[str, str] | None = None,
    ) -> Msg:
        self.request_count += 1
        if self.fail_requests:
            raise TimeoutError
        if self.failing_requests > 0:
            self.failing_requests -= 1
            raise TimeoutError
        self.replies.append(b"ack(sync)")
        return Msg(_client=self, subject=subject, data=b"")  # type: ignore[arg-type]

    def settlements(self) -> list[str]:
        """Return the replies as names: ack, ack(sync), nak, nak(<delay s>), term, progress."""
        names: list[str] = []
        for payload in self.replies:
            if payload in (b"", Msg.Ack.Ack):
                names.append("ack")
            elif payload == Msg.Ack.Term:
                names.append("term")
            elif payload == Msg.Ack.Progress:
                names.append("progress")
            elif payload == Msg.Ack.Nak:
                names.append("nak")
            elif payload.startswith(Msg.Ack.Nak + b" "):
                delay_ns = json.loads(payload[len(Msg.Ack.Nak) + 1 :])["delay"]
                names.append(f"nak({delay_ns / 1e9:g}s)")
            else:
                names.append(payload.decode())
        return names


def jetstream_msg(
    data: bytes,
    subject: str = "test.request",
    num_delivered: int = 1,
    headers: dict[str, str] | None = None,
) -> tuple[Msg, RecordingClient]:
    """Build a JetStream message delivered for the *num_delivered*-th time."""
    client = RecordingClient()
    # $JS.ACK.<stream>.<consumer>.<delivered>.<stream seq>.<consumer seq>.<timestamp ns>.<pending>
    reply = f"$JS.ACK.CODEFORGE.codeforge-py-test.{num_delivered}.10.5.1727690400000000000.0"
    msg = Msg(_client=client, subject=subject, reply=reply, data=data, headers=headers)  # type: ignore[arg-type]
    return msg, client


class FakeSubscription:
    """A push subscription that yields delivered messages and records its consumer config and unsubscribe."""

    def __init__(self, subject: str, config: object = None) -> None:
        self.subject = subject
        self.config = config
        self.unsubscribed = False
        self._incoming: asyncio.Queue[Msg] = asyncio.Queue()

    def deliver(self, data: bytes) -> None:
        """Queue a core NATS message with *data* for the subscriber."""
        self._incoming.put_nowait(Msg(_client=None, subject=self.subject, data=data))  # type: ignore[arg-type]

    async def next_msg(self, timeout: float = 1.0) -> Msg:
        try:
            return await asyncio.wait_for(self._incoming.get(), timeout=min(timeout, 0.01))
        except TimeoutError:
            raise TimeoutError from None

    async def unsubscribe(self) -> None:
        self.unsubscribed = True


class RecordingJetStream:
    """Records publishes and subscriptions.

    Publishing to a subject in ``failing`` raises; so do the next n publishes
    to a subject that ``failing_times`` maps to n (a transient failure).
    Publishing to a subject in ``duplicates`` returns a PubAck with
    ``duplicate=True`` (the stream kept nothing), as JetStream does for a
    repeated Nats-Msg-Id. ``attempts`` lists every publish attempt's subject.
    """

    def __init__(
        self,
        failing: set[str] | None = None,
        duplicates: set[str] | None = None,
        failing_times: dict[str, int] | None = None,
    ) -> None:
        self.published: list[tuple[str, bytes]] = []
        self.published_headers: list[dict[str, str] | None] = []
        self.attempts: list[str] = []
        self.attempt_headers: list[dict[str, str] | None] = []
        self.subscriptions: list[FakeSubscription] = []
        self.failing = failing or set()
        self.duplicates = duplicates or set()
        self.failing_times = dict(failing_times or {})

    async def subscribe(self, subject: str, config: object = None) -> FakeSubscription:
        sub = FakeSubscription(subject, config)
        self.subscriptions.append(sub)
        return sub

    async def publish(
        self,
        subject: str,
        payload: bytes = b"",
        timeout: float | None = None,
        stream: str | None = None,
        headers: dict[str, str] | None = None,
    ) -> PubAck:
        self.attempts.append(subject)
        self.attempt_headers.append(headers)
        if subject in self.failing:
            msg = f"publish to {subject} failed"
            raise ConnectionError(msg)
        if self.failing_times.get(subject, 0) > 0:
            self.failing_times[subject] -= 1
            msg = f"publish to {subject} failed (transient)"
            raise ConnectionError(msg)
        if subject in self.duplicates:
            return PubAck(stream="CODEFORGE", seq=len(self.published), duplicate=True)
        self.published.append((subject, payload))
        self.published_headers.append(headers)
        return PubAck(stream="CODEFORGE", seq=len(self.published))

    def subjects(self) -> list[str]:
        return [subject for subject, _ in self.published]
