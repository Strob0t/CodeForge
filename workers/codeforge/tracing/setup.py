"""TracingManager — AgentNeo integration for dev-mode agent observability."""

from __future__ import annotations

import os
from typing import Any

import structlog

logger = structlog.get_logger()

# Sentinel for no-op mode when tracing is disabled
_NOOP = object()


class _NoOpTracer:
    """Stub tracer that does nothing when tracing is disabled."""

    def trace_agent(self, name: str) -> Any:
        """Return a no-op decorator."""

        def decorator(fn: Any) -> Any:
            return fn

        return decorator

    def trace_tool(self, name: str) -> Any:
        """Return a no-op decorator."""

        def decorator(fn: Any) -> Any:
            return fn

        return decorator

    def instrument_litellm(self) -> None:
        pass

    def start_session(self, run_id: str) -> None:
        pass

    def end_session(self, run_id: str) -> None:
        pass


class TracingManager:
    """Manages AgentNeo tracing lifecycle.

    When ``APP_ENV=development``, initializes AgentNeo tracing with
    auto-instrumentation for LiteLLM calls. Otherwise returns no-op
    stubs so tracing decorators have zero overhead in production.
    """

    def __init__(self, project_name: str = "codeforge") -> None:
        self._project_name = project_name
        self._enabled = os.getenv("APP_ENV") == "development"
        self._tracer: Any = None
        self._initialized = False

    def init(self) -> None:
        """Initialize the tracer if dev mode is active."""
        if not self._enabled:
            logger.info("tracing disabled (APP_ENV != development)")
            self._tracer = _NoOpTracer()
            self._initialized = True
            return

        try:
            from agentneo import Tracer

            self._tracer = Tracer(project_name=self._project_name)
            self._tracer.instrument_litellm()
            self._initialized = True
            logger.info("agentneo tracing initialized", project=self._project_name)
        except ImportError:
            logger.warning("agentneo not installed, tracing disabled")
            self._tracer = _NoOpTracer()
            self._initialized = True

    def get_tracer(self) -> Any:
        """Return the active tracer instance (or no-op stub)."""
        if not self._initialized:
            self.init()
        return self._tracer

    @property
    def enabled(self) -> bool:
        return self._enabled and not isinstance(self._tracer, _NoOpTracer)

    def start_session(self, run_id: str) -> None:
        """Start a tracing session for a benchmark or conversation run."""
        tracer = self.get_tracer()
        tracer.start_session(run_id)

    def end_session(self, run_id: str) -> None:
        """End a tracing session."""
        tracer = self.get_tracer()
        tracer.end_session(run_id)
