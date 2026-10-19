"""Skill selection: local (BM25) ranking of skills for a task, and the model of LLM skill checks.

Design decision: the skill check (safety) uses the cheapest tool-capable
model via get_available_models() + filter_models_by_capability() + cost
sorting rather than routing through the HybridRouter. See design doc
section 5 for rationale and alternatives if this needs to change later.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

from codeforge.model_resolver import get_available_models
from codeforge.routing.capabilities import enrich_model_capabilities, filter_models_by_capability
from codeforge.skills.recommender import SkillRecommender

if TYPE_CHECKING:
    from codeforge.skills.models import Skill

_MAX_SKILLS_PER_RUN = 5


def resolve_skill_selection_model() -> str:
    """Pick the cheapest available model that supports function calling.

    Design note: This bypasses the HybridRouter intentionally because
    skill selection is always a simple task (short list in, JSON out).
    """
    available = get_available_models()
    capable = filter_models_by_capability(available, needs_tools=True)

    if not capable:
        return available[0] if available else ""

    return min(
        capable,
        key=lambda m: float(enrich_model_capabilities(m).get("input_cost_per_token", float("inf"))),
    )


def rank_skills(skills: list[Skill], task_context: str, max_skills: int = _MAX_SKILLS_PER_RUN) -> list[Skill]:
    """Select the skills relevant to a task locally (BM25), without an LLM call.

    The conversation path uses it (KI-192): the LLM selection sent the user's
    message to a model the user did not choose, uncosted.
    """
    if not skills or not task_context:
        return []
    return _bm25_fallback(skills, task_context, max_skills)


def _bm25_fallback(skills: list[Skill], task_context: str, max_skills: int) -> list[Skill]:
    recommender = SkillRecommender()
    recommender.index(skills)
    recs = recommender.recommend(task_context, top_k=max_skills)
    return [r.skill for r in recs]
