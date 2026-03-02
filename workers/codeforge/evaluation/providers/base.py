"""Base types and registry for benchmark providers.

Each provider knows how to load tasks and define evaluation capabilities
for a specific benchmark suite (e.g. HumanEval, SWE-bench, SPARC-bench).

Providers self-register via register_provider() at module import time,
following the same pattern as the Go benchprovider port.
"""

from __future__ import annotations

from enum import StrEnum
from typing import Protocol, runtime_checkable

from pydantic import BaseModel


class BenchmarkType(StrEnum):
    """Classifies the execution mode of a benchmark suite."""

    SIMPLE = "simple"
    TOOL_USE = "tool_use"
    AGENT = "agent"


class ToolCall(BaseModel):
    """Expected or actual tool invocation."""

    name: str
    args: str = ""


class TaskSpec(BaseModel):
    """Single benchmark task as provided by the benchmark suite."""

    id: str
    name: str
    input: str
    expected_output: str = ""
    expected_tools: list[ToolCall] = []
    context: list[str] = []
    difficulty: str = "medium"
    initial_files: dict[str, str] = {}
    test_command: str = ""
    repo_url: str = ""
    repo_commit: str = ""
    metadata: dict[str, str] = {}


class ExecutionResult(BaseModel):
    """Captures the output of running a benchmark task."""

    actual_output: str = ""
    tool_calls: list[ToolCall] = []
    files_changed: list[str] = []
    test_output: str = ""
    exit_code: int = 0
    cost_usd: float = 0.0
    tokens_in: int = 0
    tokens_out: int = 0
    duration_ms: int = 0
    step_count: int = 0
    metadata: dict[str, str] = {}


class EvalDimension(BaseModel):
    """Single named score from an evaluator."""

    name: str
    score: float
    details: dict[str, str] = {}
    cost_usd: float = 0.0


class EvalScore(BaseModel):
    """Aggregated evaluation result with all dimensions."""

    dimensions: list[EvalDimension] = []
    total_cost_usd: float = 0.0
    cost_per_score_point: float = 0.0
    token_efficiency: float = 0.0

    def average_score(self) -> float:
        """Compute mean score across all dimensions."""
        if not self.dimensions:
            return 0.0
        return sum(d.score for d in self.dimensions) / len(self.dimensions)


class Capabilities(BaseModel):
    """Declares which evaluation methods a provider supports."""

    functional_tests: bool = False
    llm_judge: bool = False
    swe_bench_style: bool = False
    sparc_style: bool = False


@runtime_checkable
class BenchmarkProvider(Protocol):
    """Interface that benchmark suite adapters must implement."""

    @property
    def name(self) -> str:
        """Unique identifier for this provider (e.g. 'humaneval', 'swe-bench')."""
        ...

    @property
    def benchmark_type(self) -> BenchmarkType:
        """Benchmark type this provider targets."""
        ...

    @property
    def capabilities(self) -> Capabilities:
        """Evaluation methods this provider supports."""
        ...

    async def load_tasks(self) -> list[TaskSpec]:
        """Load all tasks available in this benchmark suite."""
        ...

    async def task_count(self) -> int:
        """Return the number of tasks without loading all task data."""
        ...


# --- Provider Registry ---

_registry: dict[str, type[BenchmarkProvider]] = {}


def register_provider(name: str, provider_cls: type[BenchmarkProvider]) -> None:
    """Register a benchmark provider class by name.

    Args:
        name: Unique provider identifier.
        provider_cls: Provider class (not instance) to register.

    Raises:
        ValueError: If a provider with the same name is already registered.
    """
    if name in _registry:
        raise ValueError(f"benchprovider: duplicate registration for {name!r}")
    _registry[name] = provider_cls


def get_provider(name: str) -> type[BenchmarkProvider]:
    """Retrieve a registered provider class by name.

    Args:
        name: Provider identifier.

    Returns:
        The provider class.

    Raises:
        KeyError: If the provider is not registered.
    """
    if name not in _registry:
        raise KeyError(f"benchprovider: unknown provider {name!r}")
    return _registry[name]


def list_providers() -> list[str]:
    """Return the names of all registered providers."""
    return list(_registry.keys())
