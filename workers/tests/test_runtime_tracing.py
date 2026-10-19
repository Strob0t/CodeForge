"""The worker's NATS publishes carry the W3C trace context (KI-36).

The Go Core extracts ``traceparent`` from every message it consumes, so the
completions, results and events the worker publishes continue the trace of the
message the worker is handling.
"""

from __future__ import annotations

import json
from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock

import pytest
from opentelemetry import context
from opentelemetry.sdk.trace import TracerProvider

from codeforge.models import TerminationConfig
from codeforge.nats_publish import publish_with_retry
from codeforge.runtime import RuntimeClient
from codeforge.tracing.propagation import TracingJetStreamContext, extract_trace_context

_PUB_ACK = SimpleNamespace(data=json.dumps({"stream": "CODEFORGE", "seq": 1}).encode())
_TRACE_ID = 0x0AF7651916CD43DD8448EB211C80319C
_SPAN_ID = 0xB7AD6B7169203331


def _client() -> MagicMock:
    """A NATS client whose requests (JetStream publishes) are answered with a PubAck."""
    nc = MagicMock()
    nc.request = AsyncMock(return_value=_PUB_ACK)
    return nc


def _published_headers(nc: MagicMock) -> list[dict[str, str] | None]:
    return [call.kwargs.get("headers") for call in nc.request.await_args_list]


def _traceparent(trace_id: int, span_id: int) -> str:
    return f"00-{trace_id:032x}-{span_id:016x}-01"


class TestRuntimeTraceInjection:
    async def test_runtime_publish_injects_traceparent(self) -> None:
        """RuntimeClient publishes through the worker's JetStream context and carries the active span."""
        tracer = TracerProvider().get_tracer("test")
        nc = _client()
        client = RuntimeClient(
            js=TracingJetStreamContext(nc),
            run_id="run-1",
            task_id="task-1",
            project_id="proj-1",
            termination=TerminationConfig(),
        )

        with tracer.start_as_current_span("parent") as span:
            await client.send_output("hello")
            await client.complete_run(status="completed")

        span_context = span.get_span_context()
        expected = _traceparent(span_context.trace_id, span_context.span_id)
        headers = _published_headers(nc)
        assert len(headers) == 3, "runs.output, agents.output and runs.complete"
        assert all(h is not None and h["traceparent"] == expected for h in headers)


class TestTracingJetStreamContext:
    async def test_continues_the_trace_of_the_handled_message(self) -> None:
        """Without a span of its own the worker passes on the trace context it extracted from the message."""
        nc = _client()
        _, token = extract_trace_context({"traceparent": _traceparent(_TRACE_ID, _SPAN_ID)})
        try:
            await TracingJetStreamContext(nc).publish("runs.complete", b"{}")
        finally:
            context.detach(token)

        [headers] = _published_headers(nc)
        assert headers is not None
        assert headers["traceparent"] == _traceparent(_TRACE_ID, _SPAN_ID)

    async def test_keeps_the_callers_headers(self) -> None:
        """publish_with_retry's Nats-Msg-Id (JetStream dedup) must survive the injection."""
        tracer = TracerProvider().get_tracer("test")
        nc = _client()
        with tracer.start_as_current_span("parent"):
            await publish_with_retry(
                TracingJetStreamContext(nc), "runs.complete", b"{}", headers={"Nats-Msg-Id": "id-1"}
            )

        [headers] = _published_headers(nc)
        assert headers is not None
        assert headers["Nats-Msg-Id"] == "id-1"
        assert "traceparent" in headers

    async def test_does_not_mutate_the_callers_headers(self) -> None:
        tracer = TracerProvider().get_tracer("test")
        nc = _client()
        caller_headers = {"X-Custom": "value"}
        with tracer.start_as_current_span("parent"):
            await TracingJetStreamContext(nc).publish("runs.output", b"{}", headers=caller_headers)
        assert caller_headers == {"X-Custom": "value"}

    @pytest.mark.parametrize("headers", [None, {}])
    async def test_without_trace_context_the_message_is_unchanged(self, headers: dict[str, str] | None) -> None:
        """No active trace: no header block is added to the message."""
        nc = _client()
        await TracingJetStreamContext(nc).publish("runs.output", b"{}", headers=headers)
        assert _published_headers(nc) == [None]
