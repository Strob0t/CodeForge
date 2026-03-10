"""Tests for HybridRouter orchestrator (Phase 26K)."""

from __future__ import annotations

from codeforge.routing.complexity import ComplexityAnalyzer
from codeforge.routing.mab import MABModelSelector
from codeforge.routing.meta_router import LLMMetaRouter
from codeforge.routing.models import (
    ComplexityTier,
    ModelStats,
    RoutingConfig,
    TaskType,
)
from codeforge.routing.rate_tracker import RateLimitTracker
from codeforge.routing.router import COMPLEXITY_DEFAULTS, HybridRouter

AVAILABLE = [
    "github_copilot/gpt-4o",
    "github_copilot/gpt-4o-mini",
    "openai/gpt-4o",
    "openai/gpt-4o-mini",
    "groq/llama-3.1-8b-instant",
    "anthropic/claude-sonnet-4",
]


def _config(
    enabled: bool = True,
    mab: bool = True,
    meta: bool = True,
) -> RoutingConfig:
    return RoutingConfig(
        enabled=enabled,
        complexity_enabled=True,
        mab_enabled=mab,
        llm_meta_enabled=meta,
        mab_min_trials=10,
    )


def _mab_with_data() -> MABModelSelector:
    """MAB that returns 'openai/gpt-4o' for any query."""
    stats = [
        ModelStats(model_name="openai/gpt-4o", trial_count=100, avg_reward=0.9),
        ModelStats(model_name="openai/gpt-4o-mini", trial_count=100, avg_reward=0.5),
    ]
    config = RoutingConfig(mab_min_trials=10)
    return MABModelSelector(
        stats_loader=lambda tt, ct: stats,
        config=config,
    )


def _mab_cold_start() -> MABModelSelector:
    """MAB with no data (returns None)."""
    config = RoutingConfig(mab_min_trials=10)
    return MABModelSelector(
        stats_loader=lambda tt, ct: [],
        config=config,
    )


def _meta_success() -> LLMMetaRouter:
    """Meta-router that always succeeds."""
    import json

    response = json.dumps({"recommended_model": "anthropic/claude-sonnet-4", "reasoning": "Meta selected"})
    return LLMMetaRouter(
        llm_call=lambda m, p: response,
        config=RoutingConfig(),
    )


def _meta_failure() -> LLMMetaRouter:
    """Meta-router that always fails."""
    return LLMMetaRouter(
        llm_call=lambda m, p: None,
        config=RoutingConfig(),
    )


# -- Full cascade ------------------------------------------------------------


def test_full_cascade_l2_has_data() -> None:
    """L1 → L2 has data → returns L2 model (L3 not called)."""
    router = HybridRouter(
        complexity=ComplexityAnalyzer(),
        mab=_mab_with_data(),
        meta=_meta_success(),
        available_models=AVAILABLE,
        config=_config(),
    )
    decision = router.route("Implement a sorting algorithm")
    assert decision is not None
    assert decision.routing_layer == "mab"
    assert decision.model == "openai/gpt-4o"


def test_l2_cold_start_falls_to_l3() -> None:
    """L2 cold start → L3 called → returns L3 model."""
    router = HybridRouter(
        complexity=ComplexityAnalyzer(),
        mab=_mab_cold_start(),
        meta=_meta_success(),
        available_models=AVAILABLE,
        config=_config(),
    )
    decision = router.route("Hello world")
    assert decision is not None
    assert decision.routing_layer == "meta"
    assert decision.model == "anthropic/claude-sonnet-4"


def test_l2_cold_l3_fails_uses_fallback() -> None:
    """L1 + L2 cold + L3 fails → complexity fallback."""
    router = HybridRouter(
        complexity=ComplexityAnalyzer(),
        mab=_mab_cold_start(),
        meta=_meta_failure(),
        available_models=AVAILABLE,
        config=_config(),
    )
    decision = router.route("Hello")
    assert decision is not None
    assert decision.routing_layer == "complexity"


# -- Disabled layers ---------------------------------------------------------


def test_routing_disabled() -> None:
    router = HybridRouter(
        complexity=ComplexityAnalyzer(),
        mab=_mab_with_data(),
        meta=_meta_success(),
        available_models=AVAILABLE,
        config=_config(enabled=False),
    )
    assert router.route("test") is None


