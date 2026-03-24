"""GEMMAS scoring, result conversion, and evaluation helpers for benchmarks."""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    from collections.abc import Callable

logger = structlog.get_logger()


# ---------------------------------------------------------------------------
# Score key normalization
# ---------------------------------------------------------------------------

_DIMENSION_TO_METRIC: dict[str, str] = {
    "correctness": "llm_judge",
    "faithfulness": "llm_judge",
    "answer_relevancy": "llm_judge",
    "tool_correctness": "llm_judge",
    "sparc_steps": "sparc",
    "sparc_time": "sparc",
    "sparc_cost": "sparc",
    "sparc_complexity": "sparc",
    "sparc_code_quality": "sparc",
    "sparc_security": "sparc",
    "trajectory_solution_quality": "trajectory_verifier",
    "trajectory_approach_efficiency": "trajectory_verifier",
    "trajectory_code_quality": "trajectory_verifier",
    "trajectory_error_recovery": "trajectory_verifier",
    "trajectory_completeness": "trajectory_verifier",
    "trajectory_quality": "trajectory_verifier",
    "logprob_verification": "logprob_verifier",
    "functional_test": "functional_test",
    "filesystem_state": "filesystem_state",
}


def aggregate_metric_scores(scores: dict[str, float]) -> None:
    """Add averaged parent-metric keys to scores dict in-place."""
    groups: dict[str, list[float]] = {}
    for dim_name, value in scores.items():
        metric = _DIMENSION_TO_METRIC.get(dim_name)
        if metric is not None:
            groups.setdefault(metric, []).append(value)

    for metric, values in groups.items():
        if metric in scores:
            continue
        scores[metric] = sum(values) / len(values)


# ---------------------------------------------------------------------------
# Result conversion
# ---------------------------------------------------------------------------


def convert_rollout_outcome(task: object, outcome: object, rollout_count: int) -> object:
    """Convert a RolloutOutcome to a BenchmarkTaskResult with rollout fields."""
    from codeforge.models import BenchmarkTaskResult

    scores: dict[str, float] = {}
    if outcome.eval_score:
        for dim in outcome.eval_score.dimensions:
            scores[dim.name] = dim.score
    aggregate_metric_scores(scores)

    return BenchmarkTaskResult(
        task_id=task.id,
        task_name=task.name,
        scores=scores,
        actual_output=outcome.result.actual_output,
        expected_output=task.expected_output,
        tool_calls=[{"name": tc.name, "args": tc.args} for tc in outcome.result.tool_calls],
        cost_usd=outcome.result.cost_usd,
        tokens_in=outcome.result.tokens_in,
        tokens_out=outcome.result.tokens_out,
        duration_ms=outcome.result.duration_ms,
        evaluator_scores={},
        files_changed=outcome.result.files_changed,
        functional_test_output=outcome.result.test_output,
        rollout_id=outcome.rollout_id,
        rollout_count=rollout_count,
        is_best_rollout=outcome.is_best,
        diversity_score=outcome.diversity_score,
    )


def convert_result(r: object) -> object:
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
    aggregate_metric_scores(scores)

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


# ---------------------------------------------------------------------------
# GEMMAS scoring
# ---------------------------------------------------------------------------


def build_embed_fn(litellm_url: str, litellm_key: str) -> Callable[[list[str]], list[list[float]]] | None:
    """Build a sync embedding function using LiteLLM's /v1/embeddings endpoint."""
    import httpx

    url = litellm_url.rstrip("/") + "/v1/embeddings"
    headers: dict[str, str] = {"Content-Type": "application/json"}
    if litellm_key:
        headers["Authorization"] = f"Bearer {litellm_key}"

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


# ---------------------------------------------------------------------------
# Summary / routing report helpers
# ---------------------------------------------------------------------------


def compute_summary(results: list, elapsed_ms: int) -> dict[str, object]:
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


def compute_routing_report(results: list) -> dict[str, object]:
    """Aggregate routing metadata from annotated results into a summary report."""
    model_counts: dict[str, int] = {}
    layer_counts: dict[str, int] = {}
    model_scores: dict[str, list[float]] = {}

    for r in results:
        model = getattr(r, "selected_model", "") or "unknown"
        layer = getattr(r, "routing_reason", "") or "unknown"
        model_counts[model] = model_counts.get(model, 0) + 1
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
