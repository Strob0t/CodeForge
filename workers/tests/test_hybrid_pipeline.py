"""Tests for HybridEvaluationPipeline — two-stage verification (filter → rank).

Tests cover: both stages run, filter-only, rank-only, all filtered fallback,
threshold edge cases, single result, cost accumulation, backward compat.
"""

from __future__ import annotations

import pytest

from codeforge.evaluation.hybrid_pipeline import HybridEvaluationPipeline
from codeforge.evaluation.providers.base import EvalDimension, ExecutionResult, TaskSpec

# ---------------------------------------------------------------------------
# Helpers: Fake evaluators for isolation
# ---------------------------------------------------------------------------


class _FakeFilterEvaluator:
    """Evaluator that returns a configurable score (simulates execution-based test)."""

    def __init__(self, score: float = 1.0) -> None:
        self._score = score
        self.call_count = 0

    @property
    def name(self) -> str:
        return "fake_filter"

    @property
    def stage(self) -> str:
        return "filter"

    async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
        self.call_count += 1
        return [EvalDimension(name="functional_test", score=self._score)]


class _FakeRankEvaluator:
    """Evaluator that returns a configurable score (simulates LLM judge)."""

    def __init__(self, score: float = 0.8) -> None:
        self._score = score
        self.call_count = 0

    @property
    def name(self) -> str:
        return "fake_rank"

    @property
    def stage(self) -> str:
        return "rank"

    async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
        self.call_count += 1
        return [EvalDimension(name="correctness", score=self._score, cost_usd=0.01)]


def _task(task_id: str = "t1") -> TaskSpec:
    return TaskSpec(id=task_id, name="test task", input="solve this")


def _result(output: str = "solution", exit_code: int = 0) -> ExecutionResult:
    return ExecutionResult(actual_output=output, exit_code=exit_code, cost_usd=0.05, tokens_in=100, tokens_out=50)


# ---------------------------------------------------------------------------
# Tests: verify() — single result
# ---------------------------------------------------------------------------


class TestVerifySingle:
    """Tests for verify() with a single task/result pair."""

    @pytest.mark.asyncio
    async def test_both_stages_pass(self) -> None:
        """Filter passes, rank runs — combined score includes both dimensions."""
        filt = _FakeFilterEvaluator(score=1.0)
        rank = _FakeRankEvaluator(score=0.9)
        pipeline = HybridEvaluationPipeline([filt], [rank])

        vr = await pipeline.verify(_task(), _result())

        assert vr.passed_filter is True
        assert len(vr.filter_scores) == 1
        assert vr.filter_scores[0].score == 1.0
        assert len(vr.rank_scores) == 1
        assert vr.rank_scores[0].score == 0.9
        assert vr.combined_score is not None
        assert filt.call_count == 1
        assert rank.call_count == 1

    @pytest.mark.asyncio
    async def test_filter_fails_below_threshold(self) -> None:
        """Result filtered out — rank evaluators should NOT run."""
        filt = _FakeFilterEvaluator(score=0.0)
        rank = _FakeRankEvaluator(score=0.9)
        pipeline = HybridEvaluationPipeline([filt], [rank], filter_threshold=0.5)

        vr = await pipeline.verify(_task(), _result())

        assert vr.passed_filter is False
        assert len(vr.filter_scores) == 1
        assert vr.rank_scores == []
        assert vr.combined_score is None
        assert rank.call_count == 0

    @pytest.mark.asyncio
    async def test_no_filter_evaluators(self) -> None:
        """No filter stage — rank runs directly on all results."""
        rank = _FakeRankEvaluator(score=0.7)
        pipeline = HybridEvaluationPipeline([], [rank])

        vr = await pipeline.verify(_task(), _result())

        assert vr.passed_filter is True
        assert vr.filter_scores == []
        assert len(vr.rank_scores) == 1
        assert vr.combined_score is not None

    @pytest.mark.asyncio
    async def test_no_rank_evaluators(self) -> None:
        """No rank stage — filter score is the combined score."""
        filt = _FakeFilterEvaluator(score=1.0)
        pipeline = HybridEvaluationPipeline([filt], [])

        vr = await pipeline.verify(_task(), _result())

        assert vr.passed_filter is True
        assert len(vr.filter_scores) == 1
        assert vr.rank_scores == []
        assert vr.combined_score is not None
        assert vr.combined_score.average_score() == 1.0

    @pytest.mark.asyncio
    async def test_threshold_exactly_at_boundary(self) -> None:
        """Score exactly at threshold — should pass filter."""
        filt = _FakeFilterEvaluator(score=0.5)
        rank = _FakeRankEvaluator(score=0.8)
        pipeline = HybridEvaluationPipeline([filt], [rank], filter_threshold=0.5)

        vr = await pipeline.verify(_task(), _result())

        assert vr.passed_filter is True
        assert rank.call_count == 1

    @pytest.mark.asyncio
    async def test_threshold_zero_passes_everything(self) -> None:
        """Threshold 0.0 — all results pass filter."""
        filt = _FakeFilterEvaluator(score=0.0)
        rank = _FakeRankEvaluator(score=0.6)
        pipeline = HybridEvaluationPipeline([filt], [rank], filter_threshold=0.0)

        vr = await pipeline.verify(_task(), _result())

        assert vr.passed_filter is True
        assert rank.call_count == 1

    @pytest.mark.asyncio
    async def test_cost_accumulation(self) -> None:
        """Costs from both stages are tracked in combined_score."""
        filt = _FakeFilterEvaluator(score=1.0)
        rank = _FakeRankEvaluator(score=0.8)
        pipeline = HybridEvaluationPipeline([filt], [rank])

        vr = await pipeline.verify(_task(), _result())

        assert vr.combined_score is not None
        # Rank evaluator returns cost_usd=0.01 per dimension
        assert vr.combined_score.total_cost_usd >= 0.01


