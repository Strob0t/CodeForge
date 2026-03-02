"""NATS consumer handler for benchmark evaluation runs.

Routes benchmark requests to the appropriate runner based on benchmark_type:
- simple  → SimpleBenchmarkRunner (prompt → LLM → compare output)
- tool_use → ToolUseBenchmarkRunner (prompt + tools → LLM → output + tool calls)
- agent   → (Phase 26D — falls back to simple with warning)
"""

from __future__ import annotations

import json
import time
import traceback
from typing import TYPE_CHECKING

import structlog

from codeforge.evaluation.evaluators.functional_test import FunctionalTestEvaluator
from codeforge.evaluation.evaluators.llm_judge import LLMJudgeEvaluator
from codeforge.evaluation.evaluators.sparc import SPARCEvaluator
from codeforge.evaluation.pipeline import EvaluationPipeline
from codeforge.evaluation.runners.simple import RunResult, SimpleBenchmarkRunner
from codeforge.evaluation.runners.tool_use import ToolUseBenchmarkRunner
from codeforge.models import BenchmarkRunRequest, BenchmarkRunResult, BenchmarkTaskResult

if TYPE_CHECKING:
    from codeforge.evaluation.evaluators.base import Evaluator
    from codeforge.llm import LiteLLMClient

logger = structlog.get_logger(__name__)


async def handle_benchmark_run(
    msg_data: bytes,
    llm: LiteLLMClient,
) -> None:
    """Handle an incoming benchmark run request from NATS."""
    raw = json.loads(msg_data)
    req = BenchmarkRunRequest(**raw)
    log = logger.bind(run_id=req.run_id, benchmark_type=req.benchmark_type)
    log.info("benchmark run started")

    start = time.monotonic()
    try:
        benchmark_type = req.benchmark_type or "simple"
        evaluators = _build_evaluators(req.evaluators, req.model)
        pipeline = EvaluationPipeline(evaluators)

        if benchmark_type == "tool_use":
            results = await _run_tool_use_benchmark(req, llm, pipeline)
        elif benchmark_type == "agent":
            log.warning("agent benchmark not yet implemented, falling back to simple")
            results = await _run_simple_benchmark(req, llm, pipeline)
        else:
            results = await _run_simple_benchmark(req, llm, pipeline)

        elapsed_ms = int((time.monotonic() - start) * 1000)
        summary = _compute_summary(results, elapsed_ms)

        run_result = BenchmarkRunResult(
            run_id=req.run_id,
            status="completed",
            results=results,
            summary=summary,
        )
        log.info(
            "benchmark run completed",
            task_count=len(results),
            elapsed_ms=elapsed_ms,
            avg_score=summary.get("avg_score", 0),
        )
    except Exception:
        log.error("benchmark run failed", exc=traceback.format_exc())
        run_result = BenchmarkRunResult(
            run_id=req.run_id,
            status="failed",
            results=[],
            summary={"error": traceback.format_exc()},
        )

    return run_result


async def _run_simple_benchmark(
    req: BenchmarkRunRequest,
    llm: LiteLLMClient,
    pipeline: EvaluationPipeline,
) -> list[BenchmarkTaskResult]:
    """Run a simple prompt -> LLM -> compare benchmark."""
    from codeforge.evaluation.datasets import load_dataset

    runner = SimpleBenchmarkRunner(llm=llm, pipeline=pipeline, model=req.model)
    tasks = load_dataset(req.dataset_path)
    run_results: list[RunResult] = await runner.run_tasks(tasks)
    return [_convert_result(r) for r in run_results]


async def _run_tool_use_benchmark(
    req: BenchmarkRunRequest,
    llm: LiteLLMClient,
    pipeline: EvaluationPipeline,
) -> list[BenchmarkTaskResult]:
    """Run a tool-use benchmark with tools in task metadata."""
    from codeforge.evaluation.datasets import load_dataset

    runner = ToolUseBenchmarkRunner(llm=llm, pipeline=pipeline, model=req.model)
    tasks = load_dataset(req.dataset_path)
    run_results: list[RunResult] = await runner.run_tasks(tasks)
    return [_convert_result(r) for r in run_results]


def _convert_result(r: RunResult) -> BenchmarkTaskResult:
    """Convert internal RunResult to the NATS-serializable BenchmarkTaskResult."""
    scores: dict[str, float] = {}
    evaluator_scores: dict[str, dict[str, float]] = {}
    if r.eval_score:
        for dim in r.eval_score.dimensions:
            scores[dim.name] = dim.score
            parts = dim.name.split(".", 1)
            if len(parts) == 2:
                evaluator_scores.setdefault(parts[0], {})[parts[1]] = dim.score
            else:
                evaluator_scores.setdefault("default", {})[dim.name] = dim.score

    return BenchmarkTaskResult(
        task_id=r.task.id,
        task_name=r.task.name,
        scores=scores,
        actual_output=r.execution.actual_output,
        expected_output=r.task.expected_output,
        tool_calls=[{"name": tc.name, "args": tc.args} for tc in r.execution.tool_calls],
        cost_usd=r.execution.cost_usd,
        tokens_in=r.execution.tokens_in,
        tokens_out=r.execution.tokens_out,
        duration_ms=r.execution.duration_ms,
        evaluator_scores=evaluator_scores,
        files_changed=r.execution.files_changed,
        functional_test_output=r.execution.test_output,
    )


def _build_evaluators(evaluator_names: list[str], model: str) -> list[Evaluator]:
    """Create evaluator instances from a list of evaluator names."""
    evaluators: list[Evaluator] = []
    for name in evaluator_names:
        if name == "llm_judge":
            evaluators.append(LLMJudgeEvaluator(model=model))
        elif name == "functional_test":
            evaluators.append(FunctionalTestEvaluator())
        elif name == "sparc":
            evaluators.append(SPARCEvaluator())
        else:
            logger.warning("unknown evaluator, skipping", evaluator=name)

    if not evaluators:
        evaluators.append(LLMJudgeEvaluator(model=model))

    return evaluators


def _compute_summary(results: list[BenchmarkTaskResult], elapsed_ms: int) -> dict[str, object]:
    """Compute summary statistics for a benchmark run."""
    if not results:
        return {"task_count": 0, "elapsed_ms": elapsed_ms}

    all_scores = [s for r in results for s in r.scores.values()]
    avg_score = sum(all_scores) / len(all_scores) if all_scores else 0.0
    total_cost = sum(r.cost_usd for r in results)
    total_tokens_in = sum(r.tokens_in for r in results)
    total_tokens_out = sum(r.tokens_out for r in results)

    return {
        "task_count": len(results),
        "avg_score": round(avg_score, 4),
        "total_cost_usd": round(total_cost, 6),
        "total_tokens_in": total_tokens_in,
        "total_tokens_out": total_tokens_out,
        "elapsed_ms": elapsed_ms,
    }
