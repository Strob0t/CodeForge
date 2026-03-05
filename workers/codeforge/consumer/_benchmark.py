"""Benchmark and GEMMAS evaluation handler mixins.

Routes benchmark requests to the appropriate runner based on benchmark_type:
- simple  → SimpleBenchmarkRunner (prompt → LLM → compare output)
- tool_use → ToolUseBenchmarkRunner (prompt + tools → LLM → output + tool calls)
- agent   → AgentBenchmarkRunner (full multi-turn agent loop)
"""

from __future__ import annotations

import asyncio
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

        # Wait for LiteLLM to be ready before running benchmarks.
        if not await _wait_for_litellm(self._llm, log):
            log.error("LiteLLM not available, aborting benchmark run")
            error_result = BenchmarkRunResult(
                run_id="",
                status="failed",
                error="LiteLLM proxy not available after health check retries",
            )
            if self._js is not None:
                await self._js.publish(
                    SUBJECT_BENCHMARK_RUN_RESULT,
                    error_result.model_dump_json().encode(),
                )
            await msg.ack()
            return

        run_id = ""
        try:
            req = BenchmarkRunRequest.model_validate_json(msg.data)
            run_id = req.run_id
            benchmark_type = req.benchmark_type or "simple"
            log = log.bind(run_id=run_id, benchmark_type=benchmark_type, model=req.model)

            if self._is_duplicate(f"bench-{req.run_id}"):
                log.warning("duplicate benchmark run, skipping")
                await msg.ack()
                return

            log.info("benchmark run started")

            start = time.monotonic()
            evaluators = _build_evaluators(req.evaluators, req.model)

            # Phase 28A: Split evaluators by stage for hybrid verification.
            pipeline = (
                _build_hybrid_pipeline(evaluators)
                if getattr(req, "hybrid_verification", False)
                else EvaluationPipeline(evaluators)
            )

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

        except Exception as exc:
            log.exception("benchmark run failed")
            if self._js is not None:
                error_result = BenchmarkRunResult(
                    run_id=run_id,
                    status="failed",
                    error=str(exc),
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

            if self._is_duplicate(f"gemmas-{request.plan_id}"):
                log.warning("duplicate GEMMAS evaluation, skipping")
                await msg.ack()
                return

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

        except Exception as exc:
            logger.exception("failed to process GEMMAS evaluation request", error=str(exc))
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


# --- Health check ---

# Max attempts and initial backoff for LiteLLM readiness probe.
_HEALTH_MAX_ATTEMPTS = 5
_HEALTH_BACKOFF_BASE = 2.0


async def _wait_for_litellm(
    llm: object,
    log: structlog.stdlib.BoundLogger,
) -> bool:
    """Poll LiteLLM health endpoint with exponential backoff.

    Returns True once healthy, False if all attempts are exhausted.
    """
    from codeforge.llm import LiteLLMClient

    if not isinstance(llm, LiteLLMClient):
        return True

    for attempt in range(_HEALTH_MAX_ATTEMPTS):
        if await llm.health():
            if attempt > 0:
                log.info("LiteLLM became healthy", attempt=attempt + 1)
            return True
        wait = _HEALTH_BACKOFF_BASE**attempt  # 1, 2, 4, 8, 16s
        log.warning(
            "LiteLLM not ready, retrying",
            attempt=attempt + 1,
            max_attempts=_HEALTH_MAX_ATTEMPTS,
            retry_in_seconds=wait,
        )
        await asyncio.sleep(wait)

    return False


# --- Module-level helpers used by the mixin ---


def _dataset_to_task_specs(dataset_path: str) -> list:
    """Load dataset YAML and convert BenchmarkTasks to TaskSpec objects."""
    from codeforge.evaluation.datasets import load_dataset
    from codeforge.evaluation.providers.base import TaskSpec

    dataset = load_dataset(dataset_path)
    return [
        TaskSpec(
            id=t.id,
            name=t.name,
            input=t.input,
            expected_output=t.expected_output,
            expected_tools=[],
            context=t.context,
            difficulty=t.difficulty,
        )
        for t in dataset.tasks
    ]


async def _run_simple_benchmark(req, llm, pipeline) -> list:
    """Run a simple prompt -> LLM -> compare benchmark."""
    from codeforge.evaluation.runners.simple import SimpleBenchmarkRunner

    runner = SimpleBenchmarkRunner(llm=llm, pipeline=pipeline, model=req.model)
    tasks = _dataset_to_task_specs(req.dataset_path)
    run_results = await runner.run_tasks(tasks)
    return [_convert_result(r) for r in run_results]


async def _run_tool_use_benchmark(req, llm, pipeline) -> list:
    """Run a tool-use benchmark with tools in task metadata."""
    from codeforge.evaluation.runners.tool_use import ToolUseBenchmarkRunner

    runner = ToolUseBenchmarkRunner(llm=llm, pipeline=pipeline, model=req.model)
    tasks = _dataset_to_task_specs(req.dataset_path)
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
    """Create evaluator instances from a list of evaluator/metric names.

    Accepts both evaluator names (llm_judge, functional_test, sparc, trajectory_verifier)
    and metric names (correctness, faithfulness, etc.) — metric names map to LLMJudgeEvaluator.
    """
    from codeforge.evaluation.evaluators.functional_test import FunctionalTestEvaluator
    from codeforge.evaluation.evaluators.llm_judge import LLMJudgeEvaluator
    from codeforge.evaluation.evaluators.sparc import SPARCEvaluator
    from codeforge.evaluation.evaluators.trajectory_verifier import TrajectoryVerifierEvaluator

    # Metric names that map to LLMJudgeEvaluator
    llm_judge_metrics = {"correctness", "faithfulness", "relevance", "coherence", "fluency"}

    evaluators = []
    collected_llm_metrics: list[str] = []

    for name in evaluator_names:
        if name == "llm_judge":
            collected_llm_metrics.append("correctness")
        elif name == "functional_test":
            evaluators.append(FunctionalTestEvaluator())
        elif name == "sparc":
            evaluators.append(SPARCEvaluator())
        elif name == "trajectory_verifier":
            evaluators.append(TrajectoryVerifierEvaluator(model=model))
        elif name in llm_judge_metrics:
            collected_llm_metrics.append(name)
        else:
            logger.warning("unknown evaluator, skipping", evaluator=name)

    # Create a single LLMJudgeEvaluator with all collected metrics,
    # using the same model as the benchmark run for the judge.
    if collected_llm_metrics:
        from codeforge.evaluation.litellm_judge import LiteLLMJudge

        judge = LiteLLMJudge(model=model)
        evaluators.append(LLMJudgeEvaluator(judge=judge, metrics=collected_llm_metrics))

    if not evaluators:
        from codeforge.evaluation.litellm_judge import LiteLLMJudge

        judge = LiteLLMJudge(model=model)
        evaluators.append(LLMJudgeEvaluator(judge=judge))

    return evaluators


def _build_hybrid_pipeline(evaluators: list) -> object:
    """Split evaluators by stage and build a HybridEvaluationPipeline (Phase 28A)."""
    from codeforge.evaluation.hybrid_pipeline import HybridEvaluationPipeline

    filter_evals = [e for e in evaluators if getattr(e, "stage", "rank") == "filter"]
    rank_evals = [e for e in evaluators if getattr(e, "stage", "rank") == "rank"]

    return HybridEvaluationPipeline(
        filter_evaluators=filter_evals,
        rank_evaluators=rank_evals,
    )


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
