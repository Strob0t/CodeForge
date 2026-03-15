"""Tests for the retrieval index, search, and sub-agent handler mixins."""

from __future__ import annotations

import json
from dataclasses import dataclass
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._retrieval import RetrievalHandlerMixin
from codeforge.consumer._subjects import (
    SUBJECT_RETRIEVAL_INDEX_RESULT,
    SUBJECT_RETRIEVAL_SEARCH_RESULT,
    SUBJECT_SUBAGENT_SEARCH_RESULT,
)
from codeforge.models import (
    RetrievalIndexRequest,
    RetrievalSearchRequest,
    SubAgentSearchRequest,
)

# ---------------------------------------------------------------------------
# Test harness
# ---------------------------------------------------------------------------


class _TestMixin(RetrievalHandlerMixin, ConsumerBaseMixin):
    def __init__(self) -> None:
        self._js: AsyncMock | None = AsyncMock()
        self._processed_ids: set[str] = set()
        self._processed_ids_max = 10_000
        self._retriever = MagicMock()
        self._subagent = MagicMock()


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


@dataclass
class _FakeIndexStatus:
    project_id: str = "proj-1"
    status: str = "ready"
    file_count: int = 10
    chunk_count: int = 100
    embedding_model: str = "text-embedding-3-small"
    error: str = ""
    incremental: bool = False
    files_changed: int = 5
    files_unchanged: int = 5


def _index_payload(project_id: str = "proj-1") -> dict:
    return RetrievalIndexRequest(
        project_id=project_id,
        workspace_path="/tmp/workspace",
    ).model_dump()


def _search_payload(request_id: str = "req-1") -> dict:
    return RetrievalSearchRequest(
        project_id="proj-1",
        query="How does authentication work?",
        request_id=request_id,
    ).model_dump()


def _subagent_payload(request_id: str = "sub-1") -> dict:
    return SubAgentSearchRequest(
        project_id="proj-1",
        query="Find authentication logic",
        request_id=request_id,
    ).model_dump()


# ---------------------------------------------------------------------------
# Retrieval Index Tests
# ---------------------------------------------------------------------------


async def test_retrieval_index_success() -> None:
    """Successful index build publishes result and acks."""
    mixin = _TestMixin()
    mixin._retriever.build_index = AsyncMock(return_value=_FakeIndexStatus())
    msg = _make_msg(_index_payload())

    await mixin._handle_retrieval_index(msg)

    assert mixin._js is not None
    mixin._js.publish.assert_called_once()
    subject = mixin._js.publish.call_args.args[0]
    assert subject == SUBJECT_RETRIEVAL_INDEX_RESULT
    result = json.loads(mixin._js.publish.call_args.args[1])
    assert result["status"] == "ready"
    assert result["file_count"] == 10
    msg.ack.assert_called_once()


async def test_retrieval_index_duplicate() -> None:
    """Duplicate index request is acked but not processed again."""
    mixin = _TestMixin()
    mixin._retriever.build_index = AsyncMock(return_value=_FakeIndexStatus())
    msg1 = _make_msg(_index_payload())
    msg2 = _make_msg(_index_payload())

    await mixin._handle_retrieval_index(msg1)
    await mixin._handle_retrieval_index(msg2)

    assert mixin._js is not None
    assert mixin._js.publish.call_count == 1
    msg2.ack.assert_called_once()


async def test_retrieval_index_invalid_json() -> None:
    """Invalid JSON causes nak."""
    mixin = _TestMixin()
    msg = MagicMock()
    msg.data = b"not json"
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.headers = {}

    await mixin._handle_retrieval_index(msg)

    msg.nak.assert_called_once()


# ---------------------------------------------------------------------------
# Retrieval Search Tests
# ---------------------------------------------------------------------------


async def test_retrieval_search_success() -> None:
    """Successful search publishes results and acks."""
    mixin = _TestMixin()
    mixin._retriever.search = AsyncMock(return_value=[])
    msg = _make_msg(_search_payload())

    await mixin._handle_retrieval_search(msg)

    assert mixin._js is not None
    mixin._js.publish.assert_called_once()
    subject = mixin._js.publish.call_args.args[0]
    assert subject == SUBJECT_RETRIEVAL_SEARCH_RESULT
    msg.ack.assert_called_once()


