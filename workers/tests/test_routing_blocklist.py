"""Tests for the reactive model blocklist."""

from __future__ import annotations

import threading
import time

from codeforge.routing.blocklist import ModelBlocklist, get_blocklist


class TestModelBlocklist:
    """Unit tests for ModelBlocklist."""

    def _make_blocklist(self, now: float = 1000.0) -> ModelBlocklist:
        """Create a blocklist with a controllable clock."""
        bl = ModelBlocklist()
        bl._now = lambda: now  # type: ignore[assignment]
        return bl

    def test_block_and_is_blocked(self) -> None:
        bl = self._make_blocklist()
        bl.block("openai/gpt-4o-mini", reason="HTTP 401")
        assert bl.is_blocked("openai/gpt-4o-mini") is True

    def test_unblocked_model_not_blocked(self) -> None:
        bl = self._make_blocklist()
        bl.block("openai/gpt-4o-mini", reason="HTTP 401")
        assert bl.is_blocked("groq/llama-3.1-8b-instant") is False

    def test_ttl_expiry_unblocks(self) -> None:
        current_time = 1000.0
        bl = ModelBlocklist()
        bl._now = lambda: current_time  # type: ignore[assignment]

        bl.block("openai/gpt-4o-mini", reason="HTTP 401", ttl=60.0)
        assert bl.is_blocked("openai/gpt-4o-mini") is True

        # Advance time past TTL.
        current_time = 1061.0
        bl._now = lambda: current_time  # type: ignore[assignment]
        assert bl.is_blocked("openai/gpt-4o-mini") is False

    def test_filter_available_removes_blocked(self) -> None:
        bl = self._make_blocklist()
        bl.block("openai/gpt-4o-mini", reason="HTTP 401")
        models = ["openai/gpt-4o-mini", "groq/llama-3.1-8b-instant", "anthropic/claude-sonnet-4"]
        result = bl.filter_available(models)
        assert result == ["groq/llama-3.1-8b-instant", "anthropic/claude-sonnet-4"]

    def test_filter_available_preserves_order(self) -> None:
        bl = self._make_blocklist()
        bl.block("anthropic/claude-sonnet-4", reason="HTTP 403")
        models = ["groq/llama-3.1-8b-instant", "openai/gpt-4o-mini", "anthropic/claude-sonnet-4"]
        result = bl.filter_available(models)
        assert result == ["groq/llama-3.1-8b-instant", "openai/gpt-4o-mini"]

    def test_empty_blocklist_returns_all(self) -> None:
        bl = self._make_blocklist()
        models = ["openai/gpt-4o-mini", "groq/llama-3.1-8b-instant"]
        assert bl.filter_available(models) == models

    def test_block_same_model_resets_ttl(self) -> None:
        current_time = 1000.0
        bl = ModelBlocklist()
        bl._now = lambda: current_time  # type: ignore[assignment]

        bl.block("openai/gpt-4o-mini", reason="first", ttl=60.0)

        # Advance 50s, re-block with new TTL.
        current_time = 1050.0
        bl._now = lambda: current_time  # type: ignore[assignment]
        bl.block("openai/gpt-4o-mini", reason="second", ttl=60.0)

        # At t=1080 (30s after re-block), should still be blocked.
        current_time = 1080.0
        bl._now = lambda: current_time  # type: ignore[assignment]
        assert bl.is_blocked("openai/gpt-4o-mini") is True

        # At t=1111 (61s after re-block), should be unblocked.
        current_time = 1111.0
        bl._now = lambda: current_time  # type: ignore[assignment]
        assert bl.is_blocked("openai/gpt-4o-mini") is False

    def test_get_blocked_excludes_expired(self) -> None:
        current_time = 1000.0
        bl = ModelBlocklist()
        bl._now = lambda: current_time  # type: ignore[assignment]

        bl.block("model-a", reason="401", ttl=30.0)
        bl.block("model-b", reason="403", ttl=120.0)

        # Advance past model-a's TTL but not model-b's.
        current_time = 1031.0
        bl._now = lambda: current_time  # type: ignore[assignment]

        blocked = bl.get_blocked()
        assert "model-a" not in blocked
        assert "model-b" in blocked

    def test_filter_empty_list(self) -> None:
        bl = self._make_blocklist()
        bl.block("openai/gpt-4o-mini", reason="401")
        assert bl.filter_available([]) == []


