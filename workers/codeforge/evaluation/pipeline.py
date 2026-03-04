"""Evaluation pipeline — composes multiple evaluators into a unified scoring flow.

The pipeline takes a list of evaluator instances, runs all of them on each
task result, merges their EvalDimension scores into a single EvalScore,
and tracks per-evaluator cost separately.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from codeforge.evaluation.providers.base import EvalDimension, EvalScore, ExecutionResult, TaskSpec

if TYPE_CHECKING:
    from codeforge.evaluation.evaluators.base import Evaluator

logger = structlog.get_logger()


class EvaluationPipeline:
    """Orchestrates multiple evaluators and merges results.

    Usage::

        pipeline = EvaluationPipeline([
            LLMJudgeEvaluator(metrics=["correctness"]),
            FunctionalTestEvaluator(),
            SPARCEvaluator(),
        ])
        score = await pipeline.evaluate(task, result)
    """

    def __init__(self, evaluators: list[Evaluator]) -> None:
        if not evaluators:
            raise ValueError("at least one evaluator is required")
        self._evaluators = evaluators

    @property
    def evaluator_names(self) -> list[str]:
        """Return names of all configured evaluators."""
        return [e.name for e in self._evaluators]

    async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> EvalScore:
        """Run all evaluators and merge into a unified EvalScore."""
        all_dimensions: list[EvalDimension] = []
        total_eval_cost = 0.0

        for evaluator in self._evaluators:
            log = logger.bind(evaluator=evaluator.name, task_id=task.id)
            try:
                dimensions = await evaluator.evaluate(task, result)
                for dim in dimensions:
                    total_eval_cost += dim.cost_usd
                all_dimensions.extend(dimensions)
                log.debug("evaluator completed", dimensions=len(dimensions))
            except Exception as exc:
                log.exception("evaluator failed", error=str(exc))
                all_dimensions.append(
                    EvalDimension(
                        name=f"{evaluator.name}_error",
                        score=0.0,
                        details={"error": "evaluator raised an exception"},
                    )
                )

        # Compute derived metrics.
        avg_score = _average_score(all_dimensions)
        cost_per_point = (result.cost_usd / avg_score) if avg_score > 0 else 0.0
        total_tokens = result.tokens_in + result.tokens_out
        token_efficiency = (avg_score / total_tokens) if total_tokens > 0 else 0.0

        return EvalScore(
            dimensions=all_dimensions,
            total_cost_usd=total_eval_cost,
            cost_per_score_point=round(cost_per_point, 6),
            token_efficiency=round(token_efficiency, 8),
        )

    async def evaluate_batch(
        self,
        tasks_and_results: list[tuple[TaskSpec, ExecutionResult]],
    ) -> list[EvalScore]:
        """Evaluate multiple task/result pairs sequentially."""
        scores: list[EvalScore] = []
        for task, result in tasks_and_results:
            score = await self.evaluate(task, result)
            scores.append(score)
        return scores


def _average_score(dimensions: list[EvalDimension]) -> float:
    """Compute mean score across dimensions."""
    if not dimensions:
        return 0.0
    return sum(d.score for d in dimensions) / len(dimensions)
