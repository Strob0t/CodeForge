"""Schema for code review step."""

from __future__ import annotations

from pydantic import BaseModel, Field


class Issue(BaseModel):
    """A single issue found during code review."""

    severity: str = Field(
        description="Issue severity",
        pattern="^(critical|high|medium|low)$",
    )
    file: str = ""
    line: int = 0
    description: str
    suggestion: str = ""


class ReviewInput(BaseModel):
    """Input for the code review step."""

    code: str
    spec: str = ""
    criteria: list[str] = Field(default_factory=list)


class ReviewOutput(BaseModel):
    """Output from the code review step."""

    approved: bool
    issues: list[Issue] = Field(default_factory=list)
    suggestions: list[str] = Field(default_factory=list)
    summary: str = ""
