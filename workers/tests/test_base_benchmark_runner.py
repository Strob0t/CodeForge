"""Tests for BaseBenchmarkRunner and RunResult."""

from __future__ import annotations

import pytest

from codeforge.evaluation.providers.base import ExecutionResult, TaskSpec
from codeforge.evaluation.runners._base import BaseBenchmarkRunner, RunResult


class FakeRunner(BaseBenchmarkRunner):
    """Concrete runner for testing the abstract base."""

    async def run_task(self, task: TaskSpec) -> RunResult:
        return RunResult(
            task=task,
            execution=ExecutionResult(actual_output=f"done:{task.id}"),
        )


@pytest.mark.asyncio
async def test_run_tasks_returns_results_for_each_task():
    runner = FakeRunner()
    tasks = [
        TaskSpec(id="t1", name="task1", input="hello"),
        TaskSpec(id="t2", name="task2", input="world"),
    ]
    results = await runner.run_tasks(tasks)

    assert len(results) == 2
    assert results[0].task.id == "t1"
    assert results[0].execution.actual_output == "done:t1"
    assert results[1].task.id == "t2"
    assert results[1].execution.actual_output == "done:t2"


@pytest.mark.asyncio
async def test_run_tasks_empty_list():
    runner = FakeRunner()
    results = await runner.run_tasks([])
    assert results == []


def test_run_result_default_eval_score_is_none():
    task = TaskSpec(id="t1", name="task1", input="hi")
    execution = ExecutionResult(actual_output="ok")
    result = RunResult(task=task, execution=execution)
    assert result.eval_score is None


def test_base_runner_is_abstract():
    with pytest.raises(TypeError, match="abstract"):
        BaseBenchmarkRunner()  # type: ignore[abstract]
