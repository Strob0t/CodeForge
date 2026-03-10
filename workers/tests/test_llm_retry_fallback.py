"""Tests for immediate model fallback on 429 rate-limit errors (F7).

Verifies that 429 errors propagate immediately to the agent loop's fallback
logic instead of retrying the same exhausted model with exponential backoff.
"""

from __future__ import annotations

import time
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.llm import LLMError


class TestRateLimitNotRetried:
    """429 errors should NOT be retried by _with_retry — they should raise immediately."""

    @pytest.mark.asyncio
    async def test_429_raises_immediately(self):
        """429 should NOT be retried — should raise on first attempt."""
        from codeforge.llm import LiteLLMClient

        client = LiteLLMClient(base_url="http://localhost:4000", api_key="test")

        call_count = 0

        async def mock_fn():
            nonlocal call_count
            call_count += 1
            raise LLMError(status_code=429, model="gemini/gemini-2.0-flash", body="rate limited")

        start = time.monotonic()
        with pytest.raises(LLMError) as exc_info:
            await client._with_retry(mock_fn)
        elapsed = time.monotonic() - start

        assert exc_info.value.status_code == 429
        assert call_count == 1, "429 should NOT be retried, expected exactly 1 call"
        assert elapsed < 2.0, f"429 should fail fast, took {elapsed:.1f}s"

    @pytest.mark.asyncio
    async def test_502_is_still_retried(self):
        """502 errors should still be retried with backoff."""
        from codeforge.llm import LiteLLMClient

        client = LiteLLMClient(base_url="http://localhost:4000", api_key="test")

        call_count = 0

        async def mock_fn():
            nonlocal call_count
            call_count += 1
            raise LLMError(status_code=502, model="openai/gpt-4o", body="bad gateway")

        with pytest.raises(LLMError) as exc_info:
            await client._with_retry(mock_fn)

        assert exc_info.value.status_code == 502
        assert call_count > 1, "502 should be retried"

    @pytest.mark.asyncio
    async def test_503_is_still_retried(self):
        """503 errors should still be retried with backoff."""
        from codeforge.llm import LiteLLMClient

        client = LiteLLMClient(base_url="http://localhost:4000", api_key="test")

        call_count = 0

        async def mock_fn():
            nonlocal call_count
            call_count += 1
            raise LLMError(status_code=503, model="openai/gpt-4o", body="service unavailable")

        with pytest.raises(LLMError) as exc_info:
            await client._with_retry(mock_fn)

        assert exc_info.value.status_code == 503
        assert call_count > 1, "503 should be retried"

    @pytest.mark.asyncio
    async def test_504_is_still_retried(self):
        """504 gateway timeout should still be retried."""
        from codeforge.llm import LiteLLMClient

        client = LiteLLMClient(base_url="http://localhost:4000", api_key="test")

        call_count = 0

        async def mock_fn():
            nonlocal call_count
            call_count += 1
            raise LLMError(status_code=504, model="openai/gpt-4o", body="gateway timeout")

        with pytest.raises(LLMError) as exc_info:
            await client._with_retry(mock_fn)

        assert exc_info.value.status_code == 504
        assert call_count > 1, "504 should be retried"


class TestFallbackChain:
    """Agent loop should switch models immediately on 429."""

    @pytest.mark.asyncio
    async def test_model_fallback_on_429(self):
        """On 429, _try_model_fallback should switch to next model."""
        from codeforge.agent_loop import AgentLoopExecutor, LoopConfig

        mock_llm = AsyncMock()
        mock_registry = MagicMock()
        mock_runtime = MagicMock()
        mock_runtime.run_id = "test"
        mock_runtime.send_output = AsyncMock()

        executor = AgentLoopExecutor(
            llm=mock_llm,
            tool_registry=mock_registry,
            runtime=mock_runtime,
            workspace_path="/tmp",
        )

        cfg = LoopConfig(
            model="gemini/gemini-2.0-flash",
            fallback_models=["openai/gpt-4o-mini", "anthropic/claude-haiku-3.5"],
        )

        from codeforge.agent_loop import _LoopState

        state = _LoopState()

        exc = LLMError(status_code=429, model="gemini/gemini-2.0-flash", body="rate limited")
        result = await executor._try_model_fallback(cfg, state, exc)

        assert result is None, "Should return None to signal retry with new model"
        assert cfg.model == "openai/gpt-4o-mini", "Should switch to first fallback"
        assert "gemini/gemini-2.0-flash" in state.failed_models

    @pytest.mark.asyncio
    async def test_fallback_chain_exhausted(self):
        """When all fallback models are already failed, should return error string."""
        from codeforge.agent_loop import AgentLoopExecutor, LoopConfig

        mock_llm = AsyncMock()
        mock_registry = MagicMock()
        mock_runtime = MagicMock()
        mock_runtime.run_id = "test"
        mock_runtime.send_output = AsyncMock()

        executor = AgentLoopExecutor(
            llm=mock_llm,
            tool_registry=mock_registry,
            runtime=mock_runtime,
            workspace_path="/tmp",
        )

        cfg = LoopConfig(
            model="gemini/gemini-2.0-flash",
            fallback_models=["openai/gpt-4o-mini"],
        )

        from codeforge.agent_loop import _LoopState

        state = _LoopState()
        state.failed_models = {"openai/gpt-4o-mini"}  # only fallback already failed

        exc = LLMError(status_code=429, model="gemini/gemini-2.0-flash", body="rate limited")
        result = await executor._try_model_fallback(cfg, state, exc)

        assert result is not None, "Should return error when all fallbacks exhausted"
        assert "LLM call failed" in result

    @pytest.mark.asyncio
    async def test_429_total_time_under_2_seconds(self):
        """A 429 on the first model should resolve to fallback in <2s total."""
        from codeforge.llm import LiteLLMClient

        client = LiteLLMClient(base_url="http://localhost:4000", api_key="test")

        start = time.monotonic()

        with pytest.raises(LLMError):
            await client._with_retry(
                AsyncMock(side_effect=LLMError(status_code=429, model="test", body="rate limited"))
            )

        elapsed = time.monotonic() - start
        assert elapsed < 2.0, f"429 should propagate immediately, took {elapsed:.1f}s"


class TestRateLimitTrackerIntegration:
    """Verify rate tracker records 429 errors when they propagate."""

    @pytest.mark.asyncio
    async def test_429_recorded_in_tracker(self):
        """When 429 propagates, the agent loop records it in the rate tracker."""
        from codeforge.llm import classify_error_type
        from codeforge.routing.rate_tracker import get_tracker

        exc = LLMError(status_code=429, model="gemini/gemini-2.0-flash", body="rate limited")
        err_type = classify_error_type(exc)

        assert err_type == "rate_limit"

        tracker = get_tracker()
        tracker.record_error("gemini", error_type=err_type)
        assert tracker.is_exhausted("gemini")
