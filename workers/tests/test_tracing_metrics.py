"""Tests for OTEL metric instruments."""

from __future__ import annotations

from codeforge.tracing.metrics import (
    llm_call_duration,
    llm_tokens,
    loop_duration,
    loop_iterations,
    nats_processing,
    tool_duration,
)


class TestMetricInstruments:
    def test_counter_add(self) -> None:
        """Counters should accept add() calls without error."""
        loop_iterations.add(1)
        llm_tokens.add(150)

    def test_histogram_record(self) -> None:
        """Histograms should accept record() calls without error."""
        loop_duration.record(2.5)
        llm_call_duration.record(0.8)
        tool_duration.record(0.05)
        nats_processing.record(1.2)

    def test_counter_with_attributes(self) -> None:
        """Counters should accept attributes."""
        loop_iterations.add(1, {"model": "gpt-4o"})
        llm_tokens.add(100, {"model": "claude-3-opus", "direction": "input"})

    def test_histogram_with_attributes(self) -> None:
        """Histograms should accept attributes."""
        llm_call_duration.record(1.5, {"model": "gpt-4o"})
        tool_duration.record(0.3, {"tool.name": "read_file"})
