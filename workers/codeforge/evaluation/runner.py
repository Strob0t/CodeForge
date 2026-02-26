"""Benchmark runner — executes evaluation tasks and collects results."""

from __future__ import annotations

import time
from typing import TYPE_CHECKING

import structlog

from codeforge.evaluation.datasets import BenchmarkDataset, BenchmarkTask, TaskResult
from codeforge.evaluation.litellm_judge import LiteLLMJudge
from codeforge.evaluation.metrics import (
    evaluate_answer_relevancy,
    evaluate_correctness,
    evaluate_faithfulness,
    evaluate_tool_correctness,
)

if TYPE_CHECKING:
    from codeforge.llm import ChatCompletionResponse, LiteLLMClient

logger = structlog.get_logger()


class BenchmarkRunner:
    """Runs a benchmark dataset against an LLM and collects scored results.

    The runner takes a dataset and an LLM client, executes each task by
    sending the input prompt to the model, then evaluates the output
    against the expected results using the configured metrics.
    """

    def __init__(
        self,
        llm: LiteLLMClient,
        model: str = "openai/gpt-4o",
        metrics: list[str] | None = None,
        judge: LiteLLMJudge | None = None,
    ) -> None:
        self._llm = llm
        self._model = model
        self._metrics = metrics or ["correctness"]
        self._judge = judge or LiteLLMJudge()

    async def run(self, dataset: BenchmarkDataset) -> list[TaskResult]:
        """Execute all tasks in the dataset and return scored results."""
        results: list[TaskResult] = []
        for task in dataset.tasks:
            log = logger.bind(task_id=task.id, task_name=task.name)
            log.info("running benchmark task")
            start = time.monotonic()

            actual_tools: list[dict[str, str]] = []
            try:
                response = await self._execute_task(task.input)
                output = response.content
                actual_tools = [{"name": tc.name, "args": tc.arguments} for tc in response.tool_calls]
            except Exception:
                log.exception("task execution failed")
                output = ""

            duration_ms = int((time.monotonic() - start) * 1000)

            scores = await self._evaluate(task, output, actual_tools)

            result = TaskResult(
                task_id=task.id,
                task_name=task.name,
                scores=scores,
                actual_output=output,
                expected_output=task.expected_output,
                tool_calls=actual_tools,
                duration_ms=duration_ms,
            )
            results.append(result)
            log.info("task completed", scores=scores, duration_ms=duration_ms)

        return results

    async def _execute_task(self, prompt: str) -> ChatCompletionResponse:
        """Send the task prompt to the LLM and return the full response."""
        messages = [{"role": "user", "content": prompt}]
        return await self._llm.chat_completion(model=self._model, messages=messages)

    async def _evaluate(
        self,
        task: BenchmarkTask,
        actual_output: str,
        actual_tools: list[dict[str, str]],
    ) -> dict[str, float]:
        """Run all configured metrics on a single task result."""
        scores: dict[str, float] = {}

        for metric_name in self._metrics:
            try:
                score = await self._run_metric(metric_name, task, actual_output, actual_tools)
                scores[metric_name] = score
            except Exception:
                logger.exception("metric evaluation failed", metric=metric_name, task_id=task.id)
                scores[metric_name] = 0.0

        return scores

    async def _run_metric(
        self,
        name: str,
        task: BenchmarkTask,
        output: str,
        actual_tools: list[dict[str, str]],
    ) -> float:
        """Dispatch to the appropriate metric evaluator."""
        if name == "correctness":
            return await evaluate_correctness(
                user_input=task.input,
                actual_output=output,
                expected_output=task.expected_output,
                judge=self._judge,
            )
        if name == "tool_correctness":
            return await evaluate_tool_correctness(
                user_input=task.input,
                actual_output=output,
                expected_tools=task.expected_tools,
                actual_tools=actual_tools,
                judge=self._judge,
            )
        if name == "faithfulness":
            return await evaluate_faithfulness(
                user_input=task.input,
                actual_output=output,
                retrieval_context=task.context,
                judge=self._judge,
            )
        if name == "answer_relevancy":
            return await evaluate_answer_relevancy(
                user_input=task.input,
                actual_output=output,
                judge=self._judge,
            )
        logger.warning("unknown metric, skipping", metric=name)
        return 0.0
