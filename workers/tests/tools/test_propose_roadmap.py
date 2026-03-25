"""Tests for the propose_roadmap tool."""

from __future__ import annotations

from unittest.mock import AsyncMock

import pytest

from codeforge.tools.propose_roadmap import PROPOSE_ROADMAP_DEFINITION, ProposeRoadmapExecutor


class TestProposeRoadmapDefinition:
    def test_name(self):
        assert PROPOSE_ROADMAP_DEFINITION.name == "propose_roadmap"

    def test_required_fields(self):
        assert "required" in PROPOSE_ROADMAP_DEFINITION.parameters
        assert set(PROPOSE_ROADMAP_DEFINITION.parameters["required"]) == {"action", "milestone_title"}

    def test_action_enum(self):
        props = PROPOSE_ROADMAP_DEFINITION.parameters["properties"]
        assert props["action"]["enum"] == ["create_milestone", "create_step"]

    def test_complexity_enum(self):
        props = PROPOSE_ROADMAP_DEFINITION.parameters["properties"]
        assert props["complexity"]["enum"] == ["trivial", "simple", "medium", "complex"]


class TestProposeRoadmapExecutor:
    @pytest.fixture
    def runtime(self):
        rt = AsyncMock()
        rt.publish_trajectory_event = AsyncMock()
        return rt

    @pytest.fixture
    def executor(self, runtime):
        return ProposeRoadmapExecutor(runtime=runtime)

    @pytest.mark.asyncio
    async def test_propose_milestone(self, executor, runtime):
        result = await executor.execute(
            {"action": "create_milestone", "milestone_title": "Auth", "milestone_description": "JWT auth"},
            "/workspace",
        )
        assert result.success is not False
        assert "Auth" in result.output
        runtime.publish_trajectory_event.assert_called_once()
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["event_type"] == "agent.roadmap_proposed"
        assert event["data"]["action"] == "create_milestone"

    @pytest.mark.asyncio
    async def test_propose_step_maps_complexity_to_model_tier(self, executor, runtime):
        result = await executor.execute(
            {"action": "create_step", "milestone_title": "Auth", "step_title": "User model", "complexity": "simple"},
            "/workspace",
        )
        assert "User model" in result.output
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["data"]["step_model_tier"] == "weak"

    @pytest.mark.asyncio
    async def test_propose_step_complex_routes_to_strong(self, executor, runtime):
        await executor.execute(
            {"action": "create_step", "milestone_title": "Auth", "step_title": "Auth arch", "complexity": "complex"},
            "/workspace",
        )
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["data"]["step_model_tier"] == "strong"

    @pytest.mark.asyncio
    async def test_propose_step_trivial_routes_to_weak(self, executor, runtime):
        await executor.execute(
            {"action": "create_step", "milestone_title": "Auth", "step_title": "Rename var", "complexity": "trivial"},
            "/workspace",
        )
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["data"]["step_model_tier"] == "weak"

    @pytest.mark.asyncio
    async def test_propose_step_medium_routes_to_mid(self, executor, runtime):
        await executor.execute(
            {"action": "create_step", "milestone_title": "Auth", "step_title": "Add endpoint", "complexity": "medium"},
            "/workspace",
        )
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["data"]["step_model_tier"] == "mid"

    @pytest.mark.asyncio
    async def test_missing_milestone_title(self, executor, runtime):
        result = await executor.execute({"action": "create_milestone"}, "/workspace")
        assert result.success is False
        runtime.publish_trajectory_event.assert_not_called()

    @pytest.mark.asyncio
    async def test_create_step_missing_step_title(self, executor, runtime):
        result = await executor.execute(
            {"action": "create_step", "milestone_title": "Auth"},
            "/workspace",
        )
        assert result.success is False
        runtime.publish_trajectory_event.assert_not_called()

    @pytest.mark.asyncio
    async def test_unknown_action(self, executor, runtime):
        result = await executor.execute(
            {"action": "invalid", "milestone_title": "Auth"},
            "/workspace",
        )
        assert result.success is False
        runtime.publish_trajectory_event.assert_not_called()

    @pytest.mark.asyncio
    async def test_proposal_id_is_uuid(self, executor, runtime):
        await executor.execute(
            {"action": "create_milestone", "milestone_title": "Auth", "milestone_description": "JWT"},
            "/workspace",
        )
        event = runtime.publish_trajectory_event.call_args[0][0]
        import uuid

        uuid.UUID(event["data"]["proposal_id"])  # raises if not valid UUID

    @pytest.mark.asyncio
    async def test_milestone_description_defaults_to_empty(self, executor, runtime):
        await executor.execute(
            {"action": "create_milestone", "milestone_title": "Auth"},
            "/workspace",
        )
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["data"]["milestone_description"] == ""

    @pytest.mark.asyncio
    async def test_step_default_complexity_maps_to_mid(self, executor, runtime):
        await executor.execute(
            {"action": "create_step", "milestone_title": "Auth", "step_title": "Something"},
            "/workspace",
        )
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["data"]["step_model_tier"] == "mid"
