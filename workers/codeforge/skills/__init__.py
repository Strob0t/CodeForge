"""Skills system: reusable code snippets injected into agent prompts via BM25."""

from codeforge.skills.models import Skill, SkillRecommendation
from codeforge.skills.recommender import SkillRecommender
from codeforge.skills.registry import SkillRegistry

__all__ = [
    "Skill",
    "SkillRecommendation",
    "SkillRecommender",
    "SkillRegistry",
]
