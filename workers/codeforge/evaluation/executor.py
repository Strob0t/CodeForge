"""GEMMAS evaluation executor for multi-agent collaboration metrics."""

from __future__ import annotations

from typing import TYPE_CHECKING

from codeforge.evaluation.collaboration import InformationDiversityScore, UnnecessaryPathRatio
from codeforge.evaluation.dag_builder import build_collaboration_dag

if TYPE_CHECKING:
    from collections.abc import Callable


async def handle_gemmas_evaluation(
    messages: list[dict],
    plan_id: str,
    embed_fn: Callable[[list[str]], list[list[float]]] | None = None,
) -> dict:
    """Compute GEMMAS metrics for a set of agent messages.

    Args:
        messages: Raw agent message dicts with agent_id, content, round, parent_agent_id.
        plan_id: ID of the execution plan being evaluated.
        embed_fn: Optional embedding function for semantic IDS computation.

    Returns:
        Dict with plan_id, information_diversity_score, unnecessary_path_ratio, error.
    """
    try:
        if not messages:
            return {
                "plan_id": plan_id,
                "information_diversity_score": 1.0,
                "unnecessary_path_ratio": 0.0,
                "error": "",
            }

        dag = build_collaboration_dag(messages)
        ids = InformationDiversityScore(embed_fn=embed_fn).compute(dag)
        upr = UnnecessaryPathRatio().compute(dag)

        return {
            "plan_id": plan_id,
            "information_diversity_score": ids,
            "unnecessary_path_ratio": upr,
            "error": "",
        }
    except Exception as exc:
        return {
            "plan_id": plan_id,
            "information_diversity_score": 0.0,
            "unnecessary_path_ratio": 0.0,
            "error": str(exc),
        }
