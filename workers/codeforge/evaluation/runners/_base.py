"""Base benchmark runner — shared run_tasks loop and RunResult dataclass."""

from __future__ import annotations

import abc
from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from codeforge.evaluation.providers.base import EvalScore, ExecutionResult, TaskSpec

# Callback signatures for per-task progress reporting.
OnTaskStart = Callable[["TaskSpec", int, int], Awaitable[None]]
OnTaskComplete = Callable[["TaskSpec", "RunResult", int, int], Awaitable[None]]


@dataclass(slots=True)
class RunResult:
    """Holds task, execution output, and evaluation score together."""

    task: TaskSpec
    execution: ExecutionResult
    eval_score: EvalScore | None = None


class BaseBenchmarkRunner(abc.ABC):
    """Abstract base for benchmark runners with shared sequential loop."""

    async def run_tasks(
        self,
        tasks: list[TaskSpec],
        on_task_start: OnTaskStart | None = None,
        on_task_complete: OnTaskComplete | None = None,
    ) -> list[RunResult]:
        """Run all tasks sequentially, invoking callbacks for progress."""
        results: list[RunResult] = []
        total = len(tasks)
        for i, task in enumerate(tasks):
            if on_task_start is not None:
                await on_task_start(task, i, total)
            result = await self.run_task(task)
            results.append(result)
            if on_task_complete is not None:
                await on_task_complete(task, result, i, total)
        return results

    @abc.abstractmethod
    async def run_task(self, task: TaskSpec) -> RunResult:
        """Run a single benchmark task. Subclasses must implement this."""
