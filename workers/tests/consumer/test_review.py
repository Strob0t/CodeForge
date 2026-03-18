"""Tests for ReviewHandlerMixin._do_review_trigger (STUB-003).

Verifies that the review trigger handler dispatches a boundary-analyzer
conversation run via NATS and publishes a completion event.
"""

from __future__ import annotations

import json
from unittest.mock import AsyncMock

import pytest
import structlog

from codeforge.consumer._review import ReviewHandlerMixin
from codeforge.consumer._subjects import (
    SUBJECT_CONVERSATION_RUN_START,
    SUBJECT_REVIEW_TRIGGER_COMPLETE,
)
from codeforge.models import (
    ConversationRunStartMessage,
    ReviewTriggerRequestPayload,
)


class _FakeHandler(ReviewHandlerMixin):
    """Minimal concrete class that satisfies the mixin dependencies."""

    def __init__(self, js: AsyncMock) -> None:
        self._js = js
        self._processed_ids: set[str] = set()

    @staticmethod
    def _stamp_trust(payload: dict, source_id: str = "python-worker") -> dict:
        """No-op trust stamping for tests."""
        payload["_trust"] = {"source_id": source_id}
        return payload


@pytest.fixture
def mock_js() -> AsyncMock:
    """Create a mock JetStream context with an async publish method."""
    js = AsyncMock()
    js.publish = AsyncMock()
    return js


@pytest.fixture
def handler(mock_js: AsyncMock) -> _FakeHandler:
    return _FakeHandler(js=mock_js)


@pytest.fixture
def sample_request() -> ReviewTriggerRequestPayload:
    return ReviewTriggerRequestPayload(
        project_id="proj-123",
        tenant_id="tenant-abc",
        commit_sha="abcdef1234567890",
        source="branch-merge",
    )


@pytest.fixture
def log() -> structlog.BoundLogger:
    return structlog.get_logger().bind(test=True)


# ---------------------------------------------------------------------------
# Happy path tests
# ---------------------------------------------------------------------------


