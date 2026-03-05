"""OTEL metric instruments for CodeForge Python workers."""

from __future__ import annotations

from opentelemetry import metrics

meter = metrics.get_meter("codeforge")

loop_iterations = meter.create_counter(
    "codeforge.agent.loop.iterations",
    unit="{iteration}",
    description="Total agent loop iterations",
)

loop_duration = meter.create_histogram(
    "codeforge.agent.loop.duration",
    unit="s",
    description="Duration of a complete agent run",
)

llm_call_duration = meter.create_histogram(
    "codeforge.llm.call.duration",
    unit="s",
    description="LLM API call latency",
)

llm_tokens = meter.create_counter(
    "codeforge.llm.tokens.used",
    unit="{token}",
    description="Total tokens consumed (prompt + completion)",
)

tool_duration = meter.create_histogram(
    "codeforge.tool.execution.duration",
    unit="s",
    description="Per-tool execution time",
)

nats_processing = meter.create_histogram(
    "codeforge.nats.message.processing.duration",
    unit="s",
    description="NATS message handling time",
)
