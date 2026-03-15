"""Tests for the backend health handler mixin."""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.consumer._backend_health import BackendHealthHandlerMixin
from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._subjects import SUBJECT_BACKEND_HEALTH_RESULT

# ---------------------------------------------------------------------------
# Test harness
# ---------------------------------------------------------------------------


class _TestMixin(BackendHealthHandlerMixin, ConsumerBaseMixin):
    def __init__(self) -> None:
        self._js: AsyncMock | None = AsyncMock()
        self._processed_ids: set[str] = set()
        self._processed_ids_max = 10_000
        self._backend_router = MagicMock()


@pytest.fixture(autouse=True)
def _fresh_state() -> None:
    ConsumerBaseMixin._processed_ids = set()


def _make_msg(data: dict | None = None) -> MagicMock:
    msg = MagicMock()
    msg.data = json.dumps(data or {}).encode()
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.headers = {}
    return msg


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


async def test_backend_health_success() -> None:
    """Health check returns backends list and publishes result."""
    mixin = _TestMixin()
    mixin._backend_router.check_all_health = AsyncMock(return_value=[{"name": "aider", "available": True}])
    mixin._backend_router.get_config_schemas = MagicMock(return_value=[{"name": "aider", "config_fields": []}])
    msg = _make_msg({"request_id": "req-42"})

    await mixin._handle_backend_health(msg)

    assert mixin._js is not None
    mixin._js.publish.assert_called_once()
    subject = mixin._js.publish.call_args.args[0]
    assert subject == SUBJECT_BACKEND_HEALTH_RESULT
    result = json.loads(mixin._js.publish.call_args.args[1])
    assert result["request_id"] == "req-42"
    assert len(result["backends"]) == 1
    msg.ack.assert_called_once()


async def test_backend_health_correct_subject() -> None:
    """Published result goes to the correct NATS subject."""
    mixin = _TestMixin()
    mixin._backend_router.check_all_health = AsyncMock(return_value=[])
    mixin._backend_router.get_config_schemas = MagicMock(return_value=[])
    msg = _make_msg()

    await mixin._handle_backend_health(msg)

    assert mixin._js is not None
    subject = mixin._js.publish.call_args.args[0]
    assert subject == SUBJECT_BACKEND_HEALTH_RESULT


async def test_backend_health_empty_data() -> None:
    """Empty msg.data still processes correctly."""
    mixin = _TestMixin()
    mixin._backend_router.check_all_health = AsyncMock(return_value=[])
    mixin._backend_router.get_config_schemas = MagicMock(return_value=[])

    msg = MagicMock()
    msg.data = b""
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.headers = {}

    await mixin._handle_backend_health(msg)

    msg.ack.assert_called_once()


async def test_backend_health_router_failure() -> None:
    """Router failure causes nak, not ack."""
    mixin = _TestMixin()
    mixin._backend_router.check_all_health = AsyncMock(side_effect=RuntimeError("router down"))
    msg = _make_msg()

    await mixin._handle_backend_health(msg)

    msg.nak.assert_called_once()
    msg.ack.assert_not_called()


async def test_backend_health_request_id_propagated() -> None:
    """request_id from the payload is included in the result."""
    mixin = _TestMixin()
    mixin._backend_router.check_all_health = AsyncMock(return_value=[])
    mixin._backend_router.get_config_schemas = MagicMock(return_value=[])
    msg = _make_msg({"request_id": "propagated-id"})

    await mixin._handle_backend_health(msg)

    assert mixin._js is not None
    result = json.loads(mixin._js.publish.call_args.args[1])
    assert result["request_id"] == "propagated-id"
