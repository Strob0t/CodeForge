"""Tests for consumer dispatch logic: dedup, base mixin helpers, conversation routing, and subject constants."""

from __future__ import annotations

from typing import ClassVar
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._subjects import (
    HEADER_RETRY_COUNT,
    SUBJECT_A2A_TASK_CANCEL,
    SUBJECT_A2A_TASK_CREATED,
    SUBJECT_BENCHMARK_RUN_REQUEST,
    SUBJECT_BENCHMARK_RUN_RESULT,
    SUBJECT_CONVERSATION_RUN_COMPLETE,
    SUBJECT_CONVERSATION_RUN_START,
    SUBJECT_GRAPH_BUILD_REQUEST,
    SUBJECT_GRAPH_BUILD_RESULT,
    SUBJECT_GRAPH_SEARCH_REQUEST,
    SUBJECT_GRAPH_SEARCH_RESULT,
    SUBJECT_HANDOFF_REQUEST,
    SUBJECT_MEMORY_RECALL,
    SUBJECT_MEMORY_RECALL_RESULT,
    SUBJECT_MEMORY_STORE,
    SUBJECT_REPOMAP_REQUEST,
    SUBJECT_REPOMAP_RESULT,
    SUBJECT_RETRIEVAL_INDEX_REQUEST,
    SUBJECT_RETRIEVAL_INDEX_RESULT,
    SUBJECT_RETRIEVAL_SEARCH_REQUEST,
    SUBJECT_RETRIEVAL_SEARCH_RESULT,
    SUBJECT_SUBAGENT_SEARCH_REQUEST,
    SUBJECT_SUBAGENT_SEARCH_RESULT,
    consumer_name,
)

# ---------------------------------------------------------------------------
# Helpers — isolated mixin for dedup tests (avoids cross-test state leaks)
# ---------------------------------------------------------------------------


class _FreshMixin(ConsumerBaseMixin):
    """Subclass with its own _processed_ids ClassVar to isolate test state."""

    _processed_ids: ClassVar[set[str]] = set()


@pytest.fixture(autouse=True)
def _clear_dedup_state() -> None:
    """Ensure each test starts with a clean dedup set."""
    _FreshMixin._processed_ids = set()


# ---------------------------------------------------------------------------
# 1. Duplicate detection tests
# ---------------------------------------------------------------------------


class TestIsDuplicate:
    """Tests for ConsumerBaseMixin._is_duplicate."""

    def test_first_call_returns_false(self) -> None:
        assert _FreshMixin._is_duplicate("msg-1") is False

    def test_second_call_returns_true(self) -> None:
        _FreshMixin._is_duplicate("msg-2")
        assert _FreshMixin._is_duplicate("msg-2") is True

    def test_different_ids_are_independent(self) -> None:
        _FreshMixin._is_duplicate("msg-a")
        assert _FreshMixin._is_duplicate("msg-b") is False

    def test_clear_processed_allows_reprocessing(self) -> None:
        _FreshMixin._is_duplicate("msg-3")
        assert _FreshMixin._is_duplicate("msg-3") is True
        _FreshMixin._clear_processed("msg-3")
        assert _FreshMixin._is_duplicate("msg-3") is False

    def test_clear_nonexistent_id_is_safe(self) -> None:
        _FreshMixin._clear_processed("does-not-exist")  # should not raise

    def test_duplicate_set_eviction(self) -> None:
        """Exceeding _processed_ids_max evicts roughly half the entries."""
        original_max = _FreshMixin._processed_ids_max
        try:
            _FreshMixin._processed_ids_max = 10
            for i in range(11):
                _FreshMixin._is_duplicate(f"evict-{i}")
            # After eviction, set size should be roughly half of max + 1 new entry
            # (set is unordered, so we can't predict which entries survive)
            assert len(_FreshMixin._processed_ids) <= 10
            assert len(_FreshMixin._processed_ids) >= 5  # at least half survived
        finally:
            _FreshMixin._processed_ids_max = original_max


# ---------------------------------------------------------------------------
# 2. Base mixin tests
# ---------------------------------------------------------------------------


