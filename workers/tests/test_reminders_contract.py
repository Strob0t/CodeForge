"""Layer 5b: Python-side NATS contract test for the Reminders field.

Verifies that the ConversationRunStartMessage Pydantic model correctly
deserializes the `reminders` field sent from the Go Core via NATS,
ensuring cross-language compatibility.
"""

from __future__ import annotations

import json

from codeforge.models import ConversationRunStartMessage


def _base_payload(**overrides: object) -> dict:
    """Return a minimal valid ConversationRunStartMessage payload."""
    base: dict = {
        "run_id": "run-1",
        "conversation_id": "conv-1",
        "project_id": "proj-1",
        "system_prompt": "You are an assistant.",
        "model": "gpt-4",
        "agentic": True,
        "messages": [{"role": "user", "content": "hello"}],
    }
    base.update(overrides)
    return base


class TestRemindersDeserialization:
    """Verify reminders field round-trips through JSON correctly."""

    def test_reminders_present(self) -> None:
        """Reminders list deserializes from Go JSON payload."""
        raw = json.dumps(
            _base_payload(
                reminders=[
                    "<system-reminder>Budget at 80%</system-reminder>",
                    "<system-reminder>Stay on topic</system-reminder>",
                ]
            )
        )
        msg = ConversationRunStartMessage.model_validate_json(raw)
        assert len(msg.reminders) == 2
        assert "<system-reminder>" in msg.reminders[0]
        assert "Budget at 80%" in msg.reminders[0]
        assert "Stay on topic" in msg.reminders[1]

    def test_reminders_absent(self) -> None:
        """Missing reminders field defaults to empty list."""
        raw = json.dumps(_base_payload())
        msg = ConversationRunStartMessage.model_validate_json(raw)
        assert msg.reminders == []

    def test_reminders_null(self) -> None:
        """Null reminders field coerces to empty list."""
        raw = json.dumps(_base_payload(reminders=None))
        msg = ConversationRunStartMessage.model_validate_json(raw)
        assert msg.reminders == []

    def test_reminders_empty_list(self) -> None:
        """Empty reminders list stays empty."""
        raw = json.dumps(_base_payload(reminders=[]))
        msg = ConversationRunStartMessage.model_validate_json(raw)
        assert msg.reminders == []

    def test_reminders_serialization_roundtrip(self) -> None:
        """Reminders survive serialize -> deserialize cycle."""
        raw = json.dumps(_base_payload(reminders=["<system-reminder>Check budget</system-reminder>"]))
        original = ConversationRunStartMessage.model_validate_json(raw)
        serialized = original.model_dump_json()
        restored = ConversationRunStartMessage.model_validate_json(serialized)
        assert restored.reminders == original.reminders

    def test_reminders_field_name_matches_go(self) -> None:
        """Field name is 'reminders' matching Go JSON tag."""
        raw = json.dumps(
            _base_payload(
                run_id="r",
                conversation_id="c",
                project_id="p",
                system_prompt="s",
                model="m",
                reminders=["test"],
            )
        )
        msg = ConversationRunStartMessage.model_validate_json(raw)
        data = json.loads(msg.model_dump_json())
        assert "reminders" in data
        assert data["reminders"] == ["test"]
