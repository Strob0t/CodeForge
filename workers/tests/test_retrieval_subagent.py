"""Tests for the RetrievalSubAgent (Phase 6C)."""

from __future__ import annotations

import os
from unittest.mock import AsyncMock, MagicMock, patch

import numpy as np
import pytest

from codeforge.consumer import TaskConsumer
from codeforge.llm import CompletionResponse
from codeforge.models import (
    RetrievalSearchHit,
    RetrievalSearchRequest,
    RetrievalSearchResult,
    SubAgentSearchRequest,
    SubAgentSearchResult,
)
from codeforge.retrieval import HybridRetriever, RetrievalSubAgent

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _hit(
    filepath: str = "a.py",
    start_line: int = 1,
    end_line: int = 5,
    content: str = "code",
    language: str = "python",
    symbol_name: str = "",
    score: float = 0.5,
    bm25_rank: int = 1,
    semantic_rank: int = 1,
) -> RetrievalSearchHit:
    """Shorthand constructor for test search hits."""
    return RetrievalSearchHit(
        filepath=filepath,
        start_line=start_line,
        end_line=end_line,
        content=content,
        language=language,
        symbol_name=symbol_name,
        score=score,
        bm25_rank=bm25_rank,
        semantic_rank=semantic_rank,
    )


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


def _make_embedding_response(texts: list[str], dim: int = 8) -> dict[str, object]:
    """Create a fake LiteLLM /v1/embeddings response."""
    data = []
    for i, _ in enumerate(texts):
        rng = np.random.default_rng(seed=i)
        vec = rng.standard_normal(dim).tolist()
        data.append({"object": "embedding", "index": i, "embedding": vec})
    return {
        "object": "list",
        "data": data,
        "model": "text-embedding-3-small",
        "usage": {"prompt_tokens": len(texts) * 10, "total_tokens": len(texts) * 10},
    }


@pytest.fixture
def workspace(tmp_path: object) -> str:
    """Create a temporary workspace with sample source files."""
    ws = str(tmp_path)

    src_dir = os.path.join(ws, "src")
    os.makedirs(src_dir)
    with open(os.path.join(src_dir, "service.py"), "w") as f:
        f.write(
            "class UserService:\n"
            "    def get_user(self, user_id):\n"
            "        return None\n"
            "\n"
            "def create_handler():\n"
            "    svc = UserService()\n"
            "    return svc\n"
        )

    pkg_dir = os.path.join(ws, "pkg")
    os.makedirs(pkg_dir)
    with open(os.path.join(pkg_dir, "handler.go"), "w") as f:
        f.write(
            "package pkg\n"
            "\n"
            "func NewHandler() *Handler { return &Handler{} }\n"
            "\n"
            "type Handler struct {\n"
            "    Name string\n"
            "}\n"
        )

    return ws


# ---------------------------------------------------------------------------
# RetrievalSubAgent unit tests
# ---------------------------------------------------------------------------


def test_deduplicate() -> None:
    """Should keep only the highest-scored hit per (filepath, start_line)."""
    hits = [
        _hit(
            filepath="a.py",
            start_line=1,
            end_line=10,
            content="content",
            symbol_name="fn_a",
            score=0.5,
            bm25_rank=1,
            semantic_rank=2,
        ),
        _hit(
            filepath="a.py",
            start_line=1,
            end_line=10,
            content="content",
            symbol_name="fn_a",
            score=0.8,
            bm25_rank=3,
            semantic_rank=4,
        ),
        _hit(
            filepath="b.go",
            start_line=5,
            end_line=15,
            content="content",
            language="go",
            symbol_name="fn_b",
            score=0.6,
            bm25_rank=2,
            semantic_rank=1,
        ),
    ]
    deduped = RetrievalSubAgent._deduplicate(hits)
    assert len(deduped) == 2

    by_path = {h.filepath: h for h in deduped}
    assert by_path["a.py"].score == 0.8
    assert by_path["b.go"].score == 0.6


async def test_expand_queries_success() -> None:
    """Should parse LLM response into query lines."""
    mock_llm = AsyncMock()
    mock_llm.completion.return_value = CompletionResponse(
        content="user authentication logic\nlogin handler endpoint\nJWT token validation",
        tokens_in=10,
        tokens_out=20,
        model="test-model",
    )

    retriever = MagicMock(spec=HybridRetriever)
    agent = RetrievalSubAgent(retriever=retriever, llm=mock_llm)

    queries = await agent._expand_queries("implement login", max_queries=5, model="test-model")

    assert len(queries) == 3
    assert "user authentication logic" in queries
    assert "login handler endpoint" in queries
    assert "JWT token validation" in queries


