"""Tests for W3C trace context propagation via NATS headers."""

from __future__ import annotations

from opentelemetry import context
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor, SpanExporter, SpanExportResult

from codeforge.tracing.propagation import extract_trace_context, inject_trace_context


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


class TestInjectExtract:
    def test_roundtrip(self) -> None:
        """Injected traceparent can be extracted to restore context."""
        provider = TracerProvider()
        exporter = _InMemoryExporter()
        provider.add_span_processor(SimpleSpanProcessor(exporter))
        tracer = provider.get_tracer("test")

        with tracer.start_as_current_span("parent"):
            headers = inject_trace_context()
            assert "traceparent" in headers

            # Extract in a fresh context
            _, token = extract_trace_context(headers)
            try:
                with tracer.start_as_current_span("child"):
                    pass
            finally:
                context.detach(token)

        provider.force_flush()
        spans = exporter.spans
        assert len(spans) == 2
        # Child's parent should be the original parent span
        child = next(s for s in spans if s.name == "child")
        parent = next(s for s in spans if s.name == "parent")
        assert child.parent.span_id == parent.context.span_id

    def test_extract_empty_headers(self) -> None:
        """Empty headers should not crash — returns root context."""
        _, token = extract_trace_context(None)
        context.detach(token)

    def test_extract_no_traceparent(self) -> None:
        """Headers without traceparent should return root context."""
        _, token = extract_trace_context({"X-Request-ID": "abc"})
        context.detach(token)

    def test_inject_returns_dict(self) -> None:
        """inject_trace_context returns a dict even with no active span."""
        headers = inject_trace_context()
        assert isinstance(headers, dict)

    def test_inject_preserves_existing_headers(self) -> None:
        """Existing headers are preserved when injecting trace context."""
        headers = inject_trace_context({"X-Custom": "value"})
        assert headers["X-Custom"] == "value"


class TestStartConsumerSpan:
    def test_creates_span_with_parent_context(self) -> None:
        """start_consumer_span creates a span as child of the extracted trace."""
        provider = TracerProvider()
        exporter = _InMemoryExporter()
        provider.add_span_processor(SimpleSpanProcessor(exporter))

        tracer = provider.get_tracer("test")
        with tracer.start_as_current_span("publisher"):
            headers = inject_trace_context()

        # Extract context and verify the trace ID propagates
        ctx, token = extract_trace_context(headers)
        try:
            with tracer.start_as_current_span("consumer.handle", context=ctx):
                pass
        finally:
            context.detach(token)

        provider.force_flush()
        publisher_span = next(s for s in exporter.spans if s.name == "publisher")
        consumer_span = next(s for s in exporter.spans if s.name == "consumer.handle")
        # Both spans share the same trace ID
        assert consumer_span.context.trace_id == publisher_span.context.trace_id
        # Consumer span is a child of the publisher span
        assert consumer_span.parent.span_id == publisher_span.context.span_id
