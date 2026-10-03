"""Tests for ConversationHandlerMixin.

Verifies:
- _handle_conversation_run: valid message processing, invalid JSON dead-lettered, duplicate dedup
- _publish_completion: correct NATS subject and payload structure
- _build_system_prompt: returns a non-empty string
"""

from __future__ import annotations

import json
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._conversation import ConversationHandlerMixin
from codeforge.consumer._subjects import SUBJECT_CONVERSATION_RUN_COMPLETE
from codeforge.loop_config import ModelCapability
from codeforge.models import (
    AgentLoopResult,
    ConversationMessagePayload,
    ConversationRunStartMessage,
    ModeConfig,
)
from codeforge.tools.capability import CapabilityLevel

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable

_CAPABILITY = ModelCapability(level=CapabilityLevel.FULL, context_limit=120_000)


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


class _Handler(ConversationHandlerMixin, ConsumerBaseMixin):
    """The conversation mixin with the shared consumer helpers, as in TaskConsumer."""


def _make_handler() -> ConversationHandlerMixin:
    """Create a ConversationHandlerMixin instance with mocked dependencies.

    The mixin expects attributes from the TaskConsumer hierarchy
    (ConsumerBaseMixin + ConversationHandlerMixin combined):
    _js, _llm, _db_url, _litellm_url, _litellm_key, _experience_pool,
    _stamp_trust (staticmethod from ConsumerBaseMixin).
    """
    handler = _Handler()
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


