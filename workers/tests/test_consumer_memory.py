"""Tests for the memory store and recall handler mixins."""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._memory import MemoryHandlerMixin
from codeforge.consumer._subjects import SUBJECT_MEMORY_RECALL_RESULT
from codeforge.memory.models import MemoryKind

# ---------------------------------------------------------------------------
# Test harness
# ---------------------------------------------------------------------------


class _TestMixin(MemoryHandlerMixin, ConsumerBaseMixin):
    def __init__(self) -> None:
        self._js: AsyncMock | None = AsyncMock()
        self._processed_ids: set[str] = set()
        self._processed_ids_max = 10_000
        self._llm = MagicMock()
        self._db_url = "postgresql://test:5432/test"


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


def _store_payload(project_id: str = "proj-1", run_id: str = "run-1") -> dict:
    return {
        "tenant_id": "00000000-0000-0000-0000-000000000000",
        "project_id": project_id,
        "agent_id": "agent-1",
        "run_id": run_id,
        "content": "Test memory content",
        "kind": MemoryKind.OBSERVATION.value,
        "importance": 0.7,
        "metadata": {},
    }


def _recall_payload(
    project_id: str = "proj-1",
    query: str = "What happened?",
    request_id: str = "req-1",
) -> dict:
    return {
        "request_id": request_id,
        "tenant_id": "00000000-0000-0000-0000-000000000000",
        "project_id": project_id,
        "query": query,
        "top_k": 5,
    }


# ---------------------------------------------------------------------------
# Mock helpers
# ---------------------------------------------------------------------------


def _mock_db_success():
    """Patch psycopg.AsyncConnection to simulate successful DB operations."""
    mock_cur = MagicMock()
    mock_cur.execute = AsyncMock()
    mock_cur.fetchall = AsyncMock(return_value=[])
    mock_cur.__aenter__ = AsyncMock(return_value=mock_cur)
    mock_cur.__aexit__ = AsyncMock(return_value=False)

    mock_conn = MagicMock()
    mock_conn.cursor = MagicMock(return_value=mock_cur)
    mock_conn.commit = AsyncMock()
    mock_conn.__aenter__ = AsyncMock(return_value=mock_conn)
    mock_conn.__aexit__ = AsyncMock(return_value=False)

    return patch("psycopg.AsyncConnection.connect", AsyncMock(return_value=mock_conn))


# ---------------------------------------------------------------------------
# Memory Store Tests
# ---------------------------------------------------------------------------


async def test_memory_store_success() -> None:
    """Store request with successful embedding and DB write acks."""
    mixin = _TestMixin()
    mixin._llm.embedding = AsyncMock(return_value=[0.1, 0.2, 0.3])
    msg = _make_msg(_store_payload())

    with _mock_db_success():
        await mixin._handle_memory_store(msg)

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


async def test_memory_store_embedding_failure_still_stores() -> None:
    """If embedding fails, memory is still stored (with embedding=None)."""
    mixin = _TestMixin()
    mixin._llm.embedding = AsyncMock(side_effect=RuntimeError("embed fail"))
    msg = _make_msg(_store_payload())

    with _mock_db_success():
        await mixin._handle_memory_store(msg)

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


async def test_memory_store_db_failure_acks() -> None:
    """DB failure in store is caught; message is acked (not redelivered)."""
    mixin = _TestMixin()
    mixin._llm.embedding = AsyncMock(return_value=[0.1])
    msg = _make_msg(_store_payload())

    with patch(
        "psycopg.AsyncConnection.connect",
        AsyncMock(side_effect=RuntimeError("db down")),
    ):
        await mixin._handle_memory_store(msg)

    # The _handle_request catches the exception and naks, but _do_memory_store
    # catches DB errors internally so _handle_request should ack.
    msg.ack.assert_called_once()


async def test_memory_store_duplicate_skipped() -> None:
    """Duplicate store request is acked but not processed again."""
    mixin = _TestMixin()
    mixin._llm.embedding = AsyncMock(return_value=[0.1])
    msg1 = _make_msg(_store_payload())
    msg2 = _make_msg(_store_payload())

    with _mock_db_success():
        await mixin._handle_memory_store(msg1)
        await mixin._handle_memory_store(msg2)

    # Both acked
    msg1.ack.assert_called_once()
    msg2.ack.assert_called_once()


# ---------------------------------------------------------------------------
# Memory Recall Tests
# ---------------------------------------------------------------------------


