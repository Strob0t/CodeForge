"""TracingManager — OpenTelemetry integration for agent observability."""

from __future__ import annotations

import asyncio
import functools
from dataclasses import dataclass
from typing import Protocol

import structlog
from opentelemetry import metrics, trace
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.metrics.export import PeriodicExportingMetricReader
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.semconv.resource import ResourceAttributes
from opentelemetry.trace import StatusCode

logger = structlog.get_logger()

TRACER_NAME = "codeforge"


@dataclass(frozen=True)
class OTELConfig:
    """OTEL configuration matching Go Core's config fields."""

    enabled: bool = False
    endpoint: str = "localhost:4317"
    service_name: str = "codeforge-worker"
    insecure: bool = False
    sample_rate: float = 1.0

    @classmethod
    def from_env(cls) -> OTELConfig:
        """Build config from centralized WorkerSettings."""
        from codeforge.config import get_settings

        s = get_settings()
        return cls(
            enabled=s.otel_enabled,
            endpoint=s.otel_endpoint,
            service_name=s.otel_service_name,
            insecure=s.otel_insecure,
            sample_rate=s.otel_sample_rate,
        )


class TracerProtocol(Protocol):
    """Minimal interface for tracing backends (OTEL or no-op stub)."""

    def trace_agent(self, name: str) -> object: ...

    def trace_tool(self, name: str) -> object: ...


class _NoOpTracer:
    """Stub tracer that does nothing when tracing is disabled."""

    def trace_agent(self, name: str) -> object:
        def decorator(fn: object) -> object:
            return fn

        return decorator

    def trace_tool(self, name: str) -> object:
        def decorator(fn: object) -> object:
            return fn

        return decorator


class _OTELTracer:
    """Tracer that creates real OpenTelemetry spans."""

    def __init__(self, tracer: trace.Tracer) -> None:
        self._tracer = tracer

    def trace_agent(self, name: str) -> object:
        return self._make_decorator(f"agent:{name}", "agent.name", name)

    def trace_tool(self, name: str) -> object:
        return self._make_decorator(f"tool:{name}", "tool.name", name)

    def _make_decorator(self, span_name: str, attr_key: str, attr_value: str) -> object:
        tracer = self._tracer

        def decorator(fn: object) -> object:
            if asyncio.iscoroutinefunction(fn):

                @functools.wraps(fn)
                async def async_wrapper(*args: object, **kwargs: object) -> object:
                    with tracer.start_as_current_span(span_name, attributes={attr_key: attr_value}) as span:
                        try:
                            return await fn(*args, **kwargs)
                        except Exception as exc:
                            span.set_status(StatusCode.ERROR, str(exc))
                            span.record_exception(exc)
                            raise

                return async_wrapper

            @functools.wraps(fn)
            def sync_wrapper(*args: object, **kwargs: object) -> object:
                with tracer.start_as_current_span(span_name, attributes={attr_key: attr_value}) as span:
                    try:
                        return fn(*args, **kwargs)
                    except Exception as exc:
                        span.set_status(StatusCode.ERROR, str(exc))
                        span.record_exception(exc)
                        raise

            return sync_wrapper

        return decorator


class TracingManager:
    """Manages OpenTelemetry tracing lifecycle.

    Initializes OTEL TracerProvider and MeterProvider with OTLP gRPC exporters when enabled,
    or falls back to no-op stubs for zero overhead when disabled.
    """

    def __init__(self) -> None:
        self._tracer: TracerProtocol = _NoOpTracer()
        self._provider: TracerProvider | None = None
        self._meter_provider: MeterProvider | None = None
        self._initialized = False
        self._config: OTELConfig | None = None
        self._exporter_error = ""

    def init(self) -> None:
        """Initialize tracing and metrics from the OTEL config.

        Modules call get_tracer() at import time to decorate their functions,
        so this runs before the worker sets up logging and must not log: the
        entry point calls log_status() once logging is ready.
        """
        cfg = OTELConfig.from_env()
        self._config = cfg

        if not cfg.enabled:
            self._tracer = _NoOpTracer()
            self._initialized = True
            return

        resource = Resource.create({ResourceAttributes.SERVICE_NAME: cfg.service_name})

        from opentelemetry.sdk.trace import sampling

        if cfg.sample_rate >= 1.0:
            sampler = sampling.ALWAYS_ON
        elif cfg.sample_rate <= 0.0:
            sampler = sampling.ALWAYS_OFF
        else:
            sampler = sampling.TraceIdRatioBased(cfg.sample_rate)

        self._provider = TracerProvider(resource=resource, sampler=sampler)

        try:
            from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter

            otlp_exporter = OTLPSpanExporter(endpoint=cfg.endpoint, insecure=cfg.insecure)
            self._provider.add_span_processor(BatchSpanProcessor(otlp_exporter))
        except Exception as exc:
            # Spans are recorded but not exported; log_status() reports why. (A
            # console exporter would write multi-line JSON between the log lines.)
            self._exporter_error = str(exc)

        trace.set_tracer_provider(self._provider)
        self._meter_provider = _otlp_meter_provider(cfg, resource)
        # The instruments in codeforge.tracing.metrics were created on the
        # global proxy meter at import; they record into this provider from now on.
        metrics.set_meter_provider(self._meter_provider)
        otel_tracer = trace.get_tracer(TRACER_NAME)
        self._tracer = _OTELTracer(otel_tracer)
        self._initialized = True

    def log_status(self) -> None:
        """Log whether and where OTEL data is exported (init() cannot log, see there)."""
        cfg = self._config
        if cfg is None or not cfg.enabled:
            logger.info("otel tracing and metrics disabled", enable_with="CODEFORGE_OTEL_ENABLED=true")
            return
        if self._exporter_error:
            logger.error("otlp span exporter setup failed, spans are not exported", error=self._exporter_error)
        logger.info(
            "otel tracing and metrics enabled",
            service=cfg.service_name,
            endpoint=cfg.endpoint,
            insecure=cfg.insecure,
            sample_rate=cfg.sample_rate,
        )

    def get_tracer(self) -> TracerProtocol:
        """Return the active tracer instance (or no-op stub)."""
        if not self._initialized:
            self.init()
        return self._tracer

    @property
    def enabled(self) -> bool:
        return self._initialized and not isinstance(self._tracer, _NoOpTracer)

    def shutdown(self) -> None:
        """Flush and shut down the MeterProvider and the TracerProvider."""
        if self._meter_provider is not None:
            self._meter_provider.shutdown()
        if self._provider is not None:
            self._provider.shutdown()
            logger.info("otel tracer provider shut down")


def _otlp_meter_provider(cfg: OTELConfig, resource: Resource) -> MeterProvider:
    """A MeterProvider that exports periodically to the OTLP gRPC endpoint (like the Go core)."""
    from opentelemetry.exporter.otlp.proto.grpc.metric_exporter import OTLPMetricExporter

    exporter = OTLPMetricExporter(endpoint=cfg.endpoint, insecure=cfg.insecure)
    return MeterProvider(resource=resource, metric_readers=[PeriodicExportingMetricReader(exporter)])
