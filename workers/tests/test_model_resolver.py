"""Tests for centralized model resolver with LiteLLM auto-discovery."""

from __future__ import annotations

import os
import threading
import time
from unittest.mock import MagicMock, patch

import httpx
import pytest

from codeforge.model_resolver import _ModelCache, get_available_models, resolve_model
from codeforge.routing import rate_tracker
from codeforge.routing.rate_tracker import RateLimitTracker


def _refresh_count(mock_get: MagicMock) -> int:
    """Count LiteLLM /v1/models fetches; each refresh also probes /health."""
    return sum(1 for c in mock_get.call_args_list if c.args[0].endswith("/v1/models"))


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
            assert _refresh_count(mock_get) == 1

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
            assert _refresh_count(mock_get) == 2

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
    @pytest.fixture
    def tracker(self, monkeypatch: pytest.MonkeyPatch) -> RateLimitTracker:
        """Fresh module-level rate tracker; provider keys set so the key filter keeps every model."""
        fresh = RateLimitTracker()
        monkeypatch.setattr(rate_tracker, "_tracker", fresh)
        monkeypatch.setenv("ANTHROPIC_API_KEY", "test-key-anthropic")
        monkeypatch.setenv("GROQ_API_KEY", "test-key-groq")
        return fresh

    def test_best_skips_exhausted_provider(self, tracker: RateLimitTracker) -> None:
        """First model's provider is exhausted → best should be second model."""
        cache = _ModelCache()
        response = httpx.Response(
            200,
            json={"data": [{"id": "ollama/llama2"}, {"id": "anthropic/claude-sonnet"}]},
        )
        tracker.record_error("ollama", error_type="auth")

        with patch("codeforge.model_resolver.httpx.get", return_value=response):
            cache.get_models()
        assert cache.get_best() == "anthropic/claude-sonnet"

    def test_best_falls_back_when_all_exhausted(self, tracker: RateLimitTracker) -> None:
        """When all providers are exhausted, fall back to first model."""
        cache = _ModelCache()
        response = httpx.Response(
            200,
            json={"data": [{"id": "ollama/llama2"}, {"id": "groq/llama3"}]},
        )
        tracker.record_error("ollama", error_type="auth")
        tracker.record_error("groq", error_type="tpm_exceeded")

        with patch("codeforge.model_resolver.httpx.get", return_value=response):
            cache.get_models()
        # All exhausted → falls back to first
        assert cache.get_best() == "ollama/llama2"


class TestGetAvailableModels:
    def test_returns_list(self) -> None:
        with patch("codeforge.model_resolver._cache") as mock_cache:
            mock_cache.get_models.return_value = ["a", "b"]
            assert get_available_models() == ["a", "b"]


class TestRefreshBackoff:
    """KI-196 (R9-7): a failed refresh was retried on every call, each waiting up to 2 x 5 s for LiteLLM."""

    def test_a_failed_refresh_is_not_repeated_at_once(self) -> None:
        cache = _ModelCache()
        with patch("codeforge.model_resolver.httpx.get", side_effect=httpx.ConnectError("down")) as mock_get:
            assert cache.get_models() == []
            assert cache.get_best() == ""
            assert cache.get_models() == []
        assert _refresh_count(mock_get) == 1

    def test_the_refresh_is_tried_again_after_the_backoff(self) -> None:
        cache = _ModelCache()
        with patch("codeforge.model_resolver.httpx.get", side_effect=httpx.ConnectError("down")):
            cache.get_models()
        cache._retry_at = time.monotonic() - 1  # the backoff has passed
        response = httpx.Response(200, json={"data": [{"id": "back/model"}]})
        with patch("codeforge.model_resolver.httpx.get", return_value=response) as mock_get:
            assert cache.get_models() == ["back/model"]
        assert _refresh_count(mock_get) == 1

    def test_an_error_status_backs_off_too(self) -> None:
        cache = _ModelCache()
        with patch("codeforge.model_resolver.httpx.get", return_value=httpx.Response(503)) as mock_get:
            cache.get_models()
            cache.get_models()
        assert _refresh_count(mock_get) == 1

    def test_a_failure_keeps_the_last_good_list(self) -> None:
        cache = _ModelCache()
        with patch(
            "codeforge.model_resolver.httpx.get", return_value=httpx.Response(200, json={"data": [{"id": "a/m"}]})
        ):
            cache.get_models()
        cache._last_refresh = time.monotonic() - 120
        with patch("codeforge.model_resolver.httpx.get", side_effect=httpx.ConnectError("down")) as mock_get:
            assert cache.get_models() == ["a/m"]
            assert cache.get_models() == ["a/m"]
        assert _refresh_count(mock_get) == 1

    def test_concurrent_refreshes_fetch_once(self) -> None:
        cache = _ModelCache()
        started = threading.Event()
        release = threading.Event()

        def slow_get(url: str, **_kwargs: object) -> httpx.Response:
            if url.endswith("/v1/models"):
                started.set()
                release.wait(5)
            return httpx.Response(200, json={"data": [{"id": "one/model"}]})

        with patch("codeforge.model_resolver.httpx.get", side_effect=slow_get) as mock_get:
            first = threading.Thread(target=cache.get_models)
            first.start()
            assert started.wait(5)
            second = threading.Thread(target=cache.get_models)
            second.start()
            time.sleep(0.2)  # the second caller reaches the refresh while the first one runs
            release.set()
            first.join(5)
            second.join(5)
        assert _refresh_count(mock_get) == 1
