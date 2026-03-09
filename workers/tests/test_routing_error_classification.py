"""Test rate tracker error classification for billing/auth errors."""

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