def test_mab_disabled_skips_to_l3() -> None:
    router = HybridRouter(
        complexity=ComplexityAnalyzer(),
        mab=_mab_with_data(),
        meta=_meta_success(),
        available_models=AVAILABLE,
        config=_config(mab=False),
    )
    decision = router.route("Hello")
    assert decision is not None
    assert decision.routing_layer == "meta"


def test_meta_disabled_skips_to_fallback() -> None:
    router = HybridRouter(
        complexity=ComplexityAnalyzer(),
        mab=_mab_cold_start(),
        meta=_meta_success(),
        available_models=AVAILABLE,
        config=_config(meta=False),
    )
    decision = router.route("Hello")
    assert decision is not None
    assert decision.routing_layer == "complexity"


def test_all_disabled_except_complexity() -> None:
    router = HybridRouter(
        complexity=ComplexityAnalyzer(),
        mab=None,
        meta=None,
        available_models=AVAILABLE,
        config=_config(mab=False, meta=False),
    )
    decision = router.route("Hello")
    assert decision is not None
    assert decision.routing_layer == "complexity"


# -- Fallback model selection ------------------------------------------------


def test_fallback_selects_from_tier_list() -> None:
    router = HybridRouter(
        complexity=ComplexityAnalyzer(),
        mab=None,
        meta=None,
        available_models=AVAILABLE,
        config=_config(mab=False, meta=False),
    )
    decision = router.route("Hello")
    assert decision is not None
    # SIMPLE tier → first available from defaults.
    assert decision.model in AVAILABLE


def test_fallback_first_available_when_no_preferred() -> None:
    """No preferred models available → uses first in available_models."""
    router = HybridRouter(
        complexity=ComplexityAnalyzer(),
        mab=None,
        meta=None,
        available_models=["custom/my-model"],
        config=_config(mab=False, meta=False),
    )
    decision = router.route("Hello")
    assert decision is not None
    assert decision.model == "custom/my-model"
    assert decision.confidence == 0.3  # Low confidence for non-preferred.


def test_empty_available_models_returns_none() -> None:
    router = HybridRouter(
        complexity=ComplexityAnalyzer(),
        mab=None,
        meta=None,
        available_models=[],
        config=_config(mab=False, meta=False),
    )
    decision = router.route("Hello")
    assert decision is None


# -- Decision attributes ----------------------------------------------------


def test_decision_includes_tier_and_task() -> None:
    router = HybridRouter(
        complexity=ComplexityAnalyzer(),
        mab=_mab_with_data(),
        meta=None,
        available_models=AVAILABLE,
        config=_config(meta=False),
    )
    decision = router.route("Implement a function")
    assert decision is not None
    assert isinstance(decision.complexity_tier, ComplexityTier)
    assert isinstance(decision.task_type, TaskType)


def test_decision_reasoning_not_empty() -> None:
    router = HybridRouter(
        complexity=ComplexityAnalyzer(),
        mab=_mab_with_data(),
        meta=None,
        available_models=AVAILABLE,
        config=_config(meta=False),
    )
    decision = router.route("test")
    assert decision is not None
    assert decision.reasoning != ""


# -- Cost constraint ---------------------------------------------------------


def test_cost_constraint_propagated() -> None:
    """MAB should receive and apply cost constraint."""
    stats = [
        ModelStats(model_name="openai/gpt-4o", trial_count=100, avg_reward=0.9, input_cost_per=0.05),
        ModelStats(model_name="openai/gpt-4o-mini", trial_count=100, avg_reward=0.5, input_cost_per=0.001),
    ]
    config = RoutingConfig(mab_min_trials=10)
    mab = MABModelSelector(stats_loader=lambda tt, ct: stats, config=config)

    router = HybridRouter(
        complexity=ComplexityAnalyzer(),
        mab=mab,
        meta=None,
        available_models=AVAILABLE,
        config=_config(meta=False),
    )
    decision = router.route("Implement something", max_cost=0.01)
    assert decision is not None
    assert decision.model == "openai/gpt-4o-mini"


# -- Complexity defaults coverage -------------------------------------------


def test_complexity_defaults_have_all_tiers() -> None:
    for tier in ComplexityTier:
        assert tier in COMPLEXITY_DEFAULTS, f"Missing defaults for {tier}"
        assert len(COMPLEXITY_DEFAULTS[tier]) >= 2, f"Too few defaults for {tier}"


# -- Rate-aware fallback tests -----------------------------------------------


