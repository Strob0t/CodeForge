"""Tests for TracingManager initialization and OTEL/no-op behavior."""

from __future__ import annotations

import os
from typing import ClassVar
from unittest.mock import patch

import pytest
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.metrics.export import MetricExporter, MetricExportResult, MetricsData
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import ReadableSpan, SimpleSpanProcessor, SpanExporter, SpanExportResult

from codeforge.tracing.setup import (
    TRACER_NAME,
    OTELConfig,
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


class _RecordingMetricExporter(MetricExporter):
    """Stands in for the OTLP metric exporter; records its settings, exports and shutdown."""

    instances: ClassVar[list[_RecordingMetricExporter]] = []

    def __init__(self, **kwargs: object) -> None:
        super().__init__()
        self.kwargs = kwargs
        self.exported: list[MetricsData] = []
        self.shut_down = False
        _RecordingMetricExporter.instances.append(self)

    def export(
        self, metrics_data: MetricsData, timeout_millis: float = 10_000, **_kwargs: object
    ) -> MetricExportResult:
        self.exported.append(metrics_data)
        return MetricExportResult.SUCCESS

    def force_flush(self, timeout_millis: float = 10_000) -> bool:
        return True

    def shutdown(self, timeout_millis: float = 30_000, **_kwargs: object) -> None:
        self.shut_down = True


class _RecordingLogger:
    """Records (level, event, fields) of the calls made to the module logger."""

    def __init__(self) -> None:
        self.entries: list[tuple[str, str, dict[str, object]]] = []

    def info(self, event: str, **fields: object) -> None:
        self.entries.append(("info", event, fields))

    def error(self, event: str, **fields: object) -> None:
        self.entries.append(("error", event, fields))


class TestTracingManagerMetrics:
    """KI-36: with OTEL enabled the worker exports its metrics (codeforge.tracing.metrics) via OTLP."""

    @pytest.fixture
    def installed(self, monkeypatch: pytest.MonkeyPatch) -> list[object]:
        """Replace the OTLP metric exporter and the global setters; return the meter providers init() installs."""
        providers: list[object] = []
        _RecordingMetricExporter.instances = []
        monkeypatch.setattr(
            "opentelemetry.exporter.otlp.proto.grpc.metric_exporter.OTLPMetricExporter", _RecordingMetricExporter
        )
        monkeypatch.setattr("opentelemetry.metrics.set_meter_provider", providers.append)
        monkeypatch.setattr("opentelemetry.trace.set_tracer_provider", lambda _provider: None)
        return providers

    def test_enabled_installs_an_otlp_meter_provider(
        self, monkeypatch: pytest.MonkeyPatch, installed: list[object]
    ) -> None:
        monkeypatch.setenv("CODEFORGE_OTEL_ENABLED", "true")
        monkeypatch.setenv("CODEFORGE_OTEL_ENDPOINT", "collector:4317")
        monkeypatch.setenv("CODEFORGE_OTEL_INSECURE", "true")
        tm = TracingManager()
        tm.init()
        try:
            assert len(installed) == 1
            provider = installed[0]
            assert isinstance(provider, MeterProvider)
            [exporter] = _RecordingMetricExporter.instances
            assert exporter.kwargs == {"endpoint": "collector:4317", "insecure": True}

            provider.get_meter("test").create_counter("codeforge.test.counter").add(3)
            provider.force_flush()
            assert exporter.exported, "a recorded measurement must reach the exporter"
        finally:
            tm.shutdown()
        assert exporter.shut_down, "shutdown() must flush and stop the metric exporter"

    def test_span_exporter_failure_is_reported_not_printed(
        self, monkeypatch: pytest.MonkeyPatch, installed: list[object], capsys: pytest.CaptureFixture[str]
    ) -> None:
        """init() runs before logging is set up: no console exporter, log_status() reports the error."""

        def broken_exporter(**_kwargs: object) -> None:
            raise ValueError("invalid endpoint")

        monkeypatch.setattr("opentelemetry.exporter.otlp.proto.grpc.trace_exporter.OTLPSpanExporter", broken_exporter)
        monkeypatch.setenv("CODEFORGE_OTEL_ENABLED", "true")
        logs = _RecordingLogger()
        monkeypatch.setattr("codeforge.tracing.setup.logger", logs)
        tm = TracingManager()
        tm.init()
        try:
            assert tm.enabled
            assert logs.entries == [], "init() runs before logging is set up and must not log"
            assert capsys.readouterr().out == "", "init() must not print"
            tm.log_status()
        finally:
            tm.shutdown()
        errors = [fields for level, _event, fields in logs.entries if level == "error"]
        assert errors == [{"error": "invalid endpoint"}]

    def test_log_status_when_disabled(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("CODEFORGE_OTEL_ENABLED", "false")
        logs = _RecordingLogger()
        monkeypatch.setattr("codeforge.tracing.setup.logger", logs)
        tm = TracingManager()
        tm.init()
        assert logs.entries == []
        tm.log_status()
        assert [(level, event) for level, event, _fields in logs.entries] == [
            ("info", "otel tracing and metrics disabled")
        ]

    def test_disabled_installs_no_meter_provider(
        self, monkeypatch: pytest.MonkeyPatch, installed: list[object]
    ) -> None:
        monkeypatch.setenv("CODEFORGE_OTEL_ENABLED", "false")
        tm = TracingManager()
        tm.init()
        tm.shutdown()
        assert installed == []
        assert _RecordingMetricExporter.instances == []


class TestOTELConfigDefaults:
    def test_insecure_defaults_to_tls_like_the_go_core(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Go and the worker read the same otel.insecure / CODEFORGE_OTEL_INSECURE: same default (false)."""
        monkeypatch.delenv("CODEFORGE_OTEL_INSECURE", raising=False)
        assert OTELConfig().insecure is False
        assert OTELConfig.from_env().insecure is False

    @pytest.mark.parametrize(("value", "expected"), [("true", True), ("1", True), ("false", False), ("0", False)])
    def test_insecure_from_env(self, monkeypatch: pytest.MonkeyPatch, value: str, expected: bool) -> None:
        monkeypatch.setenv("CODEFORGE_OTEL_INSECURE", value)
        assert OTELConfig.from_env().insecure is expected
