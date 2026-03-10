"""Pre-validation filter: remove models whose provider has no API key configured."""

from __future__ import annotations

import logging
import os

logger = logging.getLogger(__name__)

# Provider prefix -> environment variable that holds the API key.
PROVIDER_KEY_MAP: dict[str, str] = {
    "openai": "OPENAI_API_KEY",
    "anthropic": "ANTHROPIC_API_KEY",
    "gemini": "GEMINI_API_KEY",
    "groq": "GROQ_API_KEY",
    "mistral": "MISTRAL_API_KEY",
    "deepseek": "DEEPSEEK_API_KEY",
    "cohere": "COHERE_API_KEY",
    "together_ai": "TOGETHERAI_API_KEY",
    "fireworks_ai": "FIREWORKS_API_KEY",
    "github_copilot": "GITHUB_TOKEN",
}

# Providers that never need an API key.
_KEYLESS_PROVIDERS: frozenset[str] = frozenset({"ollama", "lm_studio"})

# Track which providers we've already warned about to avoid log spam.
_warned_providers: set[str] = set()


def reset_warnings() -> None:
    """Clear the warned-providers set (for test teardown)."""
    _warned_providers.clear()


def _has_key(provider: str) -> bool:
    """Check whether *provider* has a usable API key in the environment."""
    if provider in _KEYLESS_PROVIDERS:
        return True
    env_var = PROVIDER_KEY_MAP.get(provider)
    if env_var is None:
        # Unknown provider -- assume key is available (safe default).
        return True
    value = os.environ.get(env_var, "")
    return bool(value.strip())


def filter_keyless_models(models: list[str]) -> list[str]:
    """Return only models whose provider has an API key set.

    Models without a ``provider/`` prefix are always kept.
    Unknown providers are always kept (safe default).
    """
    kept: list[str] = []
    for model in models:
        if "/" not in model:
            kept.append(model)
            continue
        provider = model.split("/", 1)[0]
        if _has_key(provider):
            kept.append(model)
        elif provider not in _warned_providers:
            _warned_providers.add(provider)
            logger.warning(
                "Excluding %s models: env var %s is not set or empty",
                provider,
                PROVIDER_KEY_MAP.get(provider, "?"),
            )
    return kept
