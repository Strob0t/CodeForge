"""Trust middleware — stamp outgoing payloads, validate incoming annotations."""

from __future__ import annotations

from typing import TYPE_CHECKING

from codeforge.trust.levels import TrustLevel, internal_annotation, meets_minimum

if TYPE_CHECKING:
    from codeforge.models import TrustAnnotation


def stamp_outgoing(payload: dict, source_id: str = "python-worker") -> dict:
    """Add a trust annotation to *payload* if not already present.

    Returns the (possibly mutated) payload dict.
    """
    if payload.get("trust"):
        return payload
    ann = internal_annotation(source_id)
    payload["trust"] = ann.model_dump()
    return payload


def validate_incoming(
    annotation: TrustAnnotation | None,
    min_level: TrustLevel = TrustLevel.UNTRUSTED,
) -> bool:
    """Return True if *annotation* meets the minimum trust level.

    None annotations are treated as untrusted.
    """
    if annotation is None:
        return meets_minimum(TrustLevel.UNTRUSTED, min_level)
    return meets_minimum(TrustLevel(annotation.trust_level), min_level)
