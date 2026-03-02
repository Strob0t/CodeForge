"""Hybrid two-stage evaluation pipeline — filter then rank.

Inspired by R2E-Gym's hybrid verification: execution-based evaluators (Stage 1)
filter out broken results quickly, then LLM-based evaluators (Stage 2) rank only
the survivors. This avoids wasting expensive LLM calls on clearly wrong outputs
and yields complementary signal (R2E-Gym: 51% vs ~42% for either stage alone).

Usage::

    pipeline = HybridEvaluationPipeline(
        filter_evaluators=[FunctionalTestEvaluator()],
        rank_evaluators=[LLMJudgeEvaluator(), TrajectoryVerifierEvaluator()],
        filter_threshold=0.5,
    )
    # Single result
    vr = await pipeline.verify(task, result)

    # Multiple rollouts — filter all, rank survivors, sort by score
    vrs = await pipeline.verify_batch(task, results)
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import TYPE_CHECKING

import structlog

from codeforge.evaluation.pipeline import EvaluationPipeline
from codeforge.evaluation.providers.base import EvalDimension, EvalScore, ExecutionResult, TaskSpec

if TYPE_CHECKING:
    from codeforge.evaluation.evaluators.base import Evaluator

logger = structlog.get_logger()


@dataclass(frozen=True)
class VerificationResult:
    """Outcome of hybrid two-stage verification for a single result."""

    passed_filter: bool
    filter_scores: list[EvalDimension] = field(default_factory=list)
    rank_scores: list[EvalDimension] = field(default_factory=list)
    combined_score: EvalScore | None = None


class HybridEvaluationPipeline:
    """Two-stage verification: filter (execution-based) → rank (LLM-based).

    Stage 1 (filter): Cheap, fast evaluators that produce binary pass/fail.
        Results with average filter score below ``filter_threshold`` are
        discarded before Stage 2 runs.

    Stage 2 (rank): Expensive, precise evaluators (LLM judge, trajectory
        verifier) that produce continuous quality scores.  Only invoked on
        results that survived Stage 1.

    If *all* results are filtered out, the pipeline falls back to ranking
    all of them anyway (graceful degradation — never discard everything).
    """

    def __init__(
        self,
        filter_evaluators: list[Evaluator],
        rank_evaluators: list[Evaluator],
        filter_threshold: float = 0.5,
    ) -> None:
        self._filter_evaluators = filter_evaluators
        self._rank_evaluators = rank_evaluators
        self._filter_threshold = filter_threshold

        # Compose internal pipelines only when evaluators are present.
        self._filter_pipeline = EvaluationPipeline(filter_evaluators) if filter_evaluators else None
        self._rank_pipeline = EvaluationPipeline(rank_evaluators) if rank_evaluators else None

    async def verify(self, task: TaskSpec, result: ExecutionResult) -> VerificationResult:
        """Verify a single result through both stages."""
        # Stage 1: Filter
        filter_dims: list[EvalDimension] = []
        passed = True

        if self._filter_pipeline is not None:
            filter_score = await self._filter_pipeline.evaluate(task, result)
            filter_dims = filter_score.dimensions
            passed = filter_score.average_score() >= self._filter_threshold

        if not passed:
            return VerificationResult(
                passed_filter=False,
                filter_scores=filter_dims,
                rank_scores=[],
                combined_score=None,
            )

        # Stage 2: Rank
        rank_dims: list[EvalDimension] = []
        if self._rank_pipeline is not None:
            rank_score = await self._rank_pipeline.evaluate(task, result)
            rank_dims = rank_score.dimensions

        combined = _merge_scores(filter_dims, rank_dims, result)
        return VerificationResult(
            passed_filter=True,
            filter_scores=filter_dims,
            rank_scores=rank_dims,
            combined_score=combined,
        )

    async def verify_batch(
        self,
        task: TaskSpec,
        results: list[ExecutionResult],
    ) -> list[VerificationResult]:
        """Verify multiple results: filter all, then rank survivors.

        Returns results sorted by combined score (descending).
        """
        if not results:
            return []

        # Stage 1: Filter all results.
        filter_outcomes: list[tuple[ExecutionResult, list[EvalDimension], bool]] = []

        for result in results:
            filter_dims: list[EvalDimension] = []
            passed = True

            if self._filter_pipeline is not None:
                filter_score = await self._filter_pipeline.evaluate(task, result)
                filter_dims = filter_score.dimensions
                passed = filter_score.average_score() >= self._filter_threshold

            filter_outcomes.append((result, filter_dims, passed))

        survivors = [(r, dims) for r, dims, passed in filter_outcomes if passed]

        # Fallback: if all filtered out, rank all (graceful degradation).
        all_filtered = len(survivors) == 0
        if all_filtered:
            logger.info(
                "all results filtered — falling back to ranking all",
                task_id=task.id,
                total=len(results),
            )
            rank_targets = [(r, dims) for r, dims, _ in filter_outcomes]
        else:
            rank_targets = survivors

        # Stage 2: Rank survivors (or all on fallback).
        ranked_vrs: list[VerificationResult] = []
        rank_result_set = {id(r) for r, _ in rank_targets}

        for result, filter_dims, passed in filter_outcomes:
            if id(result) in rank_result_set:
                rank_dims: list[EvalDimension] = []
                if self._rank_pipeline is not None:
                    rank_score = await self._rank_pipeline.evaluate(task, result)
                    rank_dims = rank_score.dimensions
                combined = _merge_scores(filter_dims, rank_dims, result)
                ranked_vrs.append(
                    VerificationResult(
                        passed_filter=passed,
                        filter_scores=filter_dims,
                        rank_scores=rank_dims,
                        combined_score=combined,
                    )
                )
            else:
                ranked_vrs.append(
                    VerificationResult(
                        passed_filter=False,
                        filter_scores=filter_dims,
                        rank_scores=[],
                        combined_score=None,
                    )
                )

        # Sort by combined score descending (None scores last).
        ranked_vrs.sort(
            key=lambda vr: vr.combined_score.average_score() if vr.combined_score else -1.0,
            reverse=True,
        )
        return ranked_vrs


def _merge_scores(
    filter_dims: list[EvalDimension],
    rank_dims: list[EvalDimension],
    result: ExecutionResult,
) -> EvalScore:
    """Merge filter and rank dimensions into a single EvalScore."""
    all_dims = [*filter_dims, *rank_dims]
    total_cost = sum(d.cost_usd for d in all_dims)
    avg = _average(all_dims)
    cost_per_point = (result.cost_usd / avg) if avg > 0 else 0.0
    total_tokens = result.tokens_in + result.tokens_out
    token_eff = (avg / total_tokens) if total_tokens > 0 else 0.0

    return EvalScore(
        dimensions=all_dims,
        total_cost_usd=total_cost,
        cost_per_score_point=round(cost_per_point, 6),
        token_efficiency=round(token_eff, 8),
    )


def _average(dims: list[EvalDimension]) -> float:
    if not dims:
        return 0.0
    return sum(d.score for d in dims) / len(dims)
