"""Benchmark and GEMMAS evaluation handler mixins.

Routes benchmark requests to the appropriate runner based on benchmark_type:
- simple  -> SimpleBenchmarkRunner (prompt -> LLM -> compare output)
- tool_use -> ToolUseBenchmarkRunner (prompt + tools -> LLM -> output + tool calls)
- agent   -> AgentBenchmarkRunner (full multi-turn agent loop)
"""

from __future__ import annotations

import asyncio
import contextlib
import time
from typing import TYPE_CHECKING

import structlog

from codeforge.config import get_settings
from codeforge.consumer._subjects import (
    HEADER_REQUEST_ID,
    SUBJECT_BENCHMARK_RUN_RESULT,
    SUBJECT_BENCHMARK_TASK_PROGRESS,
    SUBJECT_BENCHMARK_TASK_STARTED,
    SUBJECT_EVAL_GEMMAS_RESULT,
)
from codeforge.consumer.benchmark_gemmas import (
    build_embed_fn,
    compute_routing_report,
    compute_summary,
)
from codeforge.consumer.benchmark_runners import (
    run_agent_benchmark,
    run_simple_benchmark,
    run_tool_use_benchmark,
)
from codeforge.models import GemmasEvalRequest, GemmasEvalResult

if TYPE_CHECKING:
    import nats.aio.msg


logger = structlog.get_logger()


# --- Parallel benchmark execution helpers ---


def _get_benchmark_semaphore() -> asyncio.Semaphore:
    return asyncio.Semaphore(get_settings().benchmark_max_parallel)


_benchmark_semaphore: asyncio.Semaphore | None = None


def _ensure_benchmark_semaphore() -> asyncio.Semaphore:
    global _benchmark_semaphore
    if _benchmark_semaphore is None:
        _benchmark_semaphore = _get_benchmark_semaphore()
    return _benchmark_semaphore


def _handle_task_exception(task: asyncio.Task[None]) -> None:
    if task.cancelled():
        return
    exc = task.exception()
    if exc is not None:
        logger.error("benchmark task failed with unhandled exception", task_name=task.get_name(), error=str(exc))


# --- Model validation ---


async def _fetch_available_models() -> list[str]:
    import httpx

    from codeforge.config import WorkerSettings

    litellm_url = WorkerSettings().litellm_url
    api_key = get_settings().litellm_api_key
    headers: dict[str, str] = {}
    if api_key:
        headers["Authorization"] = f"Bearer {api_key}"
    try:
        async with httpx.AsyncClient(timeout=5.0) as client:
            resp = await client.get(f"{litellm_url}/v1/models", headers=headers)
        if resp.status_code != 200:
            return []
        data = resp.json()
        return [m.get("id", "") for m in data.get("data", []) if m.get("id")]
    except Exception as exc:
        logger.warning("failed to fetch available models from LiteLLM", error=str(exc))
        return []


async def _fetch_configured_models() -> list[str]:
    import httpx

    from codeforge.config import WorkerSettings

    litellm_url = WorkerSettings().litellm_url
    api_key = get_settings().litellm_api_key
    headers: dict[str, str] = {}
    if api_key:
        headers["Authorization"] = f"Bearer {api_key}"
    try:
        async with httpx.AsyncClient(timeout=5.0) as client:
            resp = await client.get(f"{litellm_url}/model/info", headers=headers)
        if resp.status_code != 200:
            return []
        data = resp.json()
        models: list[str] = []
        for m in data.get("data", []):
            name = m.get("model_name", "")
            if name:
                models.append(name)
            litellm_model = m.get("litellm_params", {}).get("model", "")
            if litellm_model and litellm_model != name:
                models.append(litellm_model)
        return models
    except Exception as exc:
        logger.warning("failed to fetch configured models from LiteLLM", error=str(exc))
        return []


