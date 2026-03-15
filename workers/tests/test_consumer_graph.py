"""Tests for the graph build and search handler mixins."""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._graph import GraphHandlerMixin
from codeforge.consumer._subjects import SUBJECT_GRAPH_BUILD_RESULT, SUBJECT_GRAPH_SEARCH_RESULT
from codeforge.models import GraphBuildRequest, GraphBuildResult, GraphSearchRequest

# ---------------------------------------------------------------------------
# Test harness
# ---------------------------------------------------------------------------


class _TestMixin(GraphHandlerMixin, ConsumerBaseMixin):
    def __init__(self) -> None:
        self._js: AsyncMock | None = AsyncMock()
        self._processed_ids: set[str] = set()
        self._processed_ids_max = 10_000
        self._db_url = "postgresql://test:5432/test"
        self._graph_builder = MagicMock()
        self._graph_searcher = MagicMock()


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


def _build_payload(project_id: str = "proj-1", scope_id: str = "scope-1") -> dict:
    return GraphBuildRequest(
        project_id=project_id,
        workspace_path="/tmp/workspace",
        scope_id=scope_id,
    ).model_dump()


def _search_payload(request_id: str = "req-1") -> dict:
    return GraphSearchRequest(
        project_id="proj-1",
        request_id=request_id,
        seed_symbols=["main", "handler"],
        max_hops=2,
        top_k=10,
    ).model_dump()


# ---------------------------------------------------------------------------
# Graph Build Tests
# ---------------------------------------------------------------------------


async def test_graph_build_success() -> None:
    """Successful build publishes result and acks."""
    mixin = _TestMixin()
    mixin._graph_builder.build_graph = AsyncMock(
        return_value=GraphBuildResult(
            project_id="proj-1",
            status="ready",
            node_count=100,
            edge_count=50,
            languages=["python"],
        )
    )
    msg = _make_msg(_build_payload())

    await mixin._handle_graph_build(msg)

    assert mixin._js is not None
    mixin._js.publish.assert_called_once()
    subject = mixin._js.publish.call_args.args[0]
    assert subject == SUBJECT_GRAPH_BUILD_RESULT
    result = json.loads(mixin._js.publish.call_args.args[1])
    assert result["status"] == "ready"
    assert result["node_count"] == 100
    msg.ack.assert_called_once()


async def test_graph_build_duplicate() -> None:
    """Duplicate build request is acked but not processed again."""
    mixin = _TestMixin()
    mixin._graph_builder.build_graph = AsyncMock(return_value=GraphBuildResult(project_id="proj-1", status="ready"))
    msg1 = _make_msg(_build_payload())
    msg2 = _make_msg(_build_payload())

    await mixin._handle_graph_build(msg1)
    await mixin._handle_graph_build(msg2)

    assert mixin._js is not None
    assert mixin._js.publish.call_count == 1
    msg2.ack.assert_called_once()


async def test_graph_build_failure_naks() -> None:
    """Builder failure causes nak (via _handle_request exception path)."""
    mixin = _TestMixin()
    mixin._graph_builder.build_graph = AsyncMock(side_effect=RuntimeError("build failed"))
    msg = _make_msg(_build_payload())

    await mixin._handle_graph_build(msg)

    msg.nak.assert_called_once()
    msg.ack.assert_not_called()


async def test_graph_build_invalid_json() -> None:
    """Invalid JSON causes nak."""
    mixin = _TestMixin()
    msg = MagicMock()
    msg.data = b"not json"
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.headers = {}

    await mixin._handle_graph_build(msg)

    msg.nak.assert_called_once()


# ---------------------------------------------------------------------------
# Graph Search Tests
# ---------------------------------------------------------------------------


async def test_graph_search_success() -> None:
    """Successful search publishes results and acks."""
    mixin = _TestMixin()
    mixin._graph_searcher.search = AsyncMock(return_value=[])
    msg = _make_msg(_search_payload())

    await mixin._handle_graph_search(msg)

    assert mixin._js is not None
    mixin._js.publish.assert_called_once()
    subject = mixin._js.publish.call_args.args[0]
    assert subject == SUBJECT_GRAPH_SEARCH_RESULT
    msg.ack.assert_called_once()


async def test_graph_search_duplicate() -> None:
    """Duplicate search request is acked but not processed again."""
    mixin = _TestMixin()
    mixin._graph_searcher.search = AsyncMock(return_value=[])
    msg1 = _make_msg(_search_payload("req-dup"))
    msg2 = _make_msg(_search_payload("req-dup"))

    await mixin._handle_graph_search(msg1)
    await mixin._handle_graph_search(msg2)

    assert mixin._js is not None
    assert mixin._js.publish.call_count == 1
    msg2.ack.assert_called_once()


async def test_graph_search_failure_publishes_error() -> None:
    """Search failure publishes error result (so Go waiter gets a response) and naks."""
    mixin = _TestMixin()
    mixin._graph_searcher.search = AsyncMock(side_effect=RuntimeError("search fail"))
    msg = _make_msg(_search_payload())

    await mixin._handle_graph_search(msg)

    assert mixin._js is not None
    # Error result should be published before the nak
    error_published = False
    for call in mixin._js.publish.call_args_list:
        if call.args[0] == SUBJECT_GRAPH_SEARCH_RESULT:
            result = json.loads(call.args[1])
            if result.get("error"):
                error_published = True
    assert error_published
    msg.nak.assert_called_once()


async def test_graph_search_no_js() -> None:
    """When _js is None, search error does not crash on publish."""
    mixin = _TestMixin()
    mixin._js = None
    mixin._graph_searcher.search = AsyncMock(side_effect=RuntimeError("fail"))
    msg = _make_msg(_search_payload())

    # Should not raise even with _js=None
    await mixin._handle_graph_search(msg)

    msg.nak.assert_called_once()