async def test_expand_queries_fallback_on_error() -> None:
    """Should fall back to original query if LLM fails."""
    mock_llm = AsyncMock()
    mock_llm.completion.side_effect = Exception("LLM unavailable")

    retriever = MagicMock(spec=HybridRetriever)
    agent = RetrievalSubAgent(retriever=retriever, llm=mock_llm)

    queries = await agent._expand_queries("test query", max_queries=5, model="test-model")

    assert queries == ["test query"]


async def test_parallel_search() -> None:
    """Should gather results from multiple queries in parallel."""
    mock_retriever = AsyncMock(spec=HybridRetriever)
    mock_retriever._indexes = {}  # No index → skip batch embedding
    hit1 = _hit(filepath="a.py", content="code_a", score=0.9)
    hit2 = _hit(filepath="b.py", content="code_b", score=0.8)
    mock_retriever.search.side_effect = [[hit1], [hit2]]

    mock_llm = AsyncMock()
    agent = RetrievalSubAgent(retriever=mock_retriever, llm=mock_llm)

    all_hits = await agent._parallel_search("proj-1", ["query1", "query2"], top_k=10)

    assert len(all_hits) == 2
    assert mock_retriever.search.call_count == 2


async def test_rerank_success() -> None:
    """Should reorder hits based on LLM ranking."""
    mock_llm = AsyncMock()
    mock_llm.completion.return_value = CompletionResponse(
        content="2\n1\n3",
        tokens_in=50,
        tokens_out=10,
        model="test-model",
    )

    mock_retriever = MagicMock(spec=HybridRetriever)
    agent = RetrievalSubAgent(retriever=mock_retriever, llm=mock_llm)

    hits = [
        _hit(filepath="a.py", content="aaa", score=0.9, bm25_rank=1, semantic_rank=1),
        _hit(filepath="b.py", content="bbb", score=0.8, bm25_rank=2, semantic_rank=2),
        _hit(filepath="c.py", content="ccc", score=0.7, bm25_rank=3, semantic_rank=3),
    ]

    reranked = await agent._rerank("test query", hits, top_k=3, model="test-model")

    # LLM said 2,1,3 so order should be b.py, a.py, c.py
    assert reranked[0].filepath == "b.py"
    assert reranked[1].filepath == "a.py"
    assert reranked[2].filepath == "c.py"


async def test_rerank_partial_ranking() -> None:
    """Should fill remaining results when LLM returns fewer indices than top_k."""
    mock_llm = AsyncMock()
    # LLM only mentions item 2 — items 1 and 3 should still be included.
    mock_llm.completion.return_value = CompletionResponse(
        content="2",
        tokens_in=50,
        tokens_out=5,
        model="test-model",
    )

    mock_retriever = MagicMock(spec=HybridRetriever)
    agent = RetrievalSubAgent(retriever=mock_retriever, llm=mock_llm)

    hits = [
        _hit(filepath="a.py", content="aaa", score=0.9, bm25_rank=1, semantic_rank=1),
        _hit(filepath="b.py", content="bbb", score=0.8, bm25_rank=2, semantic_rank=2),
        _hit(filepath="c.py", content="ccc", score=0.7, bm25_rank=3, semantic_rank=3),
    ]

    reranked = await agent._rerank("test query", hits, top_k=3, model="test-model")

    # LLM ranked only b.py first; a.py and c.py should follow (unranked fill)
    assert len(reranked) == 3
    assert reranked[0].filepath == "b.py"
    # Remaining two should be present (order is index order: a.py, c.py)
    remaining = {reranked[1].filepath, reranked[2].filepath}
    assert remaining == {"a.py", "c.py"}


async def test_rerank_fallback_on_error() -> None:
    """Should fall back to score-based ranking if LLM fails."""
    mock_llm = AsyncMock()
    mock_llm.completion.side_effect = Exception("LLM error")

    mock_retriever = MagicMock(spec=HybridRetriever)
    agent = RetrievalSubAgent(retriever=mock_retriever, llm=mock_llm)

    hits = [
        _hit(filepath="a.py", content="aaa", score=0.5, bm25_rank=1, semantic_rank=1),
        _hit(filepath="b.py", content="bbb", score=0.9, bm25_rank=2, semantic_rank=2),
        _hit(filepath="c.py", content="ccc", score=0.7, bm25_rank=3, semantic_rank=3),
    ]

    reranked = await agent._rerank("test query", hits, top_k=3, model="test-model")

    # Should fall back to score-based: b.py (0.9), c.py (0.7), a.py (0.5)
    assert reranked[0].filepath == "b.py"
    assert reranked[1].filepath == "c.py"
    assert reranked[2].filepath == "a.py"


