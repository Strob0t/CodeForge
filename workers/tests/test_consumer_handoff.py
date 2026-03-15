"""Tests for the handoff request handler mixin."""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._handoff import HandoffHandlerMixin

# ---------------------------------------------------------------------------
# Test harness
# ---------------------------------------------------------------------------


class _TestMixin(HandoffHandlerMixin, ConsumerBaseMixin):
    """Lightweight harness combining the mixin with base helpers."""

    def __init__(self) -> None:
        self._js: AsyncMock | None = AsyncMock()
        self._processed_ids: set[str] = set()
        self._processed_ids_max = 10_000


@pytest.fixture(autouse=True)
def _fresh_mixin_state() -> None:
    """Reset class-level dedup set between tests."""
    ConsumerBaseMixin._processed_ids = set()


def _make_msg(data: dict) -> MagicMock:
    msg = MagicMock()
    msg.data = json.dumps(data).encode()
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.headers = {}
    return msg


def _base_payload() -> dict:
    return {
        "target_agent_id": "agent-2",
        "context": "Please continue",
        "target_mode_id": "coder",
        "source_run_id": "run-100",
        "project_id": "proj-1",
        "artifacts": [],
        "plan_id": "",
        "step_id": "",
        "metadata": {},
    }


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


async def test_handoff_success_publishes_run_start() -> None:
    """Valid payload publishes to SUBJECT_RUN_START and acks."""
    mixin = _TestMixin()
    msg = _make_msg(_base_payload())

    await mixin._handle_handoff_request(msg)

    assert mixin._js is not None
    mixin._js.publish.assert_called_once()
    subject = mixin._js.publish.call_args.args[0]
    assert subject == "runs.start"
    msg.ack.assert_called_once()


async def test_handoff_increments_hop_counter() -> None:
    """handoff_hop is incremented from '0' to '1' in the published payload."""
    mixin = _TestMixin()
    payload = _base_payload()
    payload["metadata"] = {"handoff_hop": "0"}
    msg = _make_msg(payload)

    await mixin._handle_handoff_request(msg)

    assert mixin._js is not None
    published_data = json.loads(mixin._js.publish.call_args.args[1])
    assert published_data["config"]["handoff_handoff_hop"] == "1"


async def test_handoff_duplicate_skipped() -> None:
    """Same source_run + target_agent is acked but not published a second time."""
    mixin = _TestMixin()
    msg1 = _make_msg(_base_payload())
    msg2 = _make_msg(_base_payload())

    await mixin._handle_handoff_request(msg1)
    await mixin._handle_handoff_request(msg2)

    assert mixin._js is not None
    assert mixin._js.publish.call_count == 1
    msg2.ack.assert_called_once()


async def test_handoff_includes_artifacts() -> None:
    """Artifacts list appears in the handoff_context prompt."""
    mixin = _TestMixin()
    payload = _base_payload()
    payload["artifacts"] = ["file1.py", "file2.py"]
    msg = _make_msg(payload)

    await mixin._handle_handoff_request(msg)

    assert mixin._js is not None
    published_data = json.loads(mixin._js.publish.call_args.args[1])
    assert "file1.py" in published_data["prompt"]
    assert "file2.py" in published_data["prompt"]


async def test_handoff_stamps_trust() -> None:
    """Published payload contains a trust annotation."""
    mixin = _TestMixin()
    msg = _make_msg(_base_payload())

    await mixin._handle_handoff_request(msg)

    assert mixin._js is not None
    published_data = json.loads(mixin._js.publish.call_args.args[1])
    assert "trust" in published_data
    assert published_data["trust"]["origin"] == "internal"


async def test_handoff_invalid_json_acks() -> None:
    """Invalid msg.data is caught and the message is still acked."""
    mixin = _TestMixin()
    msg = MagicMock()
    msg.data = b"not valid json"
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.headers = {}

    await mixin._handle_handoff_request(msg)

    msg.ack.assert_called_once()
    assert mixin._js is not None
    mixin._js.publish.assert_not_called()


async def test_handoff_no_js_skips_publish() -> None:
    """When _js is None, no publish occurs but the message is still acked."""
    mixin = _TestMixin()
    mixin._js = None
    msg = _make_msg(_base_payload())

    await mixin._handle_handoff_request(msg)

    msg.ack.assert_called_once()


async def test_handoff_plan_step_propagated() -> None:
    """plan_id and step_id appear in the published config."""
    mixin = _TestMixin()
    payload = _base_payload()
    payload["plan_id"] = "plan-42"
    payload["step_id"] = "step-7"
    msg = _make_msg(payload)

    await mixin._handle_handoff_request(msg)

    assert mixin._js is not None
    published_data = json.loads(mixin._js.publish.call_args.args[1])
    assert published_data["config"]["plan_id"] == "plan-42"
    assert published_data["config"]["step_id"] == "step-7"
