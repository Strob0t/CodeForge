"""Tests for routing data models (Phase 26E)."""

from __future__ import annotations

import pytest

from codeforge.routing.models import (
    ComplexityTier,
    ModelStats,
    PromptAnalysis,
    RoutingConfig,
    RoutingDecision,
    TaskType,
)

# -- ComplexityTier ----------------------------------------------------------


def test_complexity_tier_values() -> None:
    assert ComplexityTier.SIMPLE == "simple"
    assert ComplexityTier.MEDIUM == "medium"
    assert ComplexityTier.COMPLEX == "complex"
    assert ComplexityTier.REASONING == "reasoning"


def test_complexity_tier_count() -> None:
    assert len(ComplexityTier) == 4


def test_complexity_tier_is_str() -> None:
    assert isinstance(ComplexityTier.SIMPLE, str)


# -- TaskType ----------------------------------------------------------------


def test_task_type_values() -> None:
    assert TaskType.CODE == "code"
    assert TaskType.REVIEW == "review"
    assert TaskType.PLAN == "plan"
    assert TaskType.QA == "qa"
    assert TaskType.CHAT == "chat"
    assert TaskType.DEBUG == "debug"
    assert TaskType.REFACTOR == "refactor"


def test_task_type_count() -> None:
    assert len(TaskType) == 7


def test_task_type_is_str() -> None:
    assert isinstance(TaskType.CODE, str)


# -- PromptAnalysis ----------------------------------------------------------


def test_prompt_analysis_creation() -> None:
    dims = {"code_presence": 0.5, "reasoning_markers": 0.3}
    pa = PromptAnalysis(
        complexity_tier=ComplexityTier.MEDIUM,
        task_type=TaskType.CODE,
        dimensions=dims,
        confidence=0.7,
    )
    assert pa.complexity_tier == ComplexityTier.MEDIUM
    assert pa.task_type == TaskType.CODE
    assert pa.dimensions == dims
    assert pa.confidence == 0.7


def test_prompt_analysis_frozen() -> None:
    pa = PromptAnalysis(
        complexity_tier=ComplexityTier.SIMPLE,
        task_type=TaskType.CHAT,
        dimensions={},
        confidence=0.5,
    )
    with pytest.raises(AttributeError):
        pa.confidence = 0.9  # type: ignore[misc]


# -- RoutingDecision ---------------------------------------------------------


def test_routing_decision_defaults() -> None:
    rd = RoutingDecision(
        model="openai/gpt-4o",
        routing_layer="mab",
        complexity_tier=ComplexityTier.COMPLEX,
        task_type=TaskType.CODE,
    )
    assert rd.confidence == 1.0
    assert rd.reasoning == ""
    assert rd.estimated_cost_per_1m == 0.0
    assert rd.fallback_model == ""


def test_routing_decision_all_fields() -> None:
    rd = RoutingDecision(
        model="anthropic/claude-sonnet-4",
        routing_layer="complexity",
        complexity_tier=ComplexityTier.REASONING,
        task_type=TaskType.PLAN,
        confidence=0.95,
        reasoning="High reasoning demand",
        estimated_cost_per_1m=3.0,
        fallback_model="openai/gpt-4o",
    )
    assert rd.model == "anthropic/claude-sonnet-4"
    assert rd.reasoning == "High reasoning demand"
    assert rd.fallback_model == "openai/gpt-4o"


def test_routing_decision_frozen() -> None:
    rd = RoutingDecision(
        model="x",
        routing_layer="mab",
        complexity_tier=ComplexityTier.SIMPLE,
        task_type=TaskType.CHAT,
    )
    with pytest.raises(AttributeError):
        rd.model = "y"  # type: ignore[misc]


# -- ModelStats --------------------------------------------------------------


def test_model_stats_defaults() -> None:
    ms = ModelStats(model_name="openai/gpt-4o")
    assert ms.trial_count == 0
    assert ms.avg_reward == 0.0
    assert ms.avg_cost_usd == 0.0
    assert ms.avg_latency_ms == 0
    assert ms.avg_quality == 0.0
    assert ms.input_cost_per == 0.0
    assert ms.supports_tools is False
    assert ms.supports_vision is False
    assert ms.max_context == 0


def test_model_stats_all_fields() -> None:
    ms = ModelStats(
        model_name="anthropic/claude-opus-4.6",
        trial_count=100,
        avg_reward=0.85,
        avg_cost_usd=0.05,
        avg_latency_ms=3000,
        avg_quality=0.92,
        input_cost_per=0.015,
        supports_tools=True,
        supports_vision=True,
        max_context=200000,
    )
    assert ms.model_name == "anthropic/claude-opus-4.6"
    assert ms.trial_count == 100
    assert ms.supports_tools is True
    assert ms.max_context == 200000


# -- RoutingConfig -----------------------------------------------------------


def test_routing_config_defaults() -> None:
    rc = RoutingConfig()
    assert rc.enabled is False
    assert rc.complexity_enabled is True
    assert rc.mab_enabled is True
    assert rc.llm_meta_enabled is True
    assert rc.mab_min_trials == 10
    assert rc.mab_exploration_rate == 1.414
    assert rc.cost_weight == 0.3
    assert rc.quality_weight == 0.5
    assert rc.latency_weight == 0.2
    assert rc.meta_router_model == "groq/llama-3.1-8b-instant"
    assert rc.stats_refresh_interval == "5m"


def test_routing_config_weights_sum_to_one() -> None:
    rc = RoutingConfig()
    total = rc.cost_weight + rc.quality_weight + rc.latency_weight
    assert abs(total - 1.0) < 1e-9


def test_routing_config_mutable() -> None:
    rc = RoutingConfig()
    rc.enabled = True
    rc.mab_min_trials = 20
    assert rc.enabled is True
    assert rc.mab_min_trials == 20


def test_routing_config_custom_values() -> None:
    rc = RoutingConfig(
        enabled=True,
        mab_exploration_rate=2.0,
        cost_weight=0.4,
        quality_weight=0.4,
        latency_weight=0.2,
        meta_router_model="openai/gpt-4o-mini",
    )
    assert rc.enabled is True
    assert rc.mab_exploration_rate == 2.0
    assert rc.meta_router_model == "openai/gpt-4o-mini"