async def _run_with_patched_dependencies(
    handler: ConversationHandlerMixin,
    msg: MagicMock,
    fake_execute: Callable[..., Awaitable[AgentLoopResult]],
) -> MagicMock:
    """Run _handle_conversation_run with every collaborator patched.

    Returns the patched RuntimeClient class so tests can inspect how it was built.
    """
    with (
        patch(
            "codeforge.consumer._conversation.build_system_prompt",
            new_callable=AsyncMock,
            return_value=("prompt", []),
        ),
        patch("codeforge.consumer._conversation.wire_skill_tools"),
        patch("codeforge.consumer._conversation.register_handoff_tool"),
        patch("codeforge.consumer._conversation.register_propose_goal_tool"),
        patch("codeforge.consumer._conversation_routing.get_hybrid_router", new_callable=AsyncMock, return_value=None),
        patch("codeforge.consumer._conversation_routing.build_fallback_chain", new_callable=AsyncMock, return_value=[]),
        patch.object(handler, "_execute_conversation_run", side_effect=fake_execute),
        patch.object(handler, "_publish_completion", new_callable=AsyncMock),
        patch("codeforge.consumer._conversation.RuntimeClient") as mock_runtime_cls,
        patch("codeforge.tools.build_default_registry") as mock_registry_fn,
        patch("codeforge.history.ConversationHistoryManager") as mock_history_cls,
        patch("codeforge.history.HistoryConfig"),
        patch("codeforge.consumer._conversation.resolve_model_capability", AsyncMock(return_value=_CAPABILITY)),
        patch("asyncio.to_thread") as mock_to_thread,
    ):
        runtime_instance = AsyncMock()
        mock_runtime_cls.return_value = runtime_instance

        mock_registry_fn.return_value = MagicMock()

        history_instance = MagicMock()
        history_instance.build_messages.return_value = [{"role": "system", "content": "prompt"}]
        mock_history_cls.return_value = history_instance

        mock_routing_result = MagicMock()
        mock_routing_result.model = "openai/gpt-4o"
        mock_routing_result.temperature = 0.7
        mock_routing_result.tags = []
        mock_routing_result.routing_layer = ""
        mock_routing_result.complexity_tier = "simple"
        mock_routing_result.task_type = "code"
        mock_to_thread.return_value = mock_routing_result

        await handler._handle_conversation_run(msg)
    return mock_runtime_cls


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
        msg.ack_sync = AsyncMock()

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

        await _run_with_patched_dependencies(handler, msg, fake_execute)

        msg.ack_sync.assert_awaited_once()  # accepted with a confirmed ack (ADR-016)
        msg.nak.assert_not_called()
        assert tracked_during_exec, "run_id was not tracked in _active_runs during execution"
        # After completion, the run_id should be cleaned up from _active_runs.
        assert "run-track-test" not in ConversationHandlerMixin._active_runs

    @pytest.mark.asyncio
    async def test_runtime_client_carries_the_run_tenant(self) -> None:
        """Every message the run sends back must carry its tenant (KI-12)."""
        handler = _make_handler()
        run_msg = _make_valid_run_start(run_id="run-tenant-test")
        run_msg.tenant_id = "aaaaaaaa-0000-0000-0000-000000000001"
        msg = MagicMock()
        msg.data = run_msg.model_dump_json().encode()
        msg.headers = {}
        msg.ack = AsyncMock()
        msg.nak = AsyncMock()
        msg.ack_sync = AsyncMock()

        async def fake_execute(*_args, **_kwargs):
            return AgentLoopResult(final_content="Done", step_count=1, model="openai/gpt-4o")

        runtime_cls = await _run_with_patched_dependencies(handler, msg, fake_execute)

        msg.ack_sync.assert_awaited_once()  # accepted with a confirmed ack (ADR-016)
        assert runtime_cls.call_args.kwargs["tenant_id"] == "aaaaaaaa-0000-0000-0000-000000000001"

    @pytest.mark.asyncio
    async def test_spawn_subagent_is_not_offered(self) -> None:
        """spawn_subagent starts nothing yet, so a run does not offer it (KI-25)."""
        handler = _make_handler()
        run_msg = _make_valid_run_start(run_id="run-no-subagent")
        msg = MagicMock()
        msg.data = run_msg.model_dump_json().encode()
        msg.headers = {}
        msg.ack = AsyncMock()
        msg.nak = AsyncMock()
        msg.ack_sync = AsyncMock()

        async def fake_execute(*_args, **_kwargs):
            return AgentLoopResult(final_content="Done", step_count=1, model="openai/gpt-4o")

        with patch("codeforge.tools.spawn_subagent.SpawnSubagentExecutor") as executor_cls:
            await _run_with_patched_dependencies(handler, msg, fake_execute)

        msg.ack_sync.assert_awaited_once()
        executor_cls.assert_not_called()

    @pytest.mark.asyncio
    @pytest.mark.parametrize(("mode", "expected_mode_id"), [(ModeConfig(id="architect"), "architect"), (None, "")])
    async def test_runtime_client_reports_mode(self, mode: ModeConfig | None, expected_mode_id: str) -> None:
        """The RuntimeClient sends the dispatched mode with every tool call (KI-10)."""
        handler = _make_handler()
        run_msg = _make_valid_run_start(run_id="run-mode-test")
        run_msg.mode = mode
        msg = MagicMock()
        msg.data = run_msg.model_dump_json().encode()
        msg.headers = {}
        msg.ack = AsyncMock()
        msg.nak = AsyncMock()
        msg.ack_sync = AsyncMock()

        async def fake_execute(*_args, **_kwargs):
            return AgentLoopResult(final_content="Done", step_count=1, model="openai/gpt-4o")

        runtime_cls = await _run_with_patched_dependencies(handler, msg, fake_execute)

        runtime_cls.assert_called_once()
        assert runtime_cls.call_args.kwargs["mode_id"] == expected_mode_id

    @pytest.mark.asyncio
    @pytest.mark.parametrize("turn_id", ["turn-7", ""])
    async def test_runtime_client_reports_turn(self, turn_id: str) -> None:
        """The RuntimeClient sends the run's turn with every tool call, so Go can
        reject calls of a stopped run of the conversation (review finding 11)."""
        handler = _make_handler()
        run_msg = _make_valid_run_start(run_id="run-turn-test")
        run_msg.turn_id = turn_id
        msg = MagicMock()
        msg.data = run_msg.model_dump_json().encode()
        msg.headers = {}
        msg.ack = AsyncMock()
        msg.nak = AsyncMock()
        msg.ack_sync = AsyncMock()

        async def fake_execute(*_args, **_kwargs):
            return AgentLoopResult(final_content="Done", step_count=1, model="openai/gpt-4o")

        runtime_cls = await _run_with_patched_dependencies(handler, msg, fake_execute)

        runtime_cls.assert_called_once()
        assert runtime_cls.call_args.kwargs["turn_id"] == turn_id

    @pytest.mark.asyncio
    @pytest.mark.parametrize("approval_timeout", [120, 0])
    async def test_runtime_client_waits_for_the_go_approval_timeout(self, approval_timeout: int) -> None:
        """Tool call decisions are awaited longer than Go's HITL approval wait (KI-21)."""
        handler = _make_handler()
        run_msg = _make_valid_run_start(run_id="run-approval-test")
        run_msg.approval_timeout_seconds = approval_timeout
        msg = MagicMock()
        msg.data = run_msg.model_dump_json().encode()
        msg.headers = {}
        msg.ack = AsyncMock()
        msg.nak = AsyncMock()
        msg.ack_sync = AsyncMock()

        async def fake_execute(*_args, **_kwargs):
            return AgentLoopResult(final_content="Done", step_count=1, model="openai/gpt-4o")

        runtime_cls = await _run_with_patched_dependencies(handler, msg, fake_execute)

        runtime_cls.assert_called_once()
        assert runtime_cls.call_args.kwargs["approval_timeout_seconds"] == approval_timeout

    @pytest.mark.asyncio
    @pytest.mark.parametrize(("heartbeat_seconds", "interval"), [(7, 7.0), (0, 30.0)])
    async def test_heartbeats_at_the_go_heartbeat_interval(self, heartbeat_seconds: int, interval: float) -> None:
        """S2-F review, F11: the run beats at Go's runtime.heartbeat_interval (30 s by default)."""
        handler = _make_handler()
        run_msg = _make_valid_run_start(run_id="run-heartbeat-test")
        run_msg.heartbeat_seconds = heartbeat_seconds
        msg = MagicMock()
        msg.data = run_msg.model_dump_json().encode()
        msg.headers = {}
        msg.ack = AsyncMock()
        msg.nak = AsyncMock()
        msg.ack_sync = AsyncMock()

        async def fake_execute(*_args, **_kwargs):
            return AgentLoopResult(final_content="Done", step_count=1, model="openai/gpt-4o")

        runtime_cls = await _run_with_patched_dependencies(handler, msg, fake_execute)

        runtime_cls.return_value.start_heartbeat.assert_awaited_once_with(interval)

    @pytest.mark.asyncio
    async def test_invalid_json_is_dead_lettered_and_terminated(self) -> None:
        """Invalid JSON goes to the DLQ and is terminated: never NAK'd, never run."""
        handler = _make_handler()
        msg = MagicMock()
        msg.subject = "conversation.run.start"
        msg.data = b"not valid json {{"
        msg.headers = {}
        msg.ack = AsyncMock()
        msg.nak = AsyncMock()
        msg.ack_sync = AsyncMock()
        msg.term = AsyncMock()

        await handler._handle_conversation_run(msg)

        handler._js.publish.assert_awaited_once_with("conversation.run.start.dlq", b"not valid json {{", headers=None)
        msg.term.assert_awaited_once()
        msg.ack.assert_not_called()
        msg.nak.assert_not_called()

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
        msg.ack_sync = AsyncMock()

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
        msg.ack_sync = AsyncMock()

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
    @pytest.mark.parametrize("turn_id", ["turn-9", ""])
    async def test_completion_reports_the_turn(self, turn_id: str) -> None:
        """The completion names the run's turn: Go ends the conversation's run
        only when the completion belongs to it (review 2, finding 12)."""
        handler = _make_handler()
        handler._stamp_trust = staticmethod(lambda p, **kw: p)  # type: ignore[assignment]
        run_msg = _make_valid_run_start()
        run_msg.turn_id = turn_id

        await handler._publish_completion(run_msg, AgentLoopResult(final_content="ok", step_count=1, model="m"))

        payload = json.loads(handler._js.publish.call_args.args[1].decode())
        assert payload["turn_id"] == turn_id

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
    """Tests for build_system_prompt assembly (now a standalone function)."""

    @pytest.mark.asyncio
    async def test_returns_nonempty_string(self) -> None:
        """build_system_prompt should return a non-empty system prompt string."""
        run_msg = _make_valid_run_start()
        registry = MagicMock()
        log = MagicMock()

        with (
            patch(
                "codeforge.consumer._conversation_prompt_builder.inject_skills",
                new_callable=AsyncMock,
                return_value=("base prompt", []),
            ),
            patch(
                "codeforge.consumer._conversation_prompt_builder.inject_tool_guide",
                return_value="base prompt with guide",
            ),
        ):
            from codeforge.consumer._conversation_prompt_builder import build_system_prompt

            prompt, _skills = await build_system_prompt(
                run_msg, registry, log, "postgresql://fake", MagicMock(), capability=_CAPABILITY
            )

        assert isinstance(prompt, str)
        assert len(prompt) > 0

    @pytest.mark.asyncio
    async def test_includes_microagent_prompts(self) -> None:
        """When microagent_prompts are present, they should be injected."""
        run_msg = _make_valid_run_start()
        run_msg.microagent_prompts = ["Do X carefully", "Always check Y"]
        registry = MagicMock()
        log = MagicMock()

        with (
            patch(
                "codeforge.consumer._conversation_prompt_builder.inject_skills",
                new_callable=AsyncMock,
                side_effect=lambda prompt, *a, **kw: (prompt, []),
            ),
            patch(
                "codeforge.consumer._conversation_prompt_builder.inject_tool_guide",
                side_effect=lambda prompt, *a, **kw: prompt,
            ),
        ):
            from codeforge.consumer._conversation_prompt_builder import build_system_prompt

            prompt, _ = await build_system_prompt(
                run_msg, registry, log, "postgresql://fake", MagicMock(), capability=_CAPABILITY
            )

        assert "Microagent Instructions" in prompt
        assert "Do X carefully" in prompt
        assert "Always check Y" in prompt

    @pytest.mark.asyncio
    async def test_includes_reminders(self) -> None:
        """When reminders are present, they should be injected."""
        run_msg = _make_valid_run_start()
        run_msg.reminders = ["Remember to commit"]
        registry = MagicMock()
        log = MagicMock()

        with (
            patch(
                "codeforge.consumer._conversation_prompt_builder.inject_skills",
                new_callable=AsyncMock,
                side_effect=lambda prompt, *a, **kw: (prompt, []),
            ),
            patch(
                "codeforge.consumer._conversation_prompt_builder.inject_tool_guide",
                side_effect=lambda prompt, *a, **kw: prompt,
            ),
        ):
            from codeforge.consumer._conversation_prompt_builder import build_system_prompt

            prompt, _ = await build_system_prompt(
                run_msg, registry, log, "postgresql://fake", MagicMock(), capability=_CAPABILITY
            )

        assert "System Reminders" in prompt
        assert "Remember to commit" in prompt

    @pytest.mark.asyncio
    async def test_returns_loaded_skills(self) -> None:
        """build_system_prompt should return loaded skills from inject_skills."""
        run_msg = _make_valid_run_start()
        registry = MagicMock()
        log = MagicMock()

        fake_skills = [MagicMock(name="skill-1"), MagicMock(name="skill-2")]

        with (
            patch(
                "codeforge.consumer._conversation_prompt_builder.inject_skills",
                new_callable=AsyncMock,
                return_value=("prompt with skills", fake_skills),
            ),
            patch(
                "codeforge.consumer._conversation_prompt_builder.inject_tool_guide",
                side_effect=lambda prompt, *a, **kw: prompt,
            ),
        ):
            from codeforge.consumer._conversation_prompt_builder import build_system_prompt

            _, skills = await build_system_prompt(
                run_msg, registry, log, "postgresql://fake", MagicMock(), capability=_CAPABILITY
            )

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
# _publish_failed_completion (last-resort failed completion)
# ---------------------------------------------------------------------------


