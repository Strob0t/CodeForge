"""Benchmark and GEMMAS evaluation handler mixins.

Routes benchmark requests to the appropriate runner based on benchmark_type:
- simple  → SimpleBenchmarkRunner (prompt → LLM → compare output)
- tool_use → ToolUseBenchmarkRunner (prompt + tools → LLM → output + tool calls)
- agent   → AgentBenchmarkRunner (full multi-turn agent loop)
"""

from __future__ import annotations

import asyncio
import contextlib
import tempfile
import time
from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._subjects import (
    HEADER_REQUEST_ID,
    SUBJECT_BENCHMARK_RUN_RESULT,
    SUBJECT_BENCHMARK_TASK_PROGRESS,
    SUBJECT_BENCHMARK_TASK_STARTED,
    SUBJECT_EVAL_GEMMAS_RESULT,
)
from codeforge.models import GemmasEvalRequest, GemmasEvalResult

if TYPE_CHECKING:
    from collections.abc import Callable

    import nats.aio.msg


logger = structlog.get_logger()


async def _fetch_available_models() -> list[str]:
    """Fetch model IDs from LiteLLM /v1/models endpoint."""
    import os

    import httpx

    litellm_url = os.environ.get("LITELLM_BASE_URL", "http://localhost:4000")
    api_key = os.environ.get("LITELLM_MASTER_KEY", "sk-codeforge-dev")
    headers = {"Authorization": f"Bearer {api_key}"}
    try:
        async with httpx.AsyncClient(timeout=5.0) as client:
            resp = await client.get(f"{litellm_url}/v1/models", headers=headers)
        if resp.status_code != 200:
            return []
        data = resp.json()
        return [m.get("id", "") for m in data.get("data", []) if m.get("id")]
    except Exception:
        return []


