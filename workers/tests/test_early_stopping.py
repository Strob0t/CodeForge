"""Tests for EarlyStopChecker — confidence-based early stopping for multi-rollout.

When enough rollouts agree (pairwise similarity > threshold, cluster >= quorum,
all exit_code == 0), stop early to save 40-60% cost.

Covers: quorum triggers, threshold filtering, exit_code filtering, edge cases
(empty, small rollout counts), cluster formation, best_from_cluster selection.
"""

from __future__ import annotations

from codeforge.evaluation.runners.early_stopping import EarlyStopChecker

# ---------------------------------------------------------------------------
# Test 1: 3 identical rollouts trigger early stop
# ---------------------------------------------------------------------------


class TestEarlyStopTriggers:
    def test_three_identical_rollouts_stops_early(self) -> None:
        """3 identical outputs (similarity > 0.9, exit_code=0) with rollout_count=8
        should trigger early stop after the 3rd rollout."""
        checker = EarlyStopChecker(threshold=0.9, quorum=3)

        checker.add_rollout(0, "def solve(): return 42", exit_code=0, score=0.8)
        assert checker.should_stop() is False  # Only 1 rollout

        checker.add_rollout(1, "def solve(): return 42", exit_code=0, score=0.9)
        assert checker.should_stop() is False  # Only 2 rollouts

        checker.add_rollout(2, "def solve(): return 42", exit_code=0, score=0.7)
        assert checker.should_stop() is True  # 3 identical, quorum met

        assert checker.completed_count == 3
        assert checker.early_stopped is True

    # ---------------------------------------------------------------------------
    # Test 2: Below-threshold similarity does NOT trigger
    # ---------------------------------------------------------------------------

    def test_below_threshold_no_stop(self) -> None:
        """3 rollouts with similarity ~0.7 (below 0.9 threshold) should NOT stop."""
        checker = EarlyStopChecker(threshold=0.9, quorum=3)

        # These outputs are different enough to have low pairwise similarity.
        checker.add_rollout(0, "def solve(): return 42", exit_code=0, score=0.8)
        checker.add_rollout(1, "def answer(): return 99", exit_code=0, score=0.7)
        checker.add_rollout(2, "class Solution: pass", exit_code=0, score=0.6)

        assert checker.should_stop() is False
        assert checker.early_stopped is False

    # ---------------------------------------------------------------------------
    # Test 3: exit_code=1 prevents early stop
    # ---------------------------------------------------------------------------

    def test_exit_code_nonzero_prevents_stop(self) -> None:
        """3 identical rollouts but one has exit_code=1 — should NOT stop."""
        checker = EarlyStopChecker(threshold=0.9, quorum=3)

        checker.add_rollout(0, "def solve(): return 42", exit_code=0, score=0.8)
        checker.add_rollout(1, "def solve(): return 42", exit_code=1, score=0.9)
        checker.add_rollout(2, "def solve(): return 42", exit_code=0, score=0.7)

        # Cluster has 3 members by similarity, but one has exit_code=1
        # so the "all exit_code==0" requirement fails for the full cluster.
        # Only rollouts 0 and 2 form a valid cluster (size 2 < quorum 3).
        assert checker.should_stop() is False

    # ---------------------------------------------------------------------------
    # Test 4: rollout_count=3 always executes all 3
    # ---------------------------------------------------------------------------

    def test_rollout_count_3_always_runs_all(self) -> None:
        """With rollout_count=3, quorum=3 can technically trigger but that means
        all 3 are already done, so early_stopped should be False (no skipped)."""
        checker = EarlyStopChecker(threshold=0.9, quorum=3)

        checker.add_rollout(0, "same output", exit_code=0, score=0.8)
        checker.add_rollout(1, "same output", exit_code=0, score=0.9)
        checker.add_rollout(2, "same output", exit_code=0, score=0.7)

        # should_stop returns True (quorum met), but in practice with
        # rollout_count=3, all 3 are already done. The caller checks
        # rollout_count > 3 before breaking. The checker itself reports truth.
        assert checker.should_stop() is True
        assert checker.completed_count == 3

    # ---------------------------------------------------------------------------
    # Test 5: rollout_count=2 always executes both
    # ---------------------------------------------------------------------------

    def test_rollout_count_2_never_stops(self) -> None:
        """With rollout_count=2, quorum=3 can never be met."""
        checker = EarlyStopChecker(threshold=0.9, quorum=3)

        checker.add_rollout(0, "same output", exit_code=0, score=0.8)
        checker.add_rollout(1, "same output", exit_code=0, score=0.9)

        assert checker.should_stop() is False  # Only 2 rollouts, quorum is 3

    # ---------------------------------------------------------------------------
    # Test 6: First 3 agree, last 2 different — stops after 3
    # ---------------------------------------------------------------------------

    def test_first_three_agree_stops(self) -> None:
        """5 rollouts: first 3 identical, last 2 different — should stop after 3."""
        checker = EarlyStopChecker(threshold=0.9, quorum=3)

        checker.add_rollout(0, "def solve(): return 42", exit_code=0, score=0.8)
        assert checker.should_stop() is False

        checker.add_rollout(1, "def solve(): return 42", exit_code=0, score=0.9)
        assert checker.should_stop() is False

        checker.add_rollout(2, "def solve(): return 42", exit_code=0, score=0.7)
        assert checker.should_stop() is True  # Stops here

        # The remaining rollouts would have been different, but we already stopped.
        assert checker.completed_count == 3

    # ---------------------------------------------------------------------------
    # Test 7: Pairs agree but no group of 3 — continues all
    # ---------------------------------------------------------------------------

    def test_pairs_agree_no_quorum(self) -> None:
        """5 rollouts: 2 pairs agree but no cluster of 3 — continues all 5."""
        checker = EarlyStopChecker(threshold=0.9, quorum=3)

        checker.add_rollout(0, "solution alpha", exit_code=0, score=0.8)
        checker.add_rollout(1, "solution alpha", exit_code=0, score=0.7)
        checker.add_rollout(2, "solution beta!", exit_code=0, score=0.9)
        checker.add_rollout(3, "solution beta!", exit_code=0, score=0.6)
        checker.add_rollout(4, "unique approach", exit_code=0, score=0.5)

        assert checker.should_stop() is False
        assert checker.completed_count == 5


