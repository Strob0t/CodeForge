"""TracingManager — AgentNeo integration for dev-mode agent observability."""

from __future__ import annotations

import os
from typing import Protocol

import structlog

logger = structlog.get_logger()


class TracerProtocol(Protocol):
    """Minimal interface for tracing backends (AgentNeo or no-op stub)."""

    def trace_agent(self, name: str) -> object: ...

    def trace_tool(self, name: str) -> object: ...

    def instrument_litellm(self) -> None: ...

    def start_session(self, run_id: str) -> None: ...

    def end_session(self, run_id: str) -> None: ...


class _NoOpTracer:
    """Stub tracer that does nothing when tracing is disabled."""

    def trace_agent(self, name: str) -> object:
        """Return a no-op decorator."""

        def decorator(fn: object) -> object:
            return fn

        return decorator

    def trace_tool(self, name: str) -> object:
        """Return a no-op decorator."""

        def decorator(fn: object) -> object:
            return fn

        return decorator

    def instrument_litellm(self) -> None:
        pass

    def start_session(self, run_id: str) -> None:
        pass

    def end_session(self, run_id: str) -> None:
        pass


class _SafeTracer:
    """Wraps an agentneo Tracer so tracing errors never crash the application.

    AgentNeo's trace_agent/trace_tool decorators write to an internal SQLite
    database.  If the session isn't fully initialised (e.g. missing trace_id)
    the decorator raises IntegrityError, killing the decorated function.
    This wrapper catches any tracing error and falls through to the original
    function, logging a warning instead.
    """

    def __init__(self, inner: object) -> None:
        self._inner = inner

    def trace_agent(self, name: str) -> object:
        """Return a safe decorator that silences tracing failures."""
        import asyncio
        import functools

        try:
            real_decorator = self._inner.trace_agent(name)
        except Exception:
            return lambda fn: fn

        def safe_decorator(fn: object) -> object:
            decorated = real_decorator(fn)

            if asyncio.iscoroutinefunction(fn):

                @functools.wraps(fn)
                async def wrapper(*args: object, **kwargs: object) -> object:
                    try:
                        return await decorated(*args, **kwargs)
                    except Exception as exc:
                        if "trace_id" in str(exc) or "IntegrityError" in type(exc).__name__:
                            logger.warning("tracing error suppressed", agent=name, error=str(exc))
                            return await fn(*args, **kwargs)
                        raise
            else:

                @functools.wraps(fn)
                def wrapper(*args: object, **kwargs: object) -> object:
                    try:
                        return decorated(*args, **kwargs)
                    except Exception as exc:
                        if "trace_id" in str(exc) or "IntegrityError" in type(exc).__name__:
                            logger.warning("tracing error suppressed", agent=name, error=str(exc))
                            return fn(*args, **kwargs)
                        raise

            return wrapper

        return safe_decorator

    def trace_tool(self, name: str) -> object:
        """Return a safe decorator that silences tracing failures."""
        import asyncio
        import functools

        try:
            real_decorator = self._inner.trace_tool(name)
        except Exception:
            return lambda fn: fn

        def safe_decorator(fn: object) -> object:
            decorated = real_decorator(fn)

            if asyncio.iscoroutinefunction(fn):

                @functools.wraps(fn)
                async def wrapper(*args: object, **kwargs: object) -> object:
                    try:
                        return await decorated(*args, **kwargs)
                    except Exception as exc:
                        if "trace_id" in str(exc) or "IntegrityError" in type(exc).__name__:
                            logger.warning("tracing error suppressed", tool=name, error=str(exc))
                            return await fn(*args, **kwargs)
                        raise

                return wrapper

            @functools.wraps(fn)
            def wrapper(*args: object, **kwargs: object) -> object:
                try:
                    return decorated(*args, **kwargs)
                except Exception as exc:
                    if "trace_id" in str(exc) or "IntegrityError" in type(exc).__name__:
                        logger.warning("tracing error suppressed", tool=name, error=str(exc))
                        return fn(*args, **kwargs)
                    raise

            return wrapper

        return safe_decorator

    def instrument_litellm(self) -> None:
        try:
            self._inner.instrument_litellm()
        except Exception:
            logger.warning("instrument_litellm failed, skipping")

    def start_session(self, run_id: str) -> None:
        try:
            self._inner.start_session(run_id)
        except Exception:
            logger.warning("start_session failed", run_id=run_id)

    def end_session(self, run_id: str) -> None:
        try:
            self._inner.end_session(run_id)
        except Exception:
            logger.warning("end_session failed", run_id=run_id)


class TracingManager:
    """Manages AgentNeo tracing lifecycle.

    When ``APP_ENV=development``, initializes AgentNeo tracing with
    auto-instrumentation for LiteLLM calls. Otherwise returns no-op
    stubs so tracing decorators have zero overhead in production.
    """

    def __init__(self, project_name: str = "codeforge") -> None:
        self._project_name = project_name
        self._enabled = os.getenv("APP_ENV") == "development"
        self._tracer: TracerProtocol = _NoOpTracer()
        self._initialized = False

    def init(self) -> None:
        """Initialize the tracer if dev mode is active."""
        if not self._enabled:
            logger.info("tracing disabled (APP_ENV != development)")
            self._tracer = _NoOpTracer()
            self._initialized = True
            return

        try:
            from agentneo import AgentNeo, Tracer

            neo = AgentNeo(session_name=self._project_name)
            try:
                neo.create_project(project_name=self._project_name)
            except Exception:
                neo.connect_project(project_name=self._project_name)
            self._tracer = _SafeTracer(Tracer(session=neo))
            self._initialized = True
            logger.info("agentneo tracing initialized", project=self._project_name)
        except ImportError:
            logger.warning("agentneo not installed — falling back to no-op tracer")
            self._tracer = _NoOpTracer()
            self._initialized = True

    def get_tracer(self) -> TracerProtocol:
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
