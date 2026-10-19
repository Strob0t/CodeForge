"""Pre-validation filter: remove models whose provider has no API key configured.

Provides both a ``KeyFilter`` class for dependency injection and module-level
wrapper functions for backward compatibility.
"""

from __future__ import annotations

import os

import structlog

from codeforge.config import get_settings
from codeforge.provider_keys import KEYED_PROVIDERS_ENV, KEYLESS_PROVIDERS, PROVIDER_KEY_MAP

logger = structlog.get_logger(component="routing")


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
        """Check whether *provider* has a usable API key, as the Go Core decides (KI-125).

        Named in ``keyed_providers`` (the production worker has no provider
        key in its environment) or its key variable is set; whitespace-only
        keys are treated as absent (F14-D3).
        """
        if provider in KEYLESS_PROVIDERS:
            return True
        env_var = PROVIDER_KEY_MAP.get(provider)
        if env_var is None:
            # Unknown provider -- assume key is available (safe default).
            return True
        if provider in get_settings().keyed_providers:
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
                    "excluding models, provider has no API key (env var not set or empty, "
                    f"provider not named in {KEYED_PROVIDERS_ENV})",
                    provider=provider,
                    env_var=PROVIDER_KEY_MAP.get(provider, "?"),
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
