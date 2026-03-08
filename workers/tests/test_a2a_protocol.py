"""Tests for A2A protocol expansion — state enum and handler enrichment."""

from __future__ import annotations

from codeforge.a2a_protocol import TERMINAL_STATES, A2ATaskState, is_terminal, is_valid_state
from codeforge.models import A2ATaskCompleteMessage

# ---------------------------------------------------------------------------
# Enum tests
# ---------------------------------------------------------------------------


class TestA2ATaskStateEnum:
    """A2ATaskState must mirror Go internal/domain/a2a/task.go exactly."""

    def test_all_eight_states_defined(self) -> None:
        """Enum has exactly 8 values."""
        assert len(A2ATaskState) == 8

    def test_state_values_match_go(self) -> None:
        """String values match Go constants exactly."""
        expected = {
            "submitted",
            "working",
            "completed",
            "failed",
            "canceled",
            "rejected",
            "input-required",
            "auth-required",
        }
        actual = {s.value for s in A2ATaskState}
        assert actual == expected

    def test_is_terminal_state(self) -> None:
        """completed, failed, canceled, rejected are terminal."""
        for state in (
            A2ATaskState.COMPLETED,
            A2ATaskState.FAILED,
            A2ATaskState.CANCELED,
            A2ATaskState.REJECTED,
        ):
            assert is_terminal(state), f"{state} should be terminal"

    def test_is_active_state(self) -> None:
        """submitted, working, input-required, auth-required are NOT terminal."""
        for state in (
            A2ATaskState.SUBMITTED,
            A2ATaskState.WORKING,
            A2ATaskState.INPUT_REQUIRED,
            A2ATaskState.AUTH_REQUIRED,
        ):
            assert not is_terminal(state), f"{state} should be active (non-terminal)"

    def test_terminal_states_frozenset(self) -> None:
        """TERMINAL_STATES contains exactly 4 states."""
        assert len(TERMINAL_STATES) == 4
        assert (
            frozenset(
                {
                    A2ATaskState.COMPLETED,
                    A2ATaskState.FAILED,
                    A2ATaskState.CANCELED,
                    A2ATaskState.REJECTED,
                }
            )
            == TERMINAL_STATES
        )


# ---------------------------------------------------------------------------
# Validation tests
# ---------------------------------------------------------------------------


class TestIsValidState:
    """is_valid_state() validates arbitrary strings against the enum."""

    def test_valid_states(self) -> None:
        for value in (
            "submitted",
            "working",
            "completed",
            "failed",
            "canceled",
            "rejected",
            "input-required",
            "auth-required",
        ):
            assert is_valid_state(value), f"{value} should be valid"

    def test_invalid_states(self) -> None:
        for value in ("pending", "running", "done", "error", "", "COMPLETED"):
            assert not is_valid_state(value), f"{value!r} should be invalid"


# ---------------------------------------------------------------------------
# Model tests
# ---------------------------------------------------------------------------


class TestA2ACompleteMessage:
    """A2ATaskCompleteMessage integration with A2ATaskState enum."""

    def test_default_state(self) -> None:
        """Default state is 'completed'."""
        msg = A2ATaskCompleteMessage(task_id="t1")
        assert msg.state == "completed"
        assert msg.state == A2ATaskState.COMPLETED

    def test_all_states_serialize(self) -> None:
        """All 8 states serialize correctly via the model."""
        for state in A2ATaskState:
            msg = A2ATaskCompleteMessage(task_id="t1", state=state.value)
            data = msg.model_dump()
            assert data["state"] == state.value

    def test_state_transition_submitted_to_working(self) -> None:
        """Validates a valid intermediate transition."""
        msg = A2ATaskCompleteMessage(task_id="t1", state=A2ATaskState.WORKING)
        assert msg.state == "working"
        assert not is_terminal(A2ATaskState(msg.state))

    def test_state_transition_working_to_completed(self) -> None:
        """Validates a valid terminal transition."""
        msg = A2ATaskCompleteMessage(task_id="t1", state=A2ATaskState.COMPLETED)
        assert msg.state == "completed"
        assert is_terminal(A2ATaskState(msg.state))

    def test_roundtrip_json(self) -> None:
        """JSON round-trip preserves enum string value."""
        msg = A2ATaskCompleteMessage(
            task_id="t1",
            tenant_id="tenant-abc",
            state=A2ATaskState.FAILED,
            error="timeout",
        )
        json_bytes = msg.model_dump_json()
        restored = A2ATaskCompleteMessage.model_validate_json(json_bytes)
        assert restored.state == "failed"
        assert restored.task_id == "t1"
        assert restored.error == "timeout"
