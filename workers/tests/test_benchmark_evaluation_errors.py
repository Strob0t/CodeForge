"""Evaluator errors are reported apart from the scores (S6-G review, item 3).

convert_result and convert_rollout_outcome copied `<evaluator>_error`
dimensions (and dimensions an evaluator marked as errors) into `scores`,
where compute_summary and the Go averages counted them as 0.0. Published
scores now hold results only; errors go to `evaluation_errors`.
"""

from __future__ import annotations

from codeforge.consumer._benchmark_gemmas import compute_summary, convert_result, convert_rollout_outcome
from codeforge.evaluation.providers.base import EvalDimension, EvalScore
from codeforge.models import BenchmarkTaskResult
from tests.test_score_key_normalization import _FakeExecution, _FakeRolloutOutcome, _FakeRunResult, _FakeTask


def _score() -> EvalScore:
    return EvalScore(
        dimensions=[
            EvalDimension(name="correctness", score=0.8),
            EvalDimension(name="llm_judge_error", score=0.0, details={"error": "proxy down"}, error="proxy down"),
            EvalDimension(name="trajectory_completeness", score=0.0, error="dimension missing from the verdict"),
            EvalDimension(name="functional_test.pass_rate", score=1.0),
        ]
    )


def _assert_split(result: BenchmarkTaskResult) -> None:
    assert result.evaluation_errors == {
        "llm_judge_error": "proxy down",
        "trajectory_completeness": "dimension missing from the verdict",
    }
    assert "llm_judge_error" not in result.scores
    assert "trajectory_completeness" not in result.scores
    assert "trajectory_verifier" not in result.scores  # no result, no aggregate
    assert result.scores["correctness"] == 0.8
    assert result.scores["llm_judge"] == 0.8
    for dims in result.evaluator_scores.values():
        assert "llm_judge_error" not in dims
        assert "trajectory_completeness" not in dims


def test_convert_result_keeps_errors_out_of_the_scores() -> None:
    run = _FakeRunResult(task=_FakeTask(), execution=_FakeExecution(), eval_score=_score())
    _assert_split(convert_result(run))


def test_convert_rollout_outcome_keeps_errors_out_of_the_scores() -> None:
    outcome = _FakeRolloutOutcome(execution=_FakeExecution(), eval_score=_score())
    _assert_split(convert_rollout_outcome(_FakeTask(), outcome, rollout_count=2))


def test_summary_average_ignores_errors() -> None:
    run = _FakeRunResult(task=_FakeTask(), execution=_FakeExecution(), eval_score=_score())
    summary = compute_summary([convert_result(run)], elapsed_ms=10)
    # correctness 0.8, functional_test.pass_rate 1.0, llm_judge 0.8 (aggregate)
    assert summary["avg_score"] == round((0.8 + 1.0 + 0.8) / 3, 4)


def test_result_without_errors_has_an_empty_error_map() -> None:
    run = _FakeRunResult(
        task=_FakeTask(),
        execution=_FakeExecution(),
        eval_score=EvalScore(dimensions=[EvalDimension(name="x", score=1)]),
    )
    result = convert_result(run)
    assert result.evaluation_errors == {}
    assert BenchmarkTaskResult(task_id="t", task_name="n").evaluation_errors == {}
