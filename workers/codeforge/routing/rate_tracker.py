"""Per-provider rate-limit tracker fed by LLM response headers.

Tracks remaining request quota per provider and exposes which providers
are currently exhausted.  Used by HybridRouter to skip rate-limited
providers during model selection.
"""

from __future__ import annotations

import threading
import time
from dataclasses import dataclass
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Callable


@dataclass
class RateLimitInfo:
    """Snapshot of a provider's rate-limit state from response headers."""

    remaining_requests: int | None = None
    limit_requests: int | None = None
    reset_after_seconds: float | None = None
    provider: str = ""
    timestamp: float = 0.0


class RateLimitTracker:
    """Thread-safe per-provider rate-limit state tracker.

    State is updated after every LLM response (from ``x-ratelimit-*``
    headers).  The HybridRouter queries ``is_exhausted`` /
    ``get_exhausted_providers`` to skip rate-limited providers during
    model selection.
    """

    __slots__ = ("_lock", "_now", "_state")

    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._state: dict[str, RateLimitInfo] = {}
        self._now: Callable[[], float] = time.monotonic

    # -- mutations -----------------------------------------------------------

    def update(self, provider: str, info: RateLimitInfo) -> None:
        """Record the latest rate-limit state for *provider*."""
        with self._lock:
            self._state[provider] = info

    # -- queries -------------------------------------------------------------

    def is_exhausted(self, provider: str) -> bool:
        """Return ``True`` if *provider* has no remaining requests.

        A provider is considered exhausted when ``remaining_requests == 0``
        and the reset window has not yet elapsed.  If no data exists for the
        provider (e.g. it never sent rate-limit headers) it is **not**
        considered exhausted.
        """
        with self._lock:
            info = self._state.get(provider)
        if info is None:
            return False
        if info.remaining_requests is None:
            return False
        if info.remaining_requests > 0:
            return False
        # remaining == 0 — check if the reset window has passed.
        return not self._is_stale(info)

    def get_exhausted_providers(self) -> set[str]:
        """Return the set of providers that are currently rate-limited."""
        with self._lock:
            providers = list(self._state.keys())
        return {p for p in providers if self.is_exhausted(p)}

    def get_best_reset_time(self) -> float | None:
        """Return the shortest ``reset_after_seconds`` among exhausted providers.

        Returns ``None`` when no provider is exhausted.  Useful for the
        caller to decide how long to sleep before retrying.
        """
        best: float | None = None
        with self._lock:
            for info in self._state.values():
                if (
                    info.remaining_requests is not None
                    and info.remaining_requests == 0
                    and not self._is_stale(info)
                    and info.reset_after_seconds is not None
                    and (best is None or info.reset_after_seconds < best)
                ):
                    best = info.reset_after_seconds
        return best

    # -- internal ------------------------------------------------------------

    def _is_stale(self, info: RateLimitInfo) -> bool:
        """Return ``True`` if the info's reset window has elapsed."""
        if info.reset_after_seconds is None:
            # No reset info — assume a default 60s window.
            return (self._now() - info.timestamp) > 60.0
        return (self._now() - info.timestamp) > info.reset_after_seconds


# Module-level singleton.
_tracker = RateLimitTracker()


def get_tracker() -> RateLimitTracker:
    """Return the module-level rate-limit tracker singleton."""
    return _tracker
