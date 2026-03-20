"""Data models for the intelligent routing system."""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import StrEnum


class ComplexityTier(StrEnum):
    SIMPLE = "simple"
    MEDIUM = "medium"
    COMPLEX = "complex"
    REASONING = "reasoning"


class TaskType(StrEnum):
    CODE = "code"
    REVIEW = "review"
    PLAN = "plan"
    QA = "qa"
    CHAT = "chat"
    DEBUG = "debug"
    REFACTOR = "refactor"


class RoutingProfile(StrEnum):
    COST_FIRST = "cost_first"
    BALANCED = "balanced"
    QUALITY_FIRST = "quality_first"


PROFILE_WEIGHTS: dict[RoutingProfile, dict[str, float]] = {
    RoutingProfile.COST_FIRST: {"cost": 0.6, "quality": 0.25, "latency": 0.15},
    RoutingProfile.BALANCED: {"cost": 0.3, "quality": 0.5, "latency": 0.2},
    RoutingProfile.QUALITY_FIRST: {"cost": 0.1, "quality": 0.7, "latency": 0.2},
}


@dataclass(frozen=True)
class PromptAnalysis:
    complexity_tier: ComplexityTier
    task_type: TaskType
    dimensions: dict[str, float]
    confidence: float
    estimated_output_tokens: int = 0


@dataclass(frozen=True)
class RoutingDecision:
    model: str
    routing_layer: str
    complexity_tier: ComplexityTier
    task_type: TaskType
    confidence: float = 1.0
    reasoning: str = ""
    estimated_cost_per_1m: float = 0.0
    fallback_model: str = ""
    recommended_max_tokens: int | None = None


@dataclass(frozen=True)
class RoutingMetadata:
    """Transparent routing metadata for observability (C1)."""

    complexity_tier: ComplexityTier
    selected_model: str
    reason: str
    mab_score: float = 0.0
    alternatives: tuple[dict[str, object], ...] = field(default_factory=tuple)


@dataclass(frozen=True)
class RoutingPlan:
    primary: RoutingDecision
    fallbacks: tuple[str, ...] = field(default_factory=tuple)


@dataclass(frozen=True)
class ModelStats:
    model_name: str
    trial_count: int = 0
    avg_reward: float = 0.0
    avg_cost_usd: float = 0.0
    avg_latency_ms: int = 0
    avg_quality: float = 0.0
    input_cost_per: float = 0.0
    supports_tools: bool = False
    supports_vision: bool = False
    max_context: int = 0


@dataclass(frozen=True)
class CascadeStep:
    model: str
    confidence_threshold: float = 0.7
    max_tokens: int | None = None


@dataclass(frozen=True)
class CascadePlan:
    steps: list[CascadeStep] = field(default_factory=list)


@dataclass
class RoutingConfig:
    enabled: bool = True
    complexity_enabled: bool = True
    mab_enabled: bool = True
    llm_meta_enabled: bool = True
    mab_min_trials: int = 10
    mab_exploration_rate: float = 1.414
    cost_weight: float = 0.3
    quality_weight: float = 0.5
    latency_weight: float = 0.2
    meta_router_model: str = "groq/llama-3.1-8b-instant"
    stats_refresh_interval: str = "5m"
    diversity_mode: bool = False
    entropy_weight: float = 0.1
    mab_cost_penalty: float = 0.0
    cost_penalty_mode: str = "linear"
    max_cost_ceiling: float = 0.10
    max_latency_ceiling: int = 30_000
    cascade_enabled: bool = False
    cascade_confidence_threshold: float = 0.7
    cascade_max_steps: int = 3

    # Complexity analyzer weights (dimension -> weight, must sum to ~1.0)
    complexity_weights: dict[str, float] = field(
        default_factory=lambda: {
            "code_presence": 0.20,
            "reasoning_markers": 0.20,
            "technical_terms": 0.15,
            "prompt_length": 0.10,
            "multi_step": 0.15,
            "context_requirements": 0.10,
            "output_complexity": 0.10,
        }
    )

    # Tier thresholds: (min_score, tier) evaluated top-down
    tier_thresholds: list[tuple[float, str]] = field(
        default_factory=lambda: [
            (0.75, "reasoning"),
            (0.50, "complex"),
            (0.25, "medium"),
            (0.0, "simple"),
        ]
    )

    # Task-type score boost
    task_type_boost: dict[str, float] = field(
        default_factory=lambda: {
            "chat": 0.0,
            "code": 0.10,
            "debug": 0.20,
            "qa": 0.15,
            "refactor": 0.20,
            "review": 0.25,
            "plan": 0.25,
        }
    )
