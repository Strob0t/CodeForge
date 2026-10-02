"""Every handler that starts tool processes runs them as the tenant's tool identity (KI-96).

A handler enters codeforge.tool_identity.tool_tenant once its heartbeat
runs; a refused payload (no tool_uid from an older Go Core, a UID outside
the range, a workspace outside the tenant's directory) ends the work as
failed with the reason, and nothing runs.
"""

from __future__ import annotations

import json
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge import tool_identity, tool_process
from codeforge.consumer import TaskConsumer
from codeforge.models import (
    ConversationRunStartMessage,
    QualityGateRequest,
    QualityGateResult,
    RunStartMessage,
    WorkspaceTestRequest,
    WorkspaceTestResult,
)
from codeforge.tool_identity import ToolIdentity, current_identity
from codeforge.tool_process import IsolationConfig, IsolationStatus
from tests.jetstream_fakes import RecordingJetStream, jetstream_msg

if TYPE_CHECKING:
    from pathlib import Path

REQUIRED = IsolationStatus(
    config=IsolationConfig(mode="required", workspace_root="/data/workspaces"),
    ready=True,
    launcher="/usr/bin/setpriv",
    interpreter="/usr/bin/python3",
)


@pytest.fixture(autouse=True)
def required(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(tool_process, "_status", REQUIRED)


@pytest.fixture
def accepted(monkeypatch: pytest.MonkeyPatch) -> list[tuple[str, int, str | None]]:
    """Accept every payload with a tool_uid (no volumes in unit tests); record what was accepted."""
    calls: list[tuple[str, int, str | None]] = []

    def accept(tenant_id: str, tool_uid: int, workspace: str | None, *, claude_config: bool = False) -> ToolIdentity:
        tool_identity.check_tool_uid(tenant_id, tool_uid)
        calls.append((tenant_id, tool_uid, workspace))
        return ToolIdentity(tenant_id=tenant_id, uid=tool_uid, home=f"/h/{tool_uid}", work_id="t", workspace=workspace)

    async def no_share(_root: str, _identity: ToolIdentity | None = None) -> None:
        return None

    monkeypatch.setattr(tool_identity, "accept_identity", accept)
    monkeypatch.setattr(tool_process, "share_tool_files", no_share)
    return calls


@pytest.fixture
def consumer() -> TaskConsumer:
    worker = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    worker._js = RecordingJetStream()  # type: ignore[assignment]
    worker._notifications = worker._js
    return worker


def _published(worker: TaskConsumer, subject: str) -> list[dict[str, object]]:
    return [json.loads(data) for s, data in worker._js.published if s == subject]  # type: ignore[union-attr]


def _run_start(tool_uid: int) -> tuple[object, object]:
    payload = RunStartMessage(
        run_id="run-1",
        task_id="task-1",
        project_id="p1",
        tenant_id="tenant-a",
        agent_id="a1",
        prompt="fix it",
        workspace_path="/data/workspaces/tenant-a/p1",
        tool_uid=tool_uid,
    )
    return jetstream_msg(payload.model_dump_json().encode(), subject="runs.start", stream_seq=10)


async def test_a_run_runs_as_the_tenants_tool_identity(
    consumer: TaskConsumer, accepted: list[tuple[str, int, str | None]]
) -> None:
    seen: list[ToolIdentity | None] = []

    async def execute(*_args: object, **_kwargs: object) -> None:
        seen.append(current_identity.get())

    consumer._executor = MagicMock()
    consumer._executor.execute_with_runtime = AsyncMock(side_effect=execute)
    msg, _client = _run_start(tool_uid=20003)

    await consumer._handle_run_start(msg)  # type: ignore[arg-type]

    assert accepted == [("tenant-a", 20003, "/data/workspaces/tenant-a/p1")]
    assert [(i.tenant_id, i.uid) for i in seen if i] == [("tenant-a", 20003)]
    assert current_identity.get() is None


async def test_a_run_without_a_tool_uid_fails_and_runs_nothing(
    consumer: TaskConsumer, accepted: list[tuple[str, int, str | None]]
) -> None:
    consumer._executor = MagicMock()
    consumer._executor.execute_with_runtime = AsyncMock()
    msg, client = _run_start(tool_uid=0)

    await consumer._handle_run_start(msg)  # type: ignore[arg-type]

    consumer._executor.execute_with_runtime.assert_not_awaited()
    assert client.settlements() == ["ack(sync)"]  # type: ignore[attr-defined]
    (completion,) = _published(consumer, "runs.complete")
    assert completion["status"] == "failed"
    assert "without tool_uid" in str(completion["error"])
    assert "CODEFORGE_WORKSPACE_TOOL_ACLS=required" in str(completion["error"])


async def test_a_conversation_run_without_a_tool_uid_fails_with_the_reason(
    consumer: TaskConsumer, accepted: list[tuple[str, int, str | None]]
) -> None:
    payload = ConversationRunStartMessage(
        run_id="conv-1",
        conversation_id="conv-1",
        project_id="p1",
        messages=[],
        system_prompt="",
        model="test-model",
        turn_id="turn-1",
        tenant_id="tenant-a",
        workspace_path="/data/workspaces/tenant-a/p1",
        tool_uid=0,
    )
    msg, _client = jetstream_msg(payload.model_dump_json().encode(), subject="conversation.run.start", stream_seq=10)

    await consumer._handle_conversation_run(msg)  # type: ignore[arg-type]

    (completion,) = _published(consumer, "conversation.run.complete")
    assert completion["status"] == "failed"
    assert "without tool_uid" in str(completion["error"])
    assert accepted == []


async def test_a_quality_gate_without_a_tool_uid_has_no_verdict(
    consumer: TaskConsumer, accepted: list[tuple[str, int, str | None]]
) -> None:
    request = QualityGateRequest(
        run_id="run-1",
        project_id="p1",
        tenant_id="tenant-a",
        workspace_path="/data/workspaces/tenant-a/p1",
        run_tests=True,
        test_command="pytest",
    )
    msg = MagicMock()
    msg.data = request.model_dump_json().encode()
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.in_progress = AsyncMock()
    msg.is_acked = False
    consumer._js = AsyncMock()

    with patch.object(consumer._gate_executor, "execute", AsyncMock()) as execute:
        await consumer._handle_quality_gate(msg)

    execute.assert_not_awaited()
    (call,) = [c for c in consumer._js.publish.call_args_list if c.args[0] == "runs.qualitygate.result"]
    result = QualityGateResult.model_validate_json(call.args[1])
    assert result.tests_passed is None
    assert "without tool_uid" in result.error
    msg.ack.assert_called_once()


async def test_a_quality_gate_runs_as_the_tenants_tool_identity(
    consumer: TaskConsumer, accepted: list[tuple[str, int, str | None]]
) -> None:
    request = QualityGateRequest(
        run_id="run-1",
        project_id="p1",
        tenant_id="tenant-a",
        workspace_path="/data/workspaces/tenant-a/p1",
        run_tests=True,
        test_command="pytest",
        tool_uid=20003,
    )
    msg = MagicMock()
    msg.data = request.model_dump_json().encode()
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.in_progress = AsyncMock()
    msg.is_acked = False
    consumer._js = AsyncMock()
    seen: list[ToolIdentity | None] = []

    async def execute(_request: QualityGateRequest) -> QualityGateResult:
        seen.append(current_identity.get())
        return QualityGateResult(run_id="run-1", tests_passed=True)

    with patch.object(consumer._gate_executor, "execute", side_effect=execute):
        await consumer._handle_quality_gate(msg)

    assert [(i.tenant_id, i.uid) for i in seen if i] == [("tenant-a", 20003)]


async def test_a_workspace_test_without_a_tool_uid_runs_nothing(
    consumer: TaskConsumer, accepted: list[tuple[str, int, str | None]], tmp_path: Path
) -> None:
    (tmp_path / "test_feature.py").write_text("def test_x(): pass\n")
    request = WorkspaceTestRequest(
        request_id="req-1",
        tenant_id="tenant-a",
        project_id="p1",
        conversation_id="conv-1",
        workspace_path=str(tmp_path),
        test_file="test_feature.py",
        timeout_seconds=30,
    )
    msg = MagicMock()
    msg.data = request.model_dump_json().encode()
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.term = AsyncMock()
    msg.in_progress = AsyncMock()
    msg.is_acked = False
    consumer._js = AsyncMock()

    with patch.object(consumer._gate_executor, "run_command", AsyncMock()) as run_command:
        await consumer._handle_workspace_test(msg)

    run_command.assert_not_awaited()
    (call,) = [c for c in consumer._js.publish.call_args_list if c.args[0] == "conversation.test.result"]
    result = WorkspaceTestResult.model_validate_json(call.args[1])
    assert result.passed is None
    assert "without tool_uid" in result.error
