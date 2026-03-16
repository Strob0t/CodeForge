"""Tests for Bug 3 (model validation) and Bug 4 (routing guard).

Bug 3: _validate_model_exists() rejects unknown models before benchmark runs.
Bug 4: _resolve_effective_llm() raises ValueError when model=auto without routing.
"""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock

import pytest

# ---------------------------------------------------------------------------
# Bug 3: _validate_model_exists
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_known_model_passes() -> None:
    """Known model in available list should not raise."""
    from codeforge.consumer._benchmark import _validate_model_exists

    # Should complete without error
    await _validate_model_exists("gpt-4o", available_models=["gpt-4o", "claude-3-opus"])


@pytest.mark.asyncio
async def test_unknown_model_raises() -> None:
    """Unknown model should raise ValueError with 'not available' message."""
    from codeforge.consumer._benchmark import _validate_model_exists

    with pytest.raises(ValueError, match="not available"):
        await _validate_model_exists(
            "nonexistent-model",
            available_models=["gpt-4o", "claude-3-opus"],
        )


@pytest.mark.asyncio
async def test_auto_model_skips_validation() -> None:
    """model='auto' should skip validation entirely, even with a populated list."""
    from codeforge.consumer._benchmark import _validate_model_exists

    # Should not raise even though "auto" is not in the list
    await _validate_model_exists("auto", available_models=["gpt-4o"])


@pytest.mark.asyncio
async def test_empty_available_list_raises_when_litellm_unreachable(monkeypatch) -> None:
    """Empty available list (LiteLLM unreachable) should raise ValueError."""
    from codeforge.consumer import _benchmark

    # Both endpoints return empty — LiteLLM completely unreachable
    async def mock_fetch_configured() -> list[str]:
        return []

    monkeypatch.setattr(_benchmark, "_fetch_configured_models", mock_fetch_configured)

    with pytest.raises(ValueError, match="LiteLLM is unreachable"):
        await _benchmark._validate_model_exists("nonexistent-model", available_models=[])


# ---------------------------------------------------------------------------
# Bug 4: _resolve_effective_llm raises when model=auto without routing
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_auto_without_router_raises() -> None:
    """model=auto with router=None should raise ValueError mentioning 'routing'."""
    from codeforge.consumer._benchmark import BenchmarkHandlerMixin

    mixin = BenchmarkHandlerMixin.__new__(BenchmarkHandlerMixin)
    mixin._llm = MagicMock()
    mixin._get_hybrid_router = AsyncMock(return_value=None)

    req = MagicMock()
    req.model = "auto"
    log = MagicMock()

    with pytest.raises(ValueError, match="routing"):
        await mixin._resolve_effective_llm(req, log)


@pytest.mark.asyncio
async def test_non_auto_returns_llm_directly() -> None:
    """Non-auto model should return the raw LLM client without routing."""
    from codeforge.consumer._benchmark import BenchmarkHandlerMixin

    mixin = BenchmarkHandlerMixin.__new__(BenchmarkHandlerMixin)
    sentinel_llm = MagicMock(name="sentinel_llm")
    mixin._llm = sentinel_llm

    req = MagicMock()
    req.model = "gpt-4o"
    log = MagicMock()

    result = await mixin._resolve_effective_llm(req, log)
    assert result is sentinel_llm


# ---------------------------------------------------------------------------
# Issue A: _validate_model_exists with provider-prefixed models
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_provider_prefixed_model_not_in_list_raises() -> None:
    """Model with provider prefix not in available list should raise."""
    from codeforge.consumer._benchmark import _validate_model_exists

    with pytest.raises(ValueError, match="not available"):
        await _validate_model_exists(
            "nonexistent/model-xyz-404",
            available_models=["lm_studio/qwen3-30b-a3b", "openai/gpt-4"],
        )


@pytest.mark.asyncio
async def test_provider_prefixed_model_in_list_passes() -> None:
    """Model with provider prefix that IS in available list should pass."""
    from codeforge.consumer._benchmark import _validate_model_exists

    await _validate_model_exists(
        "lm_studio/qwen3-30b-a3b",
        available_models=["lm_studio/qwen3-30b-a3b", "openai/gpt-4"],
    )


@pytest.mark.asyncio
async def test_substring_model_does_not_match() -> None:
    """Model that is a substring of a valid model should not pass."""
    from codeforge.consumer._benchmark import _validate_model_exists

    with pytest.raises(ValueError, match="not available"):
        await _validate_model_exists(
            "lm_studio/qwen3",
            available_models=["lm_studio/qwen3-30b-a3b"],
        )


@pytest.mark.asyncio
async def test_fallback_to_configured_models(monkeypatch) -> None:
    """When /v1/models returns empty, should try /model/info endpoint."""
    from codeforge.consumer import _benchmark

    async def mock_fetch_available() -> list[str]:
        return []  # Simulate LiteLLM unreachable via /v1/models

    async def mock_fetch_configured() -> list[str]:
        return ["lm_studio/qwen3-30b-a3b"]

    monkeypatch.setattr(_benchmark, "_fetch_available_models", mock_fetch_available)
    monkeypatch.setattr(_benchmark, "_fetch_configured_models", mock_fetch_configured)

    # Valid model in configured list should pass
    await _benchmark._validate_model_exists("lm_studio/qwen3-30b-a3b")


@pytest.mark.asyncio
async def test_fallback_to_configured_rejects_unknown(monkeypatch) -> None:
    """When /v1/models returns empty but /model/info works, reject unknown models."""
    from codeforge.consumer import _benchmark

    async def mock_fetch_available() -> list[str]:
        return []

    async def mock_fetch_configured() -> list[str]:
        return ["lm_studio/qwen3-30b-a3b"]

    monkeypatch.setattr(_benchmark, "_fetch_available_models", mock_fetch_available)
    monkeypatch.setattr(_benchmark, "_fetch_configured_models", mock_fetch_configured)

    with pytest.raises(ValueError, match="not found in LiteLLM configured"):
        await _benchmark._validate_model_exists("nonexistent/model-xyz-404")


@pytest.mark.asyncio
async def test_both_endpoints_unreachable_raises(monkeypatch) -> None:
    """When both /v1/models AND /model/info return empty, raise ValueError."""
    from codeforge.consumer import _benchmark

    async def mock_fetch_available() -> list[str]:
        return []

    async def mock_fetch_configured() -> list[str]:
        return []

    monkeypatch.setattr(_benchmark, "_fetch_available_models", mock_fetch_available)
    monkeypatch.setattr(_benchmark, "_fetch_configured_models", mock_fetch_configured)

    with pytest.raises(ValueError, match="LiteLLM is unreachable"):
        await _benchmark._validate_model_exists("nonexistent/model-xyz-404")
