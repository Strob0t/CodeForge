"""Simple benchmark runner — prompt -> LLM -> output comparison."""

from __future__ import annotations

import time
from typing import TYPE_CHECKING

import structlog

from codeforge.evaluation.providers.base import ExecutionResult, TaskSpec
from codeforge.evaluation.runners._base import BaseBenchmarkRunner, RunResult

if TYPE_CHECKING:
    from codeforge.evaluation.pipeline import EvaluationPipeline
    from codeforge.llm import LiteLLMClient

logger = structlog.get_logger(__name__)


class SimpleBenchmarkRunner(BaseBenchmarkRunner):
    """Runs simple prompt -> LLM -> compare output benchmarks."""

    def __init__(
        self,
        llm: LiteLLMClient,
        pipeline: EvaluationPipeline,
        model: str = "openai/gpt-4o",
    ) -> None:
        self._llm = llm
        self._pipeline = pipeline
        self._model = model

    async def run_task(self, task: TaskSpec) -> RunResult:
        """Run a single task: send prompt to LLM, evaluate output."""
        log = logger.bind(task_id=task.id, task_name=task.name)
        log.info("running simple benchmark task")

        start = time.monotonic()
        try:
            response = await self._llm.chat_completion(
                model=self._model,
                messages=[{"role": "user", "content": task.input}],
            )
            actual_output = response.content
            tokens_in = response.tokens_in
            tokens_out = response.tokens_out
            cost_usd = response.cost_usd
        except Exception as exc:
            log.error("LLM call failed", error=str(exc))
            actual_output = f"ERROR: {exc}"
            tokens_in = 0
            tokens_out = 0
            cost_usd = 0.0

        duration_ms = int((time.monotonic() - start) * 1000)

        execution = ExecutionResult(
            actual_output=actual_output,
            tokens_in=tokens_in,
            tokens_out=tokens_out,
            cost_usd=cost_usd,
            duration_ms=duration_ms,
        )

        eval_score = await self._pipeline.evaluate(task, execution)
        log.info(
            "task completed",
            task_id=task.id,
            task_name=task.name,
            duration_ms=duration_ms,
            avg_score=eval_score.average_score() if eval_score else 0,
        )

        return RunResult(task=task, execution=execution, eval_score=eval_score)
