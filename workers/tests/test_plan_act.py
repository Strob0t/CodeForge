"""Tests for Plan/Act mode toggle (A3).

TDD RED phase: these tests define the expected behavior of PlanActController.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    import pytest


class TestPlanActControllerToolRestriction:
    """A3.1: When in plan phase, write/edit/bash tools are blocked, read-only tools are allowed."""

    def test_plan_phase_allows_read_only_tools(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True)
        assert ctrl.is_tool_allowed("read_file") is True
        assert ctrl.is_tool_allowed("search_files") is True
        assert ctrl.is_tool_allowed("glob_files") is True
        assert ctrl.is_tool_allowed("list_directory") is True

    def test_plan_phase_blocks_write_tools(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True)
        assert ctrl.is_tool_allowed("write_file") is False
        assert ctrl.is_tool_allowed("edit_file") is False
        assert ctrl.is_tool_allowed("bash") is False

    def test_plan_phase_blocks_unknown_tools(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True)
        assert ctrl.is_tool_allowed("some_random_tool") is False

    def test_plan_phase_case_insensitive(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True)
        assert ctrl.is_tool_allowed("Read_File") is True
        assert ctrl.is_tool_allowed("BASH") is False
        assert ctrl.is_tool_allowed("Search_Files") is True


class TestPlanActControllerPhaseTransition:
    """A3.2: Calling transition_to_act() switches phase, all tools become available."""

    def test_transition_enables_all_tools(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True)
        assert ctrl.phase == "plan"
        assert ctrl.is_tool_allowed("write_file") is False

        ctrl.transition_to_act()
        assert ctrl.phase == "act"
        assert ctrl.is_tool_allowed("write_file") is True
        assert ctrl.is_tool_allowed("edit_file") is True
        assert ctrl.is_tool_allowed("bash") is True
        assert ctrl.is_tool_allowed("read_file") is True

    def test_double_transition_is_safe(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True)
        ctrl.transition_to_act()
        ctrl.transition_to_act()  # Should not raise
        assert ctrl.phase == "act"
        assert ctrl.is_tool_allowed("bash") is True


class TestPlanActControllerMaxIterations:
    """A3.3: After max_plan_iterations, auto-transitions to act."""

    def test_auto_transition_at_max(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True, max_plan_iterations=3)
        assert ctrl.tick_and_should_transition() is False  # iteration 1
        assert ctrl.tick_and_should_transition() is False  # iteration 2
        assert ctrl.tick_and_should_transition() is True  # iteration 3 -> auto-transition

    def test_auto_transition_does_not_fire_in_act_phase(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True, max_plan_iterations=2)
        ctrl.transition_to_act()
        assert ctrl.tick_and_should_transition() is False

    def test_auto_transition_with_default_max(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True)
        for _ in range(9):
            assert ctrl.tick_and_should_transition() is False
        assert ctrl.tick_and_should_transition() is True  # iteration 10

    def test_zero_max_iterations_transitions_immediately(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True, max_plan_iterations=0)
        assert ctrl.tick_and_should_transition() is True

    def test_one_max_iteration_transitions_on_first(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True, max_plan_iterations=1)
        assert ctrl.tick_and_should_transition() is True


class TestPlanActControllerDisabled:
    """A3.4: plan_act_enabled=False (autonomy < 4) -- all tools always allowed."""

    def test_disabled_allows_all_tools(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=False)
        assert ctrl.is_tool_allowed("write_file") is True
        assert ctrl.is_tool_allowed("edit_file") is True
        assert ctrl.is_tool_allowed("bash") is True
        assert ctrl.is_tool_allowed("read_file") is True

    def test_disabled_phase_is_act(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=False)
        assert ctrl.phase == "act"

    def test_disabled_auto_transition_never_fires(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=False)
        for _ in range(20):
            assert ctrl.tick_and_should_transition() is False

    def test_disabled_transition_is_noop(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=False)
        ctrl.transition_to_act()  # Should not raise
        assert ctrl.phase == "act"


class TestPlanActControllerRoutingTags:
    """A3.5: Plan phase returns "plan" tag, act phase returns "default"."""

    def test_plan_phase_returns_plan_tag(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True)
        assert ctrl.get_routing_tag() == "plan"

    def test_act_phase_returns_default_tag(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True)
        ctrl.transition_to_act()
        assert ctrl.get_routing_tag() == "default"

    def test_disabled_returns_default_tag(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=False)
        assert ctrl.get_routing_tag() == "default"


class TestPlanActControllerSystemSuffix:
    """System suffix tests: correct phase-specific prompt additions."""

    def test_plan_phase_suffix_mentions_plan(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True)
        suffix = ctrl.get_system_suffix()
        assert "PLAN" in suffix
        assert "read-only" in suffix.lower() or "read_file" in suffix.lower()

    def test_act_phase_suffix_mentions_act(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True)
        ctrl.transition_to_act()
        suffix = ctrl.get_system_suffix()
        assert "ACT" in suffix
        assert "execute" in suffix.lower() or "implement" in suffix.lower()

    def test_disabled_returns_empty_suffix(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=False)
        assert ctrl.get_system_suffix() == ""

    def test_plan_suffix_mentions_transition(self) -> None:
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True)
        suffix = ctrl.get_system_suffix()
        assert "transition_to_act" in suffix


class TestPlanActControllerEnvVar:
    """A3.10: CODEFORGE_PLAN_ACT_MAX_ITERATIONS env var support."""

    def test_env_var_overrides_default(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("CODEFORGE_PLAN_ACT_MAX_ITERATIONS", "5")
        from codeforge.plan_act import get_max_plan_iterations

        assert get_max_plan_iterations() == 5

    def test_env_var_not_set_uses_default(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("CODEFORGE_PLAN_ACT_MAX_ITERATIONS", raising=False)
        from codeforge.plan_act import get_max_plan_iterations

        assert get_max_plan_iterations() == 10

    def test_env_var_invalid_uses_default(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("CODEFORGE_PLAN_ACT_MAX_ITERATIONS", "not_a_number")
        from codeforge.plan_act import get_max_plan_iterations

        assert get_max_plan_iterations() == 10


class TestPlanActPayloadModel:
    """A3.6: plan_act_enabled field on ConversationRunStartMessage."""

    def test_default_is_false(self) -> None:
        from codeforge.models import ConversationRunStartMessage

        msg = ConversationRunStartMessage(
            run_id="r1",
            conversation_id="c1",
            project_id="p1",
            messages=[],
            system_prompt="test",
            model="gpt-4o",
        )
        assert msg.plan_act_enabled is False

    def test_explicit_true(self) -> None:
        from codeforge.models import ConversationRunStartMessage

        msg = ConversationRunStartMessage(
            run_id="r1",
            conversation_id="c1",
            project_id="p1",
            messages=[],
            system_prompt="test",
            model="gpt-4o",
            plan_act_enabled=True,
        )
        assert msg.plan_act_enabled is True

    def test_json_round_trip(self) -> None:
        """Verify the field survives JSON serialization/deserialization."""
        import json

        from codeforge.models import ConversationRunStartMessage

        msg = ConversationRunStartMessage(
            run_id="r1",
            conversation_id="c1",
            project_id="p1",
            messages=[],
            system_prompt="test",
            model="gpt-4o",
            plan_act_enabled=True,
        )
        data = json.loads(msg.model_dump_json())
        assert data["plan_act_enabled"] is True

        # Deserialize back
        msg2 = ConversationRunStartMessage.model_validate(data)
        assert msg2.plan_act_enabled is True

    def test_null_coercion(self) -> None:
        """Go can serialize bool as null; Python should default to False."""
        from codeforge.models import ConversationRunStartMessage

        raw = {
            "run_id": "r1",
            "conversation_id": "c1",
            "project_id": "p1",
            "messages": [],
            "system_prompt": "test",
            "model": "gpt-4o",
            "plan_act_enabled": None,
        }
        # Pydantic should coerce None to False or raise; we want False.
        # If it raises, we need a validator.
        msg = ConversationRunStartMessage.model_validate(raw)
        assert msg.plan_act_enabled is False


class TestPlanActControllerExtraPlanTools:
    """Bug #2: Mode-declared tools should be allowed in PLAN phase."""

    def test_mode_extra_plan_tools_allowed(self) -> None:
        """Mode-declared tools should be allowed in PLAN phase."""
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True, extra_plan_tools=frozenset({"propose_goal", "write_file"}))
        assert ctrl.is_tool_allowed("read_file") is True  # base PLAN tool
        assert ctrl.is_tool_allowed("propose_goal") is True  # mode extra tool
        assert ctrl.is_tool_allowed("write_file") is True  # mode extra tool
        assert ctrl.is_tool_allowed("bash") is False  # still blocked

    def test_no_extra_plan_tools_default(self) -> None:
        """Without extra tools, behavior is unchanged."""
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True)
        assert ctrl.is_tool_allowed("propose_goal") is False
        assert ctrl.is_tool_allowed("read_file") is True

    def test_extra_plan_tools_empty_frozenset(self) -> None:
        """Empty frozenset behaves like no extras."""
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True, extra_plan_tools=frozenset())
        assert ctrl.is_tool_allowed("propose_goal") is False
        assert ctrl.is_tool_allowed("read_file") is True

    def test_extra_plan_tools_ignored_in_act_phase(self) -> None:
        """Extra tools parameter has no effect in act phase (all tools already allowed)."""
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True, extra_plan_tools=frozenset({"propose_goal"}))
        ctrl.transition_to_act()
        assert ctrl.is_tool_allowed("bash") is True
        assert ctrl.is_tool_allowed("propose_goal") is True

    def test_extra_plan_tools_ignored_when_disabled(self) -> None:
        """Extra tools parameter has no effect when plan/act is disabled."""
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=False, extra_plan_tools=frozenset({"propose_goal"}))
        assert ctrl.is_tool_allowed("bash") is True
        assert ctrl.is_tool_allowed("propose_goal") is True

    def test_system_suffix_mentions_extra_tools(self) -> None:
        """PLAN suffix should mention extra tools when present."""
        from codeforge.plan_act import PlanActController

        ctrl = PlanActController(enabled=True, extra_plan_tools=frozenset({"propose_goal"}))
        suffix = ctrl.get_system_suffix()
        assert "propose_goal" in suffix