class TestPublishFailedCompletion:
    """Tests for the failed completion of a run that could not publish its own."""

    @pytest.mark.asyncio
    async def test_publishes_failed_status(self) -> None:
        handler = _make_handler()
        run_msg = _make_valid_run_start(run_id="run-err-001", conversation_id="conv-err-001")

        await handler._publish_failed_completion(run_msg, "internal worker error")

        handler._js.publish.assert_called_once()
        call_args = handler._js.publish.call_args
        assert call_args.args[0] == SUBJECT_CONVERSATION_RUN_COMPLETE
        payload = json.loads(call_args.args[1].decode())
        assert payload["status"] == "failed"
        assert payload["error"] == "internal worker error"
        assert payload["run_id"] == "run-err-001"

    @pytest.mark.asyncio
    async def test_reports_the_turn(self) -> None:
        handler = _make_handler()
        run_msg = _make_valid_run_start()
        run_msg.turn_id = "turn-failed"

        await handler._publish_failed_completion(run_msg, "internal worker error")

        payload = json.loads(handler._js.publish.call_args.args[1].decode())
        assert payload["turn_id"] == "turn-failed"

    @pytest.mark.asyncio
    async def test_no_crash_without_jetstream(self) -> None:
        handler = _make_handler()
        handler._js = None

        # Should not raise.
        await handler._publish_failed_completion(_make_valid_run_start(), "internal worker error")

    @pytest.mark.asyncio
    async def test_publish_failure_is_logged_not_raised(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setattr("codeforge.nats_publish.PUBLISH_BACKOFF_SECONDS", 0.0)
        handler = _make_handler()
        handler._js.publish = AsyncMock(side_effect=ConnectionError("nats down"))

        # Should not raise: this is the last resort, the error is logged.
        await handler._publish_failed_completion(_make_valid_run_start(), "internal worker error")

        assert handler._js.publish.await_count == 3
