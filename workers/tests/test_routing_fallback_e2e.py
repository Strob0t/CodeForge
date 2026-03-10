"""End-to-end test: billing error on primary provider triggers fallback to secondary.

Validates F3.6: When the primary provider returns a billing error (402),
the agent loop classifies the error, marks the provider as exhausted,
and transparently falls back to the next available provider.
"""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.agent_loop import AgentLoopExecutor, LoopConfig
from codeforge.llm import LLMError, classify_error_type
from codeforge.routing.rate_tracker import RateLimitTracker, get_tracker


class TestRoutingFallbackE2E:
    """Verify full routing fallback chain: error -> classify -> mark exhausted -> switch model."""

    def test_billing_error_classifies_and_exhausts_provider(self) -> None:
        """402 billing error -> classify -> mark provider exhausted -> skip on next call."""
        tracker = RateLimitTracker()

        # Simulate: Anthropic returns 402
        exc = LLMError(status_code=402, model="anthropic/claude-sonnet-4", body="Your credits have been exhausted")
        error_type = classify_error_type(exc)
        assert error_type == "billing"

        # Record the error
        provider = "anthropic"
        tracker.record_error(provider, error_type=error_type)

        # Provider should now be exhausted
        assert tracker.is_exhausted("anthropic")
        assert "anthropic" in tracker.get_exhausted_providers()

        # Other providers should still be available
        assert not tracker.is_exhausted("mistral")
        assert not tracker.is_exhausted("openai")

    def test_fallback_chain_skips_exhausted_providers(self) -> None:
        """After billing error, model selection skips the exhausted provider."""
        tracker = RateLimitTracker()

        # Exhaust anthropic
        tracker.record_error("anthropic", error_type="billing")

        # Simulate HybridRouter model selection logic: filter out exhausted providers
        available_models = [
            "anthropic/claude-sonnet-4",
            "mistral/mistral-large-latest",
            "openai/gpt-4o",
        ]
        exhausted = tracker.get_exhausted_providers()

        # Filter: remove models whose provider prefix is exhausted
        filtered = [m for m in available_models if m.split("/", 1)[0] not in exhausted]

        assert "anthropic/claude-sonnet-4" not in filtered
        assert "mistral/mistral-large-latest" in filtered
        assert "openai/gpt-4o" in filtered

    @pytest.mark.asyncio
    async def test_agent_loop_model_fallback_on_billing_error(self) -> None:
        """AgentLoopExecutor._try_model_fallback switches model on 402."""
        # Set up executor with mocked runtime
        runtime = MagicMock()
        runtime.run_id = "test-run-1"
        runtime.send_output = AsyncMock()

        executor = AgentLoopExecutor.__new__(AgentLoopExecutor)
        executor._runtime = runtime

        # Config with primary + fallback
        cfg = LoopConfig(
            model="anthropic/claude-sonnet-4",
            fallback_models=["mistral/mistral-large-latest", "openai/gpt-4o"],
        )

        # Create a simple _LoopState-like object
        class FakeState:
            def __init__(self) -> None:
                self.failed_models: set[str] = set()

        state = FakeState()

        # Simulate billing error
        exc = LLMError(status_code=402, model="anthropic/claude-sonnet-4", body="Payment required")

        # Call the fallback handler
        result = await executor._try_model_fallback(cfg, state, exc)

        # Should return None (meaning "retry with new model"), not an error string
        assert result is None

        # Model should have been switched
        assert cfg.model == "mistral/mistral-large-latest"
        assert "anthropic/claude-sonnet-4" in state.failed_models

        # Runtime should have notified the user
        runtime.send_output.assert_called_once()
        msg = runtime.send_output.call_args[0][0]
        assert "anthropic/claude-sonnet-4" in msg
        assert "mistral/mistral-large-latest" in msg

        # Provider should be marked exhausted in the global tracker
        assert get_tracker().is_exhausted("anthropic")

    @pytest.mark.asyncio
    async def test_agent_loop_exhausts_all_fallbacks(self) -> None:
        """When all fallback models fail, _try_model_fallback returns error string."""
        runtime = MagicMock()
        runtime.run_id = "test-run-2"
        runtime.send_output = AsyncMock()

        executor = AgentLoopExecutor.__new__(AgentLoopExecutor)
        executor._runtime = runtime

        cfg = LoopConfig(
            model="anthropic/claude-sonnet-4",
            fallback_models=["mistral/mistral-large-latest"],
        )

        class FakeState:
            def __init__(self) -> None:
                self.failed_models: set[str] = set()

        state = FakeState()

        # First failure: anthropic -> switches to mistral
        exc1 = LLMError(status_code=402, model="anthropic/claude-sonnet-4", body="Payment required")
        result1 = await executor._try_model_fallback(cfg, state, exc1)
        assert result1 is None
        assert cfg.model == "mistral/mistral-large-latest"

        # Second failure: mistral -> no more fallbacks
        exc2 = LLMError(status_code=401, model="mistral/mistral-large-latest", body="Unauthorized")
        result2 = await executor._try_model_fallback(cfg, state, exc2)
        assert result2 is not None  # error string
        assert "failed" in result2.lower()

    def test_auth_error_also_triggers_fallback(self) -> None:
        """401 auth error is also fallback-eligible and marks provider exhausted."""
        tracker = RateLimitTracker()
        exc = LLMError(status_code=401, model="openai/gpt-4o", body="Invalid API key")
        error_type = classify_error_type(exc)
        assert error_type == "auth"

        tracker.record_error("openai", error_type=error_type)
        assert tracker.is_exhausted("openai")

    def test_rate_limit_triggers_short_cooldown(self) -> None:
        """429 rate limit marks provider exhausted for only 60s (vs 1h for billing)."""
        tracker = RateLimitTracker()
        now = 1000.0
        tracker._now = lambda: now  # type: ignore[assignment]

        tracker.record_error("groq", error_type="rate_limit")
        assert tracker.is_exhausted("groq")

        # After 61s, should recover
        now = 1061.0
        assert not tracker.is_exhausted("groq")

        # Billing would still be exhausted at 61s
        tracker.record_error("anthropic", error_type="billing")
        now = 2061.0  # 1000s from billing record
        assert tracker.is_exhausted("anthropic")
