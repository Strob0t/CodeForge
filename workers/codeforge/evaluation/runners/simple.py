"""Simple benchmark runner — prompt -> LLM -> output comparison."""

from __future__ import annotations

import time
from dataclasses import dataclass
from typing import TYPE_CHECKING

import structlog

from codeforge.evaluation.providers.base import ExecutionResult, TaskSpec

if TYPE_CHECKING:
    from codeforge.evaluation.pipeline import EvaluationPipeline
    from codeforge.evaluation.providers.base import EvalScore
    from codeforge.llm import LiteLLMClient

logger = structlog.get_logger(__name__)


@dataclass(slots=True)
class RunResult:
    """Holds task, execution output, and evaluation score together."""

    task: TaskSpec
    execution: ExecutionResult
    eval_score: EvalScore | None = None


class SimpleBenchmarkRunner:
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

    async def run_tasks(self, tasks: list[TaskSpec]) -> list[RunResult]:
        """Run all tasks sequentially and return results."""
        results: list[RunResult] = []
        for task in tasks:
            result = await self.run_task(task)
            results.append(result)
        return results

    async def run_task(self, task: TaskSpec) -> RunResult:
        """Run a single task: send prompt to LLM, evaluate output."""
        log = logger.bind(task_id=task.id, task_name=task.name)
        log.info("running simple benchmark task")

        start = time.monotonic()
        try:
            response = await self._llm.chat(
                model=self._model,
                messages=[{"role": "user", "content": task.input}],
            )
            actual_output = response.content
            tokens_in = response.usage.prompt_tokens if response.usage else 0
            tokens_out = response.usage.completion_tokens if response.usage else 0
            cost_usd = response.cost if hasattr(response, "cost") else 0.0
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
