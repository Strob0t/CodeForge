"""Tests for RuntimeClient trace context injection on outgoing NATS publishes."""

from __future__ import annotations

from unittest.mock import AsyncMock

import pytest
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor, SpanExporter, SpanExportResult

from codeforge.models import TerminationConfig
from codeforge.runtime import RuntimeClient


class _InMemoryExporter(SpanExporter):
    def __init__(self) -> None:
        self.spans: list = []

    def export(self, spans: list) -> SpanExportResult:
        self.spans.extend(spans)
        return SpanExportResult.SUCCESS

    def shutdown(self) -> None:
        pass

    def force_flush(self, timeout_millis: int = 0) -> bool:
        return True


class TestRuntimeTraceInjection:
    @pytest.mark.asyncio
    async def test_runtime_publish_injects_traceparent(self) -> None:
        """RuntimeClient._publish should inject traceparent into outgoing NATS headers."""
        provider = TracerProvider()
        exporter = _InMemoryExporter()
        provider.add_span_processor(SimpleSpanProcessor(exporter))
        tracer = provider.get_tracer("test")

        js = AsyncMock()
        client = RuntimeClient(
            js=js,
            run_id="run-1",
            task_id="task-1",
            project_id="proj-1",
            termination=TerminationConfig(),
        )

        with tracer.start_as_current_span("parent"):
            await client._publish("test.subject", b"data")

        call_kwargs = js.publish.call_args
        headers = call_kwargs.kwargs.get("headers") or call_kwargs[1].get("headers")
        assert headers is not None
        assert "traceparent" in headers