async def _validate_model_exists(model: str, available_models: list[str] | None = None) -> None:
    if model == "auto":
        return
    if available_models is None:
        available_models = await _fetch_available_models()
    if not available_models:
        configured = await _fetch_configured_models()
        if not configured:
            logger.warning("cannot validate model: LiteLLM unreachable", model=model)
            raise ValueError(
                f"cannot validate model {model!r}: LiteLLM is unreachable. "
                "Ensure LiteLLM proxy is running and accessible."
            )
        if model not in configured:
            raise ValueError(
                f"model {model!r} not found in LiteLLM configured models. "
                f"Configured: {', '.join(sorted(configured)[:10])}"
            )
        return
    if model not in available_models:
        raise ValueError(
            f"model {model!r} not available in LiteLLM. Available: {', '.join(sorted(available_models)[:10])}"
        )


# --- Health check ---

_HEALTH_MAX_ATTEMPTS = 5
_HEALTH_BACKOFF_BASE = 2.0


async def _wait_for_litellm(llm: object, log: structlog.stdlib.BoundLogger) -> bool:
    from codeforge.llm import LiteLLMClient

    if not isinstance(llm, LiteLLMClient):
        return True
    for attempt in range(_HEALTH_MAX_ATTEMPTS):
        if await llm.health():
            if attempt > 0:
                log.info("LiteLLM became healthy", attempt=attempt + 1)
            return True
        wait = _HEALTH_BACKOFF_BASE**attempt
        log.warning(
            "LiteLLM not ready, retrying", attempt=attempt + 1, max_attempts=_HEALTH_MAX_ATTEMPTS, retry_in_seconds=wait
        )
        await asyncio.sleep(wait)
    return False


# --- Evaluator + pipeline builders ---


def _build_evaluators(evaluator_names: list[str], model: str) -> list:
    from codeforge.evaluation.evaluators.functional_test import FunctionalTestEvaluator
    from codeforge.evaluation.evaluators.llm_judge import LLMJudgeEvaluator
    from codeforge.evaluation.evaluators.sparc import SPARCEvaluator
    from codeforge.evaluation.evaluators.trajectory_verifier import TrajectoryVerifierEvaluator

    llm_judge_metrics = {
        "correctness",
        "faithfulness",
        "relevance",
        "coherence",
        "fluency",
        "tool_correctness",
        "answer_relevancy",
        "contextual_precision",
    }
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
        elif name == "logprob_verifier":
            from codeforge.evaluation.evaluators.logprob_verifier import LogprobVerifierEvaluator

            evaluators.append(LogprobVerifierEvaluator(model=model))
        elif name == "filesystem_state":
            from codeforge.evaluation.evaluators.filesystem_state import FilesystemStateEvaluator

            evaluators.append(FilesystemStateEvaluator())
        elif name in llm_judge_metrics:
            collected_llm_metrics.append(name)
        else:
            valid_names = (
                "llm_judge, functional_test, sparc, trajectory_verifier, "
                "logprob_verifier, filesystem_state, correctness, faithfulness, relevance, "
                "coherence, fluency, tool_correctness, answer_relevancy, contextual_precision"
            )
            raise ValueError(f"unknown evaluator/metric: {name!r}. Valid: {valid_names}")

    if collected_llm_metrics:
        from codeforge.evaluation.litellm_judge import LiteLLMJudge

        judge = LiteLLMJudge(model=model)
        evaluators.append(LLMJudgeEvaluator(judge=judge, metrics=collected_llm_metrics))
    if not evaluators:
        raise ValueError("no valid evaluators produced from metrics list")
    return evaluators


def _build_hybrid_pipeline(evaluators: list) -> object:
    from codeforge.evaluation.hybrid_pipeline import HybridEvaluationPipeline

    filter_evals = [e for e in evaluators if getattr(e, "stage", "rank") == "filter"]
    rank_evals = [e for e in evaluators if getattr(e, "stage", "rank") == "rank"]
    return HybridEvaluationPipeline(filter_evaluators=filter_evals, rank_evaluators=rank_evals)


def _build_progress_callbacks(js: object, run_id: str) -> tuple:
    import json as _json

    accumulated_cost = 0.0
    accumulated_scores: list[float] = []

    async def on_task_start(task: object, index: int, total: int) -> None:
        if js is None:
            return
        payload = _json.dumps(
            {"run_id": run_id, "task_id": task.id, "task_name": task.name, "index": index + 1, "total": total}
        ).encode()
        try:
            await js.publish(SUBJECT_BENCHMARK_TASK_STARTED, payload)
        except Exception as exc:
            logger.debug("failed to publish benchmark.task.started", exc_info=True, error=str(exc))

    async def on_task_complete(task: object, result: object, index: int, total: int) -> None:
        nonlocal accumulated_cost
        if js is None:
            return
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
        except Exception as exc:
            logger.debug("failed to publish benchmark.task.progress", exc_info=True, error=str(exc))

    return on_task_start, on_task_complete


