"""Tests for handoff enrichment -- payload fields, trust, chain tracking."""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.consumer._subjects import SUBJECT_HANDOFF_REQUEST
from codeforge.tools.handoff import HANDOFF_TOOL_DEF, execute_handoff

# ---------------------------------------------------------------------------
# Tool tests (handoff.py)
# ---------------------------------------------------------------------------


async def test_handoff_payload_has_plan_fields() -> None:
    """execute_handoff includes plan_id, step_id in payload when provided."""
    published: list[tuple[str, bytes]] = []

    async def fake_publish(subject: str, data: bytes) -> None:
        published.append((subject, data))

    result = await execute_handoff(
        run_id="run-1",
        arguments={
            "target_agent_id": "agent-2",
            "context": "Review this code",
            "plan_id": "plan-42",
            "step_id": "step-7",
        },
        nats_publish=fake_publish,
        workspace_path="/ws",
    )

    assert "initiated" in result
    assert len(published) == 1
    payload = json.loads(published[0][1])
    assert payload["plan_id"] == "plan-42"
    assert payload["step_id"] == "step-7"


async def test_handoff_payload_has_metadata() -> None:
    """execute_handoff includes metadata dict."""
    published: list[tuple[str, bytes]] = []

    async def fake_publish(subject: str, data: bytes) -> None:
        published.append((subject, data))

    result = await execute_handoff(
        run_id="run-1",
        arguments={
            "target_agent_id": "agent-2",
            "context": "Review this code",
            "metadata": {"priority": "high", "reason": "urgent"},
        },
        nats_publish=fake_publish,
        workspace_path="/ws",
    )

    assert "initiated" in result
    payload = json.loads(published[0][1])
    assert payload["metadata"]["priority"] == "high"
    assert payload["metadata"]["reason"] == "urgent"


async def test_handoff_payload_has_chain_tracking() -> None:
    """metadata includes handoff_chain_id and handoff_hop."""
    published: list[tuple[str, bytes]] = []

    async def fake_publish(subject: str, data: bytes) -> None:
        published.append((subject, data))

    # No existing chain — should auto-generate chain_id and start hop at 0
    await execute_handoff(
        run_id="run-1",
        arguments={
            "target_agent_id": "agent-2",
            "context": "Do something",
        },
        nats_publish=fake_publish,
        workspace_path="/ws",
    )

    payload = json.loads(published[0][1])
    meta = payload["metadata"]
    assert "handoff_chain_id" in meta
    assert len(meta["handoff_chain_id"]) > 0  # UUID
    assert meta["handoff_hop"] == "0"


async def test_handoff_chain_tracking_preserves_existing() -> None:
    """When metadata already has handoff_chain_id, it is preserved and hop increments."""
    published: list[tuple[str, bytes]] = []

    async def fake_publish(subject: str, data: bytes) -> None:
        published.append((subject, data))

    await execute_handoff(
        run_id="run-1",
        arguments={
            "target_agent_id": "agent-2",
            "context": "Do something",
            "metadata": {
                "handoff_chain_id": "existing-chain-id",
                "handoff_hop": "3",
            },
        },
        nats_publish=fake_publish,
        workspace_path="/ws",
    )

    payload = json.loads(published[0][1])
    meta = payload["metadata"]
    assert meta["handoff_chain_id"] == "existing-chain-id"
    assert meta["handoff_hop"] == "4"


async def test_handoff_payload_missing_required_fields() -> None:
    """Returns error without target_agent_id or context."""

    async def fake_publish(subject: str, data: bytes) -> None:
        pytest.fail("Should not publish on validation failure")

    # Missing target_agent_id
    result = await execute_handoff(
        run_id="run-1",
        arguments={"context": "Do something"},
        nats_publish=fake_publish,
        workspace_path="/ws",
    )
    assert "Error" in result

    # Missing context
    result = await execute_handoff(
        run_id="run-1",
        arguments={"target_agent_id": "agent-2"},
        nats_publish=fake_publish,
        workspace_path="/ws",
    )
    assert "Error" in result

    # Both missing
    result = await execute_handoff(
        run_id="run-1",
        arguments={},
        nats_publish=fake_publish,
        workspace_path="/ws",
    )
    assert "Error" in result


