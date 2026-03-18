"""Shared string similarity utilities for rollout comparison.

Extracted to avoid circular imports between multi_rollout and early_stopping.
"""

from __future__ import annotations


def normalized_edit_distance(a: str, b: str) -> float:
    """Compute normalized Levenshtein distance between two strings.

    Returns a value in [0.0, 1.0] where 0 = identical, 1 = completely different.
    Uses a fast approximation for long strings to avoid O(n*m) cost.
    """
    if a == b:
        return 0.0

    max_len = max(len(a), len(b))
    if max_len == 0:
        return 0.0

    # For very long strings, use character frequency approximation.
    if max_len > 5000:
        return _char_freq_distance(a, b)

    # Standard Levenshtein with two-row optimization.
    la, lb = len(a), len(b)
    if la > lb:
        a, b = b, a
        la, lb = lb, la

    prev = list(range(la + 1))
    for j in range(1, lb + 1):
        curr = [j] + [0] * la
        for i in range(1, la + 1):
            cost = 0 if a[i - 1] == b[j - 1] else 1
            curr[i] = min(curr[i - 1] + 1, prev[i] + 1, prev[i - 1] + cost)
        prev = curr

    return prev[la] / max_len


def _char_freq_distance(a: str, b: str) -> float:
    """Approximate string distance using character frequency vectors."""
    from collections import Counter

    ca, cb = Counter(a), Counter(b)
    all_chars = set(ca) | set(cb)
    diff = sum(abs(ca.get(c, 0) - cb.get(c, 0)) for c in all_chars)
    total = sum(ca.values()) + sum(cb.values())
    return diff / total if total > 0 else 0.0
