"""W3C Trace Context propagation helpers for NATS messages."""

from __future__ import annotations

from typing import TYPE_CHECKING

from nats.js.client import JetStreamContext
from opentelemetry import context
from opentelemetry.propagate import extract, inject

if TYPE_CHECKING:
    from nats.js.api import PubAck
    from opentelemetry.context import Context


def extract_trace_context(headers: dict[str, str] | None) -> tuple[Context, object]:
    """Extract W3C traceparent from NATS message headers.

    Returns (context, token) — caller must call ``context.detach(token)``
    when done processing the message.
    """
    carrier = dict(headers) if headers else {}
    ctx = extract(carrier)
    token = context.attach(ctx)
    return ctx, token


def inject_trace_context(headers: dict[str, str] | None = None) -> dict[str, str]:
    """Inject current trace context into a header dict for outgoing NATS messages.

    Returns the headers dict (creates one if None was passed).
    """
    carrier = dict(headers) if headers else {}
    inject(carrier)
    return carrier


class TracingJetStreamContext(JetStreamContext):
    """JetStream context that adds the current W3C trace context to every published message.

    The worker handles each message inside the trace context extracted from its
    headers, so results and events published while handling it continue the
    Go Core's trace (the Go side extracts ``traceparent`` from every message).
    A publish without an active trace context is sent unchanged.
    """

    async def publish(
        self,
        subject: str,
        payload: bytes = b"",
        timeout: float | None = None,
        stream: str | None = None,
        headers: dict[str, str] | None = None,
        msg_ttl: float | None = None,
    ) -> PubAck:
        return await super().publish(
            subject,
            payload,
            timeout=timeout,
            stream=stream,
            headers=inject_trace_context(headers) or None,
            msg_ttl=msg_ttl,
        )