async def test_handoff_uses_subject_constant() -> None:
    """Verify the constant SUBJECT_HANDOFF_REQUEST is used (published subject matches)."""
    published: list[tuple[str, bytes]] = []

    async def fake_publish(subject: str, data: bytes) -> None:
        published.append((subject, data))

    await execute_handoff(
        run_id="run-1",
        arguments={
            "target_agent_id": "agent-2",
            "context": "Do something",
        },
        nats_publish=fake_publish,
        workspace_path="/ws",
    )

    assert len(published) == 1
    assert published[0][0] == SUBJECT_HANDOFF_REQUEST


async def test_handoff_tool_def_has_new_parameters() -> None:
    """HANDOFF_TOOL_DEF includes plan_id, step_id, metadata parameters."""
    params = HANDOFF_TOOL_DEF["function"]["parameters"]["properties"]
    assert "plan_id" in params
    assert params["plan_id"]["type"] == "string"
    assert "step_id" in params
    assert params["step_id"]["type"] == "string"
    assert "metadata" in params
    assert params["metadata"]["type"] == "object"


async def test_handoff_payload_carries_run_tenant_and_project() -> None:
    """The handoff request carries the source run's tenant and project (KI-12)."""
    published: list[tuple[str, bytes]] = []

    async def fake_publish(subject: str, data: bytes) -> None:
        published.append((subject, data))

    await execute_handoff(
        run_id="run-1",
        arguments={"target_agent_id": "agent-2", "context": "Review this code"},
        nats_publish=fake_publish,
        tenant_id="tenant-a",
        project_id="proj-a",
        workspace_path="/ws",
    )

    payload = json.loads(published[0][1])
    assert payload["tenant_id"] == "tenant-a"
    assert payload["project_id"] == "proj-a"


async def test_registered_handoff_tool_uses_run_tenant_and_project() -> None:
    """register_handoff_tool binds the conversation run's tenant and project to the tool."""
    from codeforge.consumer._conversation_skill_integration import register_handoff_tool

    registry = MagicMock()
    js = MagicMock()
    js.publish = AsyncMock()

    register_handoff_tool(registry, "run-1", js, tenant_id="tenant-a", project_id="proj-a")

    executor = registry.register.call_args.args[1]
    await executor.execute({"target_agent_id": "agent-2", "context": "go"}, "/ws")

    subject, data = js.publish.call_args.args
    assert subject == SUBJECT_HANDOFF_REQUEST
    payload = json.loads(data)
    assert payload["tenant_id"] == "tenant-a"
    assert payload["project_id"] == "proj-a"


# ---------------------------------------------------------------------------
# Workspace and approval timeout (review of KI-21/KI-23)
# ---------------------------------------------------------------------------


async def test_handoff_payload_carries_workspace_and_approval_timeout() -> None:
    """The handoff run works in the source run's workspace and waits as long for approvals."""
    published: list[tuple[str, bytes]] = []

    async def fake_publish(subject: str, data: bytes) -> None:
        published.append((subject, data))

    await execute_handoff(
        run_id="run-1",
        arguments={"target_agent_id": "agent-2", "context": "Review this code"},
        nats_publish=fake_publish,
        workspace_path="/data/workspaces/proj-a",
        approval_timeout_seconds=90,
    )

    payload = json.loads(published[0][1])
    assert payload["workspace_path"] == "/data/workspaces/proj-a"
    assert payload["approval_timeout_seconds"] == 90


@pytest.mark.parametrize("workspace", ["", "   "])
async def test_handoff_without_workspace_fails_at_handoff_time(workspace: str) -> None:
    """Without a workspace the handoff run would fail later: refuse the handoff now."""
    publish = AsyncMock()

    result = await execute_handoff(
        run_id="run-1",
        arguments={"target_agent_id": "agent-2", "context": "Review this code"},
        nats_publish=publish,
        workspace_path=workspace,
    )

    assert result.startswith("Error:")
    assert "workspace" in result
    publish.assert_not_awaited()


