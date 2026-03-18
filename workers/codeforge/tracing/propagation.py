"""W3C Trace Context propagation helpers for NATS messages."""

from __future__ import annotations

from typing import TYPE_CHECKING

from opentelemetry import context
from opentelemetry.propagate import extract, inject

if TYPE_CHECKING:
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
