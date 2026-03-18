"""Tests for LLM-based context re-ranking."""

from __future__ import annotations

import pytest

from codeforge.context_reranker import ContextReranker, RerankEntry
from codeforge.llm import CompletionResponse
from tests.fake_llm import FakeLLM


@pytest.fixture
def fake_llm() -> FakeLLM:
    return FakeLLM(responses=[])


def _entry(path: str, priority: int, content: str = "x") -> RerankEntry:
    return RerankEntry(path=path, kind="file", content=content, priority=priority, tokens=10)


class TestContextRerankerReorder:
    """B1.1: LLM reranks entries and updates priorities."""

    @pytest.mark.asyncio
    async def test_reorder_by_llm_ranking(self) -> None:
        """LLM returns '3\n1\n2' -> entry C gets highest priority."""
        llm = FakeLLM(
            responses=[
                CompletionResponse(content="3\n1\n2", tokens_in=100, tokens_out=10, model="test"),
            ]
        )
        reranker = ContextReranker(llm=llm)
        entries = [_entry("a.go", 80), _entry("b.go", 70), _entry("c.go", 60)]

        result = await reranker.rerank(entries=entries, query="implement auth")

        assert result.entries[0].path == "c.go"
        assert result.entries[1].path == "a.go"
        assert result.entries[2].path == "b.go"
        # Priorities should be reassigned descending from 85
        assert result.entries[0].priority > result.entries[1].priority
        assert result.entries[1].priority > result.entries[2].priority

    @pytest.mark.asyncio
    async def test_single_entry_returned_as_is(self) -> None:
        """A single entry should not trigger an LLM call."""
        llm = FakeLLM(responses=[])
        reranker = ContextReranker(llm=llm)
        entries = [_entry("only.go", 80)]

        result = await reranker.rerank(entries=entries, query="anything")

        assert len(result.entries) == 1
        assert result.entries[0].path == "only.go"
        assert len(llm.calls) == 0

    @pytest.mark.asyncio
    async def test_empty_entries(self) -> None:
        llm = FakeLLM(responses=[])
        reranker = ContextReranker(llm=llm)

        result = await reranker.rerank(entries=[], query="anything")

        assert result.entries == []
        assert len(llm.calls) == 0


class TestContextRerankerPrompt:
    """B1.2: Prompt format includes query and numbered entry list."""

    @pytest.mark.asyncio
    async def test_prompt_contains_query_and_entries(self) -> None:
        llm = FakeLLM(
            responses=[
                CompletionResponse(content="1\n2", tokens_in=50, tokens_out=5, model="test"),
            ]
        )
        reranker = ContextReranker(llm=llm)
        entries = [_entry("a.go", 80, content="func Auth()"), _entry("b.go", 70, content="func DB()")]

        await reranker.rerank(entries=entries, query="implement auth")

        assert len(llm.calls) == 1
        prompt = llm.calls[0].prompt
        assert "implement auth" in prompt
        assert "a.go" in prompt
        assert "b.go" in prompt
        assert "1." in prompt  # numbered list


class TestContextRerankerFallback:
    """B1.3: Fallback to original order when LLM fails or returns garbage."""

    @pytest.mark.asyncio
    async def test_llm_error_returns_original_order(self) -> None:
        llm = FakeLLM(
            responses=[
                CompletionResponse(content="not a number list!?!", tokens_in=10, tokens_out=5, model="test"),
            ]
        )
        reranker = ContextReranker(llm=llm)
        entries = [_entry("a.go", 80), _entry("b.go", 70)]

        result = await reranker.rerank(entries=entries, query="test")

        # Should fall back to original order (by priority desc)
        assert result.entries[0].path == "a.go"
        assert result.entries[1].path == "b.go"
        assert result.fallback_used is True

    @pytest.mark.asyncio
    async def test_llm_exception_returns_original(self) -> None:
        """LLM raises -> graceful fallback."""
        llm = FakeLLM(responses=[])  # will raise RuntimeError
        reranker = ContextReranker(llm=llm)
        entries = [_entry("a.go", 80), _entry("b.go", 70)]

        result = await reranker.rerank(entries=entries, query="test")

        assert result.entries[0].path == "a.go"
        assert result.fallback_used is True


class TestContextRerankerRoutingTags:
    """B1.4: LLM call uses 'background' routing tag."""

    @pytest.mark.asyncio
    async def test_uses_background_routing_tag(self) -> None:
        llm = FakeLLM(
            responses=[
                CompletionResponse(content="1\n2", tokens_in=50, tokens_out=5, model="test"),
            ]
        )
        reranker = ContextReranker(llm=llm)
        entries = [_entry("a.go", 80), _entry("b.go", 70)]

        await reranker.rerank(entries=entries, query="test")

        assert llm.calls[0].tags == ["background"]


class TestContextRerankerCost:
    """Cost tracking for rerank calls."""

    @pytest.mark.asyncio
    async def test_cost_tracked(self) -> None:
        llm = FakeLLM(
            responses=[
                CompletionResponse(content="1\n2", tokens_in=100, tokens_out=20, model="test", cost_usd=0.001),
            ]
        )
        reranker = ContextReranker(llm=llm)
        entries = [_entry("a.go", 80), _entry("b.go", 70)]

        result = await reranker.rerank(entries=entries, query="test")

        assert result.tokens_in == 100
        assert result.tokens_out == 20
        assert result.cost_usd == pytest.approx(0.001)
