"""LLM-based skill selection for the auto-agent pre-loop phase.

Design decision: We select the cheapest tool-capable model via
get_available_models() + filter_models_by_capability() + cost sorting
rather than routing through the HybridRouter. See design doc section 5
for rationale and alternatives if this needs to change later.
"""

from __future__ import annotations

import json
from typing import TYPE_CHECKING

import structlog

from codeforge.model_resolver import get_available_models
from codeforge.routing.capabilities import enrich_model_capabilities, filter_models_by_capability
from codeforge.skills.recommender import SkillRecommender

if TYPE_CHECKING:
    from codeforge.llm import LiteLLMClient
    from codeforge.skills.models import Skill

logger = structlog.get_logger(__name__)

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


async def select_skills_for_task(
    skills: list[Skill],
    task_context: str,
    llm_client: LiteLLMClient,
    max_skills: int = _MAX_SKILLS_PER_RUN,
) -> list[Skill]:
    """Select relevant skills for a task using LLM, with BM25 fallback."""
    if not skills or not task_context:
        return []

    try:
        return await _llm_select(skills, task_context, llm_client, max_skills)
    except Exception as exc:
        logger.warning("LLM skill selection failed, falling back to BM25", exc_info=True, error=str(exc))
        return _bm25_fallback(skills, task_context, max_skills)


async def _llm_select(
    skills: list[Skill],
    task_context: str,
    llm_client: LiteLLMClient,
    max_skills: int,
) -> list[Skill]:
    model = resolve_skill_selection_model()
    if not model:
        return _bm25_fallback(skills, task_context, max_skills)

    skill_list = "\n".join(f'{i + 1}. id="{s.id}" - {s.name} ({s.type}): {s.description}' for i, s in enumerate(skills))

    messages: list[dict[str, object]] = [
        {
            "role": "system",
            "content": (
                "You are a skill selector. Given a task and a list of available skills, "
                "return a JSON array of skill IDs that are relevant to the task. "
                f"Return at most {max_skills} IDs. Return [] if none are relevant. "
                "Respond ONLY with a JSON array of strings, nothing else."
            ),
        },
        {
            "role": "user",
            "content": f"Task: {task_context}\n\nAvailable skills:\n{skill_list}",
        },
    ]

    resp = await llm_client.chat_completion(messages=messages, model=model, temperature=0.0)
    selected_ids = json.loads(resp.content.strip())

    if not isinstance(selected_ids, list):
        return []

    id_set = set(selected_ids[:max_skills])
    return [s for s in skills if s.id in id_set]


def _bm25_fallback(skills: list[Skill], task_context: str, max_skills: int) -> list[Skill]:
    recommender = SkillRecommender()
    recommender.index(skills)
    recs = recommender.recommend(task_context, top_k=max_skills)
    return [r.skill for r in recs]
