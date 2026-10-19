"""Fallback pricing table for models where LiteLLM doesn't return cost."""

from __future__ import annotations

from pathlib import Path

import structlog
import yaml

from codeforge.provider_keys import KEYLESS_PROVIDERS, model_provider

logger = structlog.get_logger(__name__)

# The worker is installed editable (Dockerfile.worker: /app/workers), so this
# is /app/configs/model_pricing.yaml in the image, which copies it there.
DEFAULT_PRICING_PATH = Path(__file__).resolve().parents[2] / "configs" / "model_pricing.yaml"

# Models a call with tokens was costed $0 for, warned about once each (KI-196).
_warned_unpriced: set[str] = set()


class PricingTable:
    """Loads per-token pricing from YAML and calculates cost from token counts."""

    def __init__(self, pricing_path: Path | None = None) -> None:
        if pricing_path is None:
            pricing_path = DEFAULT_PRICING_PATH
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
    """Return LiteLLM cost if positive, else fall back to the pricing table.

    A cloud model that still costs $0 for a call with tokens is logged once:
    its budget (max_cost) cannot stop a run.
    """
    if litellm_cost > 0:
        return litellm_cost
    cost = _get_default_table().calculate(model, tokens_in, tokens_out)
    if cost <= 0 and (tokens_in or tokens_out) and model_provider(model) not in KEYLESS_PROVIDERS:
        _warn_unpriced(model)
    return cost


def _warn_unpriced(model: str) -> None:
    if model in _warned_unpriced:
        return
    _warned_unpriced.add(model)
    logger.warning(
        "no price for a model that is not local: its calls cost $0 and budgets cannot stop its runs",
        model=model,
        pricing_table=str(DEFAULT_PRICING_PATH),
    )
