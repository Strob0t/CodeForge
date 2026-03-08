"""A2A protocol types — mirrors Go internal/domain/a2a/task.go."""

from __future__ import annotations

from enum import StrEnum


class A2ATaskState(StrEnum):
    """Task lifecycle states matching Go TaskState constants exactly."""

    SUBMITTED = "submitted"
    WORKING = "working"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELED = "canceled"
    REJECTED = "rejected"
    INPUT_REQUIRED = "input-required"
    AUTH_REQUIRED = "auth-required"


TERMINAL_STATES: frozenset[A2ATaskState] = frozenset(
    {
        A2ATaskState.COMPLETED,
        A2ATaskState.FAILED,
        A2ATaskState.CANCELED,
        A2ATaskState.REJECTED,
    }
)


def is_terminal(state: A2ATaskState) -> bool:
    """Return True if the state is a terminal (final) state."""
    return state in TERMINAL_STATES


def is_valid_state(state: str) -> bool:
    """Return True if the string is a valid A2A task state."""
    try:
        A2ATaskState(state)
        return True
    except ValueError:
        return False
