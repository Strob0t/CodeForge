"""LLM-as-Judge evaluator — wraps existing DeepEval metrics.

Produces EvalDimension scores for: correctness, tool_correctness,
faithfulness, and answer_relevancy via the LiteLLM proxy.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from codeforge.evaluation.evaluators.prompt_compressor import compress_for_context
from codeforge.evaluation.metrics import (
    evaluate_answer_relevancy,
    evaluate_correctness,
    evaluate_faithfulness,
    evaluate_tool_correctness,
)
from codeforge.evaluation.providers.base import EvalDimension, ExecutionResult, TaskSpec

if TYPE_CHECKING:
    from codeforge.evaluation.litellm_judge import LiteLLMJudge

logger = structlog.get_logger()

# Supported metric names for this evaluator.
SUPPORTED_METRICS = frozenset({"correctness", "tool_correctness", "faithfulness", "answer_relevancy"})

# Max character budgets for prompt compression (prevents context overflow on local models).
_MAX_INPUT_CHARS = 4000
_MAX_OUTPUT_CHARS = 4000
_MAX_EXPECTED_CHARS = 2000


class LLMJudgeEvaluator:
    """Evaluator that uses DeepEval G-Eval and other LLM-based metrics."""

    def __init__(
        self,
        judge: LiteLLMJudge | None = None,
        metrics: list[str] | None = None,
    ) -> None:
        self._judge = judge
        self._metrics = metrics or ["correctness"]

    @property
    def name(self) -> str:
        return "llm_judge"

    @property
    def stage(self) -> str:
        return "rank"

    async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
        """Run configured LLM-judge metrics on the task result."""
        if not result.actual_output or not result.actual_output.strip():
            return [
                EvalDimension(name=m, score=0.0, details={"error": "empty actual_output"})
                for m in self._metrics
                if m in SUPPORTED_METRICS
            ]

        # Compress inputs to avoid context overflow on local models.
        compressed_task = task.model_copy(
            update={
                "input": compress_for_context(task.input, _MAX_INPUT_CHARS),
                "expected_output": compress_for_context(task.expected_output, _MAX_EXPECTED_CHARS),
            }
        )
        compressed_result = result.model_copy(
            update={
                "actual_output": compress_for_context(result.actual_output, _MAX_OUTPUT_CHARS),
            }
        )

        dimensions: list[EvalDimension] = []
        for metric_name in self._metrics:
            if metric_name not in SUPPORTED_METRICS:
                logger.warning("unsupported llm_judge metric", metric=metric_name)
                continue
            try:
                score = await self._run_metric(metric_name, compressed_task, compressed_result)
                dimensions.append(EvalDimension(name=metric_name, score=score))
            except Exception as exc:
                error_msg = str(exc)
                is_context_overflow = "context" in error_msg.lower() or "400" in error_msg
                logger.exception("llm_judge metric failed", metric=metric_name, task_id=task.id, error=error_msg)
                dimensions.append(
                    EvalDimension(
                        name=metric_name,
                        score=0.0,
                        details={
                            "error": "context_overflow" if is_context_overflow else "evaluation_failed",
                            "error_message": error_msg[:200],
                        },
                    )
                )
        return dimensions

    async def _run_metric(self, name: str, task: TaskSpec, result: ExecutionResult) -> float:
        """Dispatch to the appropriate DeepEval metric wrapper."""
        if name == "correctness":
            return await evaluate_correctness(
                user_input=task.input,
                actual_output=result.actual_output,
                expected_output=task.expected_output,
                judge=self._judge,
            )
        if name == "tool_correctness":
            expected_tools = [{"name": t.name, "args": t.args} for t in task.expected_tools]
            actual_tools = [{"name": t.name, "args": t.args} for t in result.tool_calls]
            return await evaluate_tool_correctness(
                user_input=task.input,
                actual_output=result.actual_output,
                expected_tools=expected_tools,
                actual_tools=actual_tools,
                judge=self._judge,
            )
        if name == "faithfulness":
            return await evaluate_faithfulness(
                user_input=task.input,
                actual_output=result.actual_output,
                retrieval_context=task.context,
                judge=self._judge,
            )
        if name == "answer_relevancy":
            return await evaluate_answer_relevancy(
                user_input=task.input,
                actual_output=result.actual_output,
                judge=self._judge,
            )
        return 0.0
