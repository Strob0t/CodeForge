"""Tests for score key normalization in _convert_result and _convert_rollout_outcome.

Bug: When a user requests metrics: ["llm_judge"], the API returns scores under
{"correctness": 0.8} instead of {"llm_judge": 0.8}. This is because evaluators
produce EvalDimension(name="correctness", ...) and the convert functions use
dim.name as the dict key without adding an aggregated metric-level key.

Fix: _aggregate_metric_scores() adds averaged parent-metric keys to the scores
dict while preserving the raw dimension keys.
"""

from __future__ import annotations

import pytest

from codeforge.consumer._benchmark import (
    _DIMENSION_TO_METRIC,
    _aggregate_metric_scores,
    _convert_result,
    _convert_rollout_outcome,
)
from codeforge.evaluation.providers.base import EvalDimension, EvalScore

# --- Fake objects that duck-type the real RunResult / RolloutOutcome ---


class _FakeToolCall:
    def __init__(self, name: str = "read", args: str = "{}") -> None:
        self.name = name
        self.args = args


class _FakeExecution:
    def __init__(
        self,
        actual_output: str = "output",
        cost_usd: float = 0.01,
        tokens_in: int = 100,
        tokens_out: int = 50,
        duration_ms: int = 500,
        tool_calls: list[_FakeToolCall] | None = None,
        files_changed: list[str] | None = None,
        test_output: str = "",
    ) -> None:
        self.actual_output = actual_output
        self.cost_usd = cost_usd
        self.tokens_in = tokens_in
        self.tokens_out = tokens_out
        self.duration_ms = duration_ms
        self.tool_calls = tool_calls or []
        self.files_changed = files_changed or []
        self.test_output = test_output


class _FakeTask:
    def __init__(self, task_id: str = "t1", name: str = "task1", expected_output: str = "") -> None:
        self.id = task_id
        self.name = name
        self.expected_output = expected_output


class _FakeRunResult:
    """Duck-types the RunResult used by _convert_result."""

    def __init__(self, task: _FakeTask, execution: _FakeExecution, eval_score: EvalScore | None) -> None:
        self.task = task
        self.execution = execution
        self.eval_score = eval_score


class _FakeRolloutOutcome:
    """Duck-types the RolloutOutcome used by _convert_rollout_outcome."""

    def __init__(
        self,
        execution: _FakeExecution,
        eval_score: EvalScore | None,
        rollout_id: int = 0,
        is_best: bool = True,
        diversity_score: float = 0.0,
    ) -> None:
        self.result = execution
        self.eval_score = eval_score
        self.rollout_id = rollout_id
        self.is_best = is_best
        self.diversity_score = diversity_score


# ---------------------------------------------------------------------------
# Tests for _aggregate_metric_scores helper
# ---------------------------------------------------------------------------


class TestAggregateMetricScores:
    """Unit tests for the _aggregate_metric_scores helper."""

    def test_llm_judge_single_dimension(self) -> None:
        """A single LLM judge dimension produces an aggregated 'llm_judge' key."""
        scores: dict[str, float] = {"correctness": 0.8}
        _aggregate_metric_scores(scores)
        assert "llm_judge" in scores
        assert scores["llm_judge"] == pytest.approx(0.8)
        # Raw key preserved
        assert scores["correctness"] == pytest.approx(0.8)

    def test_llm_judge_multiple_dimensions_averaged(self) -> None:
        """Multiple LLM judge dimensions are averaged into 'llm_judge'."""
        scores: dict[str, float] = {
            "correctness": 0.8,
            "faithfulness": 0.6,
            "answer_relevancy": 0.7,
            "tool_correctness": 0.9,
        }
        _aggregate_metric_scores(scores)
        assert "llm_judge" in scores
        expected = (0.8 + 0.6 + 0.7 + 0.9) / 4
        assert scores["llm_judge"] == pytest.approx(expected)
        # All raw keys preserved
        assert scores["correctness"] == pytest.approx(0.8)
        assert scores["faithfulness"] == pytest.approx(0.6)

    def test_sparc_dimensions_averaged(self) -> None:
        """All SPARC dimensions produce an aggregated 'sparc' key."""
        scores: dict[str, float] = {
            "sparc_steps": 0.9,
            "sparc_time": 0.8,
            "sparc_cost": 0.7,
            "sparc_complexity": 0.75,
            "sparc_code_quality": 1.0,
            "sparc_security": 1.0,
        }
        _aggregate_metric_scores(scores)
        assert "sparc" in scores
        expected = (0.9 + 0.8 + 0.7 + 0.75 + 1.0 + 1.0) / 6
        assert scores["sparc"] == pytest.approx(expected)

    def test_trajectory_verifier_dimensions_averaged(self) -> None:
        """All trajectory verifier dimensions produce an aggregated 'trajectory_verifier' key."""
        scores: dict[str, float] = {
            "trajectory_solution_quality": 0.9,
            "trajectory_approach_efficiency": 0.85,
            "trajectory_code_quality": 0.8,
            "trajectory_error_recovery": 0.7,
            "trajectory_completeness": 0.95,
        }
        _aggregate_metric_scores(scores)
        assert "trajectory_verifier" in scores
        expected = (0.9 + 0.85 + 0.8 + 0.7 + 0.95) / 5
        assert scores["trajectory_verifier"] == pytest.approx(expected)
        # Also check the error fallback key
        scores2: dict[str, float] = {"trajectory_quality": 0.0}
        _aggregate_metric_scores(scores2)
        assert "trajectory_verifier" in scores2
        assert scores2["trajectory_verifier"] == pytest.approx(0.0)

    def test_functional_test_identity(self) -> None:
        """functional_test is an identity mapping — no duplicate key created."""
        scores: dict[str, float] = {"functional_test": 1.0}
        _aggregate_metric_scores(scores)
        assert scores["functional_test"] == pytest.approx(1.0)
        # Should only have one key (no extra aggregated key because metric == dim)
        assert len(scores) == 1

    def test_empty_scores_unchanged(self) -> None:
        """An empty scores dict remains empty."""
        scores: dict[str, float] = {}
        _aggregate_metric_scores(scores)
        assert scores == {}

    def test_unknown_dimensions_ignored(self) -> None:
        """Dimension names not in the mapping are left alone, no aggregation."""
        scores: dict[str, float] = {"some_custom_metric": 0.5, "another_metric": 0.7}
        _aggregate_metric_scores(scores)
        assert len(scores) == 2
        assert "some_custom_metric" in scores
        assert "another_metric" in scores

    def test_mixed_evaluators(self) -> None:
        """Scores from multiple evaluators each get their own aggregated key."""
        scores: dict[str, float] = {
            "correctness": 0.8,
            "faithfulness": 0.6,
            "sparc_steps": 0.9,
            "sparc_cost": 0.7,
            "functional_test": 1.0,
        }
        _aggregate_metric_scores(scores)
        # LLM judge aggregated from correctness + faithfulness
        assert scores["llm_judge"] == pytest.approx((0.8 + 0.6) / 2)
        # SPARC aggregated from sparc_steps + sparc_cost
        assert scores["sparc"] == pytest.approx((0.9 + 0.7) / 2)
        # functional_test stays as-is (identity)
        assert scores["functional_test"] == pytest.approx(1.0)
        # Raw keys preserved
        assert scores["correctness"] == pytest.approx(0.8)
        assert scores["sparc_steps"] == pytest.approx(0.9)