# --- Auto-routing wrapper ---


class _RoutingLLMWrapper:
    """Transparent LLM wrapper that routes each call through HybridRouter."""

    def __init__(self, llm: object, router: object) -> None:
        self._llm = llm
        self._router = router
        self.routing_log: list[dict[str, str]] = []

    def _route_model(self, kwargs: dict[str, object]) -> None:
        messages = kwargs.get("messages", [])
        prompt = ""
        for m in reversed(messages):
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
            from codeforge.model_resolver import resolve_model

            fallback_model = resolve_model()
            kwargs["model"] = fallback_model
            self.routing_log.append(
                {"model": fallback_model, "layer": "fallback", "reasoning": "router returned no decision"}
            )

    @staticmethod
    def _sanitize_messages(kwargs: dict[str, object]) -> None:
        from codeforge.loop_helpers import sanitize_tool_messages

        messages = kwargs.get("messages")
        if isinstance(messages, list):
            sanitize_tool_messages(messages)

    async def chat_completion(self, **kwargs: object) -> object:
        self._route_model(kwargs)
        self._sanitize_messages(kwargs)
        return await self._llm.chat_completion(**kwargs)

    async def chat_completion_stream(self, **kwargs: object) -> object:
        self._route_model(kwargs)
        self._sanitize_messages(kwargs)
        return await self._llm.chat_completion_stream(**kwargs)

    def __getattr__(self, name: str) -> object:
        return getattr(self._llm, name)


def _annotate_routing(results: list, routing_log: list[dict[str, str]]) -> None:
    for i, result in enumerate(results):
        if i >= len(routing_log):
            break
        entry = routing_log[i]
        result.selected_model = entry.get("model", "")
        result.routing_reason = entry.get("reasoning", "")


# --- BenchmarkHandlerMixin ---


