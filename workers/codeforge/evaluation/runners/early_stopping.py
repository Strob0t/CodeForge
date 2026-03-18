"""Confidence-based early stopping for multi-rollout execution.

Stops multi-rollout execution early when enough rollouts agree, saving
40-60% cost. After each rollout (starting from rollout 2), computes
pairwise similarity and forms agreement clusters. If any cluster meets
the quorum and all members have exit_code == 0, signals early stop.
"""

from __future__ import annotations

import os
from dataclasses import dataclass

from codeforge.evaluation.runners._similarity import normalized_edit_distance


@dataclass
class _RolloutEntry:
    """Internal record of a single completed rollout."""

    rollout_id: int
    output: str
    exit_code: int
    score: float


class EarlyStopChecker:
    """Check whether multi-rollout execution can stop early.

    After each rollout is added, computes pairwise similarity with all
    prior rollouts and forms agreement clusters. If any cluster has
    ``>= quorum`` members and all have ``exit_code == 0``, signals stop.

    Args:
        threshold: Minimum pairwise similarity (1 - edit_distance) to
            consider two rollouts as "agreeing". Default 0.9.
        quorum: Minimum cluster size to trigger early stop. Default 3.
    """

    def __init__(
        self,
        threshold: float | None = None,
        quorum: int | None = None,
    ) -> None:
        default_threshold = float(os.environ.get("CODEFORGE_EARLY_STOP_THRESHOLD", "0.9"))
        default_quorum = int(os.environ.get("CODEFORGE_EARLY_STOP_QUORUM", "3"))
        self._threshold = threshold if threshold is not None else default_threshold
        self._quorum = quorum if quorum is not None else default_quorum
        self._rollouts: list[_RolloutEntry] = []
        self._stopped = False
        self._best_cluster: list[_RolloutEntry] = []

    def add_rollout(self, rollout_id: int, output: str, exit_code: int, score: float = 0.0) -> None:
        """Record a completed rollout."""
        self._rollouts.append(
            _RolloutEntry(
                rollout_id=rollout_id,
                output=output,
                exit_code=exit_code,
                score=score,
            )
        )

    def should_stop(self) -> bool:
        """Return True if an agreement cluster meets the quorum.

        Clusters are formed greedily: for each rollout, collect all peers
        whose pairwise similarity exceeds the threshold. If any such
        cluster has ``>= quorum`` members *and* all members have
        ``exit_code == 0``, we can stop.
        """
        n = len(self._rollouts)
        if n < self._quorum:
            return False

        best_cluster: list[_RolloutEntry] = []

        for i in range(n):
            # Build cluster around rollout i: includes i itself plus all
            # rollouts whose similarity to i exceeds the threshold.
            cluster = [self._rollouts[i]]
            for j in range(n):
                if i == j:
                    continue
                dist = normalized_edit_distance(self._rollouts[i].output, self._rollouts[j].output)
                similarity = 1.0 - dist
                if similarity >= self._threshold:
                    cluster.append(self._rollouts[j])

            if len(cluster) < self._quorum:
                continue

            # All members must have exit_code == 0.
            if not all(r.exit_code == 0 for r in cluster):
                continue

            if len(cluster) > len(best_cluster):
                best_cluster = cluster

        if best_cluster:
            self._stopped = True
            self._best_cluster = best_cluster
            return True

        return False

    def best_from_cluster(self) -> int:
        """Return the rollout_id with the highest score in the best cluster.

        Returns -1 if no quorum has been met.
        """
        if not self._best_cluster:
            return -1

        # Sort by score descending, then by rollout_id ascending for stability.
        best = min(
            self._best_cluster,
            key=lambda r: (-r.score, r.rollout_id),
        )
        return best.rollout_id

    @property
    def completed_count(self) -> int:
        """Number of rollouts added so far."""
        return len(self._rollouts)

    @property
    def early_stopped(self) -> bool:
        """Whether early stopping was triggered."""
        return self._stopped
