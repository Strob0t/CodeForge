"""An agentic conversation run needs a workspace directory (KI-193).

Before, an empty ``workspace_path`` was normalised to the worker's own
directory: ``read_file`` and ``list_directory`` read the worker's files
(``.env`` included), outside the tool UID and Landlock.
"""

from __future__ import annotations

import json
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.consumer import TaskConsumer
from codeforge.models import AgentLoopResult, ConversationRunStartMessage
from tests.jetstream_fakes import RecordingJetStream, jetstream_msg

if TYPE_CHECKING:
    from pathlib import Path


@pytest.fixture
def consumer() -> TaskConsumer:
    worker = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    worker._js = RecordingJetStream()  # type: ignore[assignment]
    worker._notifications = worker._js
    return worker


def _payload(workspace: str, *, agentic: bool = True) -> bytes:
    return (
        ConversationRunStartMessage(
            run_id="conv-1",
            conversation_id="conv-1",
            project_id="p1",
            messages=[],
            system_prompt="s",
            model="m",
            workspace_path=workspace,
            agentic=agentic,
        )
        .model_dump_json()
        .encode()
    )


def _patch_pipeline(consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch) -> tuple[MagicMock, AsyncMock]:
    registry = MagicMock()
    monkeypatch.setattr("codeforge.tools.build_default_registry", registry)
    monkeypatch.setattr(consumer, "_maybe_prefetch_docs", AsyncMock())
    monkeypatch.setattr(consumer, "_build_conversation_messages", AsyncMock(return_value=[]))
    monkeypatch.setattr(consumer, "_resolve_routing_and_fallbacks", AsyncMock(return_value=("m", MagicMock(), [])))
    execute = AsyncMock(return_value=AgentLoopResult(final_content="done"))
    monkeypatch.setattr(consumer, "_execute_conversation_run", execute)
    return registry, execute


def _completions(worker: TaskConsumer) -> list[dict[str, object]]:
    return [
        json.loads(data)
        for subject, data in worker._js.published  # type: ignore[union-attr]
        if subject == "conversation.run.complete"
    ]


@pytest.mark.parametrize("kind", ["empty", "blank", "missing", "file"])
async def test_agentic_run_without_a_workspace_directory_fails_before_any_tool(
    consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch, tmp_path: Path, kind: str
) -> None:
    (tmp_path / "file.txt").write_text("x")
    workspace = {
        "empty": "",
        "blank": "   ",
        "missing": str(tmp_path / "missing"),
        "file": str(tmp_path / "file.txt"),
    }[kind]
    registry, execute = _patch_pipeline(consumer, monkeypatch)
    tool_tenant = MagicMock()
    monkeypatch.setattr("codeforge.consumer._conversation.tool_tenant", tool_tenant)
    msg, client = jetstream_msg(_payload(workspace), subject="conversation.run.start")

    await consumer._handle_conversation_run(msg)

    completions = _completions(consumer)
    assert [(c["run_id"], c["status"]) for c in completions] == [("conv-1", "failed")]
    assert "workspace" in str(completions[0]["error"])
    registry.assert_not_called()
    tool_tenant.assert_not_called()
    execute.assert_not_awaited()
    assert client.settlements() == ["ack(sync)"]


async def test_agentic_run_with_a_workspace_directory_runs(
    consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    _, execute = _patch_pipeline(consumer, monkeypatch)
    msg, _ = jetstream_msg(_payload(str(tmp_path)), subject="conversation.run.start")

    await consumer._handle_conversation_run(msg)

    execute.assert_awaited_once()
    assert [c["status"] for c in _completions(consumer)] == ["completed"]


async def test_simple_chat_needs_no_workspace(consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch) -> None:
    _, execute = _patch_pipeline(consumer, monkeypatch)
    msg, _ = jetstream_msg(_payload("", agentic=False), subject="conversation.run.start")

    await consumer._handle_conversation_run(msg)

    execute.assert_awaited_once()
    assert [c["status"] for c in _completions(consumer)] == ["completed"]
