"""Risk scorer — mirrors Go internal/domain/quarantine/scorer.go.

Same regex patterns, same weights. Returns (score, factors) where score is [0.0, 1.0].
"""

from __future__ import annotations

import re
from typing import TYPE_CHECKING

from codeforge.trust.levels import TrustLevel

if TYPE_CHECKING:
    from codeforge.models import TrustAnnotation

# Precompiled patterns — identical to Go scorer.go.
_SHELL_PATTERN = re.compile(r"(;\s*rm\s|\|\s*curl\s|\|\s*wget\s|\|\s*bash|`[^`]+`)")
_SQL_PATTERN = re.compile(
    r"(DROP\s+TABLE|DELETE\s+FROM|TRUNCATE|ALTER\s+TABLE|INSERT\s+INTO.*SELECT)",
    re.IGNORECASE,
)
_PATH_PATTERN = re.compile(r"\.\.[/\\]")
_ENV_PATTERN = re.compile(r"(\$ENV|\$\{\w+\}|os\.environ|process\.env)")
_B64_PATTERN = re.compile(r"[A-Za-z0-9+/=]{100,}")


def score_message(
    annotation: TrustAnnotation | None,
    payload: bytes,
) -> tuple[float, list[str]]:
    """Compute a risk score for a message based on trust and content.

    Returns (score capped at 1.0, list of human-readable factor strings).
    """
    score = 0.0
    factors: list[str] = []

    # Trust-based scoring.
    if annotation is not None:
        level = annotation.trust_level
        if level == TrustLevel.UNTRUSTED:
            score += 0.5
            factors.append("untrusted source (+0.5)")
        elif level == TrustLevel.PARTIAL:
            score += 0.2
            factors.append("partial trust (+0.2)")

        if annotation.origin == "a2a":
            score += 0.1
            factors.append("A2A origin (+0.1)")

    # Content-based scoring.
    body = payload.decode(errors="replace")

    if _SHELL_PATTERN.search(body):
        score += 0.3
        factors.append("shell injection pattern detected")

    if _SQL_PATTERN.search(body):
        score += 0.2
        factors.append("SQL injection pattern detected")

    if _PATH_PATTERN.search(body):
        score += 0.2
        factors.append("path traversal pattern detected")

    if _ENV_PATTERN.search(body):
        score += 0.1
        factors.append("environment variable access detected")

    if _B64_PATTERN.search(body):
        score += 0.1
        factors.append("large base64 block detected")

    if body.count('"tool_call"') > 10 or body.count("tool_calls") > 10:
        score += 0.1
        factors.append("excessive tool calls detected")

    return min(score, 1.0), factors
