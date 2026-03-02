"""Tests for Layer 2: MABModelSelector / UCB1 (Phase 26G)."""

from __future__ import annotations

import math
from datetime import timedelta

from codeforge.routing.mab import MABModelSelector, _parse_interval
from codeforge.routing.models import ComplexityTier, ModelStats, RoutingConfig, TaskType


def _make_stats(
    model: str,
    trials: int = 100,
    avg_reward: float = 0.5,
    input_cost: float = 0.0,
) -> ModelStats:
    return ModelStats(
        model_name=model,
        trial_count=trials,
        avg_reward=avg_reward,
        input_cost_per=input_cost,
    )


def _make_selector(
    stats: list[ModelStats],
    min_trials: int = 10,
    exploration_rate: float = 1.414,
) -> MABModelSelector:
    def loader(task_type: str, tier: str) -> list[ModelStats]:
        return stats

    config = RoutingConfig(
        mab_min_trials=min_trials,
        mab_exploration_rate=exploration_rate,
        stats_refresh_interval="5m",
    )
    return MABModelSelector(stats_loader=loader, config=config)


# -- UCB1 score correctness --------------------------------------------------


def test_ucb1_score_known_values() -> None:
    selector = _make_selector([])
    stats = _make_stats("m1", trials=100, avg_reward=0.8)
    score = selector._ucb1_score(stats, total_trials=1000)
    # UCB1 = 0.8 + 1.414 * sqrt(ln(1000) / 100)
    exploration = 1.414 * math.sqrt(math.log(1000) / 100)
    expected = 0.8 + exploration
    assert math.isclose(score, expected, abs_tol=1e-6)


def test_ucb1_undertested_gets_infinity() -> None:
    selector = _make_selector([], min_trials=10)
    stats = _make_stats("m1", trials=5, avg_reward=0.9)
    score = selector._ucb1_score(stats, total_trials=100)
    assert score == math.inf


def test_ucb1_zero_trials_gets_infinity() -> None:
    selector = _make_selector([], min_trials=10)
    stats = _make_stats("m1", trials=0, avg_reward=0.0)
    score = selector._ucb1_score(stats, total_trials=100)
    assert score == math.inf


# -- Cold start --------------------------------------------------------------


def test_cold_start_all_under_min_trials() -> None:
    stats = [
        _make_stats("m1", trials=3),
        _make_stats("m2", trials=5),
    ]
    selector = _make_selector(stats, min_trials=10)
    result = selector.select(TaskType.CODE, ComplexityTier.SIMPLE, ["m1", "m2"])
    assert result is None


# -- Exploration vs exploitation ---------------------------------------------


def test_exploration_untested_beats_good_model() -> None:
    stats = [
        _make_stats("m1", trials=100, avg_reward=0.9),
        _make_stats("m2", trials=3, avg_reward=0.1),  # Under min_trials
    ]
    selector = _make_selector(stats, min_trials=10)
    result = selector.select(TaskType.CODE, ComplexityTier.SIMPLE, ["m1", "m2"])
    # m2 gets infinity exploration bonus → selected.
    assert result == "m2"


def test_exploitation_high_reward_wins() -> None:
    stats = [
        _make_stats("m1", trials=100, avg_reward=0.9),
        _make_stats("m2", trials=100, avg_reward=0.5),
    ]
    selector = _make_selector(stats, min_trials=10)
    result = selector.select(TaskType.CODE, ComplexityTier.SIMPLE, ["m1", "m2"])
    assert result == "m1"


# -- Cost constraint ---------------------------------------------------------


def test_cost_constraint_filters_expensive() -> None:
    stats = [
        _make_stats("cheap", trials=100, avg_reward=0.7, input_cost=0.001),
        _make_stats("expensive", trials=100, avg_reward=0.9, input_cost=0.05),
    ]
    selector = _make_selector(stats, min_trials=10)
    result = selector.select(TaskType.CODE, ComplexityTier.SIMPLE, ["cheap", "expensive"], max_cost=0.01)
    assert result == "cheap"


def test_all_models_filtered_by_cost() -> None:
    stats = [
        _make_stats("m1", trials=100, avg_reward=0.9, input_cost=0.05),
    ]
    selector = _make_selector(stats, min_trials=10)
    result = selector.select(TaskType.CODE, ComplexityTier.SIMPLE, ["m1"], max_cost=0.001)
    assert result is None


# -- Model availability -----------------------------------------------------


