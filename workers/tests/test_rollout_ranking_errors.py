"""A rollout whose rank evaluator failed never outranks a fully evaluated one.

S6-G review, item 4: errored dimensions are left out of the average, so a
rollout whose LLM verifier failed was ranked on its filter scores alone and
could outrank verified rollouts. It now ranks below every fully evaluated
rollout; is_best goes to a fully evaluated rollout, or, when there is none,
to the best partial one (logged).
"""

from __future__ import annotations

from unittest.mock import patch

import pytest
from structlog.testing import capture_logs

from codeforge.evaluation.hybrid_pipeline import HybridEvaluationPipeline
from codeforge.evaluation.providers.base import EvalDimension, EvalScore, ExecutionResult, TaskSpec
from codeforge.evaluation.runners.early_stopping import EarlyStopChecker
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


class _FailingFilter:
    """A filter evaluator that fails on outputs starting with "unfiltered"."""

    name = "fake_filter"
    stage = "filter"

    async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
        if result.actual_output.startswith("unfiltered"):
            return [
                EvalDimension(name="functional_test", score=1.0),
                EvalDimension(name="lint", score=0.0, error="linter crashed"),
            ]
        return [EvalDimension(name="functional_test", score=0.7), EvalDimension(name="lint", score=0.7)]


class _OkRank:
    name = "fake_rank"
    stage = "rank"

    async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
        return [EvalDimension(name="correctness", score=0.7)]


def _hybrid_with_failing_filter() -> HybridEvaluationPipeline:
    return HybridEvaluationPipeline(
        filter_evaluators=[_FailingFilter()], rank_evaluators=[_OkRank()], filter_threshold=0.5
    )


async def test_filter_stage_error_means_not_fully_evaluated() -> None:
    """S6-G re-review 2: an error in the filter stage counts like one in the rank stage."""
    vr = await _hybrid_with_failing_filter().verify(_task(), ExecutionResult(actual_output="unfiltered a"))
    assert vr.passed_filter
    assert not vr.fully_evaluated
    ranked = await _hybrid_with_failing_filter().verify_batch(
        _task(), [ExecutionResult(actual_output="unfiltered a"), ExecutionResult(actual_output="verified b")]
    )
    assert [vr.fully_evaluated for vr in ranked] == [True, False]


def test_early_stop_cluster_prefers_fully_evaluated_rollouts() -> None:
    checker = EarlyStopChecker(threshold=0.9, quorum=3)
    checker.add_rollout(0, "same", exit_code=0, score=1.0, fully_evaluated=False)
    checker.add_rollout(1, "same", exit_code=0, score=0.5)
    checker.add_rollout(2, "same", exit_code=0, score=0.6)
    assert checker.should_stop()
    assert checker.best_from_cluster() == 2


class _ScoredRunner:
    """Identical outputs (so early stopping triggers) with preset eval scores."""

    def __init__(self, scores: list[EvalScore]) -> None:
        self._scores = scores
        self._i = 0

    async def run_task(self, task: TaskSpec) -> RunResult:
        score = self._scores[self._i]
        self._i += 1
        return RunResult(task=task, execution=ExecutionResult(actual_output="same output"), eval_score=score)


async def test_early_stop_selects_by_rank_key() -> None:
    """S6-G re-review 2: the early-stop selection ranks like the hybrid one.

    Rollout 0 has the highest average only because its failed dimension is
    left out of it; a fully evaluated rollout of the cluster wins.
    """
    partial = EvalScore(
        dimensions=[EvalDimension(name="a", score=1.0), EvalDimension(name="b", score=0.0, error="judge failed")]
    )
    full_low = EvalScore(dimensions=[EvalDimension(name="a", score=0.5), EvalDimension(name="b", score=0.5)])
    full_high = EvalScore(dimensions=[EvalDimension(name="a", score=0.8), EvalDimension(name="b", score=0.6)])
    runner = MultiRolloutRunner(_ScoredRunner([partial, full_low, full_high]), None, rollout_count=5)
    with patch("codeforge.evaluation.runners.early_stopping.get_settings") as settings:
        settings.return_value.early_stop_threshold = 0.9
        settings.return_value.early_stop_quorum = 3
        outcomes = await runner.run_task(_task())
    assert runner.last_run_metadata.early_stopped
    assert [o.is_best for o in outcomes] == [False, False, True]


async def test_early_stop_logs_a_partial_choice() -> None:
    partial = EvalScore(dimensions=[EvalDimension(name="a", score=0.0, error="judge failed")])
    runner = MultiRolloutRunner(_ScoredRunner([partial, partial, partial]), None, rollout_count=5)
    with patch("codeforge.evaluation.runners.early_stopping.get_settings") as settings, capture_logs() as logs:
        settings.return_value.early_stop_threshold = 0.9
        settings.return_value.early_stop_quorum = 3
        outcomes = await runner.run_task(_task())
    assert [o.is_best for o in outcomes] == [True, False, False]
    assert any("best partial one selected" in entry["event"] for entry in logs)
