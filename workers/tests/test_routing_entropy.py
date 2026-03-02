"""Tests for diversity-aware MAB routing (entropy-enhanced UCB1).

Tests cover: backward compat (diversity_mode=False), entropy bonus effect,
select_diverse with N models, edge cases (empty, cold start, extreme weights).
"""

from __future__ import annotations

from codeforge.routing.mab import MABModelSelector
from codeforge.routing.models import ComplexityTier, ModelStats, RoutingConfig, TaskType


def _config(
    *,
    diversity_mode: bool = False,
    entropy_weight: float = 0.1,
    min_trials: int = 5,
    exploration_rate: float = 1.414,
) -> RoutingConfig:
    return RoutingConfig(
        enabled=True,
        mab_enabled=True,
        mab_min_trials=min_trials,
        mab_exploration_rate=exploration_rate,
        diversity_mode=diversity_mode,
        entropy_weight=entropy_weight,
    )


def _stats(
    name: str,
    trial_count: int = 20,
    avg_reward: float = 0.5,
    input_cost_per: float = 0.001,
) -> ModelStats:
    return ModelStats(
        model_name=name,
        trial_count=trial_count,
        avg_reward=avg_reward,
        input_cost_per=input_cost_per,
    )


def _loader(stats: list[ModelStats]):
    """Create a stats_loader function returning fixed stats."""

    def load(task_type: str, tier: str) -> list[ModelStats]:
        return stats

    return load


# ---------------------------------------------------------------------------
# Tests: Backward compatibility (diversity_mode=False)
# ---------------------------------------------------------------------------


class TestBackwardCompat:
    def test_standard_ucb1_unchanged(self) -> None:
        """diversity_mode=False → select() behaves exactly like standard UCB1."""
        stats = [_stats("gpt-4o", avg_reward=0.8), _stats("claude", avg_reward=0.6)]
        mab = MABModelSelector(_loader(stats), _config(diversity_mode=False))

        result = mab.select(TaskType.CODE, ComplexityTier.COMPLEX, ["gpt-4o", "claude"])

        assert result == "gpt-4o"  # Higher reward wins

    def test_select_diverse_n1_same_as_select(self) -> None:
        """select_diverse(n=1) returns same model as select()."""
        stats = [_stats("gpt-4o", avg_reward=0.8), _stats("claude", avg_reward=0.6)]
        mab = MABModelSelector(_loader(stats), _config(diversity_mode=False))

        single = mab.select(TaskType.CODE, ComplexityTier.COMPLEX, ["gpt-4o", "claude"])
        diverse = mab.select_diverse(TaskType.CODE, ComplexityTier.COMPLEX, ["gpt-4o", "claude"], n=1)

        assert diverse == [single]


# ---------------------------------------------------------------------------
# Tests: Entropy-aware selection
# ---------------------------------------------------------------------------


class TestEntropyAwareSelection:
    def test_entropy_mode_distributes_selections(self) -> None:
        """diversity_mode=True with 3 equal models → select_diverse returns 3 different models."""
        stats = [
            _stats("model_a", avg_reward=0.5),
            _stats("model_b", avg_reward=0.5),
            _stats("model_c", avg_reward=0.5),
        ]
        mab = MABModelSelector(_loader(stats), _config(diversity_mode=True, entropy_weight=1.0))

        selected = mab.select_diverse(
            TaskType.CODE,
            ComplexityTier.COMPLEX,
            ["model_a", "model_b", "model_c"],
            n=3,
        )

        assert len(selected) == 3
        assert len(set(selected)) == 3  # All different

    def test_entropy_weight_zero_collapses_to_standard(self) -> None:
        """entropy_weight=0.0 → same as standard UCB1 (always picks best)."""
        stats = [_stats("best", avg_reward=0.9), _stats("worst", avg_reward=0.1)]
        mab = MABModelSelector(_loader(stats), _config(diversity_mode=True, entropy_weight=0.0))

        selected = mab.select_diverse(TaskType.CODE, ComplexityTier.COMPLEX, ["best", "worst"], n=3)

        # Without entropy, always picks "best"
        assert all(m == "best" for m in selected)

    def test_high_entropy_favors_underexplored(self) -> None:
        """High entropy_weight → previously unselected models get priority."""
        stats = [
            _stats("popular", avg_reward=0.9, trial_count=100),
            _stats("rare", avg_reward=0.3, trial_count=20),
        ]
        mab = MABModelSelector(_loader(stats), _config(diversity_mode=True, entropy_weight=5.0))

        # First selection: "popular" might win on reward
        # Second selection: entropy penalty on "popular" should boost "rare"
        selected = mab.select_diverse(TaskType.CODE, ComplexityTier.COMPLEX, ["popular", "rare"], n=2)

        assert len(selected) == 2
        assert "rare" in selected  # Entropy should force diversity

    def test_select_diverse_more_than_available(self) -> None:
        """Requesting more models than available → reuses models."""
        stats = [_stats("only_a", avg_reward=0.5), _stats("only_b", avg_reward=0.5)]
        mab = MABModelSelector(_loader(stats), _config(diversity_mode=True, entropy_weight=0.5))

        selected = mab.select_diverse(TaskType.CODE, ComplexityTier.COMPLEX, ["only_a", "only_b"], n=5)

        assert len(selected) == 5
        # Should alternate between available models due to entropy
        assert set(selected) == {"only_a", "only_b"}

    def test_select_diverse_empty_models(self) -> None:
        """No available models → empty list."""
        mab = MABModelSelector(_loader([]), _config(diversity_mode=True))

        selected = mab.select_diverse(TaskType.CODE, ComplexityTier.SIMPLE, [], n=3)

        assert selected == []

    def test_select_diverse_cold_start(self) -> None:
        """All models under min_trials → returns empty list (cold start)."""
        stats = [_stats("a", trial_count=2), _stats("b", trial_count=1)]
        mab = MABModelSelector(_loader(stats), _config(diversity_mode=True, min_trials=10))

        selected = mab.select_diverse(TaskType.CODE, ComplexityTier.COMPLEX, ["a", "b"], n=2)

        assert selected == []

    def test_entropy_bonus_is_symmetric(self) -> None:
        """Two models with equal stats and no prior selections → deterministic tiebreak."""
        stats = [_stats("alpha", avg_reward=0.5), _stats("beta", avg_reward=0.5)]
        mab = MABModelSelector(_loader(stats), _config(diversity_mode=True, entropy_weight=0.5))

        selected = mab.select_diverse(TaskType.CODE, ComplexityTier.COMPLEX, ["alpha", "beta"], n=2)

        assert len(selected) == 2
        assert set(selected) == {"alpha", "beta"}