async def test_full_search_pipeline() -> None:
    """Full pipeline: expand -> search -> dedup -> rerank."""
    mock_llm = AsyncMock()
    # Expansion response
    mock_llm.completion.side_effect = [
        CompletionResponse(
            content="query expansion 1\nquery expansion 2",
            tokens_in=10,
            tokens_out=20,
            model="test",
        ),
        # Reranking response
        CompletionResponse(
            content="1\n2",
            tokens_in=50,
            tokens_out=10,
            model="test",
        ),
    ]

    mock_retriever = AsyncMock(spec=HybridRetriever)
    mock_retriever._indexes = {}  # No index → skip batch embedding
    hit1 = _hit(filepath="a.py", content="code_a", score=0.9)
    hit2 = _hit(filepath="b.py", content="code_b", score=0.8)
    hit3 = _hit(filepath="a.py", content="code_a", score=0.7, bm25_rank=2, semantic_rank=2)  # Duplicate of hit1
    mock_retriever.search.side_effect = [[hit1, hit2], [hit3]]

    agent = RetrievalSubAgent(retriever=mock_retriever, llm=mock_llm)

    results, expanded, total = await agent.search(
        project_id="proj-1",
        query="implement feature",
        top_k=1,
        max_queries=3,
        model="test-model",
        rerank=True,
    )

    assert len(expanded) == 2
    assert total == 3  # 3 before dedup
    assert len(results) == 1  # top_k=1


# ---------------------------------------------------------------------------
# Consumer integration test for sub-agent
# ---------------------------------------------------------------------------


async def test_handle_subagent_search_message(workspace: str) -> None:
    """Consumer should handle sub-agent search request end-to-end."""
    consumer = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")

    async def _mock_post(url: str, json: dict[str, object] | None = None, **kwargs: object) -> MagicMock:
        texts = json.get("input", []) if json else []
        resp = MagicMock()
        resp.status_code = 200
        resp.raise_for_status = MagicMock()
        resp.json = MagicMock(return_value=_make_embedding_response(texts, dim=8))
        return resp

    # Build an index first
    with patch.object(consumer._retriever._client, "post", side_effect=_mock_post):
        await consumer._retriever.build_index("proj-1", workspace)

    # Mock LLM for expansion and reranking
    mock_expansion = CompletionResponse(
        content="user service handler\nhandler function",
        tokens_in=10,
        tokens_out=20,
        model="test",
    )
    consumer._subagent._llm = AsyncMock()
    consumer._subagent._llm.completion.return_value = mock_expansion

    request = SubAgentSearchRequest(
        project_id="proj-1",
        query="handler",
        request_id="req-sub-1",
        top_k=5,
        max_queries=3,
        model="test-model",
        rerank=False,
    )

    msg = MagicMock()
    msg.data = request.model_dump_json().encode()
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()

    consumer._js = AsyncMock()

    with patch.object(consumer._retriever._client, "post", side_effect=_mock_post):
        await consumer._handle_subagent_search(msg)

    consumer._js.publish.assert_called_once()
    call_args = consumer._js.publish.call_args
    assert call_args.args[0] == "retrieval.subagent.result"

    result = SubAgentSearchResult.model_validate_json(call_args.args[1])
    assert result.project_id == "proj-1"
    assert result.query == "handler"
    assert result.request_id == "req-sub-1"
    assert len(result.expanded_queries) > 0
    assert result.total_candidates > 0
    assert len(result.results) > 0

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()

    await consumer._retriever.close()


# ---------------------------------------------------------------------------
# Additional tests from code review (Phase 6C review)
# ---------------------------------------------------------------------------


async def test_expand_queries_empty_response() -> None:
    """Should fall back to original query when LLM returns empty content."""
    mock_llm = AsyncMock()
    mock_llm.completion.return_value = CompletionResponse(
        content="",
        tokens_in=10,
        tokens_out=0,
        model="test-model",
    )

    retriever = MagicMock(spec=HybridRetriever)
    agent = RetrievalSubAgent(retriever=retriever, llm=mock_llm)

    queries = await agent._expand_queries("test query", max_queries=5, model="test-model")

    # Empty LLM response → _expand_queries returns empty list →
    # search() method should use [query] as fallback.
    # The _expand_queries itself returns the parsed lines; empty content → empty list.
    assert queries == []


