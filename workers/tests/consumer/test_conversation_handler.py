"""Tests for ConversationHandlerMixin.

Verifies:
- _handle_conversation_run: valid message processing, invalid JSON nack, duplicate dedup
- _publish_completion: correct NATS subject and payload structure
- _build_system_prompt: returns a non-empty string
"""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.consumer._conversation import ConversationHandlerMixin
from codeforge.consumer._subjects import SUBJECT_CONVERSATION_RUN_COMPLETE
from codeforge.models import (
    AgentLoopResult,
    ConversationMessagePayload,
    ConversationRunStartMessage,
)


def _make_valid_run_start(
    run_id: str = "run-001",
    conversation_id: str = "conv-001",
    project_id: str = "proj-001",
) -> ConversationRunStartMessage:
    """Build a minimal valid ConversationRunStartMessage."""
    return ConversationRunStartMessage(
        run_id=run_id,
        conversation_id=conversation_id,
        project_id=project_id,
        messages=[ConversationMessagePayload(role="user", content="Hello")],
        system_prompt="You are a helpful assistant.",
        model="openai/gpt-4o",
    )


def _make_handler() -> ConversationHandlerMixin:
    """Create a ConversationHandlerMixin instance with mocked dependencies.

    The mixin expects attributes from the TaskConsumer hierarchy
    (ConsumerBaseMixin + ConversationHandlerMixin combined):
    _js, _llm, _db_url, _litellm_url, _litellm_key, _experience_pool,
    _stamp_trust (staticmethod from ConsumerBaseMixin).
    """
    handler = ConversationHandlerMixin()
    handler._js = AsyncMock()
    handler._js.publish = AsyncMock()
    handler._llm = AsyncMock()
    handler._db_url = "postgresql://test:test@localhost:5432/test"
    handler._litellm_url = "http://localhost:4000"
    handler._litellm_key = "sk-test"
    handler._experience_pool = None
    # _stamp_trust lives on ConsumerBaseMixin; provide it here since the
    # mixin is tested in isolation (not via TaskConsumer which inherits both).
    from codeforge.consumer._base import ConsumerBaseMixin

    handler._stamp_trust = ConsumerBaseMixin._stamp_trust  # type: ignore[attr-defined]
    return handler


@pytest.fixture(autouse=True)
def _clear_active_runs():
    """Ensure _active_runs is empty before and after each test."""
    ConversationHandlerMixin._active_runs.clear()
    yield
    ConversationHandlerMixin._active_runs.clear()


# ---------------------------------------------------------------------------
# _handle_conversation_run
# ---------------------------------------------------------------------------


