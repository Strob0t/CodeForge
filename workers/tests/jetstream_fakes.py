"""Fakes for JetStream messages used by consumer tests.

``jetstream_msg`` builds a real ``nats.aio.msg.Msg`` whose client records what
the message was settled with, so ack/nak/term/in-progress behave exactly as
in production and tests can assert the settlement and the delivery count.
"""

from __future__ import annotations

import asyncio
import json

from nats.aio.msg import Msg


class RecordingClient:
    """Stands in for the NATS client that a Msg uses to send its ack replies."""

    def __init__(self) -> None:
        self.replies: list[bytes] = []

    async def publish(
        self,
        subject: str,
        payload: bytes = b"",
        reply: str = "",
        headers: dict[str, str] | None = None,
    ) -> None:
        self.replies.append(payload)

    def settlements(self) -> list[str]:
        """Return the replies as names: ack, nak, nak(<delay s>), term, progress."""
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
    """A push subscription that never receives a message and records unsubscribe."""

    def __init__(self, subject: str) -> None:
        self.subject = subject
        self.unsubscribed = False

    async def next_msg(self, timeout: float = 1.0) -> Msg:
        await asyncio.sleep(min(timeout, 0.01))
        raise TimeoutError

    async def unsubscribe(self) -> None:
        self.unsubscribed = True


class RecordingJetStream:
    """Records publishes and subscriptions; publishing to a subject in ``failing`` raises."""

    def __init__(self, failing: set[str] | None = None) -> None:
        self.published: list[tuple[str, bytes]] = []
        self.subscriptions: list[FakeSubscription] = []
        self.failing = failing or set()

    async def subscribe(self, subject: str, config: object = None) -> FakeSubscription:
        sub = FakeSubscription(subject)
        self.subscriptions.append(sub)
        return sub

    async def publish(
        self,
        subject: str,
        payload: bytes = b"",
        timeout: float | None = None,
        stream: str | None = None,
        headers: dict[str, str] | None = None,
    ) -> None:
        if subject in self.failing:
            msg = f"publish to {subject} failed"
            raise ConnectionError(msg)
        self.published.append((subject, payload))

    def subjects(self) -> list[str]:
        return [subject for subject, _ in self.published]
