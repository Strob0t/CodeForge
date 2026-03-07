"""Reward signal computation for the Multi-Armed Bandit model selector.

The reward function combines quality, cost, and latency into a single scalar
that the UCB1 algorithm uses to rank models.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from codeforge.routing.models import RoutingConfig

# Failure penalty — applied when the LLM call errors out.
_FAILURE_REWARD: float = -0.5

# Normalization ceilings (fallback defaults).
_MAX_COST_USD: float = 0.10  # Per-call cost ceiling
_MAX_LATENCY_MS: int = 30_000  # 30 seconds


def compute_reward(
    success: bool,
    quality_score: float,
    cost_usd: float,
    latency_ms: int,
    config: RoutingConfig,
) -> float:
    """Compute a composite reward for a routing outcome.

    Formula (success):
        reward = quality_weight * quality
               - cost_weight * normalized_cost
               - latency_weight * normalized_latency

    Failure: reward = -0.5 regardless of other metrics.

    Normalization ceilings are read from *config* when positive, falling back
    to the module-level constants otherwise.  When ``config.cost_penalty_mode``
    is ``"quadratic"``, the normalized cost is squared before weighting.
    """
    if not success:
        return _FAILURE_REWARD

    max_cost = config.max_cost_ceiling if config.max_cost_ceiling > 0 else _MAX_COST_USD
    max_latency = config.max_latency_ceiling if config.max_latency_ceiling > 0 else _MAX_LATENCY_MS

    norm_cost = min(1.0, cost_usd / max_cost) if max_cost > 0 else 0.0
    norm_latency = min(1.0, latency_ms / max_latency) if max_latency > 0 else 0.0

    if config.cost_penalty_mode == "quadratic":
        norm_cost = norm_cost**2

    return config.quality_weight * quality_score - config.cost_weight * norm_cost - config.latency_weight * norm_latency


def compute_quality_from_benchmark(scores: dict[str, float]) -> float:
    """Compute a normalized quality score (0.0-1.0) from benchmark metric scores.

    Averages all available metrics. Returns 0.0 if no scores are present.
    """
    relevant = {
        k: v for k, v in scores.items() if k in {"correctness", "tool_correctness", "faithfulness", "answer_relevancy"}
    }
    if not relevant:
        return 0.0
    return sum(relevant.values()) / len(relevant)
