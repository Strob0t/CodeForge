"""Layer 2: Multi-Armed Bandit model selector using UCB1.

Selects the best model for a given task_type/complexity_tier combination by
balancing exploitation (models with high average reward) with exploration
(under-tested models). Uses statistics from model_performance_stats table.
"""

from __future__ import annotations

import math
from datetime import UTC, datetime, timedelta
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Callable

    from codeforge.routing.models import ComplexityTier, ModelStats, RoutingConfig, RoutingProfile, TaskType


class MABModelSelector:
    """UCB1-based model selector.

    Fetches model performance stats via a provided async loader function,
    caches them in memory, and selects models using the UCB1 algorithm.
    """

    def __init__(
        self,
        stats_loader: Callable[[str, str], list[ModelStats]],
        config: RoutingConfig,
    ) -> None:
        self._load_stats = stats_loader
        self._config = config
        self._cache: dict[tuple[str, str], list[ModelStats]] = {}
        self._cache_expiry: datetime = datetime.min.replace(tzinfo=UTC)
        self._refresh_interval = _parse_interval(config.stats_refresh_interval)

    def select(
        self,
        task_type: TaskType,
        complexity_tier: ComplexityTier,
        available_models: list[str],
        max_cost: float | None = None,
        profile: RoutingProfile | None = None,
    ) -> str | None:
        """Select a model using UCB1. Returns None if insufficient data (cold start)."""
        if not available_models:
            return None

        stats = self._get_stats(str(task_type), str(complexity_tier))

        # Filter to models that are both available and have stats.
        candidates: list[ModelStats] = []
        for s in stats:
            if s.model_name not in available_models:
                continue
            if max_cost is not None and s.input_cost_per > max_cost:
                continue
            candidates.append(s)

        if not candidates:
            return None

        # If ALL candidates have trial_count < min_trials -> cold start.
        if all(c.trial_count < self._config.mab_min_trials for c in candidates):
            return None

        total_trials = sum(c.trial_count for c in candidates)
        if total_trials == 0:
            return None

        effective_penalty = self._effective_cost_penalty(profile)
        max_pool_cost = max((c.avg_cost_usd for c in candidates), default=0.0)

        best_model: str = ""
        best_score: float = -math.inf

        for c in candidates:
            score = self._ucb1_score(c, total_trials, max_pool_cost=max_pool_cost, effective_penalty=effective_penalty)
            # Deterministic tiebreak by model name for stability.
            if score > best_score or (score == best_score and c.model_name < best_model):
                best_score = score
                best_model = c.model_name

        return best_model or None

    def select_diverse(
        self,
        task_type: TaskType,
        complexity_tier: ComplexityTier,
        available_models: list[str],
        n: int = 1,
        max_cost: float | None = None,
        profile: RoutingProfile | None = None,
    ) -> list[str]:
        """Select N diverse models using entropy-aware UCB1.

        After each selection the local selection count is incremented so
        the entropy bonus penalises re-selecting the same model.  When
        ``diversity_mode`` is off the result is the standard UCB1 pick
        repeated N times.
        """
        if not available_models:
            return []

        stats = self._get_stats(str(task_type), str(complexity_tier))

        candidates: list[ModelStats] = []
        for s in stats:
            if s.model_name not in available_models:
                continue
            if max_cost is not None and s.input_cost_per > max_cost:
                continue
            candidates.append(s)

        if not candidates:
            return []

        # Cold start check.
        if all(c.trial_count < self._config.mab_min_trials for c in candidates):
            return []

        total_trials = sum(c.trial_count for c in candidates)
        if total_trials == 0:
            return []

        effective_penalty = self._effective_cost_penalty(profile)
        max_pool_cost = max((c.avg_cost_usd for c in candidates), default=0.0)

        selected: list[str] = []
        local_counts: dict[str, int] = {}

        for _ in range(n):
            best_model = ""
            best_score = -math.inf

            for c in candidates:
                score = self._entropy_ucb1_score(
                    c, total_trials, local_counts, max_pool_cost=max_pool_cost, effective_penalty=effective_penalty
                )
                if score > best_score or (score == best_score and c.model_name < best_model):
                    best_score = score
                    best_model = c.model_name

            if best_model:
                selected.append(best_model)
                local_counts[best_model] = local_counts.get(best_model, 0) + 1

        return selected

    def score(
        self,
        model: str,
        task_type: TaskType | str,
        complexity_tier: ComplexityTier | str,
    ) -> float | None:
        """Return the UCB1 score for a single model, or None if no stats."""
        stats_list = self._get_stats(str(task_type), str(complexity_tier))
        total_trials = sum(s.trial_count for s in stats_list)
        if total_trials == 0:
            return None
        for s in stats_list:
            if s.model_name == model:
                return self._ucb1_score(s, total_trials)
        return None

    def invalidate_cache(self) -> None:
        """Force cache refresh on next select."""
        self._cache.clear()
        self._cache_expiry = datetime.min.replace(tzinfo=UTC)

    def _get_stats(self, task_type: str, tier: str) -> list[ModelStats]:
        """Load stats, using cache if still valid."""
        now = datetime.now(tz=UTC)

        if now < self._cache_expiry and (task_type, tier) in self._cache:
            return self._cache[(task_type, tier)]

        stats = self._load_stats(task_type, tier)
        self._cache[(task_type, tier)] = stats
        self._cache_expiry = now + self._refresh_interval
        return stats

    def _ucb1_score(
        self,
        stats: ModelStats,
        total_trials: int,
        max_pool_cost: float = 0.0,
        effective_penalty: float = 0.0,
    ) -> float:
        """Compute UCB1 score: avg_reward + exploration_rate * sqrt(ln(total) / trials).

        When *effective_penalty* > 0, a cost-ratio penalty is applied:
        ``score *= (1.0 - cost_ratio * effective_penalty)`` where
        ``cost_ratio = stats.avg_cost_usd / max_pool_cost``.
        """
        if stats.trial_count < self._config.mab_min_trials:
            return math.inf  # Exploration bonus for under-tested models.

        if stats.trial_count == 0:
            return math.inf

        exploration = self._config.mab_exploration_rate * math.sqrt(math.log(total_trials) / stats.trial_count)
        score = stats.avg_reward + exploration

        if effective_penalty > 0 and stats.avg_cost_usd > 0 and max_pool_cost > 0:
            cost_ratio = stats.avg_cost_usd / max_pool_cost
            score *= 1.0 - cost_ratio * effective_penalty

        return score

    def _entropy_ucb1_score(
        self,
        stats: ModelStats,
        total_trials: int,
        selection_counts: dict[str, int],
        max_pool_cost: float = 0.0,
        effective_penalty: float = 0.0,
    ) -> float:
        """UCB1 with entropy regularisation (Phase 28D).

        Adds ``entropy_weight * entropy_bonus`` to the standard UCB1 score.
        """
        base = self._ucb1_score(stats, total_trials, max_pool_cost=max_pool_cost, effective_penalty=effective_penalty)

        if not self._config.diversity_mode or self._config.entropy_weight == 0.0:
            return base

        # Under-tested models already have infinity bonus.
        if base == math.inf:
            return base

        total_selections = sum(selection_counts.values()) or 0
        model_selections = selection_counts.get(stats.model_name, 0)

        if total_selections == 0 or model_selections == 0:
            entropy_bonus = math.log(total_selections + 2)
        else:
            p_i = model_selections / total_selections
            entropy_bonus = -math.log(p_i)

        return base + self._config.entropy_weight * entropy_bonus

    def _effective_cost_penalty(self, profile: RoutingProfile | None) -> float:
        """Compute the effective cost penalty based on the routing profile.

        - ``COST_FIRST``:    max(config.mab_cost_penalty, 0.4)
        - ``BALANCED``:      config.mab_cost_penalty
        - ``QUALITY_FIRST``: 0.0
        - ``None``:          config.mab_cost_penalty
        """
        if profile is None:
            return self._config.mab_cost_penalty

        profile_str = str(profile)
        if profile_str == "cost_first":
            return max(self._config.mab_cost_penalty, 0.4)
        if profile_str == "quality_first":
            return 0.0
        return self._config.mab_cost_penalty


def _parse_interval(interval: str) -> timedelta:
    """Parse a simple interval string like '5m', '1h', '30s'."""
    if not interval:
        return timedelta(minutes=5)

    unit = interval[-1]
    try:
        value = int(interval[:-1])
    except ValueError:
        return timedelta(minutes=5)

    if unit == "s":
        return timedelta(seconds=value)
    if unit == "m":
        return timedelta(minutes=value)
    if unit == "h":
        return timedelta(hours=value)

    return timedelta(minutes=5)