class TestHandleConversationRun:
    """Tests for _handle_conversation_run message handler."""

    @pytest.mark.asyncio
    async def test_valid_message_tracks_run_id_and_cleans_up(self) -> None:
        """A valid message should add run_id to _active_runs and clean up after."""
        handler = _make_handler()
        run_msg = _make_valid_run_start(run_id="run-track-test")
        msg = MagicMock()
        msg.data = run_msg.model_dump_json().encode()
        msg.headers = {}
        msg.ack = AsyncMock()
        msg.nak = AsyncMock()

        fake_result = AgentLoopResult(
            final_content="Done",
            step_count=1,
            model="openai/gpt-4o",
        )

        # Capture _active_runs state during execution to prove tracking works.
        tracked_during_exec = False

        async def fake_execute(*_args, **_kwargs):
            nonlocal tracked_during_exec
            tracked_during_exec = "run-track-test" in ConversationHandlerMixin._active_runs
            return fake_result

        with (
            patch.object(handler, "_build_system_prompt", new_callable=AsyncMock, return_value=("prompt", [])),
            patch.object(handler, "_wire_skill_tools"),
            patch.object(handler, "_register_handoff_tool"),
            patch.object(handler, "_register_propose_goal_tool"),
            patch.object(handler, "_get_hybrid_router", new_callable=AsyncMock, return_value=None),
            patch.object(handler, "_build_fallback_chain", new_callable=AsyncMock, return_value=[]),
            patch.object(handler, "_execute_conversation_run", side_effect=fake_execute),
            patch.object(handler, "_publish_completion", new_callable=AsyncMock),
            patch("codeforge.consumer._conversation.RuntimeClient") as mock_runtime_cls,
            patch("codeforge.tools.build_default_registry") as mock_registry_fn,
            patch("codeforge.history.ConversationHistoryManager") as mock_history_cls,
            patch("codeforge.history.HistoryConfig"),
            patch("codeforge.tools.capability.classify_model") as mock_classify,
            patch("asyncio.to_thread") as mock_to_thread,
        ):
            runtime_instance = AsyncMock()
            mock_runtime_cls.return_value = runtime_instance

            mock_registry_fn.return_value = MagicMock()

            history_instance = MagicMock()
            history_instance.build_messages.return_value = [{"role": "system", "content": "prompt"}]
            mock_history_cls.return_value = history_instance

            from codeforge.tools.capability import CapabilityLevel

            mock_classify.return_value = CapabilityLevel.FULL

            mock_routing_result = MagicMock()
            mock_routing_result.model = "openai/gpt-4o"
            mock_routing_result.temperature = 0.7
            mock_routing_result.tags = []
            mock_routing_result.routing_layer = ""
            mock_routing_result.complexity_tier = "simple"
            mock_routing_result.task_type = "code"
            mock_to_thread.return_value = mock_routing_result

            await handler._handle_conversation_run(msg)

        msg.ack.assert_called_once()
        msg.nak.assert_not_called()
        assert tracked_during_exec, "run_id was not tracked in _active_runs during execution"
        # After completion, the run_id should be cleaned up from _active_runs.
        assert "run-track-test" not in ConversationHandlerMixin._active_runs

    @pytest.mark.asyncio
    async def test_invalid_json_publishes_error_and_acks(self) -> None:
        """Invalid JSON data should trigger error publishing and ack (not nak)."""
        handler = _make_handler()
        msg = MagicMock()
        msg.data = b"not valid json {{"
        msg.headers = {}
        msg.ack = AsyncMock()
        msg.nak = AsyncMock()

        # _handle_conversation_run catches Exception, calls _publish_error_result, then acks.
        with patch.object(handler, "_publish_error_result", new_callable=AsyncMock):
            # _publish_error_result in _conversation.py has its own signature: just (msg,)
            await handler._handle_conversation_run(msg)

        # The handler catches Exception and acks after publishing error.
        msg.ack.assert_called_once()

    @pytest.mark.asyncio
    async def test_duplicate_run_id_skipped(self) -> None:
        """A message with a run_id already in _active_runs should be acked and skipped."""
        handler = _make_handler()
        run_msg = _make_valid_run_start(run_id="dup-run-001")

        # Pre-populate _active_runs with the same run_id.
        ConversationHandlerMixin._active_runs.add("dup-run-001")

        msg = MagicMock()
        msg.data = run_msg.model_dump_json().encode()
        msg.headers = {}
        msg.ack = AsyncMock()
        msg.nak = AsyncMock()

        await handler._handle_conversation_run(msg)

        # Duplicate should be acked (not nak'd) and no further processing.
        msg.ack.assert_called_once()
        msg.nak.assert_not_called()
        # _js.publish should NOT have been called (no completion published).
        handler._js.publish.assert_not_called()

    @pytest.mark.asyncio
    async def test_jetstream_unavailable_naks(self) -> None:
        """When _js is None, the handler should nak the message."""
        handler = _make_handler()
        handler._js = None
        run_msg = _make_valid_run_start(run_id="run-no-js")

        msg = MagicMock()
        msg.data = run_msg.model_dump_json().encode()
        msg.headers = {}
        msg.ack = AsyncMock()
        msg.nak = AsyncMock()

        # Without JetStream the handler hits the _js is None guard after dedup,
        # but before that it tries to validate JSON and add to _active_runs.
        # The code path: validate -> dedup check -> _js is None -> nak.
        # However, _publish_error_result also needs _js. Let's see the flow:
        # It will nak because _js is None after the dedup check.
        await handler._handle_conversation_run(msg)

        msg.nak.assert_called_once()
        msg.ack.assert_not_called()


