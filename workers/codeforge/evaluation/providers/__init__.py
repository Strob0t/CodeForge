"""Benchmark provider registry — pluggable benchmark suite adapters."""

from codeforge.evaluation.providers.base import (
    BenchmarkProvider,
    BenchmarkType,
    Capabilities,
    EvalDimension,
    EvalScore,
    ExecutionResult,
    TaskSpec,
    ToolCall,
    get_provider,
    list_providers,
    register_provider,
)

__all__ = [
    "BenchmarkProvider",
    "BenchmarkType",
    "Capabilities",
    "EvalDimension",
    "EvalScore",
    "ExecutionResult",
    "TaskSpec",
    "ToolCall",
    "get_provider",
    "list_providers",
    "register_provider",
]

# Phase 28F: Self-register synthetic benchmark provider.
import codeforge.evaluation.providers.codeforge_synthetic as _  # noqa: F401