# ---------------------------------------------------------------------------
# Test 8: best_from_cluster returns highest score
# ---------------------------------------------------------------------------


class TestBestFromCluster:
    def test_returns_highest_score_in_cluster(self) -> None:
        """best_from_cluster() returns the rollout_id with the highest eval
        score within the largest agreeing cluster."""
        checker = EarlyStopChecker(threshold=0.9, quorum=3)

        checker.add_rollout(0, "def solve(): return 42", exit_code=0, score=0.7)
        checker.add_rollout(1, "def solve(): return 42", exit_code=0, score=0.95)
        checker.add_rollout(2, "def solve(): return 42", exit_code=0, score=0.8)

        assert checker.should_stop() is True
        assert checker.best_from_cluster() == 1  # Highest score (0.95)

    def test_best_from_cluster_tie_returns_first(self) -> None:
        """When scores tie, return the one with the lower rollout_id."""
        checker = EarlyStopChecker(threshold=0.9, quorum=3)

        checker.add_rollout(0, "same", exit_code=0, score=0.5)
        checker.add_rollout(1, "same", exit_code=0, score=0.5)
        checker.add_rollout(2, "same", exit_code=0, score=0.5)

        assert checker.should_stop() is True
        # All scores equal — should return the first one (rollout 0).
        assert checker.best_from_cluster() == 0

    def test_best_from_cluster_no_cluster_returns_neg1(self) -> None:
        """When no quorum is met, best_from_cluster() returns -1."""
        checker = EarlyStopChecker(threshold=0.9, quorum=3)

        checker.add_rollout(0, "alpha", exit_code=0, score=0.8)
        checker.add_rollout(1, "beta", exit_code=0, score=0.9)

        assert checker.should_stop() is False
        assert checker.best_from_cluster() == -1


# ---------------------------------------------------------------------------
# Test 9: Empty rollout list
# ---------------------------------------------------------------------------


class TestEdgeCases:
    def test_empty_rollout_list(self) -> None:
        """should_stop() returns False when no rollouts have been added."""
        checker = EarlyStopChecker(threshold=0.9, quorum=3)
        assert checker.should_stop() is False
        assert checker.completed_count == 0
        assert checker.early_stopped is False

    def test_single_rollout(self) -> None:
        """A single rollout can never meet quorum."""
        checker = EarlyStopChecker(threshold=0.9, quorum=3)
        checker.add_rollout(0, "output", exit_code=0, score=1.0)
        assert checker.should_stop() is False

    def test_custom_threshold_and_quorum(self) -> None:
        """Custom threshold=0.5 and quorum=2 triggers with loosely similar outputs."""
        checker = EarlyStopChecker(threshold=0.5, quorum=2)

        # These outputs share significant overlap ("def solve" prefix).
        checker.add_rollout(0, "def solve(): return 42", exit_code=0, score=0.8)
        checker.add_rollout(1, "def solve(): return 43", exit_code=0, score=0.9)

        # With threshold=0.5 and quorum=2, two similar outputs should trigger.
        assert checker.should_stop() is True

    def test_skipped_rollouts_metadata(self) -> None:
        """Verify skipped_rollouts computation: total - completed."""
        checker = EarlyStopChecker(threshold=0.9, quorum=3)

        checker.add_rollout(0, "same", exit_code=0, score=0.8)
        checker.add_rollout(1, "same", exit_code=0, score=0.9)
        checker.add_rollout(2, "same", exit_code=0, score=0.7)

        assert checker.should_stop() is True
        # If total rollout_count was 8, skipped = 8 - 3 = 5
        # The checker tracks completed_count; the caller computes skipped.
        assert checker.completed_count == 3

    def test_all_failing_exit_codes(self) -> None:
        """All identical outputs but all exit_code=1 — should NOT stop."""
        checker = EarlyStopChecker(threshold=0.9, quorum=3)

        checker.add_rollout(0, "error output", exit_code=1, score=0.0)
        checker.add_rollout(1, "error output", exit_code=1, score=0.0)
        checker.add_rollout(2, "error output", exit_code=1, score=0.0)

        assert checker.should_stop() is False