async def test_registered_handoff_tool_sends_the_run_workspace_and_approval_timeout() -> None:
    from codeforge.consumer._conversation_skill_integration import register_handoff_tool

    registry = MagicMock()
    js = MagicMock()
    js.publish = AsyncMock()

    register_handoff_tool(registry, "run-1", js, tenant_id="t", project_id="p", approval_timeout_seconds=90)
    executor = registry.register.call_args.args[1]
    await executor.execute({"target_agent_id": "agent-2", "context": "go"}, "/data/workspaces/p")

    payload = json.loads(js.publish.call_args.args[1])
    assert payload["workspace_path"] == "/data/workspaces/p"
    assert payload["approval_timeout_seconds"] == 90


async def test_handoff_metadata_values_are_strings() -> None:
    """S2-G fix, 4: the Go Core reads metadata as string values (map[string]string).

    The LLM supplies a free-form object; a nested or numeric value made the
    request fail to decode in Go and dead-lettered it silently. The worker
    sends every value as a string, JSON-encoding the others.
    """
    published: list[tuple[str, bytes]] = []

    async def fake_publish(subject: str, data: bytes) -> None:
        published.append((subject, data))

    result = await execute_handoff(
        run_id="run-1",
        arguments={
            "target_agent_id": "agent-2",
            "context": "Review this code",
            "metadata": {
                "priority": "high",
                "attempts": 3,
                "score": 0.5,
                "urgent": True,
                "none": None,
                "files": ["a.go", "b.go"],
                "nested": {"depth": 2, "tags": ["x"]},
                "handoff_chain_id": "chain-1",
                "handoff_hop": 1,
            },
        },
        nats_publish=fake_publish,
        workspace_path="/ws",
    )

    assert "initiated" in result
    meta = json.loads(published[0][1])["metadata"]
    assert all(isinstance(value, str) for value in meta.values()), meta
    assert meta["priority"] == "high"
    assert meta["attempts"] == "3"
    assert meta["score"] == "0.5"
    assert meta["urgent"] == "true"
    assert meta["none"] == "null"
    assert json.loads(meta["files"]) == ["a.go", "b.go"]
    assert json.loads(meta["nested"]) == {"depth": 2, "tags": ["x"]}
    assert meta["handoff_hop"] == "2", "a numeric hop counts on"


@pytest.mark.parametrize("metadata", ["not an object", ["a"], {"handoff_chain_id": "c", "handoff_hop": "many"}])
async def test_handoff_with_invalid_metadata_is_refused(metadata: object) -> None:
    published: list[tuple[str, bytes]] = []

    async def fake_publish(subject: str, data: bytes) -> None:
        published.append((subject, data))

    result = await execute_handoff(
        run_id="run-1",
        arguments={"target_agent_id": "agent-2", "context": "Review", "metadata": metadata},
        nats_publish=fake_publish,
        workspace_path="/ws",
    )

    assert result.startswith("Error:")
    assert published == []


async def test_handoff_carries_a_handoff_id() -> None:
    """S2-G fix, 3: the Go Core carries a handoff out once per handoff_id, whatever its redeliveries."""
    published: list[tuple[str, bytes]] = []

    async def fake_publish(subject: str, data: bytes) -> None:
        published.append((subject, data))

    arguments = {"target_agent_id": "agent-2", "context": "Review"}
    await execute_handoff(run_id="run-1", arguments=arguments, nats_publish=fake_publish, workspace_path="/ws")
    await execute_handoff(run_id="run-1", arguments=arguments, nats_publish=fake_publish, workspace_path="/ws")
    await execute_handoff(
        run_id="run-1", arguments=arguments, nats_publish=fake_publish, workspace_path="/ws", handoff_id="given"
    )

    ids = [json.loads(data)["handoff_id"] for _, data in published]
    assert all(ids[:2])
    assert ids[0] != ids[1], "every handoff_to call is a handoff of its own"
    assert ids[2] == "given"
