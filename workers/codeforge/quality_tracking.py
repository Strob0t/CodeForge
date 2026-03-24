"""Iteration quality tracking and rollout scoring for the agentic loop.

Tracks per-iteration quality signals for mid-loop model switching (C1)
and provides rollout scoring/selection for inference-time scaling (A4).
"""

from __future__ import annotations

import math
from collections import deque
from difflib import SequenceMatcher
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from codeforge.models import AgentLoopResult
    from codeforge.routing.models import ComplexityTier

# Quality tracking constants.
_QUALITY_WINDOW = 3
_LOW_QUALITY_THRESHOLD = 0.3
_MIN_MEANINGFUL_OUTPUT = 50


class IterationQualityTracker:
    """Track per-iteration quality signals for mid-loop model switching (C1)."""

    MAX_SWITCHES: int = 2

    def __init__(self) -> None:
        self._records: deque[float] = deque(maxlen=_QUALITY_WINDOW)
        self._iteration_signals: list[float] = []
        self.switch_count: int = 0

    def record(self, tool_success: bool, output_length: int) -> None:
        """Record a single tool call outcome within the current iteration."""
        if tool_success and output_length >= _MIN_MEANINGFUL_OUTPUT:
            self._records.append(1.0)
        elif tool_success:
            self._records.append(0.0)  # success but empty/short output
        else:
            self._records.append(0.0)

    def signal(self) -> float:
        """Compute quality signal from the last N tool calls. 0.5 if no data."""
        if not self._records:
            return 0.5
        return sum(self._records) / len(self._records)

    def end_iteration(self) -> None:
        """Mark end of an iteration, recording its signal for consecutive-low tracking."""
        self._iteration_signals.append(self.signal())
        self._records.clear()

    def should_switch(self) -> bool:
        """Return True if 2+ consecutive iterations had low quality AND switches remain."""
        if self.switch_count >= self.MAX_SWITCHES:
            return False
        if len(self._iteration_signals) < 2:
            return False
        last_two = self._iteration_signals[-2:]
        return all(s < _LOW_QUALITY_THRESHOLD for s in last_two)

    def register_switch(self) -> None:
        """Record that a model switch occurred."""
        self.switch_count += 1
        self._iteration_signals.clear()

    @staticmethod
    def bump_tier(current: ComplexityTier) -> ComplexityTier:
        """Bump complexity tier by one level, capping at REASONING."""
        from codeforge.routing.models import ComplexityTier

        order = [ComplexityTier.SIMPLE, ComplexityTier.MEDIUM, ComplexityTier.COMPLEX, ComplexityTier.REASONING]
        try:
            idx = order.index(current)
        except ValueError:
            return ComplexityTier.MEDIUM
        return order[min(idx + 1, len(order) - 1)]


def compute_rollout_score(result: AgentLoopResult) -> float:
    """Score a single rollout result using quality metrics.

    Scoring formula (0.0-1.0):
    - 0.0 if the result has an error
    - Otherwise: weighted combination of content length, step count, and tool usage
    """
    if result.error:
        return 0.0

    content_len = len(result.final_content)
    if content_len == 0:
        return 0.0

    # Content quality: logarithmic scale, capped at 1.0
    # Short outputs (<50 chars) score low; 500+ chars score near 1.0
    content_score = min(1.0, math.log1p(content_len) / math.log1p(500))

    # Step efficiency: penalize excessive steps (>30 steps starts to reduce score)
    max_efficient_steps = 30
    step_score = min(1.0, result.step_count / max_efficient_steps) if result.step_count > 0 else 0.1

    # Tool usage: having tool messages indicates productive work
    tool_score = min(1.0, len(result.tool_messages) / 5) if result.tool_messages else 0.0

    # Weighted combination: content is most important, then tool usage, then step efficiency
    return 0.5 * content_score + 0.3 * tool_score + 0.2 * step_score


def select_best_rollout(
    results: list[AgentLoopResult],
    scores: list[float],
) -> int:
    """Select the best rollout index by score, excluding errored results."""
    best_idx = 0
    best_score = -1.0
    for i, (result, score) in enumerate(zip(results, scores, strict=True)):
        has_error = bool(getattr(result, "error", ""))
        effective_score = score if not has_error else -1.0
        if effective_score > best_score:
            best_score = effective_score
            best_idx = i
    return best_idx


def should_early_stop(
    outputs: list[str],
    exit_codes: list[int],
    total_rollouts: int,
    threshold: float = 0.9,
    quorum: int = 3,
) -> bool:
    """Return True if enough rollouts agree to stop early."""
    if total_rollouts <= 3:
        return False
    if len(outputs) < quorum:
        return False

    # Check if quorum of outputs are similar AND all have exit_code == 0.
    n = len(outputs)
    for i in range(n):
        if exit_codes[i] != 0:
            continue
        cluster = [i]
        for j in range(i + 1, n):
            if exit_codes[j] != 0:
                continue
            sim = SequenceMatcher(None, outputs[i], outputs[j]).ratio()
            if sim >= threshold:
                cluster.append(j)
        if len(cluster) >= quorum:
            return True
    return False
