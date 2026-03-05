"""Tests for MultiRolloutRunner — N independent rollouts with best-of-N selection.

Tests cover: single rollout (no-op), multiple with distinct outputs (diversity),
identical outputs, best-of-N selection via hybrid pipeline, majority voting,
empty tasks, and integration with HybridEvaluationPipeline.
"""

from __future__ import annotations

import pytest

from codeforge.evaluation.hybrid_pipeline import HybridEvaluationPipeline
from codeforge.evaluation.providers.base import EvalDimension, ExecutionResult, TaskSpec
from codeforge.evaluation.runners.multi_rollout import MultiRolloutRunner, compute_diversity
from codeforge.evaluation.runners.simple import RunResult

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _task() -> TaskSpec:
    return TaskSpec(id="t1", name="Fix bug", input="fix it")


class _FakeInnerRunner:
    """Fake runner that returns configurable outputs per call."""

    def __init__(self, outputs: list[str]) -> None:
        self._outputs = outputs
        self._call_idx = 0

    async def run_task(self, task: TaskSpec) -> RunResult:
        output = self._outputs[self._call_idx % len(self._outputs)]
        self._call_idx += 1
        execution = ExecutionResult(
            actual_output=output,
            exit_code=0 if output != "FAIL" else 1,
            cost_usd=0.01,
            tokens_in=100,
            tokens_out=50,
        )
        return RunResult(task=task, execution=execution)


class _FakeFilterEvaluator:
    @property
    def name(self) -> str:
        return "fake_filter"

    @property
    def stage(self) -> str:
        return "filter"

    async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
        score = 1.0 if result.exit_code == 0 else 0.0
        return [EvalDimension(name="functional_test", score=score)]


class _FakeRankEvaluator:
    """Rank evaluator that scores based on output length (longer = better)."""

    @property
    def name(self) -> str:
        return "fake_rank"

    @property
    def stage(self) -> str:
        return "rank"

    async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
        score = min(1.0, len(result.actual_output) / 20.0)
        return [EvalDimension(name="correctness", score=score)]


# ---------------------------------------------------------------------------
# Tests: compute_diversity
# ---------------------------------------------------------------------------


class TestComputeDiversity:
    def test_identical_outputs_zero_diversity(self) -> None:
        outputs = ["same", "same", "same"]
        scores = compute_diversity(outputs)
        assert len(scores) == 3
        for s in scores:
            assert s == 0.0

    def test_distinct_outputs_positive_diversity(self) -> None:
        outputs = ["aaa", "bbb", "ccc"]
        scores = compute_diversity(outputs)
        assert len(scores) == 3
        for s in scores:
            assert s > 0.0

    def test_single_output_zero_diversity(self) -> None:
        scores = compute_diversity(["only"])
        assert scores == [0.0]

    def test_empty_outputs(self) -> None:
        scores = compute_diversity([])
        assert scores == []

    def test_two_completely_different(self) -> None:
        scores = compute_diversity(["abc", "xyz"])
        assert len(scores) == 2
        # Both should have the same diversity (symmetric)
        assert scores[0] == scores[1]
        assert scores[0] > 0.0


# ---------------------------------------------------------------------------
# Tests: MultiRolloutRunner
# ---------------------------------------------------------------------------


class TestMultiRolloutRunner:
    @pytest.mark.asyncio
    async def test_single_rollout_passthrough(self) -> None:
        """rollout_count=1 → single execution, no selection logic."""
        inner = _FakeInnerRunner(["solution"])
        runner = MultiRolloutRunner(inner, hybrid_pipeline=None, rollout_count=1)

        outcomes = await runner.run_task(_task())

        assert len(outcomes) == 1
        assert outcomes[0].rollout_id == 0
        assert outcomes[0].result.actual_output == "solution"
        assert outcomes[0].is_best is True

    @pytest.mark.asyncio
    async def test_multiple_rollouts_all_executed(self) -> None:
        """rollout_count=3 → 3 independent executions."""
        inner = _FakeInnerRunner(["sol_a", "sol_b", "sol_c"])
        runner = MultiRolloutRunner(inner, hybrid_pipeline=None, rollout_count=3)

        outcomes = await runner.run_task(_task())

        assert len(outcomes) == 3
        outputs = {o.result.actual_output for o in outcomes}
        assert outputs == {"sol_a", "sol_b", "sol_c"}

    @pytest.mark.asyncio
    async def test_diversity_scores_computed(self) -> None:
        """Distinct outputs → positive diversity scores."""
        inner = _FakeInnerRunner(["aaa", "bbb", "ccc"])
        runner = MultiRolloutRunner(inner, hybrid_pipeline=None, rollout_count=3)

        outcomes = await runner.run_task(_task())

        for o in outcomes:
            assert o.diversity_score > 0.0

    @pytest.mark.asyncio
    async def test_identical_outputs_zero_diversity(self) -> None:
        """Identical outputs → diversity scores = 0."""
        inner = _FakeInnerRunner(["same"])
        runner = MultiRolloutRunner(inner, hybrid_pipeline=None, rollout_count=3)

        outcomes = await runner.run_task(_task())

        for o in outcomes:
            assert o.diversity_score == 0.0

    @pytest.mark.asyncio
    async def test_best_of_n_with_hybrid_pipeline(self) -> None:
        """Hybrid pipeline selects best rollout based on combined score."""
        inner = _FakeInnerRunner(["short", "a much longer solution here", "mid length"])
        pipeline = HybridEvaluationPipeline(
            filter_evaluators=[_FakeFilterEvaluator()],
            rank_evaluators=[_FakeRankEvaluator()],
        )
        runner = MultiRolloutRunner(inner, hybrid_pipeline=pipeline, rollout_count=3, strategy="best")

        outcomes = await runner.run_task(_task())

        assert len(outcomes) == 3
        best = [o for o in outcomes if o.is_best]
        assert len(best) == 1
        # Longest output should win (rank evaluator scores by length)
        assert best[0].result.actual_output == "a much longer solution here"

    @pytest.mark.asyncio
    async def test_majority_strategy(self) -> None:
        """Majority voting — pass if majority of rollouts succeed."""
        inner = _FakeInnerRunner(["ok", "FAIL", "ok"])
        runner = MultiRolloutRunner(inner, hybrid_pipeline=None, rollout_count=3, strategy="majority")

        outcomes = await runner.run_task(_task())

        assert len(outcomes) == 3
        best = [o for o in outcomes if o.is_best]
        # 2 out of 3 succeeded → majority passes → a passing one is best
        assert len(best) >= 1
        assert best[0].result.exit_code == 0

    @pytest.mark.asyncio
    async def test_rollout_ids_sequential(self) -> None:
        """Each rollout gets a sequential rollout_id."""
        inner = _FakeInnerRunner(["a", "b", "c", "d"])
        runner = MultiRolloutRunner(inner, hybrid_pipeline=None, rollout_count=4)

        outcomes = await runner.run_task(_task())

        ids = [o.rollout_id for o in outcomes]
        assert ids == [0, 1, 2, 3]

    @pytest.mark.asyncio
    async def test_no_pipeline_first_rollout_is_best(self) -> None:
        """Without hybrid pipeline, first rollout is marked best (no selection)."""
        inner = _FakeInnerRunner(["a", "b"])
        runner = MultiRolloutRunner(inner, hybrid_pipeline=None, rollout_count=2)

        outcomes = await runner.run_task(_task())

        best = [o for o in outcomes if o.is_best]
        assert len(best) == 1
        assert best[0].rollout_id == 0
