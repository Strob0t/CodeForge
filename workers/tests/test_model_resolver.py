"""Tests for centralized model resolver with LiteLLM auto-discovery."""

from __future__ import annotations

import os
import time
from unittest.mock import patch

import httpx
import pytest

from codeforge.model_resolver import _ModelCache, get_available_models, resolve_model


class TestResolveModel:
    def test_explicit_takes_precedence(self) -> None:
        assert resolve_model("explicit/model") == "explicit/model"

    def test_env_var_fallback(self) -> None:
        with patch.dict(os.environ, {"CODEFORGE_DEFAULT_MODEL": "env/model"}):
            assert resolve_model() == "env/model"

    def test_env_var_empty_skipped(self) -> None:
        with (
            patch.dict(os.environ, {"CODEFORGE_DEFAULT_MODEL": ""}, clear=False),
            patch("codeforge.model_resolver._cache") as mock_cache,
        ):
            mock_cache.get_best.return_value = "discovered/model"
            assert resolve_model() == "discovered/model"

    def test_litellm_discovery(self) -> None:
        with patch.dict(os.environ, {}, clear=True), patch("codeforge.model_resolver._cache") as mock_cache:
            mock_cache.get_best.return_value = "auto/discovered"
            assert resolve_model() == "auto/discovered"

    def test_no_models_raises(self) -> None:
        with patch.dict(os.environ, {}, clear=True), patch("codeforge.model_resolver._cache") as mock_cache:
            mock_cache.get_best.return_value = ""
            with pytest.raises(RuntimeError, match="No LLM model available"):
                resolve_model()


class TestModelCache:
    def test_refresh_from_litellm(self) -> None:
        cache = _ModelCache()
        response = httpx.Response(
            200,
            json={"data": [{"id": "model-a"}, {"id": "model-b"}]},
        )
        with patch("codeforge.model_resolver.httpx.get", return_value=response):
            models = cache.get_models()
        assert models == ["model-a", "model-b"]
        assert cache.get_best() == "model-a"

    def test_refresh_litellm_down(self) -> None:
        cache = _ModelCache()
        with patch("codeforge.model_resolver.httpx.get", side_effect=httpx.ConnectError("down")):
            models = cache.get_models()
        assert models == []
        assert cache.get_best() == ""

    def test_refresh_litellm_error_status(self) -> None:
        cache = _ModelCache()
        response = httpx.Response(500, text="Internal Server Error")
        with patch("codeforge.model_resolver.httpx.get", return_value=response):
            models = cache.get_models()
        assert models == []

    def test_cache_reuses_within_ttl(self) -> None:
        cache = _ModelCache()
        response = httpx.Response(
            200,
            json={"data": [{"id": "cached-model"}]},
        )
        with patch("codeforge.model_resolver.httpx.get", return_value=response) as mock_get:
            cache.get_models()
            cache.get_models()
            assert mock_get.call_count == 1

    def test_cache_refreshes_after_ttl(self) -> None:
        cache = _ModelCache()
        response = httpx.Response(
            200,
            json={"data": [{"id": "fresh-model"}]},
        )
        with patch("codeforge.model_resolver.httpx.get", return_value=response) as mock_get:
            cache.get_models()
            # Force staleness by backdating the last refresh.
            cache._last_refresh = time.monotonic() - 120
            cache.get_models()
            assert mock_get.call_count == 2

    def test_empty_model_ids_filtered(self) -> None:
        cache = _ModelCache()
        response = httpx.Response(
            200,
            json={"data": [{"id": ""}, {"id": "valid"}, {}]},
        )
        with patch("codeforge.model_resolver.httpx.get", return_value=response):
            models = cache.get_models()
        assert models == ["valid"]


class TestModelCacheExhaustionFiltering:
    def test_best_skips_exhausted_provider(self) -> None:
        """First model's provider is exhausted → best should be second model."""
        from codeforge.routing.rate_tracker import get_tracker

        cache = _ModelCache()
        response = httpx.Response(
            200,
            json={"data": [{"id": "ollama/llama2"}, {"id": "anthropic/claude-sonnet"}]},
        )
        tracker = get_tracker()
        tracker.record_error("ollama", error_type="auth")

        with patch("codeforge.model_resolver.httpx.get", return_value=response):
            cache.get_models()
        assert cache.get_best() == "anthropic/claude-sonnet"

        # Cleanup tracker state
        tracker._state.pop("ollama", None)

    def test_best_falls_back_when_all_exhausted(self) -> None:
        """When all providers are exhausted, fall back to first model."""
        from codeforge.routing.rate_tracker import get_tracker

        cache = _ModelCache()
        response = httpx.Response(
            200,
            json={"data": [{"id": "ollama/llama2"}, {"id": "groq/llama3"}]},
        )
        tracker = get_tracker()
        tracker.record_error("ollama", error_type="auth")
        tracker.record_error("groq", error_type="tpm_exceeded")

        with patch("codeforge.model_resolver.httpx.get", return_value=response):
            cache.get_models()
        # All exhausted → falls back to first
        assert cache.get_best() == "ollama/llama2"

        # Cleanup
        tracker._state.pop("ollama", None)
        tracker._state.pop("groq", None)


class TestGetAvailableModels:
    def test_returns_list(self) -> None:
        with patch("codeforge.model_resolver._cache") as mock_cache:
            mock_cache.get_models.return_value = ["a", "b"]
            assert get_available_models() == ["a", "b"]
