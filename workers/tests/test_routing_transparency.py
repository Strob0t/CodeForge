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


# ---------------------------------------------------------------------------
# C1.5 — Routing metadata trajectory event published from agent loop
# ---------------------------------------------------------------------------


class TestRoutingMetadataTrajectoryEvent:
    """Verify routing metadata is published as trajectory event during agent loop."""

    @pytest.fixture
    def _mock_resolve_model(self):
        from unittest.mock import patch

        with patch("codeforge.agent_loop.resolve_model", return_value="fake-model"):
            yield

    def _make_runtime(self) -> object:
        from unittest.mock import AsyncMock, MagicMock

        from codeforge.models import ToolCallDecision

        runtime = MagicMock()
        runtime.run_id = "run-1"
        runtime.project_id = "proj-1"
        runtime.is_cancelled = False
        runtime.send_output = AsyncMock()
        runtime.request_tool_call = AsyncMock(
            return_value=ToolCallDecision(call_id="tc-1", decision="allow", reason="")
        )
        runtime.report_tool_result = AsyncMock()
        runtime.publish_trajectory_event = AsyncMock()
        return runtime

    @pytest.mark.usefixtures("_mock_resolve_model")
    async def test_routing_metadata_event_published_on_first_iteration(self) -> None:
        """When routing_layer is set, a trajectory.routing_decision event is published."""
        from unittest.mock import AsyncMock

        from codeforge.agent_loop import AgentLoopExecutor, LoopConfig
        from codeforge.llm import ChatCompletionResponse
        from codeforge.tools import ToolRegistry

        runtime = self._make_runtime()
        llm = AsyncMock()
        llm.chat_completion_stream = AsyncMock(
            return_value=ChatCompletionResponse(
                content="Hello!",
                tool_calls=[],
                finish_reason="stop",
                tokens_in=10,
                tokens_out=5,
                model="openai/gpt-4o-mini",
                cost_usd=0.001,
            )
        )
        registry = ToolRegistry()
        executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
        cfg = LoopConfig(
            model="openai/gpt-4o-mini",
            routing_layer="complexity",
            complexity_tier="simple",
            task_type="code",
        )

        await executor.run([{"role": "user", "content": "Hi"}], config=cfg)

        # Find the routing_decision trajectory event among all published events.
        routing_events = [
            call.args[0]
            for call in runtime.publish_trajectory_event.call_args_list
            if isinstance(call.args[0], dict) and call.args[0].get("event_type") == "trajectory.routing_decision"
        ]
        assert len(routing_events) >= 1, (
            "Expected at least one trajectory.routing_decision event, "
            f"got events: {[c.args[0].get('event_type') for c in runtime.publish_trajectory_event.call_args_list]}"
        )
        event = routing_events[0]
        assert event["selected_model"] == "openai/gpt-4o-mini"
        assert event["complexity_tier"] == "simple"
        assert "reason" in event

    @pytest.mark.usefixtures("_mock_resolve_model")
    async def test_no_routing_event_when_routing_layer_empty(self) -> None:
        """When routing_layer is empty, no trajectory.routing_decision event is published."""
        from unittest.mock import AsyncMock

        from codeforge.agent_loop import AgentLoopExecutor, LoopConfig
        from codeforge.llm import ChatCompletionResponse
        from codeforge.tools import ToolRegistry

        runtime = self._make_runtime()
        llm = AsyncMock()
        llm.chat_completion_stream = AsyncMock(
            return_value=ChatCompletionResponse(
                content="Hello!",
                tool_calls=[],
                finish_reason="stop",
                tokens_in=10,
                tokens_out=5,
                model="fake-model",
                cost_usd=0.001,
            )
        )
        registry = ToolRegistry()
        executor = AgentLoopExecutor(llm, registry, runtime, "/tmp/workspace")
        cfg = LoopConfig(model="fake-model")  # No routing_layer

        await executor.run([{"role": "user", "content": "Hi"}], config=cfg)

        routing_events = [
            call.args[0]
            for call in runtime.publish_trajectory_event.call_args_list
            if isinstance(call.args[0], dict) and call.args[0].get("event_type") == "trajectory.routing_decision"
        ]
        assert len(routing_events) == 0


# ---------------------------------------------------------------------------
# C1.6 — Model switch publishes routing trajectory event
# ---------------------------------------------------------------------------


class TestModelSwitchTrajectoryEvent:
    """Verify that check_model_switch publishes a trajectory event."""

    def test_check_model_switch_bumps_tier_and_logs(self) -> None:
        """check_model_switch should bump complexity_tier on cfg when quality is low."""
        from codeforge.agent_loop import LoopConfig
        from codeforge.loop_helpers import check_model_switch
        from codeforge.quality_tracking import IterationQualityTracker

        tracker = IterationQualityTracker()
        cfg = LoopConfig(
            model="openai/gpt-4o-mini",
            routing_layer="complexity",
            complexity_tier="simple",
            task_type="code",
        )
        # Simulate 2 low-quality iterations.
        for _ in range(2):
            tracker.record(tool_success=False, output_length=0)
            tracker.record(tool_success=False, output_length=0)
            tracker.record(tool_success=False, output_length=0)
            tracker.end_iteration()

        assert tracker.should_switch() is True
        check_model_switch(tracker, cfg)
        assert cfg.complexity_tier == "medium"
        assert tracker.switch_count == 1

    def test_check_model_switch_noop_without_routing_layer(self) -> None:
        """check_model_switch does nothing when routing_layer is empty."""
        from codeforge.agent_loop import LoopConfig
        from codeforge.loop_helpers import check_model_switch
        from codeforge.quality_tracking import IterationQualityTracker

        tracker = IterationQualityTracker()
        cfg = LoopConfig(model="fake-model", routing_layer="", complexity_tier="simple")
        # Simulate 2 low-quality iterations.
        for _ in range(2):
            tracker.record(tool_success=False, output_length=0)
            tracker.record(tool_success=False, output_length=0)
            tracker.record(tool_success=False, output_length=0)
            tracker.end_iteration()

        assert tracker.should_switch() is True
        check_model_switch(tracker, cfg)
        # No switch because routing_layer is empty.
        assert cfg.complexity_tier == "simple"
        assert tracker.switch_count == 0


# ---------------------------------------------------------------------------
# C1.7 — resolve_model_with_routing returns metadata
# ---------------------------------------------------------------------------


class TestResolveModelWithRoutingMetadata:
    """Verify resolve_model_with_routing populates routing_metadata when available."""

    def test_resolve_with_router_returns_metadata(self) -> None:
        """When HybridRouter is provided, result includes routing_metadata."""
        from codeforge.llm import resolve_model_with_routing
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
        result = resolve_model_with_routing(
            prompt="Write a hello world program",
            scenario="",
            router=router,
        )
        assert result.model != ""
        assert result.routing_layer != ""
        assert result.routing_metadata is not None
        assert result.routing_metadata.selected_model == result.model
        assert isinstance(result.routing_metadata.reason, str)

    def test_resolve_without_router_has_no_metadata(self) -> None:
        """Without a router, routing_metadata is None."""
        from codeforge.llm import resolve_model_with_routing

        result = resolve_model_with_routing(prompt="hello", scenario="", router=None)
        assert result.routing_metadata is None

    def test_resolve_with_disabled_router_has_no_metadata(self) -> None:
        """With a disabled router, routing_metadata is None."""
        from codeforge.llm import resolve_model_with_routing
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
        result = resolve_model_with_routing(prompt="hello", scenario="", router=router)
        assert result.routing_metadata is None
