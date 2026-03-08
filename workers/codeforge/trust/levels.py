"""Trust levels and comparison — mirrors Go internal/domain/trust/trust.go."""

from __future__ import annotations

from datetime import UTC, datetime
from enum import StrEnum

from codeforge.models import TrustAnnotation


class TrustLevel(StrEnum):
    """Trust level of a message source (highest to lowest)."""

    FULL = "full"
    VERIFIED = "verified"
    PARTIAL = "partial"
    UNTRUSTED = "untrusted"


_RANK: dict[TrustLevel, int] = {
    TrustLevel.UNTRUSTED: 0,
    TrustLevel.PARTIAL: 1,
    TrustLevel.VERIFIED: 2,
    TrustLevel.FULL: 3,
}


def rank(level: TrustLevel) -> int:
    """Return the numeric rank of a trust level. Unknown levels return -1."""
    return _RANK.get(level, -1)


def meets_minimum(level: TrustLevel, minimum: TrustLevel) -> bool:
    """Return True if *level* is at least *minimum*."""
    return rank(level) >= rank(minimum)


def internal_annotation(source_id: str) -> TrustAnnotation:
    """Create a trust annotation for an internal CodeForge agent."""
    return TrustAnnotation(
        origin="internal",
        trust_level=TrustLevel.FULL,
        source_id=source_id,
        timestamp=datetime.now(UTC).isoformat(),
    )
