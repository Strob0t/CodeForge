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

# Self-register all benchmark providers on import.
import codeforge.evaluation.providers.aider_polyglot as _ap  # noqa: F401
import codeforge.evaluation.providers.bigcodebench as _b  # noqa: F401
import codeforge.evaluation.providers.codeforge_agent as _ca  # noqa: F401
import codeforge.evaluation.providers.codeforge_simple as _cs  # noqa: F401
import codeforge.evaluation.providers.codeforge_synthetic as _  # noqa: F401
import codeforge.evaluation.providers.codeforge_tool_use as _ct  # noqa: F401
import codeforge.evaluation.providers.cruxeval as _cr  # noqa: F401
import codeforge.evaluation.providers.humaneval as _h  # noqa: F401
import codeforge.evaluation.providers.livecodebench as _l  # noqa: F401
import codeforge.evaluation.providers.mbpp as _m  # noqa: F401
import codeforge.evaluation.providers.sparcbench as _sp  # noqa: F401
import codeforge.evaluation.providers.swebench as _s  # noqa: F401
