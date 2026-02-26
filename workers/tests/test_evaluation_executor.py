"""Tests for GEMMAS evaluation executor."""

from __future__ import annotations

import pytest

from codeforge.evaluation.executor import handle_gemmas_evaluation


@pytest.mark.asyncio
async def test_basic_evaluation() -> None:
    """Test GEMMAS evaluation with 3 agents."""
    messages = [
        {"agent_id": "coder-1", "content": "Implemented feature X with quicksort algorithm", "round": 1},
        {
            "agent_id": "reviewer-1",
            "content": "Found security vulnerability in input validation",
            "round": 2,
            "parent_agent_id": "coder-1",
        },
        {
            "agent_id": "coder-1",
            "content": "Fixed the input validation bug",
            "round": 3,
            "parent_agent_id": "reviewer-1",
        },
    ]
    result = await handle_gemmas_evaluation(messages, plan_id="plan-1")
    assert result["plan_id"] == "plan-1"
    assert result["error"] == ""
    assert 0.0 <= result["information_diversity_score"] <= 1.0
    assert 0.0 <= result["unnecessary_path_ratio"] <= 1.0


@pytest.mark.asyncio
async def test_with_embed_fn() -> None:
    """Test GEMMAS evaluation with a mock embedding function."""
    messages = [
        {"agent_id": "a", "content": "implement feature", "round": 1},
        {"agent_id": "b", "content": "review code", "round": 2, "parent_agent_id": "a"},
    ]

    def mock_embed(texts: list[str]) -> list[list[float]]:
        return [[1.0, 0.0], [0.0, 1.0]]

    result = await handle_gemmas_evaluation(messages, plan_id="plan-2", embed_fn=mock_embed)
    assert result["error"] == ""
    assert result["information_diversity_score"] > 0.5


@pytest.mark.asyncio
async def test_single_agent() -> None:
    """Single agent should have IDS=1.0 (trivially diverse)."""
    messages = [
        {"agent_id": "solo", "content": "did everything myself", "round": 1},
    ]
    result = await handle_gemmas_evaluation(messages, plan_id="plan-3")
    assert result["information_diversity_score"] == 1.0
    assert result["unnecessary_path_ratio"] == 0.0


@pytest.mark.asyncio
async def test_empty_messages() -> None:
    """Empty messages should return defaults gracefully."""
    result = await handle_gemmas_evaluation([], plan_id="plan-4")
    assert result["information_diversity_score"] == 1.0
    assert result["unnecessary_path_ratio"] == 0.0
    assert result["error"] == ""