class TestRetryCount:
    """Tests for ConsumerBaseMixin._retry_count."""

    def test_retry_count_extraction(self) -> None:
        msg = MagicMock()
        msg.headers = {HEADER_RETRY_COUNT: "3"}
        assert ConsumerBaseMixin._retry_count(msg) == 3

    def test_retry_count_default_zero_no_headers(self) -> None:
        msg = MagicMock()
        msg.headers = None
        assert ConsumerBaseMixin._retry_count(msg) == 0

    def test_retry_count_default_zero_missing_key(self) -> None:
        msg = MagicMock()
        msg.headers = {"Other-Header": "value"}
        assert ConsumerBaseMixin._retry_count(msg) == 0

    def test_retry_count_invalid_value_returns_zero(self) -> None:
        msg = MagicMock()
        msg.headers = {HEADER_RETRY_COUNT: "not-a-number"}
        assert ConsumerBaseMixin._retry_count(msg) == 0

    def test_retry_count_none_value_returns_zero(self) -> None:
        msg = MagicMock()
        msg.headers = {HEADER_RETRY_COUNT: None}
        assert ConsumerBaseMixin._retry_count(msg) == 0


class TestStampTrust:
    """Tests for ConsumerBaseMixin._stamp_trust."""

    def test_adds_trust_annotation(self) -> None:
        payload: dict[str, object] = {"key": "value"}
        result = ConsumerBaseMixin._stamp_trust(payload)
        assert "trust" in result
        trust = result["trust"]
        assert trust["origin"] == "internal"
        assert trust["trust_level"] == "full"
        assert trust["source_id"] == "python-worker"

    def test_custom_source_id(self) -> None:
        payload: dict[str, object] = {"key": "value"}
        result = ConsumerBaseMixin._stamp_trust(payload, source_id="custom-agent")
        assert result["trust"]["source_id"] == "custom-agent"

    def test_preserves_existing_trust(self) -> None:
        payload: dict[str, object] = {
            "key": "value",
            "trust": {"origin": "external", "trust_level": "partial"},
        }
        result = ConsumerBaseMixin._stamp_trust(payload)
        # Should not overwrite existing trust annotation
        assert result["trust"]["origin"] == "external"
        assert result["trust"]["trust_level"] == "partial"

    def test_returns_same_dict_reference(self) -> None:
        payload: dict[str, object] = {"key": "value"}
        result = ConsumerBaseMixin._stamp_trust(payload)
        assert result is payload


# ---------------------------------------------------------------------------
# 3. Move to DLQ tests
# ---------------------------------------------------------------------------


class TestMoveToDLQ:
    """Tests for ConsumerBaseMixin._move_to_dlq."""

    async def test_publishes_to_dlq_subject(self) -> None:
        mixin = _FreshMixin()
        mixin._js = AsyncMock()
        msg = MagicMock()
        msg.subject = "tasks.agent.aider"
        msg.data = b"payload"
        msg.headers = {"X-Request-ID": "req-1"}
        msg.ack = AsyncMock()

        await mixin._move_to_dlq(msg)

        mixin._js.publish.assert_called_once_with(
            "tasks.agent.aider.dlq",
            b"payload",
            headers={"X-Request-ID": "req-1"},
        )
        msg.ack.assert_called_once()

    async def test_noop_when_js_is_none(self) -> None:
        mixin = _FreshMixin()
        mixin._js = None
        msg = MagicMock()
        msg.subject = "tasks.agent.aider"
        msg.data = b"payload"
        msg.headers = None
        msg.ack = AsyncMock()

        await mixin._move_to_dlq(msg)
        msg.ack.assert_not_called()


# ---------------------------------------------------------------------------
# 4. Conversation dispatch tests
# ---------------------------------------------------------------------------


