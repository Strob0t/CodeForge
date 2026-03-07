"""Tests for reward computation (Phase 26I)."""

from __future__ import annotations

import math

from codeforge.routing.models import RoutingConfig
from codeforge.routing.reward import compute_quality_from_benchmark, compute_reward


def _cfg() -> RoutingConfig:
    """Default config for tests."""
    return RoutingConfig()


# -- compute_reward ----------------------------------------------------------


def test_perfect_quality_zero_cost_zero_latency() -> None:
    reward = compute_reward(
        success=True,
        quality_score=1.0,
        cost_usd=0.0,
        latency_ms=0,
        config=_cfg(),
    )
    # reward = 0.5 * 1.0 - 0.3 * 0.0 - 0.2 * 0.0 = 0.5
    assert math.isclose(reward, 0.5, abs_tol=1e-9)


def test_failure_returns_negative() -> None:
    reward = compute_reward(
        success=False,
        quality_score=1.0,
        cost_usd=0.0,
        latency_ms=0,
        config=_cfg(),
    )
    assert reward == -0.5


def test_failure_ignores_other_metrics() -> None:
    r1 = compute_reward(False, 1.0, 0.0, 0, _cfg())
    r2 = compute_reward(False, 0.0, 0.10, 30000, _cfg())
    assert r1 == r2 == -0.5


def test_high_cost_reduces_reward() -> None:
    reward = compute_reward(
        success=True,
        quality_score=1.0,
        cost_usd=0.10,
        latency_ms=0,
        config=_cfg(),
    )
    # reward = 0.5 * 1.0 - 0.3 * 1.0 - 0.2 * 0.0 = 0.2
    assert math.isclose(reward, 0.2, abs_tol=1e-9)


def test_high_latency_reduces_reward() -> None:
    reward = compute_reward(
        success=True,
        quality_score=1.0,
        cost_usd=0.0,
        latency_ms=30_000,
        config=_cfg(),
    )
    # reward = 0.5 * 1.0 - 0.3 * 0.0 - 0.2 * 1.0 = 0.3
    assert math.isclose(reward, 0.3, abs_tol=1e-9)


def test_zero_quality() -> None:
    reward = compute_reward(
        success=True,
        quality_score=0.0,
        cost_usd=0.05,
        latency_ms=15_000,
        config=_cfg(),
    )
    # reward = 0.5 * 0.0 - 0.3 * 0.5 - 0.2 * 0.5 = -0.25
    assert math.isclose(reward, -0.25, abs_tol=1e-9)


def test_cost_boundary_exactly_max() -> None:
    reward = compute_reward(
        success=True,
        quality_score=1.0,
        cost_usd=0.10,
        latency_ms=0,
        config=_cfg(),
    )
    assert math.isclose(reward, 0.2, abs_tol=1e-9)


def test_cost_above_max_capped() -> None:
    r1 = compute_reward(True, 1.0, 0.10, 0, _cfg())
    r2 = compute_reward(True, 1.0, 0.50, 0, _cfg())
    assert math.isclose(r1, r2, abs_tol=1e-9)


def test_latency_boundary_exactly_max() -> None:
    reward = compute_reward(
        success=True,
        quality_score=1.0,
        cost_usd=0.0,
        latency_ms=30_000,
        config=_cfg(),
    )
    assert math.isclose(reward, 0.3, abs_tol=1e-9)


def test_latency_above_max_capped() -> None:
    r1 = compute_reward(True, 1.0, 0.0, 30_000, _cfg())
    r2 = compute_reward(True, 1.0, 0.0, 100_000, _cfg())
    assert math.isclose(r1, r2, abs_tol=1e-9)


def test_custom_weights() -> None:
    cfg = RoutingConfig(cost_weight=0.1, quality_weight=0.8, latency_weight=0.1)
    reward = compute_reward(
        success=True,
        quality_score=1.0,
        cost_usd=0.10,
        latency_ms=30_000,
        config=cfg,
    )
    assert math.isclose(reward, 0.6, abs_tol=1e-9)


def test_partial_cost_and_latency() -> None:
    reward = compute_reward(
        success=True,
        quality_score=0.8,
        cost_usd=0.02,
        latency_ms=6000,
        config=_cfg(),
    )
    assert math.isclose(reward, 0.30, abs_tol=1e-9)


# -- compute_quality_from_benchmark ------------------------------------------


def test_quality_all_four_metrics() -> None:
    scores = {
        "correctness": 0.8,
        "tool_correctness": 0.9,
        "faithfulness": 0.7,
        "answer_relevancy": 1.0,
    }
    quality = compute_quality_from_benchmark(scores)
    assert math.isclose(quality, 0.85, abs_tol=1e-9)


