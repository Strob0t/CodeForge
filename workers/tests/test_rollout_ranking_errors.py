"""A rollout whose rank evaluator failed never outranks a fully evaluated one.

S6-G review, item 4: errored dimensions are left out of the average, so a
rollout whose LLM verifier failed was ranked on its filter scores alone and
could outrank verified rollouts. It now ranks below every fully evaluated
rollout; is_best goes to a fully evaluated rollout, or, when there is none,
to the best partial one (logged).
"""

from __future__ import annotations

import pytest
from structlog.testing import capture_logs

from codeforge.evaluation.hybrid_pipeline import HybridEvaluationPipeline
from codeforge.evaluation.providers.base import EvalDimension, ExecutionResult, TaskSpec
from codeforge.evaluation.runners.multi_rollout import MultiRolloutRunner
from codeforge.evaluation.runners.simple import RunResult


def _task() -> TaskSpec:
    return TaskSpec(id="t1", name="Fix bug", input="fix it")


class _Runner:
    def __init__(self, outputs: list[str]) -> None:
        self._outputs = outputs
        self._i = 0

    async def run_task(self, task: TaskSpec) -> RunResult:
        output = self._outputs[self._i]
        self._i += 1
        return RunResult(task=task, execution=ExecutionResult(actual_output=output))


class _Filter:
    name = "fake_filter"
    stage = "filter"

    async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
        # "partial" rollouts pass the filter perfectly, verified ones less so.
        score = 1.0 if result.actual_output.startswith("partial") else 0.8
        return [EvalDimension(name="functional_test", score=score)]


class _Rank:
    """An LLM verifier that fails on outputs starting with "partial"."""

    name = "fake_rank"
    stage = "rank"

    async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
        if result.actual_output.startswith("partial"):
            return [EvalDimension(name="correctness", score=0.0, error="verifier unavailable")]
        return [EvalDimension(name="correctness", score=0.6)]


def _hybrid() -> HybridEvaluationPipeline:
    return HybridEvaluationPipeline(filter_evaluators=[_Filter()], rank_evaluators=[_Rank()], filter_threshold=0.5)


async def test_best_rollout_is_fully_evaluated() -> None:
    runner = MultiRolloutRunner(_Runner(["partial a", "verified b", "partial c"]), _hybrid(), rollout_count=3)
    outcomes = await runner.run_task(_task())
    assert [o.is_best for o in outcomes] == [False, True, False]


async def test_best_partial_rollout_when_none_is_fully_evaluated() -> None:
    runner = MultiRolloutRunner(_Runner(["partial a", "partial b"]), _hybrid(), rollout_count=2)
    with capture_logs() as logs:
        outcomes = await runner.run_task(_task())
    assert sum(o.is_best for o in outcomes) == 1
    assert any("no fully evaluated rollout" in entry["event"] for entry in logs)


@pytest.mark.parametrize("order", [["partial a", "verified b"], ["verified b", "partial a"]])
async def test_verify_batch_ranks_partial_results_last(order: list[str]) -> None:
    results = [ExecutionResult(actual_output=o) for o in order]
    ranked = await _hybrid().verify_batch(_task(), results)
    assert [vr.fully_evaluated for vr in ranked] == [True, False]
