"""Multi-rollout runner — runs N independent rollouts and selects the best.

Inspired by R2E-Gym/EntroPO test-time scaling: running multiple independent
agent attempts on the same task and selecting the best via hybrid verification
dramatically improves results (59.8% @16 rollouts vs 51.6% @1).
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import TYPE_CHECKING, Protocol

import structlog

if TYPE_CHECKING:
    from codeforge.evaluation.hybrid_pipeline import HybridEvaluationPipeline, VerificationResult
    from codeforge.evaluation.providers.base import ExecutionResult, TaskSpec

logger = structlog.get_logger()


class _InnerRunner(Protocol):
    """Protocol for any runner that can execute a single task."""

    async def run_single_task(self, task: TaskSpec) -> ExecutionResult: ...


@dataclass
class RolloutOutcome:
    """Result of a single rollout within a multi-rollout execution."""

    rollout_id: int
    result: ExecutionResult
    verification: VerificationResult | None = None
    diversity_score: float = 0.0
    is_best: bool = False


class MultiRolloutRunner:
    """Wraps any benchmark runner to execute N independent rollouts per task.

    Args:
        inner_runner: Runner implementing ``run_single_task(task) -> ExecutionResult``.
        hybrid_pipeline: Optional hybrid verifier for best-of-N selection.
        rollout_count: Number of independent rollouts per task.
        strategy: Selection strategy — ``"best"`` (hybrid select) or ``"majority"`` (vote).
    """

    def __init__(
        self,
        inner_runner: _InnerRunner,
        hybrid_pipeline: HybridEvaluationPipeline | None,
        rollout_count: int = 1,
        strategy: str = "best",
    ) -> None:
        self._inner = inner_runner
        self._hybrid = hybrid_pipeline
        self._rollout_count = max(1, rollout_count)
        self._strategy = strategy

    async def run_task(self, task: TaskSpec) -> list[RolloutOutcome]:
        """Run the task N times and return all outcomes with selection markers."""
        # Phase 1: Execute N independent rollouts.
        outcomes: list[RolloutOutcome] = []
        for i in range(self._rollout_count):
            result = await self._inner.run_single_task(task)
            outcomes.append(RolloutOutcome(rollout_id=i, result=result))

        if self._rollout_count == 1:
            outcomes[0].is_best = True
            return outcomes

        # Phase 2: Compute diversity scores.
        outputs = [o.result.actual_output for o in outcomes]
        diversity_scores = compute_diversity(outputs)
        for outcome, div_score in zip(outcomes, diversity_scores, strict=True):
            outcome.diversity_score = div_score

        # Phase 3: Select best.
        if self._strategy == "best" and self._hybrid is not None:
            await self._select_best_hybrid(task, outcomes)
        elif self._strategy == "majority":
            self._select_majority(outcomes)
        else:
            # No pipeline: first rollout is best (fallback).
            outcomes[0].is_best = True

        return outcomes

    async def _select_best_hybrid(self, task: TaskSpec, outcomes: list[RolloutOutcome]) -> None:
        """Use hybrid verification to select the best rollout."""
        results = [o.result for o in outcomes]
        verifications = await self._hybrid.verify_batch(task, results)  # type: ignore[union-attr]

        # verify_batch returns results sorted by combined score desc.
        # Map verifications back to outcomes by matching ExecutionResult identity.
        result_to_verification: dict[int, tuple[int, VerificationResult]] = {}
        for _rank, _vr in enumerate(verifications):
            # Find which outcome this verification belongs to by matching scores.
            for outcome in outcomes:
                if id(outcome.result) not in {id(r) for _, r in result_to_verification.values()}:
                    # Match by comparing filter scores and rank scores presence.
                    pass

        # Simpler approach: re-verify and sort outcomes directly.
        # Since verify_batch sorts internally, we just need to find the best.
        best_idx = -1
        best_score = -1.0

        for i, outcome in enumerate(outcomes):
            vr = await self._hybrid.verify(task, outcome.result)  # type: ignore[union-attr]
            outcome.verification = vr
            score = vr.combined_score.average_score() if vr.combined_score else -1.0
            if score > best_score:
                best_score = score
                best_idx = i

        if best_idx >= 0:
            outcomes[best_idx].is_best = True

        logger.info(
            "best-of-N selected",
            task_id=task.id,
            best_rollout=best_idx,
            best_score=best_score,
            rollout_count=len(outcomes),
        )

    def _select_majority(self, outcomes: list[RolloutOutcome]) -> None:
        """Select best via majority vote on pass/fail (exit_code == 0)."""
        passing = [o for o in outcomes if o.result.exit_code == 0]
        failing = [o for o in outcomes if o.result.exit_code != 0]

        if len(passing) >= len(failing):
            # Majority passes — pick first passing rollout as best.
            if passing:
                passing[0].is_best = True
            elif outcomes:
                outcomes[0].is_best = True
        else:
            # Majority fails — still pick first passing if any, else first.
            if passing:
                passing[0].is_best = True
            elif outcomes:
                outcomes[0].is_best = True


def compute_diversity(outputs: list[str]) -> list[float]:
    """Compute diversity score for each output using normalized edit distance.

    Each output's diversity score is the average normalized edit distance
    to all other outputs. Higher = more different from peers.

    Returns:
        List of diversity scores (0.0-1.0), one per output.
    """
    n = len(outputs)
    if n <= 1:
        return [0.0] * n

    scores: list[float] = []
    for i in range(n):
        total_dist = 0.0
        for j in range(n):
            if i != j:
                total_dist += _normalized_edit_distance(outputs[i], outputs[j])
        scores.append(total_dist / (n - 1))

    return scores


def _normalized_edit_distance(a: str, b: str) -> float:
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
