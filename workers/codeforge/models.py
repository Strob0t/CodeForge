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


# --- Run Protocol Models (Phase 4B) ---


class TerminationConfig(BaseModel):
    """Termination conditions received from the Go control plane."""

    max_steps: int = 50
    timeout_seconds: int = 600
    max_cost: float = 5.0


class RunStartMessage(BaseModel):
    """Message received from NATS when a run is started."""

    run_id: str
    task_id: str
    project_id: str
    agent_id: str
    prompt: str
    policy_profile: str = ""
    exec_mode: str = "mount"
    config: dict[str, str] = Field(default_factory=dict)
    termination: TerminationConfig = Field(default_factory=TerminationConfig)


class ToolCallDecision(BaseModel):
    """Response from Go control plane for a tool call permission request."""

    call_id: str
    decision: str  # allow, deny, ask
    reason: str = ""


class RunCompleteMessage(BaseModel):
    """Completion message sent to Go control plane when a run finishes."""

    run_id: str
    task_id: str
    project_id: str
    status: str = "completed"
    output: str = ""
    error: str = ""
    cost_usd: float = 0.0
    step_count: int = 0
