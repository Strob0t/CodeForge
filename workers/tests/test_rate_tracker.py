"""Tests for the per-provider rate-limit tracker."""

from __future__ import annotations

from codeforge.routing.rate_tracker import RateLimitInfo, RateLimitTracker, get_tracker


class TestRateLimitTracker:
    """Unit tests for RateLimitTracker state management."""

    def _make_tracker(self, now: float | None = None) -> RateLimitTracker:
        t = RateLimitTracker()
        if now is not None:
            t._now = lambda: now  # type: ignore[assignment]
        return t

    def test_update_and_query(self) -> None:
        tracker = self._make_tracker(now=100.0)
        info = RateLimitInfo(
            remaining_requests=5,
            limit_requests=60,
            reset_after_seconds=10.0,
            provider="groq",
            timestamp=100.0,
        )
        tracker.update("groq", info)
        assert not tracker.is_exhausted("groq")

    def test_exhausted_when_remaining_zero(self) -> None:
        tracker = self._make_tracker(now=100.0)
        info = RateLimitInfo(
            remaining_requests=0,
            limit_requests=60,
            reset_after_seconds=30.0,
            provider="mistral",
            timestamp=100.0,
        )
        tracker.update("mistral", info)
        assert tracker.is_exhausted("mistral")

    def test_not_exhausted_when_remaining_positive(self) -> None:
        tracker = self._make_tracker(now=100.0)
        info = RateLimitInfo(
            remaining_requests=10,
            limit_requests=60,
            reset_after_seconds=30.0,
            provider="groq",
            timestamp=100.0,
        )
        tracker.update("groq", info)
        assert not tracker.is_exhausted("groq")

    def test_recovery_after_reset(self) -> None:
        tracker = self._make_tracker(now=100.0)
        info = RateLimitInfo(
            remaining_requests=0,
            limit_requests=60,
            reset_after_seconds=10.0,
            provider="groq",
            timestamp=100.0,
        )
        tracker.update("groq", info)
        assert tracker.is_exhausted("groq")

        # Advance time past reset window.
        tracker._now = lambda: 111.0  # type: ignore[assignment]
        assert not tracker.is_exhausted("groq")

    def test_get_exhausted_providers(self) -> None:
        tracker = self._make_tracker(now=100.0)
        tracker.update(
            "mistral",
            RateLimitInfo(
                remaining_requests=0, limit_requests=60, reset_after_seconds=30.0, provider="mistral", timestamp=100.0
            ),
        )
        tracker.update(
            "groq",
            RateLimitInfo(
                remaining_requests=5, limit_requests=30, reset_after_seconds=60.0, provider="groq", timestamp=100.0
            ),
        )
        tracker.update(
            "openai",
            RateLimitInfo(
                remaining_requests=0, limit_requests=100, reset_after_seconds=20.0, provider="openai", timestamp=100.0
            ),
        )
        exhausted = tracker.get_exhausted_providers()
        assert exhausted == {"mistral", "openai"}

    def test_stale_info_clears(self) -> None:
        tracker = self._make_tracker(now=100.0)
        tracker.update(
            "mistral",
            RateLimitInfo(
                remaining_requests=0, limit_requests=60, reset_after_seconds=5.0, provider="mistral", timestamp=100.0
            ),
        )
        assert tracker.is_exhausted("mistral")

        # 6 seconds later — past the 5s reset window.
        tracker._now = lambda: 106.0  # type: ignore[assignment]
        assert not tracker.is_exhausted("mistral")
        assert tracker.get_exhausted_providers() == set()

    def test_unknown_provider_never_exhausted(self) -> None:
        tracker = self._make_tracker()
        assert not tracker.is_exhausted("unknown_provider")
        assert tracker.get_exhausted_providers() == set()

    def test_get_best_reset_time(self) -> None:
        tracker = self._make_tracker(now=100.0)
        tracker.update(
            "mistral",
            RateLimitInfo(
                remaining_requests=0, limit_requests=60, reset_after_seconds=30.0, provider="mistral", timestamp=100.0
            ),
        )
        tracker.update(
            "openai",
            RateLimitInfo(
                remaining_requests=0, limit_requests=100, reset_after_seconds=10.0, provider="openai", timestamp=100.0
            ),
        )
        # Best reset is the soonest: openai resets at 100+10=110, mistral at 100+30=130.
        best = tracker.get_best_reset_time()
        assert best is not None
        assert best == 10.0  # openai's reset_after_seconds

    def test_get_best_reset_time_none_when_no_exhausted(self) -> None:
        tracker = self._make_tracker()
        assert tracker.get_best_reset_time() is None

    def test_remaining_none_not_exhausted(self) -> None:
        """Provider with remaining=None (no header) should never be exhausted."""
        tracker = self._make_tracker(now=100.0)
        tracker.update(
            "ollama",
            RateLimitInfo(
                remaining_requests=None,
                limit_requests=None,
                reset_after_seconds=None,
                provider="ollama",
                timestamp=100.0,
            ),
        )
        assert not tracker.is_exhausted("ollama")


class TestGetTracker:
    """Test the module-level singleton."""

    def test_singleton_returns_same_instance(self) -> None:
        t1 = get_tracker()
        t2 = get_tracker()
        assert t1 is t2

    def test_singleton_is_tracker(self) -> None:
        assert isinstance(get_tracker(), RateLimitTracker)
