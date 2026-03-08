"""Base benchmark runner — shared run_tasks loop and RunResult dataclass."""

from __future__ import annotations

import abc
from dataclasses import dataclass
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from codeforge.evaluation.providers.base import EvalScore, ExecutionResult, TaskSpec


@dataclass(slots=True)
class RunResult:
    """Holds task, execution output, and evaluation score together."""

    task: TaskSpec
    execution: ExecutionResult
    eval_score: EvalScore | None = None


class BaseBenchmarkRunner(abc.ABC):
    """Abstract base for benchmark runners with shared sequential loop."""

    async def run_tasks(self, tasks: list[TaskSpec]) -> list[RunResult]:
        """Run all tasks sequentially and return results."""
        results: list[RunResult] = []
        for task in tasks:
            result = await self.run_task(task)
            results.append(result)
        return results

    @abc.abstractmethod
    async def run_task(self, task: TaskSpec) -> RunResult:
        """Run a single benchmark task. Subclasses must implement this."""