class TestDoReviewTriggerHappyPath:
    """_do_review_trigger publishes correct messages to NATS."""

    async def test_publishes_conversation_run_start(
        self,
        handler: _FakeHandler,
        mock_js: AsyncMock,
        sample_request: ReviewTriggerRequestPayload,
        log: structlog.BoundLogger,
    ) -> None:
        """Must publish a ConversationRunStartMessage to conversation.run.start."""
        await handler._do_review_trigger(sample_request, log)

        # Should have published at least one message to conversation.run.start
        calls = mock_js.publish.call_args_list
        conv_calls = [c for c in calls if c[0][0] == SUBJECT_CONVERSATION_RUN_START]
        assert len(conv_calls) == 1, f"Expected 1 publish to {SUBJECT_CONVERSATION_RUN_START}, got {len(conv_calls)}"

    async def test_conversation_message_is_valid(
        self,
        handler: _FakeHandler,
        mock_js: AsyncMock,
        sample_request: ReviewTriggerRequestPayload,
        log: structlog.BoundLogger,
    ) -> None:
        """Published payload must be a valid ConversationRunStartMessage."""
        await handler._do_review_trigger(sample_request, log)

        calls = mock_js.publish.call_args_list
        conv_calls = [c for c in calls if c[0][0] == SUBJECT_CONVERSATION_RUN_START]
        raw_data = conv_calls[0][0][1]
        payload = json.loads(raw_data)
        # Should parse without error
        msg = ConversationRunStartMessage.model_validate(payload)
        assert msg.project_id == "proj-123"
        assert msg.tenant_id == "tenant-abc"

    async def test_mode_is_boundary_analyzer(
        self,
        handler: _FakeHandler,
        mock_js: AsyncMock,
        sample_request: ReviewTriggerRequestPayload,
        log: structlog.BoundLogger,
    ) -> None:
        """The mode must be set to boundary-analyzer with correct config."""
        await handler._do_review_trigger(sample_request, log)

        calls = mock_js.publish.call_args_list
        conv_calls = [c for c in calls if c[0][0] == SUBJECT_CONVERSATION_RUN_START]
        payload = json.loads(conv_calls[0][0][1])
        msg = ConversationRunStartMessage.model_validate(payload)

        assert msg.mode is not None
        assert msg.mode.id == "boundary-analyzer"
        assert "Read" in msg.mode.tools
        assert "Glob" in msg.mode.tools
        assert "Grep" in msg.mode.tools
        assert "ListDir" in msg.mode.tools
        assert "Write" in msg.mode.denied_tools
        assert "Edit" in msg.mode.denied_tools
        assert "Bash" in msg.mode.denied_tools
        assert msg.mode.llm_scenario == "plan"
        assert msg.mode.required_artifact == "BOUNDARIES.json"

    async def test_agentic_flag_is_true(
        self,
        handler: _FakeHandler,
        mock_js: AsyncMock,
        sample_request: ReviewTriggerRequestPayload,
        log: structlog.BoundLogger,
    ) -> None:
        """The run must be agentic so the conversation handler runs the agent loop."""
        await handler._do_review_trigger(sample_request, log)

        calls = mock_js.publish.call_args_list
        conv_calls = [c for c in calls if c[0][0] == SUBJECT_CONVERSATION_RUN_START]
        payload = json.loads(conv_calls[0][0][1])
        msg = ConversationRunStartMessage.model_validate(payload)
        assert msg.agentic is True

    async def test_run_id_is_unique_uuid(
        self,
        handler: _FakeHandler,
        mock_js: AsyncMock,
        sample_request: ReviewTriggerRequestPayload,
        log: structlog.BoundLogger,
    ) -> None:
        """run_id and conversation_id must be non-empty unique UUIDs."""
        await handler._do_review_trigger(sample_request, log)

        calls = mock_js.publish.call_args_list
        conv_calls = [c for c in calls if c[0][0] == SUBJECT_CONVERSATION_RUN_START]
        payload = json.loads(conv_calls[0][0][1])
        msg = ConversationRunStartMessage.model_validate(payload)

        assert len(msg.run_id) > 0
        assert len(msg.conversation_id) > 0
        # run_id should equal conversation_id (architecture constraint)
        assert msg.run_id == msg.conversation_id

    async def test_system_prompt_mentions_boundary_analysis(
        self,
        handler: _FakeHandler,
        mock_js: AsyncMock,
        sample_request: ReviewTriggerRequestPayload,
        log: structlog.BoundLogger,
    ) -> None:
        """System prompt must instruct the agent to perform boundary analysis."""
        await handler._do_review_trigger(sample_request, log)

        calls = mock_js.publish.call_args_list
        conv_calls = [c for c in calls if c[0][0] == SUBJECT_CONVERSATION_RUN_START]
        payload = json.loads(conv_calls[0][0][1])
        msg = ConversationRunStartMessage.model_validate(payload)

        prompt_lower = msg.system_prompt.lower()
        assert "boundar" in prompt_lower  # boundary / boundaries
        assert "boundaries.json" in prompt_lower or "BOUNDARIES.json" in msg.system_prompt

    async def test_user_message_includes_commit_sha(
        self,
        handler: _FakeHandler,
        mock_js: AsyncMock,
        sample_request: ReviewTriggerRequestPayload,
        log: structlog.BoundLogger,
    ) -> None:
        """The user message should reference the commit SHA and project."""
        await handler._do_review_trigger(sample_request, log)

        calls = mock_js.publish.call_args_list
        conv_calls = [c for c in calls if c[0][0] == SUBJECT_CONVERSATION_RUN_START]
        payload = json.loads(conv_calls[0][0][1])
        msg = ConversationRunStartMessage.model_validate(payload)

        assert len(msg.messages) >= 1
        user_msgs = [m for m in msg.messages if m.role == "user"]
        assert len(user_msgs) >= 1
        user_content = user_msgs[0].content
        assert sample_request.commit_sha in user_content

    async def test_termination_has_reasonable_limits(
        self,
        handler: _FakeHandler,
        mock_js: AsyncMock,
        sample_request: ReviewTriggerRequestPayload,
        log: structlog.BoundLogger,
    ) -> None:
        """Termination config must have reasonable step/cost limits."""
        await handler._do_review_trigger(sample_request, log)

        calls = mock_js.publish.call_args_list
        conv_calls = [c for c in calls if c[0][0] == SUBJECT_CONVERSATION_RUN_START]
        payload = json.loads(conv_calls[0][0][1])
        msg = ConversationRunStartMessage.model_validate(payload)

        assert msg.termination.max_steps >= 10
        assert msg.termination.max_steps <= 100
        assert msg.termination.max_cost > 0
        assert msg.termination.timeout_seconds > 0

    async def test_publishes_review_trigger_complete(
        self,
        handler: _FakeHandler,
        mock_js: AsyncMock,
        sample_request: ReviewTriggerRequestPayload,
        log: structlog.BoundLogger,
    ) -> None:
        """Must publish a completion event to review.trigger.complete."""
        await handler._do_review_trigger(sample_request, log)

        calls = mock_js.publish.call_args_list
        complete_calls = [c for c in calls if c[0][0] == SUBJECT_REVIEW_TRIGGER_COMPLETE]
        assert len(complete_calls) == 1, (
            f"Expected 1 publish to {SUBJECT_REVIEW_TRIGGER_COMPLETE}, got {len(complete_calls)}"
        )

        raw_data = complete_calls[0][0][1]
        payload = json.loads(raw_data)
        assert payload["project_id"] == "proj-123"
        assert payload["tenant_id"] == "tenant-abc"
        assert payload["commit_sha"] == "abcdef1234567890"
        assert payload.get("status") == "dispatched"

    async def test_nats_msg_id_header_is_unique(
        self,
        handler: _FakeHandler,
        mock_js: AsyncMock,
        sample_request: ReviewTriggerRequestPayload,
        log: structlog.BoundLogger,
    ) -> None:
        """NATS publish calls should include unique Nats-Msg-Id headers for dedup."""
        await handler._do_review_trigger(sample_request, log)

        calls = mock_js.publish.call_args_list
        conv_calls = [c for c in calls if c[0][0] == SUBJECT_CONVERSATION_RUN_START]
        assert len(conv_calls) == 1
        # headers may be a keyword arg
        call_kwargs = conv_calls[0][1] or {}
        headers = call_kwargs.get("headers", {})
        assert "Nats-Msg-Id" in headers
        assert len(headers["Nats-Msg-Id"]) > 0

    async def test_trust_stamp_applied(
        self,
        handler: _FakeHandler,
        mock_js: AsyncMock,
        sample_request: ReviewTriggerRequestPayload,
        log: structlog.BoundLogger,
    ) -> None:
        """Outgoing payload should have trust annotations stamped."""
        await handler._do_review_trigger(sample_request, log)

        calls = mock_js.publish.call_args_list
        conv_calls = [c for c in calls if c[0][0] == SUBJECT_CONVERSATION_RUN_START]
        payload = json.loads(conv_calls[0][0][1])
        assert "_trust" in payload


