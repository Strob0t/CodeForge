"""Model capabilities enrichment via LiteLLM metadata.

Provides capability lookups and filtering for available models, using
the litellm.model_cost dictionary as the data source.
"""

from __future__ import annotations

import structlog

logger = structlog.get_logger(component="routing")


def enrich_model_capabilities(model_name: str) -> dict[str, object]:
    """Look up model capabilities from litellm.model_cost.

    Returns a dict with keys: supports_function_calling, supports_vision,
    max_tokens, input_cost_per_token, output_cost_per_token.
    Falls back to empty defaults if the model is not found.
    """
    defaults: dict[str, object] = {
        "supports_function_calling": False,
        "supports_vision": False,
        "max_tokens": 0,
        "input_cost_per_token": 0.0,
        "output_cost_per_token": 0.0,
    }

    try:
        from litellm import model_cost  # type: ignore[import-untyped]
    except ImportError:
        logger.debug("litellm not available for capability lookup", model=model_name)
        return defaults

    # Try exact match first, then without provider prefix.
    info = model_cost.get(model_name)
    if info is None and "/" in model_name:
        _, short_name = model_name.split("/", 1)
        info = model_cost.get(short_name)

    if info is None:
        return defaults

    return {
        "supports_function_calling": bool(info.get("supports_function_calling", False)),
        "supports_vision": bool(info.get("supports_vision", False)),
        "max_tokens": int(info.get("max_tokens", 0)),
        "input_cost_per_token": float(info.get("input_cost_per_token", 0.0)),
        "output_cost_per_token": float(info.get("output_cost_per_token", 0.0)),
    }


def filter_models_by_capability(
    models: list[str],
    needs_tools: bool = False,
    needs_vision: bool = False,
    min_context: int = 0,
) -> list[str]:
    """Filter models by required capabilities.

    Uses litellm.model_cost for lookups. Models whose capabilities cannot
    be determined are excluded when a specific capability is required.
    """
    if not models:
        return []

    if not needs_tools and not needs_vision and min_context <= 0:
        return list(models)

    result: list[str] = []
    for model in models:
        caps = enrich_model_capabilities(model)

        if needs_tools and not caps["supports_function_calling"]:
            continue
        if needs_vision and not caps["supports_vision"]:
            continue
        max_tokens = caps["max_tokens"]
        if min_context > 0 and isinstance(max_tokens, int) and max_tokens < min_context:
            continue

        result.append(model)

    return result
