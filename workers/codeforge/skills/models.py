"""Pydantic models for the skills subsystem."""

from __future__ import annotations

from pydantic import BaseModel, Field


class Skill(BaseModel):
    """A reusable workflow or code pattern for agent prompt injection."""

    id: str = ""
    tenant_id: str = ""
    project_id: str = ""
    name: str
    type: str = "pattern"  # workflow | pattern
    description: str = ""
    language: str = ""
    content: str = ""  # primary field - markdown body
    tags: list[str] = Field(default_factory=list)
    source: str = "user"  # builtin | import | user | agent
    source_url: str = ""
    format_origin: str = "codeforge"  # claude | cursor | markdown | codeforge
    status: str = "active"  # draft | active | disabled
    usage_count: int = 0

    # Deprecated: use content. Kept for DB rows that still use code.
    code: str = ""
    # Deprecated: use status.
    enabled: bool = True


class SkillRecommendation(BaseModel):
    """A skill recommended by BM25 relevance scoring."""

    skill: Skill
    score: float = 0.0
