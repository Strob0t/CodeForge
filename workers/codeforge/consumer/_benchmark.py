"""Benchmark and GEMMAS evaluation handler mixins.

Routes benchmark requests to the appropriate runner based on benchmark_type:
- simple  → SimpleBenchmarkRunner (prompt → LLM → compare output)
- tool_use → ToolUseBenchmarkRunner (prompt + tools → LLM → output + tool calls)
- agent   → AgentBenchmarkRunner (full multi-turn agent loop)
"""

from __future__ import annotations

import time
from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._subjects import (
    HEADER_REQUEST_ID,
    SUBJECT_BENCHMARK_RUN_RESULT,
    SUBJECT_EVAL_GEMMAS_RESULT,
)
from codeforge.models import GemmasEvalRequest, GemmasEvalResult

if TYPE_CHECKING:
    from collections.abc import Callable

    import nats.aio.msg


logger = structlog.get_logger()


class BenchmarkHandlerMixin:
    """Handles benchmark.run.request and evaluation.gemmas.request messages."""

    async def _handle_benchmark_run(self, msg: nats.aio.msg.Msg) -> None:
        """Handle a benchmark run request (dev-mode only)."""
        import os

        from codeforge.evaluation.pipeline import EvaluationPipeline
        from codeforge.models import BenchmarkRunRequest, BenchmarkRunResult

        if os.getenv("APP_ENV") != "development":
            logger.warning("benchmark run ignored (not in dev mode)")
            await msg.ack()
            return

        request_id = (msg.headers or {}).get(HEADER_REQUEST_ID, "")
        log = logger.bind(request_id=request_id)

        try:
            req = BenchmarkRunRequest.model_validate_json(msg.data)
            benchmark_type = req.benchmark_type or "simple"
            log = log.bind(run_id=req.run_id, benchmark_type=benchmark_type, model=req.model)
            log.info("benchmark run started")

            start = time.monotonic()
            evaluators = _build_evaluators(req.evaluators, req.model)
            pipeline = EvaluationPipeline(evaluators)

            if benchmark_type == "tool_use":
                results = await _run_tool_use_benchmark(req, self._llm, pipeline)
            elif benchmark_type == "agent":
                results = await _run_agent_benchmark(req, self._llm, pipeline)
            else:
                results = await _run_simple_benchmark(req, self._llm, pipeline)

            elapsed_ms = int((time.monotonic() - start) * 1000)
            summary = _compute_summary(results, elapsed_ms)

            result = BenchmarkRunResult(
                run_id=req.run_id,
                status="completed",
                results=results,
                summary=summary,
            )

            if self._js is not None:
                await self._js.publish(
                    SUBJECT_BENCHMARK_RUN_RESULT,
                    result.model_dump_json().encode(),
                )

            await msg.ack()
            log.info(
                "benchmark run completed",
                task_count=len(results),
                elapsed_ms=elapsed_ms,
                avg_score=summary.get("avg_score", 0),
            )

        except Exception:
            log.exception("benchmark run failed")
            if self._js is not None:
                error_result = BenchmarkRunResult(
                    run_id=req.run_id if "req" in dir() else "",
                    status="failed",
                    error=str(log),
                )
                await self._js.publish(
                    SUBJECT_BENCHMARK_RUN_RESULT,
                    error_result.model_dump_json().encode(),
                )
            await msg.ack()

    async def _handle_gemmas_eval(self, msg: nats.aio.msg.Msg) -> None:
        """Process a GEMMAS evaluation request: compute IDS + UPR and publish result."""
        from codeforge.evaluation.executor import handle_gemmas_evaluation

        try:
            request = GemmasEvalRequest.model_validate_json(msg.data)
            log = logger.bind(plan_id=request.plan_id)
            log.info("received GEMMAS evaluation request", messages=len(request.messages))

            embed_fn = self._build_embed_fn()
            result_dict = await handle_gemmas_evaluation(
                messages=request.messages,
                plan_id=request.plan_id,
                embed_fn=embed_fn,
            )

            result = GemmasEvalResult(**result_dict)
            if self._js is not None:
                await self._js.publish(
                    SUBJECT_EVAL_GEMMAS_RESULT,
                    result.model_dump_json().encode(),
                )

            await msg.ack()
            log.info(
                "GEMMAS evaluation completed",
                ids=result.information_diversity_score,
                upr=result.unnecessary_path_ratio,
            )

        except Exception:
            logger.exception("failed to process GEMMAS evaluation request")
            await msg.ack()

    def _build_embed_fn(self) -> Callable[[list[str]], list[list[float]]] | None:
        """Build a sync embedding function using LiteLLM's /v1/embeddings endpoint."""
        import httpx

        url = self._litellm_url.rstrip("/") + "/v1/embeddings"
        headers: dict[str, str] = {"Content-Type": "application/json"}
        if self._litellm_key:
            headers["Authorization"] = f"Bearer {self._litellm_key}"

        def embed(texts: list[str]) -> list[list[float]]:
            with httpx.Client(timeout=30.0) as client:
                resp = client.post(
                    url,
                    json={"input": texts, "model": "text-embedding-3-small"},
                    headers=headers,
                )
                resp.raise_for_status()
                data = resp.json()
                return [item["embedding"] for item in data["data"]]

        return embed