class TestConversationDispatch:
    """Tests for ConversationHandlerMixin._execute_conversation_run routing."""

    async def test_agentic_true_routes_to_agent_loop(self) -> None:
        """When agentic=True, _execute_conversation_run should invoke AgentLoopExecutor."""
        from codeforge.consumer._conversation import ConversationHandlerMixin
        from codeforge.models import AgentLoopResult, ConversationRunStartMessage

        mixin = type("_TestMixin", (ConversationHandlerMixin,), {})()
        mixin._llm = MagicMock()

        run_msg = ConversationRunStartMessage(
            run_id="run-agentic",
            conversation_id="conv-1",
            project_id="proj-1",
            messages=[],
            system_prompt="You are a helper.",
            model="test-model",
            agentic=True,
        )

        fake_result = AgentLoopResult(
            final_content="done",
            step_count=3,
            total_cost=0.01,
        )

        with patch("codeforge.agent_loop.AgentLoopExecutor") as mock_executor_cls:
            mock_executor = MagicMock()
            mock_executor.run = AsyncMock(return_value=fake_result)
            mock_executor_cls.return_value = mock_executor

            runtime = MagicMock()
            registry = MagicMock()
            routing = MagicMock()
            routing.temperature = 0.2
            routing.tags = []
            routing.routing_layer = ""
            routing.complexity_tier = "simple"
            routing.task_type = "code"

            result = await mixin._execute_conversation_run(
                run_msg=run_msg,
                messages=[{"role": "user", "content": "hi"}],
                primary_model="test-model",
                routing=routing,
                runtime=runtime,
                registry=registry,
                fallback_models=[],
            )

            mock_executor_cls.assert_called_once()
            mock_executor.run.assert_called_once()
            assert result.final_content == "done"

    async def test_agentic_false_routes_to_simple_chat(self) -> None:
        """When agentic=False, _execute_conversation_run should invoke _run_simple_chat."""
        from codeforge.consumer._conversation import ConversationHandlerMixin
        from codeforge.models import AgentLoopResult, ConversationRunStartMessage

        mixin = type("_TestMixin", (ConversationHandlerMixin,), {})()
        mixin._llm = MagicMock()

        run_msg = ConversationRunStartMessage(
            run_id="run-simple",
            conversation_id="conv-2",
            project_id="proj-2",
            messages=[],
            system_prompt="You are a helper.",
            model="test-model",
            agentic=False,
        )

        fake_result = AgentLoopResult(
            final_content="simple response",
            step_count=1,
            total_cost=0.001,
        )

        mixin._run_simple_chat = AsyncMock(return_value=fake_result)

        routing = MagicMock()
        routing.temperature = 0.2
        routing.tags = []

        result = await mixin._execute_conversation_run(
            run_msg=run_msg,
            messages=[{"role": "user", "content": "hello"}],
            primary_model="test-model",
            routing=routing,
            runtime=MagicMock(),
            registry=MagicMock(),
            fallback_models=[],
        )

        mixin._run_simple_chat.assert_called_once()
        assert result.final_content == "simple response"
        assert result.step_count == 1

    async def test_conversation_duplicate_run_skipped(self) -> None:
        """Duplicate run IDs should be acked and skipped."""
        from codeforge.consumer._conversation import ConversationHandlerMixin
        from codeforge.models import ConversationRunStartMessage

        mixin = type("_TestMixin", (ConversationHandlerMixin,), {"_active_runs": set()})()

        run_msg = ConversationRunStartMessage(
            run_id="run-dup",
            conversation_id="conv-dup",
            project_id="proj-1",
            messages=[],
            system_prompt="test",
            model="test-model",
        )

        msg = MagicMock()
        msg.data = run_msg.model_dump_json().encode()
        msg.ack = AsyncMock()
        msg.nak = AsyncMock()

        # Pre-register the run_id as active
        mixin._active_runs.add("run-dup")
        mixin._js = AsyncMock()
        mixin._llm = MagicMock()
        mixin._litellm_key = ""
        mixin._litellm_url = "http://test:4000"
        mixin._db_url = "postgresql://test"

        await mixin._handle_conversation_run(msg)

        msg.ack.assert_called_once()
        # Should NOT have published anything (just skipped)
        mixin._js.publish.assert_not_called()


# ---------------------------------------------------------------------------
# 5. Subject registration / constant consistency tests
# ---------------------------------------------------------------------------


