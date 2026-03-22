"""Reactive blocklist for models that fail with auth/billing errors.

Models are blocked with a TTL. After expiry they become eligible again,
self-healing when API keys are added or billing is restored.

Provides both a ``ModelBlocklist`` class for dependency injection and a
module-level default instance via ``get_blocklist()`` for backward
compatibility.
"""

from __future__ import annotations

import logging
import threading
import time
from dataclasses import dataclass
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Callable

from codeforge.config import get_settings

logger = logging.getLogger(__name__)

_settings = get_settings()
_DEFAULT_BLOCK_TTL = _settings.model_block_ttl
_AUTH_BLOCK_TTL = _settings.model_auth_block_ttl


@dataclass(frozen=True)
class BlockEntry:
    """A blocked model with expiration metadata."""

    model: str
    reason: str
    blocked_at: float
    ttl: float
    auth_failure: bool = False


class ModelBlocklist:
    """Thread-safe blocklist for models that returned auth/billing errors."""

    __slots__ = ("_blocked", "_lock", "_now")

    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._blocked: dict[str, BlockEntry] = {}
        self._now: Callable[[], float] = time.monotonic

    def block(self, model: str, reason: str = "", ttl: float = _DEFAULT_BLOCK_TTL) -> None:
        """Block a model for *ttl* seconds."""
        entry = BlockEntry(model=model, reason=reason, blocked_at=self._now(), ttl=ttl)
        with self._lock:
            self._blocked[model] = entry
        logger.warning("model_blocklist: blocked %s for %.0fs (reason: %s)", model, ttl, reason)

    def block_auth(self, model: str, reason: str = "") -> None:
        """Block a model for auth/billing failures (long TTL)."""
        entry = BlockEntry(
            model=model,
            reason=reason,
            blocked_at=self._now(),
            ttl=_AUTH_BLOCK_TTL,
            auth_failure=True,
        )
        with self._lock:
            self._blocked[model] = entry
        logger.warning(
            "model_blocklist: auth-blocked %s for %.0fs (reason: %s)",
            model,
            _AUTH_BLOCK_TTL,
            reason,
        )

    def is_blocked(self, model: str) -> bool:
        """Return True if *model* is currently blocked (not expired).

        Note: The check-then-remove pattern below has a theoretical TOCTOU race
        under threading, but in practice this is safe because:
        1. The codeforge worker uses asyncio (single-threaded event loop).
        2. The lock protects the dict read; expiry removal is best-effort cleanup.
        3. A stale block entry only causes a brief extra block period (fail-safe).
        """
        with self._lock:
            entry = self._blocked.get(model)
        if entry is None:
            return False
        if (self._now() - entry.blocked_at) > entry.ttl:
            with self._lock:
                self._blocked.pop(model, None)
            return False
        return True

    def filter_available(self, models: list[str]) -> list[str]:
        """Return models not currently blocked."""
        return [m for m in models if not self.is_blocked(m)]

    def get_blocked(self) -> dict[str, BlockEntry]:
        """Return snapshot of currently blocked (non-expired) models."""
        now = self._now()
        with self._lock:
            return {k: v for k, v in self._blocked.items() if (now - v.blocked_at) <= v.ttl}


_blocklist = ModelBlocklist()


def get_blocklist() -> ModelBlocklist:
    """Return the module-level model blocklist singleton."""
    return _blocklist
