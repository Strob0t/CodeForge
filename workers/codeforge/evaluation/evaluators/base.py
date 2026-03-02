"""Evaluator protocol — defines the contract for all evaluation plugins.

Each evaluator takes a TaskSpec and an ExecutionResult and produces a list
of EvalDimension scores. Evaluators can be composed via the EvaluationPipeline.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Protocol, runtime_checkable

if TYPE_CHECKING:
    from codeforge.evaluation.providers.base import EvalDimension, ExecutionResult, TaskSpec


@runtime_checkable
class Evaluator(Protocol):
    """Interface that all evaluator plugins must implement."""

    @property
    def name(self) -> str:
        """Unique identifier for this evaluator (e.g. 'llm_judge', 'functional_test')."""
        ...

    @property
    def stage(self) -> str:
        """Evaluation stage: 'filter' (execution-based) or 'rank' (execution-free).

        Used by HybridEvaluationPipeline to split evaluators into two phases.
        Defaults to 'rank' for backward compatibility.
        """
        return "rank"

    async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
        """Evaluate a single task execution and return scored dimensions.

        Args:
            task: The benchmark task specification.
            result: The captured execution result.

        Returns:
            One or more EvalDimension scores.
        """
        ...
