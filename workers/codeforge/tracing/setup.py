"""TracingManager — OpenTelemetry integration for agent observability."""

from __future__ import annotations

import asyncio
import functools
import os
from dataclasses import dataclass
from typing import Protocol

import structlog
from opentelemetry import trace
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor, ConsoleSpanExporter
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
    insecure: bool = True
    sample_rate: float = 1.0

    @classmethod
    def from_env(cls) -> OTELConfig:
        """Build config from CODEFORGE_OTEL_* environment variables."""
        enabled_str = os.getenv("CODEFORGE_OTEL_ENABLED", "false")
        return cls(
            enabled=enabled_str.lower() in ("true", "1", "yes"),
            endpoint=os.getenv("CODEFORGE_OTEL_ENDPOINT", "localhost:4317"),
            service_name=os.getenv("CODEFORGE_OTEL_SERVICE_NAME", "codeforge-worker"),
            insecure=os.getenv("CODEFORGE_OTEL_INSECURE", "true").lower() in ("true", "1", "yes"),
            sample_rate=float(os.getenv("CODEFORGE_OTEL_SAMPLE_RATE", "1.0")),
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

    Initializes OTEL TracerProvider with OTLP gRPC exporter when enabled,
    or falls back to no-op stubs for zero overhead when disabled.
    """

    def __init__(self) -> None:
        self._tracer: TracerProtocol = _NoOpTracer()
        self._provider: TracerProvider | None = None
        self._initialized = False

    def init(self) -> None:
        """Initialize the tracer based on OTEL config."""
        cfg = OTELConfig.from_env()

        if not cfg.enabled:
            logger.info("otel tracing disabled (CODEFORGE_OTEL_ENABLED != true)")
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

        if cfg.endpoint:
            try:
                from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter

                otlp_exporter = OTLPSpanExporter(
                    endpoint=cfg.endpoint,
                    insecure=cfg.insecure,
                )
                self._provider.add_span_processor(BatchSpanProcessor(otlp_exporter))
            except Exception as exc:
                logger.warning("otlp exporter setup failed, using console", error=str(exc))
                self._provider.add_span_processor(BatchSpanProcessor(ConsoleSpanExporter()))
        else:
            self._provider.add_span_processor(BatchSpanProcessor(ConsoleSpanExporter()))

        trace.set_tracer_provider(self._provider)
        otel_tracer = trace.get_tracer(TRACER_NAME)
        self._tracer = _OTELTracer(otel_tracer)
        self._initialized = True
        logger.info("otel tracing initialized", service=cfg.service_name, endpoint=cfg.endpoint)

    def get_tracer(self) -> TracerProtocol:
        """Return the active tracer instance (or no-op stub)."""
        if not self._initialized:
            self.init()
        return self._tracer

    @property
    def enabled(self) -> bool:
        return self._initialized and not isinstance(self._tracer, _NoOpTracer)

    def shutdown(self) -> None:
        """Gracefully shutdown the TracerProvider."""
        if self._provider is not None:
            self._provider.shutdown()
            logger.info("otel tracer provider shut down")
