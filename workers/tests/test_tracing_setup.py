"""Tests for TracingManager initialization and no-op behavior."""

from __future__ import annotations

import os
from unittest.mock import patch

from codeforge.tracing.setup import TracingManager, _NoOpTracer


class TestTracingManager:
    def test_disabled_when_not_dev(self) -> None:
        with patch.dict(os.environ, {"APP_ENV": "production"}):
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

    def test_noop_tracer_session_lifecycle(self) -> None:
        noop = _NoOpTracer()
        noop.start_session("run-1")
        noop.end_session("run-1")
        noop.instrument_litellm()
        # No errors should occur

    def test_dev_mode_without_agentneo(self) -> None:
        with patch.dict(os.environ, {"APP_ENV": "development"}):
            tm = TracingManager()
            tm.init()
            # agentneo is not installed in test env, should fallback to noop
            assert isinstance(tm.get_tracer(), _NoOpTracer)
