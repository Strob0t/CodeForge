"""Pre-validation filter: remove models whose provider has no API key configured.

Provides both a ``KeyFilter`` class for dependency injection and module-level
wrapper functions for backward compatibility.
"""

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


class KeyFilter:
    """Stateful pre-validation filter for provider API key availability.

    Encapsulates the warned-providers set and healthy-models set that were
    previously module-level globals, enabling dependency injection and
    improving testability.
    """

    __slots__ = ("_healthy_models", "_warned_providers")

    def __init__(self) -> None:
        self._warned_providers: set[str] = set()
        self._healthy_models: set[str] = set()

    def reset_warnings(self) -> None:
        """Clear the warned-providers set (for test teardown)."""
        self._warned_providers.clear()

    def set_healthy_models(self, models: set[str]) -> None:
        """Update the set of models known to be healthy from LiteLLM /health."""
        self._healthy_models = models

    @staticmethod
    def has_key(provider: str) -> bool:
        """Check whether *provider* has a usable API key in the environment.

        Whitespace-only keys are treated as absent (F14-D3).
        """
        if provider in _KEYLESS_PROVIDERS:
            return True
        env_var = PROVIDER_KEY_MAP.get(provider)
        if env_var is None:
            # Unknown provider -- assume key is available (safe default).
            return True
        key = os.environ.get(env_var, "").strip()
        return bool(key)  # empty after strip = no key

    def filter_keyless_models(self, models: list[str]) -> list[str]:
        """Return only models whose provider has an API key set OR are known healthy.

        Models without a ``provider/`` prefix are always kept.
        Unknown providers are always kept (safe default).
        Models reported as healthy by LiteLLM /health are always kept (local models).
        """
        kept: list[str] = []
        for model in models:
            if "/" not in model:
                kept.append(model)
                continue
            # Always keep models that LiteLLM reports as healthy (covers local
            # models like openai/container backed by LM Studio).
            if model in self._healthy_models:
                kept.append(model)
                continue
            provider = model.split("/", 1)[0]
            if self.has_key(provider):
                kept.append(model)
            elif provider not in self._warned_providers:
                self._warned_providers.add(provider)
                logger.warning(
                    "Excluding %s models: env var %s is not set or empty",
                    provider,
                    PROVIDER_KEY_MAP.get(provider, "?"),
                )
        return kept


# ---------------------------------------------------------------------------
# Module-level default instance + backward-compatible wrapper functions
# ---------------------------------------------------------------------------

_default_key_filter = KeyFilter()


def get_key_filter() -> KeyFilter:
    """Return the module-level default KeyFilter instance."""
    return _default_key_filter


def reset_warnings() -> None:
    """Clear the warned-providers set (for test teardown)."""
    _default_key_filter.reset_warnings()


def set_healthy_models(models: set[str]) -> None:
    """Update the set of models known to be healthy from LiteLLM /health."""
    _default_key_filter.set_healthy_models(models)


def filter_keyless_models(models: list[str]) -> list[str]:
    """Return only models whose provider has an API key set OR are known healthy."""
    return _default_key_filter.filter_keyless_models(models)
