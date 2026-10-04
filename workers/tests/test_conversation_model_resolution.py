"""A conversation run without a model from the Core (KI-125 review).

The Core dispatches without a model when it knows none (no configured model,
no keyed provider or local model discovered): the worker resolves one
(routing, CODEFORGE_DEFAULT_MODEL, its own discovery) before it sizes the
prompt for the model's capability, or fails the run with the reason.
"""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.consumer import TaskConsumer
from codeforge.loop_config import ModelCapability
from codeforge.models import AgentLoopResult, ConversationRunStartMessage
from codeforge.tools.capability import CapabilityLevel
from tests.jetstream_fakes import RecordingJetStream, jetstream_msg


@pytest.fixture
def worker() -> TaskConsumer:
    consumer = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    consumer._js = RecordingJetStream()  # type: ignore[assignment]
    consumer._notifications = consumer._js
    return consumer


def _start_without_model() -> bytes:
    return (
        ConversationRunStartMessage(
            run_id="conv-1",
            conversation_id="conv-1",
            project_id="p1",
            messages=[{"role": "user", "content": "read README.md"}],
            system_prompt="s",
            model="",
        )
        .model_dump_json()
        .encode()
    )


def _completions(worker: TaskConsumer) -> list[dict[str, object]]:
    published = worker._js.published  # type: ignore[union-attr]
    return [json.loads(data) for subject, data in published if subject == "conversation.run.complete"]


async def test_the_prompt_is_sized_for_the_model_the_worker_resolves(
    worker: TaskConsumer, monkeypatch: pytest.MonkeyPatch
) -> None:
    capability_of: list[str] = []

    async def capability(_llm: object, model: str) -> ModelCapability:
        capability_of.append(model)
        return ModelCapability(CapabilityLevel.API_WITH_TOOLS, 32_000)

    monkeypatch.setattr("codeforge.tools.build_default_registry", MagicMock)
    monkeypatch.setattr(worker, "_maybe_prefetch_docs", AsyncMock())
    monkeypatch.setattr(
        worker,
        "_resolve_routing_and_fallbacks",
        AsyncMock(return_value=("ollama/qwen3:4b-instruct", MagicMock(), [])),
    )
    monkeypatch.setattr(
        worker, "_execute_conversation_run", AsyncMock(return_value=AgentLoopResult(final_content="ok"))
    )
    msg, _ = jetstream_msg(_start_without_model(), subject="conversation.run.start")

    with (
        patch("codeforge.consumer._conversation.resolve_model_capability", capability),
        patch("codeforge.consumer._conversation.build_system_prompt", AsyncMock(return_value=("prompt", []))),
        patch("codeforge.consumer._conversation.wire_skill_tools"),
        patch("codeforge.consumer._conversation.register_handoff_tool"),
        patch("codeforge.consumer._conversation.register_propose_goal_tool"),
        patch("codeforge.consumer._conversation.register_propose_roadmap_tool"),
    ):
        await worker._handle_conversation_run(msg)

    assert capability_of == ["ollama/qwen3:4b-instruct"]
    assert [c["status"] for c in _completions(worker)] == ["completed"]


async def test_no_model_anywhere_fails_the_run_with_the_reason(
    worker: TaskConsumer, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr("codeforge.tools.build_default_registry", MagicMock)
    monkeypatch.setattr(worker, "_maybe_prefetch_docs", AsyncMock())
    monkeypatch.setattr(worker, "_build_conversation_messages", AsyncMock(return_value=[]))
    monkeypatch.setattr("codeforge.consumer._conversation_routing.get_hybrid_router", AsyncMock(return_value=None))
    monkeypatch.setattr("codeforge.model_resolver._ModelCache.get_best", lambda _self: "")
    monkeypatch.delenv("CODEFORGE_DEFAULT_MODEL", raising=False)
    msg, _ = jetstream_msg(_start_without_model(), subject="conversation.run.start")

    await worker._handle_conversation_run(msg)

    completions = _completions(worker)
    assert [c["status"] for c in completions] == ["failed"]
    assert "No LLM model available" in str(completions[0]["error"])
