"""Tests for the fallback pricing table."""

from __future__ import annotations

from pathlib import Path
from textwrap import dedent

import pytest

from codeforge.pricing import PricingTable, resolve_cost


@pytest.fixture
def pricing_yaml(tmp_path: Path) -> Path:
    """Create a temporary pricing YAML file."""
    p = tmp_path / "model_pricing.yaml"
    p.write_text(
        dedent("""\
        models:
          "openai/gpt-4o":
            input_per_1m: 2.50
            output_per_1m: 10.00
          "ollama/llama3.2":
            input_per_1m: 0.0
            output_per_1m: 0.0
        """)
    )
    return p


def test_calculate_known_model(pricing_yaml: Path) -> None:
    """PricingTable should compute cost for a known model."""
    table = PricingTable(pricing_yaml)
    # 1000 input tokens = 1000/1M * 2.50 = 0.0025
    # 500 output tokens = 500/1M * 10.00 = 0.005
    cost = table.calculate("openai/gpt-4o", tokens_in=1000, tokens_out=500)
    assert cost == pytest.approx(0.0075)


def test_calculate_unknown_model(pricing_yaml: Path) -> None:
    """PricingTable should return 0 for an unknown model."""
    table = PricingTable(pricing_yaml)
    cost = table.calculate("unknown/model", tokens_in=1000, tokens_out=500)
    assert cost == 0.0


def test_calculate_free_model(pricing_yaml: Path) -> None:
    """PricingTable should return 0 for a model with zero pricing."""
    table = PricingTable(pricing_yaml)
    cost = table.calculate("ollama/llama3.2", tokens_in=10000, tokens_out=5000)
    assert cost == 0.0


def test_calculate_zero_tokens(pricing_yaml: Path) -> None:
    """PricingTable should return 0 when token counts are zero."""
    table = PricingTable(pricing_yaml)
    cost = table.calculate("openai/gpt-4o", tokens_in=0, tokens_out=0)
    assert cost == 0.0


def test_missing_file() -> None:
    """PricingTable should handle a missing file gracefully."""
    table = PricingTable(Path("/nonexistent/path.yaml"))
    cost = table.calculate("openai/gpt-4o", tokens_in=1000, tokens_out=500)
    assert cost == 0.0


def test_resolve_cost_prefers_litellm() -> None:
    """resolve_cost should return LiteLLM cost when positive."""
    cost = resolve_cost(litellm_cost=0.05, model="openai/gpt-4o", tokens_in=1000, tokens_out=500)
    assert cost == pytest.approx(0.05)


def test_resolve_cost_falls_back_to_table() -> None:
    """resolve_cost should fall back to pricing table when LiteLLM cost is zero."""
    cost = resolve_cost(litellm_cost=0.0, model="openai/gpt-4o", tokens_in=1000, tokens_out=500)
    # Uses the default singleton table loaded from configs/model_pricing.yaml
    assert cost == pytest.approx(0.0075)


def test_resolve_cost_negative_litellm() -> None:
    """resolve_cost should fall back when LiteLLM cost is negative."""
    cost = resolve_cost(litellm_cost=-1.0, model="openai/gpt-4o", tokens_in=1000, tokens_out=500)
    assert cost == pytest.approx(0.0075)
