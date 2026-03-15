"""Prompt compressor utility for context-limited local models.

Truncates long text using a head+tail strategy so that the beginning
and end of the content are preserved. A ``[truncated]`` marker is
inserted between the two sections.

Budget split: ~60% head, ~40% tail.
"""

from __future__ import annotations

_TRUNCATION_MARKER = "\n[truncated]\n"


def compress_for_context(text: str, max_chars: int) -> str:
    """Compress *text* to fit within *max_chars* using head+tail truncation.

    Returns the original text unchanged when it already fits. For
    ``max_chars <= 0`` an empty string is returned.
    """
    if max_chars <= 0:
        return ""
    if len(text) <= max_chars:
        return text

    # Reserve space for the marker itself.
    budget = max_chars - len(_TRUNCATION_MARKER)
    if budget <= 0:
        # Even the marker doesn't fit — just return hard-truncated head.
        return text[:max_chars]

    head_budget = int(budget * 0.6)
    tail_budget = budget - head_budget

    head = text[:head_budget]
    tail = text[-tail_budget:] if tail_budget > 0 else ""

    return head + _TRUNCATION_MARKER + tail