async def _validate_model_exists(model: str, available_models: list[str] | None = None) -> None:
    """Validate model exists in LiteLLM. Raises ValueError if not found.

    Skips for "auto" or if model list is empty (LiteLLM unreachable).
    """
    if model == "auto":
        return
    if available_models is None:
        available_models = await _fetch_available_models()
    if not available_models:
        return  # Can't reach LiteLLM — skip validation, don't block
    if model not in available_models:
        raise ValueError(
            f"model {model!r} not available in LiteLLM. Available: {', '.join(sorted(available_models)[:10])}"
        )


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
            await _validate_model_exists(req.model)
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
            pipeline = _build_hybrid_pipeline(evaluators) if req.hybrid_verification else EvaluationPipeline(evaluators)

            # Auto-routing: wrap LLM with routing for per-task model selection.
            effective_llm = await self._resolve_effective_llm(req, log)

            # Build per-task progress callbacks for real-time WS updates.
            on_start, on_complete = _build_progress_callbacks(self._js, req.run_id)

            if benchmark_type == "tool_use":
                results = await _run_tool_use_benchmark(req, effective_llm, pipeline, on_start, on_complete)
            elif benchmark_type == "agent":
                results = await _run_agent_benchmark(req, effective_llm, pipeline, on_start, on_complete)
            else:
                results = await _run_simple_benchmark(req, effective_llm, pipeline, on_start, on_complete)

            # Annotate results with routing metadata.
            if req.model == "auto" and hasattr(effective_llm, "routing_log"):
                _annotate_routing(results, effective_llm.routing_log)

            elapsed_ms = int((time.monotonic() - start) * 1000)
            summary = _compute_summary(results, elapsed_ms)

            if req.model == "auto":
                summary["routing_report"] = _compute_routing_report(results)

            result = BenchmarkRunResult(
                run_id=req.run_id,
                status="completed",
                results=results,
                summary=summary,
                total_cost=summary.get("total_cost_usd", 0.0),
                total_tokens=summary.get("total_tokens_in", 0) + summary.get("total_tokens_out", 0),
                total_duration_ms=summary.get("elapsed_ms", 0),
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

    async def _resolve_effective_llm(self, req: object, log: structlog.BoundLogger) -> object:
        """Resolve the effective LLM client, wrapping with router for auto mode."""
        if req.model != "auto":
            return self._llm
        try:
            router = await self._get_hybrid_router()
            if router is not None:
                log.info("auto-routing enabled for benchmark run")
                return _RoutingLLMWrapper(self._llm, router)
        except Exception:
            log.warning("HybridRouter initialization failed", exc_info=True)

        raise ValueError(
            "model='auto' requires intelligent routing, but routing is not enabled. "
            "Set CODEFORGE_ROUTING_ENABLED=true or specify an explicit model name."
        )

    async def _handle_gemmas_eval(self, msg: nats.aio.msg.Msg) -> None:
        """Process a GEMMAS scoring request: compute IDS + UPR and publish result."""
        await self._handle_request(
            msg=msg,
            request_model=GemmasEvalRequest,
            dedup_key=lambda r: f"gemmas-{r.plan_id}",
            handler=self._do_gemmas_scoring,
            result_subject=SUBJECT_EVAL_GEMMAS_RESULT,
            log_context=lambda r: {"plan_id": r.plan_id},
        )

    async def _do_gemmas_scoring(
        self, request: GemmasEvalRequest, log: structlog.BoundLogger
    ) -> GemmasEvalResult | None:
        """Business logic for GEMMAS scoring. Catches errors to ensure ack (not nak)."""
        try:
            from codeforge.evaluation.executor import handle_gemmas_evaluation

            log.info("received GEMMAS scoring request", messages=len(request.messages))

            embed_fn = self._build_embed_fn()
            result_dict = await handle_gemmas_evaluation(
                messages=[m.model_dump() for m in request.messages],
                plan_id=request.plan_id,
                embed_fn=embed_fn,
            )

            result = GemmasEvalResult(**result_dict)
            log.info(
                "GEMMAS scoring completed",
                ids=result.information_diversity_score,
                upr=result.unnecessary_path_ratio,
            )
            return result
        except Exception as exc:
            # Log but do not re-raise: GEMMAS scoring is best-effort, ack to prevent
            # infinite redelivery.
            logger.exception("failed to process GEMMAS scoring request", error=str(exc))
            return None

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
    from codeforge.evaluation.providers.base import TaskSpec, ToolCall

    dataset = load_dataset(dataset_path)
    return [
        TaskSpec(
            id=t.id,
            name=t.name,
            input=t.input,
            expected_output=t.expected_output,
            expected_tools=[ToolCall(name=tc.get("name", ""), args=tc.get("args", "")) for tc in t.expected_tools],
            context=t.context,
            difficulty=t.difficulty,
        )
        for t in dataset.tasks
    ]


async def _load_tasks_for_run(req) -> list:
    """Load tasks from provider registry or legacy YAML dataset."""
    from codeforge.evaluation.providers import get_provider
    from codeforge.evaluation.task_filter import apply_task_filters

    if req.provider_name:
        provider_cls = get_provider(req.provider_name)
        try:
            provider = provider_cls(config=req.provider_config)
        except TypeError:
            # Built-in providers (codeforge_simple/agent/tool_use) accept
            # dataset_path instead of config.
            provider = provider_cls(dataset_path=req.dataset_path)
        tasks = await provider.load_tasks()
        return apply_task_filters(tasks, req.provider_config)

    return _dataset_to_task_specs(req.dataset_path)


async def _run_simple_benchmark(req, llm, pipeline, on_start=None, on_complete=None) -> list:
    """Run a simple prompt -> LLM -> compare benchmark."""
    from codeforge.evaluation.runners.simple import SimpleBenchmarkRunner

    runner = SimpleBenchmarkRunner(llm=llm, pipeline=pipeline, model=req.model)
    tasks = await _load_tasks_for_run(req)
    return await _run_with_optional_rollout(runner, tasks, req, pipeline, on_start, on_complete)


async def _run_tool_use_benchmark(req, llm, pipeline, on_start=None, on_complete=None) -> list:
    """Run a tool-use benchmark with tools in task metadata."""
    from codeforge.evaluation.runners.tool_use import ToolUseBenchmarkRunner

    runner = ToolUseBenchmarkRunner(llm=llm, pipeline=pipeline, model=req.model)
    tasks = await _load_tasks_for_run(req)
    return await _run_with_optional_rollout(runner, tasks, req, pipeline, on_start, on_complete)


class _BenchmarkRuntime:
    """Lightweight runtime stub for benchmark runs (no NATS dependency).

    Auto-approves all tool calls and silently discards output/trajectory
    events that would normally be published to the Go control plane.
    """

    def __init__(self, run_id: str = "benchmark") -> None:
        self.run_id = run_id
        self.project_id = ""
        self.is_cancelled = False

    async def send_output(self, _text: str) -> None:
        pass

    async def request_tool_call(self, **_kwargs: object) -> object:
        from codeforge.models import ToolCallDecision

        return ToolCallDecision(call_id="bench", decision="allow")

    async def report_tool_result(self, **_kwargs: object) -> None:
        pass

    async def publish_trajectory_event(self, **_kwargs: object) -> None:
        pass


async def _run_agent_benchmark(req, llm, pipeline, on_start=None, on_complete=None) -> list:
    """Run an agent benchmark using the full agent loop."""
    from codeforge.agent_loop import AgentLoopExecutor, LoopConfig
    from codeforge.evaluation.runners.agent import AgentBenchmarkRunner
    from codeforge.tools import build_default_registry

    if req.provider_name:
        tasks = await _load_tasks_for_run(req)
    else:
        from codeforge.evaluation.providers.codeforge_agent import CodeForgeAgentProvider

        provider = CodeForgeAgentProvider(datasets_dir=req.dataset_path)
        tasks = await provider.load_tasks()

    config = LoopConfig(
        model=req.model,
        max_cost=req.provider_config.get("max_cost", 1.0) if req.provider_config else 1.0,
    )
    registry = build_default_registry()
    runtime = _BenchmarkRuntime(run_id=req.run_id)
    executor = AgentLoopExecutor(
        llm=llm,
        tool_registry=registry,
        runtime=runtime,
        workspace_path=tempfile.gettempdir(),
    )
    runner = AgentBenchmarkRunner(executor=executor, pipeline=pipeline, loop_config=config)
    return await _run_with_optional_rollout(runner, tasks, req, pipeline, on_start, on_complete)


async def _run_with_optional_rollout(runner, tasks, req, pipeline, on_start=None, on_complete=None) -> list:
    """Wrap runner in MultiRolloutRunner when rollout_count > 1."""
    from codeforge.models import BenchmarkRunRequest

    if isinstance(req, BenchmarkRunRequest) and req.rollout_count > 1:
        from codeforge.evaluation.runners.multi_rollout import MultiRolloutRunner

        hybrid_pipeline = pipeline if req.hybrid_verification else None
        multi_runner = MultiRolloutRunner(
            inner_runner=runner,
            hybrid_pipeline=hybrid_pipeline,
            rollout_count=req.rollout_count,
            strategy=req.rollout_strategy,
        )
        results: list = []
        total = len(tasks)
        for i, task in enumerate(tasks):
            if on_start is not None:
                await on_start(task, i, total)
            outcomes = await multi_runner.run_task(task)
            converted = [_convert_rollout_outcome(task, outcome, req.rollout_count) for outcome in outcomes]
            results.extend(converted)
            if on_complete is not None and converted:
                await on_complete(task, converted[0], i, total)
        return results

    run_results = await runner.run_tasks(tasks, on_task_start=on_start, on_task_complete=on_complete)
    return [_convert_result(r) for r in run_results]


# ---------------------------------------------------------------------------
# Score key normalization: map raw EvalDimension names to parent metric keys.
# ---------------------------------------------------------------------------

_DIMENSION_TO_METRIC: dict[str, str] = {
    # LLMJudgeEvaluator dimensions -> "llm_judge"
    "correctness": "llm_judge",
    "faithfulness": "llm_judge",
    "answer_relevancy": "llm_judge",
    "tool_correctness": "llm_judge",
    # SPARCEvaluator dimensions -> "sparc"
    "sparc_steps": "sparc",
    "sparc_time": "sparc",
    "sparc_cost": "sparc",
    "sparc_complexity": "sparc",
    "sparc_code_quality": "sparc",
    "sparc_security": "sparc",
    # TrajectoryVerifierEvaluator dimensions -> "trajectory_verifier"
    "trajectory_solution_quality": "trajectory_verifier",
    "trajectory_approach_efficiency": "trajectory_verifier",
    "trajectory_code_quality": "trajectory_verifier",
    "trajectory_error_recovery": "trajectory_verifier",
    "trajectory_completeness": "trajectory_verifier",
    "trajectory_quality": "trajectory_verifier",  # error fallback
    # FunctionalTestEvaluator -> identity (already matches)
    "functional_test": "functional_test",
}


def _aggregate_metric_scores(scores: dict[str, float]) -> None:
    """Add averaged parent-metric keys to scores dict in-place.

    For each group of dimension keys that map to the same metric name,
    compute the average and store it under the metric name.  Raw dimension
    keys are preserved.  If the metric name already exists as a raw key
    (e.g. ``functional_test``), it is not overwritten.
    """
    groups: dict[str, list[float]] = {}
    for dim_name, value in scores.items():
        metric = _DIMENSION_TO_METRIC.get(dim_name)
        if metric is not None:
            groups.setdefault(metric, []).append(value)

    for metric, values in groups.items():
        # Skip identity mappings where the metric key already exists as a raw dimension.
        if metric in scores:
            continue
        scores[metric] = sum(values) / len(values)


def _convert_rollout_outcome(task, outcome, rollout_count) -> object:
    """Convert a RolloutOutcome to a BenchmarkTaskResult with rollout fields."""
    from codeforge.models import BenchmarkTaskResult

    scores: dict[str, float] = {}
    if outcome.eval_score:
        for dim in outcome.eval_score.dimensions:
            scores[dim.name] = dim.score
    _aggregate_metric_scores(scores)

    return BenchmarkTaskResult(
        task_id=task.id,
        task_name=task.name,
        scores=scores,
        actual_output=outcome.execution.actual_output,
        expected_output=task.expected_output,
        tool_calls=[{"name": tc.name, "args": tc.args} for tc in outcome.execution.tool_calls],
        cost_usd=outcome.execution.cost_usd,
        tokens_in=outcome.execution.tokens_in,
        tokens_out=outcome.execution.tokens_out,
        duration_ms=outcome.execution.duration_ms,
        evaluator_scores={},
        files_changed=outcome.execution.files_changed,
        functional_test_output=outcome.execution.test_output,
        rollout_id=outcome.rollout_id,
        rollout_count=rollout_count,
        is_best_rollout=outcome.is_best,
        diversity_score=outcome.diversity_score,
    )


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
    _aggregate_metric_scores(scores)

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


def _build_progress_callbacks(js: object, run_id: str) -> tuple:
    """Create NATS publishing callbacks for per-task progress events.

    Returns (on_task_start, on_task_complete) callables suitable for
    BaseBenchmarkRunner.run_tasks().
    """
    import json as _json

    accumulated_cost = 0.0
    accumulated_scores: list[float] = []

    async def on_task_start(task: object, index: int, total: int) -> None:
        if js is None:
            return
        payload = _json.dumps(
            {
                "run_id": run_id,
                "task_id": task.id,
                "task_name": task.name,
                "index": index + 1,
                "total": total,
            }
        ).encode()
        try:
            await js.publish(SUBJECT_BENCHMARK_TASK_STARTED, payload)
        except Exception:
            logger.debug("failed to publish benchmark.task.started", exc_info=True)

    async def on_task_complete(task: object, result: object, index: int, total: int) -> None:
        nonlocal accumulated_cost
        if js is None:
            return

        # RunResult has .execution.cost_usd; BenchmarkTaskResult has .cost_usd directly.
        cost = 0.0
        avg_task_score = 0.0
        if hasattr(result, "execution"):
            cost = getattr(result.execution, "cost_usd", 0.0) or 0.0
            if result.eval_score is not None:
                dims = getattr(result.eval_score, "dimensions", [])
                dim_scores = [d.score for d in dims if hasattr(d, "score")]
                avg_task_score = sum(dim_scores) / len(dim_scores) if dim_scores else 0.0
        else:
            cost = getattr(result, "cost_usd", 0.0) or 0.0
            scores = getattr(result, "scores", {}) or {}
            score_vals = list(scores.values()) if isinstance(scores, dict) else []
            avg_task_score = sum(score_vals) / len(score_vals) if score_vals else 0.0

        accumulated_cost += cost
        accumulated_scores.append(avg_task_score)
        avg_score = sum(accumulated_scores) / len(accumulated_scores) if accumulated_scores else 0.0

        payload = _json.dumps(
            {
                "run_id": run_id,
                "task_id": task.id,
                "task_name": task.name,
                "score": round(avg_task_score, 4),
                "cost_usd": round(cost, 6),
                "completed_tasks": index + 1,
                "total_tasks": total,
                "avg_score": round(avg_score, 4),
                "total_cost_usd": round(accumulated_cost, 6),
            }
        ).encode()
        try:
            await js.publish(SUBJECT_BENCHMARK_TASK_PROGRESS, payload)
        except Exception:
            logger.debug("failed to publish benchmark.task.progress", exc_info=True)

    return on_task_start, on_task_complete


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


# --- Auto-routing support (Tasks 19 & 20) ---


class _RoutingLLMWrapper:
    """Transparent LLM wrapper that routes each call through HybridRouter.

    Delegates all attributes to the underlying LLM client, but intercepts
    ``chat_completion`` and ``chat_completion_stream`` to select the model
    via the router before forwarding.
    Records routing decisions in ``routing_log`` for post-run annotation.
    """

    def __init__(self, llm: object, router: object) -> None:
        self._llm = llm
        self._router = router
        self.routing_log: list[dict[str, str]] = []

    def _route_model(self, kwargs: dict[str, object]) -> None:
        """Extract user prompt from messages, route, and replace model in kwargs."""
        messages = kwargs.get("messages", [])
        prompt = ""
        for m in reversed(messages):  # type: ignore[arg-type]
            role = m.get("role", "") if isinstance(m, dict) else getattr(m, "role", "")
            if role == "user":
                prompt = m.get("content", "") if isinstance(m, dict) else getattr(m, "content", "")
                break

        decision = None
        if prompt:
            with contextlib.suppress(Exception):
                decision = self._router.route(prompt)

        if decision is not None and decision.model:
            kwargs["model"] = decision.model
            self.routing_log.append(
                {
                    "model": decision.model,
                    "layer": getattr(decision, "routing_layer", "unknown"),
                    "reasoning": getattr(decision, "reasoning", ""),
                }
            )
        else:
            # No routing decision — resolve a real model instead of passing "auto".
            from codeforge.model_resolver import resolve_model

            fallback_model = resolve_model()
            kwargs["model"] = fallback_model
            self.routing_log.append(
                {
                    "model": fallback_model,
                    "layer": "fallback",
                    "reasoning": "router returned no decision",
                }
            )

    @staticmethod
    def _sanitize_messages(kwargs: dict[str, object]) -> None:
        """Ensure tool messages have required fields for all providers."""
        from codeforge.agent_loop import sanitize_tool_messages

        messages = kwargs.get("messages")
        if isinstance(messages, list):
            sanitize_tool_messages(messages)

    async def chat_completion(self, **kwargs: object) -> object:
        """Route, sanitize, then delegate to the real LLM client."""
        self._route_model(kwargs)
        self._sanitize_messages(kwargs)
        return await self._llm.chat_completion(**kwargs)

    async def chat_completion_stream(self, **kwargs: object) -> object:
        """Route, sanitize, then delegate to the real LLM client (streaming)."""
        self._route_model(kwargs)
        self._sanitize_messages(kwargs)
        return await self._llm.chat_completion_stream(**kwargs)

    def __getattr__(self, name: str) -> object:
        """Proxy all other attributes to the underlying LLM client."""
        return getattr(self._llm, name)


def _annotate_routing(results: list, routing_log: list[dict[str, str]]) -> None:
    """Stamp routing metadata onto each BenchmarkTaskResult from the routing log.

    Matches results to log entries by position (1:1 correspondence between
    tasks and LLM calls). Extra log entries or results are silently ignored.
    """
    for i, result in enumerate(results):
        if i >= len(routing_log):
            break
        entry = routing_log[i]
        result.selected_model = entry.get("model", "")
        result.routing_reason = entry.get("reasoning", "")


def _compute_routing_report(results: list) -> dict[str, object]:
    """Aggregate routing metadata from annotated results into a summary report.

    Returns a dict with:
    - models_used: list of distinct models selected
    - model_distribution: {model: count}
    - layer_distribution: {layer: count}
    - avg_score_by_model: {model: average_score}
    """
    model_counts: dict[str, int] = {}
    layer_counts: dict[str, int] = {}
    model_scores: dict[str, list[float]] = {}

    for r in results:
        model = getattr(r, "selected_model", "") or "unknown"
        layer = getattr(r, "routing_reason", "") or "unknown"

        model_counts[model] = model_counts.get(model, 0) + 1

        # Routing reason may contain the layer info; use it as-is.
        layer_counts[layer] = layer_counts.get(layer, 0) + 1

        scores = list(r.scores.values()) if r.scores else []
        if scores:
            avg = sum(scores) / len(scores)
            if model not in model_scores:
                model_scores[model] = []
            model_scores[model].append(avg)

    avg_by_model: dict[str, float] = {}
    for model, score_list in model_scores.items():
        avg_by_model[model] = round(sum(score_list) / len(score_list), 4) if score_list else 0.0

    return {
        "models_used": sorted(model_counts.keys()),
        "model_distribution": model_counts,
        "layer_distribution": layer_counts,
        "avg_score_by_model": avg_by_model,
    }
