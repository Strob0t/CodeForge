"""Centralized model resolver with LiteLLM auto-discovery and caching."""

from __future__ import annotations

import logging
import os
import threading
import time

import httpx

logger = logging.getLogger(__name__)

_CACHE_TTL_SECONDS = 60.0


def expand_wildcard_models(raw_ids: list[str]) -> list[str]:
    """Expand wildcard model IDs into concrete model names from COMPLEXITY_DEFAULTS.

    Wildcards like ``openai/*`` are mapped to their provider prefix, then
    concrete model names from COMPLEXITY_DEFAULTS whose provider matches are
    expanded (these come first). Non-wildcard concrete models are appended
    after the expanded ones, preserving dedup order.
    """
    concrete: list[str] = []
    wildcard_providers: set[str] = set()
    for mid in raw_ids:
        if "*" in mid:
            prefix = mid.split("/")[0]
            if prefix:
                wildcard_providers.add(prefix)
        else:
            concrete.append(mid)

    if not wildcard_providers:
        return concrete

    from codeforge.routing.router import COMPLEXITY_DEFAULTS

    result: list[str] = []
    seen: set[str] = set()
    for tier_models in COMPLEXITY_DEFAULTS.values():
        for m in tier_models:
            provider = m.split("/")[0] if "/" in m else ""
            if provider in wildcard_providers and m not in seen:
                result.append(m)
                seen.add(m)
    for m in concrete:
        if m not in seen:
            result.append(m)
            seen.add(m)
    return result


def _fetch_healthy_models(litellm_url: str, headers: dict[str, str]) -> set[str]:
    """Query LiteLLM /health and return model names of healthy endpoints."""
    try:
        resp = httpx.get(f"{litellm_url}/health", headers=headers, timeout=5.0)
        if resp.status_code != 200:
            return set()
        data = resp.json()
        healthy: set[str] = set()
        for ep in data.get("healthy_endpoints", []):
            model = ep.get("model", ep.get("model_name", ""))
            if model:
                healthy.add(model)
        return healthy
    except Exception:
        return set()


class _ModelCache:
    """Thread-safe cached list of available models from LiteLLM."""

    __slots__ = ("_best", "_healthy", "_last_refresh", "_lock", "_models")

    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._models: list[str] = []
        self._healthy: set[str] = set()
        self._best: str = ""
        self._last_refresh: float = 0.0

    def _is_stale(self) -> bool:
        return (time.monotonic() - self._last_refresh) > _CACHE_TTL_SECONDS

    def get_models(self) -> list[str]:
        if self._is_stale():
            self._refresh()
        with self._lock:
            models = list(self._models)
        from codeforge.routing.blocklist import get_blocklist

        return get_blocklist().filter_available(models)

    def get_best(self) -> str:
        if self._is_stale():
            self._refresh()
        with self._lock:
            return self._best

    def _refresh(self) -> None:
        litellm_url = os.environ.get("LITELLM_BASE_URL", "http://localhost:4000")
        api_key = os.environ.get("LITELLM_MASTER_KEY", "")
        headers: dict[str, str] = {}
        if api_key:
            headers["Authorization"] = f"Bearer {api_key}"

        healthy = _fetch_healthy_models(litellm_url, headers)
        from codeforge.routing.key_filter import set_healthy_models

        set_healthy_models(healthy)

        models = self._fetch_and_filter_models(litellm_url, headers)
        if models is None:
            return

        best = _select_best_model(models, healthy)

        with self._lock:
            self._models = models
            self._healthy = healthy
            self._best = best
            self._last_refresh = time.monotonic()

        if models:
            logger.info(
                "model_resolver: discovered %d models, %d healthy, best=%s",
                len(models),
                len(healthy),
                best,
            )
        else:
            logger.warning("model_resolver: no models available from LiteLLM")

    @staticmethod
    def _fetch_and_filter_models(litellm_url: str, headers: dict[str, str]) -> list[str] | None:
        """Fetch models from LiteLLM and apply key filter. Returns None on error."""
        try:
            resp = httpx.get(f"{litellm_url}/v1/models", headers=headers, timeout=5.0)
            if resp.status_code != 200:
                logger.warning("model_resolver: LiteLLM /v1/models returned %d", resp.status_code)
                return None
            data = resp.json()
            raw_ids = [m.get("id", "") for m in data.get("data", []) if m.get("id")]
            models = expand_wildcard_models(raw_ids)
            from codeforge.routing.key_filter import filter_keyless_models

            return filter_keyless_models(models)
        except Exception as exc:
            logger.warning("model_resolver: failed to fetch from LiteLLM: %s", exc, exc_info=True)
            return None


def _select_best_model(models: list[str], healthy: set[str]) -> str:
    """Pick the best model: prefer healthy, skip exhausted providers."""
    if not models:
        return ""
    from codeforge.routing.rate_tracker import get_tracker

    tracker = get_tracker()

    def _first_available(candidates: list[str]) -> str:
        for m in candidates:
            provider = m.split("/")[0] if "/" in m else ""
            if provider and tracker.is_exhausted(provider):
                continue
            return m
        return ""

    # First: healthy + non-exhausted.
    best = _first_available([m for m in models if m in healthy])
    # Second: any non-exhausted.
    if not best:
        best = _first_available(models)
    return best or models[0]


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
