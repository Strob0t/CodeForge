"""Benchmark and GEMMAS evaluation handler mixins."""

from __future__ import annotations

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
        import time

        from codeforge.evaluation.datasets import load_dataset
        from codeforge.evaluation.litellm_judge import LiteLLMJudge
        from codeforge.evaluation.runner import BenchmarkRunner
        from codeforge.models import BenchmarkRunRequest, BenchmarkRunResult, BenchmarkTaskResult

        if os.getenv("APP_ENV") != "development":
            logger.warning("benchmark run ignored (not in dev mode)")
            await msg.ack()
            return

        request_id = (msg.headers or {}).get(HEADER_REQUEST_ID, "")
        log = logger.bind(request_id=request_id)

        try:
            req = BenchmarkRunRequest.model_validate_json(msg.data)
            log = log.bind(run_id=req.run_id, dataset=req.dataset_path, model=req.model)
            log.info("benchmark run started")

            start = time.monotonic()
            dataset = load_dataset(req.dataset_path)

            judge = LiteLLMJudge(model=req.model, base_url=self._litellm_url + "/v1")
            runner = BenchmarkRunner(llm=self._llm, model=req.model, metrics=req.metrics, judge=judge)

            task_results = await runner.run(dataset)
            total_ms = int((time.monotonic() - start) * 1000)

            summary: dict[str, float] = {}
            for metric in req.metrics:
                values = [tr.scores.get(metric, 0.0) for tr in task_results]
                summary[metric] = sum(values) / len(values) if values else 0.0

            result = BenchmarkRunResult(
                run_id=req.run_id,
                status="completed",
                tasks=[
                    BenchmarkTaskResult(
                        task_id=tr.task_id,
                        task_name=tr.task_name,
                        scores=tr.scores,
                        actual_output=tr.actual_output,
                        expected_output=tr.expected_output,
                        duration_ms=tr.duration_ms,
                        cost_usd=tr.cost_usd,
                        tokens_in=tr.tokens_in,
                        tokens_out=tr.tokens_out,
                    )
                    for tr in task_results
                ],
                summary_scores=summary,
                total_duration_ms=total_ms,
            )

            if self._js is not None:
                await self._js.publish(
                    SUBJECT_BENCHMARK_RUN_RESULT,
                    result.model_dump_json().encode(),
                )

            await msg.ack()
            log.info("benchmark run completed", summary=summary, duration_ms=total_ms)

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
