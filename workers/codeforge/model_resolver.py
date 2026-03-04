"""Centralized model resolver with LiteLLM auto-discovery and caching."""

from __future__ import annotations

import logging
import os
import threading
import time

import httpx

logger = logging.getLogger(__name__)

_CACHE_TTL_SECONDS = 60.0


class _ModelCache:
    """Thread-safe cached list of available models from LiteLLM."""

    __slots__ = ("_best", "_last_refresh", "_lock", "_models")

    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._models: list[str] = []
        self._best: str = ""
        self._last_refresh: float = 0.0

    def _is_stale(self) -> bool:
        return (time.monotonic() - self._last_refresh) > _CACHE_TTL_SECONDS

    def get_models(self) -> list[str]:
        if self._is_stale():
            self._refresh()
        with self._lock:
            return list(self._models)

    def get_best(self) -> str:
        if self._is_stale():
            self._refresh()
        with self._lock:
            return self._best

    def _refresh(self) -> None:
        litellm_url = os.environ.get("LITELLM_BASE_URL", "http://localhost:4000")
        try:
            resp = httpx.get(f"{litellm_url}/v1/models", timeout=5.0)
            if resp.status_code != 200:
                logger.warning("model_resolver: LiteLLM /v1/models returned %d", resp.status_code)
                return
            data = resp.json()
            models = [m.get("id", "") for m in data.get("data", []) if m.get("id")]
        except Exception as exc:
            logger.warning("model_resolver: failed to fetch from LiteLLM: %s", exc, exc_info=True)
            return

        with self._lock:
            self._models = models
            self._best = models[0] if models else ""
            self._last_refresh = time.monotonic()

        if models:
            logger.info("model_resolver: discovered %d models, best=%s", len(models), models[0])
        else:
            logger.warning("model_resolver: no models available from LiteLLM")


# Module-level singleton.
_cache = _ModelCache()


def resolve_model(explicit: str = "") -> str:
    """Return the best available model.

    Priority: explicit > CODEFORGE_DEFAULT_MODEL env var > LiteLLM auto-discovery.
    Raises RuntimeError if no model can be resolved.
    """
    if explicit:
        return explicit

    env_model = os.environ.get("CODEFORGE_DEFAULT_MODEL", "")
    if env_model:
        return env_model

    best = _cache.get_best()
    if best:
        return best

    raise RuntimeError(
        "No LLM model available. Configure CODEFORGE_DEFAULT_MODEL or ensure LiteLLM has at least one reachable model."
    )


def get_available_models() -> list[str]:
    """Return cached list of available model names from LiteLLM."""
    return _cache.get_models()