async def test_retrieval_search_failure_publishes_error() -> None:
    """Search failure publishes error result so Go waiter gets a response, then naks."""
    mixin = _TestMixin()
    mixin._retriever.search = AsyncMock(side_effect=RuntimeError("search fail"))
    msg = _make_msg(_search_payload())

    await mixin._handle_retrieval_search(msg)

    assert mixin._js is not None
    error_published = False
    for call in mixin._js.publish.call_args_list:
        if call.args[0] == SUBJECT_RETRIEVAL_SEARCH_RESULT:
            result = json.loads(call.args[1])
            if result.get("error"):
                error_published = True
    assert error_published
    msg.nak.assert_called_once()


async def test_retrieval_search_error_shape() -> None:
    """Error result includes project_id, query, and request_id."""
    mixin = _TestMixin()
    mixin._retriever.search = AsyncMock(side_effect=RuntimeError("fail"))
    msg = _make_msg(_search_payload("req-shape"))

    await mixin._handle_retrieval_search(msg)

    assert mixin._js is not None
    for call in mixin._js.publish.call_args_list:
        if call.args[0] == SUBJECT_RETRIEVAL_SEARCH_RESULT:
            result = json.loads(call.args[1])
            assert result["project_id"] == "proj-1"
            assert result["request_id"] == "req-shape"
            assert "error" in result


# ---------------------------------------------------------------------------
# Subagent Search Tests
# ---------------------------------------------------------------------------


@dataclass
class _FakeCost:
    model: str = "fake-model"
    tokens_in: int = 50
    tokens_out: int = 25
    cost_usd: float = 0.005


async def test_subagent_search_success() -> None:
    """Successful subagent search publishes results and acks."""
    mixin = _TestMixin()
    mixin._subagent.search = AsyncMock(return_value=([], ["expanded q1"], 10))
    mixin._subagent.last_cost = _FakeCost()
    msg = _make_msg(_subagent_payload())

    await mixin._handle_subagent_search(msg)

    assert mixin._js is not None
    mixin._js.publish.assert_called_once()
    subject = mixin._js.publish.call_args.args[0]
    assert subject == SUBJECT_SUBAGENT_SEARCH_RESULT
    result = json.loads(mixin._js.publish.call_args.args[1])
    assert result["expanded_queries"] == ["expanded q1"]
    assert result["total_candidates"] == 10
    msg.ack.assert_called_once()


async def test_subagent_search_failure() -> None:
    """Subagent search failure publishes error result and naks."""
    mixin = _TestMixin()
    mixin._subagent.search = AsyncMock(side_effect=RuntimeError("subagent fail"))
    msg = _make_msg(_subagent_payload())

    await mixin._handle_subagent_search(msg)

    assert mixin._js is not None
    error_published = False
    for call in mixin._js.publish.call_args_list:
        if call.args[0] == SUBJECT_SUBAGENT_SEARCH_RESULT:
            result = json.loads(call.args[1])
            if result.get("error"):
                error_published = True
    assert error_published
    msg.nak.assert_called_once()


async def test_subagent_search_duplicate() -> None:
    """Duplicate subagent request is acked but not processed again."""
    mixin = _TestMixin()
    mixin._subagent.search = AsyncMock(return_value=([], [], 0))
    mixin._subagent.last_cost = _FakeCost()
    msg1 = _make_msg(_subagent_payload("sub-dup"))
    msg2 = _make_msg(_subagent_payload("sub-dup"))

    await mixin._handle_subagent_search(msg1)
    await mixin._handle_subagent_search(msg2)

    assert mixin._js is not None
    assert mixin._js.publish.call_count == 1
    msg2.ack.assert_called_once()


async def test_retrieval_no_js_error_publish() -> None:
    """When _js is None, search error does not crash on publish."""
    mixin = _TestMixin()
    mixin._js = None
    mixin._retriever.search = AsyncMock(side_effect=RuntimeError("fail"))
    msg = _make_msg(_search_payload())

    # Should not raise even with _js=None
    await mixin._handle_retrieval_search(msg)

    msg.nak.assert_called_once()
