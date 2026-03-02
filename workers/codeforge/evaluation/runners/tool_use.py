"""Tool-use benchmark runner — prompt + tools -> LLM -> output + tool calls."""

from __future__ import annotations

import json
import time
from typing import TYPE_CHECKING

import structlog

from codeforge.evaluation.providers.base import ExecutionResult, TaskSpec, ToolCall
from codeforge.evaluation.runners.simple import RunResult

if TYPE_CHECKING:
    from codeforge.evaluation.pipeline import EvaluationPipeline
    from codeforge.llm import LiteLLMClient

logger = structlog.get_logger(__name__)


def _parse_tools(task: TaskSpec) -> list[dict]:
    """Extract OpenAI-compatible tool definitions from task metadata."""
    tools_json = task.metadata.get("tools", "")
    if not tools_json:
        return []
    try:
        tools = json.loads(tools_json)
        if not isinstance(tools, list):
            logger.warning("tools metadata is not a list", task_id=task.id)
            return []
        return tools
    except json.JSONDecodeError:
        logger.warning("invalid tools JSON in task metadata", task_id=task.id)
        return []


class ToolUseBenchmarkRunner:
    """Runs tool-use benchmarks: prompt + tools -> LLM -> output + tool calls."""

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
        """Run a single tool-use task."""
        log = logger.bind(task_id=task.id, task_name=task.name)
        log.info("running tool-use benchmark task")

        tools = _parse_tools(task)
        start = time.monotonic()

        try:
            kwargs: dict = {
                "model": self._model,
                "messages": [{"role": "user", "content": task.input}],
            }
            if tools:
                kwargs["tools"] = tools

            response = await self._llm.chat(**kwargs)
            actual_output = response.content or ""
            tokens_in = response.usage.prompt_tokens if response.usage else 0
            tokens_out = response.usage.completion_tokens if response.usage else 0
            cost_usd = response.cost if hasattr(response, "cost") else 0.0

            # Extract tool calls from response
            tool_calls: list[ToolCall] = []
            if hasattr(response, "tool_calls") and response.tool_calls:
                for tc in response.tool_calls:
                    tool_calls.append(
                        ToolCall(
                            name=tc.function.name if hasattr(tc, "function") else str(tc.get("name", "")),
                            args=tc.function.arguments if hasattr(tc, "function") else json.dumps(tc.get("args", {})),
                        )
                    )
        except Exception as exc:
            log.error("LLM call failed", error=str(exc))
            actual_output = f"ERROR: {exc}"
            tokens_in = 0
            tokens_out = 0
            cost_usd = 0.0
            tool_calls = []

        duration_ms = int((time.monotonic() - start) * 1000)

        execution = ExecutionResult(
            actual_output=actual_output,
            tool_calls=tool_calls,
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
            tool_calls=len(tool_calls),
            avg_score=eval_score.average_score() if eval_score else 0,
        )

        return RunResult(task=task, execution=execution, eval_score=eval_score)
