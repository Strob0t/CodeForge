"""Tests for ContextEventsHandlerMixin (Task 2 of NATS remaining wiring).

Verifies:
- SUBJECT_SHARED_UPDATED constant matches Go side
- _handle_shared_context_updated acks message and parses payload
- Handler dead-letters and terminates invalid JSON (never NAKs it)
"""

from __future__ import annotations

import json
from unittest.mock import AsyncMock

import pytest

from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._context_events import ContextEventsHandlerMixin
from codeforge.consumer._subjects import SUBJECT_SHARED_UPDATED


class _FakeHandler(ContextEventsHandlerMixin, ConsumerBaseMixin):
    """Minimal concrete class satisfying mixin dependencies."""

    def __init__(self) -> None:
        self._js = AsyncMock()


@pytest.fixture
def handler() -> _FakeHandler:
    return _FakeHandler()


class TestSubjectSharedUpdated:
    """SUBJECT_SHARED_UPDATED constant must match Go side."""

    def test_constant_value(self) -> None:
        assert SUBJECT_SHARED_UPDATED == "context.shared.updated"

    def test_constant_is_string(self) -> None:
        assert isinstance(SUBJECT_SHARED_UPDATED, str)


class TestHandleSharedContextUpdated:
    """_handle_shared_context_updated processes messages correctly."""

    async def test_acks_valid_message(self, handler: _FakeHandler) -> None:
        """Handler must ack a valid message."""
        msg = AsyncMock()
        msg.data = json.dumps(
            {
                "team_id": "team-1",
                "project_id": "proj-1",
                "key": "shared-key",
                "author": "agent-a",
                "version": 3,
            }
        ).encode()
        msg.headers = None

        await handler._handle_shared_context_updated(msg)
        msg.ack.assert_awaited_once()

    async def test_acks_minimal_payload(self, handler: _FakeHandler) -> None:
        """Handler must ack even with minimal fields."""
        msg = AsyncMock()
        msg.data = json.dumps({"team_id": "t1", "key": "k1"}).encode()
        msg.headers = None

        await handler._handle_shared_context_updated(msg)
        msg.ack.assert_awaited_once()

    async def test_dead_letters_invalid_json(self, handler: _FakeHandler) -> None:
        """Invalid JSON goes to the DLQ and is terminated (never NAK'd, so no redelivery loop)."""
        msg = AsyncMock()
        msg.subject = SUBJECT_SHARED_UPDATED
        msg.data = b"not json"
        msg.headers = None

        await handler._handle_shared_context_updated(msg)
        handler._js.publish.assert_awaited_once_with(f"{SUBJECT_SHARED_UPDATED}.dlq", b"not json", headers=None)
        msg.term.assert_awaited_once()
        msg.ack.assert_not_awaited()
        msg.nak.assert_not_awaited()

    async def test_acks_on_empty_payload(self, handler: _FakeHandler) -> None:
        """Handler must ack on empty payload."""
        msg = AsyncMock()
        msg.data = b"{}"
        msg.headers = None

        await handler._handle_shared_context_updated(msg)
        msg.ack.assert_awaited_once()

    async def test_does_not_raise_on_valid_message(self, handler: _FakeHandler) -> None:
        """Handler must not raise on a valid message."""
        msg = AsyncMock()
        msg.data = json.dumps(
            {
                "team_id": "team-1",
                "key": "context-key",
                "author": "agent-b",
                "version": 1,
            }
        ).encode()
        msg.headers = None

        # Should not raise
        await handler._handle_shared_context_updated(msg)
