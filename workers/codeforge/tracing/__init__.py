"""Agent tracing and observability — OpenTelemetry integration."""

from codeforge.tracing.setup import TracerProtocol, TracingManager

# Shared singleton — import this instead of creating new TracingManager() instances.
tracing_manager = TracingManager()

__all__ = [
    "TracerProtocol",
    "TracingManager",
    "tracing_manager",
]