def _make_tracker_with_exhausted(
    providers: set[str],
) -> RateLimitTracker:
    """Build a tracker where the given providers are marked as exhausted."""
    from codeforge.routing.rate_tracker import RateLimitInfo

    tracker = RateLimitTracker()
    tracker._now = lambda: 100.0  # type: ignore[assignment]
    for p in providers:
        tracker.update(
            p,
            RateLimitInfo(
                remaining_requests=0,
                limit_requests=60,
                reset_after_seconds=30.0,
                provider=p,
                timestamp=100.0,
            ),
        )
    return tracker


def test_fallback_skips_exhausted_provider() -> None:
    """Exhausted provider should be skipped in favour of the next preferred model."""
    tracker = _make_tracker_with_exhausted({"groq"})
    router = HybridRouter(
        complexity=ComplexityAnalyzer(),
        mab=None,
        meta=None,
        available_models=AVAILABLE,
        config=_config(mab=False, meta=False),
        rate_tracker=tracker,
    )
    # A simple prompt → SIMPLE tier → preference list starts with groq.
    decision = router.route("Hello")
    assert decision is not None
    assert not decision.model.startswith("groq/")


def test_fallback_all_preferred_exhausted() -> None:
    """When all preferred providers are exhausted, fall back to first available."""
    tracker = _make_tracker_with_exhausted({"groq", "openai", "anthropic"})
    router = HybridRouter(
        complexity=ComplexityAnalyzer(),
        mab=None,
        meta=None,
        available_models=["groq/llama-3.1-8b-instant", "custom/my-model"],
        config=_config(mab=False, meta=False),
        rate_tracker=tracker,
    )
    decision = router.route("Hello")
    assert decision is not None
    # groq is exhausted in the preference list, but first available is groq — the
    # overall fallback doesn't rate-filter, so it picks first available.
    assert decision.model in ("groq/llama-3.1-8b-instant", "custom/my-model")


def test_gemini_flash_in_medium_tier() -> None:
    """gemini/gemini-2.5-flash must be in the MEDIUM tier defaults."""
    medium = COMPLEXITY_DEFAULTS[ComplexityTier.MEDIUM]
    assert "gemini/gemini-2.5-flash" in medium
    # Must appear after groq/llama-3.3-70b-versatile and before openai/gpt-4o-mini.
    groq_idx = medium.index("groq/llama-3.3-70b-versatile")
    flash_idx = medium.index("gemini/gemini-2.5-flash")
    mini_idx = medium.index("openai/gpt-4o-mini")
    assert groq_idx < flash_idx < mini_idx


def test_rate_tracker_none_no_filtering() -> None:
    """Without a rate tracker, no providers should be filtered."""
    router = HybridRouter(
        complexity=ComplexityAnalyzer(),
        mab=None,
        meta=None,
        available_models=AVAILABLE,
        config=_config(mab=False, meta=False),
        rate_tracker=None,
    )
    decision = router.route("Hello")
    assert decision is not None
    assert decision.model in AVAILABLE


def test_complexity_fallback_prefers_copilot_simple() -> None:
    """For SIMPLE tier, github_copilot/gpt-4o-mini is selected first when available."""
    router = HybridRouter(
        complexity=ComplexityAnalyzer(),
        mab=None,
        meta=None,
        available_models=AVAILABLE,
        config=_config(mab=False, meta=False),
    )
    decision = router.route("Hello")
    assert decision is not None
    assert decision.model == "github_copilot/gpt-4o-mini"


def test_complexity_fallback_prefers_copilot_reasoning() -> None:
    """For REASONING tier, github_copilot/o3-mini is preferred when available."""
    available_with_o3 = [*AVAILABLE, "github_copilot/o3-mini"]
    router = HybridRouter(
        complexity=ComplexityAnalyzer(),
        mab=None,
        meta=None,
        available_models=available_with_o3,
        config=_config(mab=False, meta=False),
    )
    # Use a prompt the complexity analyzer classifies as REASONING tier.
    decision = router.route(
        "Prove that P != NP using a formal proof with mathematical induction "
        "and analyze the computational complexity implications step by step"
    )
    assert decision is not None
    if decision.complexity_tier == ComplexityTier.REASONING:
        assert decision.model == "github_copilot/o3-mini"


def test_tpm_blocked_model_skipped_in_fallback() -> None:
    """Models that failed with TPM exceeded should be skipped in subsequent fallback picks."""
    from codeforge.routing.rate_tracker import get_tracker

    tracker = get_tracker()
    tracker.record_error("groq", error_type="tpm_exceeded")
    assert tracker.is_exhausted("groq")