# ---------------------------------------------------------------------------
# Edge case tests
# ---------------------------------------------------------------------------


class TestDoReviewTriggerEdgeCases:
    """Edge cases for _do_review_trigger."""

    async def test_js_none_does_not_raise(
        self,
        sample_request: ReviewTriggerRequestPayload,
        log: structlog.BoundLogger,
    ) -> None:
        """When JetStream is unavailable, handler should log but not crash."""
        handler = _FakeHandler(js=None)  # type: ignore[arg-type]
        handler._js = None
        # Should not raise
        await handler._do_review_trigger(sample_request, log)

    async def test_empty_commit_sha_still_dispatches(
        self,
        handler: _FakeHandler,
        mock_js: AsyncMock,
        log: structlog.BoundLogger,
    ) -> None:
        """An empty commit_sha should still dispatch (defensive)."""
        request = ReviewTriggerRequestPayload(
            project_id="proj-456",
            tenant_id="tenant-xyz",
            commit_sha="",
            source="manual",
        )
        await handler._do_review_trigger(request, log)

        calls = mock_js.publish.call_args_list
        conv_calls = [c for c in calls if c[0][0] == SUBJECT_CONVERSATION_RUN_START]
        assert len(conv_calls) == 1

    async def test_two_calls_produce_different_run_ids(
        self,
        handler: _FakeHandler,
        mock_js: AsyncMock,
        log: structlog.BoundLogger,
    ) -> None:
        """Each dispatch must generate a unique run_id."""
        req1 = ReviewTriggerRequestPayload(project_id="proj-1", tenant_id="t-1", commit_sha="aaa", source="manual")
        req2 = ReviewTriggerRequestPayload(project_id="proj-2", tenant_id="t-2", commit_sha="bbb", source="manual")
        await handler._do_review_trigger(req1, log)
        await handler._do_review_trigger(req2, log)

        calls = mock_js.publish.call_args_list
        conv_calls = [c for c in calls if c[0][0] == SUBJECT_CONVERSATION_RUN_START]
        assert len(conv_calls) == 2

        payload1 = json.loads(conv_calls[0][0][1])
        payload2 = json.loads(conv_calls[1][0][1])
        assert payload1["run_id"] != payload2["run_id"]

    async def test_policy_profile_is_set(
        self,
        handler: _FakeHandler,
        mock_js: AsyncMock,
        sample_request: ReviewTriggerRequestPayload,
        log: structlog.BoundLogger,
    ) -> None:
        """Policy profile should be set (non-empty) for safety."""
        await handler._do_review_trigger(sample_request, log)

        calls = mock_js.publish.call_args_list
        conv_calls = [c for c in calls if c[0][0] == SUBJECT_CONVERSATION_RUN_START]
        payload = json.loads(conv_calls[0][0][1])
        msg = ConversationRunStartMessage.model_validate(payload)
        assert len(msg.policy_profile) > 0
