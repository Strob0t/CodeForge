"""Routing, skills and the docs prefetch use the turn's message (KI-192).

Go sends the whole history with the newest message last. The worker took
the first user message: on turn 5 ("refactor X and run tests") the model
complexity, skills and docs query were decided by turn 1 ("hi"), and the
auto-agent's fix turns were routed by the original feature prompt.
"""

from __future__ import annotations

import json
import os
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.consumer import TaskConsumer
from codeforge.consumer._conversation_prompt_builder import inject_skills, last_user_message
from codeforge.models import AgentLoopResult, ConversationMessagePayload, ConversationRunStartMessage
from tests.jetstream_fakes import RecordingJetStream, jetstream_msg

if TYPE_CHECKING:
    from codeforge.skills.models import Skill

HISTORY = [
    ConversationMessagePayload(role="user", content="hi"),
    ConversationMessagePayload(role="assistant", content="Hello! How can I help?"),
    ConversationMessagePayload(role="user", content="refactor the parser and run the tests"),
    ConversationMessagePayload(role="assistant", content="", tool_calls=[]),
    ConversationMessagePayload(role="tool", content="ok", name="bash", tool_call_id="c1"),
]


@pytest.mark.parametrize(
    ("messages", "want"),
    [
        (HISTORY, "refactor the parser and run the tests"),
        ([], ""),
        ([ConversationMessagePayload(role="assistant", content="x")], ""),
        ([*HISTORY, ConversationMessagePayload(role="user", content="")], "refactor the parser and run the tests"),
    ],
)
def test_last_user_message(messages: list[ConversationMessagePayload], want: str) -> None:
    assert last_user_message(messages) == want


async def test_routing_uses_the_last_user_message(monkeypatch: pytest.MonkeyPatch) -> None:
    worker = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    worker._js = RecordingJetStream()  # type: ignore[assignment]
    worker._notifications = worker._js
    routing = AsyncMock(return_value=("m", MagicMock(), []))
    monkeypatch.setattr("codeforge.tools.build_default_registry", MagicMock)
    monkeypatch.setattr(worker, "_maybe_prefetch_docs", AsyncMock())
    monkeypatch.setattr(worker, "_build_conversation_messages", AsyncMock(return_value=[]))
    monkeypatch.setattr(worker, "_resolve_routing_and_fallbacks", routing)
    monkeypatch.setattr(
        worker, "_execute_conversation_run", AsyncMock(return_value=AgentLoopResult(final_content="ok"))
    )
    payload = ConversationRunStartMessage(
        run_id="conv-1",
        conversation_id="conv-1",
        project_id="p1",
        messages=HISTORY,
        system_prompt="s",
        model="m",
        workspace_path=os.path.dirname(os.path.abspath(__file__)),
    )
    msg, _ = jetstream_msg(payload.model_dump_json().encode(), subject="conversation.run.start")

    await worker._handle_conversation_run(msg)

    assert routing.await_args.args[1] == "refactor the parser and run the tests"
    completions = [json.loads(d) for s, d in worker._js.published if s == "conversation.run.complete"]  # type: ignore[union-attr]
    assert [c["status"] for c in completions] == ["completed"]


async def test_skills_are_selected_for_the_last_user_message() -> None:
    seen: list[str] = []

    def select(skills: list[Skill], task_context: str, *_args: object, **_kwargs: object) -> list[Skill]:
        seen.append(task_context)
        return []

    cursor = AsyncMock()
    cursor.fetchall = AsyncMock(return_value=[])
    conn = MagicMock()
    conn.__aenter__ = AsyncMock(return_value=conn)
    conn.__aexit__ = AsyncMock(return_value=None)
    conn.cursor = MagicMock(return_value=MagicMock(__aenter__=AsyncMock(return_value=cursor), __aexit__=AsyncMock()))

    with (
        patch("psycopg.AsyncConnection.connect", AsyncMock(return_value=conn)),
        patch("codeforge.skills.selector.rank_skills", side_effect=select),
    ):
        await inject_skills("prompt", "p1", HISTORY, "t1", MagicMock(), "postgresql://fake")

    assert seen == ["refactor the parser and run the tests"]
