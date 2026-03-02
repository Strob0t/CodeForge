"""Tests for model capabilities enrichment (Phase 26J)."""

from __future__ import annotations

from unittest.mock import patch

from codeforge.routing.capabilities import (
    enrich_model_capabilities,
    filter_models_by_capability,
)

# Mock model cost data simulating litellm.model_cost.
_MOCK_MODEL_COST = {
    "gpt-4o": {
        "supports_function_calling": True,
        "supports_vision": True,
        "max_tokens": 128000,
        "input_cost_per_token": 0.0025,
        "output_cost_per_token": 0.01,
    },
    "gpt-4o-mini": {
        "supports_function_calling": True,
        "supports_vision": False,
        "max_tokens": 128000,
        "input_cost_per_token": 0.00015,
        "output_cost_per_token": 0.0006,
    },
    "claude-3-haiku-20240307": {
        "supports_function_calling": True,
        "supports_vision": True,
        "max_tokens": 200000,
        "input_cost_per_token": 0.00025,
        "output_cost_per_token": 0.00125,
    },
    "llama-3.1-8b-instant": {
        "supports_function_calling": False,
        "supports_vision": False,
        "max_tokens": 8192,
        "input_cost_per_token": 0.00005,
        "output_cost_per_token": 0.00008,
    },
}


def _patch_model_cost():
    """Patch litellm.model_cost with our mock data."""
    return patch.dict(
        "codeforge.routing.capabilities.logging.__class__.__module__",
        {},
    )


# -- enrich_model_capabilities -----------------------------------------------


@patch("codeforge.routing.capabilities.logger")
def test_known_model(mock_logger: object) -> None:
    with patch.dict("sys.modules", {"litellm": type("m", (), {"model_cost": _MOCK_MODEL_COST})()}):
        caps = enrich_model_capabilities("gpt-4o")
    assert caps["supports_function_calling"] is True
    assert caps["supports_vision"] is True
    assert caps["max_tokens"] == 128000
    assert caps["input_cost_per_token"] == 0.0025


@patch("codeforge.routing.capabilities.logger")
def test_known_model_with_provider_prefix(mock_logger: object) -> None:
    with patch.dict("sys.modules", {"litellm": type("m", (), {"model_cost": _MOCK_MODEL_COST})()}):
        caps = enrich_model_capabilities("openai/gpt-4o")
    assert caps["supports_function_calling"] is True
    assert caps["max_tokens"] == 128000


@patch("codeforge.routing.capabilities.logger")
def test_unknown_model_returns_defaults(mock_logger: object) -> None:
    with patch.dict("sys.modules", {"litellm": type("m", (), {"model_cost": _MOCK_MODEL_COST})()}):
        caps = enrich_model_capabilities("unknown-model-xyz")
    assert caps["supports_function_calling"] is False
    assert caps["supports_vision"] is False
    assert caps["max_tokens"] == 0
    assert caps["input_cost_per_token"] == 0.0


def test_litellm_not_installed_returns_defaults() -> None:
    with patch.dict("sys.modules", {"litellm": None}):
        caps = enrich_model_capabilities("gpt-4o")
    assert caps["supports_function_calling"] is False
    assert caps["max_tokens"] == 0


# -- filter_models_by_capability ---------------------------------------------


def test_empty_model_list() -> None:
    result = filter_models_by_capability([])
    assert result == []


def test_no_filters_returns_all() -> None:
    models = ["a", "b", "c"]
    result = filter_models_by_capability(models)
    assert result == ["a", "b", "c"]


@patch("codeforge.routing.capabilities.enrich_model_capabilities")
def test_filter_by_tools(mock_enrich: object) -> None:
    mock_enrich.side_effect = lambda m: {  # type: ignore[union-attr]
        "tool_model": {
            "supports_function_calling": True,
            "supports_vision": False,
            "max_tokens": 128000,
            "input_cost_per_token": 0.0,
            "output_cost_per_token": 0.0,
        },
        "no_tool": {
            "supports_function_calling": False,
            "supports_vision": False,
            "max_tokens": 128000,
            "input_cost_per_token": 0.0,
            "output_cost_per_token": 0.0,
        },
    }[m]

    result = filter_models_by_capability(["tool_model", "no_tool"], needs_tools=True)
    assert result == ["tool_model"]


@patch("codeforge.routing.capabilities.enrich_model_capabilities")
def test_filter_by_vision(mock_enrich: object) -> None:
    mock_enrich.side_effect = lambda m: {  # type: ignore[union-attr]
        "vision": {
            "supports_function_calling": False,
            "supports_vision": True,
            "max_tokens": 128000,
            "input_cost_per_token": 0.0,
            "output_cost_per_token": 0.0,
        },
        "no_vision": {
            "supports_function_calling": False,
            "supports_vision": False,
            "max_tokens": 128000,
            "input_cost_per_token": 0.0,
            "output_cost_per_token": 0.0,
        },
    }[m]

    result = filter_models_by_capability(["vision", "no_vision"], needs_vision=True)
    assert result == ["vision"]


@patch("codeforge.routing.capabilities.enrich_model_capabilities")
def test_filter_by_min_context(mock_enrich: object) -> None:
    mock_enrich.side_effect = lambda m: {  # type: ignore[union-attr]
        "big": {
            "supports_function_calling": False,
            "supports_vision": False,
            "max_tokens": 128000,
            "input_cost_per_token": 0.0,
            "output_cost_per_token": 0.0,
        },
        "small": {
            "supports_function_calling": False,
            "supports_vision": False,
            "max_tokens": 4096,
            "input_cost_per_token": 0.0,
            "output_cost_per_token": 0.0,
        },
    }[m]

    result = filter_models_by_capability(["big", "small"], min_context=32000)
    assert result == ["big"]


@patch("codeforge.routing.capabilities.enrich_model_capabilities")
def test_combined_filters(mock_enrich: object) -> None:
    caps = {
        "full": {
            "supports_function_calling": True,
            "supports_vision": True,
            "max_tokens": 128000,
            "input_cost_per_token": 0.0,
            "output_cost_per_token": 0.0,
        },
        "tools_only": {
            "supports_function_calling": True,
            "supports_vision": False,
            "max_tokens": 128000,
            "input_cost_per_token": 0.0,
            "output_cost_per_token": 0.0,
        },
        "small": {
            "supports_function_calling": True,
            "supports_vision": True,
            "max_tokens": 4096,
            "input_cost_per_token": 0.0,
            "output_cost_per_token": 0.0,
        },
    }
    mock_enrich.side_effect = lambda m: caps[m]  # type: ignore[union-attr]

    result = filter_models_by_capability(
        ["full", "tools_only", "small"],
        needs_tools=True,
        needs_vision=True,
        min_context=32000,
    )
    assert result == ["full"]