# --- Module-level helpers used by the mixin ---


async def _run_simple_benchmark(req, llm, pipeline) -> list:
    """Run a simple prompt -> LLM -> compare benchmark."""
    from codeforge.evaluation.datasets import load_dataset
    from codeforge.evaluation.runners.simple import SimpleBenchmarkRunner

    runner = SimpleBenchmarkRunner(llm=llm, pipeline=pipeline, model=req.model)
    tasks = load_dataset(req.dataset_path)
    run_results = await runner.run_tasks(tasks)
    return [_convert_result(r) for r in run_results]


async def _run_tool_use_benchmark(req, llm, pipeline) -> list:
    """Run a tool-use benchmark with tools in task metadata."""
    from codeforge.evaluation.datasets import load_dataset
    from codeforge.evaluation.runners.tool_use import ToolUseBenchmarkRunner

    runner = ToolUseBenchmarkRunner(llm=llm, pipeline=pipeline, model=req.model)
    tasks = load_dataset(req.dataset_path)
    run_results = await runner.run_tasks(tasks)
    return [_convert_result(r) for r in run_results]


async def _run_agent_benchmark(req, llm, pipeline) -> list:
    """Run an agent benchmark using the full agent loop."""
    from codeforge.agent_loop import AgentLoopExecutor, LoopConfig
    from codeforge.evaluation.providers.codeforge_agent import CodeForgeAgentProvider
    from codeforge.evaluation.runners.agent import AgentBenchmarkRunner

    config = LoopConfig(model=req.model, max_cost=req.config.get("max_cost", 1.0) if hasattr(req, "config") else 1.0)
    executor = AgentLoopExecutor(llm=llm, tools=None, runtime=None)
    provider = CodeForgeAgentProvider(datasets_dir=req.dataset_path)
    tasks = await provider.load_tasks()
    runner = AgentBenchmarkRunner(executor=executor, pipeline=pipeline, loop_config=config)
    run_results = await runner.run_tasks(tasks)
    return [_convert_result(r) for r in run_results]


def _convert_result(r) -> object:
    """Convert internal RunResult to the NATS-serializable BenchmarkTaskResult."""
    from codeforge.models import BenchmarkTaskResult

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


def _build_evaluators(evaluator_names: list[str], model: str) -> list:
    """Create evaluator instances from a list of evaluator names."""
    from codeforge.evaluation.evaluators.functional_test import FunctionalTestEvaluator
    from codeforge.evaluation.evaluators.llm_judge import LLMJudgeEvaluator
    from codeforge.evaluation.evaluators.sparc import SPARCEvaluator

    evaluators = []
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


def _compute_summary(results: list, elapsed_ms: int) -> dict[str, object]:
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