class TestSingleton:
    """Test module-level singleton."""

    def test_singleton_returns_same_instance(self) -> None:
        a = get_blocklist()
        b = get_blocklist()
        assert a is b


class TestAuthBlocklist:
    """Tests for auth-failure-specific blocking (24h TTL)."""

    def _make_blocklist(self, now: float = 1000.0) -> ModelBlocklist:
        bl = ModelBlocklist()
        bl._now = lambda: now  # type: ignore[assignment]
        return bl

    def test_block_auth_uses_long_ttl(self) -> None:
        bl = self._make_blocklist()
        bl.block_auth("openai/gpt-4o-mini", reason="HTTP 401")
        entry = bl.get_blocked()["openai/gpt-4o-mini"]
        assert entry.ttl == 86400.0

    def test_block_auth_sets_auth_failure_flag(self) -> None:
        bl = self._make_blocklist()
        bl.block_auth("openai/gpt-4o-mini", reason="HTTP 401")
        entry = bl.get_blocked()["openai/gpt-4o-mini"]
        assert entry.auth_failure is True

    def test_auth_blocked_model_survives_default_ttl(self) -> None:
        """After 301s (past default 300s TTL), auth-blocked model is still blocked."""
        current_time = 1000.0
        bl = ModelBlocklist()
        bl._now = lambda: current_time  # type: ignore[assignment]

        bl.block_auth("openai/gpt-4o-mini", reason="HTTP 401")
        assert bl.is_blocked("openai/gpt-4o-mini") is True

        # Advance past default TTL (300s) but within auth TTL (86400s).
        current_time = 1301.0
        bl._now = lambda: current_time  # type: ignore[assignment]
        assert bl.is_blocked("openai/gpt-4o-mini") is True

    def test_auth_blocked_model_expires_after_auth_ttl(self) -> None:
        current_time = 1000.0
        bl = ModelBlocklist()
        bl._now = lambda: current_time  # type: ignore[assignment]

        bl.block_auth("openai/gpt-4o-mini", reason="HTTP 401")

        # Advance past auth TTL (86400s).
        current_time = 1000.0 + 86401.0
        bl._now = lambda: current_time  # type: ignore[assignment]
        assert bl.is_blocked("openai/gpt-4o-mini") is False

    def test_default_block_still_uses_short_ttl(self) -> None:
        """Regular block() is unaffected — still uses the default TTL."""
        bl = self._make_blocklist()
        bl.block("groq/llama-3.1-8b-instant", reason="HTTP 429")
        entry = bl.get_blocked()["groq/llama-3.1-8b-instant"]
        assert entry.ttl == 300.0  # default
        assert entry.auth_failure is False


class TestThreadSafety:
    """Concurrent access tests."""

    def test_concurrent_block_and_check(self) -> None:
        bl = ModelBlocklist()
        errors: list[Exception] = []
        models = [f"provider/model-{i}" for i in range(20)]

        def blocker() -> None:
            try:
                for m in models:
                    bl.block(m, reason="test")
                    time.sleep(0.001)
            except Exception as exc:
                errors.append(exc)

        def checker() -> None:
            try:
                for _ in range(50):
                    bl.filter_available(models)
                    time.sleep(0.001)
            except Exception as exc:
                errors.append(exc)

        threads = [
            threading.Thread(target=blocker),
            threading.Thread(target=blocker),
            threading.Thread(target=checker),
            threading.Thread(target=checker),
        ]
        for t in threads:
            t.start()
        for t in threads:
            t.join(timeout=10)

        assert not errors, f"Thread safety errors: {errors}"
