"""Test rate tracker error classification and cooldown expiration."""

from codeforge.llm import LLMError, classify_error_type, is_fallback_eligible
from codeforge.routing.rate_tracker import RateLimitTracker


class TestRateLimitTrackerErrorClassification:
    """Verify that billing/auth errors mark providers as exhausted."""

    def test_billing_error_marks_provider_exhausted(self) -> None:
        tracker = RateLimitTracker()
        assert not tracker.is_exhausted("anthropic")
        tracker.record_error("anthropic", error_type="billing")
        assert tracker.is_exhausted("anthropic")

    def test_auth_error_marks_provider_exhausted(self) -> None:
        tracker = RateLimitTracker()
        tracker.record_error("anthropic", error_type="auth")
        assert tracker.is_exhausted("anthropic")

    def test_error_appears_in_exhausted_providers(self) -> None:
        tracker = RateLimitTracker()
        tracker.record_error("anthropic", error_type="billing")
        tracker.record_error("openai", error_type="auth")
        exhausted = tracker.get_exhausted_providers()
        assert "anthropic" in exhausted
        assert "openai" in exhausted

    def test_unknown_error_type_ignored(self) -> None:
        tracker = RateLimitTracker()
        tracker.record_error("anthropic", error_type="unknown")
        assert not tracker.is_exhausted("anthropic")

    def test_billing_exhaustion_lasts_one_hour(self) -> None:
        tracker = RateLimitTracker()
        now = 1000.0
        tracker._now = lambda: now  # type: ignore[assignment]
        tracker.record_error("anthropic", error_type="billing")
        assert tracker.is_exhausted("anthropic")
        now = 1000.0 + 3599.0
        assert tracker.is_exhausted("anthropic")
        now = 1000.0 + 3601.0
        assert not tracker.is_exhausted("anthropic")

    def test_auth_exhaustion_lasts_five_minutes(self) -> None:
        tracker = RateLimitTracker()
        now = 1000.0
        tracker._now = lambda: now  # type: ignore[assignment]
        tracker.record_error("anthropic", error_type="auth")
        assert tracker.is_exhausted("anthropic")
        now = 1000.0 + 299.0
        assert tracker.is_exhausted("anthropic")
        now = 1000.0 + 301.0
        assert not tracker.is_exhausted("anthropic")


class TestClassifyErrorType:
    """Verify classify_error_type returns correct categories."""

    def test_402_returns_billing(self) -> None:
        exc = LLMError(status_code=402, model="gpt-4", body="Payment required")
        assert classify_error_type(exc) == "billing"

    def test_401_without_billing_keywords_returns_auth(self) -> None:
        exc = LLMError(status_code=401, model="gpt-4", body="Invalid API key")
        assert classify_error_type(exc) == "auth"

    def test_403_without_billing_keywords_returns_auth(self) -> None:
        exc = LLMError(status_code=403, model="gpt-4", body="Forbidden")
        assert classify_error_type(exc) == "auth"

    def test_401_with_billing_keywords_returns_billing(self) -> None:
        exc = LLMError(status_code=401, model="gpt-4", body="Your credits are exhausted")
        assert classify_error_type(exc) == "billing"

    def test_400_with_billing_body_returns_billing(self) -> None:
        exc = LLMError(status_code=400, model="gpt-4", body="Insufficient balance")
        assert classify_error_type(exc) == "billing"

    def test_400_with_auth_body_returns_auth(self) -> None:
        exc = LLMError(status_code=400, model="gpt-4", body="authentication failed")
        assert classify_error_type(exc) == "auth"

    def test_400_without_keywords_returns_none(self) -> None:
        exc = LLMError(status_code=400, model="gpt-4", body="Bad request")
        assert classify_error_type(exc) is None

    def test_500_returns_none(self) -> None:
        exc = LLMError(status_code=500, model="gpt-4", body="Internal error")
        assert classify_error_type(exc) is None

    def test_429_returns_rate_limit(self) -> None:
        exc = LLMError(status_code=429, model="gpt-4", body="Rate limited")
        assert classify_error_type(exc) == "rate_limit"


class TestIsFallbackEligible:
    """Verify is_fallback_eligible identifies fallback-worthy errors."""

    def test_429_always_eligible(self) -> None:
        exc = LLMError(status_code=429, model="gpt-4", body="Rate limited")
        assert is_fallback_eligible(exc) is True

    def test_429_eligible_without_keywords(self) -> None:
        exc = LLMError(status_code=429, model="gpt-4", body="slow down")
        assert is_fallback_eligible(exc) is True

    def test_402_always_eligible(self) -> None:
        exc = LLMError(status_code=402, model="gpt-4", body="Payment required")
        assert is_fallback_eligible(exc) is True

    def test_401_with_keywords_eligible(self) -> None:
        exc = LLMError(status_code=401, model="gpt-4", body="unauthorized access")
        assert is_fallback_eligible(exc) is True

    def test_400_without_keywords_not_eligible(self) -> None:
        exc = LLMError(status_code=400, model="gpt-4", body="Bad request")
        assert is_fallback_eligible(exc) is False

    def test_500_not_eligible(self) -> None:
        exc = LLMError(status_code=500, model="gpt-4", body="Internal error")
        assert is_fallback_eligible(exc) is False


class TestRateLimitCooldown:
    """Verify rate_limit error type marks provider exhausted with 60s cooldown."""

    def test_rate_limit_marks_exhausted(self) -> None:
        tracker = RateLimitTracker()
        tracker.record_error("groq", error_type="rate_limit")
        assert tracker.is_exhausted("groq")

    def test_rate_limit_cooldown_60s(self) -> None:
        tracker = RateLimitTracker()
        now = 1000.0
        tracker._now = lambda: now  # type: ignore[assignment]
        tracker.record_error("groq", error_type="rate_limit")
        assert tracker.is_exhausted("groq")
        now = 1000.0 + 59.0
        assert tracker.is_exhausted("groq")
        now = 1000.0 + 61.0
        assert not tracker.is_exhausted("groq")