class TestSubjectConstants:
    """Verify all expected subject constants exist and match Go side."""

    def test_conversation_subjects_exist(self) -> None:
        assert SUBJECT_CONVERSATION_RUN_START == "conversation.run.start"
        assert SUBJECT_CONVERSATION_RUN_COMPLETE == "conversation.run.complete"

    def test_benchmark_subjects_exist(self) -> None:
        assert SUBJECT_BENCHMARK_RUN_REQUEST == "benchmark.run.request"
        assert SUBJECT_BENCHMARK_RUN_RESULT == "benchmark.run.result"

    def test_memory_subjects_exist(self) -> None:
        assert SUBJECT_MEMORY_STORE == "memory.store"
        assert SUBJECT_MEMORY_RECALL == "memory.recall"
        assert SUBJECT_MEMORY_RECALL_RESULT == "memory.recall.result"

    def test_a2a_subjects_exist(self) -> None:
        assert SUBJECT_A2A_TASK_CREATED == "a2a.task.created"
        assert SUBJECT_A2A_TASK_CANCEL == "a2a.task.cancel"

    def test_handoff_subject_exists(self) -> None:
        assert SUBJECT_HANDOFF_REQUEST == "handoff.request"

    def test_retrieval_subjects_exist(self) -> None:
        assert SUBJECT_RETRIEVAL_INDEX_REQUEST == "retrieval.index.request"
        assert SUBJECT_RETRIEVAL_INDEX_RESULT == "retrieval.index.result"
        assert SUBJECT_RETRIEVAL_SEARCH_REQUEST == "retrieval.search.request"
        assert SUBJECT_RETRIEVAL_SEARCH_RESULT == "retrieval.search.result"
        assert SUBJECT_SUBAGENT_SEARCH_REQUEST == "retrieval.subagent.request"
        assert SUBJECT_SUBAGENT_SEARCH_RESULT == "retrieval.subagent.result"

    def test_graph_subjects_exist(self) -> None:
        assert SUBJECT_GRAPH_BUILD_REQUEST == "graph.build.request"
        assert SUBJECT_GRAPH_BUILD_RESULT == "graph.build.result"
        assert SUBJECT_GRAPH_SEARCH_REQUEST == "graph.search.request"
        assert SUBJECT_GRAPH_SEARCH_RESULT == "graph.search.result"

    def test_repomap_subjects_exist(self) -> None:
        assert SUBJECT_REPOMAP_REQUEST == "repomap.generate.request"
        assert SUBJECT_REPOMAP_RESULT == "repomap.generate.result"

    def test_go_python_subject_match_conversation(self) -> None:
        """Spot check: Go SubjectConversationRunStart == Python SUBJECT_CONVERSATION_RUN_START."""
        # Go constant value: "conversation.run.start" (from internal/port/messagequeue/queue.go)
        assert SUBJECT_CONVERSATION_RUN_START == "conversation.run.start"

    def test_go_python_subject_match_benchmark(self) -> None:
        """Spot check: Go SubjectBenchmarkRunRequest == Python SUBJECT_BENCHMARK_RUN_REQUEST."""
        assert SUBJECT_BENCHMARK_RUN_REQUEST == "benchmark.run.request"

    def test_go_python_subject_match_memory(self) -> None:
        """Spot check: Go SubjectMemoryStore == Python SUBJECT_MEMORY_STORE."""
        assert SUBJECT_MEMORY_STORE == "memory.store"

    def test_go_python_subject_match_handoff(self) -> None:
        """Spot check: Go SubjectHandoffRequest == Python SUBJECT_HANDOFF_REQUEST."""
        assert SUBJECT_HANDOFF_REQUEST == "handoff.request"

    def test_go_python_subject_match_a2a(self) -> None:
        """Spot check: Go SubjectA2ATaskCreated == Python SUBJECT_A2A_TASK_CREATED."""
        assert SUBJECT_A2A_TASK_CREATED == "a2a.task.created"

    def test_consumer_name_generation(self) -> None:
        """consumer_name should produce deterministic durable names from subjects."""
        assert consumer_name("tasks.agent.*") == "codeforge-py-tasks-agent-all"
        assert consumer_name("conversation.run.start") == "codeforge-py-conversation-run-start"
        assert consumer_name("memory.>") == "codeforge-py-memory-all"
