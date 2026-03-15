"""Tests for the repomap handler mixin."""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._repomap import RepoMapHandlerMixin
from codeforge.consumer._subjects import SUBJECT_REPOMAP_RESULT
from codeforge.models import RepoMapRequest, RepoMapResult

# ---------------------------------------------------------------------------
# Test harness
# ---------------------------------------------------------------------------


class _TestMixin(RepoMapHandlerMixin, ConsumerBaseMixin):
    def __init__(self) -> None:
        self._js: AsyncMock | None = AsyncMock()
        self._processed_ids: set[str] = set()
        self._processed_ids_max = 10_000
        self._repomap_generator = MagicMock()


@pytest.fixture(autouse=True)
def _fresh_state() -> None:
    ConsumerBaseMixin._processed_ids = set()


def _make_msg(data: dict) -> MagicMock:
    msg = MagicMock()
    msg.data = json.dumps(data).encode()
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.headers = {}
    return msg


def _repomap_payload(project_id: str = "proj-1") -> dict:
    return RepoMapRequest(
        project_id=project_id,
        workspace_path="/tmp/workspace",
        token_budget=2048,
        active_files=["main.py"],
    ).model_dump()


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


async def test_repomap_build_success() -> None:
    """Successful repomap generation publishes result and acks."""
    mixin = _TestMixin()
    result = RepoMapResult(
        project_id="proj-1",
        map_text="file_a.py: class Foo\n",
        token_count=42,
        file_count=1,
        symbol_count=1,
        languages=["python"],
    )
    mixin._repomap_generator.generate = AsyncMock(return_value=result)
    msg = _make_msg(_repomap_payload())

    await mixin._handle_repomap(msg)

    assert mixin._js is not None
    mixin._js.publish.assert_called_once()
    subject = mixin._js.publish.call_args.args[0]
    assert subject == SUBJECT_REPOMAP_RESULT
    published = json.loads(mixin._js.publish.call_args.args[1])
    assert published["project_id"] == "proj-1"
    assert published["file_count"] == 1
    msg.ack.assert_called_once()


async def test_repomap_duplicate_skipped() -> None:
    """Duplicate project_id request is acked but not processed again."""
    mixin = _TestMixin()
    result = RepoMapResult(
        project_id="proj-1",
        map_text="map",
        token_count=10,
        file_count=1,
        symbol_count=1,
        languages=["go"],
    )
    mixin._repomap_generator.generate = AsyncMock(return_value=result)
    msg1 = _make_msg(_repomap_payload())
    msg2 = _make_msg(_repomap_payload())

    await mixin._handle_repomap(msg1)
    await mixin._handle_repomap(msg2)

    assert mixin._js is not None
    assert mixin._js.publish.call_count == 1
    msg2.ack.assert_called_once()


async def test_repomap_failure_naks() -> None:
    """Generator failure causes nak (via _handle_request exception path)."""
    mixin = _TestMixin()
    mixin._repomap_generator.generate = AsyncMock(side_effect=RuntimeError("gen fail"))
    # _token_budget assignment happens before generate, set it up
    mixin._repomap_generator._token_budget = 1024
    msg = _make_msg(_repomap_payload())

    await mixin._handle_repomap(msg)

    msg.nak.assert_called_once()
    msg.ack.assert_not_called()


async def test_repomap_invalid_json() -> None:
    """Invalid JSON causes nak."""
    mixin = _TestMixin()
    msg = MagicMock()
    msg.data = b"not json"
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.headers = {}

    await mixin._handle_repomap(msg)

    msg.nak.assert_called_once()