# ---------------------------------------------------------------------------
# Tests for _DIMENSION_TO_METRIC mapping completeness
# ---------------------------------------------------------------------------


class TestDimensionToMetricMapping:
    """Verify the mapping contains all expected dimension names."""

    def test_all_llm_judge_dimensions_mapped(self) -> None:
        for dim in ("correctness", "faithfulness", "answer_relevancy", "tool_correctness"):
            assert _DIMENSION_TO_METRIC[dim] == "llm_judge"

    def test_all_sparc_dimensions_mapped(self) -> None:
        for dim in (
            "sparc_steps",
            "sparc_time",
            "sparc_cost",
            "sparc_complexity",
            "sparc_code_quality",
            "sparc_security",
        ):
            assert _DIMENSION_TO_METRIC[dim] == "sparc"

    def test_all_trajectory_verifier_dimensions_mapped(self) -> None:
        for dim in (
            "trajectory_solution_quality",
            "trajectory_approach_efficiency",
            "trajectory_code_quality",
            "trajectory_error_recovery",
            "trajectory_completeness",
            "trajectory_quality",
        ):
            assert _DIMENSION_TO_METRIC[dim] == "trajectory_verifier"

    def test_functional_test_mapped(self) -> None:
        assert _DIMENSION_TO_METRIC["functional_test"] == "functional_test"


# ---------------------------------------------------------------------------
# Integration: _convert_result includes aggregated keys
# ---------------------------------------------------------------------------


class TestConvertResultAggregation:
    """Verify _convert_result produces aggregated metric keys in scores."""

    def test_convert_result_adds_llm_judge_key(self) -> None:
        task = _FakeTask()
        execution = _FakeExecution()
        eval_score = EvalScore(
            dimensions=[
                EvalDimension(name="correctness", score=0.8),
                EvalDimension(name="faithfulness", score=0.6),
            ]
        )
        run_result = _FakeRunResult(task=task, execution=execution, eval_score=eval_score)
        result = _convert_result(run_result)
        assert result.scores["llm_judge"] == pytest.approx(0.7)
        assert result.scores["correctness"] == pytest.approx(0.8)
        assert result.scores["faithfulness"] == pytest.approx(0.6)

    def test_convert_result_no_eval_score(self) -> None:
        """When eval_score is None, scores dict is empty (no crash)."""
        task = _FakeTask()
        execution = _FakeExecution()
        run_result = _FakeRunResult(task=task, execution=execution, eval_score=None)
        result = _convert_result(run_result)
        assert result.scores == {}


# ---------------------------------------------------------------------------
# Integration: _convert_rollout_outcome includes aggregated keys
# ---------------------------------------------------------------------------


class TestConvertRolloutOutcomeAggregation:
    """Verify _convert_rollout_outcome produces aggregated metric keys in scores."""

    def test_convert_rollout_outcome_adds_sparc_key(self) -> None:
        task = _FakeTask()
        execution = _FakeExecution()
        eval_score = EvalScore(
            dimensions=[
                EvalDimension(name="sparc_steps", score=0.9),
                EvalDimension(name="sparc_time", score=0.8),
                EvalDimension(name="sparc_cost", score=0.7),
            ]
        )
        outcome = _FakeRolloutOutcome(execution=execution, eval_score=eval_score)
        result = _convert_rollout_outcome(task, outcome, rollout_count=3)
        assert result.scores["sparc"] == pytest.approx((0.9 + 0.8 + 0.7) / 3)
        assert result.scores["sparc_steps"] == pytest.approx(0.9)

    def test_convert_rollout_outcome_no_eval_score(self) -> None:
        """When eval_score is None, scores dict is empty (no crash)."""
        task = _FakeTask()
        execution = _FakeExecution()
        outcome = _FakeRolloutOutcome(execution=execution, eval_score=None)
        result = _convert_rollout_outcome(task, outcome, rollout_count=1)
        assert result.scores == {}