def test_only_available_models_considered() -> None:
    stats = [
        _make_stats("m1", trials=100, avg_reward=0.9),
        _make_stats("m2", trials=100, avg_reward=0.5),
    ]
    selector = _make_selector(stats, min_trials=10)
    result = selector.select(TaskType.CODE, ComplexityTier.SIMPLE, ["m2"])
    assert result == "m2"


def test_empty_available_models() -> None:
    stats = [_make_stats("m1", trials=100)]
    selector = _make_selector(stats, min_trials=10)
    result = selector.select(TaskType.CODE, ComplexityTier.SIMPLE, [])
    assert result is None


def test_no_stats_for_available_models() -> None:
    stats = [_make_stats("m1", trials=100)]
    selector = _make_selector(stats, min_trials=10)
    result = selector.select(TaskType.CODE, ComplexityTier.SIMPLE, ["unknown_model"])
    assert result is None


# -- Single model ------------------------------------------------------------


def test_single_model_sufficient_trials() -> None:
    stats = [_make_stats("only", trials=20, avg_reward=0.7)]
    selector = _make_selector(stats, min_trials=10)
    result = selector.select(TaskType.CODE, ComplexityTier.SIMPLE, ["only"])
    assert result == "only"


def test_single_model_insufficient_trials() -> None:
    stats = [_make_stats("only", trials=3, avg_reward=0.7)]
    selector = _make_selector(stats, min_trials=10)
    result = selector.select(TaskType.CODE, ComplexityTier.SIMPLE, ["only"])
    assert result is None


# -- Deterministic tiebreak --------------------------------------------------


def test_equal_reward_deterministic_tiebreak() -> None:
    stats = [
        _make_stats("b_model", trials=100, avg_reward=0.5),
        _make_stats("a_model", trials=100, avg_reward=0.5),
    ]
    selector = _make_selector(stats, min_trials=10)
    r1 = selector.select(TaskType.CODE, ComplexityTier.SIMPLE, ["a_model", "b_model"])
    r2 = selector.select(TaskType.CODE, ComplexityTier.SIMPLE, ["b_model", "a_model"])
    assert r1 == r2  # Deterministic regardless of input order.


# -- Cache behavior ----------------------------------------------------------


def test_cache_reused_within_interval() -> None:
    call_count = 0

    def counting_loader(task_type: str, tier: str) -> list[ModelStats]:
        nonlocal call_count
        call_count += 1
        return [_make_stats("m1", trials=20, avg_reward=0.7)]

    config = RoutingConfig(mab_min_trials=10, stats_refresh_interval="5m")
    selector = MABModelSelector(stats_loader=counting_loader, config=config)

    selector.select(TaskType.CODE, ComplexityTier.SIMPLE, ["m1"])
    selector.select(TaskType.CODE, ComplexityTier.SIMPLE, ["m1"])
    assert call_count == 1  # Second call should use cache.


def test_cache_invalidated_manually() -> None:
    call_count = 0

    def counting_loader(task_type: str, tier: str) -> list[ModelStats]:
        nonlocal call_count
        call_count += 1
        return [_make_stats("m1", trials=20, avg_reward=0.7)]

    config = RoutingConfig(mab_min_trials=10, stats_refresh_interval="5m")
    selector = MABModelSelector(stats_loader=counting_loader, config=config)

    selector.select(TaskType.CODE, ComplexityTier.SIMPLE, ["m1"])
    selector.invalidate_cache()
    selector.select(TaskType.CODE, ComplexityTier.SIMPLE, ["m1"])
    assert call_count == 2


# -- Mixed trials (some above, some below min) --------------------------------


def test_mixed_trials_exploration_for_untested() -> None:
    stats = [
        _make_stats("tested", trials=50, avg_reward=0.6),
        _make_stats("untested", trials=2, avg_reward=0.0),
    ]
    selector = _make_selector(stats, min_trials=10)
    result = selector.select(TaskType.CODE, ComplexityTier.SIMPLE, ["tested", "untested"])
    # untested gets inf exploration bonus.
    assert result == "untested"


# -- _parse_interval ---------------------------------------------------------


def test_parse_interval_minutes() -> None:
    assert _parse_interval("5m") == timedelta(minutes=5)


def test_parse_interval_seconds() -> None:
    assert _parse_interval("30s") == timedelta(seconds=30)


def test_parse_interval_hours() -> None:
    assert _parse_interval("1h") == timedelta(hours=1)


def test_parse_interval_invalid() -> None:
    assert _parse_interval("invalid") == timedelta(minutes=5)


def test_parse_interval_empty() -> None:
    assert _parse_interval("") == timedelta(minutes=5)
