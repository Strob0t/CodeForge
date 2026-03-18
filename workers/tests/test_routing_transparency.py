"""Tests for C1: Routing Transparency + Mid-Loop Model Switching."""

from __future__ import annotations

import pytest

from codeforge.routing.models import ComplexityTier

# ---------------------------------------------------------------------------
# C1.1 — RoutingMetadata published as trajectory event
# ---------------------------------------------------------------------------


class TestRoutingMetadata:
    """Verify that HybridRouter.route() returns RoutingMetadata alongside the decision."""

    def test_route_returns_routing_metadata(self) -> None:
        """After a route() call, RoutingMetadata contains tier, model, reason, alternatives."""
        from codeforge.routing.complexity import ComplexityAnalyzer
        from codeforge.routing.models import RoutingConfig
        from codeforge.routing.router import HybridRouter

        router = HybridRouter(
            complexity=ComplexityAnalyzer(),
            mab=None,
            meta=None,
            available_models=["openai/gpt-4o-mini", "anthropic/claude-haiku-3.5"],
            config=RoutingConfig(enabled=True, mab_enabled=False, llm_meta_enabled=False),
        )
        decision, metadata = router.route_with_metadata("Write a hello world program")
        assert decision is not None
        assert metadata is not None
        assert metadata.complexity_tier in (
            ComplexityTier.SIMPLE,
            ComplexityTier.MEDIUM,
            ComplexityTier.COMPLEX,
            ComplexityTier.REASONING,
        )
        assert metadata.selected_model == decision.model
        assert isinstance(metadata.reason, str)
        assert len(metadata.reason) > 0

    def test_routing_metadata_contains_alternatives(self) -> None:
        """Metadata alternatives list contains at least 1 other model with score."""
        from codeforge.routing.complexity import ComplexityAnalyzer
        from codeforge.routing.models import RoutingConfig
        from codeforge.routing.router import HybridRouter

        router = HybridRouter(
            complexity=ComplexityAnalyzer(),
            mab=None,
            meta=None,
            available_models=["openai/gpt-4o-mini", "anthropic/claude-haiku-3.5", "gemini/gemini-2.0-flash"],
            config=RoutingConfig(enabled=True, mab_enabled=False, llm_meta_enabled=False),
        )
        _decision, metadata = router.route_with_metadata("Write a hello world program")
        assert len(metadata.alternatives) >= 1
        for alt in metadata.alternatives:
            assert "model" in alt
            assert "score" in alt
            assert isinstance(alt["score"], (int, float))

    def test_routing_metadata_disabled_returns_none(self) -> None:
        """When routing is disabled, route_with_metadata returns (None, None)."""
        from codeforge.routing.complexity import ComplexityAnalyzer
        from codeforge.routing.models import RoutingConfig
        from codeforge.routing.router import HybridRouter

        router = HybridRouter(
            complexity=ComplexityAnalyzer(),
            mab=None,
            meta=None,
            available_models=["openai/gpt-4o-mini"],
            config=RoutingConfig(enabled=False),
        )
        decision, metadata = router.route_with_metadata("hello")
        assert decision is None
        assert metadata is None


# ---------------------------------------------------------------------------
# C1.2 — Iteration quality signal computation
# ---------------------------------------------------------------------------


class TestIterationQualityTracker:
    """Verify quality signal computation from tool call outcomes."""

    def _make_tracker(self) -> object:
        from codeforge.agent_loop import IterationQualityTracker

        return IterationQualityTracker()

    def test_all_successful_tool_calls(self) -> None:
        """3 successful tool calls -> signal = 1.0."""
        tracker = self._make_tracker()
        tracker.record(tool_success=True, output_length=100)
        tracker.record(tool_success=True, output_length=200)
        tracker.record(tool_success=True, output_length=150)
        assert tracker.signal() == pytest.approx(1.0)

    def test_all_failed_tool_calls(self) -> None:
        """3 failed tool calls -> signal = 0.0."""
        tracker = self._make_tracker()
        tracker.record(tool_success=False, output_length=0)
        tracker.record(tool_success=False, output_length=0)
        tracker.record(tool_success=False, output_length=0)
        assert tracker.signal() == pytest.approx(0.0)

    def test_mixed_tool_calls(self) -> None:
        """1 success, 1 fail, 1 empty output -> signal approx 0.33."""
        tracker = self._make_tracker()
        tracker.record(tool_success=True, output_length=100)
        tracker.record(tool_success=False, output_length=0)
        tracker.record(tool_success=True, output_length=0)  # empty output = partial success
        # success gives 1.0, fail gives 0.0, empty-output success gives 0.0
        assert 0.2 <= tracker.signal() <= 0.5

    def test_no_tool_calls(self) -> None:
        """No tool calls (pure text response) -> signal = 0.5 (neutral)."""
        tracker = self._make_tracker()
        assert tracker.signal() == pytest.approx(0.5)

    def test_single_tool_call(self) -> None:
        """Single successful call -> signal = 1.0."""
        tracker = self._make_tracker()
        tracker.record(tool_success=True, output_length=100)
        assert tracker.signal() == pytest.approx(1.0)

    def test_window_limited_to_last_3(self) -> None:
        """Signal only considers last 3 records."""
        tracker = self._make_tracker()
        tracker.record(tool_success=False, output_length=0)
        tracker.record(tool_success=False, output_length=0)
        tracker.record(tool_success=True, output_length=100)
        tracker.record(tool_success=True, output_length=200)
        tracker.record(tool_success=True, output_length=150)
        # Last 3 are all successful
        assert tracker.signal() == pytest.approx(1.0)


