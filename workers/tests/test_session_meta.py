"""Tests for session metadata integration."""

from __future__ import annotations

from codeforge.models import ConversationRunStartMessage, SessionMetaPayload


class TestSessionMetaPayload:
    """Tests for the SessionMetaPayload model."""

    def test_defaults(self) -> None:
        meta = SessionMetaPayload()
        assert meta.operation == ""
        assert meta.parent_session_id == ""
        assert meta.parent_run_id == ""
        assert meta.fork_event_id == ""
        assert meta.rewind_event_id == ""

    def test_fork_operation(self) -> None:
        meta = SessionMetaPayload(operation="fork", fork_event_id="e1", parent_run_id="r1")
        assert meta.operation == "fork"
        assert meta.fork_event_id == "e1"
        assert meta.parent_run_id == "r1"

    def test_rewind_operation(self) -> None:
        meta = SessionMetaPayload(operation="rewind", rewind_event_id="e2")
        assert meta.operation == "rewind"
        assert meta.rewind_event_id == "e2"

    def test_resume_operation(self) -> None:
        meta = SessionMetaPayload(operation="resume", parent_run_id="r3")
        assert meta.operation == "resume"
        assert meta.parent_run_id == "r3"


class TestConversationRunStartMessageSessionMeta:
    """Tests for session_meta field on ConversationRunStartMessage."""

    def test_session_meta_none_by_default(self) -> None:
        msg = ConversationRunStartMessage(
            run_id="r1",
            conversation_id="c1",
            project_id="p1",
            messages=[],
            system_prompt="",
            model="gpt-4o",
        )
        assert msg.session_meta is None

    def test_session_meta_present(self) -> None:
        msg = ConversationRunStartMessage(
            run_id="r1",
            conversation_id="c1",
            project_id="p1",
            messages=[],
            system_prompt="",
            model="gpt-4o",
            session_meta=SessionMetaPayload(operation="fork", fork_event_id="e1"),
        )
        assert msg.session_meta is not None
        assert msg.session_meta.operation == "fork"
        assert msg.session_meta.fork_event_id == "e1"

    def test_session_meta_from_json_null(self) -> None:
        data = {
            "run_id": "r1",
            "conversation_id": "c1",
            "project_id": "p1",
            "messages": [],
            "system_prompt": "",
            "model": "gpt-4o",
            "session_meta": None,
        }
        msg = ConversationRunStartMessage.model_validate(data)
        assert msg.session_meta is None

    def test_session_meta_from_json_present(self) -> None:
        data = {
            "run_id": "r1",
            "conversation_id": "c1",
            "project_id": "p1",
            "messages": [],
            "system_prompt": "",
            "model": "gpt-4o",
            "session_meta": {
                "operation": "rewind",
                "rewind_event_id": "e2",
            },
        }
        msg = ConversationRunStartMessage.model_validate(data)
        assert msg.session_meta is not None
        assert msg.session_meta.operation == "rewind"
        assert msg.session_meta.rewind_event_id == "e2"

    def test_session_meta_omitted_in_json(self) -> None:
        """When session_meta key is missing from JSON entirely."""
        data = {
            "run_id": "r1",
            "conversation_id": "c1",
            "project_id": "p1",
            "messages": [],
            "system_prompt": "",
            "model": "gpt-4o",
        }
        msg = ConversationRunStartMessage.model_validate(data)
        assert msg.session_meta is None
