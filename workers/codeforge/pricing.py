"""Fallback pricing table for models where LiteLLM doesn't return cost."""

from __future__ import annotations

from pathlib import Path

import yaml


class PricingTable:
    """Loads per-token pricing from YAML and calculates cost from token counts."""

    def __init__(self, pricing_path: Path | None = None) -> None:
        if pricing_path is None:
            pricing_path = Path(__file__).resolve().parents[2] / "configs" / "model_pricing.yaml"
        self._models: dict[str, dict[str, float]] = {}
        if pricing_path.exists():
            with open(pricing_path) as f:
                data = yaml.safe_load(f) or {}
            self._models = data.get("models", {})

    def calculate(self, model: str, tokens_in: int, tokens_out: int) -> float:
        """Calculate cost in USD from token counts using the pricing table."""
        pricing = self._models.get(model)
        if pricing is None:
            return 0.0
        input_cost = (tokens_in / 1_000_000) * pricing.get("input_per_1m", 0.0)
        output_cost = (tokens_out / 1_000_000) * pricing.get("output_per_1m", 0.0)
        return input_cost + output_cost


# Singleton instance for the default pricing table.
_default_table: PricingTable | None = None


def _get_default_table() -> PricingTable:
    global _default_table
    if _default_table is None:
        _default_table = PricingTable()
    return _default_table


def resolve_cost(
    litellm_cost: float,
    model: str,
    tokens_in: int,
    tokens_out: int,
) -> float:
    """Return LiteLLM cost if positive, else fall back to the pricing table."""
    if litellm_cost > 0:
        return litellm_cost
    return _get_default_table().calculate(model, tokens_in, tokens_out)