async def test_expand_queries_whitespace_only_response() -> None:
    """Should return empty list when LLM returns only whitespace."""
    mock_llm = AsyncMock()
    mock_llm.completion.return_value = CompletionResponse(
        content="   \n\n   \n",
        tokens_in=10,
        tokens_out=5,
        model="test-model",
    )

    retriever = MagicMock(spec=HybridRetriever)
    agent = RetrievalSubAgent(retriever=retriever, llm=mock_llm)

    queries = await agent._expand_queries("test query", max_queries=5, model="test-model")

    # All lines are blank after strip → empty list.
    assert queries == []


async def test_handle_retrieval_search_error_publishes_result(workspace: str) -> None:
    """Consumer should publish error result when search handler raises."""
    consumer = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    consumer._js = AsyncMock()

    # Make the retriever search raise an exception.
    consumer._retriever.search = AsyncMock(side_effect=RuntimeError("index missing"))

    request = RetrievalSearchRequest(
        project_id="proj-1",
        query="test query",
        request_id="req-err-1",
        top_k=5,
    )

    msg = MagicMock()
    msg.data = request.model_dump_json().encode()
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()

    await consumer._handle_retrieval_search(msg)

    # Should have published an error result.
    consumer._js.publish.assert_called_once()
    call_args = consumer._js.publish.call_args
    assert call_args.args[0] == "retrieval.search.result"

    result = RetrievalSearchResult.model_validate_json(call_args.args[1])
    assert result.project_id == "proj-1"
    assert result.request_id == "req-err-1"
    assert result.error == "internal worker error"
    assert len(result.results) == 0

    # Message should be nak'd since the main handler failed.
    msg.nak.assert_called_once()


# ---------------------------------------------------------------------------
# Tests added from code review round 2
# ---------------------------------------------------------------------------


async def test_handle_subagent_search_error_publishes_result() -> None:
    """Consumer should publish error result when sub-agent search handler raises (#10)."""
    consumer = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    consumer._js = AsyncMock()

    # Make the sub-agent search raise an exception.
    consumer._subagent.search = AsyncMock(side_effect=RuntimeError("LLM down"))

    request = SubAgentSearchRequest(
        project_id="proj-1",
        query="test query",
        request_id="req-sub-err-1",
        top_k=5,
        max_queries=3,
        model="test-model",
        rerank=False,
    )

    msg = MagicMock()
    msg.data = request.model_dump_json().encode()
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()

    await consumer._handle_subagent_search(msg)

    # Should have published an error result.
    consumer._js.publish.assert_called_once()
    call_args = consumer._js.publish.call_args
    assert call_args.args[0] == "retrieval.subagent.result"

    result = SubAgentSearchResult.model_validate_json(call_args.args[1])
    assert result.project_id == "proj-1"
    assert result.request_id == "req-sub-err-1"
    assert result.error == "internal worker error"
    assert len(result.results) == 0

    # Message should be nak'd since the main handler failed.
    msg.nak.assert_called_once()


async def test_parallel_search_all_fail() -> None:
    """All parallel searches failing should return empty list, not raise (#12)."""
    mock_retriever = AsyncMock(spec=HybridRetriever)
    mock_retriever._indexes = {}
    # Every search call raises.
    mock_retriever.search.side_effect = RuntimeError("index corrupt")

    mock_llm = AsyncMock()
    agent = RetrievalSubAgent(retriever=mock_retriever, llm=mock_llm)

    all_hits = await agent._parallel_search("proj-1", ["q1", "q2", "q3"], top_k=10)

    assert all_hits == []
    assert mock_retriever.search.call_count == 3


async def test_pydantic_validators_clamp_values() -> None:
    """Pydantic validators should clamp top_k and max_queries to bounds (#1)."""
    req = SubAgentSearchRequest(
        project_id="p",
        query="q",
        request_id="r",
        top_k=9999,
        max_queries=100,
    )
    assert req.top_k == 500
    assert req.max_queries == 20

    req_low = SubAgentSearchRequest(
        project_id="p",
        query="q",
        request_id="r",
        top_k=-5,
        max_queries=0,
    )
    assert req_low.top_k == 1
    assert req_low.max_queries == 1

    search_req = RetrievalSearchRequest(
        project_id="p",
        query="q",
        request_id="r",
        top_k=1000,
    )
    assert search_req.top_k == 500