# ---------------------------------------------------------------------------
# _publish_completion
# ---------------------------------------------------------------------------


class TestPublishCompletion:
    """Tests for _publish_completion NATS publishing."""

    @pytest.mark.asyncio
    async def test_publishes_to_correct_subject(self) -> None:
        """Completion should be published to conversation.run.complete."""
        handler = _make_handler()
        # Override _stamp_trust to identity for this test.
        handler._stamp_trust = staticmethod(lambda p, **kw: p)  # type: ignore[assignment]
        run_msg = _make_valid_run_start()
        result = AgentLoopResult(
            final_content="All done",
            total_cost=0.05,
            total_tokens_in=100,
            total_tokens_out=50,
            step_count=3,
            model="openai/gpt-4o",
        )

        await handler._publish_completion(run_msg, result)

        handler._js.publish.assert_called_once()
        call_args = handler._js.publish.call_args
        assert call_args.args[0] == SUBJECT_CONVERSATION_RUN_COMPLETE

    @pytest.mark.asyncio
    async def test_payload_structure(self) -> None:
        """Published payload should contain expected fields from run_msg and result."""
        handler = _make_handler()
        handler._stamp_trust = staticmethod(lambda p, **kw: p)  # type: ignore[assignment]
        run_msg = _make_valid_run_start(
            run_id="run-payload-test",
            conversation_id="conv-payload-test",
        )
        result = AgentLoopResult(
            final_content="Result text",
            total_cost=0.10,
            total_tokens_in=200,
            total_tokens_out=100,
            step_count=5,
            model="openai/gpt-4o",
        )

        await handler._publish_completion(run_msg, result)

        published_data = handler._js.publish.call_args.args[1]
        payload = json.loads(published_data.decode())

        assert payload["run_id"] == "run-payload-test"
        assert payload["conversation_id"] == "conv-payload-test"
        assert payload["assistant_content"] == "Result text"
        assert payload["status"] == "completed"
        assert payload["cost_usd"] == 0.10
        assert payload["tokens_in"] == 200
        assert payload["tokens_out"] == 100
        assert payload["step_count"] == 5
        assert payload["model"] == "openai/gpt-4o"
        assert payload["error"] == ""

    @pytest.mark.asyncio
    async def test_failed_status_on_error(self) -> None:
        """When the result contains an error, status should be 'failed'."""
        handler = _make_handler()
        handler._stamp_trust = staticmethod(lambda p, **kw: p)  # type: ignore[assignment]
        run_msg = _make_valid_run_start()
        result = AgentLoopResult(
            final_content="",
            step_count=0,
            model="openai/gpt-4o",
            error="LLM timeout",
        )

        await handler._publish_completion(run_msg, result)

        published_data = handler._js.publish.call_args.args[1]
        payload = json.loads(published_data.decode())

        assert payload["status"] == "failed"
        assert payload["error"] == "LLM timeout"

    @pytest.mark.asyncio
    async def test_includes_nats_msg_id_header(self) -> None:
        """Published message should include a Nats-Msg-Id header for dedup."""
        handler = _make_handler()
        handler._stamp_trust = staticmethod(lambda p, **kw: p)  # type: ignore[assignment]
        run_msg = _make_valid_run_start()
        result = AgentLoopResult(final_content="ok", step_count=1, model="m")

        await handler._publish_completion(run_msg, result)

        call_kwargs = handler._js.publish.call_args.kwargs
        assert "headers" in call_kwargs
        headers = call_kwargs["headers"]
        assert "Nats-Msg-Id" in headers
        assert headers["Nats-Msg-Id"].startswith("conv-complete-")

    @pytest.mark.asyncio
    async def test_stamp_trust_is_applied(self) -> None:
        """_publish_completion should apply trust stamping to the payload."""
        handler = _make_handler()
        # Replace _stamp_trust with a function that adds a marker.
        handler._stamp_trust = staticmethod(lambda p, **kw: {**p, "_trust_stamped": True})  # type: ignore[assignment]
        run_msg = _make_valid_run_start()
        result = AgentLoopResult(final_content="ok", step_count=1, model="m")

        await handler._publish_completion(run_msg, result)

        published_data = handler._js.publish.call_args.args[1]
        payload = json.loads(published_data.decode())
        assert payload["_trust_stamped"] is True


