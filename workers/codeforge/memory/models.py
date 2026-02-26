"""Pydantic models for the memory subsystem."""

from __future__ import annotations

from enum import StrEnum
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from datetime import datetime

from pydantic import BaseModel, Field


class MemoryKind(StrEnum):
    """Categorizes the type of memory entry."""

    OBSERVATION = "observation"
    DECISION = "decision"
    ERROR = "error"
    INSIGHT = "insight"


class ScoreWeights(BaseModel):
    """Configurable weights for composite memory scoring."""

    semantic: float = 0.5
    recency: float = 0.3
    importance: float = 0.2


class Memory(BaseModel):
    """A single agent memory entry."""

    id: str = ""
    tenant_id: str = ""
    project_id: str
    agent_id: str = ""
    run_id: str = ""
    content: str
    kind: MemoryKind = MemoryKind.OBSERVATION
    importance: float = Field(default=0.5, ge=0.0, le=1.0)
    metadata: dict[str, str] = Field(default_factory=dict)
    created_at: datetime | None = None


class ScoredMemory(BaseModel):
    """A Memory with its composite retrieval score."""

    memory: Memory
    score: float = 0.0


class MemoryStoreRequest(BaseModel):
    """Request to store a new memory (received from NATS)."""

    project_id: str
    agent_id: str = ""
    run_id: str = ""
    content: str
    kind: MemoryKind = MemoryKind.OBSERVATION
    importance: float = Field(default=0.5, ge=0.0, le=1.0)
    metadata: dict[str, str] = Field(default_factory=dict)


class MemoryRecallRequest(BaseModel):
    """Request to recall memories (received from NATS)."""

    project_id: str
    query: str
    top_k: int = 10
    kind: MemoryKind | None = None