# ---------------------------------------------------------------------------
# Tests: verify_batch() — multiple results
# ---------------------------------------------------------------------------


class TestVerifyBatch:
    """Tests for verify_batch() with multiple results (multi-rollout scenario)."""

    @pytest.mark.asyncio
    async def test_mixed_pass_fail_filters(self) -> None:
        """Only survivors get ranked — rank evaluator NOT called on filtered results."""
        filt = _FakeFilterEvaluator(score=1.0)
        rank = _FakeRankEvaluator(score=0.8)
        pipeline = HybridEvaluationPipeline([filt], [rank], filter_threshold=0.5)

        # Override filter to return different scores per call
        call_scores = [1.0, 0.0, 1.0]  # pass, fail, pass
        call_idx = 0

        async def varying_evaluate(task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
            nonlocal call_idx
            score = call_scores[call_idx] if call_idx < len(call_scores) else 0.0
            call_idx += 1
            filt.call_count += 1
            return [EvalDimension(name="functional_test", score=score)]

        filt.evaluate = varying_evaluate  # type: ignore[assignment]

        results = [_result("sol1"), _result("sol2"), _result("sol3")]
        vrs = await pipeline.verify_batch(_task(), results)

        assert len(vrs) == 3
        # verify_batch sorts by combined score desc — filtered result sorts last
        passed = [vr for vr in vrs if vr.passed_filter]
        failed = [vr for vr in vrs if not vr.passed_filter]
        assert len(passed) == 2
        assert len(failed) == 1
        assert failed[0].rank_scores == []
        assert failed[0].combined_score is None
        # Rank evaluator called only on 2 survivors
        assert rank.call_count == 2

    @pytest.mark.asyncio
    async def test_all_filtered_fallback_ranks_all(self) -> None:
        """When ALL results fail filter — fallback: rank all anyway (graceful degradation)."""
        filt = _FakeFilterEvaluator(score=0.0)
        rank = _FakeRankEvaluator(score=0.5)
        pipeline = HybridEvaluationPipeline([filt], [rank], filter_threshold=0.5)

        results = [_result("a"), _result("b"), _result("c")]
        vrs = await pipeline.verify_batch(_task(), results)

        assert len(vrs) == 3
        # All failed filter but fallback kicks in → rank runs on all
        for vr in vrs:
            assert vr.passed_filter is False
            assert len(vr.rank_scores) == 1  # Ranked despite filter failure
            assert vr.combined_score is not None
        assert rank.call_count == 3

    @pytest.mark.asyncio
    async def test_all_pass_filter(self) -> None:
        """All results pass filter — all get ranked."""
        filt = _FakeFilterEvaluator(score=1.0)
        rank = _FakeRankEvaluator(score=0.9)
        pipeline = HybridEvaluationPipeline([filt], [rank])

        results = [_result("a"), _result("b")]
        vrs = await pipeline.verify_batch(_task(), results)

        assert len(vrs) == 2
        for vr in vrs:
            assert vr.passed_filter is True
            assert len(vr.rank_scores) == 1
        assert rank.call_count == 2

    @pytest.mark.asyncio
    async def test_single_result_batch(self) -> None:
        """Batch with 1 result — degenerate case, still works."""
        filt = _FakeFilterEvaluator(score=1.0)
        rank = _FakeRankEvaluator(score=0.7)
        pipeline = HybridEvaluationPipeline([filt], [rank])

        vrs = await pipeline.verify_batch(_task(), [_result("only")])

        assert len(vrs) == 1
        assert vrs[0].passed_filter is True
        assert vrs[0].combined_score is not None

    @pytest.mark.asyncio
    async def test_empty_batch(self) -> None:
        """Empty batch — returns empty list."""
        pipeline = HybridEvaluationPipeline([_FakeFilterEvaluator()], [_FakeRankEvaluator()])

        vrs = await pipeline.verify_batch(_task(), [])

        assert vrs == []

    @pytest.mark.asyncio
    async def test_batch_sorted_by_combined_score(self) -> None:
        """Results returned in descending combined score order."""
        rank_scores = [0.3, 0.9, 0.6]
        call_idx = 0

        rank = _FakeRankEvaluator()

        async def varying_rank(task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
            nonlocal call_idx
            score = rank_scores[call_idx] if call_idx < len(rank_scores) else 0.0
            call_idx += 1
            rank.call_count += 1
            return [EvalDimension(name="correctness", score=score)]

        rank.evaluate = varying_rank  # type: ignore[assignment]
        filt = _FakeFilterEvaluator(score=1.0)
        pipeline = HybridEvaluationPipeline([filt], [rank])

        results = [_result("low"), _result("high"), _result("mid")]
        vrs = await pipeline.verify_batch(_task(), results)

        scores = [vr.combined_score.average_score() for vr in vrs if vr.combined_score]
        assert scores == sorted(scores, reverse=True)
