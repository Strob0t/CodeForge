"""Domain models for task messages exchanged between Go Core and Python Workers."""

from __future__ import annotations

from enum import StrEnum

from pydantic import BaseModel, Field


class TaskStatus(StrEnum):
    """Status of a task in the pipeline."""

    PENDING = "pending"
    QUEUED = "queued"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


class TaskMessage(BaseModel):
    """Message received from NATS when a task is assigned to a worker."""

    id: str
    project_id: str
    title: str
    prompt: str
    config: dict[str, str] = Field(default_factory=dict)


class TaskResult(BaseModel):
    """Result sent back to NATS after task execution."""

    task_id: str
    status: TaskStatus
    output: str = ""
    files: list[str] = Field(default_factory=list)
    error: str = ""
    tokens_in: int = 0
    tokens_out: int = 0
    cost_usd: float = 0.0
