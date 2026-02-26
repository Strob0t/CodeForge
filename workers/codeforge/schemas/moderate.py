"""Schema for moderation/debate synthesis step."""

from __future__ import annotations

from pydantic import BaseModel, Field


class ModerateInput(BaseModel):
    """Input for the moderation step."""

    proposals: list[str]
    context: str = ""
    criteria: list[str] = Field(default_factory=list)


class ModerateOutput(BaseModel):
    """Output from the moderation step."""

    synthesis: str
    decision: str
    reasoning: str
    accepted_proposals: list[int] = Field(default_factory=list)
    rejected_proposals: list[int] = Field(default_factory=list)
