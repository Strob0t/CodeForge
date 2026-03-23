"""Tests for prompt.evolution.promoted and prompt.evolution.reverted handlers.

Task 3+4 of NATS remaining wiring.
"""

from __future__ import annotations

import json
from unittest.mock import AsyncMock

import pytest

from codeforge.consumer._prompt_evolution import PromptEvolutionHandlerMixin
from codeforge.consumer._subjects import (
    SUBJECT_PROMPT_EVOLUTION_PROMOTED,
    SUBJECT_PROMPT_EVOLUTION_REVERTED,
)


class TestSubjectConstants:
    """Promoted/reverted subject constants must exist and match Go side."""

    def test_promoted_value(self) -> None:
        assert SUBJECT_PROMPT_EVOLUTION_PROMOTED == "prompt.evolution.promoted"

    def test_reverted_value(self) -> None:
        assert SUBJECT_PROMPT_EVOLUTION_REVERTED == "prompt.evolution.reverted"


class _FakeHandler(PromptEvolutionHandlerMixin):
    """Minimal concrete class satisfying mixin dependencies."""

    def __init__(self) -> None:
        self._js = AsyncMock()
        self._llm = AsyncMock()
        self._processed_ids: set[str] = set()


@pytest.fixture
def handler() -> _FakeHandler:
    return _FakeHandler()


class TestHandlePromptPromoted:
    """_handle_prompt_promoted acks and logs."""

    async def test_acks_valid_message(self, handler: _FakeHandler) -> None:
        msg = AsyncMock()
        msg.data = json.dumps(
            {
                "tenant_id": "t1",
                "mode_id": "coder",
                "variant_id": "v1",
                "action": "promoted",
                "old_version": 1,
                "new_version": 2,
            }
        ).encode()
        msg.headers = None

        await handler._handle_prompt_promoted(msg)
        msg.ack.assert_awaited_once()

    async def test_acks_minimal_payload(self, handler: _FakeHandler) -> None:
        msg = AsyncMock()
        msg.data = json.dumps({"mode_id": "coder"}).encode()
        msg.headers = None

        await handler._handle_prompt_promoted(msg)
        msg.ack.assert_awaited_once()

    async def test_acks_on_invalid_json(self, handler: _FakeHandler) -> None:
        msg = AsyncMock()
        msg.data = b"bad json"
        msg.headers = None

        await handler._handle_prompt_promoted(msg)
        msg.ack.assert_awaited_once()


class TestHandlePromptReverted:
    """_handle_prompt_reverted acks and logs."""

    async def test_acks_valid_message(self, handler: _FakeHandler) -> None:
        msg = AsyncMock()
        msg.data = json.dumps(
            {
                "tenant_id": "t1",
                "mode_id": "reviewer",
                "variant_id": "v2",
                "action": "reverted",
                "old_version": 3,
                "new_version": 2,
            }
        ).encode()
        msg.headers = None

        await handler._handle_prompt_reverted(msg)
        msg.ack.assert_awaited_once()

    async def test_acks_on_empty_payload(self, handler: _FakeHandler) -> None:
        msg = AsyncMock()
        msg.data = b"{}"
        msg.headers = None

        await handler._handle_prompt_reverted(msg)
        msg.ack.assert_awaited_once()

    async def test_acks_on_invalid_json(self, handler: _FakeHandler) -> None:
        msg = AsyncMock()
        msg.data = b"not valid"
        msg.headers = None

        await handler._handle_prompt_reverted(msg)
        msg.ack.assert_awaited_once()