# ---------------------------------------------------------------------------
# C1.3 — Model switch triggers on low quality signal
# ---------------------------------------------------------------------------


class TestModelSwitchTrigger:
    """Verify model switch logic based on quality signals."""

    def _make_tracker(self) -> object:
        from codeforge.agent_loop import IterationQualityTracker

        return IterationQualityTracker()

    def test_two_consecutive_low_triggers_switch(self) -> None:
        """2 consecutive iterations with signal < 0.3 -> model switch requested."""
        tracker = self._make_tracker()
        # Iteration 1: low quality
        tracker.record(tool_success=False, output_length=0)
        tracker.record(tool_success=False, output_length=0)
        tracker.record(tool_success=False, output_length=0)
        tracker.end_iteration()
        # Iteration 2: low quality again
        tracker.record(tool_success=False, output_length=0)
        tracker.record(tool_success=False, output_length=0)
        tracker.record(tool_success=False, output_length=0)
        tracker.end_iteration()
        assert tracker.should_switch() is True

    def test_one_low_one_high_no_switch(self) -> None:
        """1 low + 1 high -> no switch (not consecutive)."""
        tracker = self._make_tracker()
        # Iteration 1: low
        tracker.record(tool_success=False, output_length=0)
        tracker.record(tool_success=False, output_length=0)
        tracker.record(tool_success=False, output_length=0)
        tracker.end_iteration()
        # Iteration 2: high
        tracker.record(tool_success=True, output_length=200)
        tracker.record(tool_success=True, output_length=200)
        tracker.record(tool_success=True, output_length=200)
        tracker.end_iteration()
        assert tracker.should_switch() is False

    def test_tier_bumped_on_switch(self) -> None:
        """Complexity tier bumped from SIMPLE to MEDIUM on switch."""
        from codeforge.agent_loop import IterationQualityTracker

        tracker = IterationQualityTracker()
        current_tier = ComplexityTier.SIMPLE
        bumped = tracker.bump_tier(current_tier)
        assert bumped == ComplexityTier.MEDIUM

    def test_tier_bump_caps_at_reasoning(self) -> None:
        """Tier bump at REASONING stays at REASONING."""
        from codeforge.agent_loop import IterationQualityTracker

        tracker = IterationQualityTracker()
        bumped = tracker.bump_tier(ComplexityTier.REASONING)
        assert bumped == ComplexityTier.REASONING


# ---------------------------------------------------------------------------
# C1.4 — Max 2 model switches per loop
# ---------------------------------------------------------------------------


class TestMaxModelSwitches:
    """Verify max 2 model switches per loop."""

    def test_third_switch_ignored(self) -> None:
        """3rd switch attempt -> ignored, should_switch returns False."""
        from codeforge.agent_loop import IterationQualityTracker

        tracker = IterationQualityTracker()
        tracker.switch_count = 2  # Already switched twice
        # Even with low quality, should not switch
        tracker.record(tool_success=False, output_length=0)
        tracker.record(tool_success=False, output_length=0)
        tracker.record(tool_success=False, output_length=0)
        tracker.end_iteration()
        tracker.record(tool_success=False, output_length=0)
        tracker.record(tool_success=False, output_length=0)
        tracker.record(tool_success=False, output_length=0)
        tracker.end_iteration()
        assert tracker.should_switch() is False

    def test_switch_count_tracked(self) -> None:
        """Switch count is properly tracked."""
        from codeforge.agent_loop import IterationQualityTracker

        tracker = IterationQualityTracker()
        assert tracker.switch_count == 0
        tracker.register_switch()
        assert tracker.switch_count == 1
        tracker.register_switch()
        assert tracker.switch_count == 2

    def test_max_switches_constant(self) -> None:
        """MAX_SWITCHES is 2."""
        from codeforge.agent_loop import IterationQualityTracker

        assert IterationQualityTracker.MAX_SWITCHES == 2
