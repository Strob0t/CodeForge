"""Agent tracing and observability — AgentNeo integration for dev-mode instrumentation."""

from codeforge.tracing.dashboard import launch as launch_dashboard
from codeforge.tracing.metrics import (
    evaluate_goal_decomposition,
    evaluate_plan_adaptability,
    evaluate_tool_selection_accuracy,
)
from codeforge.tracing.setup import TracerProtocol, TracingManager

# Shared singleton — import this instead of creating new TracingManager() instances.
tracing_manager = TracingManager()

__all__ = [
    "TracerProtocol",
    "TracingManager",
    "evaluate_goal_decomposition",
    "evaluate_plan_adaptability",
    "evaluate_tool_selection_accuracy",
    "launch_dashboard",
    "tracing_manager",
]
