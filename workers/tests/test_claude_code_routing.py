"""Tests for Claude Code routing integration."""

from __future__ import annotations

from codeforge.routing.complexity import ComplexityAnalyzer
from codeforge.routing.models import ComplexityTier, RoutingConfig
from codeforge.routing.router import COMPLEXITY_DEFAULTS, HybridRouter


class TestClaudeCodeInDefaults:
    def test_in_complex_tier(self) -> None:
        assert "claudecode/default" in COMPLEXITY_DEFAULTS[ComplexityTier.COMPLEX]

    def test_in_reasoning_tier(self) -> None:
        assert "claudecode/default" in COMPLEXITY_DEFAULTS[ComplexityTier.REASONING]

    def test_not_in_simple(self) -> None:
        assert "claudecode/default" not in COMPLEXITY_DEFAULTS[ComplexityTier.SIMPLE]

    def test_not_in_medium(self) -> None:
        assert "claudecode/default" not in COMPLEXITY_DEFAULTS[ComplexityTier.MEDIUM]

    def test_first_in_complex(self) -> None:
        assert COMPLEXITY_DEFAULTS[ComplexityTier.COMPLEX][0] == "claudecode/default"

    def test_first_in_reasoning(self) -> None:
        assert COMPLEXITY_DEFAULTS[ComplexityTier.REASONING][0] == "claudecode/default"


class TestClaudeCodeRoutingSelection:
    def test_selects_claudecode_for_complex_when_available(self) -> None:
        router = HybridRouter(
            complexity=ComplexityAnalyzer(),
            mab=None,
            meta=None,
            available_models=["claudecode/default", "openai/gpt-4o"],
            config=RoutingConfig(enabled=True),
        )
        decision = router.route(
            "Analyze the microservice architecture, review API design patterns, "
            "refactor the database layer, and implement comprehensive integration tests "
            "with error handling for all edge cases."
        )
        assert decision is not None
        assert decision.model == "claudecode/default"

    def test_skips_claudecode_when_not_available(self) -> None:
        router = HybridRouter(
            complexity=ComplexityAnalyzer(),
            mab=None,
            meta=None,
            available_models=["openai/gpt-4o", "anthropic/claude-sonnet-4"],
            config=RoutingConfig(enabled=True),
        )
        decision = router.route(
            "Analyze the microservice architecture, review API design patterns, "
            "refactor the database layer, and implement comprehensive integration tests."
        )
        assert decision is not None
        assert decision.model != "claudecode/default"

    def test_fallback_chain_has_litellm_after_claudecode(self) -> None:
        router = HybridRouter(
            complexity=ComplexityAnalyzer(),
            mab=None,
            meta=None,
            available_models=["claudecode/default", "openai/gpt-4o", "anthropic/claude-sonnet-4"],
            config=RoutingConfig(enabled=True),
        )
        plan = router.route_with_fallbacks(
            "Analyze microservice architecture and refactor the database layer "
            "with comprehensive testing and error handling.",
        )
        assert plan.primary is not None
        assert plan.primary.model == "claudecode/default"
        assert len(plan.fallbacks) > 0
        assert all(not f.startswith("claudecode/") for f in plan.fallbacks)