class BenchmarkHandlerMixin:
    """Handles benchmark.run.request and evaluation.gemmas.request messages."""

    async def _handle_benchmark_run(self, msg: nats.aio.msg.Msg) -> None:
        from codeforge.models import BenchmarkRunRequest, BenchmarkRunResult

        if get_settings().app_env != "development":
            logger.warning("benchmark run ignored (not in dev mode)")
            await msg.ack()
            return

        request_id = (msg.headers or {}).get(HEADER_REQUEST_ID, "")
        log = logger.bind(request_id=request_id)

        if not await _wait_for_litellm(self._llm, log):
            log.error("LiteLLM not available, aborting benchmark run")
            await self._publish_error(
                BenchmarkRunResult(
                    run_id="", status="failed", error="LiteLLM proxy not available after health check retries"
                ),
                SUBJECT_BENCHMARK_RUN_RESULT,
            )
            await msg.ack()
            return

        run_id = ""
        tenant_id = ""
        try:
            req = BenchmarkRunRequest.model_validate_json(msg.data)
            run_id = req.run_id
            tenant_id = req.tenant_id
            await _validate_model_exists(req.model)
            benchmark_type = req.benchmark_type or "simple"
            log = log.bind(run_id=run_id, benchmark_type=benchmark_type, model=req.model)

            if self._is_duplicate(f"bench-{req.run_id}"):
                log.warning("duplicate benchmark run, skipping")
                await msg.ack()
                return

            await msg.ack()
            task = asyncio.create_task(self._execute_benchmark_run(req, log), name=f"benchmark-{req.run_id}")
            task.add_done_callback(_handle_task_exception)

        except Exception as exc:
            log.exception("benchmark run failed")
            await self._publish_error(
                BenchmarkRunResult(run_id=run_id, tenant_id=tenant_id, status="failed", error=str(exc)),
                SUBJECT_BENCHMARK_RUN_RESULT,
            )
            await msg.ack()

    async def _execute_benchmark_run(self, req: object, log: structlog.BoundLogger) -> None:
        from codeforge.evaluation.pipeline import EvaluationPipeline
        from codeforge.models import BenchmarkRunResult

        async with _ensure_benchmark_semaphore():
            try:
                log.info("benchmark run started")
                start = time.monotonic()
                evaluators = _build_evaluators(req.evaluators, req.model)
                pipeline = EvaluationPipeline(evaluators)
                hybrid_pipeline = _build_hybrid_pipeline(evaluators) if req.hybrid_verification else None
                effective_llm = await self._resolve_effective_llm(req, log)
                on_start, on_complete = _build_progress_callbacks(self._js, req.run_id)

                benchmark_type = req.benchmark_type or "simple"
                if benchmark_type == "tool_use":
                    results = await run_tool_use_benchmark(
                        req, effective_llm, pipeline, on_start, on_complete, hybrid_pipeline
                    )
                elif benchmark_type == "agent":
                    results = await run_agent_benchmark(
                        req, effective_llm, pipeline, on_start, on_complete, hybrid_pipeline
                    )
                else:
                    results = await run_simple_benchmark(
                        req, effective_llm, pipeline, on_start, on_complete, hybrid_pipeline
                    )

                if req.model == "auto" and hasattr(effective_llm, "routing_log"):
                    _annotate_routing(results, effective_llm.routing_log)

                elapsed_ms = int((time.monotonic() - start) * 1000)
                summary = compute_summary(results, elapsed_ms)
                if req.model == "auto":
                    summary["routing_report"] = compute_routing_report(results)

                result = BenchmarkRunResult(
                    run_id=req.run_id,
                    tenant_id=req.tenant_id,
                    status="completed",
                    results=results,
                    summary=summary,
                    total_cost=summary.get("total_cost_usd", 0.0),
                    total_tokens=summary.get("total_tokens_in", 0) + summary.get("total_tokens_out", 0),
                    total_duration_ms=summary.get("elapsed_ms", 0),
                )
                if self._js is not None:
                    await self._js.publish(SUBJECT_BENCHMARK_RUN_RESULT, result.model_dump_json().encode())
                log.info(
                    "benchmark run completed",
                    task_count=len(results),
                    elapsed_ms=elapsed_ms,
                    avg_score=summary.get("avg_score", 0),
                )

            except Exception as exc:
                log.exception("benchmark run failed")
                await self._publish_error(
                    BenchmarkRunResult(run_id=req.run_id, tenant_id=req.tenant_id, status="failed", error=str(exc)),
                    SUBJECT_BENCHMARK_RUN_RESULT,
                )

    async def _resolve_effective_llm(self, req: object, log: structlog.BoundLogger) -> object:
        if req.model != "auto":
            return self._llm
        try:
            from codeforge.consumer.conversation_routing import get_hybrid_router

            router = await get_hybrid_router(self._litellm_url, self._litellm_key)
            if router is not None:
                log.info("auto-routing enabled for benchmark run")
                return _RoutingLLMWrapper(self._llm, router)
        except Exception as exc:
            log.warning("HybridRouter initialization failed", exc_info=True, error=str(exc))
        raise ValueError(
            "model='auto' requires intelligent routing, but routing is not enabled. "
            "Set CODEFORGE_ROUTING_ENABLED=true or specify an explicit model name."
        )

    async def _handle_gemmas_eval(self, msg: nats.aio.msg.Msg) -> None:
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
        try:
            from codeforge.evaluation.executor import handle_gemmas_evaluation

            log.info("received GEMMAS scoring request", messages=len(request.messages))
            embed_fn = build_embed_fn(self._litellm_url, self._litellm_key)
            result_dict = await handle_gemmas_evaluation(
                messages=[m.model_dump() for m in request.messages],
                plan_id=request.plan_id,
                embed_fn=embed_fn,
            )
            result = GemmasEvalResult(**result_dict)
            log.info(
                "GEMMAS scoring completed", ids=result.information_diversity_score, upr=result.unnecessary_path_ratio
            )
            return result
        except Exception as exc:
            logger.exception("failed to process GEMMAS scoring request", error=str(exc))
            return None
