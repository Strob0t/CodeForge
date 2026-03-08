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
    )
    assert "Error" in result

    # Missing context
    result = await execute_handoff(
        run_id="run-1",
        arguments={"target_agent_id": "agent-2"},
        nats_publish=fake_publish,
    )
    assert "Error" in result

    # Both missing
    result = await execute_handoff(
        run_id="run-1",
        arguments={},
        nats_publish=fake_publish,
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


# ---------------------------------------------------------------------------
# Consumer tests (_handoff.py)
# ---------------------------------------------------------------------------


@pytest.fixture
def consumer():
    """Create a minimal consumer-like object with HandoffHandlerMixin."""
    from codeforge.consumer import TaskConsumer

    c = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    c._js = AsyncMock()
    # Reset processed IDs between tests
    c._processed_ids.clear()
    return c


async def test_handoff_consumer_forwards_metadata(consumer) -> None:
    """metadata from incoming payload appears in run_payload config."""
    payload = {
        "source_run_id": "run-src",
        "target_agent_id": "agent-tgt",
        "context": "Do the thing",
        "metadata": {"custom_key": "custom_value"},
    }
    msg = MagicMock()
    msg.data = json.dumps(payload).encode()
    msg.headers = None
    msg.ack = AsyncMock()

    await consumer._handle_handoff_request(msg)

    msg.ack.assert_called()
    publish_call = consumer._js.publish.call_args
    run_payload = json.loads(publish_call.args[1])
    config = run_payload["config"]
    assert config["handoff_custom_key"] == "custom_value"


async def test_handoff_consumer_forwards_plan_id(consumer) -> None:
    """plan_id from incoming payload appears in run_payload config."""
    payload = {
        "source_run_id": "run-src",
        "target_agent_id": "agent-tgt",
        "context": "Do the thing",
        "plan_id": "plan-42",
        "step_id": "step-7",
    }
    msg = MagicMock()
    msg.data = json.dumps(payload).encode()
    msg.headers = None
    msg.ack = AsyncMock()

    await consumer._handle_handoff_request(msg)

    publish_call = consumer._js.publish.call_args
    run_payload = json.loads(publish_call.args[1])
    config = run_payload["config"]
    assert config["plan_id"] == "plan-42"
    assert config["step_id"] == "step-7"


async def test_handoff_consumer_stamps_trust(consumer) -> None:
    """Outgoing run payload has trust annotation."""
    payload = {
        "source_run_id": "run-src",
        "target_agent_id": "agent-tgt",
        "context": "Do the thing",
    }
    msg = MagicMock()
    msg.data = json.dumps(payload).encode()
    msg.headers = None
    msg.ack = AsyncMock()

    await consumer._handle_handoff_request(msg)

    publish_call = consumer._js.publish.call_args
    run_payload = json.loads(publish_call.args[1])
    assert "trust" in run_payload
    assert run_payload["trust"]["origin"] == "internal"
    assert run_payload["trust"]["trust_level"] == "full"


async def test_handoff_consumer_propagates_chain_hop(consumer) -> None:
    """hop counter increments in consumer."""
    payload = {
        "source_run_id": "run-src",
        "target_agent_id": "agent-tgt",
        "context": "Do the thing",
        "metadata": {
            "handoff_chain_id": "chain-1",
            "handoff_hop": "2",
        },
    }
    msg = MagicMock()
    msg.data = json.dumps(payload).encode()
    msg.headers = None
    msg.ack = AsyncMock()

    await consumer._handle_handoff_request(msg)

    publish_call = consumer._js.publish.call_args
    run_payload = json.loads(publish_call.args[1])
    config = run_payload["config"]
    assert config["handoff_handoff_hop"] == "3"
    assert config["handoff_handoff_chain_id"] == "chain-1"