# ---------------------------------------------------------------------------
# _build_system_prompt
# ---------------------------------------------------------------------------


class TestBuildSystemPrompt:
    """Tests for _build_system_prompt assembly."""

    @pytest.mark.asyncio
    async def test_returns_nonempty_string(self) -> None:
        """_build_system_prompt should return a non-empty system prompt string."""
        handler = _make_handler()
        run_msg = _make_valid_run_start()
        registry = MagicMock()
        log = MagicMock()

        with (
            patch.object(handler, "_inject_skills", new_callable=AsyncMock, return_value=("base prompt", [])),
            patch.object(
                ConversationHandlerMixin,
                "_inject_framework_skills",
                return_value="base prompt",
            ),
            patch.object(
                ConversationHandlerMixin,
                "_inject_tool_guide",
                return_value="base prompt with guide",
            ),
        ):
            prompt, _skills = await handler._build_system_prompt(run_msg, registry, log)

        assert isinstance(prompt, str)
        assert len(prompt) > 0

    @pytest.mark.asyncio
    async def test_includes_microagent_prompts(self) -> None:
        """When microagent_prompts are present, they should be injected."""
        handler = _make_handler()
        run_msg = _make_valid_run_start()
        run_msg.microagent_prompts = ["Do X carefully", "Always check Y"]
        registry = MagicMock()
        log = MagicMock()

        with (
            patch.object(
                handler,
                "_inject_skills",
                new_callable=AsyncMock,
                side_effect=lambda prompt, *a, **kw: (prompt, []),
            ),
            patch.object(
                ConversationHandlerMixin,
                "_inject_framework_skills",
                side_effect=lambda prompt, *a, **kw: prompt,
            ),
            patch.object(
                ConversationHandlerMixin,
                "_inject_tool_guide",
                side_effect=lambda prompt, *a, **kw: prompt,
            ),
        ):
            prompt, _ = await handler._build_system_prompt(run_msg, registry, log)

        assert "Microagent Instructions" in prompt
        assert "Do X carefully" in prompt
        assert "Always check Y" in prompt

    @pytest.mark.asyncio
    async def test_includes_reminders(self) -> None:
        """When reminders are present, they should be injected."""
        handler = _make_handler()
        run_msg = _make_valid_run_start()
        run_msg.reminders = ["Remember to commit"]
        registry = MagicMock()
        log = MagicMock()

        with (
            patch.object(
                handler,
                "_inject_skills",
                new_callable=AsyncMock,
                side_effect=lambda prompt, *a, **kw: (prompt, []),
            ),
            patch.object(
                ConversationHandlerMixin,
                "_inject_framework_skills",
                side_effect=lambda prompt, *a, **kw: prompt,
            ),
            patch.object(
                ConversationHandlerMixin,
                "_inject_tool_guide",
                side_effect=lambda prompt, *a, **kw: prompt,
            ),
        ):
            prompt, _ = await handler._build_system_prompt(run_msg, registry, log)

        assert "System Reminders" in prompt
        assert "Remember to commit" in prompt

    @pytest.mark.asyncio
    async def test_returns_loaded_skills(self) -> None:
        """_build_system_prompt should return loaded skills from _inject_skills."""
        handler = _make_handler()
        run_msg = _make_valid_run_start()
        registry = MagicMock()
        log = MagicMock()

        fake_skills = [MagicMock(name="skill-1"), MagicMock(name="skill-2")]

        with (
            patch.object(
                handler,
                "_inject_skills",
                new_callable=AsyncMock,
                return_value=("prompt with skills", fake_skills),
            ),
            patch.object(
                ConversationHandlerMixin,
                "_inject_framework_skills",
                side_effect=lambda prompt, *a, **kw: prompt,
            ),
            patch.object(
                ConversationHandlerMixin,
                "_inject_tool_guide",
                side_effect=lambda prompt, *a, **kw: prompt,
            ),
        ):
            _, skills = await handler._build_system_prompt(run_msg, registry, log)

        assert skills == fake_skills
        assert len(skills) == 2


