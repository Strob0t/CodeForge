"""Tests for the model that LLM skill checks use."""

from codeforge.skills.selector import resolve_skill_selection_model


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