async def test_memory_recall_success() -> None:
    """Recall with valid embedding publishes results."""
    from datetime import UTC, datetime

    import numpy as np

    mixin = _TestMixin()
    mixin._llm.embedding = AsyncMock(return_value=[0.1, 0.2, 0.3])

    embedding_bytes = np.array([0.1, 0.2, 0.3], dtype=np.float32).tobytes()
    mock_rows = [
        ("id-1", "memory content", "observation", 0.7, embedding_bytes, datetime(2025, 1, 1, tzinfo=UTC)),
    ]

    mock_cur = MagicMock()
    mock_cur.execute = AsyncMock()
    mock_cur.fetchall = AsyncMock(return_value=mock_rows)
    mock_cur.__aenter__ = AsyncMock(return_value=mock_cur)
    mock_cur.__aexit__ = AsyncMock(return_value=False)

    mock_conn = MagicMock()
    mock_conn.cursor = MagicMock(return_value=mock_cur)
    mock_conn.__aenter__ = AsyncMock(return_value=mock_conn)
    mock_conn.__aexit__ = AsyncMock(return_value=False)

    msg = _make_msg(_recall_payload())

    with patch("psycopg.AsyncConnection.connect", AsyncMock(return_value=mock_conn)):
        await mixin._handle_memory_recall(msg)

    assert mixin._js is not None
    # Should publish recall result
    published = False
    for call in mixin._js.publish.call_args_list:
        if call.args[0] == SUBJECT_MEMORY_RECALL_RESULT:
            published = True
            result = json.loads(call.args[1])
            assert "results" in result
    assert published
    msg.ack.assert_called_once()


async def test_memory_recall_embedding_failure() -> None:
    """When query embedding fails, error result is published."""
    mixin = _TestMixin()
    mixin._llm.embedding = AsyncMock(side_effect=RuntimeError("embed fail"))
    msg = _make_msg(_recall_payload())

    await mixin._handle_memory_recall(msg)

    assert mixin._js is not None
    # Should publish error result
    published = False
    for call in mixin._js.publish.call_args_list:
        if call.args[0] == SUBJECT_MEMORY_RECALL_RESULT:
            published = True
            result = json.loads(call.args[1])
            assert "error" in result
    assert published
    msg.ack.assert_called_once()


async def test_memory_recall_empty_db() -> None:
    """Recall with no rows returns empty results."""
    mixin = _TestMixin()
    mixin._llm.embedding = AsyncMock(return_value=[0.1, 0.2, 0.3])

    mock_cur = MagicMock()
    mock_cur.execute = AsyncMock()
    mock_cur.fetchall = AsyncMock(return_value=[])
    mock_cur.__aenter__ = AsyncMock(return_value=mock_cur)
    mock_cur.__aexit__ = AsyncMock(return_value=False)

    mock_conn = MagicMock()
    mock_conn.cursor = MagicMock(return_value=mock_cur)
    mock_conn.__aenter__ = AsyncMock(return_value=mock_conn)
    mock_conn.__aexit__ = AsyncMock(return_value=False)

    msg = _make_msg(_recall_payload())

    with patch("psycopg.AsyncConnection.connect", AsyncMock(return_value=mock_conn)):
        await mixin._handle_memory_recall(msg)

    assert mixin._js is not None
    published = False
    for call in mixin._js.publish.call_args_list:
        if call.args[0] == SUBJECT_MEMORY_RECALL_RESULT:
            published = True
            result = json.loads(call.args[1])
            assert result["results"] == []
    assert published
    msg.ack.assert_called_once()


async def test_memory_recall_kind_filter() -> None:
    """Recall with a kind filter passes it to the SQL query."""
    mixin = _TestMixin()
    mixin._llm.embedding = AsyncMock(return_value=[0.1, 0.2, 0.3])

    mock_cur = MagicMock()
    mock_cur.execute = AsyncMock()
    mock_cur.fetchall = AsyncMock(return_value=[])
    mock_cur.__aenter__ = AsyncMock(return_value=mock_cur)
    mock_cur.__aexit__ = AsyncMock(return_value=False)

    mock_conn = MagicMock()
    mock_conn.cursor = MagicMock(return_value=mock_cur)
    mock_conn.__aenter__ = AsyncMock(return_value=mock_conn)
    mock_conn.__aexit__ = AsyncMock(return_value=False)

    payload = _recall_payload()
    payload["kind"] = MemoryKind.DECISION.value
    msg = _make_msg(payload)

    with patch("psycopg.AsyncConnection.connect", AsyncMock(return_value=mock_conn)):
        await mixin._handle_memory_recall(msg)

    # The SQL query should include kind filter
    execute_call = mock_cur.execute.call_args
    sql = execute_call.args[0]
    assert "AND kind = %s" in sql
    msg.ack.assert_called_once()


async def test_memory_recall_db_failure() -> None:
    """DB failure in recall is caught; message is acked."""
    mixin = _TestMixin()
    mixin._llm.embedding = AsyncMock(return_value=[0.1])
    msg = _make_msg(_recall_payload())

    with patch(
        "psycopg.AsyncConnection.connect",
        AsyncMock(side_effect=RuntimeError("db down")),
    ):
        await mixin._handle_memory_recall(msg)

    msg.ack.assert_called_once()


async def test_memory_recall_no_js() -> None:
    """When _js is None, recall executes but does not publish."""
    mixin = _TestMixin()
    mixin._js = None
    mixin._llm.embedding = AsyncMock(side_effect=RuntimeError("embed fail"))
    msg = _make_msg(_recall_payload())

    await mixin._handle_memory_recall(msg)

    # _handle_request naks on exception, but _do_memory_recall catches internally
    # Since _js is None, _handle_request won't publish but will ack.
    msg.ack.assert_called_once()
