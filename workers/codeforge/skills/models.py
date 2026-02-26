"""Pydantic models for the skills subsystem."""

from __future__ import annotations

from pydantic import BaseModel, Field


class Skill(BaseModel):
    """A reusable code snippet with metadata for BM25 matching."""

    id: str = ""
    tenant_id: str = ""
    project_id: str = ""
    name: str
    description: str = ""
    language: str = ""
    code: str
    tags: list[str] = Field(default_factory=list)
    enabled: bool = True


class SkillRecommendation(BaseModel):
    """A skill recommended by BM25 relevance scoring."""

    skill: Skill
    score: float = 0.0
