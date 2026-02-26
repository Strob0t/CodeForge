"""Schema for task decomposition step."""

from __future__ import annotations

from pydantic import BaseModel, Field


class SubTask(BaseModel):
    """A single subtask produced by decomposition."""

    title: str
    description: str
    estimated_complexity: str = Field(
        default="medium",
        pattern="^(small|medium|large)$",
        description="Estimated complexity: small, medium, or large",
    )
    depends_on: list[str] = Field(default_factory=list)
    suggested_mode: str = ""


class DecomposeInput(BaseModel):
    """Input for the decomposition step."""

    goal: str
    tech_context: str = ""
    constraints: list[str] = Field(default_factory=list)


class DecomposeOutput(BaseModel):
    """Output from the decomposition step."""

    subtasks: list[SubTask]
    execution_order: str = Field(
        default="sequential",
        pattern="^(sequential|parallel|mixed)$",
    )
    reasoning: str
