"""Tests for keyless model pre-validation filter."""

from __future__ import annotations

import logging

import pytest

from codeforge.routing.key_filter import (
    PROVIDER_KEY_MAP,
    filter_keyless_models,
    reset_warnings,
)


@pytest.fixture(autouse=True)
def _clean_env(monkeypatch: pytest.MonkeyPatch) -> None:
    """Remove all provider key env vars so each test starts clean."""
    for env_var in PROVIDER_KEY_MAP.values():
        monkeypatch.delenv(env_var, raising=False)
    reset_warnings()


class TestFilterKeylessModels:
    def test_ollama_always_has_key(self) -> None:
        result = filter_keyless_models(["ollama/llama3", "ollama/mistral"])
        assert result == ["ollama/llama3", "ollama/mistral"]

    def test_known_provider_with_key_set(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("OPENAI_API_KEY", "test-key-openai-123")
        result = filter_keyless_models(["openai/gpt-4o"])
        assert result == ["openai/gpt-4o"]

    def test_known_provider_with_empty_key(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("OPENAI_API_KEY", "")
        result = filter_keyless_models(["openai/gpt-4o"])
        assert result == []

    def test_known_provider_with_whitespace_key(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("OPENAI_API_KEY", "   ")
        result = filter_keyless_models(["openai/gpt-4o"])
        assert result == []

    def test_known_provider_key_unset(self) -> None:
        result = filter_keyless_models(["anthropic/claude-3-opus"])
        assert result == []

    def test_unknown_provider_kept(self) -> None:
        result = filter_keyless_models(["newprovider/model-x"])
        assert result == ["newprovider/model-x"]

    def test_filters_openai_keeps_groq(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("GROQ_API_KEY", "test-key-groq")
        models = ["openai/gpt-4o", "groq/llama-3.3-70b", "openai/gpt-4o-mini"]
        result = filter_keyless_models(models)
        assert result == ["groq/llama-3.3-70b"]

    def test_keeps_all_when_all_keys_set(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("OPENAI_API_KEY", "test-key-openai")
        monkeypatch.setenv("ANTHROPIC_API_KEY", "test-key-anthropic")
        models = ["openai/gpt-4o", "anthropic/claude-3-opus"]
        result = filter_keyless_models(models)
        assert result == ["openai/gpt-4o", "anthropic/claude-3-opus"]

    def test_keeps_models_without_provider_prefix(self) -> None:
        result = filter_keyless_models(["gpt-4o", "claude-3-opus"])
        assert result == ["gpt-4o", "claude-3-opus"]

    def test_empty_input(self) -> None:
        assert filter_keyless_models([]) == []

    def test_preserves_order(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("GROQ_API_KEY", "test-key-groq")
        monkeypatch.setenv("MISTRAL_API_KEY", "test-key-mistral")
        models = ["groq/llama-3.3-70b", "mistral/mistral-large", "groq/mixtral"]
        result = filter_keyless_models(models)
        assert result == ["groq/llama-3.3-70b", "mistral/mistral-large", "groq/mixtral"]

    def test_warning_logged_once_per_provider(self, caplog: pytest.LogCaptureFixture) -> None:
        with caplog.at_level(logging.WARNING):
            filter_keyless_models(["openai/gpt-4o", "openai/gpt-4o-mini"])
        warnings = [r for r in caplog.records if r.levelno == logging.WARNING]
        assert len(warnings) == 1
        assert "openai" in warnings[0].message

    def test_github_copilot_with_token_set(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("GITHUB_TOKEN", "ghu_test-token")
        result = filter_keyless_models(["github_copilot/gpt-4o"])
        assert result == ["github_copilot/gpt-4o"]

    def test_github_copilot_without_token(self) -> None:
        result = filter_keyless_models(["github_copilot/gpt-4o"])
        assert result == []

    def test_all_providers_mapped(self) -> None:
        expected = {"openai", "anthropic", "gemini", "groq", "mistral", "github_copilot"}
        assert expected.issubset(PROVIDER_KEY_MAP.keys())
