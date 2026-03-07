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
class CascadeConfig:
    enabled: bool = False
    confidence_threshold: float = 0.7
    max_steps: int = 3
    strategy: str = "cheap_first"


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
    enabled: bool = False
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