def test_quality_two_metrics() -> None:
    scores = {"correctness": 0.8, "faithfulness": 0.6}
    quality = compute_quality_from_benchmark(scores)
    assert math.isclose(quality, 0.7, abs_tol=1e-9)


def test_quality_one_metric() -> None:
    scores = {"correctness": 0.9}
    quality = compute_quality_from_benchmark(scores)
    assert math.isclose(quality, 0.9, abs_tol=1e-9)


def test_quality_empty_dict() -> None:
    assert compute_quality_from_benchmark({}) == 0.0


def test_quality_irrelevant_keys_ignored() -> None:
    scores = {"some_custom_metric": 0.95, "another": 0.5}
    assert compute_quality_from_benchmark(scores) == 0.0


def test_quality_mixed_relevant_and_irrelevant() -> None:
    scores = {
        "correctness": 0.8,
        "custom_score": 0.99,
        "faithfulness": 0.6,
    }
    quality = compute_quality_from_benchmark(scores)
    assert math.isclose(quality, 0.7, abs_tol=1e-9)


# -- Phase R1.1: Configurable ceilings --------------------------------------


def test_reward_custom_cost_ceiling() -> None:
    """Custom cost ceiling changes normalization."""
    cfg = RoutingConfig(max_cost_ceiling=0.50)
    reward = compute_reward(True, 1.0, 0.10, 0, cfg)
    # norm_cost = 0.10 / 0.50 = 0.2
    # reward = 0.5*1.0 - 0.3*0.2 - 0.2*0.0 = 0.44
    assert math.isclose(reward, 0.44, abs_tol=1e-9)


def test_reward_custom_latency_ceiling() -> None:
    """Custom latency ceiling changes normalization."""
    cfg = RoutingConfig(max_latency_ceiling=60_000)
    reward = compute_reward(True, 1.0, 0.0, 30_000, cfg)
    # norm_latency = 30000 / 60000 = 0.5
    # reward = 0.5*1.0 - 0.3*0.0 - 0.2*0.5 = 0.40
    assert math.isclose(reward, 0.40, abs_tol=1e-9)


def test_reward_default_ceilings_unchanged() -> None:
    """Default config ceilings match original hardcoded values."""
    cfg = _cfg()
    assert cfg.max_cost_ceiling == 0.10
    assert cfg.max_latency_ceiling == 30_000
    reward = compute_reward(True, 1.0, 0.10, 30_000, cfg)
    assert math.isclose(reward, 0.0, abs_tol=1e-9)


# -- Phase R1.1: Quadratic cost penalty mode ---------------------------------


def test_reward_quadratic_cost_penalty_mode() -> None:
    """Quadratic mode softens mid-range cost penalty."""
    cfg = RoutingConfig(cost_penalty_mode="quadratic")
    reward = compute_reward(True, 1.0, 0.05, 0, cfg)
    # norm_cost = 0.05/0.10 = 0.5, quadratic -> 0.25
    # reward = 0.5*1.0 - 0.3*0.25 - 0.2*0.0 = 0.425
    assert math.isclose(reward, 0.425, abs_tol=1e-9)


def test_reward_quadratic_at_max_cost_same_as_linear() -> None:
    """At max cost, quadratic (1.0**2=1.0) equals linear."""
    cfg_lin = RoutingConfig(cost_penalty_mode="linear")
    cfg_quad = RoutingConfig(cost_penalty_mode="quadratic")
    r_lin = compute_reward(True, 1.0, 0.10, 0, cfg_lin)
    r_quad = compute_reward(True, 1.0, 0.10, 0, cfg_quad)
    assert math.isclose(r_lin, r_quad, abs_tol=1e-9)


def test_reward_quadratic_at_zero_cost_same_as_linear() -> None:
    """At zero cost, quadratic (0.0**2=0.0) equals linear."""
    cfg_lin = RoutingConfig(cost_penalty_mode="linear")
    cfg_quad = RoutingConfig(cost_penalty_mode="quadratic")
    r_lin = compute_reward(True, 1.0, 0.0, 0, cfg_lin)
    r_quad = compute_reward(True, 1.0, 0.0, 0, cfg_quad)
    assert math.isclose(r_lin, r_quad, abs_tol=1e-9)


def test_reward_failure_ignores_quadratic_mode() -> None:
    """Failure always returns -0.5 regardless of cost_penalty_mode."""
    cfg = RoutingConfig(cost_penalty_mode="quadratic")
    reward = compute_reward(False, 1.0, 0.05, 0, cfg)
    assert reward == -0.5
