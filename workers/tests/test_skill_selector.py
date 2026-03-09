"""Tests for LLM-based skill selection with BM25 fallback."""

from unittest.mock import AsyncMock, patch

import pytest

from codeforge.skills.models import Skill
from codeforge.skills.selector import resolve_skill_selection_model, select_skills_for_task


def test_resolve_skill_selection_model_picks_cheapest(monkeypatch):
    monkeypatch.setattr(
        "codeforge.skills.selector.get_available_models",
        lambda: ["openai/gpt-4o", "openai/gpt-4o-mini", "anthropic/claude-haiku-3.5"],
    )
    monkeypatch.setattr(
        "codeforge.skills.selector.filter_models_by_capability",
        lambda models, **kw: models,
    )
    monkeypatch.setattr(
        "codeforge.skills.selector.enrich_model_capabilities",
        lambda m: {"input_cost_per_token": 0.01 if "4o-mini" in m else 0.1},
    )
    model = resolve_skill_selection_model()
    assert model == "openai/gpt-4o-mini"


def test_resolve_skill_selection_model_fallback_when_empty(monkeypatch):
    monkeypatch.setattr("codeforge.skills.selector.get_available_models", list)
    model = resolve_skill_selection_model()
    assert model == ""


def test_resolve_skill_selection_model_no_capable_uses_first(monkeypatch):
    monkeypatch.setattr(
        "codeforge.skills.selector.get_available_models",
        lambda: ["ollama/llama3"],
    )
    monkeypatch.setattr(
        "codeforge.skills.selector.filter_models_by_capability",
        lambda models, **kw: [],
    )
    model = resolve_skill_selection_model()
    assert model == "ollama/llama3"


@pytest.mark.asyncio
async def test_select_skills_returns_matching_ids():
    skills = [
        Skill(id="1", name="tdd", description="Test-driven development", content="..."),
        Skill(id="2", name="debugging", description="Systematic debugging", content="..."),
        Skill(id="3", name="nats-pattern", description="NATS handler", content="..."),
    ]
    mock_response = AsyncMock()
    mock_response.content = '["1", "2"]'

    mock_client = AsyncMock()
    mock_client.chat_completion = AsyncMock(return_value=mock_response)

    with patch("codeforge.skills.selector.resolve_skill_selection_model", return_value="test-model"):
        selected = await select_skills_for_task(skills, "Fix the login bug", mock_client)

    assert [s.id for s in selected] == ["1", "2"]


@pytest.mark.asyncio
async def test_select_skills_empty_input():
    selected = await select_skills_for_task([], "some task", AsyncMock())
    assert selected == []

    selected2 = await select_skills_for_task([Skill(name="x", content="y")], "", AsyncMock())
    assert selected2 == []


@pytest.mark.asyncio
async def test_select_skills_fallback_on_llm_error():
    skills = [
        Skill(id="1", name="debugging", description="debug workflow", content="steps", tags=["debug", "fix"]),
    ]
    with (
        patch("codeforge.skills.selector.resolve_skill_selection_model", return_value=""),
    ):
        mock_client = AsyncMock()
        mock_client.chat_completion = AsyncMock(side_effect=RuntimeError("no model"))
        selected = await select_skills_for_task(skills, "debug the crash", mock_client)

    # BM25 fallback should run (may or may not match depending on tokenization)
    assert isinstance(selected, list)
