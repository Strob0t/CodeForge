"""Data models for the intelligent routing system."""

from __future__ import annotations

from dataclasses import dataclass
from enum import StrEnum


class ComplexityTier(StrEnum):
    """Prompt complexity tier determined by Layer 1 analysis."""

    SIMPLE = "simple"
    MEDIUM = "medium"
    COMPLEX = "complex"
    REASONING = "reasoning"


class TaskType(StrEnum):
    """Type of work a prompt requests."""

    CODE = "code"
    REVIEW = "review"
    PLAN = "plan"
    QA = "qa"
    CHAT = "chat"
    DEBUG = "debug"
    REFACTOR = "refactor"


@dataclass(frozen=True)
class PromptAnalysis:
    """Result from Layer 1 complexity analysis."""

    complexity_tier: ComplexityTier
    task_type: TaskType
    dimensions: dict[str, float]
    confidence: float


@dataclass(frozen=True)
class RoutingDecision:
    """Final routing recommendation from the HybridRouter."""

    model: str
    routing_layer: str
    complexity_tier: ComplexityTier
    task_type: TaskType
    confidence: float = 1.0
    reasoning: str = ""
    estimated_cost_per_1m: float = 0.0
    fallback_model: str = ""


@dataclass(frozen=True)
class ModelStats:
    """Aggregated performance stats for a model/task_type/tier combination (MAB state)."""

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


@dataclass
class RoutingConfig:
    """Configuration for the intelligent routing system."""

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
    # Phase 28D: Entropy-aware diversity routing (R2E-Gym/EntroPO).
    diversity_mode: bool = False
    entropy_weight: float = 0.1