# ---------------------------------------------------------------------------
# _inject_session_context (static helper)
# ---------------------------------------------------------------------------


class TestInjectSessionContext:
    """Tests for _inject_session_context static method."""

    def test_no_session_meta_does_nothing(self) -> None:
        """When session_meta is None, messages should not be modified."""
        messages: list[dict[str, str]] = [{"role": "user", "content": "hi"}]
        run_msg = _make_valid_run_start()
        run_msg.session_meta = None
        log = MagicMock()

        ConversationHandlerMixin._inject_session_context(messages, run_msg, log)

        assert len(messages) == 1

    def test_resume_operation_appends_note(self) -> None:
        """A 'resume' session should append a system note."""
        from codeforge.models import SessionMetaPayload

        messages: list[dict[str, str]] = [{"role": "user", "content": "hi"}]
        run_msg = _make_valid_run_start()
        run_msg.session_meta = SessionMetaPayload(operation="resume")
        log = MagicMock()

        ConversationHandlerMixin._inject_session_context(messages, run_msg, log)

        assert len(messages) == 2
        assert messages[1]["role"] == "system"
        assert "resumed" in messages[1]["content"].lower()

    def test_fork_operation_appends_note(self) -> None:
        """A 'fork' session should append a system note."""
        from codeforge.models import SessionMetaPayload

        messages: list[dict[str, str]] = []
        run_msg = _make_valid_run_start()
        run_msg.session_meta = SessionMetaPayload(operation="fork")
        log = MagicMock()

        ConversationHandlerMixin._inject_session_context(messages, run_msg, log)

        assert len(messages) == 1
        assert "forked" in messages[0]["content"].lower()

    def test_unknown_operation_does_nothing(self) -> None:
        """An unknown session operation should not append any note."""
        from codeforge.models import SessionMetaPayload

        messages: list[dict[str, str]] = []
        run_msg = _make_valid_run_start()
        run_msg.session_meta = SessionMetaPayload(operation="unknown_op")
        log = MagicMock()

        ConversationHandlerMixin._inject_session_context(messages, run_msg, log)

        assert len(messages) == 0


# ---------------------------------------------------------------------------
# _publish_error_result (conversation-specific override)
# ---------------------------------------------------------------------------


class TestPublishErrorResult:
    """Tests for the conversation-specific _publish_error_result."""

    @pytest.mark.asyncio
    async def test_publishes_failed_status(self) -> None:
        """_publish_error_result should publish a failed completion message."""
        handler = _make_handler()
        run_msg = _make_valid_run_start(run_id="run-err-001", conversation_id="conv-err-001")

        msg = MagicMock()
        msg.data = run_msg.model_dump_json().encode()

        await handler._publish_error_result(msg)

        handler._js.publish.assert_called_once()
        call_args = handler._js.publish.call_args
        assert call_args.args[0] == SUBJECT_CONVERSATION_RUN_COMPLETE
        payload = json.loads(call_args.args[1].decode())
        assert payload["status"] == "failed"
        assert payload["error"] == "internal worker error"
        assert payload["run_id"] == "run-err-001"

    @pytest.mark.asyncio
    async def test_no_crash_without_jetstream(self) -> None:
        """_publish_error_result should not crash when _js is None."""
        handler = _make_handler()
        handler._js = None
        run_msg = _make_valid_run_start()

        msg = MagicMock()
        msg.data = run_msg.model_dump_json().encode()

        # Should not raise.
        await handler._publish_error_result(msg)

    @pytest.mark.asyncio
    async def test_no_crash_on_invalid_data(self) -> None:
        """_publish_error_result should swallow exceptions from invalid data."""
        handler = _make_handler()
        msg = MagicMock()
        msg.data = b"invalid"

        # Should not raise -- the method has its own try/except.
        await handler._publish_error_result(msg)
