"""Tests for TracingManager initialization and OTEL/no-op behavior."""

from __future__ import annotations

import os
from unittest.mock import patch

import pytest
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import ReadableSpan, SimpleSpanProcessor, SpanExporter, SpanExportResult

from codeforge.tracing.setup import (
    TRACER_NAME,
    TracingManager,
    _NoOpTracer,
    _OTELTracer,
)


class _InMemoryExporter(SpanExporter):
    """Collects spans in a list for test assertions."""

    def __init__(self) -> None:
        self.spans: list[ReadableSpan] = []

    def export(self, spans: list[ReadableSpan]) -> SpanExportResult:
        self.spans.extend(spans)
        return SpanExportResult.SUCCESS

    def shutdown(self) -> None:
        pass

    def force_flush(self, timeout_millis: int = 0) -> bool:
        return True


class TestNoOpTracer:
    def test_disabled_when_not_dev(self) -> None:
        with patch.dict(os.environ, {"CODEFORGE_OTEL_ENABLED": "false"}):
            tm = TracingManager()
            tm.init()
            assert not tm.enabled
            assert isinstance(tm.get_tracer(), _NoOpTracer)

    def test_disabled_when_env_missing(self) -> None:
        with patch.dict(os.environ, {}, clear=True):
            tm = TracingManager()
            tm.init()
            assert not tm.enabled

    def test_noop_tracer_decorators(self) -> None:
        noop = _NoOpTracer()

        @noop.trace_agent("test")
        def my_func() -> str:
            return "hello"

        assert my_func() == "hello"

        @noop.trace_tool("test")
        def my_tool() -> int:
            return 42

        assert my_tool() == 42


class TestOTELTracer:
    @pytest.fixture
    def otel_setup(self) -> tuple[_OTELTracer, _InMemoryExporter, TracerProvider]:
        """Create an OTEL tracer with in-memory exporter for testing."""
        exporter = _InMemoryExporter()
        provider = TracerProvider()
        provider.add_span_processor(SimpleSpanProcessor(exporter))
        tracer = provider.get_tracer(TRACER_NAME)
        otel_tracer = _OTELTracer(tracer)
        return otel_tracer, exporter, provider

    def test_creates_agent_span(self, otel_setup: tuple[_OTELTracer, _InMemoryExporter, TracerProvider]) -> None:
        otel_tracer, exporter, provider = otel_setup

        @otel_tracer.trace_agent("test-agent")
        def my_func() -> str:
            return "result"

        result = my_func()
        provider.force_flush()

        assert result == "result"
        spans = exporter.spans
        assert len(spans) == 1
        assert spans[0].name == "agent:test-agent"
        assert spans[0].attributes["agent.name"] == "test-agent"

    def test_creates_tool_span(self, otel_setup: tuple[_OTELTracer, _InMemoryExporter, TracerProvider]) -> None:
        otel_tracer, exporter, provider = otel_setup

        @otel_tracer.trace_tool("read-file")
        def read_file() -> str:
            return "content"

        result = read_file()
        provider.force_flush()

        assert result == "content"
        spans = exporter.spans
        assert len(spans) == 1
        assert spans[0].name == "tool:read-file"
        assert spans[0].attributes["tool.name"] == "read-file"

    @pytest.mark.asyncio
    async def test_async_decorator(self, otel_setup: tuple[_OTELTracer, _InMemoryExporter, TracerProvider]) -> None:
        otel_tracer, exporter, provider = otel_setup

        @otel_tracer.trace_agent("async-agent")
        async def my_async() -> str:
            return "async-result"

        result = await my_async()
        provider.force_flush()

        assert result == "async-result"
        spans = exporter.spans
        assert len(spans) == 1
        assert spans[0].name == "agent:async-agent"

    def test_records_exception(self, otel_setup: tuple[_OTELTracer, _InMemoryExporter, TracerProvider]) -> None:
        otel_tracer, exporter, provider = otel_setup

        @otel_tracer.trace_agent("failing")
        def failing_func() -> None:
            raise ValueError("test error")

        with pytest.raises(ValueError, match="test error"):
            failing_func()

        provider.force_flush()
        spans = exporter.spans
        assert len(spans) == 1
        assert spans[0].status.status_code.name == "ERROR"
        events = spans[0].events
        assert any(e.name == "exception" for e in events)

    @pytest.mark.asyncio
    async def test_async_records_exception(
        self, otel_setup: tuple[_OTELTracer, _InMemoryExporter, TracerProvider]
    ) -> None:
        otel_tracer, exporter, provider = otel_setup

        @otel_tracer.trace_tool("failing-tool")
        async def failing_async() -> None:
            raise RuntimeError("async error")

        with pytest.raises(RuntimeError, match="async error"):
            await failing_async()

        provider.force_flush()
        spans = exporter.spans
        assert len(spans) == 1
        assert spans[0].status.status_code.name == "ERROR"


class TestTracingManagerOTEL:
    def test_enabled_with_otel_config(self) -> None:
        with patch.dict(os.environ, {"CODEFORGE_OTEL_ENABLED": "true"}):
            tm = TracingManager()
            tm.init()
            assert tm.enabled
            assert isinstance(tm.get_tracer(), _OTELTracer)
            tm.shutdown()

    def test_shutdown_is_safe_when_disabled(self) -> None:
        tm = TracingManager()
        tm.init()
        tm.shutdown()  # Should not raise
